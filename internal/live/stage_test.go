package live

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/schema"
	"burnmon/internal/store"
)

// TestApplyStages guards the Station's live fields: one LatestToolCalls read
// for every running session and subagent, classified by internal/stage.
// Claude's tool_calls.turn is the event's own RequestID, so "in the latest
// turn" is a key match. Codex's is a rollout turn_id that never equals its
// event keys (ordinal or byte offset), and Codex writes a response's
// token_count only after that response's tool output, so for a call whose
// turn key matches no turn of the session, "in the latest turn" means newer
// than the second-newest turn.
func TestApplyStages(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	at := func(sec int) time.Time { return now.Add(time.Duration(-sec) * time.Second) }
	ev := func(vendor, agent, sid, parent, key string, sec int, ctx int64) schema.Event {
		return schema.Event{Vendor: vendor, Agent: agent, Surface: "cli", SessionID: sid, ParentID: parent,
			RequestID: key, Model: "claude-sonnet-5", At: at(sec), Input: ctx, Output: 10}
	}
	call := func(vendor, sid, id, turn, tool string, sec int, done bool) schema.ToolCall {
		c := schema.ToolCall{Vendor: vendor, Agent: "x", SessionID: sid, CallID: id, Turn: turn, Tool: tool, At: at(sec), InputBytes: 1}
		if done {
			c.ResultBytes = ptr(5)
		}
		return c
	}
	events := []schema.Event{
		ev("anthropic", "claude-code", "read", "", "r1", 60, 1000),
		ev("anthropic", "claude-code", "read", "", "r2", 3, 1000),
		ev("anthropic", "claude-code", "sub", "read", "s0", 40, 1000),
		ev("anthropic", "claude-code", "sub", "read", "s1", 2, 1000),
		ev("anthropic", "claude-code", "past", "", "p1", 60, 1000),
		ev("anthropic", "claude-code", "past", "", "p2", 2, 1000),
		ev("anthropic", "claude-code", "think", "", "t1", 70, 1000),
		ev("anthropic", "claude-code", "think", "", "t2", 20, 1000),
		ev("anthropic", "claude-code", "compact", "", "c1", 50, 100000),
		ev("anthropic", "claude-code", "compact", "", "c2", 20, 20000),
		ev("anthropic", "claude-code", "quiet", "", "q0", 60, 1000),
		ev("anthropic", "claude-code", "quiet", "", "q1", 3, 1000),
		ev("openai", "codex", "cx-run", "", "cx-run:b100", 90, 1000),
		ev("openai", "codex", "cx-run", "", "cx-run:b200", 40, 1000),
		ev("openai", "codex", "cx-done", "", "cx-done:b100", 90, 1000),
		ev("openai", "codex", "cx-done", "", "cx-done:b200", 40, 1000),
		ev("openai", "codex", "cx-done", "", "cx-done:b300", 4, 1000),
	}
	calls := []schema.ToolCall{
		call("anthropic", "read", "a1", "r1", "Edit", 60, true),
		call("anthropic", "read", "a2", "r2", "Read", 3, false),
		call("anthropic", "sub", "b1", "s1", "Grep", 2, false),
		call("anthropic", "past", "p", "p1", "Edit", 60, true),
		call("anthropic", "think", "t", "t2", "Bash", 20, true),
		call("openai", "cx-run", "x1", "turn-abc", "exec", 5, false),
		call("openai", "cx-done", "y1", "turn-def", "exec", 50, true),
	}

	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.UpsertEvents(events); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertToolCalls(calls); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig()
	snap := BuildSnapshot(events, cfg, now)
	if err := ApplySessionTotals(snap.Sessions, st, cfg); err != nil {
		t.Fatal(err)
	}
	if err := ApplyStages(snap.Sessions, st, now); err != nil {
		t.Fatal(err)
	}

	byID := map[string]*Session{}
	var walk func(list []*Session)
	walk = func(list []*Session) {
		for _, s := range list {
			byID[s.SessionID] = s
			walk(s.Subagents)
		}
	}
	walk(snap.Sessions)

	want := []struct {
		id, stage, tool string
		since           time.Time
	}{
		{"read", "reading", "Read", at(3)},
		{"sub", "reading", "Grep", at(2)},
		{"past", "waiting", "", at(2)},
		{"think", "thinking", "Bash", at(20).Add(8 * time.Second)},
		{"compact", "compacting", "", at(20)},
		{"quiet", "waiting", "", at(3)},
		{"cx-run", "running", "exec", at(5)},
		{"cx-done", "waiting", "", at(4)},
	}
	for _, w := range want {
		s := byID[w.id]
		if s == nil {
			t.Errorf("%s: not in the snapshot", w.id)
			continue
		}
		if s.Stage != w.stage || s.StageTool != w.tool {
			t.Errorf("%s: stage %q tool %q, want %q %q", w.id, s.Stage, s.StageTool, w.stage, w.tool)
		}
		if got, err := time.Parse(time.RFC3339, s.StageSince); err != nil || !got.Equal(w.since) {
			t.Errorf("%s: stage_since %q, want %s", w.id, s.StageSince, w.since.Format(time.RFC3339))
		}
	}

	raw, err := json.Marshal(byID["read"])
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["stage"] != "reading" || m["stage_tool"] != "Read" || m["stage_since"] == nil {
		t.Fatalf("json names: got stage=%v stage_tool=%v stage_since=%v", m["stage"], m["stage_tool"], m["stage_since"])
	}
}

// TestApplyStagesAskOpen: AskOpen is true only for an AskUserQuestion or
// ExitPlanMode call in the newest turn that has no result yet.
func TestApplyStagesAskOpen(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	at := func(sec int) time.Time { return now.Add(time.Duration(-sec) * time.Second) }
	ev := func(sid, key string, sec int) schema.Event {
		return schema.Event{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: sid,
			RequestID: key, Model: "claude-sonnet-5", At: at(sec), Input: 1000, Output: 10}
	}
	call := func(sid, turn, tool string, done bool) schema.ToolCall {
		c := schema.ToolCall{Vendor: "anthropic", Agent: "claude-code", SessionID: sid, CallID: sid + "-c", Turn: turn, Tool: tool, At: at(3), InputBytes: 1}
		if done {
			c.ResultBytes = ptr(5)
		}
		return c
	}
	events := []schema.Event{
		ev("ask", "a1", 40), ev("ask", "a2", 4),
		ev("answered", "b1", 40), ev("answered", "b2", 4),
		ev("plan", "c1", 40), ev("plan", "c2", 4),
		ev("read", "d1", 40), ev("read", "d2", 4),
		ev("old", "e1", 40), ev("old", "e2", 2),
	}
	calls := []schema.ToolCall{
		call("ask", "a2", "AskUserQuestion", false),
		call("answered", "b2", "AskUserQuestion", true),
		call("plan", "c2", "ExitPlanMode", false),
		call("read", "d2", "Read", false),
		call("old", "e1", "AskUserQuestion", false),
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.UpsertEvents(events); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertToolCalls(calls); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	snap := BuildSnapshot(events, cfg, now)
	if err := ApplySessionTotals(snap.Sessions, st, cfg); err != nil {
		t.Fatal(err)
	}
	if err := ApplyStages(snap.Sessions, st, now); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"ask": true, "answered": false, "plan": true, "read": false, "old": false}
	for _, s := range snap.Sessions {
		if w, ok := want[s.SessionID]; ok && s.AskOpen != w {
			t.Errorf("%s: AskOpen = %v, want %v", s.SessionID, s.AskOpen, w)
		}
	}
}

// TestApplyStagesLeavesSnapshotWithoutStagesUntouched: burnmon.exe and the
// CLI never call ApplyStages, so their JSON carries no stage fields at all.
func TestApplyStagesLeavesSnapshotWithoutStagesUntouched(t *testing.T) {
	now := time.Now().UTC()
	snap := BuildSnapshot([]schema.Event{{Vendor: "anthropic", Agent: "claude-code", SessionID: "a", RequestID: "r",
		Model: "claude-sonnet-5", At: now.Add(-time.Second), Input: 1, Output: 1}}, testConfig(), now)
	raw, _ := json.Marshal(snap.Sessions[0])
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	for _, k := range []string{"stage", "stage_since", "stage_tool"} {
		if _, ok := m[k]; ok {
			t.Errorf("%s present without ApplyStages", k)
		}
	}
}

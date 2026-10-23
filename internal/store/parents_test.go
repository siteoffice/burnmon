package store

import (
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/schema"
)

// R6: Claude subagent rows ingested before the adapter fix have no
// parent_id. The backfill takes the parent from the cursor path
// (<parent>\subagents\agent-<id>.jsonl), counts first on a dry run, and
// only writes when asked.
func TestBackfillClaudeParents(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	at := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)
	ev := func(sid, rid string) schema.Event {
		return schema.Event{Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
			SessionID: sid, RequestID: rid, Model: "claude-x", At: at, Input: 1, Output: 1}
	}
	if err := st.UpsertEvents([]schema.Event{
		ev("agent-abc", "r1"), ev("agent-abc", "r2"), ev("parent-1", "r3"), ev("agent-orphan", "r4"),
	}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	sub := filepath.Join(root, "proj", "parent-1", "subagents", "agent-abc.jsonl")
	main := filepath.Join(root, "proj", "parent-1.jsonl")
	for _, p := range []string{sub, main} {
		if err := st.SetCursor(p, 10, at, 10); err != nil {
			t.Fatal(err)
		}
	}

	n, err := st.BackfillClaudeParents(false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("dry run counted %d rows, want 2 (both agent-abc rows)", n)
	}
	got, _ := st.EventsForSession("agent-abc")
	for _, e := range got {
		if e.ParentID != "" {
			t.Fatalf("dry run wrote parent_id %q", e.ParentID)
		}
	}

	n, err = st.BackfillClaudeParents(true)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("apply updated %d rows, want 2", n)
	}
	got, _ = st.EventsForSession("agent-abc")
	for _, e := range got {
		if e.ParentID != "parent-1" {
			t.Fatalf("parent_id = %q after apply, want parent-1", e.ParentID)
		}
	}
	if orphan, _ := st.EventsForSession("agent-orphan"); orphan[0].ParentID != "" {
		t.Fatalf("a subagent with no cursor got parent %q", orphan[0].ParentID)
	}

	if n, err = st.BackfillClaudeParents(true); err != nil || n != 0 {
		t.Fatalf("second apply = %d, %v; want 0, nil", n, err)
	}
}

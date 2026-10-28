//go:build windows

// Windows only: the Live test runs the snapshot writer, which is built for Windows.

package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burnmon/internal/agentmap"
	"burnmon/internal/graph"
	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/snapshot"
	"burnmon/internal/snapshotwriter"
	"burnmon/internal/store"
	"burnmon/internal/sysmon"
	"burnmon/internal/waits"
)

// blockedWant is the amber count of blockedFixture: the open gate, the capped
// sprint's test check, the failed sprint's test and verify checks, and the
// linked flow: its pending gate G3 and the blocked session "blk" that works
// for it (a frame that waits on you and holds a blocked session).
const blockedWant = 6

type blockedState struct {
	root    string
	now     time.Time
	sprints []graph.Sprint
	flows   []graph.Flow
}

// blockedFixture writes one invented Foreman state: an open gate, a decided
// gate, a closed sprint, a capped sprint with a failed test and a failed
// sprint with a failed test and a failed verify, and a flow with a pending
// gate whose session "blk" is Blocked (see blockedEvents).
func blockedFixture(t *testing.T) blockedState {
	t.Helper()
	root := t.TempDir()
	sprint := func(slug, branch, phase, status, extra string) {
		writeForeman(t, filepath.Join(root, "sprints", "p", slug, "sprint.json"),
			`{"sprint_id":"p__`+slug+`__1","slug":"`+slug+`","worktree_path":"C:\\example\\repo\\.worktrees\\`+slug+`","worktree_branch":"sprint/`+branch+`","phase":"`+phase+`","status":"`+status+`","updated_at":"2026-10-23T10:00:00Z"`+extra+`}`)
	}
	failTest := `,"last_test_result":{"exit_code":1,"summary":"example failure","at":"2026-10-23T10:00:00Z"}`
	failVerify := `,"last_verify_result":{"outcome":"fail","summary":"example failure","at":"2026-10-23T10:00:00Z"}`
	sprint("example-closed", "example-closed", "done", "done", "")
	sprint("example-capped", "example-capped", "dev", "capped", failTest)
	sprint("example-failed", "example-failed", "dev", "failed", failTest+failVerify)
	writeForeman(t, filepath.Join(root, "flows", "p", "example-open", "flow.json"),
		`{"flow_id":"example-open","slug":"example-open","status":"awaiting-owner","gate":{"id":"G2","status":"pending"},"updated_at":"2026-10-23T09:00:00Z"}`)
	writeForeman(t, filepath.Join(root, "flows", "p", "example-decided", "flow.json"),
		`{"flow_id":"example-decided","slug":"example-decided","status":"running","gate":{"id":"G1","status":"decided"},"updated_at":"2026-10-23T09:00:00Z"}`)
	writeForeman(t, filepath.Join(root, "flows", "p", "example-linked", "flow.json"),
		`{"flow_id":"example-linked","slug":"example-linked","status":"awaiting-owner","gate":{"id":"G3","status":"pending"},"updated_at":"2026-10-23T09:00:00Z"}`)
	writeForeman(t, filepath.Join(root, "flows", "p", "example-linked", "foreman.json"), `{"session_id":"blk"}`)
	sprints, flows, err := graph.ReadForeman(root)
	if err != nil {
		t.Fatal(err)
	}
	return blockedState{root: root, now: time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC), sprints: sprints, flows: flows}
}

// blockedEvents are the store rows of session "blk": two turns and an open
// AskUserQuestion in the newest one, so the session is Blocked.
func blockedEvents(now time.Time) ([]schema.Event, []schema.ToolCall) {
	at := func(sec int) time.Time { return now.Add(time.Duration(-sec) * time.Second) }
	ev := func(key string, sec int) schema.Event {
		return schema.Event{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "blk",
			RequestID: key, Model: "claude-sonnet-5", At: at(sec), Input: 1000, Output: 10}
	}
	call := schema.ToolCall{Vendor: "anthropic", Agent: "claude-code", SessionID: "blk", CallID: "blk-c", Turn: "a2",
		Tool: "AskUserQuestion", At: at(3), InputBytes: 1}
	return []schema.Event{ev("a1", 40), ev("a2", 4)}, []schema.ToolCall{call}
}

func openBlockedStore(t *testing.T, now time.Time) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	events, calls := blockedEvents(now)
	if err := st.UpsertEvents(events); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertToolCalls(calls); err != nil {
		t.Fatal(err)
	}
	return st
}

// checkBlockedAgainstCount asserts the four things the spec promises on one
// tab: amber lines into You equal the amber count, the You box says the same
// number, every Blocked item has a box, and a closed sprint or a decided gate
// draws nothing.
func checkBlockedAgainstCount(t *testing.T, tab string, ly agentmap.Layout, items []waits.Item, c waits.Counts) {
	t.Helper()
	if c.Blocked != blockedWant {
		t.Fatalf("%s: Blocked count %d, want %d", tab, c.Blocked, blockedWant)
	}
	amber := 0
	for _, l := range ly.Lines {
		if l.You {
			amber++
		}
	}
	if amber != c.Blocked {
		t.Errorf("%s: %d amber lines into You, Blocked count %d", tab, amber, c.Blocked)
	}
	y, ok := boxIn(ly, "you")
	if want := fmt.Sprintf("%d waiting", c.Blocked); !ok || !strings.HasPrefix(y.Line, want) {
		t.Errorf("%s: You box %q (found %v), want it to start with %q", tab, y.Line, ok, want)
	}
	for _, it := range items {
		if it.Group != waits.GroupBlocked {
			continue
		}
		if !blockedHasBox(ly, it) {
			t.Errorf("%s: Blocked item %s has no box", tab, it.ID)
		}
	}
	for _, l := range ly.Lines {
		if strings.Contains(l.From, "example-closed") || strings.Contains(l.From, "p__example-closed__1") ||
			strings.Contains(l.From, "example-decided") {
			t.Errorf("%s: line %+v from a closed sprint or a decided gate", tab, l)
		}
	}
	for _, b := range ly.Boxes {
		if (strings.Contains(b.ID, "example-closed") || strings.Contains(b.ID, "example-decided")) && b.WaitsOnYou {
			t.Errorf("%s: box %+v is Blocked but its sprint is closed or its gate decided", tab, b)
		}
	}
	for _, i := range ly.Idle {
		if (strings.Contains(i.ID, "example-closed") || strings.Contains(i.ID, "example-decided")) && i.WaitsOnYou {
			t.Errorf("%s: idle row %+v waits on you but its sprint is closed or its gate decided", tab, i)
		}
	}
}

// The Live test runs the real writer (Once builds the graph, the wait items
// and the snapshot file) and then the page's own function, agentMapFor
// "live", over the file it wrote.
func TestBlockedLinesMatchCountLive(t *testing.T) {
	s := blockedFixture(t)
	st := openBlockedStore(t, s.now)
	path := filepath.Join(t.TempDir(), "snapshot.json")
	gr := snapshotwriter.NewGraphReaders(t.TempDir(), s.root)
	gr.Listen = func() ([]graph.Listen, error) { return nil, nil }
	w := &snapshotwriter.Writer{Store: st, Path: path, Tick: snapshotwriter.Tick, Config: pricing.Defaults,
		Procs: func() ([]sysmon.ProcessGroupSample, error) { return nil, nil }, Graph: gr,
		Now: func() time.Time { return s.now }}
	if err := w.Once(); err != nil {
		t.Fatal(err)
	}
	p, err := agentMapFor(agentMapEnv{DataDir: t.TempDir(), SnapPath: path, St: st, ForemanRoot: s.root},
		agentMapRequest{Source: "live"}, s.now)
	if err != nil {
		t.Fatal(err)
	}
	r := snapshot.Load(path, s.now)
	if r.Status != snapshot.StatusOK || r.Snapshot == nil {
		t.Fatalf("snapshot status %q: %s", r.Status, r.Error)
	}
	checkBlockedAgainstCount(t, "live", p.Layout, r.Snapshot.WaitItems, r.Snapshot.WaitCounts)
}

func TestBlockedLinesMatchCountHistory(t *testing.T) {
	s := blockedFixture(t)
	st := openBlockedStore(t, s.now)
	at := s.now.Add(-3 * time.Hour)
	// Lines an earlier tick stored: the closed sprint once waited, and the
	// decided gate was once open. Both are now stale.
	if _, err := st.RecordGraph(
		[]store.GraphNode{
			{ID: "person:you", Kind: graph.KindPerson, Label: "You", Source: graph.SourceStore},
			{ID: "sprint:p__example-closed__1", Kind: graph.KindSprint, Label: "example-closed", Attrs: `{"phase":"done","status":"done"}`, Source: graph.SourceForeman},
			{ID: "gate:example-decided:G1", Kind: graph.KindGate, Label: "G1", Attrs: `{}`, Source: graph.SourceForeman},
			{ID: graph.SessionID("anthropic", "blk"), Kind: graph.KindSession, Label: "blk", Vendor: "anthropic", Attrs: `{}`, Source: graph.SourceStore},
		},
		[]store.GraphEdge{
			{From: "sprint:p__example-closed__1", To: "person:you", Kind: graph.KindWaitsOn, Basis: graph.BasisVendor, Source: graph.SourceForeman},
			{From: "gate:example-decided:G1", To: "person:you", Kind: graph.KindWaitsOn, Basis: graph.BasisVendor, Source: graph.SourceForeman},
			{From: graph.SessionID("anthropic", "blk"), To: "person:you", Kind: graph.KindWaitsOn, Basis: graph.BasisInferred, Source: graph.SourceTranscriptAsk},
		}, at); err != nil {
		t.Fatal(err)
	}
	p, err := agentMapFor(agentMapEnv{DataDir: t.TempDir(), St: st, ForemanRoot: s.root},
		agentMapRequest{Source: "history", Hours: 24}, s.now)
	if err != nil {
		t.Fatal(err)
	}
	// The count comes from the same function over the history graph; the
	// layout checked is the one agentMapFor returned.
	hg, err := graph.History(st, graph.HistoryInput{From: s.now.Add(-24 * time.Hour), To: s.now, Now: s.now,
		Sprints: s.sprints, Flows: s.flows})
	if err != nil {
		t.Fatal(err)
	}
	items, _ := waits.Build(waits.Input{Now: s.now, Sprints: s.sprints, Flows: s.flows, Graph: hg})
	c := waits.CountOf(items)
	checkBlockedAgainstCount(t, "history", p.Layout, items, c)
	if p.Layout.Counts.Waiting != c.Blocked {
		t.Errorf("history: page Blocked %d, count %d", p.Layout.Counts.Waiting, c.Blocked)
	}
}

// An agent view entry whose session has no graph node still counts Blocked,
// so it gets a node, a box and an amber line.
func TestAgentEntryWithoutSessionDrawsLine(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	agent := graph.AgentState{SessionID: "ghost", State: "waiting", Status: "waiting", WaitingFor: "permission prompt"}
	g := graph.Build(graph.Input{Now: now, Agents: []graph.AgentState{agent}})
	items, _ := waits.Build(waits.Input{Now: now, Graph: g, Agents: []waits.AgentEntry{
		{SessionID: "ghost", State: "waiting", Status: "waiting", WaitingFor: "permission prompt"}}})
	c := waits.CountOf(items)
	ly := agentmap.BuildLayout(agentmap.LayoutInput{Nodes: g.Nodes, Edges: g.Edges, Counts: &c})
	amber := 0
	for _, l := range ly.Lines {
		if l.You {
			amber++
		}
	}
	if c.Blocked != 1 || amber != 1 {
		t.Errorf("Blocked %d, amber lines %d, want 1 and 1", c.Blocked, amber)
	}
}

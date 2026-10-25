package store

import (
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/schema"
)

func openGraphStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// R6, migration 8: edges seen only live are kept with first_seen and
// last_seen, so the map can show them after the process is gone.
func TestRecordGraphKeepsFirstSeen(t *testing.T) {
	st := openGraphStore(t)
	t1 := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(5 * time.Second)
	nodes := []GraphNode{
		{ID: "process:2:100", Kind: "process", Label: "claude.exe", Attrs: `{"pid":"2"}`, Source: "sysmon"},
		{ID: "process:5:103", Kind: "process", Label: "bash.exe", Attrs: `{}`, Source: "sysmon"},
	}
	edges := []GraphEdge{{From: "process:2:100", To: "process:5:103", Kind: "started", Basis: "vendor", Source: "sysmon"}}

	first, err := st.RecordGraph(nodes, edges, t1)
	if err != nil {
		t.Fatal(err)
	}
	key := "process:2:100|started|process:5:103"
	if !first[key].Equal(t1) {
		t.Fatalf("first_seen after first record = %v, want %v", first[key], t1)
	}
	first, err = st.RecordGraph(nodes, edges, t2)
	if err != nil {
		t.Fatal(err)
	}
	if !first[key].Equal(t1) {
		t.Fatalf("first_seen after second record = %v, want %v (kept)", first[key], t1)
	}

	gotN, gotE, err := st.GraphBetween(t1.Add(-time.Hour), t2.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(gotE) != 1 || !gotE[0].FirstSeen.Equal(t1) || !gotE[0].LastSeen.Equal(t2) || gotE[0].Basis != "vendor" {
		t.Fatalf("edges = %+v", gotE)
	}
	if len(gotN) != 2 {
		t.Fatalf("nodes = %+v, want both ends", gotN)
	}
	for _, n := range gotN {
		if n.ID == "process:2:100" && (n.Label != "claude.exe" || n.Attrs != `{"pid":"2"}`) {
			t.Fatalf("node = %+v", n)
		}
	}

	// Outside the window: nothing.
	if _, e, _ := st.GraphBetween(t2.Add(time.Minute), t2.Add(time.Hour)); len(e) != 0 {
		t.Fatalf("edge outside the window returned: %+v", e)
	}
}

// History sessions come from the events table, so they survive a deleted
// transcript: one row per session with its parent, project and last turn.
func TestSessionsBetween(t *testing.T) {
	st := openGraphStore(t)
	at := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)
	ev := func(sid, parent, rid string, d time.Duration) schema.Event {
		return schema.Event{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: sid, ParentID: parent,
			RequestID: rid, Model: "m", Project: `C:\w`, At: at.Add(d), Input: 1, Output: 1}
	}
	if err := st.UpsertEvents([]schema.Event{
		ev("S1", "", "r1", 0), ev("S1", "", "r2", time.Minute),
		ev("agent-a", "S1", "r3", 2*time.Minute),
		ev("OLD", "", "r4", -48*time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := st.SessionsBetween(at.Add(-time.Hour), at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("sessions = %+v, want S1 and agent-a", got)
	}
	for _, s := range got {
		switch s.SessionID {
		case "S1":
			if !s.Last.Equal(at.Add(time.Minute)) || !s.First.Equal(at) || s.Project != `C:\w` || s.ParentID != "" {
				t.Fatalf("S1 = %+v", s)
			}
		case "agent-a":
			if s.ParentID != "S1" || !s.First.Equal(at.Add(2*time.Minute)) {
				t.Fatalf("agent-a = %+v", s)
			}
		default:
			t.Fatalf("unexpected session %+v", s)
		}
	}
}

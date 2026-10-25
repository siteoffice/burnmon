package graph

import (
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/schema"
	"burnmon/internal/store"
)

// Only edges the events table cannot rebuild are recorded: a subagent's
// parent and a session's harness come back from events anyway.
func TestRecordedSkipsRebuildableEdges(t *testing.T) {
	g := Build(scenario())
	nodes, edges := g.Recorded()
	kinds := map[string]bool{}
	for _, e := range edges {
		kinds[e.Kind] = true
		if e.Kind == KindSpawned || e.Kind == KindRunsOn || (e.Kind == KindRunsIn && e.To == "harness:claude" && e.From == "session:anthropic:S1") {
			t.Errorf("rebuildable edge recorded: %+v", e)
		}
	}
	for _, k := range []string{KindStarted, KindUses, KindRunsAs, KindListensOn, KindWorksFor, KindWaitsOn, KindTouches} {
		if !kinds[k] {
			t.Errorf("live-only kind %s not recorded", k)
		}
	}
	ids := map[string]bool{}
	for _, n := range nodes {
		ids[n.ID] = true
	}
	for _, e := range edges {
		if !ids[e.From] || !ids[e.To] {
			t.Errorf("recorded edge %s without its nodes", e.Key())
		}
	}
}

// A sprint with its subagents from history: the events give the sessions
// and the parent links, the stored live edges give processes and links that
// were only seen live, and Foreman gives the sprint.
func TestHistory(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	in := scenario()
	wt := in.Sessions[0].Project
	at := t0.Add(-2 * time.Hour)
	if err := st.UpsertEvents([]schema.Event{
		{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "S1", RequestID: "r1", Model: "m", Project: wt, At: at, Input: 1, Output: 1},
		{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "agent-a1", ParentID: "S1", RequestID: "r2", Model: "m", Project: wt, At: at, Input: 1, Output: 1},
	}); err != nil {
		t.Fatal(err)
	}
	// The live graph recorded two hours ago, when the sprint ran.
	live := in
	live.Now = at
	n, e := Build(live).Recorded()
	if _, err := st.RecordGraph(n, e, at); err != nil {
		t.Fatal(err)
	}

	h, err := History(st, HistoryInput{From: t0.Add(-24 * time.Hour), To: t0, Now: t0,
		RepoFor: in.RepoFor, Sprints: in.Sprints, SubagentFor: in.SubagentFor})
	if err != nil {
		t.Fatal(err)
	}
	s1, sub := "session:anthropic:S1", "session:anthropic:agent-a1"
	for _, c := range [][3]string{
		{s1, sub, KindSpawned},
		{s1, "harness:claude", KindRunsIn},
		{s1, "sprint:p__x__1", KindWorksFor},
		{s1, "process:2:100", KindRunsIn},
		{"process:2:100", "process:5:103", KindStarted},
		{s1, "mcp:perplexity", KindUses},
	} {
		if _, ok := edge(h, c[0], c[1], c[2]); !ok {
			t.Errorf("history misses %s -%s-> %s", c[0], c[2], c[1])
		}
	}
	if n, ok := node(h, sub); !ok || n.Kind != KindSubagent || n.Attrs["agent_type"] != "Explore" {
		t.Errorf("subagent node from history = %+v, %v", n, ok)
	}
	if n, ok := node(h, "process:2:100"); !ok || n.AgeSeconds != int64(2*time.Hour/time.Second) {
		t.Errorf("recorded node age = %+v, want two hours", n)
	}
	e2, _ := edge(h, "process:2:100", "process:5:103", KindStarted)
	if e2.FirstSeen != at.Format(time.RFC3339) || e2.LastSeen != at.Format(time.RFC3339) {
		t.Errorf("recorded edge times = %s .. %s", e2.FirstSeen, e2.LastSeen)
	}
	ids := map[string]bool{}
	for _, n := range h.Nodes {
		if ids[n.ID] {
			t.Errorf("duplicate node %s", n.ID)
		}
		ids[n.ID] = true
	}
	for _, e := range h.Edges {
		if !ids[e.From] || !ids[e.To] {
			t.Errorf("history edge %s points outside the graph", e.Key())
		}
	}
}

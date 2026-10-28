//go:build windows

package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"burnmon/internal/agentmap"
	"burnmon/internal/graph"
	"burnmon/internal/schema"
	"burnmon/internal/store"
	"burnmon/internal/waits"
)

func writeForeman(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func waitsOnFrom(edges []graph.Edge) []string {
	var out []string
	for _, e := range edges {
		if e.Kind == graph.KindWaitsOn {
			out = append(out, e.From)
		}
	}
	sort.Strings(out)
	return out
}

// The history map and the live path read the same Foreman state through the
// same merge check: a merged waiting sprint is Done in both, an unmerged one
// and an open gate are Blocked in both, with the same waits_on edges, the same
// count and the same You box line.
func TestHistoryMapMatchesLiveForMergedSprint(t *testing.T) {
	root := t.TempDir()
	writeForeman(t, filepath.Join(root, "sprints", "p", "m", "sprint.json"), `{"sprint_id":"p__m__1","slug":"m","worktree_path":"C:\\example\\repo\\.worktrees\\m","worktree_branch":"sprint/example-merged","phase":"report","status":"waiting-on-you","updated_at":"2026-10-23T10:00:00Z"}`)
	writeForeman(t, filepath.Join(root, "sprints", "p", "u", "sprint.json"), `{"sprint_id":"p__u__1","slug":"u","worktree_path":"C:\\example\\repo\\.worktrees\\u","worktree_branch":"sprint/example-open","phase":"report","status":"waiting-on-you","updated_at":"2026-10-23T10:00:00Z"}`)
	writeForeman(t, filepath.Join(root, "flows", "p", "f", "flow.json"), `{"flow_id":"f-1","slug":"f","status":"awaiting-owner","gate":{"id":"G4","status":"pending"},"updated_at":"2026-10-23T09:00:00Z"}`)
	merged := func(repo, branch string) (bool, bool) {
		if repo != `C:\example\repo` {
			return false, false
		}
		return branch == "sprint/example-merged", true
	}
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	sprints, flows, err := graph.ReadForeman(root)
	if err != nil {
		t.Fatal(err)
	}

	// Live path.
	lg := graph.Build(graph.Input{Now: now, Sprints: sprints, Flows: flows, MergedFor: merged})
	litems, _ := waits.Build(waits.Input{Now: now, Sprints: sprints, Flows: flows, Graph: lg})
	lc := waits.CountOf(litems)
	live := agentmap.BuildLayout(agentmap.LayoutInput{Nodes: lg.Nodes, Edges: lg.Edges, Counts: &lc})

	// History path, through the function the page calls.
	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, err := agentMapFor(agentMapEnv{DataDir: t.TempDir(), St: st, ForemanRoot: root, MergedFor: merged},
		agentMapRequest{Source: "history", Hours: 24}, now)
	if err != nil {
		t.Fatal(err)
	}
	hg, err := graph.History(st, graph.HistoryInput{From: now.Add(-24 * time.Hour), To: now, Now: now,
		Sprints: sprints, Flows: flows, MergedFor: merged})
	if err != nil {
		t.Fatal(err)
	}

	if lc.Blocked != 2 || lc.Done != 1 {
		t.Fatalf("live counts %+v, want blocked 2 (open sprint, gate) and done 1 (merged sprint)", lc)
	}
	if p.Layout.Counts.Waiting != lc.Blocked {
		t.Errorf("history Blocked %d, live %d", p.Layout.Counts.Waiting, lc.Blocked)
	}
	ly, _ := boxIn(live, "you")
	hy, ok := boxIn(p.Layout, "you")
	if !ok || hy.Line != ly.Line {
		t.Errorf("history You box %q (found %v), live %q", hy.Line, ok, ly.Line)
	}
	lw, hw := strings.Join(waitsOnFrom(lg.Edges), ","), strings.Join(waitsOnFrom(hg.Edges), ",")
	if lw != hw || strings.Contains(hw, "p__m__1") || !strings.Contains(hw, "p__u__1") {
		t.Errorf("waits_on edges: history %q, live %q; merged sprint must have none, unmerged one", hw, lw)
	}
	if n, _ := func() (graph.Node, bool) {
		for _, n := range hg.Nodes {
			if n.ID == "sprint:p__m__1" {
				return n, true
			}
		}
		return graph.Node{}, false
	}(); n.Attrs["closeout"] != "true" {
		t.Errorf("history merged sprint node %+v, want closeout", n)
	}
}

// The history map follows the deleted-branch rule the live path does: a
// sprint whose ref is gone is Done when a merge message names it, "branch
// gone, check by hand" (no waits_on edge, not Blocked) when none does.
func TestHistoryMapDeletedBranch(t *testing.T) {
	root := t.TempDir()
	for _, s := range []string{"named", "unnamed"} {
		writeForeman(t, filepath.Join(root, "sprints", "p", s, "sprint.json"), `{"sprint_id":"p__`+s+`__1","slug":"`+s+`","worktree_path":"C:\\example\\repo\\.worktrees\\`+s+`","worktree_branch":"sprint/example-`+s+`","phase":"report","status":"waiting-on-you","updated_at":"2026-10-23T10:00:00Z"}`)
	}
	env := agentMapEnv{DataDir: t.TempDir(), ForemanRoot: root,
		MergedFor:  func(repo, branch string) (bool, bool) { return false, false },
		BranchGone: func(repo, branch string) bool { return repo == `C:\example\repo` },
		MergedByMessage: func(repo, branch string) (bool, bool) {
			return branch == "sprint/example-named", true
		}}
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	env.St = st
	p, err := agentMapFor(env, agentMapRequest{Source: "history", Hours: 24}, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Layout.Counts.Waiting != 0 {
		t.Errorf("history Blocked %d, want 0: a merged-by-message and a branch-gone sprint are both off the amber count", p.Layout.Counts.Waiting)
	}
}

// A stored waits_on edge is only kept when its source still means Blocked
// under B6: a merged sprint and an idle-pid session (written by older ticks
// for every idle session) are not waiters in the history map, an agent view
// blocked session and a transcript ask are. Header count, You box and edge
// set agree.
func TestHistoryMapDropsStaleStoredWaitsOn(t *testing.T) {
	root := t.TempDir()
	writeForeman(t, filepath.Join(root, "sprints", "p", "m", "sprint.json"), `{"sprint_id":"p__m__1","slug":"m","worktree_path":"C:\\example\\repo\\.worktrees\\m","worktree_branch":"sprint/example-merged","phase":"report","status":"waiting-on-you","updated_at":"2026-10-23T10:00:00Z"}`)
	merged := func(repo, branch string) (bool, bool) { return branch == "sprint/example-merged", repo == `C:\example\repo` }
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	at := now.Add(-3 * time.Hour)
	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var evs []schema.Event
	for _, id := range []string{"SIDLE", "SBLOCK", "SASK"} {
		evs = append(evs, schema.Event{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: id, RequestID: "r-" + id, Model: "m", Project: `C:\w\repo`, At: at, Input: 1, Output: 1})
	}
	if err := st.UpsertEvents(evs); err != nil {
		t.Fatal(err)
	}
	idle, blocked, ask := graph.SessionID("anthropic", "SIDLE"), graph.SessionID("anthropic", "SBLOCK"), graph.SessionID("anthropic", "SASK")
	if _, err := st.RecordGraph(
		[]store.GraphNode{{ID: "person:you", Kind: graph.KindPerson, Label: "You", Source: graph.SourceStore}},
		[]store.GraphEdge{
			{From: "sprint:p__m__1", To: "person:you", Kind: graph.KindWaitsOn, Basis: graph.BasisVendor, Source: graph.SourceForeman},
			{From: idle, To: "person:you", Kind: graph.KindWaitsOn, Basis: graph.BasisVendor, Source: graph.SourceClaudePids},
			{From: blocked, To: "person:you", Kind: graph.KindWaitsOn, Basis: graph.BasisVendor, Source: graph.SourceAgentView},
			{From: ask, To: "person:you", Kind: graph.KindWaitsOn, Basis: graph.BasisInferred, Source: graph.SourceTranscriptAsk},
		}, at); err != nil {
		t.Fatal(err)
	}
	env := agentMapEnv{DataDir: t.TempDir(), St: st, ForemanRoot: root, MergedFor: merged}
	p, err := agentMapFor(env, agentMapRequest{Source: "history", Hours: 24}, now)
	if err != nil {
		t.Fatal(err)
	}
	sprints, flows, err := graph.ReadForeman(root)
	if err != nil {
		t.Fatal(err)
	}
	hg, err := graph.History(st, graph.HistoryInput{From: now.Add(-24 * time.Hour), To: now, Now: now,
		Sprints: sprints, Flows: flows, MergedFor: merged})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(waitsOnFrom(hg.Edges), ",")
	if want := ask + "," + blocked; got != want {
		t.Errorf("waits_on edges %q, want %q (agent view and transcript ask only)", got, want)
	}
	if p.Layout.Counts.Waiting != 2 {
		t.Errorf("header Blocked %d, want 2", p.Layout.Counts.Waiting)
	}
	if y, ok := boxIn(p.Layout, "you"); !ok || !strings.HasPrefix(y.Line, "2 waiting") {
		t.Errorf("You box %q (found %v), want 2 waiting", y.Line, ok)
	}
	for _, i := range p.Layout.Idle {
		if i.WaitsOnYou {
			t.Errorf("idle row %+v waits on you; the merged sprint must not", i)
		}
	}
	for _, f := range p.Layout.Frames {
		if f.WaitsOnYou {
			t.Errorf("frame %+v is amber", f)
		}
	}
	for _, l := range p.Layout.Lines {
		if l.You && !strings.Contains(l.From, "SBLOCK") && !strings.Contains(l.From, "SASK") {
			t.Errorf("amber line %+v", l)
		}
	}
	for _, b := range p.Layout.Boxes {
		if strings.Contains(b.ID, "SIDLE") && b.WaitsOnYou {
			t.Errorf("idle-pid session box %+v is Blocked", b)
		}
		if strings.Contains(b.ID, "SBLOCK") && !b.WaitsOnYou {
			t.Errorf("agent view session box %+v is not Blocked", b)
		}
	}
}

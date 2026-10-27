package waits

import (
	"reflect"
	"testing"
	"time"

	"burnmon/internal/graph"
)

func sessNode(id string, age int64, attrs map[string]string) graph.Node {
	return graph.Node{ID: "session:anthropic:" + id, Kind: graph.KindSession, Label: id, Attrs: attrs, Source: "store", AgeSeconds: age}
}

func waitEdge(id string, age int64) graph.Edge {
	return graph.Edge{From: "session:anthropic:" + id, To: personID, Kind: graph.KindWaitsOn, Source: "store", AgeSeconds: age}
}

func ids(items []Item) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

func rankingInput() Input {
	spec := `C:\example\specs\g.md`
	g := graph.Graph{
		Nodes: []graph.Node{
			sessNode("b600", 600, map[string]string{"wait": "blocked"}),
			sessNode("b900", 900, map[string]string{"wait": "blocked"}),
			sessNode("y30", 30, map[string]string{"wait": "your_turn"}),
			sessNode("y300", 300, map[string]string{"wait": "your_turn"}),
			sessNode("cc", 2000, map[string]string{"wait": "your_turn", "can_close": "true"}),
			{ID: "sprint:done1", Kind: graph.KindSprint, Label: "done1", Attrs: map[string]string{"closeout": "true"}, Source: "foreman"},
			{ID: personID, Kind: graph.KindPerson, Label: "You"},
		},
		Edges: []graph.Edge{waitEdge("b600", 600), waitEdge("b900", 900)},
	}
	return Input{
		Now:   now,
		Graph: g,
		Flows: []graph.Flow{{ID: "f1", Slug: "example-flow", Gate: "G4", GatePending: true,
			GateOpenedAt: now.Add(-60 * time.Second), SpecPath: spec}},
		Sprints: []graph.Sprint{
			{ID: "r1", Status: "running", SpecPath: spec},
			{ID: "r2", Status: "running", SpecPath: spec},
			{ID: "r3", Status: "running", SpecPath: spec},
			{ID: "done1", Slug: "done1", Status: "waiting-on-you", UpdatedAt: now.Add(-1000 * time.Second)},
		},
	}
}

func TestRankingOrder(t *testing.T) {
	w, _ := Build(rankingInput())
	want := []string{"gate:f1:G4", "session:anthropic:b900", "session:anthropic:b600",
		"session:anthropic:y300", "session:anthropic:y30", "canclose:session:anthropic:cc", "closeout:done1"}
	if got := ids(w); !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v", got)
	}
	if w[0].Holds != 4 || w[0].Group != GroupBlocked || w[3].Group != GroupYourTurn || w[5].Group != GroupDone {
		t.Fatalf("holds/groups = %+v", w)
	}
}

func TestHoldsCounts(t *testing.T) {
	spec := `C:\example\specs\h.md`
	g := graph.Graph{
		Nodes: []graph.Node{
			sessNode("w1", 5, map[string]string{"wait": "working"}),
			sessNode("w2", 5, map[string]string{"wait": "working"}),
			sessNode("w3", 5, map[string]string{"wait": "working"}),
			sessNode("p", 5, map[string]string{"wait": "blocked"}),
			{ID: "subagent:anthropic:k1", Kind: graph.KindSubagent},
			{ID: "subagent:anthropic:k2", Kind: graph.KindSubagent},
			{ID: personID, Kind: graph.KindPerson},
		},
		Edges: []graph.Edge{
			{From: "session:anthropic:w1", To: "flow:f1", Kind: graph.KindWorksFor},
			{From: "session:anthropic:w2", To: "flow:f1", Kind: graph.KindWorksFor},
			{From: "session:anthropic:w3", To: "sprint:sp1", Kind: graph.KindWorksFor},
			{From: "session:anthropic:p", To: "subagent:anthropic:k1", Kind: graph.KindSpawned},
			{From: "session:anthropic:p", To: "subagent:anthropic:k2", Kind: graph.KindSpawned},
			waitEdge("p", 5),
		},
	}
	in := Input{Now: now, Graph: g,
		Flows: []graph.Flow{{ID: "f1", Slug: "fl", Gate: "G1", GatePending: true, SpecPath: spec, UpdatedAt: now}},
		Sprints: []graph.Sprint{
			{ID: "sp0", Status: "running", SpecPath: spec},
			{ID: "sp1", Slug: "sp1", Status: "waiting-on-you", UpdatedAt: now},
		}}
	w, _ := Build(in)
	holds := map[string]int{}
	for _, it := range w {
		holds[it.ID] = it.Holds
	}
	if holds["gate:f1:G1"] != 4 || holds["sprint:sp1"] != 2 || holds["session:anthropic:p"] != 3 {
		t.Fatalf("holds = %v", holds)
	}
}

// A gate holds only the sprints on its spec that are not finished.
func TestGateHoldsSkipFinishedSprints(t *testing.T) {
	spec := `C:\example\specsin.md`
	var sprints []graph.Sprint
	for i, st := range []string{"running", "paused", "waiting-on-you", "done", "passing", "failed", "capped", "escalated"} {
		sprints = append(sprints, graph.Sprint{ID: "s" + itoa(i), Status: st, SpecPath: spec})
	}
	sprints = append(sprints, graph.Sprint{ID: "other", Status: "running", SpecPath: `C:\example\specs\other.md`})
	in := Input{Now: now, Graph: graph.Graph{Nodes: []graph.Node{{ID: personID, Kind: graph.KindPerson}}},
		Flows:   []graph.Flow{{ID: "f1", Slug: "fl", Gate: "G1", GatePending: true, SpecPath: spec, UpdatedAt: now}},
		Sprints: sprints}
	w, _ := Build(in)
	for _, it := range w {
		if it.ID == "gate:f1:G1" {
			if it.Holds != 4 {
				t.Fatalf("holds = %d, want 4 (the flow plus running, paused, waiting-on-you)", it.Holds)
			}
			return
		}
	}
	t.Fatal("no gate item")
}

func TestCostOfWaiting(t *testing.T) {
	spec := `C:\example\specs\c.md`
	g := graph.Graph{
		Nodes: []graph.Node{
			sessNode("w1", 5, map[string]string{"wait": "working"}),
			{ID: "subagent:anthropic:k1", Kind: graph.KindSubagent},
			{ID: personID, Kind: graph.KindPerson},
		},
		Edges: []graph.Edge{
			{From: "session:anthropic:w1", To: "flow:f1", Kind: graph.KindWorksFor},
			{From: "session:anthropic:w1", To: "subagent:anthropic:k1", Kind: graph.KindSpawned},
		},
	}
	opened := now.Add(-3 * time.Hour)
	in := Input{Now: now, Graph: g,
		Flows: []graph.Flow{{ID: "f1", Slug: "fl", Gate: "G2", GatePending: true, SpecPath: spec, GateOpenedAt: opened}}}
	var gotIDs []string
	var gotSince time.Time
	in.TokensSince = func(ids []string, since time.Time) (int64, bool) {
		gotIDs, gotSince = ids, since
		return 500, true
	}
	w, _ := Build(in)
	if !reflect.DeepEqual(gotIDs, []string{"session:anthropic:w1", "subagent:anthropic:k1"}) || !gotSince.Equal(opened) {
		t.Fatalf("asked %v since %v", gotIDs, gotSince)
	}
	if len(w) != 1 || w[0].CostTokens != 500 || !w[0].CostPartial {
		t.Fatalf("item = %+v", w)
	}
	in.TokensSince = func([]string, time.Time) (int64, bool) { return 0, false }
	w, _ = Build(in)
	if w[0].CostTokens != 0 || w[0].CostPartial {
		t.Fatalf("zero cost = %+v", w[0])
	}
	in.TokensSince = nil
	w, _ = Build(in)
	if w[0].CostTokens != 0 {
		t.Fatalf("nil func = %+v", w[0])
	}
}

func TestYourTurnItems(t *testing.T) {
	g := graph.Graph{Nodes: []graph.Node{
		sessNode("t", 40, map[string]string{"wait": "your_turn"}),
		sessNode("w", 40, map[string]string{"wait": "working"}),
	}}
	w, _ := Build(Input{Now: now, Graph: g})
	if len(w) != 1 || w[0].ID != "session:anthropic:t" || w[0].Group != GroupYourTurn || w[0].Kind != KindYourTurn {
		t.Fatalf("graph = %+v", w)
	}
	w, _ = Build(Input{Now: now, Agents: []AgentEntry{{SessionID: "i", Status: "idle", Cwd: `C:\example\proj`}}})
	if len(w) != 1 || w[0].ID != "session:anthropic:i" || w[0].Group != GroupYourTurn || w[0].Title != "proj" || w[0].Source != "agent_view" {
		t.Fatalf("agent idle = %+v", w)
	}
	w, _ = Build(Input{Now: now, Graph: g, Agents: []AgentEntry{{SessionID: "w", Status: "idle"}}})
	if len(w) != 1 || w[0].ID != "session:anthropic:t" {
		t.Fatalf("idle entry for a working session must not list: %+v", w)
	}
	w, _ = Build(Input{Now: now, Agents: []AgentEntry{
		{SessionID: "p", Status: "waiting", WaitingFor: "permission prompt"},
		{SessionID: "q", Status: "waiting", WaitingFor: "input needed"}}})
	if len(w) != 2 || w[0].Group != GroupBlocked || w[1].Group != GroupBlocked {
		t.Fatalf("blocked entries = %+v", w)
	}
	kinds := map[string]string{}
	for _, it := range w {
		kinds[it.ID] = it.Kind
	}
	if kinds["session:anthropic:p"] != KindPrompt || kinds["session:anthropic:q"] != KindQuestion {
		t.Fatalf("kinds = %v", kinds)
	}
}

func TestCloseOutAndCanClose(t *testing.T) {
	g := graph.Graph{Nodes: []graph.Node{
		{ID: "sprint:s9", Kind: graph.KindSprint, Label: "s9", Attrs: map[string]string{"closeout": "true"}},
		sessNode("c", 700, map[string]string{"wait": "your_turn", "can_close": "true"}),
	}}
	sp := graph.Sprint{ID: "s9", Slug: "example-sprint", Project: "example", Status: "waiting-on-you",
		SpecPath: `C:\example\s.md`, ResultPath: `C:\example\r.md`, UpdatedAt: now.Add(-time.Hour)}
	w, _ := Build(Input{Now: now, Graph: g, Sprints: []graph.Sprint{sp}})
	if len(w) != 2 {
		t.Fatalf("got %+v", w)
	}
	by := map[string]Item{}
	for _, it := range w {
		by[it.ID] = it
	}
	co := by["closeout:s9"]
	if co.Kind != KindCloseOut || co.Group != GroupDone || co.SprintID != "s9" || co.Title != "example-sprint" ||
		co.Detail != "Done, needs close-out" || co.AgeSeconds != 3600 || co.SpecPath != sp.SpecPath {
		t.Fatalf("close_out = %+v", co)
	}
	cc := by["canclose:session:anthropic:c"]
	if cc.Kind != KindCanClose || cc.Group != GroupDone || cc.Detail != "Done, can be closed" {
		t.Fatalf("can_close = %+v", cc)
	}
	if _, dup := by["sprint:s9"]; dup {
		t.Fatalf("sprint item listed too")
	}
}

func TestCountOf(t *testing.T) {
	w, _ := Build(rankingInput())
	if c := CountOf(w); c != (Counts{Blocked: 3, YourTurn: 2, Done: 2}) {
		t.Fatalf("counts = %+v", c)
	}
	if c := CountOf(nil); c != (Counts{}) {
		t.Fatalf("empty = %+v", c)
	}
}

func TestNoForemanInput(t *testing.T) {
	g := graph.Graph{
		Nodes: []graph.Node{
			sessNode("b", 10, map[string]string{"wait": "blocked"}),
			sessNode("y", 10, map[string]string{"wait": "your_turn"}),
			sessNode("c", 10, map[string]string{"wait": "your_turn", "can_close": "true"}),
			{ID: personID, Kind: graph.KindPerson},
		},
		Edges: []graph.Edge{waitEdge("b", 10)},
	}
	w, _ := Build(Input{Now: now, Graph: g})
	if len(w) != 3 {
		t.Fatalf("got %+v", w)
	}
	for _, it := range w {
		switch it.Kind {
		case KindGate, KindSprint, KindFailedCheck, KindCloseOut:
			t.Fatalf("foreman item without foreman: %+v", it)
		}
	}
	if c := CountOf(w); c != (Counts{Blocked: 1, YourTurn: 1, Done: 1}) {
		t.Fatalf("counts = %+v", c)
	}
}

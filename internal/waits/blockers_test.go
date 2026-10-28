package waits

import (
	"sort"
	"testing"

	"burnmon/internal/graph"
)

func TestWaitsBuildUsesBlockers(t *testing.T) {
	failed := &graph.Check{Failed: true, Label: "test exit 1", At: now}
	sprints := []graph.Sprint{
		{ID: "p__example-capped__1", Slug: "example-capped", Status: "capped", UpdatedAt: now, Test: failed},
		{ID: "p__example-wait__1", Slug: "example-wait", Status: "waiting-on-you", UpdatedAt: now},
	}
	flows := []graph.Flow{openFlow()}
	g := graph.Build(graph.Input{Now: now, Sprints: sprints, Flows: flows})
	w, _ := Build(Input{Now: now, Sprints: sprints, Flows: flows, Graph: g})
	got := []string{}
	for _, it := range w {
		if it.Group == GroupBlocked {
			got = append(got, it.ID)
		}
	}
	want := []string{}
	for _, b := range graph.ForemanBlockers(sprints, flows, map[string]bool{}, map[string]bool{}) {
		want = append(want, b.NodeID)
	}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("blocked items %v, blockers %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("blocked items %v, blockers %v", got, want)
			break
		}
	}
}

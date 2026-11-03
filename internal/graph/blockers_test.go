package graph

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"burnmon/internal/store"
)

func blockerFixture() ([]Sprint, []Flow) {
	at := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	fail := &Check{Failed: true, Label: "test exit 1", At: at}
	pass := &Check{Label: "verify pass", At: at}
	sprints := []Sprint{
		{ID: "p__example-closed__1", Slug: "example-closed", Status: "done", Phase: "done", UpdatedAt: at},
		{ID: "p__example-capped__1", Slug: "example-capped", Status: "capped", Phase: "dev", UpdatedAt: at, Test: fail, Verify: pass},
		{ID: "p__example-failed__1", Slug: "example-failed", Status: "failed", Phase: "dev", UpdatedAt: at, Test: fail, Verify: fail},
		{ID: "p__example-quiet__1", Slug: "example-quiet", Status: "failed", Phase: "dev", UpdatedAt: at},
		{ID: "p__example-wait__1", Slug: "example-wait", Status: "waiting-on-you", Phase: "dev", UpdatedAt: at},
		{ID: "p__example-merged__1", Slug: "example-merged", Status: "waiting-on-you", Phase: "dev", UpdatedAt: at},
		{ID: "p__example-gone__1", Slug: "example-gone", Status: "waiting-on-you", Phase: "dev", UpdatedAt: at},
	}
	flows := []Flow{
		{ID: "example-open", Slug: "example-open", Gate: "G2", GatePending: true, UpdatedAt: at},
		{ID: "example-decided", Slug: "example-decided", Gate: "G1", UpdatedAt: at},
	}
	return sprints, flows
}

func blockerIDs(bs []Blocker) []string {
	out := []string{}
	for _, b := range bs {
		out = append(out, b.Kind+" "+b.NodeID)
	}
	return out
}

func TestForemanBlockersRules(t *testing.T) {
	sprints, flows := blockerFixture()
	closeout := map[string]bool{"sprint:p__example-merged__1": true}
	gone := map[string]bool{"sprint:p__example-gone__1": true}
	got := blockerIDs(ForemanBlockers(sprints, flows, closeout, gone, time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)))
	want := []string{
		"gate gate:example-open:G2",
		"failed_check check:p__example-capped__1:test",
		"failed_check check:p__example-failed__1:test",
		"failed_check check:p__example-failed__1:verify",
		"sprint sprint:p__example-wait__1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("blockers\n got %v\nwant %v", got, want)
	}
}

func TestBuildDrawsOneEdgePerBlocker(t *testing.T) {
	sprints, flows := blockerFixture()
	g := Build(Input{Now: time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC), Sprints: sprints, Flows: flows})
	bs := ForemanBlockers(sprints, flows, map[string]bool{}, map[string]bool{}, time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC))
	edges := 0
	for _, e := range g.Edges {
		if e.Kind == KindWaitsOn && e.To == personID {
			edges++
		}
	}
	if edges != len(bs) {
		t.Errorf("%d waits_on edges into You, %d blockers", edges, len(bs))
	}
	for _, b := range bs {
		if _, ok := node(g, b.NodeID); !ok {
			t.Errorf("blocker %s has no node", b.NodeID)
		}
		if _, ok := edge(g, b.NodeID, personID, KindWaitsOn); !ok {
			t.Errorf("blocker %s has no waits_on edge", b.NodeID)
		}
	}
}

func TestHistoryDropsStoredForemanEdges(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	t0 := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	at := t0.Add(-2 * time.Hour)
	nodes := []store.GraphNode{
		{ID: "sprint:p__example-closed__1", Kind: KindSprint, Label: "example-closed", Attrs: "{}", Source: SourceForeman},
		{ID: "gate:example-decided:G1", Kind: KindGate, Label: "G1", Attrs: "{}", Source: SourceForeman},
		{ID: "session:anthropic:S9", Kind: KindSession, Label: "S9", Attrs: "{}", Source: SourceAgentView},
		{ID: personID, Kind: KindPerson, Label: "You", Attrs: "{}", Source: SourceStore},
	}
	edges := []store.GraphEdge{
		{From: "sprint:p__example-closed__1", To: personID, Kind: KindWaitsOn, Basis: BasisVendor, Source: SourceForeman},
		{From: "gate:example-decided:G1", To: personID, Kind: KindWaitsOn, Basis: BasisVendor, Source: SourceForeman},
		{From: "session:anthropic:S9", To: personID, Kind: KindWaitsOn, Basis: BasisVendor, Source: SourceAgentView},
	}
	if _, err := st.RecordGraph(nodes, edges, at); err != nil {
		t.Fatal(err)
	}
	h, err := History(st, HistoryInput{From: t0.Add(-24 * time.Hour), To: t0, Now: t0})
	if err != nil {
		t.Fatal(err)
	}
	for _, from := range []string{"sprint:p__example-closed__1", "gate:example-decided:G1"} {
		if _, ok := edge(h, from, personID, KindWaitsOn); ok {
			t.Errorf("stored foreman edge from %s came back", from)
		}
		if _, ok := node(h, from); ok {
			t.Errorf("stored foreman node %s came back", from)
		}
	}
	if _, ok := edge(h, "session:anthropic:S9", personID, KindWaitsOn); !ok {
		t.Error("stored agent_view session edge was dropped")
	}
}

// A pending gate with no id has no node to carry a waits_on edge, so it is not
// a Blocker: the count and the lines both leave it out, as the B6 map did.
func TestPendingGateWithoutIDIsNotBlocker(t *testing.T) {
	flows := []Flow{{ID: "example-bare", Slug: "example-bare", GatePending: true}}
	if bs := ForemanBlockers(nil, flows, nil, nil, time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)); len(bs) != 0 {
		t.Errorf("blockers = %v, want none", blockerIDs(bs))
	}
	g := Build(Input{Now: time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC), Flows: flows})
	for _, e := range g.Edges {
		if e.Kind == KindWaitsOn {
			t.Errorf("waits_on edge %+v for a gate with no id", e)
		}
	}
}

// A stopped sprint at phase done is closed: its failed check draws no check
// node and no line into You.
func TestBuildDrawsNoCheckForClosedSprint(t *testing.T) {
	at := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	sp := Sprint{ID: "p__example-closed-capped__1", Slug: "example-closed-capped", Status: "capped", Phase: "done",
		UpdatedAt: at.Add(-time.Hour), Test: &Check{Failed: true, Label: "test exit 1", At: at.Add(-time.Hour)}}
	g := Build(Input{Now: at, Sprints: []Sprint{sp}})
	for _, n := range g.Nodes {
		if n.Kind == KindCheck {
			t.Errorf("check node %s drawn for a closed sprint", n.ID)
		}
	}
	for _, e := range g.Edges {
		if e.Kind == KindWaitsOn && e.To == personID {
			t.Errorf("waits_on edge %+v into You for a closed sprint", e)
		}
	}
}

// A stopped sprint gives a check Blocker only while it is open (phase not
// done) and the failed check is at most FailedCheckWindow old.
func TestForemanBlockersStoppedWindow(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	failedAt := func(age time.Duration) *Check {
		return &Check{Failed: true, Label: "exit 1", At: now.Add(-age)}
	}
	const id = "p__example-window__1"
	cases := []struct {
		name string
		sp   Sprint
		want []string
	}{
		{"capped phase done", Sprint{Status: "capped", Phase: "done", Test: failedAt(time.Hour)}, nil},
		{"failed phase done", Sprint{Status: "failed", Phase: "done", Test: failedAt(time.Hour)}, nil},
		{"escalated phase done", Sprint{Status: "escalated", Phase: "done", Test: failedAt(time.Hour)}, nil},
		{"capped dev 1h", Sprint{Status: "capped", Phase: "dev", Test: failedAt(time.Hour)}, []string{"failed_check check:" + id + ":test"}},
		{"failed dev 1h", Sprint{Status: "failed", Phase: "dev", Test: failedAt(time.Hour)}, []string{"failed_check check:" + id + ":test"}},
		{"escalated dev 1h", Sprint{Status: "escalated", Phase: "dev", Test: failedAt(time.Hour)}, []string{"failed_check check:" + id + ":test"}},
		{"capped dev 25h", Sprint{Status: "capped", Phase: "dev", Test: failedAt(25 * time.Hour)}, nil},
		{"failed dev exactly 24h", Sprint{Status: "failed", Phase: "dev", Test: failedAt(24 * time.Hour)}, []string{"failed_check check:" + id + ":test"}},
		{"zero At falls back to UpdatedAt, fresh", Sprint{Status: "capped", Phase: "dev", UpdatedAt: now.Add(-time.Hour), Test: &Check{Failed: true}}, []string{"failed_check check:" + id + ":test"}},
		{"zero At falls back to UpdatedAt, stale", Sprint{Status: "capped", Phase: "dev", UpdatedAt: now.Add(-25 * time.Hour), Test: &Check{Failed: true}}, nil},
		{"test 1h plus verify 25h gives only test", Sprint{Status: "failed", Phase: "dev", Test: failedAt(time.Hour), Verify: failedAt(25 * time.Hour)}, []string{"failed_check check:" + id + ":test"}},
		{"running gives none", Sprint{Status: "running", Phase: "dev", Test: failedAt(time.Hour)}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.sp.ID = id
			c.sp.Slug = "example-window"
			got := blockerIDs(ForemanBlockers([]Sprint{c.sp}, nil, nil, nil, now))
			if len(got) == 0 {
				got = nil
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("blockers\n got %v\nwant %v", got, c.want)
			}
		})
	}
}

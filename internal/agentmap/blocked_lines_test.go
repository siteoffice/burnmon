package agentmap

import (
	"testing"

	"burnmon/internal/graph"
)

// blockedFrame is a flow whose gate G2 waits on you, holding n sessions that
// are each Blocked: every one of them must draw its own amber line into You,
// and so must the gate.
func blockedFrame(n int) ([]graph.Node, []graph.Edge) {
	nodes := []graph.Node{
		{ID: "person:you", Kind: graph.KindPerson, Label: "You"},
		{ID: "flow:f1", Kind: graph.KindFlow, Label: "f1", Attrs: map[string]string{}},
		{ID: "gate:f1:G2", Kind: graph.KindGate, Label: "G2"},
	}
	edges := []graph.Edge{
		{From: "gate:f1:G2", To: "flow:f1", Kind: graph.KindPartOf},
		{From: "gate:f1:G2", To: "person:you", Kind: graph.KindWaitsOn},
	}
	for i := 0; i < n; i++ {
		id := "session:anthropic:s" + string(rune('a'+i))
		nodes = append(nodes, graph.Node{ID: id, Kind: graph.KindSession, Label: id, Attrs: map[string]string{"wait": "blocked"}})
		edges = append(edges, graph.Edge{From: id, To: "flow:f1", Kind: graph.KindWorksFor},
			graph.Edge{From: id, To: "person:you", Kind: graph.KindWaitsOn})
	}
	return nodes, edges
}

// amberLines counts the Blocked items the amber lines into You stand for: a
// folded box's line carries its waiters in Count (1 when absent).
func amberLines(l Layout) int {
	n := 0
	for _, ln := range l.Lines {
		if ln.You {
			if ln.Count > 1 {
				n += ln.Count
			} else {
				n++
			}
		}
	}
	return n
}

func TestFrameWaitingAndHoldingBlockedSessionDrawsBoth(t *testing.T) {
	nodes, edges := blockedFrame(1)
	l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges})
	if got := amberLines(l); got != 2 {
		t.Errorf("%d amber lines, want 2 (the gate and the session)", got)
	}
}

func TestFoldedGroupKeepsWaitersOneLine(t *testing.T) {
	nodes, edges := blockedFrame(2)
	l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges, MaxBoxes: 1})
	var amber []Line
	for _, ln := range l.Lines {
		if ln.You {
			amber = append(amber, ln)
		}
	}
	if len(amber) != 1 || amber[0].Count != 3 || amber[0].Word != "3 waiting" {
		t.Fatalf("amber lines %+v, want one folded line with Count 3 and word %q", amber, "3 waiting")
	}
	if got := amberLines(l); got != 3 {
		t.Errorf("amber count %d, want 3 (the gate and two sessions)", got)
	}
	var folded bool
	for _, f := range l.Frames {
		folded = folded || f.Folded
	}
	if !folded {
		t.Error("the group did not fold")
	}
}

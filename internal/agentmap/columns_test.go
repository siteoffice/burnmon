package agentmap

import (
	"fmt"
	"strings"
	"testing"

	"burnmon/internal/graph"
	"burnmon/internal/waits"
)

const pID = "session:anthropic:p"

func withAttr(n graph.Node, k, v string) graph.Node {
	a := map[string]string{}
	for ak, av := range n.Attrs {
		a[ak] = av
	}
	a[k] = v
	n.Attrs = a
	return n
}

// children builds one parent session with n subagents under it.
func children(n int) ([]graph.Node, []graph.Edge) {
	nodes := []graph.Node{sess(pID, "p", "coding")}
	var edges []graph.Edge
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("session:anthropic:p-k%02d", i)
		nodes = append(nodes, sub(id, "Explore", "coding"))
		edges = append(edges, edge(pID, id, graph.KindSpawned))
	}
	return nodes, edges
}

func TestFiveColumnsWithZeroSubagents(t *testing.T) {
	// One session with one MCP server: no subagents and nobody waiting.
	nodes := []graph.Node{sess(pID, "p", "coding"), {ID: "mcp:example", Kind: graph.KindMCP, Label: "example"},
		{ID: "harness:claude", Kind: graph.KindHarness, Label: "Claude Code"}}
	edges := []graph.Edge{edge(pID, "mcp:example", graph.KindUses), edge(pID, "harness:claude", graph.KindRunsIn)}
	l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges})
	want := []string{"Tool", "Session", "Subagents", "Programs and servers", "You"}
	if strings.Join(l.Columns, "|") != strings.Join(want, "|") {
		t.Fatalf("columns = %v, want %v", l.Columns, want)
	}
	notes := []string{"", "", "none in this window", "", "none in this window"}
	if strings.Join(l.ColumnNotes, "|") != strings.Join(notes, "|") {
		t.Fatalf("column notes = %q, want %q", l.ColumnNotes, notes)
	}
	p := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges, Plain: true})
	wantP := []string{"Tool", "Session", "Helper", "Programs and tool servers", "You"}
	if strings.Join(p.Columns, "|") != strings.Join(wantP, "|") {
		t.Fatalf("plain columns = %v, want %v", p.Columns, wantP)
	}
	if strings.Join(p.ColumnNotes, "|") != strings.Join(notes, "|") {
		t.Errorf("plain column notes = %q", p.ColumnNotes)
	}
}

func TestDeepNestingStaysInColumnTwo(t *testing.T) {
	nodes := []graph.Node{sess(pID, "p", "coding"), sub("session:anthropic:c", "c", "coding"), sub("session:anthropic:g", "g", "coding")}
	edges := []graph.Edge{edge(pID, "session:anthropic:c", graph.KindSpawned), edge("session:anthropic:c", "session:anthropic:g", graph.KindSpawned)}
	l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges})
	c, _ := boxByID(l, "session:anthropic:c")
	g, _ := boxByID(l, "session:anthropic:g")
	if c.Column != 2 || g.Column != 2 || c.Depth != 1 || g.Depth != 2 {
		t.Errorf("child column %d depth %d, grandchild column %d depth %d; want column 2, depths 1 and 2", c.Column, c.Depth, g.Column, g.Depth)
	}
	if len(l.Columns) != 5 {
		t.Errorf("%d columns for a nested tree, want 5", len(l.Columns))
	}
	if s, _ := boxByID(l, pID); s.Depth != 0 {
		t.Errorf("session depth %d, want 0", s.Depth)
	}
}

func TestTenSubagentsDoNotFold(t *testing.T) {
	nodes, edges := children(MaxSubagentBoxes)
	l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges})
	subs := 0
	for _, b := range l.Boxes {
		if b.Kind == "sub" {
			subs++
		}
		if strings.HasPrefix(b.ID, "fold:subs:") {
			t.Errorf("10 subagents folded: %s", b.ID)
		}
	}
	if subs != 10 {
		t.Errorf("%d sub boxes, want 10", subs)
	}
}

func TestElevenSubagentsFold(t *testing.T) {
	nodes, edges := children(MaxSubagentBoxes + 1)
	l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges})
	id := "fold:subs:" + pID
	b, ok := boxByID(l, id)
	if !ok {
		t.Fatalf("no box %s in %+v", id, l.Boxes)
	}
	if b.Kind != "folded" || b.Title != "11 subagents" || b.Line != "click to open" || b.Column != 2 {
		t.Errorf("fold box = %+v", b)
	}
	for _, x := range l.Boxes {
		if x.Kind == "sub" {
			t.Errorf("sub box %s still drawn while folded", x.ID)
		}
	}
	if _, ok := lineOf(l, pID, id, "starts"); !ok {
		t.Error("no starts line from the parent to the fold box")
	}
	p := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges, Plain: true})
	if b, _ := boxByID(p, id); b.Title != "11 helpers" {
		t.Errorf("plain fold title %q, want 11 helpers", b.Title)
	}
	o := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges, Open: map[string]bool{"subs:" + pID: true}})
	subs := 0
	for _, x := range o.Boxes {
		if x.Kind == "sub" {
			subs++
		}
		if x.ID == id {
			t.Error("fold box still drawn after it was opened")
		}
	}
	if subs != 11 {
		t.Errorf("%d sub boxes after opening, want 11", subs)
	}
	seen := map[[2]int]string{}
	for _, x := range o.Boxes {
		k := [2]int{x.Column, x.Row}
		if prev, dup := seen[k]; dup {
			t.Errorf("%s and %s share column %d row %d", prev, x.ID, x.Column, x.Row)
		}
		seen[k] = x.ID
	}
}

func TestLineWordsAndStyles(t *testing.T) {
	const run, done = "session:anthropic:run", "session:anthropic:done"
	nodes := []graph.Node{sess(pID, "p", "coding"), withAttr(sub(run, "r", "coding"), "running", "true"), sub(done, "d", "waiting")}
	edges := []graph.Edge{edge(pID, run, graph.KindSpawned), edge(pID, done, graph.KindSpawned)}
	l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges})
	for _, id := range []string{run, done} {
		ln, ok := lineOf(l, pID, id, "starts")
		if !ok || ln.Style != "solid" {
			t.Errorf("parent to %s: %+v (found %v), want starts, solid", id, ln, ok)
		}
	}
	ln, ok := lineOf(l, run, pID, "reports to")
	if !ok || ln.Style != "dotted" {
		t.Errorf("running child to parent: %+v (found %v), want reports to, dotted", ln, ok)
	}
	if _, ok := lineOf(l, done, pID, "reports to"); ok {
		t.Error("a finished subagent has a reports to line")
	}
	for _, x := range l.Lines {
		if x.Style != "solid" && x.Style != "dotted" {
			t.Errorf("line %+v has style %q", x, x.Style)
		}
	}
}

func TestOnlyBlockedDrawsToYou(t *testing.T) {
	const b, y = "session:anthropic:b", "session:anthropic:y"
	nodes := []graph.Node{
		withAttr(sess(b, "b", "waiting"), "wait", graph.WaitBlocked),
		withAttr(sess(y, "y", "waiting"), "wait", graph.WaitYourTurn),
		{ID: "person:you", Kind: graph.KindPerson, Label: "You"},
	}
	edges := []graph.Edge{edge(b, "person:you", graph.KindWaitsOn)}
	l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges, Counts: &waits.Counts{Blocked: 1, YourTurn: 1}})
	n := 0
	for _, ln := range l.Lines {
		if ln.You {
			n++
			if ln.From != b {
				t.Errorf("You line from %s, want %s", ln.From, b)
			}
		}
	}
	if n != 1 {
		t.Errorf("%d lines to You, want 1", n)
	}
	yb, _ := boxByID(l, y)
	if yb.Wait != "your_turn" || yb.Pill != "your turn" || yb.WaitsOnYou {
		t.Errorf("your turn box = wait %q pill %q waits_on_you %v", yb.Wait, yb.Pill, yb.WaitsOnYou)
	}
	bb, _ := boxByID(l, b)
	if bb.Wait != "blocked" || bb.Pill != "waits on you" || !bb.WaitsOnYou {
		t.Errorf("blocked box = wait %q pill %q waits_on_you %v", bb.Wait, bb.Pill, bb.WaitsOnYou)
	}
}

func TestYouBoxUsesCount(t *testing.T) {
	nodes := []graph.Node{sess(pID, "p", "coding")}
	l := BuildLayout(LayoutInput{Nodes: nodes, Counts: &waits.Counts{Blocked: 2, YourTurn: 3}})
	y, ok := boxByID(l, "you")
	if !ok || y.Line != "2 waiting on you, 3 your turn" {
		t.Fatalf("You box = %+v (found %v)", y, ok)
	}
	if l.Counts.Waiting != 2 || l.Counts.YourTurn != 3 {
		t.Errorf("layout counts %+v, want waiting 2 and your turn 3 whatever the graph says", l.Counts)
	}
	if y.Column != 4 {
		t.Errorf("You box in column %d, want 4", y.Column)
	}
	if l.ColumnNotes[4] != "" {
		t.Errorf("You column note %q with a You box", l.ColumnNotes[4])
	}
	p := BuildLayout(LayoutInput{Nodes: nodes, Counts: &waits.Counts{Blocked: 2, YourTurn: 3}, Plain: true})
	if y, _ := boxByID(p, "you"); y.Line != "2 waiting for you, 3 your turn" {
		t.Errorf("plain You line %q", y.Line)
	}
	z := BuildLayout(LayoutInput{Nodes: nodes})
	if _, ok := boxByID(z, "you"); ok {
		t.Error("a You box with zero counts")
	}
	if z.ColumnNotes[4] != "none in this window" || len(z.Columns) != 5 {
		t.Errorf("zero counts: notes %v, columns %v", z.ColumnNotes, z.Columns)
	}
}

func TestKindTags(t *testing.T) {
	nodes, edges := askingExample(t)
	for _, c := range []struct {
		plain bool
		tags  map[string]string
	}{
		{false, map[string]string{"harness:claude": "tool", parentID: "session", subID: "subagent",
			"mcp:example-server@" + parentID: "MCP", "programs:" + parentID: "programs", "you": "you"}},
		{true, map[string]string{"harness:claude": "app", parentID: "agent", subID: "helper",
			"mcp:example-server@" + parentID: "tool server", "programs:" + parentID: "programs", "you": "you"}},
	} {
		l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges, Plain: c.plain, Counts: &waits.Counts{Blocked: 1}})
		for id, want := range c.tags {
			if b, ok := boxByID(l, id); !ok || b.Tag != want {
				t.Errorf("plain=%v: tag of %s is %q (found %v), want %q", c.plain, id, b.Tag, ok, want)
			}
		}
	}
	gn, ge := groupsOf(5, 10)
	f := BuildLayout(LayoutInput{Nodes: gn, Edges: ge})
	found := false
	for _, b := range f.Boxes {
		if b.Kind == "folded" {
			found = true
			if b.Tag != "group" {
				t.Errorf("folded box tag %q, want group", b.Tag)
			}
		}
	}
	if !found {
		t.Fatal("no folded box")
	}
}

func TestYouBoxAmberOnlyWhenBlocked(t *testing.T) {
	nodes := []graph.Node{sess(pID, "p", "coding")}
	l := BuildLayout(LayoutInput{Nodes: nodes, Counts: &waits.Counts{Blocked: 0, YourTurn: 2}})
	y, ok := boxByID(l, "you")
	if !ok || y.WaitsOnYou || y.Pill != "" {
		t.Errorf("your-turn-only You box = %+v (found %v), want no waits_on_you and no pill", y, ok)
	}
	if y.Line != "0 waiting on you, 2 your turn" {
		t.Errorf("your-turn-only You line %q", y.Line)
	}
	l = BuildLayout(LayoutInput{Nodes: nodes, Counts: &waits.Counts{Blocked: 1, YourTurn: 2}})
	if y, _ := boxByID(l, "you"); !y.WaitsOnYou || y.Pill != "waits on you" {
		t.Errorf("blocked You box = %+v, want waits_on_you and the pill", y)
	}
}

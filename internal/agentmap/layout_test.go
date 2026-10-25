package agentmap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"burnmon/internal/graph"
)

const (
	parentID = "session:anthropic:example-parent"
	subID    = "session:anthropic:example-sub"
)

func boxByID(l Layout, id string) (Box, bool) {
	for _, b := range l.Boxes {
		if b.ID == id {
			return b, true
		}
	}
	return Box{}, false
}

func lineOf(l Layout, from, to, word string) (Line, bool) {
	for _, ln := range l.Lines {
		if ln.From == from && ln.To == to && ln.Word == word {
			return ln, true
		}
	}
	return Line{}, false
}

func fact(b Box, label string) (string, bool) {
	for _, f := range b.Facts {
		if f.Label == label {
			return f.Value, true
		}
	}
	return "", false
}

func exampleLayout(t *testing.T, mut func(*LayoutInput)) Layout {
	t.Helper()
	nodes, edges := fixture(t)
	in := LayoutInput{Nodes: nodes, Edges: edges}
	if mut != nil {
		mut(&in)
	}
	return BuildLayout(in)
}

// The frozen example is one sprint: a Claude Code harness, one session with
// an MCP server and two programs it started, and one subagent that waits on
// you. Boxes sit in columns, the harness first, the tools after the deepest
// subagent column, You last.
func TestLayoutExampleColumnsAndBoxes(t *testing.T) {
	l := exampleLayout(t, nil)
	want := []string{"Tool", "Session", "Subagent", "Programs and servers", "You"}
	if strings.Join(l.Columns, "|") != strings.Join(want, "|") {
		t.Fatalf("columns = %v, want %v", l.Columns, want)
	}
	type at struct {
		id       string
		col, row int
	}
	for _, w := range []at{
		{"harness:claude", 0, 0},
		{parentID, 1, 0},
		{subID, 2, 2},
		{"mcp:example-server@" + parentID, 3, 0},
		{"programs:" + parentID, 3, 1},
		{"you", 4, 2},
	} {
		b, ok := boxByID(l, w.id)
		if !ok {
			t.Errorf("no box %s", w.id)
			continue
		}
		if b.Column != w.col || b.Row != w.row {
			t.Errorf("%s at column %d row %d, want %d, %d", w.id, b.Column, b.Row, w.col, w.row)
		}
	}
	if len(l.Boxes) != 6 {
		t.Errorf("%d boxes, want 6", len(l.Boxes))
	}
	if b, _ := boxByID(l, "programs:"+parentID); b.Title != "2 programs" {
		t.Errorf("programs box title %q, want 2 programs, not one box per process", b.Title)
	}
	if b, _ := boxByID(l, "mcp:example-server@"+parentID); !strings.Contains(b.Line, "port 8765") {
		t.Errorf("mcp line %q lacks the port", b.Line)
	}
}

// Lines carry R6's edge types in plain words, and only waits-on goes to You.
func TestLayoutExampleLines(t *testing.T) {
	l := exampleLayout(t, nil)
	for _, w := range []struct{ from, to, word string }{
		{"harness:claude", parentID, "runs"},
		{parentID, subID, "started"},
		{parentID, "mcp:example-server@" + parentID, "uses"},
		{parentID, "programs:" + parentID, "started"},
		{subID, "you", "waits on"},
	} {
		if _, ok := lineOf(l, w.from, w.to, w.word); !ok {
			t.Errorf("no line %s -[%s]-> %s", w.from, w.word, w.to)
		}
	}
	if len(l.Lines) != 5 {
		t.Errorf("%d lines, want 5: %+v", len(l.Lines), l.Lines)
	}
	if ln, _ := lineOf(l, subID, "you", "waits on"); !ln.You {
		t.Error("the waits-on line to You is not marked")
	}
	for _, ln := range l.Lines {
		if ln.Word != "waits on" && ln.You {
			t.Errorf("line %+v is marked as a You line", ln)
		}
	}
	for _, w := range []string{"runs", "started", "uses", "waits on"} {
		found := false
		for _, ln := range l.Lines {
			found = found || ln.Word == w
		}
		if !found {
			t.Errorf("no line says %q", w)
		}
	}
}

// The sprint is a frame around its agents, with a one-line explanation that
// says who linked them. The harness stays outside the frame.
func TestLayoutExampleFrame(t *testing.T) {
	l := exampleLayout(t, nil)
	if len(l.Frames) != 1 {
		t.Fatalf("%d frames, want 1", len(l.Frames))
	}
	f := l.Frames[0]
	if f.Label != "sprint-example" || f.Kind != "sprint" || f.Folded {
		t.Errorf("frame %+v", f)
	}
	if !strings.Contains(f.Explain, "BurnMon matched") {
		t.Errorf("explain %q does not say the link is BurnMon's guess", f.Explain)
	}
	if f.FirstRow != 0 || f.LastRow != 2 || f.FirstCol != 1 || f.LastCol != 3 {
		t.Errorf("frame span rows %d-%d, columns %d-%d", f.FirstRow, f.LastRow, f.FirstCol, f.LastCol)
	}
	for _, b := range l.Boxes {
		in := b.Group == f.ID
		if b.ID == "harness:claude" || b.ID == "you" {
			in = false
		}
		if in && (b.Column < f.FirstCol || b.Column > f.LastCol) {
			t.Errorf("%s is in the frame but outside its columns", b.ID)
		}
	}
	if b, _ := boxByID(l, "harness:claude"); b.Group != "" {
		t.Error("the harness box belongs to a frame")
	}
}

// Evidence goes in Facts: model, tokens, the labelled price, last activity,
// source and age. A session the snapshot does not list says unknown.
func TestLayoutEvidenceAndUnknown(t *testing.T) {
	l := exampleLayout(t, func(in *LayoutInput) {
		in.Evidence = map[string]Evidence{
			parentID: {Model: "claude-opus-5-5", Tokens: 1200, CostEUR: 2.3, CostLabel: "API price equivalent", Source: "store", AgeSeconds: 20, Known: true},
		}
	})
	p, _ := boxByID(l, parentID)
	for label, want := range map[string]string{
		"Model":                "claude-opus-5-5",
		"Tokens":               "1,200",
		"API price equivalent": "\u20ac2.30",
		"Last active":          "20 s ago",
		"Source":               "store, 20 s old",
		"Works for":            "sprint-example (matched by BurnMon from the folder)",
		"Repo":                 "project",
		"Branch":               "sprint/example",
	} {
		if got, ok := fact(p, label); !ok || got != want {
			t.Errorf("session fact %q = %q (found %v), want %q", label, got, ok, want)
		}
	}
	if !strings.Contains(p.Line, "1,200 tokens") {
		t.Errorf("session line %q lacks the tokens", p.Line)
	}
	s, _ := boxByID(l, subID)
	for _, label := range []string{"Tokens", "API price equivalent", "Source"} {
		if got, _ := fact(s, label); got != "unknown" {
			t.Errorf("subagent without evidence: %q = %q, want unknown", label, got)
		}
	}
	if got, _ := fact(s, "Model"); got != "sonnet" {
		t.Errorf("subagent model %q, want the graph's own (sonnet)", got)
	}
}

// Programs are never one box each: a hundred processes are one box, and its
// evidence lists image names, not command lines.
func TestLayoutProgramsAreOneBox(t *testing.T) {
	nodes := []graph.Node{
		sess("s:a", "a", "coding"),
		{ID: "process:1:1", Kind: graph.KindProcess, Label: "claude.exe"},
	}
	edges := []graph.Edge{edge("s:a", "process:1:1", graph.KindRunsIn)}
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("process:%d:9", 100+i)
		name := "node.exe"
		if i%2 == 0 {
			name = "bash.exe"
		}
		nodes = append(nodes, graph.Node{ID: id, Kind: graph.KindProcess, Label: name,
			Attrs: map[string]string{"cmdline": `C:\secret --token=SECRET`}})
		edges = append(edges, edge("process:1:1", id, graph.KindStarted))
	}
	l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges})
	b, ok := boxByID(l, "programs:s:a")
	if !ok || b.Title != "100 programs" {
		t.Fatalf("programs box = %+v (found %v)", b, ok)
	}
	if len(l.Boxes) != 2 {
		t.Errorf("%d boxes, want the session and one programs box", len(l.Boxes))
	}
	if v, _ := fact(b, "node.exe"); v != "50" {
		t.Errorf("node.exe count %q, want 50", v)
	}
	js, _ := json.Marshal(l)
	if strings.Contains(string(js), "SECRET") || strings.Contains(string(js), "secret") {
		t.Fatal("a command line reached the layout")
	}
}

// No one waits: no You box and no line to it. Something waits: both appear,
// and the box says how many.
func TestLayoutYouOnlyWhenSomethingWaits(t *testing.T) {
	nodes := []graph.Node{sess("s:a", "a", "coding")}
	l := BuildLayout(LayoutInput{Nodes: nodes})
	if _, ok := boxByID(l, "you"); ok || len(l.Lines) != 0 {
		t.Fatalf("You box or lines with nobody waiting: %+v", l)
	}
	nodes = append(nodes, graph.Node{ID: "person:you", Kind: graph.KindPerson, Label: "You"})
	edges := []graph.Edge{edge("s:a", "person:you", graph.KindWaitsOn)}
	l = BuildLayout(LayoutInput{Nodes: nodes, Edges: edges})
	y, ok := boxByID(l, "you")
	if !ok || !strings.Contains(y.Line, "1 waiting") || !y.WaitsOnYou {
		t.Fatalf("You box = %+v (found %v)", y, ok)
	}
	if a, _ := boxByID(l, "s:a"); a.Pill != "waits on you" {
		t.Errorf("a waiting session's pill is %q, want waits on you", a.Pill)
	}
}

// A sprint that waits on you with no session still counts in the You box, and
// is listed in its facts, so five old waiting sprints are not lost.
func TestLayoutIdleWaitingSprintCountsForYou(t *testing.T) {
	nodes := []graph.Node{
		{ID: "sprint:old", Kind: graph.KindSprint, Label: "old", Attrs: map[string]string{"status": "waiting-on-you"}, AgeSeconds: 99},
		{ID: "person:you", Kind: graph.KindPerson, Label: "You"},
	}
	edges := []graph.Edge{edge("sprint:old", "person:you", graph.KindWaitsOn)}
	l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges})
	y, ok := boxByID(l, "you")
	if !ok {
		t.Fatal("no You box for a waiting sprint")
	}
	if v, _ := fact(y, "sprint"); !strings.Contains(v, "old") {
		t.Errorf("You facts %+v do not name the sprint", y.Facts)
	}
	if len(l.Idle) != 1 || len(l.Frames) != 0 {
		t.Errorf("idle %d, frames %d", len(l.Idle), len(l.Frames))
	}
}

// Plain Mode uses plain words in the boxes, the pills, the lines and the frame.
func TestLayoutPlainWords(t *testing.T) {
	l := exampleLayout(t, func(in *LayoutInput) { in.Plain = true })
	if l.Columns[2] != "Helper" || l.Columns[3] != "Programs and tool servers" {
		t.Errorf("plain columns %v", l.Columns)
	}
	if s, _ := boxByID(l, subID); s.Pill != "waiting for you" {
		t.Errorf("plain pill %q", s.Pill)
	}
	if p, _ := boxByID(l, parentID); p.Pill != "writing code" {
		t.Errorf("plain state %q, want writing code", p.Pill)
	}
	if _, ok := lineOf(l, subID, "you", "waiting for"); !ok {
		t.Error("plain line word is not 'waiting for'")
	}
	if f := l.Frames[0]; !strings.Contains(f.Explain, "Foreman job") {
		t.Errorf("plain frame explanation %q", f.Explain)
	}
	// Only what a person reads: the key "subagents" in counts is not shown.
	js, _ := json.Marshal([]any{l.Columns, l.Boxes, l.Lines, l.Frames, l.Notes})
	for _, jargon := range []string{"MCP", "subagent", "waits on"} {
		if strings.Contains(string(js), jargon) {
			t.Errorf("plain layout still says %q", jargon)
		}
	}
}

// groupsOf builds sprints of n sessions each, none of them waiting.
func groupsOf(sprints, n int) ([]graph.Node, []graph.Edge) {
	var nodes []graph.Node
	var edges []graph.Edge
	for s := 0; s < sprints; s++ {
		sid := fmt.Sprintf("sprint:s%d", s)
		nodes = append(nodes, graph.Node{ID: sid, Kind: graph.KindSprint, Label: fmt.Sprintf("s%d", s),
			Attrs: map[string]string{"status": "running"}, AgeSeconds: int64(s)})
		for i := 0; i < n; i++ {
			id := fmt.Sprintf("session:anthropic:s%d-%d", s, i)
			nd := sess(id, fmt.Sprintf("a%d-%d", s, i), "coding")
			nd.AgeSeconds = int64(s*100 + i)
			nodes = append(nodes, nd)
			edges = append(edges, edge(id, sid, graph.KindWorksFor))
		}
	}
	return nodes, edges
}

// Above 40 boxes the biggest groups fold shut, each into one box, until the
// map fits. A group the user opened stays open.
func TestLayoutFoldsAbove40Boxes(t *testing.T) {
	nodes, edges := groupsOf(5, 10) // 50 session boxes
	l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges})
	if len(l.Boxes) > 40 {
		t.Fatalf("%d boxes, want at most 40 after folding", len(l.Boxes))
	}
	folded := 0
	for _, f := range l.Frames {
		if f.Folded {
			folded++
			if f.Count != 10 {
				t.Errorf("folded frame %s counts %d boxes, want 10", f.Label, f.Count)
			}
			if b, ok := boxByID(l, "fold:"+f.ID); !ok || b.Kind != "folded" || !strings.Contains(b.Line, "10") {
				t.Errorf("folded frame %s has no summary box: %+v", f.Label, b)
			}
		}
	}
	if folded == 0 {
		t.Fatal("nothing folded")
	}
	// Open the first frame that folded: it comes back, others may fold instead.
	var open string
	for _, f := range l.Frames {
		if f.Folded {
			open = f.ID
			break
		}
	}
	l2 := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges, Open: map[string]bool{open: true}})
	for _, f := range l2.Frames {
		if f.ID == open && f.Folded {
			t.Errorf("frame %s was opened by the user and is still folded", open)
		}
	}
	seen := map[[2]int]string{}
	for _, b := range l2.Boxes {
		k := [2]int{b.Column, b.Row}
		if o, dup := seen[k]; dup {
			t.Errorf("%s and %s share column %d row %d", o, b.ID, b.Column, b.Row)
		}
		seen[k] = b.ID
	}
}

// Under the limit nothing folds. A group that waits on you folds last.
func TestLayoutFoldOrderKeepsWaitingGroupsOpen(t *testing.T) {
	nodes, edges := fixture(t)
	l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges})
	if l.Frames[0].Folded {
		t.Fatal("a six box map folded")
	}
	nodes, edges = groupsOf(5, 10)
	nodes = append(nodes, graph.Node{ID: "person:you", Kind: graph.KindPerson, Label: "You"})
	edges = append(edges, edge("session:anthropic:s0-0", "person:you", graph.KindWaitsOn))
	l = BuildLayout(LayoutInput{Nodes: nodes, Edges: edges})
	for _, f := range l.Frames {
		if f.Label == "s0" && f.Folded {
			t.Error("the group with a session that waits on you folded while others could")
		}
	}
}

// Two boxes never share a cell, whatever the input, and a spawn cycle or a
// subagent with no parent still gives a layout.
func TestLayoutNoOverlapAndBadData(t *testing.T) {
	nodes := []graph.Node{sess("s:a", "a", "coding"), sub("s:b", "b", "coding"), sub("s:c", "c", "coding"), sub("s:orphan", "o", "coding")}
	edges := []graph.Edge{
		edge("s:a", "s:b", graph.KindSpawned), edge("s:b", "s:a", graph.KindSpawned),
		edge("s:a", "s:c", graph.KindSpawned), edge("s:a", "s:ghost", graph.KindSpawned),
	}
	l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges})
	seen := map[[2]int]string{}
	for _, b := range l.Boxes {
		k := [2]int{b.Column, b.Row}
		if o, dup := seen[k]; dup {
			t.Errorf("%s and %s share column %d row %d", o, b.ID, b.Column, b.Row)
		}
		seen[k] = b.ID
	}
	for _, ln := range l.Lines {
		if _, ok := boxByID(l, ln.From); !ok && !strings.HasPrefix(ln.From, "frame:") {
			t.Errorf("line starts at missing box %s", ln.From)
		}
		if _, ok := boxByID(l, ln.To); !ok {
			t.Errorf("line ends at missing box %s", ln.To)
		}
	}
}

// Empty input marshals to arrays, never null.
func TestLayoutEmpty(t *testing.T) {
	js, err := json.Marshal(BuildLayout(LayoutInput{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"boxes":[]`, `"lines":[]`, `"frames":[]`, `"idle":[]`, `"columns":[`} {
		if !strings.Contains(string(js), want) {
			t.Errorf("%s lacks %s", js, want)
		}
	}
}

// checkGolden compares l with testdata\<name>. The files let the page check
// (tools\uicheck d30) draw what BuildLayout really makes, in each state.
// Regenerate with UPDATE_GOLDEN=1 after a deliberate change.
func checkGolden(t *testing.T, name string, l Layout) {
	t.Helper()
	got, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("..", "..", "testdata", name)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != string(got) {
		t.Fatalf("%s is out of date; run with UPDATE_GOLDEN=1 and read the diff", path)
	}
}

func exampleEvidence() map[string]Evidence {
	return map[string]Evidence{
		parentID: {Model: "claude-opus-5-5", Tokens: 1200, CostEUR: 2.3, CostLabel: "API price equivalent", Source: "store", AgeSeconds: 20, Known: true},
		subID:    {Tokens: 300, CostEUR: 0.46, CostLabel: "API price equivalent", Source: "store", AgeSeconds: 5, Known: true},
	}
}

func TestLayoutExampleMatchesGolden(t *testing.T) {
	checkGolden(t, "agentmap_layout_example.json", exampleLayout(t, func(in *LayoutInput) { in.Evidence = exampleEvidence() }))
}

func TestLayoutPlainMatchesGolden(t *testing.T) {
	checkGolden(t, "agentmap_layout_example_plain.json", exampleLayout(t, func(in *LayoutInput) { in.Evidence = exampleEvidence(); in.Plain = true }))
}

func TestLayoutFoldedMatchesGolden(t *testing.T) {
	nodes, edges := groupsOf(5, 10)
	nodes = append(nodes, graph.Node{ID: "person:you", Kind: graph.KindPerson, Label: "You"})
	edges = append(edges, edge("session:anthropic:s0-0", "person:you", graph.KindWaitsOn))
	checkGolden(t, "agentmap_layout_folded.json", BuildLayout(LayoutInput{Nodes: nodes, Edges: edges}))
}

// Sessions with no Foreman work are framed by project folder, so a real
// machine (dozens of sessions, no sprint) is not one giant group that folds
// into one box. One project only keeps the plain "No Foreman work" frame, and
// a subagent whose parent is not in the window goes to "Other".
func TestLayoutSplitsUnlinkedSessionsByProject(t *testing.T) {
	nodes := []graph.Node{
		sess("s:a1", "alpha", "coding"), sess("s:a2", "alpha", "thinking"), sess("s:b1", "beta", "coding"),
		sub("s:orphan", "Explore", "coding"),
	}
	l := BuildLayout(LayoutInput{Nodes: nodes})
	got := map[string]int{}
	for _, f := range l.Frames {
		if f.Kind != "project" {
			t.Errorf("frame %+v is not a project frame", f)
		}
		got[f.Label] = f.Count
	}
	if len(l.Frames) != 3 || got["alpha"] != 2 || got["beta"] != 1 || got["Other"] != 1 {
		t.Fatalf("frames %v, want alpha 2, beta 1, Other 1", got)
	}
	for _, f := range l.Frames {
		if f.Label == "alpha" && !strings.Contains(f.Explain, "alpha") {
			t.Errorf("explain %q does not name the folder", f.Explain)
		}
	}

	one := BuildLayout(LayoutInput{Nodes: []graph.Node{sess("s:a1", "alpha", "coding"), sess("s:a2", "alpha", "coding")}})
	if len(one.Frames) != 1 || one.Frames[0].Kind != "none" || one.Frames[0].Label != NoGroupLabel {
		t.Errorf("one project: frames %+v, want the single No Foreman work frame", one.Frames)
	}
}

// With 45 unlinked sessions in three projects the map folds a project, not
// everything: the biggest folds first and the rest stay open.
func TestLayoutFoldsProjectsNotEverything(t *testing.T) {
	var nodes []graph.Node
	for p, n := range []int{25, 12, 8} {
		for i := 0; i < n; i++ {
			nodes = append(nodes, sess(fmt.Sprintf("session:anthropic:p%d-%d", p, i), fmt.Sprintf("proj%d", p), "coding"))
		}
	}
	l := BuildLayout(LayoutInput{Nodes: nodes})
	if len(l.Boxes) > 40 {
		t.Fatalf("%d boxes", len(l.Boxes))
	}
	var folded []string
	for _, f := range l.Frames {
		if f.Folded {
			folded = append(folded, f.Label)
		}
	}
	if len(folded) != 1 || folded[0] != "proj0" {
		t.Errorf("folded %v, want only proj0 (the biggest)", folded)
	}
}

// TestLayoutRealGraphFile tries a real graph: set BURNMON_GRAPH_JSON to the
// output of `burnmon-cli graph -since 72h` (and BURNMON_LAYOUT_OUT to keep the
// layout). It is skipped otherwise. It checks the shape, never the content.
func TestLayoutRealGraphFile(t *testing.T) {
	path := os.Getenv("BURNMON_GRAPH_JSON")
	if path == "" {
		t.Skip("set BURNMON_GRAPH_JSON to a burnmon-cli graph file to try a real graph")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	var g graph.Graph
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	l := BuildLayout(LayoutInput{Nodes: g.Nodes, Edges: g.Edges})
	seen := map[[2]int]string{}
	for _, bx := range l.Boxes {
		k := [2]int{bx.Column, bx.Row}
		if o, dup := seen[k]; dup {
			t.Errorf("%s and %s share column %d row %d", o, bx.ID, bx.Column, bx.Row)
		}
		seen[k] = bx.ID
	}
	folded := 0
	for _, f := range l.Frames {
		if f.Folded {
			folded++
		}
	}
	t.Logf("%d nodes, %d edges -> %d boxes, %d lines, %d frames (%d folded), counts %+v", len(g.Nodes), len(g.Edges), len(l.Boxes), len(l.Lines), len(l.Frames), folded, l.Counts)
	if out := os.Getenv("BURNMON_LAYOUT_OUT"); out != "" {
		js, _ := json.Marshal(l)
		if err := os.WriteFile(out, js, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// More than three MCP servers on one session are one box, listing each server
// and its port in the evidence; a real session uses a dozen and a box each
// would make the map taller than the screen.
func TestLayoutManyMCPServersAreOneBox(t *testing.T) {
	nodes := []graph.Node{sess("s:a", "a", "coding")}
	var edges []graph.Edge
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("mcp:srv%d", i)
		nodes = append(nodes, graph.Node{ID: id, Kind: graph.KindMCP, Label: fmt.Sprintf("srv%d", i), Source: "mcp_config"})
		edges = append(edges, edge("s:a", id, graph.KindUses))
	}
	nodes = append(nodes,
		graph.Node{ID: "process:9:9", Kind: graph.KindProcess, Label: "node.exe"},
		graph.Node{ID: "port:tcp:8020", Kind: graph.KindPort, Label: "tcp 8020"})
	edges = append(edges, edge("mcp:srv2", "process:9:9", graph.KindRunsAs), edge("process:9:9", "port:tcp:8020", graph.KindListensOn))
	l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges})
	if len(l.Boxes) != 2 {
		t.Fatalf("%d boxes, want the session and one MCP box", len(l.Boxes))
	}
	b, ok := boxByID(l, "mcp:all@s:a")
	if !ok || b.Title != "5 MCP servers" {
		t.Fatalf("MCP box = %+v (found %v)", b, ok)
	}
	if v, _ := fact(b, "srv2"); v != "port 8020" {
		t.Errorf("srv2 = %q, want port 8020", v)
	}
	if v, _ := fact(b, "srv0"); v != "no port seen" {
		t.Errorf("srv0 = %q, want no port seen", v)
	}
	if !strings.Contains(b.Line, "1 with a port") {
		t.Errorf("line %q", b.Line)
	}
	if _, ok := lineOf(l, "s:a", "mcp:all@s:a", "uses"); !ok {
		t.Error("no uses line to the MCP box")
	}
	p := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges, Plain: true})
	if b, _ := boxByID(p, "mcp:all@s:a"); b.Title != "5 tool servers" {
		t.Errorf("plain title %q", b.Title)
	}
	// Three or fewer keep a box each.
	three := BuildLayout(LayoutInput{Nodes: nodes[:4], Edges: edges[:3]})
	if len(three.Boxes) != 4 {
		t.Errorf("three servers: %d boxes, want 4", len(three.Boxes))
	}
}

// Counts read as sentences: one session, not "1 sessions".
func TestLayoutSingularWords(t *testing.T) {
	l := exampleLayout(t, nil)
	if h, _ := boxByID(l, "harness:claude"); h.Line != "1 session" {
		t.Errorf("harness line %q", h.Line)
	}
	nodes, edges := groupsOf(5, 10)
	f := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges})
	for _, b := range f.Boxes {
		if b.Kind == "folded" && strings.Contains(b.Line, "1 agents") {
			t.Errorf("folded line %q", b.Line)
		}
	}
}

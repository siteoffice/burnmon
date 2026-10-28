package agentmap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"burnmon/internal/graph"
)

// fixture loads the nodes and edges of the frozen snapshot example, the same
// file lane B's page check feeds the page.
func fixture(t *testing.T) ([]graph.Node, []graph.Edge) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "snapshot_example.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Nodes []graph.Node `json:"nodes"`
		Edges []graph.Edge `json:"edges"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s.Nodes, s.Edges
}

func sess(id, label, state string) graph.Node {
	return graph.Node{ID: id, Kind: graph.KindSession, Label: label, Vendor: "anthropic",
		Attrs: map[string]string{"agent": "claude-code", "state": state}, AgeSeconds: 10}
}

func sub(id, label, state string) graph.Node {
	n := sess(id, label, state)
	n.Kind = graph.KindSubagent
	n.Attrs["agent_type"] = label
	return n
}

func edge(from, to, kind string) graph.Edge {
	return graph.Edge{From: from, To: to, Kind: kind, Basis: graph.BasisVendor, Source: "store"}
}

// A Foreman sprint with its subagent: one group, the session on top, the
// subagent under it, with the MCP server, its port, the repo and the branch
// on the session's card.
func TestBuildSprintWithSubagent(t *testing.T) {
	nodes, edges := fixture(t)
	v := Build(nodes, edges)
	if len(v.Groups) != 1 {
		t.Fatalf("groups = %d, want 1: %+v", len(v.Groups), v.Groups)
	}
	g := v.Groups[0]
	if g.Kind != "sprint" || g.Label != "sprint-example" || g.Status != "running" || g.Phase != "build" {
		t.Fatalf("group = %+v", g)
	}
	if len(g.Cards) != 1 {
		t.Fatalf("cards = %d, want 1", len(g.Cards))
	}
	c := g.Cards[0]
	if c.ID != "session:anthropic:example-parent" || c.State != "coding" || c.Subagent {
		t.Fatalf("top card = %+v", c)
	}
	if c.Basis != "inferred" {
		t.Errorf("works_for basis = %q, want inferred", c.Basis)
	}
	if len(c.MCP) != 1 || c.MCP[0].Name != "example-server" || len(c.MCP[0].Ports) != 1 || c.MCP[0].Ports[0] != "8765" {
		t.Errorf("mcp = %+v", c.MCP)
	}
	if c.Repo != "project" || c.Branch != "sprint/example" {
		t.Errorf("repo %q branch %q", c.Repo, c.Branch)
	}
	if len(c.Children) != 1 {
		t.Fatalf("children = %d, want 1", len(c.Children))
	}
	k := c.Children[0]
	// The example's subagent ended its turn: it is finished, and waits on no one.
	// Only the blocked top-level session waits on you (B6).
	if !k.Subagent || k.AgentType != "Explore" || k.Model != "sonnet" || k.WaitsOnYou {
		t.Errorf("subagent card = %+v", k)
	}
	want := Counts{Sessions: 1, Subagents: 1, Waiting: 1, Sprints: 1}
	if v.Counts != want {
		t.Errorf("counts = %+v, want %+v", v.Counts, want)
	}
}

// A session that works for nothing sits in the group with no Foreman work,
// listed last.
func TestBuildSessionWithoutSprint(t *testing.T) {
	nodes := []graph.Node{
		sess("s:a", "alpha", "coding"),
		{ID: "sprint:x", Kind: graph.KindSprint, Label: "x", Attrs: map[string]string{"status": "running"}},
		sess("s:b", "beta", "thinking"),
	}
	edges := []graph.Edge{edge("s:b", "sprint:x", graph.KindWorksFor)}
	v := Build(nodes, edges)
	if len(v.Groups) != 2 || v.Groups[0].Kind != "sprint" || v.Groups[1].Kind != "none" {
		t.Fatalf("groups = %+v", v.Groups)
	}
	if v.Groups[1].Label != "No Foreman work" || len(v.Groups[1].Cards) != 1 || v.Groups[1].Cards[0].Label != "alpha" {
		t.Errorf("none group = %+v", v.Groups[1])
	}
}

// A sprint or flow that waits on you and has no session in the graph is not
// a group: it goes to Idle, so five old sprints do not push the live work down.
func TestBuildIdleSprint(t *testing.T) {
	nodes := []graph.Node{
		{ID: "sprint:old", Kind: graph.KindSprint, Label: "old", Attrs: map[string]string{"status": "waiting-on-you"}, AgeSeconds: 99},
		{ID: "person:you", Kind: graph.KindPerson, Label: "You"},
	}
	edges := []graph.Edge{edge("sprint:old", "person:you", graph.KindWaitsOn)}
	v := Build(nodes, edges)
	if len(v.Groups) != 0 {
		t.Fatalf("groups = %+v", v.Groups)
	}
	if len(v.Idle) != 1 || v.Idle[0].Label != "old" || !v.Idle[0].WaitsOnYou || v.Idle[0].Kind != "sprint" {
		t.Fatalf("idle = %+v", v.Idle)
	}
	if v.Counts.Sprints != 1 || v.Counts.Waiting != 1 {
		t.Errorf("counts = %+v", v.Counts)
	}
}

// A flow whose gate waits on you: the flow's group says so.
func TestBuildGateWaitsOnYou(t *testing.T) {
	nodes := []graph.Node{
		sess("s:a", "alpha", "coding"),
		{ID: "flow:f1", Kind: graph.KindFlow, Label: "flow-one", Attrs: map[string]string{"status": "awaiting-owner"}},
		{ID: "gate:f1:G4", Kind: graph.KindGate, Label: "G4"},
		{ID: "person:you", Kind: graph.KindPerson, Label: "You"},
	}
	edges := []graph.Edge{
		edge("s:a", "flow:f1", graph.KindWorksFor),
		edge("gate:f1:G4", "flow:f1", graph.KindPartOf),
		edge("gate:f1:G4", "person:you", graph.KindWaitsOn),
	}
	v := Build(nodes, edges)
	if len(v.Groups) != 1 {
		t.Fatalf("groups = %+v", v.Groups)
	}
	g := v.Groups[0]
	if g.Kind != "flow" || !g.WaitsOnYou || g.Gate != "G4" {
		t.Errorf("group = %+v", g)
	}
}

// A subagent whose parent is not in the graph (a history window that began
// after the parent) is a top card, not dropped.
func TestBuildOrphanSubagentIsTop(t *testing.T) {
	nodes := []graph.Node{sub("s:kid", "Explore", "coding")}
	v := Build(nodes, nil)
	if len(v.Groups) != 1 || len(v.Groups[0].Cards) != 1 || !v.Groups[0].Cards[0].Subagent {
		t.Fatalf("groups = %+v", v.Groups)
	}
	if v.Counts.Subagents != 1 || v.Counts.Sessions != 0 {
		t.Errorf("counts = %+v", v.Counts)
	}
}

// Bad data never hangs or duplicates: a spawn cycle, an edge to a node that
// is not in the list, and a duplicate edge.
func TestBuildSurvivesBadData(t *testing.T) {
	nodes := []graph.Node{sess("s:a", "a", "coding"), sub("s:b", "b", "coding")}
	edges := []graph.Edge{
		edge("s:a", "s:b", graph.KindSpawned),
		edge("s:b", "s:a", graph.KindSpawned),
		edge("s:a", "s:ghost", graph.KindSpawned),
		edge("s:a", "s:b", graph.KindSpawned),
	}
	v := Build(nodes, edges)
	seen := map[string]int{}
	var walk func(cs []Card)
	walk = func(cs []Card) {
		for _, c := range cs {
			seen[c.ID]++
			walk(c.Children)
		}
	}
	for _, g := range v.Groups {
		walk(g.Cards)
	}
	if seen["s:a"] != 1 || seen["s:b"] != 1 {
		t.Fatalf("each node once, got %v", seen)
	}
}

// Empty input still marshals to arrays and zero counts, so the page never
// meets null.
func TestBuildEmpty(t *testing.T) {
	v := Build(nil, nil)
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{`"groups":[]`, `"idle":[]`, `"notes":[`} {
		if !strings.Contains(s, want) {
			t.Errorf("%s lacks %s", s, want)
		}
	}
	if v.Counts != (Counts{}) {
		t.Errorf("counts = %+v", v.Counts)
	}
}

// The map says what it cannot show, in plain words.
func TestBuildNotesSayNoMessages(t *testing.T) {
	v := Build(nil, nil)
	var found bool
	for _, n := range v.Notes {
		if strings.Contains(n, "messages between agents") {
			found = true
		}
	}
	if !found {
		t.Errorf("notes = %v", v.Notes)
	}
}

// The view carries labels, states and ids only: no node attribute outside the
// small list is copied, so nothing a future attr holds (a path, a command
// line) reaches the page.
func TestBuildCopiesOnlyKnownAttrs(t *testing.T) {
	n := sess("s:a", "a", "coding")
	n.Attrs["cmdline"] = `C:\secret\thing --token=SECRET`
	v := Build([]graph.Node{n}, nil)
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "SECRET") || strings.Contains(string(b), "secret") {
		t.Fatalf("unknown attr leaked: %s", b)
	}
}

// Order is stable: the same input gives the same JSON, newest activity first.
func TestBuildOrderIsStable(t *testing.T) {
	a, b := sess("s:a", "a", "coding"), sess("s:b", "b", "coding")
	a.AgeSeconds, b.AgeSeconds = 50, 5
	one, _ := json.Marshal(Build([]graph.Node{a, b}, nil))
	two, _ := json.Marshal(Build([]graph.Node{b, a}, nil))
	if string(one) != string(two) {
		t.Fatalf("order depends on input:\n%s\n%s", one, two)
	}
	v := Build([]graph.Node{a, b}, nil)
	if v.Groups[0].Cards[0].ID != "s:b" {
		t.Errorf("newest first: got %s", v.Groups[0].Cards[0].ID)
	}
}

// The view of the frozen example is kept in testdata\agentmap_example.json so
// the page check (tools\uicheck d30) feeds the page what Build really makes.
// Regenerate with UPDATE_GOLDEN=1 after a deliberate change.
func TestExampleViewMatchesGolden(t *testing.T) {
	nodes, edges := askingExample(t)
	got, err := json.MarshalIndent(Build(nodes, edges), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("..", "..", "testdata", "agentmap_example.json")
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

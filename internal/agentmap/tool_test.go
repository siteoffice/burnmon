package agentmap

import (
	"encoding/json"
	"strings"
	"testing"

	"burnmon/internal/graph"
)

// The map shows the name of the tool a live session is running, never its
// arguments. A name that could carry something else is not shown as is, and an
// MCP tool is "MCP tool": its raw name holds the server's name.
func TestToolLabel(t *testing.T) {
	for _, c := range []struct {
		in, std, plain string
	}{
		{"Bash", "Bash", "Bash"},
		{"WebFetch", "WebFetch", "WebFetch"},
		{"Edit", "Edit", "Edit"},
		{"mcp__github__create_issue", "MCP tool", "tool server call"},
		{"MCP__x__y", "MCP tool", "tool server call"},
		{"", "", ""},
		{`C:\secret\thing.exe`, "tool", "tool"},
		{"a b", "tool", "tool"},
		{"echo $TOKEN", "tool", "tool"},
		{strings.Repeat("A", 40), "tool", "tool"},
	} {
		if got := toolLabel(c.in, false); got != c.std {
			t.Errorf("toolLabel(%q, standard) = %q, want %q", c.in, got, c.std)
		}
		if got := toolLabel(c.in, true); got != c.plain {
			t.Errorf("toolLabel(%q, plain) = %q, want %q", c.in, got, c.plain)
		}
	}
}

func withTool(n graph.Node, tool string) graph.Node {
	attrs := map[string]string{}
	for k, v := range n.Attrs {
		attrs[k] = v
	}
	if tool != "" {
		attrs["tool"] = tool
	}
	n.Attrs = attrs
	return n
}

// A live session's tool is on its box and in its evidence; a session with no
// tool has neither.
func TestLayoutShowsCurrentTool(t *testing.T) {
	nodes := []graph.Node{
		withTool(sess("s:a", "a", "coding"), "Bash"),
		withTool(sub("s:k", "Explore", "running"), "mcp__graphify-cipher__query"),
		sess("s:b", "b", "thinking"),
	}
	edges := []graph.Edge{edge("s:a", "s:k", graph.KindSpawned)}
	l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges})
	a, _ := boxByID(l, "s:a")
	if a.Tool != "Bash" {
		t.Errorf("session tool %q, want Bash", a.Tool)
	}
	if v, _ := fact(a, "Current tool"); v != "Bash" {
		t.Errorf("session evidence %q, want Bash", v)
	}
	k, _ := boxByID(l, "s:k")
	if k.Tool != "MCP tool" {
		t.Errorf("subagent tool %q, want MCP tool", k.Tool)
	}
	b, _ := boxByID(l, "s:b")
	if b.Tool != "" {
		t.Errorf("a session with no tool shows %q", b.Tool)
	}
	if _, ok := fact(b, "Current tool"); ok {
		t.Error("a session with no tool has a Current tool fact")
	}
	js, _ := json.Marshal(l)
	if strings.Contains(string(js), "cipher") || strings.Contains(string(js), "graphify") {
		t.Fatalf("an MCP server name reached the layout: %s", js)
	}
	p := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges, Plain: true})
	if k, _ := boxByID(p, "s:k"); k.Tool != "tool server call" {
		t.Errorf("plain subagent tool %q", k.Tool)
	}
}

// The tool is an attribute of the card; the older cards without it still build.
func TestBuildCardCarriesTool(t *testing.T) {
	v := Build([]graph.Node{withTool(sess("s:a", "a", "coding"), "Read")}, nil)
	if got := v.Groups[0].Cards[0].Tool; got != "Read" {
		t.Errorf("card tool %q, want Read", got)
	}
}

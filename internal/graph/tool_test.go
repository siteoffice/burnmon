package graph

import (
	"testing"
	"time"
)

// A live session's current tool name goes on its node as the attribute "tool"
// (attrs is the open, kind specific map, so the snapshot's version 1 field
// list does not change). A session with no tool has no such attribute.
func TestBuildSessionToolAttr(t *testing.T) {
	g := Build(Input{Now: t0, Sessions: []Session{
		{Vendor: "anthropic", Agent: "claude-code", ID: "A", State: "coding", Tool: "Bash", LastActivity: t0},
		{Vendor: "anthropic", Agent: "claude-code", ID: "agent-b", ParentID: "A", State: "reading", Tool: "Read", LastActivity: t0.Add(-time.Second)},
		{Vendor: "anthropic", Agent: "claude-code", ID: "C", State: "thinking", LastActivity: t0},
	}})
	for id, want := range map[string]string{"A": "Bash", "agent-b": "Read"} {
		n, ok := node(g, SessionID("anthropic", id))
		if !ok || n.Attrs["tool"] != want {
			t.Errorf("%s: tool attr %q (found %v), want %q", id, n.Attrs["tool"], ok, want)
		}
	}
	if n, _ := node(g, SessionID("anthropic", "C")); n.Attrs != nil {
		if _, has := n.Attrs["tool"]; has {
			t.Errorf("a session with no tool has the attribute: %+v", n.Attrs)
		}
	}
}

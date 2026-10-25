package main

import (
	"testing"

	"burnmon/internal/live"
	"burnmon/internal/snapshotwriter"
)

// The live graph gets each session's and subagent's current tool name from the
// stage the core already works out; nothing else about the call.
func TestFlatSessionsCarriesTheToolName(t *testing.T) {
	ss := []*live.Session{{
		Vendor: "anthropic", Agent: "claude-code", SessionID: "A", Stage: "coding", StageTool: "Bash", LastTurn: "2026-10-25T12:00:00Z",
		Subagents: []*live.Session{{Vendor: "anthropic", Agent: "claude-code", SessionID: "agent-b", Stage: "reading", StageTool: "Read", LastTurn: "2026-10-25T12:00:00Z"}},
	}}
	got := snapshotwriter.FlatSessions(ss)
	if len(got) != 2 || got[0].Tool != "Bash" || got[1].Tool != "Read" || got[1].ParentID != "A" {
		t.Fatalf("FlatSessions = %+v", got)
	}
}

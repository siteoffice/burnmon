//go:build windows

package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burnmon/internal/snapshot"
)

// One line per agent view state the snapshot writer records: off, refused
// with its reason, or nothing when it is on or the snapshot says nothing.
func TestAgentViewLine(t *testing.T) {
	cases := []struct {
		name string
		st   *snapshot.AgentViewStatus
		want string
	}{
		{"no status", nil, ""},
		{"off", &snapshot.AgentViewStatus{State: "off"}, `Agent view off: set "agent_view": true in burnmon.json.`},
		{"refused, .ps1 shim", &snapshot.AgentViewStatus{State: "refused", Reason: "claude is a .ps1 shim; agent_view needs claude.exe"},
			`Agent view refused (claude is a .ps1 shim; agent_view needs claude.exe): set "claude_path" in burnmon.json to claude.exe.`},
		{"refused, claude_path not a .exe", &snapshot.AgentViewStatus{State: "refused", Reason: "claude_path is not a .exe"},
			`Agent view refused (claude_path is not a .exe): set "claude_path" in burnmon.json to claude.exe.`},
		{"on", &snapshot.AgentViewStatus{State: "on"}, ""},
	}
	for _, c := range cases {
		if got := agentViewLine(c.st); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// The payload builds the line from the snapshot it already read, and only
// from a current one: a stale file says nothing about the writer now.
func TestSectionsPayloadAgentViewLineFollowsSnapshot(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	write := func(at time.Time, st *snapshot.AgentViewStatus) string {
		p := filepath.Join(t.TempDir(), "snapshot.json")
		if err := snapshot.Write(p, snapshot.Build(snapshot.Input{Now: at, TickSeconds: 5, AgentView: st})); err != nil {
			t.Fatal(err)
		}
		return p
	}
	off := &snapshot.AgentViewStatus{State: "off"}
	cases := []struct {
		name string
		path string
		want string
	}{
		{"current, off", write(now, off), `Agent view off: set "agent_view": true in burnmon.json.`},
		{"current, refused", write(now, &snapshot.AgentViewStatus{State: "refused", Reason: "the claude.cmd shim names no claude.exe"}),
			`Agent view refused (the claude.cmd shim names no claude.exe): set "claude_path" in burnmon.json to claude.exe.`},
		{"current, on", write(now, &snapshot.AgentViewStatus{State: "on"}), ""},
		{"current, written before the field", write(now, nil), ""},
		{"stale, off", write(now.Add(-time.Minute), off), ""},
		{"missing file", filepath.Join(t.TempDir(), "none.json"), ""},
	}
	for _, c := range cases {
		if got := sectionsPayloadFor(t.TempDir(), c.path, now).AgentViewLine; got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// The page reads the line as agent_view_line; an empty line is left out.
func TestSectionsPayloadAgentViewLine(t *testing.T) {
	b, err := json.Marshal(sectionsPayload{AgentViewLine: "Agent view off."})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"agent_view_line":"Agent view off."`) {
		t.Errorf("payload has no agent_view_line: %s", b)
	}
	b, err = json.Marshal(sectionsPayload{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "agent_view_line") {
		t.Errorf("an empty line is still in the payload: %s", b)
	}
}

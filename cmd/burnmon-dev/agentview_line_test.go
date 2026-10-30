//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"burnmon/internal/pricing"
)

// One line per agent view state: off, refused with Task 1's reason, or
// nothing when claude resolves.
func TestAgentViewLine(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "claude.exe")
	if err := os.WriteFile(exe, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	onPath := func(p string) func(string) (string, error) {
		return func(string) (string, error) { return p, nil }
	}
	missing := func(string) (string, error) { return "", errors.New("not on PATH") }
	cases := []struct {
		name string
		cfg  pricing.Config
		look func(string) (string, error)
		want string
	}{
		{"off", pricing.Config{}, onPath(exe), `Agent view off: set "agent_view": true in burnmon.json.`},
		{"off, claude missing too", pricing.Config{}, missing, `Agent view off: set "agent_view": true in burnmon.json.`},
		{"on, .ps1 shim", pricing.Config{AgentView: true}, onPath(`C:\npm\claude.ps1`),
			`Agent view refused (claude is a .ps1 shim; agent_view needs claude.exe): set "claude_path" in burnmon.json to claude.exe.`},
		{"on, not on PATH", pricing.Config{AgentView: true}, missing,
			`Agent view refused (claude not found: not on PATH): set "claude_path" in burnmon.json to claude.exe.`},
		{"on, claude_path not a .exe", pricing.Config{AgentView: true, ClaudePath: `C:\npm\claude.cmd`}, onPath(exe),
			`Agent view refused (claude_path is not a .exe): set "claude_path" in burnmon.json to claude.exe.`},
		{"on, claude.exe on PATH", pricing.Config{AgentView: true}, onPath(exe), ""},
		{"on, claude_path set", pricing.Config{AgentView: true, ClaudePath: exe}, missing, ""},
	}
	for _, c := range cases {
		if got := agentViewLine(c.cfg, c.look, "windows"); got != c.want {
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

//go:build windows

package main

import (
	"encoding/json"
	"os"
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
		{"off", &snapshot.AgentViewStatus{State: "off", ConfigPath: `C:\x\burnmon.json`}, `Agent view off: set "agent_view": true in C:\x\burnmon.json.`},
		{"refused, .ps1 shim", &snapshot.AgentViewStatus{State: "refused", Reason: "claude is a .ps1 shim; agent_view needs claude.exe", ConfigPath: `C:\x\burnmon.json`},
			`Agent view refused (claude is a .ps1 shim; agent_view needs claude.exe): set "claude_path" in C:\x\burnmon.json to claude.exe.`},
		{"refused, claude_path not a .exe", &snapshot.AgentViewStatus{State: "refused", Reason: "claude_path is not a .exe", ConfigPath: `C:\x\burnmon.json`},
			`Agent view refused (claude_path is not a .exe): set "claude_path" in C:\x\burnmon.json to claude.exe.`},
		{"on", &snapshot.AgentViewStatus{State: "on"}, ""},
	}
	for _, c := range cases {
		if got := agentViewLine(c.st); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// A snapshot written before config_path existed still names a path: the
// default under LOCALAPPDATA.
func TestAgentViewLineFallbackPath(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	want := filepath.Join(local, "burnmon", "burnmon.json")
	off := agentViewLine(&snapshot.AgentViewStatus{State: "off"})
	if off != `Agent view off: set "agent_view": true in `+want+`.` {
		t.Errorf("off: %q", off)
	}
	ref := agentViewLine(&snapshot.AgentViewStatus{State: "refused", Reason: "r"})
	if ref != `Agent view refused (r): set "claude_path" in `+want+` to claude.exe.` {
		t.Errorf("refused: %q", ref)
	}
}

// loadConfigAt gives the file it loaded, and that path reaches both lines
// through the status the writer records.
func TestLoadConfigAtPathReachesLines(t *testing.T) {
	exeDir, dataDir := t.TempDir(), t.TempDir()
	want := filepath.Join(exeDir, "burnmon.json")
	if err := os.WriteFile(want, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, got := loadConfigAt(exeDir, dataDir)
	if got != want {
		t.Fatalf("loaded path %q, want %q", got, want)
	}
	for _, st := range []*snapshot.AgentViewStatus{
		{State: "off", ConfigPath: got},
		{State: "refused", Reason: "r", ConfigPath: got},
	} {
		if line := agentViewLine(st); !strings.Contains(line, want) {
			t.Errorf("%s line %q lacks %q", st.State, line, want)
		}
	}
}

// With no file anywhere the loader says "" and the lines fall back to the
// default location.
func TestLoadConfigAtNoFile(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	_, got := loadConfigAt(t.TempDir(), t.TempDir())
	if got != "" {
		t.Fatalf("loaded path %q, want empty", got)
	}
	want := filepath.Join(local, "burnmon", "burnmon.json")
	for _, st := range []*snapshot.AgentViewStatus{{State: "off"}, {State: "refused", Reason: "r"}} {
		if line := agentViewLine(st); !strings.Contains(line, want) || strings.Contains(line, "in burnmon.json.") {
			t.Errorf("%s line %q", st.State, line)
		}
	}
}

// The app's writer carries the file the app loaded into the graph readers.
func TestNewSnapshotWriterRecordsConfigPath(t *testing.T) {
	a := &app{loadedCfg: `C:\x\burnmon.json`}
	w := a.newSnapshotWriter(nil, filepath.Join(t.TempDir(), "snapshot.json"))
	if w.Graph == nil || w.Graph.ConfigPath != `C:\x\burnmon.json` {
		t.Fatalf("graph config path: %+v", w.Graph)
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
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	fallback := filepath.Join(local, "burnmon", "burnmon.json")
	cases := []struct {
		name string
		path string
		want string
	}{
		{"current, off", write(now, off), `Agent view off: set "agent_view": true in ` + fallback + `.`},
		{"current, refused", write(now, &snapshot.AgentViewStatus{State: "refused", Reason: "the claude.cmd shim names no claude.exe"}),
			`Agent view refused (the claude.cmd shim names no claude.exe): set "claude_path" in ` + fallback + ` to claude.exe.`},
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

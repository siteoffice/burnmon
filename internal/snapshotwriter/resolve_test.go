package snapshotwriter_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"burnmon/internal/snapshotwriter"
	"burnmon/internal/waits"
)

func lookOf(p string, err error) func(string) (string, error) {
	return func(string) (string, error) { return p, err }
}

func TestResolveClaudeWindowsOnlyExe(t *testing.T) {
	for _, bad := range []string{`C:\n\claude.cmd`, `C:\n\claude.BAT`, `C:\n\claude.ps1`, `C:\n\claude`, `C:\n\claude.com`} {
		if got, err := snapshotwriter.ResolveClaudeFor(lookOf(bad, nil), "windows"); err == nil {
			t.Errorf("%s accepted as %q", bad, got)
		}
	}
	got, err := snapshotwriter.ResolveClaudeFor(lookOf(`C:\n\claude.EXE`, nil), "windows")
	if err != nil || got != `C:\n\claude.EXE` {
		t.Fatalf("exe: %q %v", got, err)
	}
}

func TestResolveClaudeCmdShimReason(t *testing.T) {
	_, err := snapshotwriter.ResolveClaudeFor(lookOf(`C:\n\claude.cmd`, nil), "windows")
	if err == nil || err.Error() != "claude is a .cmd shim; agent_view needs claude.exe" {
		t.Fatalf("reason: %v", err)
	}
}

func TestResolveClaudeNotFound(t *testing.T) {
	_, err := snapshotwriter.ResolveClaudeFor(lookOf("", errors.New("not found")), "windows")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("not found: %v", err)
	}
}

func TestResolveClaudeNonWindowsNeedsExecutableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits are not meaningful on Windows")
	}
	dir := t.TempDir()
	ok := filepath.Join(dir, "claude")
	if err := os.WriteFile(ok, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshotwriter.ResolveClaudeFor(lookOf(ok, nil), "linux"); err != nil {
		t.Fatalf("executable file refused: %v", err)
	}
	plain := filepath.Join(dir, "plain")
	if err := os.WriteFile(plain, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshotwriter.ResolveClaudeFor(lookOf(plain, nil), "linux"); err == nil {
		t.Fatal("non-executable file accepted")
	}
}

// A refused claude starts no poller and says why in the agent_view source.
func TestEnableAgentViewRefusedShowsReason(t *testing.T) {
	st := snapshotTestStore(t)
	gr, _ := fakeReaders(t, `{"pid":4100,"sessionId":"s2","procStart":"777"}`)
	gr.EnableAgentViewFor(lookOf(`C:\n\claude.cmd`, nil), "windows")
	if gr.AgentView != nil {
		t.Fatal("runner set for a .cmd shim")
	}
	w := newWriter(st, filepath.Join(t.TempDir(), "snapshot.json"), noProcesses, gr)
	got := onceRead(t, w)
	s := sourceNamed(got, waits.SourceAgentView)
	if s == nil || s.OK || s.Error != "claude is a .cmd shim; agent_view needs claude.exe" {
		t.Fatalf("agent_view source: %+v", s)
	}
}

func TestEnableAgentViewAcceptedSetsRunner(t *testing.T) {
	gr, _ := fakeReaders(t, `{"pid":4100,"sessionId":"s2","procStart":"777"}`)
	gr.EnableAgentViewFor(lookOf(`C:\n\claude.exe`, nil), "windows")
	if gr.AgentView == nil {
		t.Fatal("no runner for claude.exe")
	}
}

// R9: a gate that opens between two ticks is in the snapshot of the very
// next tick that is due, and the Foreman re-read is due within two ticks.
func TestForemanGateAppearsWithinTwoTicks(t *testing.T) {
	st := snapshotTestStore(t)
	gr, _ := fakeReaders(t, `{"pid":4100,"sessionId":"s2","procStart":"777"}`)
	w := newWriter(st, filepath.Join(t.TempDir(), "snapshot.json"), noProcesses, gr)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	w.Now = func() time.Time { return now }
	if got := onceRead(t, w); findItem(got.WaitItems, "gate") != nil {
		t.Fatalf("gate before it exists: %+v", got.WaitItems)
	}
	writeForemanGate(t, gr.ForemanRoot)
	now = now.Add(snapshotwriter.Tick)
	onceRead(t, w)
	now = now.Add(snapshotwriter.Tick)
	got := onceRead(t, w)
	if findItem(got.WaitItems, "gate") == nil {
		t.Fatalf("gate missing two ticks after it opened: %+v", got.WaitItems)
	}
}

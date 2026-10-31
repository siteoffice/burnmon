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

func TestResolveClaudeCmdShimUnreadable(t *testing.T) {
	_, err := snapshotwriter.ResolveClaudeFor(lookOf(filepath.Join(t.TempDir(), "claude.cmd"), nil), "windows")
	if err == nil || err.Error() != "the claude.cmd shim cannot be read" {
		t.Fatalf("reason: %v", err)
	}
}

// npmShim is the claude.cmd that npm writes, CRLF and all.
var npmShim = strings.ReplaceAll(`@ECHO off
GOTO start
:find_dp0
SET dp0=%~dp0
EXIT /b
:start
SETLOCAL
CALL :find_dp0
"%dp0%\node_modules\@anthropic-ai\claude-code\bin\claude.exe"   %*
`, "\n", "\r\n")

// writeShim writes text as claude.cmd in a folder with a space in its name,
// like an npm prefix under C:\Program Files\nodejs.
func writeShim(t *testing.T, text string) string {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("shim paths use backslashes")
	}
	dir := filepath.Join(t.TempDir(), "npm prefix")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(dir, "claude.cmd")
	if err := os.WriteFile(shim, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return shim
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func shimExe(shim string) string {
	return filepath.Join(filepath.Dir(shim), "node_modules", "@anthropic-ai", "claude-code", "bin", "claude.exe")
}

func TestResolveShimRealShape(t *testing.T) {
	shim := writeShim(t, npmShim)
	exe := shimExe(shim)
	touch(t, exe)
	got, err := snapshotwriter.ResolveClaudeFor(lookOf(shim, nil), "windows")
	if err != nil || got != exe {
		t.Fatalf("got %q %v, want %q", got, err, exe)
	}
	got, err = snapshotwriter.ResolveClaudeConfigured("", lookOf(shim, nil), "windows")
	if err != nil || got != exe {
		t.Fatalf("configured empty: got %q %v, want %q", got, err, exe)
	}
}

func TestResolveShimTildeSpelling(t *testing.T) {
	shim := writeShim(t, "@ECHO off\r\n\"%~dp0\\node_modules\\@anthropic-ai\\claude-code\\bin\\claude.exe\"   %*\r\n")
	exe := shimExe(shim)
	touch(t, exe)
	got, err := snapshotwriter.ResolveClaudeFor(lookOf(shim, nil), "windows")
	if err != nil || got != exe {
		t.Fatalf("got %q %v, want %q", got, err, exe)
	}
}

func TestResolveShimMissingFile(t *testing.T) {
	shim := writeShim(t, npmShim)
	_, err := snapshotwriter.ResolveClaudeFor(lookOf(shim, nil), "windows")
	if err == nil || !strings.Contains(err.Error(), "names a claude.exe that does not exist") {
		t.Fatalf("missing exe: %v", err)
	}
}

func TestResolveShimNonExe(t *testing.T) {
	shim := writeShim(t, strings.Replace(npmShim, `bin\claude.exe`, `cli.js`, 1))
	touch(t, filepath.Join(filepath.Dir(shim), "node_modules", "@anthropic-ai", "claude-code", "cli.js"))
	_, err := snapshotwriter.ResolveClaudeFor(lookOf(shim, nil), "windows")
	if err == nil || !strings.Contains(err.Error(), "names no claude.exe") {
		t.Fatalf("cli.js shim: %v", err)
	}
}

// The older cmd-shim starts node.exe with a .js file; node.exe is never taken.
func TestResolveShimNodeExeNotTaken(t *testing.T) {
	shim := writeShim(t, strings.ReplaceAll(`@IF EXIST "%~dp0\node.exe" (
  "%~dp0\node.exe"  "%~dp0\node_modules\@anthropic-ai\claude-code\cli.js" %*
) ELSE (
  @SETLOCAL
  @SET PATHEXT=%PATHEXT:;.JS;=;%
  node  "%~dp0\node_modules\@anthropic-ai\claude-code\cli.js" %*
)
`, "\n", "\r\n"))
	touch(t, filepath.Join(filepath.Dir(shim), "node.exe"))
	got, err := snapshotwriter.ResolveClaudeFor(lookOf(shim, nil), "windows")
	if err == nil {
		t.Fatalf("old shim accepted as %q", got)
	}
}

func TestResolvePs1StillRefused(t *testing.T) {
	_, err := snapshotwriter.ResolveClaudeFor(lookOf(`C:\n\claude.ps1`, nil), "windows")
	if err == nil || err.Error() != "claude is a .ps1 shim; agent_view needs claude.exe" {
		t.Fatalf("ps1: %v", err)
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
	gr.EnableAgentViewFor(lookOf(`C:\n\claude.ps1`, nil), "windows")
	if gr.AgentView != nil {
		t.Fatal("runner set for a .ps1 shim")
	}
	w := newWriter(st, filepath.Join(t.TempDir(), "snapshot.json"), noProcesses, gr)
	got := onceRead(t, w)
	s := sourceNamed(got, waits.SourceAgentView)
	if s == nil || s.OK || s.Error != "claude is a .ps1 shim; agent_view needs claude.exe" {
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

func TestResolveClaudeConfigured(t *testing.T) {
	dir := t.TempDir()
	failing := lookOf("", errors.New("look must not be used"))
	exe := filepath.Join(dir, "claude.exe")
	if err := os.WriteFile(exe, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := snapshotwriter.ResolveClaudeConfigured(exe, failing, "windows")
	if err != nil || got != exe {
		t.Fatalf("configured exe: %q %v", got, err)
	}
	cmdFile := filepath.Join(dir, "claude.cmd")
	if err := os.WriteFile(cmdFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirExe := filepath.Join(dir, "x.exe")
	if err := os.Mkdir(dirExe, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, want string }{
		{"claude.exe", "claude_path is not absolute"},
		{cmdFile, "claude_path is not a .exe"},
		{filepath.Join(dir, "missing.exe"), "claude_path is not a file"},
		{dirExe, "claude_path is not a file"},
	} {
		if _, err := snapshotwriter.ResolveClaudeConfigured(tc.path, failing, "windows"); err == nil || err.Error() != tc.want {
			t.Errorf("%s: err %v, want %q", tc.path, err, tc.want)
		}
	}
	got, err = snapshotwriter.ResolveClaudeConfigured("", lookOf(`C:\x\claude.exe`, nil), "windows")
	if err != nil || got != `C:\x\claude.exe` {
		t.Fatalf("fallback: %q %v", got, err)
	}
	_, err = snapshotwriter.ResolveClaudeConfigured("", lookOf(`C:\x\claude.ps1`, nil), "windows")
	if err == nil || err.Error() != "claude is a .ps1 shim; agent_view needs claude.exe" {
		t.Fatalf("fallback shim: %v", err)
	}
}

func TestEnableAgentViewWithConfiguredPath(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "claude.exe")
	if err := os.WriteFile(exe, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gr, _ := fakeReaders(t, `{"pid":4100,"sessionId":"s2","procStart":"777"}`)
	gr.EnableAgentViewWith(exe, lookOf("", errors.New("look must not be used")))
	if gr.AgentView == nil {
		t.Fatal("no runner for a configured claude.exe")
	}

	st := snapshotTestStore(t)
	gr2, _ := fakeReaders(t, `{"pid":4100,"sessionId":"s2","procStart":"777"}`)
	gr2.EnableAgentViewWith("", lookOf(`C:\n\claude.ps1`, nil))
	if gr2.AgentView != nil {
		t.Fatal("runner set for a .ps1 shim")
	}
	w := newWriter(st, filepath.Join(t.TempDir(), "snapshot.json"), noProcesses, gr2)
	s := sourceNamed(onceRead(t, w), waits.SourceAgentView)
	if s == nil || s.OK || s.Error != "claude is a .ps1 shim; agent_view needs claude.exe" {
		t.Fatalf("agent_view source: %+v", s)
	}
}

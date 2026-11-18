package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func firstLine(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimRight(strings.SplitN(string(b), "\n", 2)[0], "\r")
}

func TestDevBuildTags(t *testing.T) {
	want := map[string]string{
		"main.go":                "//go:build windows || (darwin && cgo)",
		"lamp_run.go":            "//go:build windows || (darwin && cgo)",
		"lamp_child.go":          "//go:build windows || (darwin && cgo)",
		"lamp_window_windows.go": "//go:build windows",
		"lamp_window_darwin.go":  "//go:build darwin && cgo",
		"main_other.go":          "//go:build !windows && !darwin",
		"windowstate.go":         "//go:build windows",
	}
	for _, f := range []string{
		"agentmap_view.go", "agentview_line.go", "app.go", "burn_report.go",
		"controls_view.go", "export_run.go", "headline.go", "history_admin.go",
		"history_delta.go", "item_files.go", "perf.go", "sections_view.go",
		"snapshot_line.go", "snapshot_writer.go", "station_theme.go",
		"toast_watch.go", "todo_gate.go", "uicheck_devserver.go", "viewstate.go",
		"payloads.go", "toast_start.go",
	} {
		want[f] = "//go:build windows || darwin"
	}
	for f, tag := range want {
		if got := firstLine(t, f); got != tag {
			t.Errorf("%s: first line %q, want %q", f, got, tag)
		}
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "windowstate.go" || strings.HasSuffix(f, "_windows.go") {
			continue
		}
		if firstLine(t, f) == "//go:build windows" {
			t.Errorf("%s still starts with //go:build windows", f)
		}
	}
}

func readText(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDarwinToastsAreLazy(t *testing.T) {
	s := readText(t, "platform_darwin.go")
	if !strings.Contains(s, "startLazyToasts(snapPath, setDir, backfilled, nil)") {
		t.Error("platform_darwin.go must call startLazyToasts(snapPath, setDir, backfilled, nil)")
	}
	if strings.Contains(s, "toast.Start(") {
		t.Error("platform_darwin.go must not call toast.Start(")
	}
}

func TestNoCgoDarwinMessage(t *testing.T) {
	if !strings.Contains(readText(t, "main_nocgo_darwin.go"), "needs a cgo build on a Mac") {
		t.Error("main_nocgo_darwin.go must say it needs a cgo build on a Mac")
	}
}

// lamp_unsupported.go carries no build tag, so the sentence and the exit code
// of the non-Windows lamp are tested on Windows too.
func TestLampUnsupportedHasNoBuildTag(t *testing.T) {
	if got := firstLine(t, "lamp_unsupported.go"); strings.HasPrefix(got, "//go:build") {
		t.Errorf("lamp_unsupported.go has a build tag: %q", got)
	}
}

// lamp_window_darwin.go is the Mac lamp (lane scope X4). It cannot run in
// this test binary, so the test reads its text: the borderless style, the
// level above normal windows, every Space, never the key window, and no call
// that takes the focus.
func TestLampDarwinWindowFileStylesThePanel(t *testing.T) {
	s := readText(t, "lamp_window_darwin.go")
	for _, want := range []string{
		`import "C"`,
		"NSWindowStyleMaskBorderless",
		"NSStatusWindowLevel",
		"NSWindowCollectionBehaviorCanJoinAllSpaces",
		"canBecomeKeyWindow",
		"canBecomeMainWindow",
		"NSApplicationActivationPolicyAccessory",
		"orderFrontRegardless",
		"visibleFrame",
		"syscall.Flock",
		"lampMutexName",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("lamp_window_darwin.go lacks %q", want)
		}
	}
	for _, banned := range []string{"makeKeyAndOrderFront", "makeKeyWindow", "activateIgnoringOtherApps", "instance.Acquire("} {
		if strings.Contains(s, banned) {
			t.Errorf("lamp_window_darwin.go contains %q", banned)
		}
	}
}

// lamp_run.go is shared with Windows, so it holds no platform call.
func TestLampRunHoldsNoSyscall(t *testing.T) {
	if strings.Contains(readText(t, "lamp_run.go"), "syscall") {
		t.Error("lamp_run.go uses syscall")
	}
}

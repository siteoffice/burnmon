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
		"main.go":        "//go:build windows || (darwin && cgo)",
		"main_other.go":  "//go:build !windows && !darwin",
		"windowstate.go": "//go:build windows",
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

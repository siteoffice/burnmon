package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readText(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func firstLine(t *testing.T, name string) string {
	t.Helper()
	return strings.TrimRight(strings.SplitN(readText(t, name), "\n", 2)[0], "\r")
}

func TestCCBuildTags(t *testing.T) {
	want := map[string]string{
		"main.go":              "//go:build windows || (darwin && cgo)",
		"main_nocgo_darwin.go": "//go:build darwin && !cgo",
		"main_other.go":        "//go:build !windows && !darwin",
		"window_windows.go":    "//go:build windows",
		"window_darwin.go":     "//go:build darwin && cgo",
	}
	for _, f := range []string{"bindings.go", "embeds.go", "decide.go", "agents.go"} {
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
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		// Built from parts so this file does not itself match
		// internal\repocheck's TestWebviewGoOnlyInDarwinCgoFiles.
		has := strings.Contains(readText(t, f), `"github.com/webview/`+`webview_go"`)
		if has != (f == "window_darwin.go") {
			t.Errorf("%s: imports webview_go = %v, want only window_darwin.go to", f, has)
		}
	}
}

func TestCCNoCgoDarwinMessage(t *testing.T) {
	want := "controlcentre on macOS needs a cgo build on a Mac: CGO_ENABLED=1 go build ./cmd/controlcentre"
	if !strings.Contains(readText(t, "main_nocgo_darwin.go"), want) {
		t.Errorf("main_nocgo_darwin.go must print %q", want)
	}
}

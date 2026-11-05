package main

import (
	"os"
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

func TestBurnmonBuildTags(t *testing.T) {
	want := map[string]string{
		"main.go":            "//go:build windows || (darwin && cgo)",
		"main_other.go":      "//go:build !windows && !(darwin && cgo)",
		"browser.go":         "//go:build !windows && !(darwin && cgo)",
		"browser_test.go":    "//go:build !windows && !(darwin && cgo)",
		"window_windows.go":  "//go:build windows",
		"window_darwin.go":   "//go:build darwin && cgo",
		"platform_darwin.go": "//go:build darwin",
	}
	for f, tag := range want {
		if got := firstLine(t, f); got != tag {
			t.Errorf("%s first line = %q, want %q", f, got, tag)
		}
	}
	for _, f := range []string{"uicheck_devserver.go", "binding_names.go"} {
		if got := firstLine(t, f); strings.HasPrefix(got, "//go:build") {
			t.Errorf("%s must have no build line, has %q", f, got)
		}
	}
}

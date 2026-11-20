//go:build windows || darwin

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The comment box names the paste key of the host: Cmd+V on macOS, Ctrl+V elsewhere.
func TestCommentPlaceholderNamesThePasteKey(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	b, err := os.ReadFile("page.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)
	s := strings.Index(page, "function pasteKey(")
	if s < 0 {
		t.Fatal("page.html has no pasteKey")
	}
	e := s + strings.Index(page[s:], "\n")
	js := page[s:e] + "\nconsole.log(['MacIntel', 'Win32', 'Linux x86_64', ''].map(pasteKey).join('|'));"
	out, err := exec.Command(node, "-e", js).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if got, want := strings.TrimSpace(string(out)), "Cmd+V|Ctrl+V|Ctrl+V|Ctrl+V"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if !strings.Contains(page, "cmt.placeholder = ") || !strings.Contains(page, "pasteKey(navigator.platform)") {
		t.Fatal("the comment box does not take its placeholder from pasteKey")
	}
}

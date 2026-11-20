package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The header count reads "0 sessions", "1 session", "2 sessions".
func TestHeaderSessionsLabelSingular(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	b, err := os.ReadFile("page.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)
	s := strings.Index(page, "function sessionsLabel(")
	if s < 0 {
		t.Fatal("page.html has no sessionsLabel")
	}
	e := s + strings.Index(page[s:], "\n")
	js := page[s:e] + "\nconsole.log([0, 1, 2].map(sessionsLabel).join('|'));"
	out, err := exec.Command(node, "-e", js).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if got, want := strings.TrimSpace(string(out)), "0 sessions|1 session|2 sessions"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if !strings.Contains(page, "sessionsEl.textContent = sessionsLabel(") {
		t.Fatal("the header does not use sessionsLabel")
	}
}

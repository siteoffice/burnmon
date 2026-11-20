package report

import (
	"os/exec"
	"strings"
	"testing"
)

// The total row of the Sessions table reads "0 sessions", "1 session", "2 sessions".
func TestSessionsCountSingular(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	s := strings.Index(tpl, "sessions_count:")
	if s < 0 {
		t.Fatal("template.html has no sessions_count")
	}
	e := s + strings.Index(tpl[s:], "\n")
	line := strings.TrimRight(strings.TrimSpace(tpl[s:e]), ",")
	js := "var X = {" + line + "};\nconsole.log([0, 1, 2].map(function(n){ return X.sessions_count({n:n}); }).join('|'));"
	out, err := exec.Command(node, "-e", js).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if got, want := strings.TrimSpace(string(out)), "0 sessions|1 session|2 sessions"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

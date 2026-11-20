package controls

import (
	"runtime"
	"testing"
)

// The refusal names the binary that exists on the operating system: claude.exe
// on Windows, claude on macOS and Linux.
func TestWhyClaudeNamesTheBinaryPerOS(t *testing.T) {
	for goos, want := range map[string]string{
		"windows": "needs claude.exe: set claude_path in burnmon.json",
		"darwin":  "needs claude: set claude_path in burnmon.json",
		"linux":   "needs claude: set claude_path in burnmon.json",
	} {
		if got := whyClaudeFor(goos); got != want {
			t.Errorf("%s: got %q, want %q", goos, got, want)
		}
	}
}

// The text the service gives for a missing claude is the one for this host.
func TestServiceWhyClaudeFollowsTheHost(t *testing.T) {
	r := newRig(t)
	r.env.ClaudePath, r.env.ClaudeWhy = "", ""
	r.rebuild()
	if got, want := r.svc.Availability().Why[Stop], whyClaudeFor(runtime.GOOS); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := StopArgv("", "ab12"); err == nil || err.Error() != whyClaudeFor(runtime.GOOS) {
		t.Fatalf("StopArgv error: %v", err)
	}
}

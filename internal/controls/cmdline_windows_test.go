//go:build windows

package controls

import (
	"strings"
	"syscall"
	"testing"
)

func TestCommandLineMatchesExec(t *testing.T) {
	argv := []string{`C:\Program Files\x\claude.exe`, `say "hi"`, `dir\`, "plain"}
	got := CommandLine(argv)
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = syscall.EscapeArg(a)
	}
	want := strings.Join(parts, " ")
	if got != want {
		t.Fatalf("CommandLine = %q, want %q", got, want)
	}
	if !strings.HasPrefix(got, `"C:\Program Files\x\claude.exe" `) {
		t.Errorf("exe path with a space is not quoted: %q", got)
	}
	if !strings.Contains(got, `"say \"hi\""`) {
		t.Errorf("quote not escaped: %q", got)
	}
	if !strings.Contains(got, `dir\ `) {
		t.Errorf("trailing backslash changed: %q", got)
	}
}

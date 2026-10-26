//go:build windows

package controls

import (
	"strings"
	"syscall"
)

// CommandLine renders argv the way os/exec builds the Windows command line:
// every argument through syscall.EscapeArg, joined by a space.
func CommandLine(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = syscall.EscapeArg(a)
	}
	return strings.Join(parts, " ")
}

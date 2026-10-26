//go:build !windows

package controls

import (
	"strconv"
	"strings"
)

// CommandLine joins argv with spaces, quoting an argument when it needs it.
func CommandLine(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		if a == "" || strings.ContainsAny(a, " \t\"'\\") {
			a = strconv.Quote(a)
		}
		parts[i] = a
	}
	return strings.Join(parts, " ")
}

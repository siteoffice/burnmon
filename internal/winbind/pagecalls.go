package winbind

import (
	"regexp"
	"sort"
	"strings"
)

// PageCalls returns the sorted unique names of the form P[A-Z]\w*, with P one
// of prefixes, that src calls, minus the names src defines itself (function
// NAME( or NAME = function, with an optional window. before NAME).
func PageCalls(src string, prefixes ...string) []string {
	if len(prefixes) == 0 {
		return nil
	}
	quoted := make([]string, len(prefixes))
	for i, p := range prefixes {
		quoted[i] = regexp.QuoteMeta(p)
	}
	name := `((?:` + strings.Join(quoted, "|") + `)[A-Z]\w*)`
	calls := regexp.MustCompile(`\b` + name + `\s*\(`)
	defFunc := regexp.MustCompile(`\bfunction\s+` + name + `\s*\(`)
	defAssign := regexp.MustCompile(`\b(?:window\.)?` + name + `\s*=\s*function\b`)

	defined := map[string]bool{}
	for _, re := range []*regexp.Regexp{defFunc, defAssign} {
		for _, m := range re.FindAllStringSubmatch(src, -1) {
			defined[m[1]] = true
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range calls.FindAllStringSubmatch(src, -1) {
		if defined[m[1]] || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

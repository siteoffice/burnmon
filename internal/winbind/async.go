package winbind

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AsyncResolveJS builds the JS statement that delivers a result computed off
// the UI thread back into the page. A binding that returns at once and does
// its real work in a goroutine hands the result to a page-side resolver
// (window.<resolver>) this way, instead of through go-webview2's own per-call
// Promise, which only resolves with whatever the bound function returns
// synchronously. args are the resolver's arguments in order: a key and the
// data for a binding that can have more than one call in flight (so the
// resolver can tell which call a result belongs to), or the data alone for a
// fixed periodic poll. encoding/json's default HTML escaping makes a literal
// "</script>" impossible in any marshalled argument.
func AsyncResolveJS(resolver string, args ...any) (string, error) {
	parts := make([]string, len(args))
	for i, a := range args {
		b, err := json.Marshal(a)
		if err != nil {
			return "", err
		}
		parts[i] = string(b)
	}
	return fmt.Sprintf("window.%s && window.%s(%s)", resolver, resolver, strings.Join(parts, ", ")), nil
}

//go:build windows || (darwin && cgo)

package main

import "testing"

// escLT is how encoding/json writes "<": a backslash, u, 003c.
const escLT = `\` + "u003c"

// asyncResolveJS hands a result to a page-side resolver by key. The text is
// part of the page contract, so it is pinned byte for byte.
func TestAsyncResolveJSText(t *testing.T) {
	cases := []struct {
		name     string
		resolver string
		key      string
		data     any
		want     string
	}{
		{"object", "__bmTurnResolve", "k1", map[string]int{"a": 1}, `window.__bmTurnResolve && window.__bmTurnResolve("k1", {"a":1})`},
		{"string with quote", "__bmHistoryResolve", `r"1`, "x<y", `window.__bmHistoryResolve && window.__bmHistoryResolve("r\"1", "x` + escLT + `y")`},
		{"nil data", "__bmExportHistoryResolve", "export", nil, `window.__bmExportHistoryResolve && window.__bmExportHistoryResolve("export", null)`},
	}
	for _, c := range cases {
		got, err := asyncResolveJS(c.resolver, c.key, c.data)
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v; want %q", c.name, got, err, c.want)
		}
	}
	if _, err := asyncResolveJS("r", "k", make(chan int)); err == nil {
		t.Error("an unmarshalable value must return an error")
	}
}

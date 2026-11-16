//go:build windows || (darwin && cgo)

package main

import "testing"

// escLT is how encoding/json writes "<": a backslash, u, 003c.
const escLT = `\` + "u003c"

// The two bdev helpers hand a result to a page-side resolver, with and without
// a key. The text is part of the page contract, so it is pinned byte for byte.
func TestBdevAsyncResolveJSText(t *testing.T) {
	got, err := bdevAsyncResolveJS("__bdevExportResolve", map[string]int{"a": 1})
	if want := `window.__bdevExportResolve && window.__bdevExportResolve({"a":1})`; err != nil || got != want {
		t.Errorf("no key: got %q, %v; want %q", got, err, want)
	}
	got, err = bdevAsyncResolveJS("__bdevTodoLoginResolve", nil)
	if want := `window.__bdevTodoLoginResolve && window.__bdevTodoLoginResolve(null)`; err != nil || got != want {
		t.Errorf("nil data: got %q, %v; want %q", got, err, want)
	}
	got, err = bdevAsyncResolveJSWithKey("__bdevTurnDetailResolve", `k"1`, "x<y")
	if want := `window.__bdevTurnDetailResolve && window.__bdevTurnDetailResolve("k\"1", "x` + escLT + `y")`; err != nil || got != want {
		t.Errorf("with key: got %q, %v; want %q", got, err, want)
	}
	if _, err := bdevAsyncResolveJS("r", make(chan int)); err == nil {
		t.Error("an unmarshalable value must return an error")
	}
	if _, err := bdevAsyncResolveJSWithKey("r", "k", make(chan int)); err == nil {
		t.Error("an unmarshalable value must return an error")
	}
}

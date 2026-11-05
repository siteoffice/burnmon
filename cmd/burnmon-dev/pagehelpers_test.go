package main

import (
	"strings"
	"testing"
)

// pageFuncBody returns the page script text of one function, up to the next
// two-space-indented function.
func pageFuncBody(t *testing.T, name string) string {
	t.Helper()
	i := strings.Index(pageHTML, "function "+name+"(")
	if i < 0 {
		t.Fatalf("page has no function %s", name)
	}
	body := pageHTML[i+1:]
	if j := strings.Index(body, "\n  function "); j > 0 {
		body = body[:j]
	}
	return body
}

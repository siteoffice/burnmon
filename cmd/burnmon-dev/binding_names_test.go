package main

import (
	"sort"
	"testing"

	"burnmon/internal/winbind"
)

func TestDevBindingNamesCoverPage(t *testing.T) {
	if len(devBindingNames) != 24 {
		t.Fatalf("devBindingNames has %d names, want 24", len(devBindingNames))
	}
	listed := map[string]bool{}
	for _, n := range devBindingNames {
		if listed[n] {
			t.Errorf("duplicate binding name %q", n)
		}
		listed[n] = true
	}
	called := winbind.PageCalls(pageHTML, "bdev")
	for _, n := range called {
		if !listed[n] {
			t.Errorf("page calls %s, which devBindingNames lacks", n)
		}
		if n == "bdevAgentMap" {
			t.Error("bdevAgentMap is a page-defined wrapper and must not show as a page call")
		}
	}
	if !sort.StringsAreSorted(called) {
		t.Error("PageCalls result is not sorted")
	}
}

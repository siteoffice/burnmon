package main

import (
	"sort"
	"testing"

	"burnmon/internal/winbind"
)

// The lamp page calls only names with the prefix lamp, and lampBindingNames
// lists exactly the ones the page calls (plan choice C9). The page defines
// lampRender itself, so PageCalls does not list it.
func TestLampBindingNamesEqualPageCalls(t *testing.T) {
	called := winbind.PageCalls(lampHTML, "lamp")
	if !sort.StringsAreSorted(called) {
		t.Error("PageCalls result is not sorted")
	}
	listed := map[string]bool{}
	for _, n := range lampBindingNames {
		if listed[n] {
			t.Errorf("duplicate binding name %q", n)
		}
		listed[n] = true
	}
	for _, n := range called {
		if !listed[n] {
			t.Errorf("page calls %s, which lampBindingNames lacks", n)
		}
	}
	callSet := map[string]bool{}
	for _, n := range called {
		callSet[n] = true
	}
	for _, n := range lampBindingNames {
		if !callSet[n] {
			t.Errorf("lampBindingNames lists %s, which the page never calls", n)
		}
	}
	if len(lampBindingNames) == 0 {
		t.Error("lampBindingNames is empty")
	}
	for _, want := range []string{"lampState", "lampNextLine", "lampOpenDev", "lampSize", "lampGlim"} {
		if !listed[want] {
			t.Errorf("lampBindingNames lacks %s", want)
		}
	}
}

// The lamp page never calls a BurnMon Dev binding: the two processes bind
// different names, and the lamp has no control.
func TestLampPageCallsNoDevBinding(t *testing.T) {
	if got := winbind.PageCalls(lampHTML, "bdev"); len(got) != 0 {
		t.Errorf("lamp page calls other bindings: %v", got)
	}
}

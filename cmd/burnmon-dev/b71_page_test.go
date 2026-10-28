//go:build windows

package main

import (
	"strings"
	"testing"
)

func TestIdleTableHasTwoGroups(t *testing.T) {
	body := idleFn(t, "mapIdleTableHTML")
	for _, want := range []string{"Done, needs close-out", "Other sprints and flows"} {
		if !strings.Contains(body, want) {
			t.Errorf("mapIdleTableHTML lacks the group %q", want)
		}
	}
	for _, bad := range []string{"Waits on you", "Waiting for you", "waits_on_you"} {
		if strings.Contains(body, bad) {
			t.Errorf("mapIdleTableHTML still mentions %q", bad)
		}
	}
	if n := strings.Count(body, "function(i){ return"); n != 2 {
		t.Errorf("mapIdleTableHTML has %d group predicates, want 2", n)
	}
}

func TestGearControlsTextFullWidth(t *testing.T) {
	page := idlePage()
	s := strings.Index(page, "#secControls .ctlwhy{")
	if s < 0 {
		t.Fatal("page.html lacks a #secControls .ctlwhy rule")
	}
	e := strings.Index(page[s:], "}")
	rule := page[s : s+e]
	for _, want := range []string{"max-width:none", "width:0", "min-width:100%"} {
		if !strings.Contains(rule, want) {
			t.Errorf("the #secControls .ctlwhy rule lacks %q", want)
		}
	}
}

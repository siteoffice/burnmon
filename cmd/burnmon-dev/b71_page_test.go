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

func TestSeclistJoinsTheFitPass(t *testing.T) {
	page := idlePage()
	for _, want := range []string{"function fitCollectHost(", "function fitListFloor(", "kind:'seclist'", "hostGain", "fit.items = ", "body.fit-scroll .sechost.sec-fill{flex:0 0 auto}"} {
		if !strings.Contains(page, want) {
			t.Errorf("page.html lacks %q", want)
		}
	}
}

func TestSeclistHasHouseScrollbar(t *testing.T) {
	page := idlePage()
	for _, want := range []string{
		".seclist::-webkit-scrollbar{width:8px}",
		".seclist::-webkit-scrollbar-track{background:var(--panel2)}",
		".seclist::-webkit-scrollbar-thumb{background:var(--sb)}",
		".seclist::-webkit-scrollbar-thumb:hover{background:var(--sb-hover)}",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page.html lacks %q", want)
		}
	}
	s := strings.Index(page, ".seclist{")
	if s < 0 {
		t.Fatal("no .seclist rule")
	}
	rule := page[s : s+strings.Index(page[s:], "}")]
	if !strings.Contains(rule, "scrollbar-color:var(--sb) var(--panel2)") {
		t.Errorf(".seclist rule lacks the house scrollbar-color: %s", rule)
	}
}

func TestSessionsTableInsideSeclist(t *testing.T) {
	page := idlePage()
	s := strings.Index(page, "function secBodySessions(")
	if s < 0 {
		t.Fatal("no secBodySessions")
	}
	body := page[s:]
	if e := strings.Index(body, "\n  function "); e > 0 {
		body = body[:e]
	}
	if !strings.Contains(body, `class="seclist seclist-tbl"`) {
		t.Error("secBodySessions does not wrap the table in .seclist.seclist-tbl")
	}
}
//go:build windows

package main

import (
	"strings"
	"testing"
)

// idleFn returns the body of one page function, from its header to the next
// "  function " at the same indent.
func idleFn(t *testing.T, name string) string {
	t.Helper()
	s := strings.Index(idlePage(), "function "+name+"(")
	if s < 0 {
		t.Fatalf("page.html lacks function %s", name)
	}
	e := strings.Index(idlePage()[s+1:], "\n  function ")
	if e < 0 {
		t.Fatalf("cannot find the end of %s", name)
	}
	return idlePage()[s : s+1+e]
}

func TestMapIdleButtonNotAbsolute(t *testing.T) {
	body := idleFn(t, "mapIdleHTML")
	if strings.Contains(body, "position:absolute") {
		t.Error("mapIdleHTML still pins the idle list with position:absolute")
	}
	for _, want := range []string{"mapIdleBtn", "no session", "no agent"} {
		if !strings.Contains(body, want) {
			t.Errorf("mapIdleHTML lacks %q", want)
		}
	}
	if strings.Contains(body, "<details") {
		t.Error("mapIdleHTML still draws a details element")
	}
}

func TestMapIdleTable(t *testing.T) {
	body := idleFn(t, "mapIdleTableHTML")
	last := -1
	for _, want := range []string{"Done, needs close-out", "Waits on you", "Other sprints and flows"} {
		i := strings.Index(body, want)
		if i < 0 {
			t.Errorf("mapIdleTableHTML lacks the heading %q", want)
			continue
		}
		if i < last {
			t.Errorf("heading %q is out of order", want)
		}
		last = i
	}
	for _, want := range []string{"Name", "Project", "Kind", "Waited", "Links", "secfile", "ctlChips('closeout'", "age_seconds", "esc("} {
		if !strings.Contains(body, want) {
			t.Errorf("mapIdleTableHTML lacks %q", want)
		}
	}
	if !strings.Contains(body, "b.age_seconds - a.age_seconds") {
		t.Error("the idle table does not sort longest waited first")
	}
}

func TestMapIdleWiring(t *testing.T) {
	for _, want := range []string{"var mapIdleOpen", "window.__bdevMap", "idle: function", "mappanel wide", "id=\"mapIdleBar\"", "? MAP.FRAMEHEAD : 0"} {
		if !strings.Contains(idlePage(), want) && !strings.Contains(idlePage(), strings.Replace(want, "mappanel wide", ".mappanel.wide", 1)) {
			t.Errorf("page.html lacks %q", want)
		}
	}
	s := strings.Index(idlePage(), "e.key !== 'Escape' && e.key !== 'Esc') return;\n    var overlay")
	if s < 0 {
		t.Fatal("cannot find the Esc handler")
	}
	h := idlePage()[s : s+600]
	i, m := strings.Index(h, "mapIdleOpen"), strings.Index(h, "mapCloseView()")
	if i < 0 || m < 0 || i > m {
		t.Error("Esc does not close the idle table before the map")
	}
	// A box click swaps the panel back to that box.
	if !strings.Contains(idlePage(), "mapIdleOpen = false;\n    mapSel = mapSel === id") {
		t.Error("a box click does not close the idle table")
	}
}

// idlePage is the page with LF line ends, so the checks do not depend on the
// checkout's line endings.
func idlePage() string { return strings.ReplaceAll(pageHTML, "\r\n", "\n") }

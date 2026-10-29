//go:build windows

package main

import (
	"strings"
	"testing"
)

// B7 items 1, 2 and 6: the fit pass shrinks and shares height, it never hides a part.
func TestFitPassNeverHides(t *testing.T) {
	for _, gone := range []string{
		"FIT_OFF_ORDER",
		"secCutNote",
		"hidden, the window is too small",
		"fit-dropped",
		"applyTodoGrow",
		"fitRunOff",
	} {
		if strings.Contains(pageHTML, gone) {
			t.Errorf("page still carries %q", gone)
		}
	}
	if strings.Contains(pageHTML, `id="secCutNote"`) {
		t.Error("markup still has #secCutNote")
	}
}

func TestFitPassHasFloorsAndScroll(t *testing.T) {
	for _, want := range []string{
		"var FIT_CHART_MIN = 60, FIT_ROWS_MIN = 3, FIT_TABLE_MIN_ROWS = 2, FIT_CHART_GROW = 1.6;",
		"window.__bdevFit = { scroll: true, fitScroll: false, hidden: [], columns: [], items: [] };",
		"body.fit-scroll #pageScroll",
		"function fitAbsorb(items, amt)",
		"classList.add('fit-scroll')",
	} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page lacks %s", want)
		}
	}
}

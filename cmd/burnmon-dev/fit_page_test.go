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

// 2026-11-15: every section and pane takes its typed percent; the page scrolls
// only when the scrollbar is on and the sections add up to more than 100.
func TestFitPassHasFloorsAndScroll(t *testing.T) {
	for _, want := range []string{
		"function pctPlan(shown, vals, restKey)",
		"function pctCheck(mode, secs, panes, scrollbar)",
		"function pctPanes(mode, sec, box, rec)",
		"window.__bdevFit = { scroll: true, fitScroll: false, hidden: [], sections: {}, panes: {} };",
		"body.scroll-on #pageScroll{",
		"var pct = sp0.starved ? pctEqual(secs) : (sp0.over && !scroll) ? pctScaled(secs, sp0.pct, sp0.sum) : sp0.pct;",
	} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	for _, gone := range []string{"FIT_CHART_MIN", "function fitAbsorb(", "fit-scroll", "__bdevTodoLayout"} {
		if strings.Contains(pageHTML, gone) {
			t.Errorf("page still carries %s", gone)
		}
	}
}

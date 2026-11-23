//go:build windows || darwin

package main

import (
	"strings"
	"testing"
)

// TestGitRowKeepsRoomForItsTexts pins D15 of the Mac test: with the menu as icons and text the
// pane is narrow, and the Git page cut the repo name ("burn..."), the branch ("deta..."), the
// stage labels of the strip ("Chang...", "Releas...") and the "Stops at" text. The repo and
// branch cells had a flex basis of 0, so they gave way to everything else; the strip buttons
// could shrink to nothing. Each now has a basis that holds a short word and shrinks slowly, so the
// "Stops at" text gives way first. min-width stays 0 (TestPageGitNoHorizontalScroll, rule 11).
// Text that is still shortened shows in full on hover (HOVER_SEL and the data-title cells).
func TestGitRowKeepsRoomForItsTexts(t *testing.T) {
	for sel, want := range map[string]string{
		".g1":                "flex:.8 .3 64px",
		".g2":                "flex:.8 .3 56px",
		".glane":             "flex:0 0 124px",
		".lstrip button.lst": "flex:1 .3 58px",
		".lbig":              "flex:1 1 auto",
		".gst":               "flex:2.2 1 120px",
	} {
		if !strings.Contains(compact(ruleFor(t, sel)), compact(want)) {
			t.Errorf("%s lacks %s", sel, want)
		}
	}
	for _, cell := range []string{".g1", ".g2", ".gst", ".lbig b", ".lbig span", ".lst span"} {
		if !strings.Contains(compact(ruleFor(t, cell)), "text-overflow:ellipsis") {
			t.Errorf("%s does not shorten with an ellipsis", cell)
		}
		if !strings.Contains(pageHTML, cell) {
			t.Errorf("%s is not in the page", cell)
		}
	}
	hs := pageHTML[strings.Index(pageHTML, "var HOVER_SEL = "):]
	hs = hs[:strings.Index(hs, ";")]
	for _, sel := range []string{".g1", ".g2", ".gst", ".lbig b", ".lbig span", ".lst span", ".ell"} {
		if !strings.Contains(hs, sel) {
			t.Errorf("HOVER_SEL lacks %s: a shortened text would not show in full on hover", sel)
		}
	}
}

// TestPlanningNoPlanFileLineOnce pins D17: with no plan file the line was written to the header
// and to the pane. It now stands once, in the pane, as on the other empty pages (Waiting,
// Agents, Git).
func TestPlanningNoPlanFileLineOnce(t *testing.T) {
	pp := pageFunc(t, "pagePlanning")
	i := strings.Index(pp, "if (!state.plan) {")
	if i < 0 {
		t.Fatal("pagePlanning has no no-plan branch")
	}
	branch := pp[i:]
	branch = branch[:strings.Index(branch, "return;")]
	if n := strings.Count(branch, ", line)"); n != 1 {
		t.Errorf("the no-plan branch writes the line %d times, want 1", n)
	}
	if strings.Contains(branch, "lead.appendChild") {
		t.Error("the no-plan branch still writes to the header (lead)")
	}
	if !strings.Contains(branch, "pane.appendChild(h('p', 'muted', line))") {
		t.Error("the no-plan branch does not write the line into the pane")
	}
}

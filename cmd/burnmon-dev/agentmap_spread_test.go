package main

import (
	"strings"
	"testing"
)

// The agent map spreads its columns over the window (B2, Decisions item 12).
// The pixel proof is uicheck d35; this guards the wiring in the embedded page.
func TestPageAgentMapSpreadWiring(t *testing.T) {
	for _, want := range []string{
		"function mapColW(cols, availW)",
		"function mapGeometry(L, availW)",
		"mapGeometry(L, scrollEl.clientWidth)",
		"if(mapOpen) mapRender();",
	} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page.html lacks %q", want)
		}
	}
	// The panel for the selected box is drawn before the geometry reads the width.
	panel := strings.Index(pageHTML, "mapRenderPanel(sel, L);")
	geom := strings.Index(pageHTML, "var g = mapGeometry(L, scrollEl.clientWidth)")
	if panel < 0 || geom < 0 || panel > geom {
		t.Errorf("mapRenderPanel(sel, L) must come before mapGeometry (panel %d, geometry %d)", panel, geom)
	}
}

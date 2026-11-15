//go:build windows || darwin

package main

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"burnmon/internal/centre"
)

// panelNeedPx is what Approve, Send back and Open board need on one line in
// the right panel: the labels measure 188 px in Segoe UI 12.5 px bold
// (measured 2026-11-15 with System.Drawing), plus 3 x 22 px button padding
// and borders, 2 x 6 px gaps, 24 px panel padding and the 1 px border.
const panelNeedPx = 188 + 66 + 12 + 24 + 1

func panelPresets(t *testing.T) map[string]int {
	t.Helper()
	m := regexp.MustCompile(`var PANEL_PX = \{narrow: (\d+), normal: (\d+), wide: (\d+)\}, PANEL_MIN = (\d+), PANEL_MAX = (\d+);`).FindStringSubmatch(pageHTML)
	if m == nil {
		t.Fatal("page.html has no PANEL_PX line")
	}
	out := map[string]int{}
	for i, k := range []string{"narrow", "normal", "wide", "min", "max"} {
		out[k], _ = strconv.Atoi(m[i+1])
	}
	return out
}

// Item 5 (first look 2026-11-15): the default panel fits the three buttons,
// Settings has a Right panel width row, the edge drags and saves panel_px,
// and a double-click resets it.
func TestRightPanelWidth(t *testing.T) {
	p := panelPresets(t)
	if p["normal"] < panelNeedPx {
		t.Errorf("normal panel %d px, the three buttons need %d", p["normal"], panelNeedPx)
	}
	if !(p["narrow"] < p["normal"] && p["normal"] < p["wide"]) {
		t.Errorf("presets not ordered: %v", p)
	}
	if p["min"] != centre.PanelMinPx || p["max"] != centre.PanelMaxPx {
		t.Errorf("page bounds %d..%d, Go bounds %d..%d", p["min"], p["max"], centre.PanelMinPx, centre.PanelMaxPx)
	}
	if centre.DefaultSettings().Panel != "normal" {
		t.Errorf("default panel %q, want normal", centre.DefaultSettings().Panel)
	}
	if !strings.Contains(pageHTML, `class="redge" id="redge"`) {
		t.Error("the right panel has no edge element")
	}
	mustContain(t, "applySettings", "app.style.setProperty('--rp', panelWidth(s) + 'px')")
	mustContain(t, "panelEdge", "'pointerdown'", "'pointermove'", "'pointerup'", "setPointerCapture",
		"saveSetting('panel_px', w)", "'dblclick'", "saveSetting('panel_px', 0)", "app.clientWidth / 2")
	mustContain(t, "saveSetting", "panel: settings.panel", "panel_px: settings.panel_px || 0", "if (key === 'panel') s.panel_px = 0;")
	mustContain(t, "pageSettings", "'Right panel width'", "['narrow', 'Narrow']", "['normal', 'Normal']", "['wide', 'Wide']",
		"saveSetting('panel', v)")
	if !strings.Contains(pageFunc(t, "start"), "panelEdge();") {
		t.Error("start does not wire panelEdge")
	}
	if strings.Contains(compact(ruleFor(t, ".body")), "256px") {
		t.Error(".body still pins the right panel at 256 px")
	}

	// panelWidth: a dragged width wins inside the bounds, else the preset.
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	js := "var PANEL_PX = {narrow: 256, normal: 320, wide: 400}, PANEL_MIN = 220, PANEL_MAX = 640;\n" + pageFunc(t, "panelWidth") +
		"\nconsole.log([panelWidth({panel:'wide', panel_px:0}), panelWidth({panel:'narrow', panel_px:345}), panelWidth({panel:'x', panel_px:9999}), panelWidth({})].join(','));"
	b, err := exec.Command(node, "-e", js).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, b)
	}
	if got := strings.TrimSpace(string(b)); got != "400,345,320,320" {
		t.Errorf("panelWidth = %s, want 400,345,320,320", got)
	}
}

// Item 6: the unit Flows and Sprints tables are one code path on one fixed
// column grid (Project | Name | Stage or Phase | Waits | State), header and
// rows alike, with the Waits number right-aligned.
func TestUnitTablesShareGrid(t *testing.T) {
	grid := compact(ruleFor(t, ".rw.ugrid"))
	if !strings.Contains(grid, "display:grid") ||
		!regexp.MustCompile(`grid-template-columns:\d+pxminmax\(0,\d+fr\)\d+px\d+pxminmax\(0,\d+fr\)`).MatchString(grid) {
		t.Errorf(".rw.ugrid is not a fixed five column grid: %s", grid)
	}
	if !strings.Contains(compact(ruleFor(t, ".rw.ugrid .c4")), "text-align:right") {
		t.Error("the Waits column is not right-aligned")
	}
	body := pageFunc(t, "pageUnit")
	for _, want := range []string{
		"h('div', 'rw hd ugrid')",
		"[['Project', 'c1'], ['Name', 'c2'], [fl ? 'Stage' : 'Phase', 'c3'], ['Waits', 'c4'], ['State', 'c5']]",
		"btn('rw ugrid' + (r.waiting ? ' warn' : ''), null, pick)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("pageUnit lacks %s", want)
		}
	}
	if strings.Count(body, "'rw hd ugrid'") != 1 {
		t.Error("the unit tables have more than one header path")
	}
}

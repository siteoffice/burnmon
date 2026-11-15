//go:build windows

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"burnmon/internal/sections"
)

// Item 2 (second look 2026-11-15): every gear button sits at the right end of
// its row, its hint text left of it. uicheck d48 measures it in a window.
func TestGearButtonsRightAligned(t *testing.T) {
	page := idlePage()
	for _, want := range []string{
		"#secSizes .setsave .setbtn, #histAdmin .histrow .histbtn{order:2; margin-left:auto; flex:none}",
		"#histAdmin .histgrid .histbtn{justify-self:end}",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
}

// Item 5: the header stays one line from 1280 px; the big number scales.
func TestHeaderOneLineFrom1280(t *testing.T) {
	page := idlePage()
	block := page[strings.Index(page, "@media (min-width: 1280px){"):]
	block = block[:strings.Index(block, "}\n}")+3]
	for _, want := range []string{"header{flex-wrap:nowrap", ".hltotal{font-size:clamp(24px, 1.9vw, 48px)", ".hstat{flex:0 1 auto; min-width:0}"} {
		if !strings.Contains(block, want) {
			t.Errorf("the 1280 px header block lacks %q:\n%s", want, block)
		}
	}
	// It must come after the 1152 px rule, or that rule's 48 px wins.
	if strings.Index(page, "@media (min-width: 1152px){") > strings.Index(page, "@media (min-width: 1280px){") {
		t.Error("the 1280 px header block sits before the 1152 px rule")
	}
}

// Item 4: a pane never gets less than its minimum; the others give up the px
// in proportion to their spare, and with no spare every pane keeps its minimum.
func TestPaneMinimumClamp(t *testing.T) {
	out := gearNode(t, sections.Default(), `console.log(JSON.stringify([
  pctClampMin(['a','b','c'], {a:20, b:200, c:100}, {a:60, b:40, c:40}),
  pctClampMin(['a','b'], {a:30, b:30}, {a:50, b:50}),
  pctClampMin(['a','b'], {a:100, b:100}, {a:40, b:40})
]));`)
	var got []map[string]float64
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("bad node output %q: %v", out, err)
	}
	// 40 px short: b has 160 spare, c 60, so b gives 29 and c 11.
	if got[0]["a"] != 60 || got[0]["b"] != 171 || got[0]["c"] != 89 {
		t.Errorf("clamp 1 = %v, want a 60, b 171, c 89", got[0])
	}
	if got[1]["a"] != 50 || got[1]["b"] != 50 {
		t.Errorf("clamp 2 = %v, want both at their minimum", got[1])
	}
	if got[2]["a"] != 100 || got[2]["b"] != 100 {
		t.Errorf("clamp 3 = %v, want unchanged", got[2])
	}
}

// Item 4: every pane clips, the whole-row pass runs after each layout pass and
// each render, a "+N more" line takes the cut rows' place, only row lists snap,
// and a percent box says its minimum on hover.
func TestPanesStayInTheirBoxes(t *testing.T) {
	page := idlePage()
	for _, want := range []string{
		"#todoPanel{overflow:hidden}",
		".pct-cut{display:none !important}",
		"var PCT_SNAP_LISTS = '.seclist, .ticker, .todobody';",
		"(more.firstChild || more).textContent = '+' + count + ' more';",
		"var PCT_CHARTS = ['#heatmapGrid'];",
		"i.title = m ? 'minimum ' + m.px + ' px, about ' + m.pct + ' percent at this window size' : '';",
		"var pxs = pctClampMin(ids, pctPixels(ids, pct, room), mins);",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	body := idleFn(t, "applyPanelFit")
	for _, want := range []string{"pctWholeReset();", "pctWhole();", "pctMinTips();", "pctMinRecord(fit, el);"} {
		if !strings.Contains(body, want) {
			t.Errorf("applyPanelFit lacks %q", want)
		}
	}
	if !strings.Contains(idleFn(t, "pctWholeWatch"), "{ childList: true, subtree: true }") {
		t.Error("the whole-row pass does not watch row changes")
	}
}

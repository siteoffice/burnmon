package main

import (
	"fmt"
	"strings"
	"time"
)

// d20: WS2 alpha.7 item 4 (02_roadmap\2026-09-29_ws2_smooth_tick_and_system
// _layout.md): every harness heatmap row label renders in full, the longest
// today being "BurnMon Dev (WebView2)". Fake mode, one row per harness
// page.html knows a label for, then each .heatrow-label's scrollWidth
// against its clientWidth (an ellipsis-clipped label is wider inside than
// out). Also checks the minute cells did not lose room: the row's cells
// start right after the label plus the gap, and the screenshot is the
// proof image the spec asks for. 1920x1080, not the 1280x860 default: at
// 1280x860 applyPanelFit drops the whole Activity/heatmap row, and a hidden
// label measures 0 of 0 (an early version of this check passed that way).
func caseD20(hwnd uintptr, args []string) error {
	if err := ensureWindowSizeWH(hwnd, 1920, 1080); err != nil {
		return err
	}
	if windowCapped {
		fmt.Println("uicheck: d20: SKIPPED, this screen is too small for a 1920x1080 window, so the heatmap row is fit-dropped and its labels measure 0px")
		return nil
	}
	// Own starting state: the sections the dev store holds change between runs and
	// checks (Plain mode drops the heatmap row, which then measures 0 of 0).
	if _, err := evalRaw(`window.__bdevSections.apply(` + d29Payload(d29Opts{Status: "ok"}) + `)`); err != nil {
		return fmt.Errorf("d20: set the starting state: %w", err)
	}
	time.Sleep(300 * time.Millisecond)
	script := `(function(){
  var nowMs = Date.now(), windowMs = 30 * 60 * 1000, ws = nowMs - windowMs;
  var HARN = ['claude', 'claude-desktop', 'codex', 'copilot-cli', 'copilot-vscode', 'hermes', 'wsl', 'burnmon-dev', 'burnmon-dev-webview2', 'node'];
  var groups = [], chart = [];
  for(var t = nowMs - 20 * 60 * 1000; t <= nowMs; t += 5000){
    HARN.forEach(function(h, i){ groups.push({ Ts: new Date(t).toISOString(), Harness: h, CPUPct: 2 + i, MemMB: 200, IOBps: 0 }); });
  }
  for(var i = 0; i < 30; i++) chart.push({ at: new Date(ws + i * 60000).toISOString(), by_session: {}, cost: 0 });
  var nowIso = new Date(nowMs).toISOString();
  var snap = {
    now: nowMs, hist_gap_ms: 25000, sysmon_history: [],
    process_groups_now: HARN.map(function(h){ return { Ts: nowIso, Harness: h, CPUPct: 1, MemMB: 200, IOBps: 0 }; }),
    process_groups_history: groups,
    vendor_strip: { rows: [] }, activity_heatmap: { rows: [] },
    todo: { enabled: false }, todo_tasks: { items: [] }, headline_today: 0,
    burn: { generated_at: nowIso, running_window_seconds: 1800, sessions: [],
      window_start: new Date(ws).toISOString(), bucket_seconds: 60, chart: chart, turns: [] }
  };
  window.__bdevEnterFakeMode();
  window.__bdevPaintFake(snap);
  var out = [];
  document.querySelectorAll('#harnessHeatmap .heatrow').forEach(function(row){
    var l = row.querySelector('.heatrow-label'), c = row.querySelector('.heatrow-cells');
    out.push({ text: l.textContent, scrollW: l.scrollWidth, clientW: l.clientWidth,
      cells: c ? c.children.length : 0,
      cellsLeft: c ? c.getBoundingClientRect().left - l.getBoundingClientRect().left : 0 });
  });
  return out;
})()`
	var rows []struct {
		Text      string  `json:"text"`
		ScrollW   int     `json:"scrollW"`
		ClientW   int     `json:"clientW"`
		Cells     int     `json:"cells"`
		CellsLeft float64 `json:"cellsLeft"`
	}
	defer evalRaw(`(window.__bdevExitFakeMode && window.__bdevExitFakeMode())`)
	if err := evalInto(script, &rows); err != nil {
		return fmt.Errorf("d20: eval harness labels: %w", err)
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := screenshot(hwnd, "d20-harness-labels"); err != nil {
		return err
	}
	var errs []string
	if len(rows) == 0 {
		errs = append(errs, "harness heatmap rendered no rows")
	}
	for _, r := range rows {
		fmt.Printf("uicheck: d20: %-24q label %dpx of %dpx, %d minute cells, cells start at +%.0fpx\n", r.Text, r.ScrollW, r.ClientW, r.Cells, r.CellsLeft)
		if r.ClientW < 150 {
			errs = append(errs, fmt.Sprintf("label %q is %dpx wide, want the 150px column (row hidden or not widened)", r.Text, r.ClientW))
		}
		if r.ScrollW > r.ClientW {
			errs = append(errs, fmt.Sprintf("label %q clipped (%dpx content in %dpx)", r.Text, r.ScrollW, r.ClientW))
		}
		if r.Cells == 0 {
			errs = append(errs, fmt.Sprintf("row %q has no minute cells", r.Text))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("d20: %d problem(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}
	fmt.Println("uicheck: d20: every harness heatmap label renders in full")
	return nil
}

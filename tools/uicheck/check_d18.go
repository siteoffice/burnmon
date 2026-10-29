package main

import (
	"fmt"
	"strings"
	"time"
)

// d18: WS2 alpha.7 item 1's proof (02_roadmap\2026-09-29_ws2_smooth_tick_and
// _system_layout.md): 70 minutes of fake history delivered as deltas renders
// the same System chart, process-groups sparklines and harness heatmap as
// the same data delivered in full. Drives page.html's own
// __bdevEnterFakeMode/__bdevPaintFake hooks, never live data. The delta run
// replays what a live page sees: one full answer 10 minutes in (a first
// call), then a delta every 60s, then the last 30s one second at a time, a
// 10s-cadence stretch and a 60s gap included, so pruning both rings on the
// way is exercised, not just appending. Both runs end on the same now, and
// the final paint of each is compared: the canvas bitmap (toDataURL), the
// chart's own gap segments (__bdevHistDebug), #harnessHeatmap and
// #processGroups markup.
func caseD18(hwnd uintptr, args []string) error {
	if err := ensureWindowSizeWH(hwnd, 1280, 860); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)

	script := `(function(){
  var nowMs = Math.floor(Date.now() / 1000) * 1000;
  var startMs = nowMs - 70 * 60 * 1000;
  var windowMs = 30 * 60 * 1000;
  var HARN = ['claude', 'codex', 'claude-desktop', 'wsl'];
  var sys = [], groups = [];
  for(var t = startMs; t <= nowMs; t += 1000){
    var ago = (nowMs - t) / 1000;
    if(ago <= 900 && ago > 840) continue;                      // 60s gap
    if(ago <= 1500 && ago > 1200 && ago % 10 !== 0) continue;  // hidden 10s cadence
    var s = t / 1000;
    sys.push({ Ts: new Date(t).toISOString(), CPUPct: 40 + 30 * Math.sin(s / 97), MemUsedMB: 8000 + 50 * Math.sin(s / 211), MemTotalMB: 16000,
      DiskReadBps: 1e6 * (1 + Math.sin(s / 13)), DiskWriteBps: 5e5 * (1 + Math.cos(s / 17)), NetDownBps: 2e5 * (1 + Math.sin(s / 7)), NetUpBps: 1e5, GPUPct: 10 + 5 * Math.sin(s / 31) });
    HARN.forEach(function(h, i){
      groups.push({ Ts: new Date(t).toISOString(), Harness: h, CPUPct: Math.abs(10 * Math.sin(s / (20 + 7 * i))), MemMB: 300 + 40 * i, IOBps: 1000 * i });
    });
  }
  function ms(r){ return new Date(r.Ts).getTime(); }
  var groupsNow = HARN.map(function(h, i){ return { Ts: new Date(nowMs).toISOString(), Harness: h, CPUPct: 5 + i, MemMB: 300 + 40 * i, IOBps: 1000 * i }; });
  function snap(now, sysRows, sysDelta, groupRows, groupDelta){
    var ws = now - windowMs, chart = [];
    for(var i = 0; i < 30; i++) chart.push({ at: new Date(ws + i * 60000).toISOString(), by_session: {}, cost: 0 });
    return {
      now: now, hist_gap_ms: 25000,
      sysmon_history: sysRows, sysmon_history_delta: sysDelta,
      process_groups_now: groupsNow, process_groups_history: groupRows, process_groups_history_delta: groupDelta,
      vendor_strip: { rows: [] }, activity_heatmap: { rows: [] },
      todo: { enabled: false }, todo_tasks: { items: [] }, headline_today: 0,
      burn: { generated_at: new Date(now).toISOString(), running_window_seconds: 1800, sessions: [],
        window_start: new Date(ws).toISOString(), bucket_seconds: 60, chart: chart, turns: [] }
    };
  }
  function capture(){
    return {
      canvas: document.getElementById('histCanvas').toDataURL(),
      segs: JSON.stringify(window.__bdevHistDebug && window.__bdevHistDebug.segments),
      heat: document.getElementById('harnessHeatmap').innerHTML,
      groups: document.getElementById('processGroups').innerHTML
    };
  }
  // Rows in (from, now]: what app.go's buffers hold up to now, after from.
  function upTo(rows, now, from){ return rows.filter(function(r){ var t = ms(r); return t <= now && t > from; }); }

  window.__bdevEnterFakeMode();

  // Full: one answer at now, what the pre-alpha.7 snapshot sent every tick
  // (sys trimmed to the chart window, groups to the 60 min buffer).
  window.__bdevPaintFake(snap(nowMs, upTo(sys, nowMs, nowMs - windowMs - 1), false, upTo(groups, nowMs, nowMs - 3600000 - 1), false));
  var full = capture();

  // Delta: a first call 10 minutes in, then deltas.
  var t0 = startMs + 10 * 60 * 1000;
  window.__bdevPaintFake(snap(t0, upTo(sys, t0, t0 - windowMs - 1), false, upTo(groups, t0, t0 - 3600000 - 1), false));
  var sent = t0, paints = 1;
  function step(to){
    window.__bdevPaintFake(snap(to, upTo(sys, to, sent), true, upTo(groups, to, sent), true));
    sent = to; paints++;
  }
  while(sent + 60000 <= nowMs - 30000) step(sent + 60000);
  while(sent < nowMs) step(sent + 1000);
  var delta = capture();

  return {
    paints: paints, sysRows: sys.length, groupRows: groups.length,
    canvasSame: full.canvas === delta.canvas, canvasLen: full.canvas.length,
    segsSame: full.segs === delta.segs, segCount: JSON.parse(full.segs || '[]').length,
    heatSame: full.heat === delta.heat, heatLen: full.heat.length,
    groupsSame: full.groups === delta.groups, groupsLen: full.groups.length
  };
})()`

	var out struct {
		Paints     int  `json:"paints"`
		SysRows    int  `json:"sysRows"`
		GroupRows  int  `json:"groupRows"`
		CanvasSame bool `json:"canvasSame"`
		CanvasLen  int  `json:"canvasLen"`
		SegsSame   bool `json:"segsSame"`
		SegCount   int  `json:"segCount"`
		HeatSame   bool `json:"heatSame"`
		HeatLen    int  `json:"heatLen"`
		GroupsSame bool `json:"groupsSame"`
		GroupsLen  int  `json:"groupsLen"`
	}
	defer evalRaw(`(window.__bdevExitFakeMode && window.__bdevExitFakeMode())`)
	if err := evalInto(script, &out); err != nil {
		return fmt.Errorf("d18: eval delta-vs-full paint: %w", err)
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := screenshot(hwnd, "d18-delta-history"); err != nil {
		return err
	}
	fmt.Printf("uicheck: d18: %d sys rows, %d group rows, delta run %d paints; canvas %d chars, %d chart segments, heatmap %d chars, process groups %d chars\n",
		out.SysRows, out.GroupRows, out.Paints, out.CanvasLen, out.SegCount, out.HeatLen, out.GroupsLen)

	var errs []string
	if !out.CanvasSame {
		errs = append(errs, "System chart canvas differs between full and delta delivery")
	}
	if !out.SegsSame {
		errs = append(errs, "System chart gap segments differ between full and delta delivery")
	}
	if !out.HeatSame {
		errs = append(errs, "#harnessHeatmap differs between full and delta delivery")
	}
	if !out.GroupsSame {
		errs = append(errs, "#processGroups differs between full and delta delivery")
	}
	if out.SegCount < 2 || out.HeatLen == 0 || out.GroupsLen == 0 {
		errs = append(errs, fmt.Sprintf("fixture did not render enough to compare (segments=%d heatmap=%d groups=%d)", out.SegCount, out.HeatLen, out.GroupsLen))
	}
	if len(errs) > 0 {
		return fmt.Errorf("d18: %d problem(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}
	fmt.Println("uicheck: d18: delta history renders the same as full history (chart bitmap, gap segments, harness heatmap, process groups)")
	return nil
}

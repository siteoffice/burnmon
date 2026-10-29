package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// charts/d10: moved from check_d10.go.

// d10: no-scroll patch section 1, "nothing in this chart except the
// stacked bars, the y-scale if any, and the HH:mm axis" - a uicheck case
// that finds no marker or line elements in the burn chart. The turn-marker
// overlay SVG (#turnMarkers) is gone from the DOM outright, so this checks
// both that it stays gone and that #burnBars never grows an <svg>/<line>
// some future change might reintroduce.
func caseD10(hwnd uintptr, args []string) error {
	if exists, err := evalBool(`!!document.getElementById('turnMarkers')`); err != nil {
		return fmt.Errorf("check #turnMarkers absent: %w", err)
	} else if exists {
		return fmt.Errorf("#turnMarkers still exists in the DOM; section 1 was supposed to remove the turn-marker overlay entirely")
	}
	if n, err := evalRaw(`document.querySelectorAll('#burnBars line, #burnBars svg').length`); err != nil {
		return fmt.Errorf("check #burnBars for marker/line elements: %w", err)
	} else if string(n) != "0" {
		return fmt.Errorf("found %s marker/line element(s) inside #burnBars; section 1 wants bars only", n)
	}
	fmt.Println("uicheck: d10: burn chart has no marker/line elements")
	if _, err := screenshot(hwnd, "d10-burn-chart-no-markers"); err != nil {
		return err
	}
	return nil
}

// charts/d11: moved from check_d11.go.

// d11: no-scroll patch section 5, "Verify and close" - a uicheck case that
// records paint timestamps per panel for 30s and fails if any panel paints
// outside the shared tick, or if the panels' paints within one tick are
// more than one frame apart. page.html's own paintMark (called once per
// panel inside paintTick, itself always run inside one requestAnimationFrame
// per tick) appends {name, t} to window.__bdevPaintLog; this reads that log
// back through the dev eval channel rather than re-deriving timing any other
// way, so it is checking the real, already-running page's own behaviour,
// not a simulation of it.
func caseD11(hwnd uintptr, args []string) error {

	// A clean slate: truncate whatever the log already holds so this
	// check's own 30s window is not polluted by ticks from before it
	// started (startup, an earlier check in the same uicheck-dev.ps1 run).
	if _, err := evalRaw(`(window.__bdevPaintLog = [])`); err != nil {
		return fmt.Errorf("reset paint log: %w", err)
	}
	// Same reset for WS2 alpha.3 item 3's own change counters (below).
	if _, err := evalRaw(`(window.__bdevChangeCounts = {sysmon:0, processGroups:0})`); err != nil {
		return fmt.Errorf("reset change counts: %w", err)
	}

	time.Sleep(30 * time.Second)

	type mark = d11Mark
	var log []mark
	if err := evalInto(`window.__bdevPaintLog || []`, &log); err != nil {
		return fmt.Errorf("read paint log: %w", err)
	}
	if len(log) == 0 {
		return fmt.Errorf("no paint marks recorded in 30s; is paintMark actually being called from paintTick?")
	}

	// Group into ticks by the 'tickEnd' marker paintTick itself always
	// emits last: every mark since the previous tickEnd (or the start of
	// the log) belongs to that tick.
	var ticks [][]mark
	var cur []mark
	for _, m := range log {
		cur = append(cur, m)
		if m.Name == "tickEnd" {
			ticks = append(ticks, cur)
			cur = nil
		}
	}
	// A trailing partial tick (cur, cut off by the 30s window ending
	// mid-paint, which should not happen since paintTick is synchronous)
	// is simply not appended to ticks above - dropped rather than judged
	// as a tick that never finished.
	if len(ticks) < 3 {
		return fmt.Errorf("only %d complete tick(s) recorded in 30s; expected several at the ~2s refresh cadence", len(ticks))
	}

	const expectedPanels = 12 // clock, sysmon, sysmonHistory, burn, vendorStrip, activityHeatmap, processGroups, harnessHeatmap, todo, tickEnd (+2 slack for a future panel)
	// frameToleranceMs is deliberately much looser than one 16.7ms frame
	// at 60fps: paintTick's own panels all run synchronously inside one
	// requestAnimationFrame callback with nothing else able to interleave
	// between them, but that synchronous work itself (heatmap grids,
	// table rebuilds, a canvas redraw) can legitimately take longer than
	// one frame budget to execute on real hardware without ever
	// violating "the same tick" - what this threshold actually needs to
	// catch is a panel painting on a LATER tick entirely, which would
	// show up roughly refresh_ms (~1000ms default as of phase 5b)
	// later, three orders of magnitude past this tolerance either way.
	const frameToleranceMs = 250.0

	for i, t := range ticks {
		if len(t) < 8 {
			return fmt.Errorf("tick %d only painted %d panel mark(s), expected close to %d - looks like a partial paint", i, len(t), expectedPanels)
		}
		names := map[string]bool{}
		for _, m := range t {
			names[m.Name] = true
		}
		if !names["tickEnd"] {
			return fmt.Errorf("tick %d has no tickEnd mark", i)
		}
		// The spread runs over the panel marks only. tickEnd is marked after
		// applyPanelFit (page.html paintTick), the layout pass that B7 and
		// B7.1 grew into a shrink loop; it is not a panel paint, and under
		// load it alone took up to ~110ms of a 278ms tick. A panel painting
		// on another tick still lands ~refresh_ms away from the rest.
		var sorted []mark
		for _, m := range t {
			if m.Name != "tickEnd" {
				sorted = append(sorted, m)
			}
		}
		sort.Slice(sorted, func(a, b int) bool { return sorted[a].T < sorted[b].T })
		spread := sorted[len(sorted)-1].T - sorted[0].T
		if spread > frameToleranceMs {
			return fmt.Errorf("tick %d: panel paints spread over %.1fms (want within ~%.0fms, one frame) - some panel is painting outside the shared requestAnimationFrame", i, spread, frameToleranceMs)
		}
	}

	// The fit pass gets its own budget (B7.2 spec scope item 9), since the
	// spread above leaves it out.
	var fitMax float64
	for i, t := range ticks {
		ms, err := d11CheckFitPass(i, t)
		if err != nil {
			return err
		}
		if ms > fitMax {
			fitMax = ms
		}
	}
	fmt.Printf("uicheck: d11: slowest fit pass %.1fms over %d ticks (budget %.0fms)\n", fitMax, len(ticks), d11FitBudgetMs)

	// Consecutive ticks should land close to refresh_ms apart (default
	// 1000ms as of phase 5b). Using each tick's own tickEnd timestamp as its instant.
	var tickEndTimes []float64
	for _, t := range ticks {
		for _, m := range t {
			if m.Name == "tickEnd" {
				tickEndTimes = append(tickEndTimes, m.T)
			}
		}
	}
	var refreshMs float64
	if err := evalInto(`window.__bdevRefreshMs || 1000`, &refreshMs); err != nil {
		refreshMs = 2000
	}
	for i := 1; i < len(tickEndTimes); i++ {
		gap := tickEndTimes[i] - tickEndTimes[i-1]
		// Generous tolerance: a slow round trip to bdevSnapshotNow, or a
		// GC pause, can stretch one tick's gap without it being a real
		// bug; what matters is it is not drifting wildly off cadence.
		if gap < refreshMs*0.4 || gap > refreshMs*2.5 {
			return fmt.Errorf("tick %d to %d: %.0fms apart, expected close to the %.0fms refresh cadence", i-1, i, gap, refreshMs)
		}
	}

	// WS2 alpha.3 item 3's own proof: the process-groups walk used to
	// lag the rest of the System zone on its own 3s cadence, painting
	// stale data on the ~2 out of 3 ticks its own data had not actually
	// moved - paintMark above only proves every panel paints together,
	// not that a new sample actually landed for processGroups as often
	// as it did for sysmon. window.__bdevChangeCounts (page.html's
	// trackChange, reset above) counts how many render ticks actually
	// carried a new (previously unseen) payload for each snapshot
	// field, over this same 30s window - both a.latest and
	// a.latestGroups carry their own sample's Ts, which changes every
	// time that sampler runs, so this counts "did a fresh sample land
	// this tick", not a deeper semantic value comparison (a process
	// group whose CPU/RAM/IO happen to read identically to the
	// previous tick still counts as changed here, because its Ts did).
	var changeCounts struct {
		Sysmon        int `json:"sysmon"`
		ProcessGroups int `json:"processGroups"`
	}
	if err := evalInto(`window.__bdevChangeCounts || {}`, &changeCounts); err != nil {
		return fmt.Errorf("read change counts: %w", err)
	}
	fmt.Printf("uicheck: d11: shared beat over 30s - sysmon changed %d time(s), processGroups changed %d time(s)\n",
		changeCounts.Sysmon, changeCounts.ProcessGroups)
	if changeCounts.Sysmon == 0 || changeCounts.ProcessGroups == 0 {
		return fmt.Errorf("d11: shared beat: sysmon changed %d time(s), processGroups changed %d time(s) - one of them never updated in 30s", changeCounts.Sysmon, changeCounts.ProcessGroups)
	}
	// Generous tolerance (item 3 only asks that they move on the same
	// beat, not that every single render tick sees a brand new sample
	// for both - the paint loop's own refresh_ms cadence and the 1s
	// sample ticker are two independent tickers, so ordinary scheduling
	// jitter between them can occasionally let one paint tick see the
	// same a.latestGroups twice, or miss one; a rare procSampler.Tick
	// error, logged but not fatal, also leaves a.latestGroups
	// momentarily stale for one internal sample): a 3x difference
	// either way would still indicate one of the two is stuck on a
	// materially slower cadence than the other, not just jitter.
	lo, hi := changeCounts.Sysmon, changeCounts.ProcessGroups
	if lo > hi {
		lo, hi = hi, lo
	}
	if hi > lo*3 {
		return fmt.Errorf("d11: shared beat: sysmon changed %d time(s), processGroups changed %d time(s) - too far apart for a shared 1s cadence", changeCounts.Sysmon, changeCounts.ProcessGroups)
	}

	fmt.Printf("uicheck: d11: %d complete ticks in 30s, all panels paint within %.0fms of each other, cadence holds around %.0fms\n",
		len(ticks), frameToleranceMs, refreshMs)
	return nil
}

// d11Mark is one entry of window.__bdevPaintLog.
type d11Mark struct {
	Name string  `json:"name"`
	T    float64 `json:"t"`
}

// d11FitBudgetMs is what the fit pass alone may take in one tick (Wilco,
// B7.2 spec scope item 9).
const d11FitBudgetMs = 250.0

// d11CheckFitPass times applyPanelFit in one tick. paintTick marks 'todo'
// right before applyPanelFit and 'tickEnd' right after it (page.html
// paintTick), so the gap between those two marks is the fit pass alone.
func d11CheckFitPass(i int, tick []d11Mark) (float64, error) {
	todo, end := -1, -1
	for j, m := range tick {
		switch m.Name {
		case "todo":
			todo = j
		case "tickEnd":
			end = j
		}
	}
	if todo < 0 || end < todo {
		return 0, fmt.Errorf("d11: tick %d: no todo mark before tickEnd, cannot time the fit pass", i)
	}
	ms := tick[end].T - tick[todo].T
	if ms > d11FitBudgetMs {
		return ms, fmt.Errorf("d11: tick %d: fit pass took %.0f ms, budget %.0f ms", i, ms, d11FitBudgetMs)
	}
	return ms, nil
}

// charts/d12: moved from check_d12.go.

// d12: headline patch section 1 - a uicheck case that fails when any turn
// ticker row is shorter than its own text line height, the exact regression
// 11_ticker_broken.png showed (cropToFit's own scrollHeight<=clientHeight
// check never trips because flexbox just shrinks each .trow to fit instead
// of letting the ticker's own box overflow, so every row silently squeezes
// to a sliver - only the amber tag's top border and a few dashes survive).
// Sweeps the same five sizes d9 already uses, since #turnTicker's own fixed
// row height and scroll behaviour (section 1) do not depend on window size.
func caseD12(hwnd uintptr, args []string) error {
	sizes := [][2]int32{{1024, 768}, {1280, 860}, {1152, 2048}, {1920, 1080}, {2560, 1440}}
	for _, size := range sizes {
		if err := ensureWindowSizeWH(hwnd, size[0], size[1]); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)

		var result struct {
			RowCount   int      `json:"rowCount"`
			ContainerH float64  `json:"containerH"`
			Bad        []string `json:"bad"`
		}
		script := `(function(){
  var el = document.getElementById('turnTicker');
  var rows = el ? el.querySelectorAll('.trow') : [];
  var bad = [];
  for (var i = 0; i < rows.length; i++) {
    var r = rows[i];
    var h = r.getBoundingClientRect().height;
    var lh = parseFloat(getComputedStyle(r).lineHeight);
    if (isNaN(lh)) continue;
    if (h < lh - 1) {
      bad.push('row ' + i + ': height=' + h.toFixed(1) + ' lineHeight=' + lh.toFixed(1));
    }
  }
  return { rowCount: rows.length, containerH: el ? el.clientHeight : 0, bad: bad };
})()`
		if err := evalInto(script, &result); err != nil {
			return fmt.Errorf("d12 %dx%d: eval ticker row heights: %w", size[0], size[1], err)
		}
		// A zero (or near-zero) row count means this run's own store had
		// no turns in the last 30 minutes to check against - the check
		// would otherwise silently pass having verified nothing (found
		// by review, 2026-09-26).
		if result.RowCount == 0 {
			return fmt.Errorf("d12 %dx%d: no ticker rows found to check (no turns in the last 30 minutes?)", size[0], size[1])
		}
		// A collapsed #turnTicker box (0 or near-0 clientHeight) would
		// show nothing regardless of each row's own individually-correct
		// height, since overflow-y:auto's own scrollable content still
		// reports full row heights even while none of it is visible
		// (found by review, 2026-09-26).
		if result.ContainerH < 20 {
			return fmt.Errorf("d12 %dx%d: #turnTicker itself is only %.1fpx tall (less than one row) despite %d row(s)", size[0], size[1], result.ContainerH, result.RowCount)
		}
		if len(result.Bad) > 0 {
			max := len(result.Bad)
			if max > 10 {
				max = 10
			}
			return fmt.Errorf("d12 %dx%d: %d ticker row(s) shorter than their own text line height: %v", size[0], size[1], len(result.Bad), result.Bad[:max])
		}
		fmt.Printf("uicheck: d12 %dx%d: %d ticker row(s) (container %.0fpx), none shorter than their own text line height\n", size[0], size[1], result.RowCount, result.ContainerH)
	}
	return nil
}

// charts/d13: moved from check_d13.go.

// d13: WS2 alpha.3 item 2, "all 20 core bars the same length" - a uicheck
// case that measures every .corebar's own clientWidth at a real window size
// and fails unless every one of the 20 bars is exactly the same width. Before
// the fixed left/right label slots (.corelabel-l/.corelabel-r, page.html),
// each bar's track length depended on its own value text's width ("0%" vs
// "100%"), so min and max differed; this proves they no longer do.
func caseD13(hwnd uintptr, args []string) error {
	time.Sleep(500 * time.Millisecond)

	var widths []float64
	script := `(function(){
  var bars = document.querySelectorAll('.corebar');
  var out = [];
  for (var i = 0; i < bars.length; i++) out.push(bars[i].clientWidth);
  return out;
})()`
	if err := evalInto(script, &widths); err != nil {
		return fmt.Errorf("d13: eval .corebar widths: %w", err)
	}
	if len(widths) == 0 {
		return fmt.Errorf("d13: no .corebar elements found")
	}
	min, max := widths[0], widths[0]
	for _, w := range widths {
		if w < min {
			min = w
		}
		if w > max {
			max = w
		}
	}
	fmt.Printf("uicheck: d13: %d core bars, width min=%.0fpx max=%.0fpx\n", len(widths), min, max)
	if min != max {
		return fmt.Errorf("d13: core bar widths are not equal (min=%.0fpx max=%.0fpx, want equal across all %d bars)", min, max, len(widths))
	}
	return nil
}

// charts/d14: moved from check_d14.go.

// d14: WS2 follow-up (2026-09-28), month labels - a uicheck case that fails
// if any rendered .heatmonth label is clipped by its own box (scrollWidth
// greater than clientWidth) at 1920x1080, the one size wide enough to
// render most of the heatmap's columns. alpha.3's own fix made check_d9
// pass by clipping full month names ("Mar" -> "Ma") with overflow:hidden on
// a fixed 9px box; this proves the real bug (a label wider than its box) is
// gone, not just hidden from d9's scrollbar sweep.
func caseD14(hwnd uintptr, args []string) error {
	time.Sleep(500 * time.Millisecond)

	var labels []struct {
		Text        string  `json:"text"`
		ScrollWidth float64 `json:"scrollWidth"`
		ClientWidth float64 `json:"clientWidth"`
	}
	script := `(function(){
  var els = document.querySelectorAll('.heatmonth');
  var out = [];
  for (var i = 0; i < els.length; i++) {
    var el = els[i];
    if (!el.textContent) continue;
    out.push({ text: el.textContent, scrollWidth: el.scrollWidth, clientWidth: el.clientWidth });
  }
  return out;
})()`
	if err := evalInto(script, &labels); err != nil {
		return fmt.Errorf("d14: eval .heatmonth labels: %w", err)
	}
	if len(labels) == 0 {
		return fmt.Errorf("d14: no rendered .heatmonth labels found")
	}
	var offenders []string
	for _, l := range labels {
		if l.ScrollWidth > l.ClientWidth+1 {
			offenders = append(offenders, fmt.Sprintf("%q (scrollWidth=%.0f clientWidth=%.0f)", l.Text, l.ScrollWidth, l.ClientWidth))
		}
	}
	if len(offenders) > 0 {
		return fmt.Errorf("d14: %d of %d label(s) clipped: %v", len(offenders), len(labels), offenders)
	}
	fmt.Printf("uicheck: d14: %d rendered month label(s), none clipped\n", len(labels))
	return nil
}

// charts/d15: moved from check_d15.go.

// d15: WS2 follow-up (2026-09-28), one colour family per vendor - a
// uicheck case that drives paintTick with a synthetic burn snapshot (two
// Cowork sessions, two Claude Code sessions, one Copilot CLI session, never
// live data) via page.html's own __bdevEnterFakeMode/__bdevPaintFake test
// hooks, then proves: a session's burn-chart segment colour equals its
// session-card colour; every session of one vendor shares that vendor's
// live vendor-strip hue and saturation (read from the DOM, not hardcoded,
// so this tracks whatever HARNESS_HUE actually is - saturation checked
// alongside hue since HARNESS_HUE.other's own hue happens to sit within a
// few degrees of Cowork's, so hue alone would not have caught the grey-
// cache regression below for a Cowork session); the two shades within each
// vendor differ by a real lightness step; and no session's hue drifts into
// another vendor's range. Also exercises item 3's legend label-collision
// fix (04_assets\...\2026-09-28_ws2_vendor_colour_families.md's own item
// 4): the two Cowork sessions share one project name on purpose.
//
// v0.4.0-alpha.6 extends this with a second paint, `fake-late-1`: a session
// whose tokens land in the chart's by_session bucket for one or more ticks
// before its vendor is known anywhere else (absent from sessions, turns and
// chart_agents) - exactly the gap that used to let sessionColor fall back
// to HARNESS_HUE.other and cache that grey in sessionColorMap for life,
// found live by Wilco (a Cowork session stuck grey in the chart, its legend
// dot and its card, while the vendor strip and the legend's own text both
// correctly read Cowork). Once the vendor arrives on a later tick, the
// session's chart segment, legend dot and card must all recolour to a real
// Cowork shade, not stay locked on the first tick's grey guess.
func caseD15(hwnd uintptr, args []string) error {
	time.Sleep(300 * time.Millisecond)

	vendorStripJS := `{ rows: [
      {agent:'cowork', agent_label:'Cowork', today:0, week:0, month:0},
      {agent:'claude-code', agent_label:'Claude Code', today:0, week:0, month:0},
      {agent:'copilot-cli', agent_label:'Copilot CLI', today:0, week:0, month:0}
    ] }`

	// lateSid is unique per run (found by review, 2026-09-28): d15
	// attaches to whatever burnmon-dev.exe window uicheck already
	// launched, so page.html's own sessionColorMap - the very cache this
	// scenario is about - survives from one d15 run to the next inside
	// that same window. A fixed 'fake-late-1' would already be cached
	// with its real Cowork shade by a second run, so tick 1 would return
	// that shade instead of a fresh grey guess and the regression guard
	// below would misfire, not because the fix regressed but because the
	// scenario's own precondition (vendor genuinely not yet known) no
	// longer held.
	lateSid := fmt.Sprintf("fake-late-%d", time.Now().UnixNano())

	script := `(function(){
  var nowMs = Date.now();
  function iso(ms){ return new Date(ms).toISOString(); }
  var sessions = [
    {session_id:'fake-cw-1', vendor:'cowork', agent:'cowork', surface:'cowork', model:'', project:'host-cwd', start:iso(nowMs-600000), last_turn:iso(nowMs-60000), turn_count:3, context:1000, context_window:200000, tokens:5000, cost:0.12, cache_hit_ratio:0},
    {session_id:'fake-cw-2', vendor:'cowork', agent:'cowork', surface:'cowork', model:'', project:'host-cwd', start:iso(nowMs-500000), last_turn:iso(nowMs-30000), turn_count:2, context:800, context_window:200000, tokens:4000, cost:0.10, cache_hit_ratio:0},
    {session_id:'fake-cc-1', vendor:'claude-code', agent:'claude-code', surface:'claude-code', model:'', project:'proj-alpha', start:iso(nowMs-400000), last_turn:iso(nowMs-20000), turn_count:5, context:1200, context_window:200000, tokens:7000, cost:0.20, cache_hit_ratio:0},
    {session_id:'fake-cc-2', vendor:'claude-code', agent:'claude-code', surface:'claude-code', model:'', project:'proj-beta', start:iso(nowMs-300000), last_turn:iso(nowMs-10000), turn_count:4, context:900, context_window:200000, tokens:6000, cost:0.15, cache_hit_ratio:0},
    {session_id:'fake-cli-1', vendor:'copilot-cli', agent:'copilot-cli', surface:'copilot-cli', model:'', project:'proj-gamma', start:iso(nowMs-200000), last_turn:iso(nowMs-5000), turn_count:2, context:500, context_window:200000, tokens:3000, cost:0.05, cache_hit_ratio:0}
  ];
  var bySession = {};
  sessions.forEach(function(s, i){ bySession[s.session_id] = {fresh: 1000+i*100, cache_write:0, cache_read:0, output:200}; });
  // ` + lateSid + `: on this first tick its tokens are only in the chart's
  // own by_session bucket - it is deliberately absent from sessions, turns
  // and chart_agents, so agentBySessionMap has no way to resolve its vendor yet.
  bySession['` + lateSid + `'] = {fresh: 900, cache_write:0, cache_read:0, output:150};
  var snap = {
    now: nowMs,
    sysmon_history: [], process_groups_now: [], process_groups_history: [],
    vendor_strip: ` + vendorStripJS + `,
    activity_heatmap: {rows: []},
    todo: {enabled:false}, todo_tasks: {items: []}, headline_today: 0,
    burn: {
      generated_at: iso(nowMs), running_window_seconds: 1800,
      sessions: sessions, window_start: iso(nowMs-1800000), bucket_seconds: 60,
      chart: [{at: iso(nowMs), by_session: bySession, cost: 0}],
      turns: [], chart_agents: {}
    }
  };
  window.__bdevEnterFakeMode();
  window.__bdevPaintFake(snap);

  var seg = document.querySelector('#burnBars .bar i[data-session="` + lateSid + `"]');
  return { earlyLateChart: seg ? getComputedStyle(seg).backgroundColor : null };
})()`

	// Deferred unconditionally, not after the eval/screenshot calls
	// below: a JS exception partway through script (after
	// __bdevEnterFakeMode already ran) would otherwise return early and
	// leave the real page's own tick timer stopped forever. The guard
	// inside is a no-op if fake mode was never actually entered.

	var early struct {
		EarlyLateChart string `json:"earlyLateChart"`
	}
	if err := evalInto(script, &early); err != nil {
		return fmt.Errorf("d15: eval fake-session paint (tick 1): %w", err)
	}
	if early.EarlyLateChart == "" {
		return fmt.Errorf("d15: %s rendered no chart segment on tick 1 (before its vendor is known)", lateSid)
	}

	// Tick 2: lateSid's vendor has now arrived (present in sessions, same
	// as any other running session) - still in the chart bucket, so this
	// proves the recolour, not just a fresh, never-cached paint.
	script2 := `(function(){
  var nowMs = Date.now();
  function iso(ms){ return new Date(ms).toISOString(); }
  var sessions = [
    {session_id:'fake-cw-1', vendor:'cowork', agent:'cowork', surface:'cowork', model:'', project:'host-cwd', start:iso(nowMs-600000), last_turn:iso(nowMs-60000), turn_count:3, context:1000, context_window:200000, tokens:5000, cost:0.12, cache_hit_ratio:0},
    {session_id:'fake-cw-2', vendor:'cowork', agent:'cowork', surface:'cowork', model:'', project:'host-cwd', start:iso(nowMs-500000), last_turn:iso(nowMs-30000), turn_count:2, context:800, context_window:200000, tokens:4000, cost:0.10, cache_hit_ratio:0},
    {session_id:'fake-cc-1', vendor:'claude-code', agent:'claude-code', surface:'claude-code', model:'', project:'proj-alpha', start:iso(nowMs-400000), last_turn:iso(nowMs-20000), turn_count:5, context:1200, context_window:200000, tokens:7000, cost:0.20, cache_hit_ratio:0},
    {session_id:'fake-cc-2', vendor:'claude-code', agent:'claude-code', surface:'claude-code', model:'', project:'proj-beta', start:iso(nowMs-300000), last_turn:iso(nowMs-10000), turn_count:4, context:900, context_window:200000, tokens:6000, cost:0.15, cache_hit_ratio:0},
    {session_id:'fake-cli-1', vendor:'copilot-cli', agent:'copilot-cli', surface:'copilot-cli', model:'', project:'proj-gamma', start:iso(nowMs-200000), last_turn:iso(nowMs-5000), turn_count:2, context:500, context_window:200000, tokens:3000, cost:0.05, cache_hit_ratio:0},
    {session_id:'` + lateSid + `', vendor:'cowork', agent:'cowork', surface:'cowork', model:'', project:'host-cwd-late', start:iso(nowMs-100000), last_turn:iso(nowMs-2000), turn_count:1, context:400, context_window:200000, tokens:900, cost:0.03, cache_hit_ratio:0}
  ];
  var bySession = {};
  sessions.forEach(function(s, i){ bySession[s.session_id] = {fresh: 1000+i*100, cache_write:0, cache_read:0, output:200}; });
  var chartAgents = {};
  sessions.forEach(function(s){ chartAgents[s.session_id] = s.agent; });
  var snap = {
    now: nowMs,
    sysmon_history: [], process_groups_now: [], process_groups_history: [],
    vendor_strip: ` + vendorStripJS + `,
    activity_heatmap: {rows: []},
    todo: {enabled:false}, todo_tasks: {items: []}, headline_today: 0,
    burn: {
      generated_at: iso(nowMs), running_window_seconds: 1800,
      sessions: sessions, window_start: iso(nowMs-1800000), bucket_seconds: 60,
      chart: [{at: iso(nowMs), by_session: bySession, cost: 0}],
      turns: [], chart_agents: chartAgents
    }
  };
  window.__bdevPaintFake(snap); // still in fake mode from tick 1

  var out = { chart:{}, card:{}, vendorBase:{}, legend:[], legendDot:{} };
  sessions.forEach(function(s){
    var seg = document.querySelector('#burnBars .bar i[data-session="' + s.session_id + '"]');
    out.chart[s.session_id] = seg ? getComputedStyle(seg).backgroundColor : null;
    var fill = document.querySelector('#sessionCards .card[data-session="' + s.session_id + '"] .ctxbar i');
    out.card[s.session_id] = fill ? getComputedStyle(fill).backgroundColor : null;
    var dot = document.querySelector('#burnLegend .legenditem[data-session="' + s.session_id + '"] .dot');
    out.legendDot[s.session_id] = dot ? getComputedStyle(dot).backgroundColor : null;
  });
  ['cowork', 'claude-code', 'copilot-cli'].forEach(function(agent){
    var dot = document.querySelector('#vendorStrip tr[data-agent="' + agent + '"] .dot');
    out.vendorBase[agent] = dot ? getComputedStyle(dot).backgroundColor : null;
  });
  document.querySelectorAll('#burnLegend .legenditem').forEach(function(el){ out.legend.push(el.textContent); });
  return out;
})()`

	var out struct {
		Chart      map[string]string `json:"chart"`
		Card       map[string]string `json:"card"`
		VendorBase map[string]string `json:"vendorBase"`
		Legend     []string          `json:"legend"`
		LegendDot  map[string]string `json:"legendDot"`
	}
	if err := evalInto(script2, &out); err != nil {
		return fmt.Errorf("d15: eval fake-session paint (tick 2): %w", err)
	}
	// __bdevPaintFake updates the DOM synchronously, but WebView2's own
	// compositor can lag a frame or two behind before BitBlt's screen
	// capture actually sees it (found this session: without this wait,
	// the screenshot still showed the previous real-data frame even
	// though evalInto's own DOM reads above already reflected the fake
	// one) - give it a moment to actually paint before capturing.
	time.Sleep(300 * time.Millisecond)
	if _, err := screenshot(hwnd, "d15-vendor-colours"); err != nil {
		return err
	}

	vendorOf := map[string]string{
		"fake-cw-1": "cowork", "fake-cw-2": "cowork",
		"fake-cc-1": "claude-code", "fake-cc-2": "claude-code",
		"fake-cli-1": "copilot-cli", lateSid: "cowork",
	}
	var errs []string

	// 0. lateSid's regression guard: tick 2's colour must actually differ
	// from tick 1's grey-while-unknown guess - proves the vendor arriving
	// triggered a real recolour, not that the old value simply happened
	// to already look right.
	if out.Chart[lateSid] != "" && out.Chart[lateSid] == early.EarlyLateChart {
		errs = append(errs, fmt.Sprintf("%s: chart colour unchanged after its vendor arrived (%s), want a recolour to its Cowork shade", lateSid, early.EarlyLateChart))
	}
	if out.LegendDot[lateSid] == "" {
		errs = append(errs, fmt.Sprintf("%s: missing legend dot colour on tick 2", lateSid))
	} else if out.Chart[lateSid] != out.LegendDot[lateSid] {
		errs = append(errs, fmt.Sprintf("%s: chart colour %s != legend dot colour %s", lateSid, out.Chart[lateSid], out.LegendDot[lateSid]))
	}

	// 1. chart colour == card colour == legend dot colour, for every
	// open session (fake-late-1 included, now that its vendor is known).
	for sid := range vendorOf {
		if out.Chart[sid] == "" || out.Card[sid] == "" || out.LegendDot[sid] == "" {
			errs = append(errs, fmt.Sprintf("%s: missing chart (%q), card (%q) or legend dot (%q) colour", sid, out.Chart[sid], out.Card[sid], out.LegendDot[sid]))
			continue
		}
		if out.Chart[sid] != out.Card[sid] {
			errs = append(errs, fmt.Sprintf("%s: chart colour %s != card colour %s", sid, out.Chart[sid], out.Card[sid]))
		}
		if out.Chart[sid] != out.LegendDot[sid] {
			errs = append(errs, fmt.Sprintf("%s: chart colour %s != legend dot colour %s", sid, out.Chart[sid], out.LegendDot[sid]))
		}
	}

	// 2. every open session's hue AND saturation sit within tolerance of
	// its own vendor's live vendor-strip dot, and its hue stays clear of
	// the other vendors present in this run. Saturation is checked
	// alongside hue (not hue alone) because HARNESS_HUE.other (the old
	// grey-cache bug's colour) happens to have a hue only ~1.6deg from
	// Cowork's own - a hue-only check would have missed a regression
	// back to that bug for a Cowork session specifically; grey's
	// saturation (~0.16) sits far below any real vendor base (>=0.65),
	// so the pair together is what actually rules the bug out.
	vendorHue := map[string]float64{}
	vendorSat := map[string]float64{}
	for agent, rgb := range out.VendorBase {
		h, s, _, err := parseRGBHSL(rgb)
		if err != nil {
			errs = append(errs, fmt.Sprintf("vendor strip %s: %v", agent, err))
			continue
		}
		vendorHue[agent] = h
		vendorSat[agent] = s
	}
	const hueTolerance = 6.0   // "within a few degrees" of its own vendor
	const hueSeparation = 20.0 // clear of any other vendor present
	const satTolerance = 0.20  // well inside a real vendor hue's own saturation, well outside HARNESS_HUE.other's (~0.16 vs >=0.65)
	sessionHue := map[string]float64{}
	for sid, agent := range vendorOf {
		rgb := out.Chart[sid]
		if rgb == "" {
			continue
		}
		h, s, _, err := parseRGBHSL(rgb)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", sid, err))
			continue
		}
		sessionHue[sid] = h
		own, ok := vendorHue[agent]
		if !ok {
			continue
		}
		if d := hueDelta(h, own); d > hueTolerance {
			errs = append(errs, fmt.Sprintf("%s (%s): hue %.1f is %.1f deg from its own vendor's hue %.1f, want <=%.1f", sid, agent, h, d, own, hueTolerance))
		}
		if ownSat, ok := vendorSat[agent]; ok {
			if d := math.Abs(s - ownSat); d > satTolerance {
				errs = append(errs, fmt.Sprintf("%s (%s): saturation %.2f is %.2f from its own vendor's saturation %.2f, want <=%.2f (a session stuck on HARNESS_HUE.other's grey would fail here even when its hue looks close)", sid, agent, s, d, ownSat, satTolerance))
			}
		}
		for otherAgent, otherHue := range vendorHue {
			if otherAgent == agent {
				continue
			}
			if d := hueDelta(h, otherHue); d < hueSeparation {
				errs = append(errs, fmt.Sprintf("%s (%s): hue %.1f is only %.1f deg from %s's hue %.1f, want >=%.1f", sid, agent, h, d, otherAgent, otherHue, hueSeparation))
			}
		}
	}

	// 3. the two shades within each vendor differ by a real lightness
	// step (report it), same hue.
	pairs := [][2]string{{"fake-cw-1", "fake-cw-2"}, {"fake-cc-1", "fake-cc-2"}}
	for _, pair := range pairs {
		h1, _, l1, err1 := parseRGBHSL(out.Chart[pair[0]])
		h2, _, l2, err2 := parseRGBHSL(out.Chart[pair[1]])
		if err1 != nil || err2 != nil {
			errs = append(errs, fmt.Sprintf("%s/%s: could not parse for shade check", pair[0], pair[1]))
			continue
		}
		step := math.Abs(l1 - l2)
		fmt.Printf("uicheck: d15: %s/%s lightness step %.1f pp (L %.2f vs %.2f), hue delta %.1f deg\n", pair[0], pair[1], step*100, l1, l2, hueDelta(h1, h2))
		if step < 0.10 {
			errs = append(errs, fmt.Sprintf("%s/%s: lightness step only %.1f pp, want a clearly visible step (>=10pp)", pair[0], pair[1], step*100))
		}
	}

	// 4. item 4: the two same-project Cowork legend entries must not
	// render identical text.
	seen := map[string]int{}
	for _, l := range out.Legend {
		seen[l]++
	}
	for l, n := range seen {
		if n > 1 {
			errs = append(errs, fmt.Sprintf("legend: %d entries render identical text %q, want a session id appended on collision", n, l))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("d15: %d problem(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}
	fmt.Println("uicheck: d15: vendor colour families check clean (chart==card==legend dot, hue+saturation held per vendor, shades separated, legend collision resolved, late-vendor recolour proven)")
	return nil
}

// parseRGBHSL parses a CSS "rgb(r, g, b)" / "rgba(r, g, b, a)" string (what
// getComputedStyle returns for a colour this page ever sets) into hue
// (degrees), saturation and lightness (both 0..1).
func parseRGBHSL(s string) (h, sat, l float64, err error) {
	s = strings.TrimSpace(s)
	open := strings.Index(s, "(")
	closeIdx := strings.Index(s, ")")
	if !strings.HasPrefix(s, "rgb") || open < 0 || closeIdx < 0 || closeIdx < open {
		return 0, 0, 0, fmt.Errorf("not an rgb() colour: %q", s)
	}
	parts := strings.Split(s[open+1:closeIdx], ",")
	if len(parts) < 3 {
		return 0, 0, 0, fmt.Errorf("not enough components in %q", s)
	}
	var rgb [3]float64
	for i := 0; i < 3; i++ {
		v, perr := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64)
		if perr != nil {
			return 0, 0, 0, fmt.Errorf("bad component %q in %q: %w", parts[i], s, perr)
		}
		rgb[i] = v / 255
	}
	r, g, b := rgb[0], rgb[1], rgb[2]
	max := math.Max(r, math.Max(g, b))
	min := math.Min(r, math.Min(g, b))
	l = (max + min) / 2
	if max == min {
		return 0, 0, l, nil // grey: hue undefined
	}
	d := max - min
	if l > 0.5 {
		sat = d / (2 - max - min)
	} else {
		sat = d / (max + min)
	}
	switch max {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, sat, l, nil
}

// hueDelta is the shortest distance between two hues on the 360-degree
// wheel (e.g. 350 and 5 are 15 apart, not 345).
func hueDelta(a, b float64) float64 {
	d := math.Abs(a - b)
	if d > 180 {
		d = 360 - d
	}
	return d
}

// charts/d16: moved from check_d16.go.

// d16: WS2 follow-up (2026-09-28), System chart on a real time axis - a
// uicheck case that drives paintTick with a synthetic sysmon_history (10
// minutes of 1s samples inside a 30-minute window, with one deliberate
// 60-second gap in the middle) and a matching synthetic burn.chart/
// window_start covering the same window, via page.html's own
// __bdevEnterFakeMode/__bdevPaintFake hooks, then proves: the data's first
// point lands at the pixel ratio the window math predicts (10 minutes of
// data inside a 30-minute window, ending at now, leaves the newest 1/3 of
// the width filled - see the ratio math below, which the report also flags
// against the roadmap doc's own "about one third from the left" wording);
// the 60s gap produces two separate line segments, not one straight line
// across it; and #histAxis's own labels are the same 6 labels as #burnAxis
// for the same now (found by review, 2026-09-28: the first version anchored
// #histAxis to a locally-recomputed "now minus 30 minutes" instead of
// reading burn.window_start, which internal\live\live.go rounds up to the
// next whole bucket - the two axes' real tick instants did not actually
// match on real data even though this fixture's own exact window_start
// happened to hide it; renderHistoryChart now takes window_start directly).
func caseD16(hwnd uintptr, args []string) error {
	time.Sleep(300 * time.Millisecond)

	script := `(function(){
  var nowMs = Date.now();
  var windowMs = 30 * 60 * 1000;
  var windowStart = nowMs - windowMs;
  var samples = [];
  function addRange(startOffsetSec, endOffsetSec, stepSec){
    for(var s = startOffsetSec; s >= endOffsetSec; s -= stepSec){
      var t = nowMs - s * 1000;
      samples.push({
        Ts: new Date(t).toISOString(),
        CPUPct: 40 + 10 * Math.sin(s / 30), MemUsedMB: 8000, MemTotalMB: 16000,
        DiskReadBps: 0, DiskWriteBps: 0, NetDownBps: 0, NetUpBps: 0, GPUPct: 0
      });
    }
  }
  addRange(600, 360, 1); // -600s..-360s, continuous 1s samples (10 minutes of data starts here)
  addRange(300, 0, 1);   // -300s..0s, continuous 1s samples - 60s gap against the range above

  var chart = [];
  for(var i = 0; i < 30; i++){
    chart.push({ at: new Date(windowStart + i * 60000).toISOString(), by_session: {}, cost: 0 });
  }

  var snap = {
    now: nowMs,
    sysmon_history: samples,
    process_groups_now: [], process_groups_history: [],
    vendor_strip: { rows: [] }, activity_heatmap: { rows: [] },
    todo: { enabled: false }, todo_tasks: { items: [] }, headline_today: 0,
    burn: {
      generated_at: new Date(nowMs).toISOString(), running_window_seconds: 1800,
      sessions: [], window_start: new Date(windowStart).toISOString(), bucket_seconds: 60,
      chart: chart, turns: []
    }
  };
  window.__bdevEnterFakeMode();
  window.__bdevPaintFake(snap);

  var histLabels = [];
  document.querySelectorAll('#histAxis .axistick').forEach(function(el){ if(el.textContent) histLabels.push(el.textContent); });
  var burnLabels = [];
  document.querySelectorAll('#burnAxis .axistick').forEach(function(el){ if(el.textContent) burnLabels.push(el.textContent); });

  return { debug: window.__bdevHistDebug, histLabels: histLabels, burnLabels: burnLabels };
})()`

	var out struct {
		Debug struct {
			W        float64 `json:"w"`
			Left     float64 `json:"left"`
			Right    float64 `json:"right"`
			Segments [][]struct {
				X float64 `json:"x"`
				T float64 `json:"t"`
			} `json:"segments"`
		} `json:"debug"`
		HistLabels []string `json:"histLabels"`
		BurnLabels []string `json:"burnLabels"`
	}
	// Deferred unconditionally, not after the eval/screenshot calls
	// below: a JS exception partway through script (after
	// __bdevEnterFakeMode already ran) would otherwise return early and
	// leave the real page's own tick timer stopped forever. The guard
	// inside is a no-op if fake mode was never actually entered.

	if err := evalInto(script, &out); err != nil {
		return fmt.Errorf("d16: eval fake-history paint: %w", err)
	}
	// __bdevPaintFake updates the DOM synchronously, but WebView2's own
	// compositor can lag a frame or two behind before BitBlt's screen
	// capture actually sees it (found this session: without this wait,
	// the screenshot still showed the previous real-data frame even
	// though evalInto's own DOM reads above already reflected the fake
	// one) - give it a moment to actually paint before capturing.
	time.Sleep(300 * time.Millisecond)
	if _, err := screenshot(hwnd, "d16-system-time-axis"); err != nil {
		return err
	}

	var errs []string

	// 1. Two segments (the 60s gap produced a break), not one straight line.
	if len(out.Debug.Segments) != 2 {
		errs = append(errs, fmt.Sprintf("expected 2 line segments (one break at the 60s gap), got %d", len(out.Debug.Segments)))
	} else {
		gapEnd := out.Debug.Segments[1][0].T
		gapStart := out.Debug.Segments[0][len(out.Debug.Segments[0])-1].T
		gapSec := (gapEnd - gapStart) / 1000
		fmt.Printf("uicheck: d16: gap between segments = %.0fs (want ~60s, >25s HIST_GAP_MS threshold as of v0.4.0-alpha.5)\n", gapSec)
		if gapSec < 55 || gapSec > 65 {
			errs = append(errs, fmt.Sprintf("segment gap was %.0fs, want ~60s", gapSec))
		}
	}

	// 2. First point's pixel ratio. 10 minutes of data ends at now
	// inside a 30-minute window: the data's own span (10 min) is 1/3 of
	// the window (30 min), and it is the MOST RECENT 1/3 (data runs up
	// to "now", not away from it) - so the first (oldest, leftmost)
	// point sits 2/3 of the way across, at the boundary where the
	// newest third begins, not 1/3 from the left. (2026-09-28_ws2
	// _system_chart_time_axis.md's own proof section says "about one
	// third of the width from the left" for this same case; this
	// reports the actual measured ratio for Wilco to compare against
	// that wording rather than forcing a match.)
	if len(out.Debug.Segments) > 0 && len(out.Debug.Segments[0]) > 0 {
		firstX := out.Debug.Segments[0][0].X
		ratio := firstX / out.Debug.W
		expected := 2.0 / 3.0
		fmt.Printf("uicheck: d16: first point at x=%.1f of w=%.1f -> ratio %.3f (expected ~%.3f = (30-10)/30, the newest third of the window is filled)\n", firstX, out.Debug.W, ratio, expected)
		if math.Abs(ratio-expected) > 0.03 {
			errs = append(errs, fmt.Sprintf("first point ratio %.3f, want ~%.3f (+/-0.03)", ratio, expected))
		}
	} else {
		errs = append(errs, "no first segment/point to measure a pixel ratio from")
	}

	// 3. histAxis and burnAxis render the exact same set of labels for
	// the same now: both stop one tick short of the window's own right
	// edge (renderHistoryChart no longer places a tick that would only
	// ever render inside .histaxis's own clipped, invisible edge; see
	// its own comment), so there is no longer an "extra" tick on either
	// side to explain away - a real mismatch here means the two axes
	// do not actually share one anchor.
	if len(out.BurnLabels) == 0 {
		errs = append(errs, "burnAxis rendered no labels to compare against")
	}
	histSet := map[string]bool{}
	for _, l := range out.HistLabels {
		histSet[l] = true
	}
	burnSet := map[string]bool{}
	for _, l := range out.BurnLabels {
		burnSet[l] = true
	}
	var missing, extra []string
	for _, l := range out.BurnLabels {
		if !histSet[l] {
			missing = append(missing, l)
		}
	}
	for _, l := range out.HistLabels {
		if !burnSet[l] {
			extra = append(extra, l)
		}
	}
	if len(missing) > 0 {
		errs = append(errs, fmt.Sprintf("burnAxis label(s) %v not found among histAxis labels %v", missing, out.HistLabels))
	}
	if len(extra) > 0 {
		errs = append(errs, fmt.Sprintf("histAxis label(s) %v not found among burnAxis labels %v", extra, out.BurnLabels))
	}
	fmt.Printf("uicheck: d16: histAxis labels %v, burnAxis labels %v\n", out.HistLabels, out.BurnLabels)

	if len(errs) > 0 {
		return fmt.Errorf("d16: %d problem(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}
	fmt.Println("uicheck: d16: system time axis check clean (gap breaks the line, pixel ratio matches the window math, axis labels align with the burn chart)")
	return nil
}

// charts/d17: moved from check_d17.go.

// d17: v0.4.0-alpha.5, the gap-break threshold fix. First reads
// window.__bdevHistGapMs off the real, already-running page (before
// entering fake mode) - page.html's own live mirror of the HIST_GAP_MS its
// last real paintTick used, set from the real bdevSnapshotNow's own
// hist_gap_ms field - to prove app.go's histGapThreshold (2.5x
// hiddenSampleInterval, 25000ms today) actually reaches the page end to
// end, not just that fake mode's own synthetic snap.hist_gap_ms round-trips
// (a fresh review, 2026-09-28, flagged the first version of this check for
// only proving the latter). Then drives paintTick with synthetic
// sysmon_history/process_groups_history via page.html's own
// __bdevEnterFakeMode/__bdevPaintFake hooks (never live data) to prove the
// old hard-coded 5s is actually gone from both render paths.
//
// System chart (sysmonHistory, 30-minute window): 5 minutes of 1s samples,
// then 5 minutes of 10s samples (the ordinary hidden/minimized cadence,
// app.go's own hiddenSampleInterval), then a real 60s gap, then 1s samples
// again. Under the old 5000ms threshold every 10s step in the middle stretch
// would itself have broken the line (10s > 5s); under the fix (25000ms) only
// the deliberate 60s gap does. Checked via the existing __bdevHistDebug hook
// (renderHistoryChart's own first-series segment list, same one d16 uses).
//
// Process-groups sparkline (30-second window, PROCESS_GROUP_SPARK_WINDOW_MS):
// a 60s gap cannot itself appear inside a 30s-wide window (both of its
// endpoints would have to be at most 30s apart to both be visible at once),
// so this case cannot replay the System chart's exact numbers here - it
// instead pins the precise 25000ms boundary from the same shared HIST_GAP_MS
// value: a 24s gap must not break the line, a 26s gap must, in two separate
// __bdevPaintFake ticks. Read from the trend <path>'s own "d" attribute
// (counting "M" moveto commands - one per subpath) rather than a debug hook,
// since the sparkline has none.
func caseD17(hwnd uintptr, args []string) error {
	time.Sleep(300 * time.Millisecond)

	script := `(function(){
  // Read off the real, already-running page before touching fake mode at
  // all: the last real paintTick's own HIST_GAP_MS, set from the real
  // bdevSnapshotNow's own hist_gap_ms field (main.go/app.go), not a
  // synthetic one this script is about to supply.
  var realHistGapMs = window.__bdevHistGapMs;

  var nowMs = Date.now();
  var windowMs = 30 * 60 * 1000;
  var windowStart = nowMs - windowMs;
  var HIST_GAP_MS = 25000; // app.go's histGapThreshold today (2.5 * hiddenSampleInterval, 10s)

  // --- System chart: 5min@1s, 5min@10s, 60s gap, 1s again ---
  var samples = [];
  function addRange(startOffsetSec, endOffsetSec, stepSec){
    for(var s = startOffsetSec; s >= endOffsetSec; s -= stepSec){
      var t = nowMs - s * 1000;
      samples.push({
        Ts: new Date(t).toISOString(),
        CPUPct: 40 + 10 * Math.sin(s / 30), MemUsedMB: 8000, MemTotalMB: 16000,
        DiskReadBps: 0, DiskWriteBps: 0, NetDownBps: 0, NetUpBps: 0, GPUPct: 0
      });
    }
  }
  addRange(720, 421, 1);  // -720s..-421s, 300 samples at 1s (5 minutes)
  addRange(420, 120, 10); // -420s..-120s, 31 samples at 10s (5 minutes) - the hidden-window cadence
  // 60s gap: last sample above at -120s, first sample below at -60s
  addRange(60, 0, 1);     // -60s..0s, 61 samples at 1s again

  var chart = [];
  for(var i = 0; i < 30; i++){
    chart.push({ at: new Date(windowStart + i * 60000).toISOString(), by_session: {}, cost: 0 });
  }
  function baseSnap(now, sysmonHistory, groupsNow, groupsHistory){
    return {
      now: now,
      hist_gap_ms: HIST_GAP_MS,
      sysmon_history: sysmonHistory || [],
      process_groups_now: groupsNow || [], process_groups_history: groupsHistory || [],
      vendor_strip: { rows: [] }, activity_heatmap: { rows: [] },
      todo: { enabled: false }, todo_tasks: { items: [] }, headline_today: 0,
      burn: {
        generated_at: new Date(now).toISOString(), running_window_seconds: 1800,
        sessions: [], window_start: new Date(now - windowMs).toISOString(), bucket_seconds: 60,
        chart: chart, turns: []
      }
    };
  }

  window.__bdevEnterFakeMode();

  // --- Process-groups sparkline: pin the exact 25000ms boundary, one tick
  // each way (a 60s gap cannot fit both its endpoints inside the 30s window
  // at once, so this pins the boundary instead of replaying 60s). Done
  // before the System chart tick below, so the final screenshot (taken
  // after this whole script returns) shows the System chart's own proof,
  // not an intermediate sparkline-only frame with no system history.
  var groupsNow = [{ Harness: 'claude', CPUPct: 30, MemMB: 500, IOBps: 1000 }];
  function trendD(){
    var el = document.querySelector('#processGroups .trendsvg path');
    return el ? el.getAttribute('d') : null;
  }
  function countMoves(d){
    return d ? (d.match(/M/g) || []).length : 0;
  }

  window.__bdevPaintFake(baseSnap(nowMs, [], groupsNow, [
    { Ts: new Date(nowMs - 24000).toISOString(), Harness: 'claude', CPUPct: 20, MemMB: 500, IOBps: 1000 },
    { Ts: new Date(nowMs).toISOString(), Harness: 'claude', CPUPct: 25, MemMB: 500, IOBps: 1000 }
  ]));
  var dNoBreak = trendD();

  // Final tick: System chart's own 60s-gap fixture together with the
  // sparkline's own 26s-gap (broken) case, so one screenshot shows both
  // proofs from the state actually captured.
  var breakGroupsHistory = [
    { Ts: new Date(nowMs - 26000).toISOString(), Harness: 'claude', CPUPct: 20, MemMB: 500, IOBps: 1000 },
    { Ts: new Date(nowMs).toISOString(), Harness: 'claude', CPUPct: 25, MemMB: 500, IOBps: 1000 }
  ];
  window.__bdevPaintFake(baseSnap(nowMs, samples, groupsNow, breakGroupsHistory));
  var histDebug = window.__bdevHistDebug;
  var dBreak = trendD();

  return {
    realHistGapMs: realHistGapMs,
    debug: histDebug,
    sparkNoBreakD: dNoBreak, sparkNoBreakMoves: countMoves(dNoBreak),
    sparkBreakD: dBreak, sparkBreakMoves: countMoves(dBreak)
  };
})()`

	var out struct {
		RealHistGapMs float64 `json:"realHistGapMs"`
		Debug         struct {
			W        float64 `json:"w"`
			Segments [][]struct {
				X float64 `json:"x"`
				T float64 `json:"t"`
			} `json:"segments"`
		} `json:"debug"`
		SparkNoBreakD     string `json:"sparkNoBreakD"`
		SparkNoBreakMoves int    `json:"sparkNoBreakMoves"`
		SparkBreakD       string `json:"sparkBreakD"`
		SparkBreakMoves   int    `json:"sparkBreakMoves"`
	}
	// Deferred unconditionally, not after the eval/screenshot calls
	// below: a JS exception partway through script (after
	// __bdevEnterFakeMode already ran) would otherwise return early and
	// leave the real page's own tick timer stopped forever. The guard
	// inside is a no-op if fake mode was never actually entered.

	if err := evalInto(script, &out); err != nil {
		return fmt.Errorf("d17: eval fake-history paint: %w", err)
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := screenshot(hwnd, "d17-hist-gap-threshold"); err != nil {
		return err
	}

	var errs []string

	// 0. The real, already-running page's own last live tick used a
	// HIST_GAP_MS of exactly 25000 - app.go's histGapThreshold
	// (hiddenSampleInterval * 5 / 2, 10000 * 5 / 2) reached the page
	// through the real bdevSnapshotNow, not just this check's own fake
	// snap.hist_gap_ms. If app.go's constants ever change, this
	// expected value needs updating alongside it.
	const wantRealHistGapMs = 25000
	fmt.Printf("uicheck: d17: real page's own live HIST_GAP_MS = %.0f (want %d)\n", out.RealHistGapMs, wantRealHistGapMs)
	if out.RealHistGapMs != wantRealHistGapMs {
		errs = append(errs, fmt.Sprintf("real page's own live HIST_GAP_MS was %.0f, want %d (app.go's histGapThreshold did not reach the page through bdevSnapshotNow)", out.RealHistGapMs, wantRealHistGapMs))
	}

	// 1. System chart: exactly 2 segments - the 5min@1s and 5min@10s
	// stretches stay one continuous line (the bug this item fixes: the
	// old 5000ms threshold would have broken every 10s step in the
	// second stretch, producing far more than 2 segments here), and
	// only the deliberate 60s gap breaks it.
	if len(out.Debug.Segments) != 2 {
		errs = append(errs, fmt.Sprintf("system chart: expected 2 line segments (continuous across 1s+10s, one break at the 60s gap), got %d", len(out.Debug.Segments)))
	} else {
		seg0, seg1 := out.Debug.Segments[0], out.Debug.Segments[1]
		if len(seg0) != 331 {
			errs = append(errs, fmt.Sprintf("system chart: first segment has %d points, want 331 (300 at 1s + 31 at 10s, unbroken)", len(seg0)))
		}
		if len(seg1) != 61 {
			errs = append(errs, fmt.Sprintf("system chart: second segment has %d points, want 61 (1 minute at 1s after the gap)", len(seg1)))
		}
		gapStart := seg0[len(seg0)-1].T
		gapEnd := seg1[0].T
		gapSec := (gapEnd - gapStart) / 1000
		fmt.Printf("uicheck: d17: system chart gap = %.0fs (want ~60s), segments = %d+%d points\n", gapSec, len(seg0), len(seg1))
		if gapSec < 55 || gapSec > 65 {
			errs = append(errs, fmt.Sprintf("system chart: segment gap was %.0fs, want ~60s", gapSec))
		}
	}

	// 2. Sparkline: a 24s gap (under the 25000ms threshold) must not
	// break the line - one subpath, one "M".
	fmt.Printf("uicheck: d17: sparkline 24s-gap path = %q (%d move(s), want 1)\n", out.SparkNoBreakD, out.SparkNoBreakMoves)
	if out.SparkNoBreakD == "" {
		errs = append(errs, "sparkline 24s-gap case: trend path rendered empty")
	} else if out.SparkNoBreakMoves != 1 {
		errs = append(errs, fmt.Sprintf("sparkline 24s-gap case: %d moveto command(s) in %q, want 1 (continuous)", out.SparkNoBreakMoves, out.SparkNoBreakD))
	}

	// 3. Sparkline: a 26s gap (over the 25000ms threshold) must break
	// the line - two subpaths, two "M"s.
	fmt.Printf("uicheck: d17: sparkline 26s-gap path = %q (%d move(s), want 2)\n", out.SparkBreakD, out.SparkBreakMoves)
	if out.SparkBreakD == "" {
		errs = append(errs, "sparkline 26s-gap case: trend path rendered empty")
	} else if out.SparkBreakMoves != 2 {
		errs = append(errs, fmt.Sprintf("sparkline 26s-gap case: %d moveto command(s) in %q, want 2 (broken once)", out.SparkBreakMoves, out.SparkBreakD))
	}

	if len(errs) > 0 {
		return fmt.Errorf("d17: %d problem(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}
	fmt.Println("uicheck: d17: hist-gap threshold check clean (real page's own HIST_GAP_MS reached 25000 through bdevSnapshotNow; system chart continuous across 1s+10s cadence, breaks once at 60s; sparkline holds at 24s, breaks at 26s)")
	return nil
}

// charts/d18: moved from check_d18.go.

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

// charts/d20: moved from check_d20.go.

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

// charts/d21: moved from check_d21.go.

// d21: WS2 alpha.7, never skip forever (02_roadmap\2026-09-29_ws2_smooth
// _tick_and_system_layout.md item 1.3, plus the lost-answer freeze the
// 90-minute after run found on 2026-09-29): one bdevSnapshotNow call is made
// to never answer, the way a resolve lost in go-webview2's dispatch queue
// never answers, and the page must give up on it and paint again within
// IN_FLIGHT_GIVE_UP ticks plus one, instead of waiting on it forever. Real
// ticks, not fake mode: the wrapper only swallows the first call it sees,
// every later call goes to the real binding.
func caseD21(hwnd uintptr, args []string) error {
	setup := `(function(){
  if(!window.__bdevRealSnapshotNow) window.__bdevRealSnapshotNow = window.bdevSnapshotNow;
  var swallowed = false;
  window.__bdevSwallowAt = 0;
  window.bdevSnapshotNow = function(c){
    if(!swallowed){ swallowed = true; window.__bdevSwallowAt = performance.now(); return new Promise(function(){}); }
    return window.__bdevRealSnapshotNow(c);
  };
  return true;
})()`
	if _, err := evalRaw(setup); err != nil {
		return fmt.Errorf("d21: install wrapper: %w", err)
	}
	defer evalRaw(`(function(){ if(window.__bdevRealSnapshotNow) window.bdevSnapshotNow = window.__bdevRealSnapshotNow; })()`)
	time.Sleep(6 * time.Second)

	var out struct {
		SwallowAt  float64 `json:"swallowAt"`
		FirstAfter float64 `json:"firstAfter"`
		Refresh    float64 `json:"refresh"`
		Count      int     `json:"count"`
	}
	read := `(function(){
  var at = window.__bdevSwallowAt, first = 0, n = 0;
  (window.__bdevPaintLog || []).forEach(function(e){
    if(e.name === 'tickEnd' && e.t > at){ n++; if(!first) first = e.t; }
  });
  return { swallowAt: at, firstAfter: first, refresh: window.__bdevRefreshMs || 1000, count: n };
})()`
	if err := evalInto(read, &out); err != nil {
		return fmt.Errorf("d21: read paint log: %w", err)
	}
	if out.SwallowAt == 0 {
		return fmt.Errorf("d21: the wrapper never saw a bdevSnapshotNow call in 6s (is the tick running?)")
	}
	if out.FirstAfter == 0 {
		return fmt.Errorf("d21: no paint at all in the 6s after one unanswered call: the page still waits on it forever")
	}
	gap := out.FirstAfter - out.SwallowAt
	fmt.Printf("uicheck: d21: first paint %.0fms after the unanswered call (tick %.0fms), %d paints in the rest of the 6s\n", gap, out.Refresh, out.Count)
	// Give-up after 2 ticks, fresh call on the 3rd, paint in its frame.
	if gap > out.Refresh*3+500 {
		return fmt.Errorf("d21: first paint came %.0fms after the unanswered call, want under %.0fms", gap, out.Refresh*3+500)
	}
	fmt.Println("uicheck: d21: an unanswered snapshot call no longer freezes the page")
	return nil
}

// charts/d22: moved from check_d22.go.

// d22: WS2 alpha.7 item 5 (02_roadmap\2026-09-29_ws2_smooth_tick_and_system
// _layout.md): Memory, Disks and Network on one row grid. Fake mode, two
// drives and a Wi-Fi reading, then: row N's top is the same in every box
// that has a row N and every row sits on one 18px pitch, the three boxes are the same
// height, Network shows three bars (down and up scaled to their own peak
// over the chart window, signal = its percent), and without Wi-Fi the
// signal line shows no bar. Row order follows Wilco's mock-up
// (04_assets\reference\2026-09-29_smooth_tick\2026-09-29_system_boxes
// _mockup.png): Network's "up" sits level with the second drive's value
// line. 1920x1080: a wide single row of boxes, above the 900px stacking
// breakpoint.
func caseD22(hwnd uintptr, args []string) error {
	time.Sleep(300 * time.Millisecond)
	errs := []string{}
	for _, wifi := range []bool{true, false} {
		script := fmt.Sprintf(`(function(wifiOn){
  var nowMs = Date.now(), windowMs = 30 * 60 * 1000, ws = nowMs - windowMs, sys = [], chart = [];
  for(var t = ws; t <= nowMs; t += 1000){
    var peakHere = (nowMs - t) === 600000;
    sys.push({ Ts: new Date(t).toISOString(), CPUPct: 30, MemUsedMB: 27750, MemTotalMB: 32358,
      NetDownBps: peakHere ? 40000 : 5000, NetUpBps: peakHere ? 100000 : 10000, DiskReadBps: 0, DiskWriteBps: 0, GPUPct: 0 });
  }
  for(var i = 0; i < 30; i++) chart.push({ at: new Date(ws + i * 60000).toISOString(), by_session: {}, cost: 0 });
  var sm = { Ts: new Date(nowMs).toISOString(), pressure_score: 20, CPUPct: 30, Cores: [30, 30, 30, 30], MemUsedMB: 27750, MemTotalMB: 32358,
    SwapUsedMB: 5734, SwapTotalMB: 46387, NetDownBps: 9523, NetUpBps: 28672,
    Disks: [{ Mount: 'C:', System: true, FreeGB: 137, TotalGB: 953, UsedPercent: 86, ReadBps: 1.5e6, WriteBps: 4.8e6 },
            { Mount: 'E:', System: false, FreeGB: 475, TotalGB: 1397, UsedPercent: 66, ReadBps: 0, WriteBps: 0 }],
    Wifi: wifiOn ? { OK: true, Connected: true, SSID: 'ExampleNet', SignalPct: 89 } : { OK: true, Connected: false } };
  var snap = { now: nowMs, hist_gap_ms: 25000, sysmon: sm, sysmon_history: sys, process_groups_now: [], process_groups_history: [],
    vendor_strip: { rows: [] }, activity_heatmap: { rows: [] }, todo: { enabled: false }, todo_tasks: { items: [] }, headline_today: 0,
    burn: { generated_at: new Date(nowMs).toISOString(), running_window_seconds: 1800, sessions: [],
      window_start: new Date(ws).toISOString(), bucket_seconds: 60, chart: chart, turns: [] } };
  window.__bdevEnterFakeMode();
  window.__bdevPaintFake(snap);
  function rows(id){
    var box = document.getElementById(id), grid = box.querySelector('.sysrows'), out = [];
    Array.prototype.forEach.call(grid.children, function(el){
      var r = el.getBoundingClientRect();
      var bar = el.classList.contains('sysbar') ? el.querySelector('i') : null;
      out.push({ top: r.top, h: r.height, bar: !!bar, pct: bar ? bar.getBoundingClientRect().width / r.width * 100 : -1, text: bar ? '' : el.textContent });
    });
    return { rows: out, boxH: box.getBoundingClientRect().height, gridTop: grid.getBoundingClientRect().top };
  }
  return { mem: rows('boxMemory'), disks: rows('boxDisks'), net: rows('boxNetwork') };
})(%v)`, wifi)
		type row struct {
			Top  float64 `json:"top"`
			H    float64 `json:"h"`
			Bar  bool    `json:"bar"`
			Pct  float64 `json:"pct"`
			Text string  `json:"text"`
		}
		type box struct {
			Rows    []row   `json:"rows"`
			BoxH    float64 `json:"boxH"`
			GridTop float64 `json:"gridTop"`
		}
		var out struct {
			Mem   box `json:"mem"`
			Disks box `json:"disks"`
			Net   box `json:"net"`
		}
		if err := evalInto(script, &out); err != nil {
			return fmt.Errorf("d22 wifi=%v: eval: %w", wifi, err)
		}
		time.Sleep(300 * time.Millisecond)
		name := "d22-system-boxes"
		if !wifi {
			name = "d22-system-boxes-no-wifi"
		}
		if _, err := screenshot(hwnd, name); err != nil {
			return err
		}
		boxes := map[string]box{"memory": out.Mem, "disks": out.Disks, "network": out.Net}
		for n := 0; n < 7; n++ {
			var tops []string
			ref := math.NaN()
			for _, k := range []string{"memory", "disks", "network"} {
				b := boxes[k]
				if n >= len(b.Rows) {
					continue
				}
				// Row centre, not top: .sysrows centres a 10px bar and an
				// empty detail row in the same 16px track as a text line.
				mid := b.Rows[n].Top + b.Rows[n].H/2 - b.GridTop
				tops = append(tops, fmt.Sprintf("%s=%.1f", k, mid))
				if math.IsNaN(ref) {
					ref = mid
				} else if math.Abs(mid-ref) > 1 {
					errs = append(errs, fmt.Sprintf("wifi=%v row %d not aligned: %v", wifi, n, tops))
				}
				// One pitch for every row: 16px track plus the 2px gap.
				if want := 8 + 18*float64(n); math.Abs(mid-want) > 1 {
					errs = append(errs, fmt.Sprintf("wifi=%v %s row %d centre at %.1fpx, want %.0f (18px pitch)", wifi, k, n, mid, want))
				}
			}
			fmt.Printf("uicheck: d22 wifi=%v row %d centres: %s\n", wifi, n, strings.Join(tops, " "))
		}
		if math.Abs(out.Mem.BoxH-out.Disks.BoxH) > 1 || math.Abs(out.Disks.BoxH-out.Net.BoxH) > 1 {
			errs = append(errs, fmt.Sprintf("wifi=%v box heights differ: %.0f %.0f %.0f", wifi, out.Mem.BoxH, out.Disks.BoxH, out.Net.BoxH))
		}
		bars := 0
		for _, r := range out.Net.Rows {
			if r.Bar {
				bars++
			}
		}
		want := 3
		if !wifi {
			want = 2
		}
		if bars != want {
			errs = append(errs, fmt.Sprintf("wifi=%v network shows %d bars, want %d", wifi, bars, want))
		}
		if len(out.Mem.Rows) > 1 && (out.Mem.Rows[0].Bar || !out.Mem.Rows[1].Bar) {
			errs = append(errs, "memory: value line must come before its bar")
		}
		if wifi && len(out.Net.Rows) >= 7 {
			// Mock-up rows: down, bar, (empty), up, bar, SSID, signal bar.
			// down 9523 of peak 40000 = 23.8%, up 28672 of peak 100000 =
			// 28.7%, signal 89%.
			for i, w := range map[int]float64{1: 23.8, 4: 28.7, 6: 89} {
				if !out.Net.Rows[i].Bar || math.Abs(out.Net.Rows[i].Pct-w) > 1.5 {
					errs = append(errs, fmt.Sprintf("network row %d: bar=%v at %.1f%%, want a bar at %.1f%%", i, out.Net.Rows[i].Bar, out.Net.Rows[i].Pct, w))
				}
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("d22: %d problem(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}
	fmt.Println("uicheck: d22: System boxes on one row grid (rows aligned, equal heights, three network bars, text only without Wi-Fi)")
	return nil
}

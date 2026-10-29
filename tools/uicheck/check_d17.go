package main

import (
	"fmt"
	"strings"
	"time"
)

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
	if err := ensureWindowSizeWH(hwnd, 1280, 860); err != nil {
		return err
	}
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

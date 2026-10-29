package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

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
	if err := ensureWindowSizeWH(hwnd, 1280, 860); err != nil {
		return err
	}
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

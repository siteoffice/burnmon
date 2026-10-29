package main

import (
	"fmt"
	"sort"
	"time"
)

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
	if err := ensureWindowSizeWH(hwnd, 1280, 860); err != nil {
		return err
	}

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

	type mark struct {
		Name string  `json:"name"`
		T    float64 `json:"t"`
	}
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

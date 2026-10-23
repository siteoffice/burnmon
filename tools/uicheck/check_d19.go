package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// d19: WS2 alpha.7 item 3 (02_roadmap\2026-09-29_ws2_smooth_tick_and_system
// _layout.md): with the To Do panel visible, the System chart (.histwrap) is
// 70% of the height it has in the plain layout at the same window size, To
// Do fills the window to its bottom edge, and nothing overflows. With To Do
// off the chart keeps its plain height. At a window too small for To Do in
// the plain layout (1280x860 today), applyPanelFit's drop order still wins:
// To Do is fit-dropped and the chart keeps its plain height. The three To Do
// on sizes are ones that fit this laptop's primary screen: uicheck places
// the window at (100,100) there and caps it to that screen, so the portrait
// 1152x2048 size cannot be reached from here. Fake mode only, with made-up
// task titles: a uicheck run never shows real To Do rows (todo_gate.go).
// One screenshot per case.
func init() {
	checks["d19"] = func(hwnd uintptr, args []string) error {
		cases := []struct {
			w, h       int32
			todo       bool
			expectDrop bool
		}{{1600, 1000, true, false}, {1920, 1080, true, false}, {2560, 1300, true, false}, {1920, 1080, false, false}, {1280, 860, true, true}}
		var errs []string
		skipped := 0
		defer evalRaw(`(window.__bdevExitFakeMode && window.__bdevExitFakeMode())`)
		for _, c := range cases {
			if err := ensureWindowSizeWH(hwnd, c.w, c.h); err != nil {
				return err
			}
			// A window the screen could not hold was never this size, so its
			// numbers say nothing about the layout at it (a laptop-only
			// 1600x1000 screen failed this check for that reason).
			if windowCapped {
				fmt.Printf("uicheck: d19 %dx%d todo=%v: SKIPPED, this screen is too small for that window\n", c.w, c.h, c.todo)
				skipped++
				continue
			}
			time.Sleep(500 * time.Millisecond)
			script := fmt.Sprintf(`(function(todoOn){
  var nowMs = Date.now(), windowMs = 30 * 60 * 1000, ws = nowMs - windowMs;
  var sys = [], groups = [], chart = [];
  for(var t = nowMs - windowMs; t <= nowMs; t += 1000){
    var s = t / 1000;
    sys.push({ Ts: new Date(t).toISOString(), CPUPct: 35 + 25 * Math.sin(s / 90), MemUsedMB: 9000, MemTotalMB: 16000,
      DiskReadBps: 1e6, DiskWriteBps: 4e5, NetDownBps: 2e5, NetUpBps: 1e5, GPUPct: 12 });
    ['claude', 'codex'].forEach(function(h){ groups.push({ Ts: new Date(t).toISOString(), Harness: h, CPUPct: 4, MemMB: 400, IOBps: 100 }); });
  }
  for(var i = 0; i < 30; i++) chart.push({ at: new Date(ws + i * 60000).toISOString(), by_session: {}, cost: 0 });
  var items = [];
  for(var k = 1; k <= 14; k++) items.push({ Title: 'Example task ' + k, List: 'Sample list', DueDate: '2026-09-29', Overdue: k %% 5 === 0 });
  var nowIso = new Date(nowMs).toISOString();
  var snap = {
    now: nowMs, hist_gap_ms: 25000, sysmon_history: sys,
    process_groups_now: [{ Ts: nowIso, Harness: 'claude', CPUPct: 4, MemMB: 400, IOBps: 100 }, { Ts: nowIso, Harness: 'codex', CPUPct: 4, MemMB: 400, IOBps: 100 }],
    process_groups_history: groups,
    vendor_strip: { rows: [] }, activity_heatmap: { rows: [] },
    todo: todoOn ? { enabled: true, signed_in: true } : { enabled: false },
    todo_tasks: { items: todoOn ? items : [] }, headline_today: 0,
    burn: { generated_at: nowIso, running_window_seconds: 1800, sessions: [],
      window_start: new Date(ws).toISOString(), bucket_seconds: 60, chart: chart, turns: [] }
  };
  window.__bdevEnterFakeMode();
  window.__bdevPaintFake(snap);
  // Standard scrolls (R8 follow-up): the older zones, To Do included, fill the window's height below the
  // sections, so scroll to the end before measuring where To Do sits.
  document.getElementById('pageScroll').scrollTop = 1e9;
  var todo = document.getElementById('todoPanel');
  var tr = todo.getBoundingClientRect();
  var zones = document.querySelectorAll('.zonebody'), zoneOver = false;
  for(var z = 0; z < zones.length; z++) if(zones[z].scrollHeight > zones[z].clientHeight + 1) zoneOver = true;
  return {
    layout: window.__bdevTodoLayout,
    todoDropped: todo.classList.contains('fit-dropped'),
    todoBottom: tr.bottom, innerH: window.innerHeight,
    bodyOver: document.body.scrollHeight > document.body.clientHeight + 1 || document.body.scrollWidth > document.body.clientWidth + 1,
    zoneOver: zoneOver
  };
})(%v)`, c.todo)
			var out struct {
				Layout struct {
					Applied    bool    `json:"applied"`
					PlainHistH float64 `json:"plainHistH"`
					HistH      float64 `json:"histH"`
				} `json:"layout"`
				TodoDropped bool    `json:"todoDropped"`
				TodoBottom  float64 `json:"todoBottom"`
				InnerH      float64 `json:"innerH"`
				BodyOver    bool    `json:"bodyOver"`
				ZoneOver    bool    `json:"zoneOver"`
			}
			if err := evalInto(script, &out); err != nil {
				return fmt.Errorf("d19 %dx%d: eval layout: %w", c.w, c.h, err)
			}
			time.Sleep(400 * time.Millisecond)
			state := "off"
			if c.todo {
				state = "on"
			}
			if _, err := screenshot(hwnd, fmt.Sprintf("d19-todo-%s-%dx%d", state, c.w, c.h)); err != nil {
				return err
			}
			l := out.Layout
			ratio := 0.0
			if l.PlainHistH > 0 {
				ratio = l.HistH / l.PlainHistH
			}
			fmt.Printf("uicheck: d19 %dx%d todo=%s: applied=%v chart %.0fpx of plain %.0fpx (ratio %.3f), To Do bottom %.0f of %.0f, dropped=%v\n",
				c.w, c.h, state, l.Applied, l.HistH, l.PlainHistH, ratio, out.TodoBottom, out.InnerH, out.TodoDropped)
			if out.BodyOver || out.ZoneOver {
				errs = append(errs, fmt.Sprintf("%dx%d todo=%s: page or a zone overflows", c.w, c.h, state))
			}
			if !c.todo || c.expectDrop {
				if c.expectDrop && !out.TodoDropped {
					errs = append(errs, fmt.Sprintf("%dx%d: expected To Do to be fit-dropped at this size", c.w, c.h))
				}
				if l.Applied || math.Abs(l.HistH-l.PlainHistH) > 0.5 {
					errs = append(errs, fmt.Sprintf("%dx%d To Do off or dropped: chart changed height (%.0f vs plain %.0f)", c.w, c.h, l.HistH, l.PlainHistH))
				}
				continue
			}
			if out.TodoDropped {
				errs = append(errs, fmt.Sprintf("%dx%d: To Do was fit-dropped, nothing to grow", c.w, c.h))
				continue
			}
			// Rounding the freed height to whole px moves the ratio by at
			// most 0.5px / plain height.
			if !l.Applied || math.Abs(ratio-0.7) > 0.01 {
				errs = append(errs, fmt.Sprintf("%dx%d: chart ratio %.3f applied=%v, want 0.700", c.w, c.h, ratio, l.Applied))
			}
			// .todopanel's own 4px bottom margin.
			if math.Abs(out.InnerH-4-out.TodoBottom) > 2 {
				errs = append(errs, fmt.Sprintf("%dx%d: To Do ends at %.0f, want the window bottom (%.0f)", c.w, c.h, out.TodoBottom, out.InnerH-4))
			}
		}
		if len(errs) > 0 {
			return fmt.Errorf("d19: %d problem(s):\n%s", len(errs), strings.Join(errs, "\n"))
		}
		if skipped > 0 {
			fmt.Printf("uicheck: d19: PARTIAL, %d of %d cases skipped because the screen is too small; run on a screen that holds 2560x1300 CSS px for a full pass\n", skipped, len(cases))
			return nil
		}
		fmt.Println("uicheck: d19: To Do layout clean (chart at 70% with To Do on, To Do fills the bottom, unchanged with To Do off, no overflow)")
		return nil
	}
}

package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// d19: with the To Do panel visible it is never hidden (B7 replaced the old
// 70 percent chart rule and the drop order): the fit pass shares the free
// height, To Do reaches the window bottom edge (the end of the scrolled
// page when every part sits at its floor) and no zone overflows. At a window
// too small for To Do in the plain layout (1280x860 today) it sits at its
// floor of three rows and the page scrolls. The three To Do
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
			if !c.todo {
				continue
			}
			if out.TodoDropped {
				errs = append(errs, fmt.Sprintf("%dx%d: To Do is hidden, B7 never hides a part", c.w, c.h))
				continue
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
		fmt.Println("uicheck: d19: To Do layout clean (To Do never hidden, fills the bottom, no overflow)")
		return nil
	}
}

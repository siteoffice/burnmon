package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

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
	if err := ensureWindowSizeWH(hwnd, 1920, 1080); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	errs := []string{}
	defer evalRaw(`(window.__bdevExitFakeMode && window.__bdevExitFakeMode())`)
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

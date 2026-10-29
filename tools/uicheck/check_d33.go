package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// d33: the row rules (B2). Driven by window.__bdevSections.apply with the
// frozen example, Standard, Overall scrollbar on. It proves:
//   - the first status row (#secRow1) is one row: the shown panels keep the
//     order waits, outcomes, burn, git, agents_map, share one top and one
//     width, and fill the row from edge to edge, with five, four or two on;
//   - the Activity and Heatmap row: both shown share one top and fill the
//     row; one shown takes the whole row;
//   - with the System and Processes parts off, the turn ticker panel is as
//     tall as its rows, or the room left in the burn zone when that is less.
//
// Screenshots: d33-row1-<width>-5, d33-row1-<width>-4, d33-row1-<width>-2,
// d33-ticker-noproc-<width>.

type d33Rect struct {
	ID string  `json:"id"`
	L  float64 `json:"left"`
	R  float64 `json:"right"`
	T  float64 `json:"top"`
	W  float64 `json:"width"`
}

type d33Tick struct {
	Top    float64 `json:"top"`
	Height float64 `json:"height"`
	Head   float64 `json:"head"`
	Scroll float64 `json:"scroll"`
	Bottom float64 `json:"zoneBottom"`
	Shown  bool    `json:"shown"`
}

type d33Page struct {
	RowL   float64   `json:"rowL"`
	RowR   float64   `json:"rowR"`
	Panels []d33Rect `json:"panels"`
	Split  []d33Rect `json:"split"`
	SplitL float64   `json:"splitL"`
	SplitR float64   `json:"splitR"`
	Gap    float64   `json:"gap"`
	Tick   d33Tick   `json:"tick"`
}

const d33ReadJS = `(function(){
  function vis(e){ return !!e && e.getClientRects().length > 0 && getComputedStyle(e).display !== 'none'; }
  function rect(e, id){ var r = e.getBoundingClientRect(); return {id: id, left: r.left, right: r.right, top: r.top, width: r.width}; }
  var row = document.getElementById('secRow1'), rr = row ? row.getBoundingClientRect() : {left: 0, right: 0};
  var panels = [];
  if(row) Array.prototype.forEach.call(row.querySelectorAll('.secpanel'), function(p){ if(vis(p)) panels.push(rect(p, p.getAttribute('data-sec'))); });
  var sp = document.getElementById('activityHeatmapRow'), split = [], sl = 0, sr = 0, gap = 0;
  if(sp && vis(sp)){
    var r2 = sp.getBoundingClientRect(); sl = r2.left; sr = r2.right;
    gap = parseFloat(getComputedStyle(sp).columnGap) || 0;
    ['activityPanel', 'harnessHeatmapPanel'].forEach(function(id){ var e = document.getElementById(id); if(vis(e)) split.push(rect(e, id)); });
  }
  var tp = document.getElementById('turnTickerPanel'), tk = document.getElementById('turnTicker');
  var zb = document.querySelector('.zone.burn .zonebody');
  var tick = {top: 0, height: 0, head: 0, scroll: 0, zoneBottom: 0, shown: false};
  if(vis(tp)){
    var tr = tp.getBoundingClientRect(), hd = tp.querySelector('.panelhead');
    tick = {top: tr.top, height: tr.height, head: hd ? hd.getBoundingClientRect().height : 0, scroll: tk.scrollHeight,
      zoneBottom: zb ? zb.getBoundingClientRect().bottom : 0, shown: true};
  }
  return {rowL: rr.left, rowR: rr.right, panels: panels, split: split, splitL: sl, splitR: sr, gap: gap, tick: tick};
})()`

func d33Fail(format string, args ...any) error { return fmt.Errorf("d33: "+format, args...) }

func d33Apply(payload string) (d33Page, error) {
	var p d33Page
	if _, err := evalRaw(`window.__bdevSections.apply(` + payload + `)`); err != nil {
		return p, err
	}
	time.Sleep(600 * time.Millisecond)
	err := evalInto(d33ReadJS, &p)
	return p, err
}

func caseD33(hwnd uintptr, args []string) error {
	snap, err := readFixture("snapshot_example.json")
	if err != nil {
		return fmt.Errorf("read the frozen example (uicheck runs from the tools folder): %w", err)
	}
	std := func(off ...string) string {
		m := map[string]bool{}
		for _, id := range off {
			m[id] = true
		}
		return d29Payload(d29Opts{Scrollbar: true, StandardOff: m, Status: "ok", Snap: snap})
	}
	order := []string{"waits", "outcomes", "burn", "git", "agents_map"}

	// First row: the shown panels fill one row, in order, equal widths.
	row1 := func(w, h int32, tag string, off ...string) error {
		p, err := d33Apply(std(off...))
		if err != nil {
			return err
		}
		skip := map[string]bool{}
		for _, o := range off {
			skip[o] = true
		}
		var want, got []string
		for _, id := range order {
			if !skip[id] {
				want = append(want, id)
			}
		}
		for _, r := range p.Panels {
			got = append(got, r.ID)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			return d33Fail("%dx%d first row shows %v, want %v", w, h, got, want)
		}
		for i, r := range p.Panels {
			if math.Abs(r.T-p.Panels[0].T) > 1 {
				return d33Fail("%dx%d first row wraps: %s top %.0f, %s top %.0f", w, h, r.ID, r.T, p.Panels[0].ID, p.Panels[0].T)
			}
			if math.Abs(r.W-p.Panels[0].W) > 2 {
				return d33Fail("%dx%d first row widths differ: %s %.0f, %s %.0f", w, h, r.ID, r.W, p.Panels[0].ID, p.Panels[0].W)
			}
			if i > 0 && r.L < p.Panels[i-1].L {
				return d33Fail("%dx%d first row order wrong at %s", w, h, r.ID)
			}
		}
		first, last := p.Panels[0], p.Panels[len(p.Panels)-1]
		if math.Abs(first.L-p.RowL) > 2 || math.Abs(last.R-p.RowR) > 2 {
			return d33Fail("%dx%d first row does not fill the row: %.0f..%.0f of %.0f..%.0f", w, h, first.L, last.R, p.RowL, p.RowR)
		}
		if _, err := screenshot(hwnd, tag); err != nil {
			return err
		}
		return nil
	}
	for _, size := range [][2]int32{{1280, 860}, {1920, 1080}} {
		w, h := size[0], size[1]
		if err := ensureWindowSizeWH(hwnd, w, h); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		if err := row1(w, h, fmt.Sprintf("d33-row1-%d-5", w)); err != nil {
			return err
		}
		if err := row1(w, h, fmt.Sprintf("d33-row1-%d-4", w), "git"); err != nil {
			return err
		}
		if err := row1(w, h, fmt.Sprintf("d33-row1-%d-2", w), "waits", "burn", "agents_map"); err != nil {
			return err
		}
	}

	// Activity and Heatmap: both fill the row, one takes it all.
	for _, size := range [][2]int32{{1920, 1080}, {1152, 2048}} {
		w, h := size[0], size[1]
		if err := ensureWindowSizeWH(hwnd, w, h); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		for _, c := range []struct {
			off  []string
			want int
		}{{nil, 2}, {[]string{"activity"}, 1}, {[]string{"heatmap"}, 1}} {
			p, err := d33Apply(std(c.off...))
			if err != nil {
				return err
			}
			if len(p.Split) != c.want {
				return d33Fail("Activity and Heatmap row dropped at %dx%d, cannot measure (off %v, shown %d)", w, h, c.off, len(p.Split))
			}
			width := p.SplitR - p.SplitL
			if c.want == 2 {
				a, b := p.Split[0], p.Split[1]
				if math.Abs(a.T-b.T) > 1 {
					return d33Fail("%dx%d Activity and Heatmap do not share a top: %.0f, %.0f", w, h, a.T, b.T)
				}
				if math.Abs(a.W+p.Gap+b.W-width) > 2 {
					return d33Fail("%dx%d Activity %.0f + gap %.0f + Heatmap %.0f != row %.0f", w, h, a.W, p.Gap, b.W, width)
				}
			} else if math.Abs(p.Split[0].W-width) > 2 {
				return d33Fail("%dx%d lone %s is %.0f wide, row is %.0f (off %v)", w, h, p.Split[0].ID, p.Split[0].W, width, c.off)
			}
		}
	}

	// Turn ticker: as tall as its rows, at most the room left in the burn zone, but never below the 80 px min-height floor.
	for _, size := range [][2]int32{{1920, 1080}, {1152, 2048}} {
		w, h := size[0], size[1]
		if err := ensureWindowSizeWH(hwnd, w, h); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		p, err := d33Apply(std("system", "process_groups", "processes"))
		if err != nil {
			return err
		}
		if !p.Tick.Shown {
			return d33Fail("%dx%d turn ticker not shown, cannot measure", w, h)
		}
		need := p.Tick.Head + p.Tick.Scroll
		room := p.Tick.Bottom - p.Tick.Top
		if p.Tick.Height > math.Max(80, math.Min(need, room))+6 {
			return d33Fail("%dx%d turn ticker is %.0f px tall, rows need %.0f, room %.0f, 80 px min-height floor", w, h, p.Tick.Height, need, room)
		}
		if _, err := screenshot(hwnd, fmt.Sprintf("d33-ticker-noproc-%d", w)); err != nil {
			return err
		}
	}
	return nil
}

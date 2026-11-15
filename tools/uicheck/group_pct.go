package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// pct/d47: the typed section and pane sizes (2026-11-15, replaces d42's drag
// handles). Standard, every part on except To Do, the frozen example through
// fake mode. Variants:
//   stacked   1280x900, scrollbar off, sections 30/50/20 (Status, Sessions,
//             Processes) and panes 40/20/20/20 in Sessions: each drawn section
//             height is within 2 px of its percent of the room the layout
//             pass measured, each Sessions pane within 2 px of its percent of
//             its section, and the page does not scroll.
//   wide      1920x1080, same numbers: Sessions and Processes side by side
//             share one row of the sum (50 + 20 = 70), Status keeps its 30.
//   scroll    1280x900, scrollbar on, sections 40/50/30 (120): drawn as typed
//             and the page scrolls.
//   refused   1280x900, scrollbar off, the same 120: drawn scaled to fit,
//             and typing 60 and 52 in the gear's Standard boxes turns Save off
//             with the sentence about the Overall scrollbar.
// A size the screen cannot hold prints SKIPPED and is not a pass.
// Screenshot d47-<variant>.

const d47ReadJS = `(function(){
  var f = window.__bdevFit || {};
  var shown = function(e){ return !!e && getComputedStyle(e).display !== 'none'; };
  var h = function(s){ var e = document.querySelector(s); return shown(e) ? e.getBoundingClientRect().height : 0; };
  var span = function(sels){
    var t = Infinity, b = -Infinity;
    sels.forEach(function(s){ var e = document.querySelector(s); if(!shown(e)) return; var r = e.getBoundingClientRect(); t = Math.min(t, r.top); b = Math.max(b, r.bottom); });
    return b > t ? b - t : 0;
  };
  var ps = document.getElementById('pageScroll');
  var fp = f.panes || {}, px = {};
  Object.keys(fp).forEach(function(k){ px[k] = fp[k].px; });
  return { room: f.room || 0, err: f.err || '', two: !!f.two, fitScroll: !!f.fitScroll,
    psH: ps.clientHeight, psScroll: ps.scrollHeight,
    sec: { status: h('#secHost'), sessions_30min: h('.zone.burn'), processes_30min: h('.zone.system'), todo: h('#todoPanel') },
    pane: { burn_30min: span(['.burnchart', '#burnLegend', '#sessionCards']), vendor_strip: h('#vendorStripPanel'),
      activity_heatmap: h('#activityHeatmapRow'), turn_ticker: h('#turnTickerPanel') },
    panePx: px };
})()`

type d47Read struct {
	Room      float64            `json:"room"`
	Err       string             `json:"err"`
	Two       bool               `json:"two"`
	FitScroll bool               `json:"fitScroll"`
	PsH       float64            `json:"psH"`
	PsScroll  float64            `json:"psScroll"`
	Sec       map[string]float64 `json:"sec"`
	Pane      map[string]float64 `json:"pane"`
	PanePx    map[string]float64 `json:"panePx"`
}

// d47Want is the drawn height each section must have, in percent of the room.
type d47Variant struct {
	name      string
	w, h      int32
	scrollbar bool
	secs      map[string]int
	panes     map[string]int
	want      map[string]float64 // section -> percent of the room
	scrolls   bool
	err       string
	gear      bool
}

// d47Check returns the problems in one read; pure, so it can be tested.
func d47Check(v d47Variant, r d47Read) []string {
	var bad []string
	if r.Room <= 0 {
		return []string{"the layout pass measured no room"}
	}
	for id, pct := range v.want {
		want := r.Room * pct / 100
		if math.Abs(r.Sec[id]-want) > 2 {
			bad = append(bad, fmt.Sprintf("%s drawn %.1f px, want %.1f (%.1f%% of %.0f)", id, r.Sec[id], want, pct, r.Room))
		}
	}
	// Only typed panes are held to their size: an untyped share can be smaller
	// than a pane's fixed parts (the Burn legend and cards, the CPU box), and
	// then the pane overflows and its section clips at the bottom.
	for id, px := range r.PanePx {
		got, ok := r.Pane[id]
		if _, typed := v.panes[id]; !ok || !typed {
			continue
		}
		if math.Abs(got-px) > 2 {
			bad = append(bad, fmt.Sprintf("pane %s drawn %.1f px, want %.1f", id, got, px))
		}
	}
	if v.panes != nil && len(r.PanePx) == 0 {
		bad = append(bad, "no pane sizes reported")
	}
	if v.scrolls != r.FitScroll {
		bad = append(bad, fmt.Sprintf("page scrolls = %v, want %v (content %.0f, box %.0f)", r.FitScroll, v.scrolls, r.PsScroll, r.PsH))
	}
	if r.Err != v.err {
		bad = append(bad, fmt.Sprintf("layout error %q, want %q", r.Err, v.err))
	}
	return bad
}

func d47Variants() []d47Variant {
	secs := map[string]int{"status": 30, "sessions_30min": 50, "processes_30min": 20}
	panes := map[string]int{"burn_30min": 40, "vendor_strip": 20, "activity_heatmap": 20, "turn_ticker": 20}
	over := map[string]int{"status": 40, "sessions_30min": 50, "processes_30min": 30}
	return []d47Variant{
		{name: "stacked", w: 1280, h: 900, secs: secs, panes: panes, want: map[string]float64{"status": 30, "sessions_30min": 50, "processes_30min": 20}},
		// Side by side the row is the sum, 50 + 20 = 70; Status keeps its 30 (second look 2026-11-15).
		{name: "wide", w: 1920, h: 1080, secs: secs, panes: panes, want: map[string]float64{"status": 30, "sessions_30min": 70, "processes_30min": 70}},
		{name: "scroll", w: 1280, h: 900, scrollbar: true, secs: over, want: map[string]float64{"status": 40, "sessions_30min": 50, "processes_30min": 30}, scrolls: true},
		{name: "refused", w: 1280, h: 900, secs: over, want: map[string]float64{"status": 40.0 / 1.2, "sessions_30min": 50.0 / 1.2, "processes_30min": 30.0 / 1.2},
			err: "Sections add up to 120 percent; switch on Overall scrollbar or lower a number", gear: true},
	}
}

const d47GearJS = `(function(){
  var set = function(id, v){ var i = document.querySelector('#secSizeGrid input[data-pct-kind="sections"][data-pct-id="' + id + '"][data-mode="standard"]'); i.value = v; };
  set('status', '60'); set('sessions_30min', '52'); set('processes_30min', ''); set('todo', '');
  document.getElementById('secSizeGrid').dispatchEvent(new Event('input', {bubbles: true}));
  return { disabled: document.getElementById('secSizeSave').disabled, why: document.getElementById('secSizeWhy').textContent };
})()`

func caseD47(hwnd uintptr, args []string) error {
	snap, err := d34Snapshot(false)
	if err != nil {
		return err
	}
	var fails []string
	ran := 0
	for _, v := range d47Variants() {
		if err := ensureWindowSizeWH(hwnd, v.w, v.h); err != nil {
			return err
		}
		if windowCapped {
			fmt.Printf("uicheck: d47: SKIPPED %s %dx%d, this screen is too small\n", v.name, v.w, v.h)
			continue
		}
		time.Sleep(500 * time.Millisecond)
		extra := map[string]any{"sections_pct": map[string]any{"standard": v.secs}}
		if v.panes != nil {
			extra["panes_pct"] = map[string]any{"standard": v.panes}
		}
		payload, err := d41Payload(d29Opts{Status: "ok", Snap: snap, Scrollbar: v.scrollbar, StandardOff: map[string]bool{"todo": true}}, extra)
		if err != nil {
			return err
		}
		if _, err := d34Apply(payload, snap); err != nil {
			return err
		}
		var r d47Read
		if err := evalInto(d47ReadJS, &r); err != nil {
			return fmt.Errorf("d47 %s: read: %w", v.name, err)
		}
		if _, err := screenshot(hwnd, "d47-"+v.name); err != nil {
			return err
		}
		bad := d47Check(v, r)
		if v.wide() && !r.Two {
			bad = append(bad, "the window is wide but the layout pass did not put Sessions and Processes side by side")
		}
		if v.gear {
			var g struct {
				Disabled bool   `json:"disabled"`
				Why      string `json:"why"`
			}
			if err := evalInto(d47GearJS, &g); err != nil {
				return fmt.Errorf("d47 %s: gear: %w", v.name, err)
			}
			want := "Standard: Sections add up to 112 percent; switch on Overall scrollbar or lower a number"
			if !g.Disabled || g.Why != want {
				bad = append(bad, fmt.Sprintf("gear Save disabled=%v why %q, want disabled with %q", g.Disabled, g.Why, want))
			}
		}
		ran++
		if len(bad) > 0 {
			fails = append(fails, v.name+": "+strings.Join(bad, "; "))
			continue
		}
		fmt.Printf("uicheck: d47: PASS %s (room %.0f px, sections %.0f/%.0f/%.0f px)\n", v.name, r.Room, r.Sec["status"], r.Sec["sessions_30min"], r.Sec["processes_30min"])
	}
	if len(fails) > 0 {
		return fmt.Errorf("d47: %d variant(s) failed:\n%s", len(fails), strings.Join(fails, "\n"))
	}
	if ran == 0 {
		return fmt.Errorf("d47: every variant was skipped, this screen holds none of the sizes")
	}
	fmt.Println("uicheck: d47: typed sizes drawn within 2 px, over 100 scrolls with the scrollbar on and is refused with it off")
	return nil
}

func (v d47Variant) wide() bool { return v.w >= 1600 }

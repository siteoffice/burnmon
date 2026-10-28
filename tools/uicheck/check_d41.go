package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// d41: the Station takes exactly the height of the parts it replaces (B7.1,
// spec 2026-10-28, item 4). At 1280x800 and 1920x1080 (a size the screen
// cannot hold prints SKIPPED and is not a pass), Standard, Overall scrollbar
// off, three variants: all parts on; turn ticker off; saved shares injected
// through heights_standard (burn_30min 0.25, vendor_strip 0.15,
// activity_heatmap 0.3, turn_ticker 0.3, system_chart 0.6, process_groups 0.4).
// Each variant applies the payload and the frozen example through fake mode,
// reads the page, presses a real O (the window must be in front for keys),
// reads again, presses O again and reads a third time.
// Asserted: while open the Station box is within 2 px of the span the parts had
// (bottom of the last minus top of the first); the burn zone, the system zone,
// main and the System chart keep their top and height within 1 px; the zone
// label stays; after the second O there is no box and every part has its top
// and height back within 1 px. Screenshot d41-station-<size>-<variant> while
// the Station is open.

const d41ReadJS = `(function(){
  var shown = function(e){ return !!e && e.getClientRects().length > 0 && getComputedStyle(e).display !== 'none'; };
  var zb = document.querySelector('.zone.burn .zonebody');
  var parts = [], top = Infinity, bot = -Infinity, box = 0;
  Array.prototype.forEach.call(zb.children, function(c){
    if(c.classList.contains('hdl')) return;
    var r = c.getBoundingClientRect();
    if(c.id === 'stationPanel'){ box = shown(c) ? r.height : 0; return; }
    if(!shown(c) || r.height <= 0.5) return;
    parts.push({id: c.id || c.className, top: r.top, h: r.height});
    top = Math.min(top, r.top); bot = Math.max(bot, r.bottom);
  });
  var rc = function(s){ var e = document.querySelector(s); if(!e || !shown(e)) return {top: 0, h: 0}; var r = e.getBoundingClientRect(); return {top: r.top, h: r.height}; };
  var hw = document.querySelector('.histwrap');
  return {span: bot > top ? bot - top : 0, parts: parts, box: box, burn: rc('.zone.burn'), sys: rc('.zone.system'), main: rc('main'),
    hist: hw ? hw.getBoundingClientRect().height : 0, label: shown(document.querySelector('.zone.burn .zonelabel'))};
})()`

type d41Box struct {
	Top float64 `json:"top"`
	H   float64 `json:"h"`
}

type d41Part struct {
	ID  string  `json:"id"`
	Top float64 `json:"top"`
	H   float64 `json:"h"`
}

type d41Read struct {
	Span  float64   `json:"span"`
	Parts []d41Part `json:"parts"`
	Box   float64   `json:"box"`
	Burn  d41Box    `json:"burn"`
	Sys   d41Box    `json:"sys"`
	Main  d41Box    `json:"main"`
	Hist  float64   `json:"hist"`
	Label bool      `json:"label"`
}

// d41Payload is d29Payload with extra top-level fields set on the decoded map.
func d41Payload(o d29Opts, extra map[string]any) (string, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(d29Payload(o)), &m); err != nil {
		return "", err
	}
	for k, v := range extra {
		m[k] = v
	}
	b, err := json.Marshal(m)
	return string(b), err
}

func d41Close(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// d41PressO presses a real O and waits (up to 1.5 s) for the Station box to be
// there (want true) or gone (want false). Keys need the window to hold input
// focus, which drifts now and then, so when the state did not change it gets
// focus back with a real click on the logo (d23's way) and presses once more.
// The state is checked before the retry, so a key that did arrive is never
// pressed twice. After four tries it fails the run with the focus as the cause.
func d41PressO(hwnd uintptr, want bool) error {
	for try := 0; try < 4; try++ {
		bringToFront(hwnd)
		if try > 0 {
			time.Sleep(500 * time.Millisecond)
			fmt.Println("uicheck: d41: O had no effect, taking focus with a click and pressing again")
		}
		if err := clickSelectorAt(hwnd, ".logo", 0.5, 0.5); err != nil {
			fmt.Println("uicheck: d41: could not click .logo to establish focus:", err)
		}
		time.Sleep(200 * time.Millisecond)
		if err := pressKey(vkO); err != nil {
			return err
		}
		for i := 0; i < 8; i++ {
			time.Sleep(200 * time.Millisecond)
			var has bool
			if err := evalInto(`!!document.getElementById('stationPanel')`, &has); err != nil {
				return err
			}
			if has == want {
				return nil
			}
		}
	}
	return fmt.Errorf("d41: O had no effect after 4 tries (the window did not take input focus), want the Station box present=%v", want)
}

func init() {
	checks["d41"] = func(hwnd uintptr, args []string) error {
		defer evalRaw(`(window.__bdevExitFakeMode && window.__bdevExitFakeMode())`)
		snap, err := d34Snapshot(false)
		if err != nil {
			return err
		}
		shares := map[string]any{"burn_30min": 0.25, "vendor_strip": 0.15, "activity_heatmap": 0.3, "turn_ticker": 0.3, "system_chart": 0.6, "process_groups": 0.4}
		variants := []struct {
			name  string
			off   map[string]bool
			extra map[string]any
		}{
			{"all", nil, nil},
			{"no-ticker", map[string]bool{"turn_ticker": true}, nil},
			{"saved-shares", nil, map[string]any{"heights_standard": shares}},
		}
		var fails []string
		ran := 0
		for _, sz := range [][2]int32{{1280, 800}, {1920, 1080}} {
			tag := fmt.Sprintf("%dx%d", sz[0], sz[1])
			if err := ensureWindowSizeWH(hwnd, sz[0], sz[1]); err != nil {
				return err
			}
			if windowCapped {
				fmt.Printf("uicheck: d41: SKIPPED %s, this screen is too small\n", tag)
				continue
			}
			time.Sleep(500 * time.Millisecond)
			for _, v := range variants {
				payload, err := d41Payload(d29Opts{Status: "ok", Snap: snap, StandardOff: v.off}, v.extra)
				if err != nil {
					return err
				}
				if _, err := d34Apply(payload, snap); err != nil {
					return err
				}
				var r0, r1, r2 d41Read
				if err := evalInto(d41ReadJS, &r0); err != nil {
					return err
				}
				if err := d41PressO(hwnd, true); err != nil {
					return err
				}
				time.Sleep(700 * time.Millisecond)
				err = evalInto(d41ReadJS, &r1)
				_, shotErr := screenshot(hwnd, fmt.Sprintf("d41-station-%s-%s", tag, v.name))
				if err == nil {
					err = shotErr
				}
				// Close the Station even when a read failed.
				if kerr := d41PressO(hwnd, false); err == nil {
					err = kerr
				}
				time.Sleep(700 * time.Millisecond)
				if err != nil {
					return err
				}
				if err := evalInto(d41ReadJS, &r2); err != nil {
					return err
				}
				var bad []string
				if len(r0.Parts) == 0 || r0.Span <= 0 {
					bad = append(bad, "no parts in the burn zone before the swap")
				}
				if r1.Box <= 0 || !d41Close(r1.Box, r0.Span, 2) {
					diag, _ := evalString(`"focus " + document.hasFocus() + ", view " + window.__bdevStationView() + ", active " + (document.activeElement ? document.activeElement.tagName + "#" + document.activeElement.id : "-")`)
					bad = append(bad, fmt.Sprintf("box %.1f px vs span %.1f px (%s)", r1.Box, r0.Span, diag))
				}
				for _, p := range []struct {
					n    string
					a, b d41Box
				}{{"burn zone", r0.Burn, r1.Burn}, {"system zone", r0.Sys, r1.Sys}, {"main", r0.Main, r1.Main}} {
					if !d41Close(p.a.Top, p.b.Top, 1) || !d41Close(p.a.H, p.b.H, 1) {
						bad = append(bad, fmt.Sprintf("%s moved (top %.1f to %.1f, height %.1f to %.1f)", p.n, p.a.Top, p.b.Top, p.a.H, p.b.H))
					}
				}
				if !d41Close(r0.Hist, r1.Hist, 1) {
					bad = append(bad, fmt.Sprintf("System chart height %.1f to %.1f", r0.Hist, r1.Hist))
				}
				if !r1.Label {
					bad = append(bad, "the zone label is hidden")
				}
				if r2.Box != 0 {
					bad = append(bad, "the box is still there after the second O")
				}
				if len(r2.Parts) != len(r0.Parts) {
					bad = append(bad, fmt.Sprintf("%d parts back, want %d", len(r2.Parts), len(r0.Parts)))
				} else {
					for i := range r0.Parts {
						a, b := r0.Parts[i], r2.Parts[i]
						if !d41Close(a.Top, b.Top, 1) || !d41Close(a.H, b.H, 1) {
							bad = append(bad, fmt.Sprintf("part %s back at top %.1f height %.1f, was %.1f and %.1f", a.ID, b.Top, b.H, a.Top, a.H))
						}
					}
				}
				ran++
				if len(bad) == 0 {
					fmt.Printf("uicheck: d41: PASS %s %s (%d parts, span %.1f px, box %.1f px, burn zone %.1f px, main %.1f px, label shown, parts back)\n", tag, v.name, len(r0.Parts), r0.Span, r1.Box, r1.Burn.H, r1.Main.H)
				} else {
					msg := fmt.Sprintf("%s %s: %s", tag, v.name, strings.Join(bad, "; "))
					fmt.Println("uicheck: d41: FAIL " + msg)
					fails = append(fails, msg)
				}
			}
		}
		if err := ensureWindowSizeWH(hwnd, 1152, 2048); err != nil {
			return err
		}
		if len(fails) > 0 {
			return fmt.Errorf("d41: %d case(s) failed, first: %s", len(fails), fails[0])
		}
		if ran == 0 {
			return fmt.Errorf("d41: no size ran")
		}
		return nil
	}
}

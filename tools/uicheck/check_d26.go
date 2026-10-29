package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// d26: the space theme's engine glow under the deck has no hard top edge
// (02_roadmap\2026-10-03_station_ui_and_backlog.md, item D, 2026-10-05).
// station.js fills the radial gradient into a rect that started at the deck's
// bottom corner, so the upper part of the glow was cut flat. A spy on the 2D
// context records every glow fill (the gradient whose first stop is
// rgba(56,189,248,0.22)) and the floor canvas that buildFloor bakes, then per
// state it reads
//
//	cover:  the filled rect contains the gradient's whole bounding square
//	cols/maxDelta: along the two floor rows 2 px apart across the old edge,
//	  for every column that is neither deck nor slab (alpha under 250 on both),
//	  the largest alpha step between them (0 to 255). A flat edge shows as a
//	  step of 25 to 40, a clean glow as 0 or 1. cols guards against a
//	  vacuous read.
//	hash:   FNV-1a of every floor pixel, so a before and an after run can be
//	  compared for the views and themes that must not change.
//
// Views are driven with synthetic key events (no window focus needed) and the
// camera with synthetic wheel and mouse events. Only the P view in the space
// theme is asserted; I and the site theme are recorded for comparison, and O
// (drawTop, buildPixel) must not run buildFloor at all: neither the floor
// build counter nor the glow counter may move while it is open.
// UICHECK_TAG names the run: screenshots are d26-<tag>-<state> and the lines
// go to testdata\uicheck\out\d26-<tag>.txt.
const d26SpyJS = `(function(){
  if(window.__glow) return true;
  var g = window.__glow = {calls: [], floor: null, builds: 0};
  var C = CanvasRenderingContext2D.prototype, H = HTMLCanvasElement.prototype;
  var cg = C.createRadialGradient, fr = C.fillRect, gc = H.getContext;
  C.createRadialGradient = function(){
    var gr = cg.apply(this, arguments);
    gr.__a = Array.prototype.slice.call(arguments); gr.__stops = [];
    var as = gr.addColorStop;
    gr.addColorStop = function(o, c){ gr.__stops.push(c); return as.call(gr, o, c); };
    return gr;
  };
  C.fillRect = function(x, y, w, h){
    var fs = this.fillStyle;
    if(fs && fs.__a && fs.__stops[0] && fs.__stops[0].indexOf('56,189,248,0.22') >= 0) g.calls.push({a: fs.__a, rect: [x, y, w, h]});
    return fr.apply(this, arguments);
  };
  H.getContext = function(){
    if(/buildFloor/.test(new Error().stack)){ g.floor = this; g.builds++; }
    return gc.apply(this, arguments);
  };
  return true;
})()`

const d26ReadJS = `(function(){
  var g = window.__glow, cv = g.floor, out = {builds: g.builds, glows: g.calls.length};
  if(!cv) return out;
  var W = cv.width, Hh = cv.height, data = cv.getContext('2d').getImageData(0, 0, W, Hh).data, h = 2166136261, i;
  for(i = 0; i < data.length; i++){ h ^= data[i]; h = Math.imul(h, 16777619); }
  out.hash = (h >>> 0).toString(16); out.w = W; out.h = Hh;
  var c = g.calls[g.calls.length - 1];
  if(!c) return out;
  var cx = c.a[0], cy = c.a[1], r = c.a[5], rc = c.rect, eps = 0.5;
  out.cover = rc[0] <= cx - r + eps && rc[1] <= cy - r + eps && rc[0] + rc[2] >= cx + r - eps && rc[1] + rc[3] >= cy + r - eps;
  var dpr = window.devicePixelRatio || 1, d = 34 * r / 260, b1 = cy - 2 * d;
  var ya = Math.floor(b1 * dpr) - 1, yb = ya + 2, x0 = Math.max(0, Math.floor((cx - r) * dpr)), x1 = Math.min(W - 1, Math.floor((cx + r) * dpr));
  out.cols = 0; out.maxDelta = 0;
  if(ya >= 0 && yb < Hh) for(var x = x0; x <= x1; x++){
    // Glow alone tops out at 0.22 * 255 = 56. A column with anything stronger
    // within 5 rows (deck, slab, or the antialiased pixel at the slab's edge)
    // is not glow-only, so it is left out.
    var strong = false, yy;
    for(yy = ya - 5; yy <= yb + 5 && !strong; yy++) if(yy < 0 || yy >= Hh || data[(yy * W + x) * 4 + 3] > 60) strong = true;
    if(strong) continue;
    var A = data[(ya * W + x) * 4 + 3], B = data[(yb * W + x) * 4 + 3];
    out.cols++;
    if(Math.abs(B - A) > out.maxDelta){
      out.maxDelta = Math.abs(B - A); out.worstX = x - Math.round(cx * dpr); out.profile = [];
      for(yy = ya - 5; yy <= yb + 5; yy++) out.profile.push(data[(yy * W + x) * 4 + 3]);
    }
  }
  return out;
})()`

type d26Read struct {
	Builds   int    `json:"builds"`
	Glows    int    `json:"glows"`
	Hash     string `json:"hash"`
	W        int    `json:"w"`
	H        int    `json:"h"`
	Cover    *bool  `json:"cover"`
	Cols     int    `json:"cols"`
	MaxDelta int    `json:"maxDelta"`
	WorstX   int    `json:"worstX"`
	Profile  []int  `json:"profile"`
}

const d26KeyJS = `(document.dispatchEvent(new KeyboardEvent('keydown', {key: %q, bubbles: true})), true)`

const d26CanvasJS = `(function(){
  var c = document.querySelector('#stationOverlay canvas') || document.querySelector('#stationPanel canvas');
  var r = c.getBoundingClientRect();
  return {c: c, x: r.left + r.width / 2, y: r.top + r.height / 2};
})()`

func caseD26(hwnd uintptr, args []string) error {
	tag := os.Getenv("UICHECK_TAG")
	if tag == "" {
		tag = "run"
	}
	if err := ensureWindowSizeWH(hwnd, 1600, 900); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := evalRaw(d23FakeJS); err != nil {
		return fmt.Errorf("d26: fake paint: %w", err)
	}
	if _, err := evalRaw(d26SpyJS); err != nil {
		return fmt.Errorf("d26: spy: %w", err)
	}
	bringToFront(hwnd)
	var errs, lines []string
	key := func(k string) {
		if _, err := evalRaw(fmt.Sprintf(d26KeyJS, k)); err != nil {
			errs = append(errs, fmt.Sprintf("key %s: %v", k, err))
		}
	}
	theme := func(want string) {
		var cur string
		if err := evalInto(`(window.__bdevStationTheme ? (window.__bdevStationTheme() || '') : '')`, &cur); err != nil {
			errs = append(errs, fmt.Sprintf("theme read: %v", err))
			return
		}
		if cur != want {
			key("t")
		}
	}
	// camera drives the Station canvas with synthetic events.
	camera := func(js string) {
		if _, err := evalRaw(`(function(){ var q = ` + d26CanvasJS + `, c = q.c, x = q.x, y = q.y; ` + js + ` return true; })()`); err != nil {
			errs = append(errs, fmt.Sprintf("camera: %v", err))
		}
	}
	// zoomIn anchors the wheel on the deck's bottom corner (read back from
	// the last glow fill), so the corner stays on screen while it scales.
	zoomIn := func() {
		camera(`var k = window.__glow.calls[window.__glow.calls.length - 1].a, r = c.getBoundingClientRect();
				x = r.left + k[0]; y = r.top + k[1] - 2 * 34 * k[5] / 260;
				c.dispatchEvent(new WheelEvent('wheel', {deltaY: -400, clientX: x, clientY: y, bubbles: true, cancelable: true}));`)
	}
	pan := func() {
		camera(`c.dispatchEvent(new MouseEvent('mousedown', {clientX: x, clientY: y, bubbles: true}));
				c.dispatchEvent(new MouseEvent('mousemove', {clientX: x - 170, clientY: y - 110, bubbles: true}));
				window.dispatchEvent(new MouseEvent('mouseup', {clientX: x - 170, clientY: y - 110, bubbles: true}));`)
	}
	// measure waits for the floor to be rebuilt after the last change,
	// then shoots and reads. kind is "assert" (the glow must be whole),
	// "record" (print only) or "same" (the plan view never builds the
	// isometric floor: neither counter may move).
	var prevBuilds, prevGlows int
	measure := func(label, kind string) {
		time.Sleep(900 * time.Millisecond)
		bringToFront(hwnd)
		if _, err := screenshot(hwnd, "d26-"+tag+"-"+label); err != nil {
			errs = append(errs, fmt.Sprintf("%s: screenshot: %v", label, err))
		}
		var r d26Read
		if err := evalInto(d26ReadJS, &r); err != nil {
			errs = append(errs, fmt.Sprintf("%s: read: %v", label, err))
			return
		}
		// The glow numbers belong to the last glow fill; a build with no new
		// fill (site theme, plan view) has none of its own.
		fresh := r.Glows > prevGlows
		cover := "n/a"
		if r.Cover != nil && fresh {
			cover = fmt.Sprint(*r.Cover)
		} else {
			r.Cols, r.MaxDelta = 0, 0
		}
		hash := r.Hash
		if kind == "same" {
			hash = "n/a"
		}
		line := fmt.Sprintf("d26 %-18s hash=%s size=%dx%d builds=%d glows=%d cover=%s cols=%d maxDelta=%d", label, hash, r.W, r.H, r.Builds, r.Glows, cover, r.Cols, r.MaxDelta)
		if r.MaxDelta > 3 {
			line += fmt.Sprintf(" worstDX=%d profile=%v", r.WorstX, r.Profile)
		}
		fmt.Println("uicheck: " + line)
		lines = append(lines, line)
		if kind == "same" {
			if r.Builds != prevBuilds || r.Glows != prevGlows {
				errs = append(errs, fmt.Sprintf("%s: the plan view ran buildFloor (builds %d to %d, glows %d to %d)", label, prevBuilds, r.Builds, prevGlows, r.Glows))
			}
		} else if r.Builds <= prevBuilds {
			errs = append(errs, fmt.Sprintf("%s: the floor was not rebuilt (builds %d)", label, r.Builds))
		}
		prevBuilds, prevGlows = r.Builds, r.Glows
		if kind == "assert" {
			switch {
			case !fresh || r.Cover == nil:
				errs = append(errs, fmt.Sprintf("%s: no glow fill recorded for this build", label))
			case !*r.Cover:
				errs = append(errs, fmt.Sprintf("%s: the glow fill does not cover its gradient square", label))
			}
			if r.Cols < 20 {
				errs = append(errs, fmt.Sprintf("%s: only %d glow-only columns read, the edge measure is vacuous", label, r.Cols))
			} else if r.MaxDelta > 3 {
				errs = append(errs, fmt.Sprintf("%s: hard edge, alpha steps by %d across the old rect top", label, r.MaxDelta))
			}
		}
	}

	// P, the full-window 3D view: site first (no glow), then space.
	key("p")
	time.Sleep(700 * time.Millisecond)
	theme("site")
	measure("P-site-fit", "record")
	theme("space")
	measure("P-space-fit", "assert")
	zoomIn()
	measure("P-space-zoomin", "assert")
	pan()
	measure("P-space-zoomin-pan", "assert")
	key("p")
	time.Sleep(400 * time.Millisecond)

	// O, the plan view in the burn zone: it must not touch the floor code.
	key("o")
	time.Sleep(700 * time.Millisecond)
	theme("space")
	measure("O-space-fit", "same")
	theme("site")
	measure("O-site-fit", "same")
	key("o")
	time.Sleep(400 * time.Millisecond)

	// I, the 3D view in the burn zone: recorded, not asserted.
	key("i")
	time.Sleep(700 * time.Millisecond)
	theme("space")
	measure("I-space-fit", "record")
	theme("site")
	measure("I-site-fit", "record")
	key("i")

	dir := filepath.Join("..", "..", "testdata", "uicheck", "out")
	if err := os.MkdirAll(dir, 0o755); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "d26-"+tag+".txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	}
	if len(errs) > 0 {
		return fmt.Errorf("d26: %d problem(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}
	fmt.Println("uicheck: d26: the engine glow is whole at three camera states in the P view, space theme")
	return nil
}

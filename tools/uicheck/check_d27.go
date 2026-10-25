package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// d27: the site theme's clouds look like clouds
// (02_roadmap\2026-10-03_station_ui_and_backlog.md, item E, 2026-10-05).
// buildSky drew each cloud as four flat ellipses at 50 percent white, so the
// overlaps doubled up and read as stacked discs. The fix draws each cloud on
// its own offscreen canvas as 5 to 8 soft puffs and composites it once at one
// alpha, seeded and baked once. A spy on the 2D context records, per sky
// build (buildStars with the site theme): the offscreen cloud canvases, their
// arc counts and the alpha each is composited at, and the ellipses drawn
// straight onto the sky. Then per state it reads the baked sky canvas:
//
//	step:   the largest luminance step between two neighbouring pixels in the
//	  sky above the horizon. A flat ellipse edge steps by 20 to 40, soft puffs
//	  by a few, the plain gradient and sun by about 1.
//	ground: FNV-1a of the ground below the horizon, which the fix must leave
//	  alone (the seeded random stream after the clouds is unchanged).
//	hash:   FNV-1a of the whole sky, to prove a rebuild at the same size is
//	  identical (seeded).
//
// The baked sky is saved as d27-<tag>-<state>-sky.png (no HUD, no props) so a
// before and an after run can be compared pixel for pixel. Views are driven
// with synthetic key events, the page theme with L (restored on exit, the
// choice is stored by the Go side), the window with ensureWindowSizeWH.
// UICHECK_TAG names the run.
const d27SpyJS = `(function(){
  if(window.__sky) return true;
  var g = window.__sky = {sky: null, builds: 0, clouds: [], ellipses: 0};
  var C = CanvasRenderingContext2D.prototype, H = HTMLCanvasElement.prototype;
  var gc = H.getContext, ar = C.arc, el = C.ellipse, di = C.drawImage;
  H.getContext = function(){
    var st = new Error().stack;
    if(/buildSky/.test(st)){
      if(!this.__cl){ this.__cl = {arcs: 0, alpha: null, draws: 0, w: this.width, h: this.height}; g.clouds.push(this.__cl); }
    } else if(/buildStars/.test(st) && this !== g.sky){ g.sky = this; g.builds++; g.clouds = []; g.ellipses = 0; }
    return gc.apply(this, arguments);
  };
  C.arc = function(){ var cl = this.canvas && this.canvas.__cl; if(cl) cl.arcs++; return ar.apply(this, arguments); };
  C.ellipse = function(){ if(this.canvas === g.sky) g.ellipses++; return el.apply(this, arguments); };
  C.drawImage = function(src){ if(src && src.__cl){ src.__cl.alpha = this.globalAlpha; src.__cl.draws++; } return di.apply(this, arguments); };
  return true;
})()`

const d27ReadJS = `(function(){
  var g = window.__sky, cv = g.sky;
  var out = {builds: g.builds, ellipses: g.ellipses, theme: window.__bdevStationTheme ? (window.__bdevStationTheme() || '') : '',
    page: document.documentElement.getAttribute('data-theme') || '',
    clouds: g.clouds.map(function(c){ return {arcs: c.arcs, alpha: c.alpha, draws: c.draws}; })};
  if(!cv) return out;
  var W = cv.width, H = cv.height, d = cv.getContext('2d').getImageData(0, 0, W, H).data, i, x, y;
  var hz = Math.floor(H * 0.3);
  function fnv(from, to){ var h = 2166136261; for(var k = from; k < to; k++){ h ^= d[k]; h = Math.imul(h, 16777619); } return (h >>> 0).toString(16); }
  function lum(k){ return 0.299 * d[k] + 0.587 * d[k + 1] + 0.114 * d[k + 2]; }
  out.w = W; out.h = H; out.hash = fnv(0, d.length); out.ground = fnv((hz + 10) * W * 4, d.length);
  var rows = hz - 10, step = 0, a, s;
  for(y = 0; y < rows; y++) for(x = 0; x < W - 1; x++){
    i = (y * W + x) * 4; a = lum(i);
    s = Math.abs(a - lum(i + 4)); if(s > step) step = s;
    if(y + 1 < rows){ s = Math.abs(a - lum(i + W * 4)); if(s > step) step = s; }
  }
  out.step = Math.round(step * 10) / 10;
  return out;
})()`

type d27Cloud struct {
	Arcs  int      `json:"arcs"`
	Alpha *float64 `json:"alpha"`
	Draws int      `json:"draws"`
}

type d27Read struct {
	Builds   int        `json:"builds"`
	Ellipses int        `json:"ellipses"`
	Theme    string     `json:"theme"`
	Page     string     `json:"page"`
	Clouds   []d27Cloud `json:"clouds"`
	W        int        `json:"w"`
	H        int        `json:"h"`
	Hash     string     `json:"hash"`
	Ground   string     `json:"ground"`
	Step     float64    `json:"step"`
}

func init() {
	checks["d27"] = func(hwnd uintptr, args []string) error {
		tag := os.Getenv("UICHECK_TAG")
		if tag == "" {
			tag = "run"
		}
		if err := ensureWindowSizeWH(hwnd, 1600, 900); err != nil {
			return err
		}
		time.Sleep(300 * time.Millisecond)
		defer evalRaw(`(window.__bdevExitFakeMode && window.__bdevExitFakeMode())`)
		if _, err := evalRaw(d23FakeJS); err != nil {
			return fmt.Errorf("d27: fake paint: %w", err)
		}
		if _, err := evalRaw(d27SpyJS); err != nil {
			return fmt.Errorf("d27: spy: %w", err)
		}
		bringToFront(hwnd)
		var errs, lines []string
		key := func(k string) {
			if _, err := evalRaw(fmt.Sprintf(d26KeyJS, k)); err != nil {
				errs = append(errs, fmt.Sprintf("key %s: %v", k, err))
			}
		}
		read := func() (r d27Read) {
			if err := evalInto(d27ReadJS, &r); err != nil {
				errs = append(errs, fmt.Sprintf("read: %v", err))
			}
			return r
		}
		// The page starts in the remembered theme; whatever it was is put back.
		startPage := read().Page
		defer func() {
			if read().Page != startPage {
				key("l")
				time.Sleep(400 * time.Millisecond)
			}
		}()
		// measure shoots, saves the baked sky and prints one line. asserted
		// says the clouds must be puffs, one alpha, with no hard step.
		measure := func(label string, asserted bool, shot bool) d27Read {
			time.Sleep(900 * time.Millisecond)
			bringToFront(hwnd)
			if shot {
				if _, err := screenshot(hwnd, "d27-"+tag+"-"+label); err != nil {
					errs = append(errs, fmt.Sprintf("%s: screenshot: %v", label, err))
				}
			}
			r := read()
			if r.Theme != "site" {
				errs = append(errs, fmt.Sprintf("%s: station theme is %q, want site", label, r.Theme))
			}
			arcs := make([]int, len(r.Clouds))
			for i, c := range r.Clouds {
				arcs[i] = c.Arcs
			}
			line := fmt.Sprintf("d27 %-22s size=%dx%d page=%q builds=%d clouds=%d arcs=%v ellipses=%d step=%.1f hash=%s ground=%s",
				label, r.W, r.H, r.Page, r.Builds, len(r.Clouds), arcs, r.Ellipses, r.Step, r.Hash, r.Ground)
			fmt.Println("uicheck: " + line)
			lines = append(lines, line)
			if shot {
				if data, err := evalString(`window.__sky.sky.toDataURL('image/png')`); err != nil {
					errs = append(errs, fmt.Sprintf("%s: sky dump: %v", label, err))
				} else if raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, "data:image/png;base64,")); err != nil {
					errs = append(errs, fmt.Sprintf("%s: sky decode: %v", label, err))
				} else {
					_ = os.WriteFile(filepath.Join("..", "..", "testdata", "uicheck", "out", "d27-"+tag+"-"+label+"-sky.png"), raw, 0o644)
				}
			}
			if !asserted {
				return r
			}
			if len(r.Clouds) != 6 {
				errs = append(errs, fmt.Sprintf("%s: %d offscreen clouds, want 6", label, len(r.Clouds)))
			}
			for i, c := range r.Clouds {
				if c.Arcs < 5 || c.Arcs > 8 {
					errs = append(errs, fmt.Sprintf("%s: cloud %d has %d puffs, want 5 to 8", label, i, c.Arcs))
				}
				if c.Draws != 1 || c.Alpha == nil || *c.Alpha <= 0 || *c.Alpha >= 1 {
					errs = append(errs, fmt.Sprintf("%s: cloud %d is composited %d times at alpha %v, want once below 1", label, i, c.Draws, c.Alpha))
				}
			}
			if r.Ellipses != 0 {
				errs = append(errs, fmt.Sprintf("%s: %d ellipses drawn straight onto the sky", label, r.Ellipses))
			}
			if r.Step > 12 {
				errs = append(errs, fmt.Sprintf("%s: hard edge in the sky, neighbouring pixels step by %.1f", label, r.Step))
			}
			return r
		}

		key("p")
		time.Sleep(900 * time.Millisecond)
		// The Station starts in the saved theme (space by default); the sky
		// being measured is the site one, so one T reaches it.
		if read().Theme == "space" {
			key("t")
			time.Sleep(400 * time.Millisecond)
		}
		a := measure("P-site-1600", true, true)

		// Seeded: a rebuild at the same size is the same sky.
		key("t")
		time.Sleep(700 * time.Millisecond)
		key("t")
		a2 := measure("P-site-1600-rebuilt", true, false)
		if a2.Builds <= a.Builds {
			errs = append(errs, fmt.Sprintf("rebuild: the sky was not rebuilt (builds %d to %d)", a.Builds, a2.Builds))
		}
		if a2.Hash != a.Hash {
			errs = append(errs, fmt.Sprintf("rebuild: sky hash %s then %s, the sky is not seeded", a.Hash, a2.Hash))
		}

		// Baked once: zooming and panning draw frames and never rebuild it.
		if _, err := evalRaw(`(function(){ var c = document.querySelector('#stationOverlay canvas'), r = c.getBoundingClientRect();
			c.dispatchEvent(new WheelEvent('wheel', {deltaY: -300, clientX: r.left + r.width / 2, clientY: r.top + r.height / 2, bubbles: true, cancelable: true})); return true; })()`); err != nil {
			errs = append(errs, fmt.Sprintf("zoom: %v", err))
		}
		time.Sleep(1500 * time.Millisecond)
		if z := read(); z.Builds != a2.Builds || z.Hash != a.Hash {
			errs = append(errs, fmt.Sprintf("zoom: the sky was rebuilt or changed while frames ran (builds %d to %d)", a2.Builds, z.Builds))
		}
		fmt.Printf("uicheck: d27 baked once: builds %d before and %d after a zoom and 1.5 s of frames\n", a2.Builds, read().Builds)

		// A resized window rebuilds the sky for the new canvas.
		if err := ensureWindowSizeWH(hwnd, 1100, 700); err != nil {
			errs = append(errs, fmt.Sprintf("resize: %v", err))
		}
		time.Sleep(600 * time.Millisecond)
		b := measure("P-site-1100x700", true, true)
		if b.Builds <= a2.Builds || b.W == a.W {
			errs = append(errs, fmt.Sprintf("resize: the sky did not follow the window (builds %d to %d, width %d to %d)", a2.Builds, b.Builds, a.W, b.W))
		}

		// The light page theme leaves the sky alone.
		key("l")
		time.Sleep(700 * time.Millisecond)
		c := measure("P-site-1100x700-light", true, true)
		if c.Page == startPage {
			errs = append(errs, fmt.Sprintf("light: the page theme stayed %q after L", c.Page))
		}
		if c.Hash != b.Hash {
			errs = append(errs, fmt.Sprintf("light: the sky changed with the page theme (%s then %s)", b.Hash, c.Hash))
		}
		key("p")

		dir := filepath.Join("..", "..", "testdata", "uicheck", "out")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			_ = os.WriteFile(filepath.Join(dir, "d27-"+tag+".txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
		}
		if len(errs) > 0 {
			return fmt.Errorf("d27: %d problem(s):\n%s", len(errs), strings.Join(errs, "\n"))
		}
		fmt.Println("uicheck: d27: six cumulus clouds, one alpha each, no hard step, seeded, baked once, follows the window, page theme leaves the sky alone")
		return nil
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// d42: the drag handles (B7.1, spec 2026-10-28, item 5), through real mouse
// input. Standard, Overall scrollbar off, the frozen example with long lists
// (d42Snapshot: d34Snapshot(true) plus 24 turns for the ticker), with the status\r
// parts, Processes and To Do off so the columns have free height (with every\r
// part on, 1920x1080 is already short of room and every range is 0): at 1280x800 when a handle has room there, else at
// 1920x1080 (at 1280x800 every part sits at its floor and every range is 0),
// with a resize leg to the other of the two sizes (SKIPPED when this screen
// cannot hold it). With the plain example every handle range
// is zero, so a drag would prove nothing: step 1 fails unless some handle has
// at least 12 px of room.
//   1  reset the saved heights through the real binding, apply the payload,
//      read every keyed part and every handle range (R0); no handle is a failure
//   2  pick the handle with the most room; dy is half of it (6 to 60 px)
//   3  drag the handle with the real mouse (dragSelector); the part above
//      changes by dy and the part below by -dy within 2 px, no other keyed part
//      moves more than 1 px, nothing overflows (R1)
//   4  burnmon-dev-view.json has heights_standard with both keys in (0, 1] and
//      no heights_plain; the real binding bdevSections returns the same map
//   5  "reload": the saved map is applied again as a payload; every height
//      equals R1 within 2 px (R2)
//   6  resize to the other size: the saved shares still hold (R3), back at the drag size
//      the heights equal R1 again (R4)
//   7  real clicks on the gear and on Reset heights: the heights equal R0
//      within 2 px (R5), the file has no heights_standard and the page state is null
// The heights are written into the real view file; the wrapper script restores
// it byte for byte afterwards, and the check sends reset_heights on the way out.
// Screenshot d42-drag-<size> after step 3.

// d42Off switches the status parts, Processes and To Do off, so the two columns own the page and have free height to hand over.
var d42Off = map[string]bool{"waits": true, "outcomes": true, "burn": true, "git": true, "agents_map": true, "sessions": true, "processes": true, "todo": true}

var d42Keys = []string{"burn_30min", "vendor_strip", "activity_heatmap", "turn_ticker", "system_chart", "process_groups"}

var d42Column = map[string]string{
	"burn_30min": "burn", "vendor_strip": "burn", "activity_heatmap": "burn", "turn_ticker": "burn",
	"system_chart": "system", "process_groups": "system",
}

const d42ReadJS = `(function(){
  var sel = {burn_30min: '.burnchart', vendor_strip: '#vendorStripPanel', activity_heatmap: '#activityHeatmapRow', turn_ticker: '#turnTickerPanel', system_chart: '.histwrap', process_groups: '#processGroupsPanel'};
  var h = {};
  Object.keys(sel).forEach(function(k){
    var e = document.querySelector(sel[k]);
    h[k] = (e && e.getClientRects().length > 0) ? e.getBoundingClientRect().height : -1;
  });
  return {handles: window.__bdevFitHandles ? window.__bdevFitHandles() : [], h: h,
    state: window.__bdevSections.state().heightsStandard};
})()`

type d42Handle struct {
	Above string `json:"above"`
	Below string `json:"below"`
	Min   int    `json:"min"`
	Max   int    `json:"max"`
}

type d42Read struct {
	Handles []d42Handle        `json:"handles"`
	H       map[string]float64 `json:"h"`
	State   map[string]float64 `json:"state"`
}

func d42Read1() (d42Read, error) {
	var r d42Read
	err := evalInto(d42ReadJS, &r)
	return r, err
}

// d42Snapshot is d34Snapshot(true) plus 24 invented turns in burn.turns, so the
// turn ticker holds more rows than its three-row floor and a handle next to it
// has room to move. Without the turns the ticker cap equals its floor.
func d42Snapshot() ([]byte, error) {
	raw, err := d34Snapshot(true)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	sid, agent := "example-session-01", "claude-code"
	if ss, _ := m["sessions"].([]any); len(ss) > 0 {
		if s0, _ := ss[0].(map[string]any); s0 != nil {
			if v, ok := s0["id"].(string); ok && v != "" {
				sid = v
			}
			if v, ok := s0["agent"].(string); ok && v != "" {
				agent = v
			}
		}
	}
	var turns []map[string]any
	base := time.Now().Add(-2 * time.Minute)
	for k := 1; k <= 24; k++ {
		turns = append(turns, map[string]any{
			"session_id": sid, "turn": 100 - k, "agent": agent,
			"at":    base.Add(-time.Duration(k) * 20 * time.Second).UTC().Format(time.RFC3339),
			"fresh": 1200 + k, "cache_write": 300, "cache_read": 40000, "output": 500,
		})
	}
	burn, _ := m["burn"].(map[string]any)
	if burn == nil {
		burn = map[string]any{}
	}
	burn["turns"] = turns
	m["burn"] = burn
	return json.Marshal(m)
}
func d42ViewFile() (map[string]json.RawMessage, error) {
	lad := os.Getenv("LOCALAPPDATA")
	if lad == "" {
		return nil, fmt.Errorf("LOCALAPPDATA not set")
	}
	raw, err := os.ReadFile(filepath.Join(lad, "burnmon", "burnmon-dev-view.json"))
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	return m, json.Unmarshal(raw, &m)
}

// d42FileHeights polls the view file until pred accepts what it holds (up to 3 s).
func d42FileHeights(pred func(std map[string]float64, plain bool) bool) (map[string]float64, bool, error) {
	var std map[string]float64
	var plain bool
	for i := 0; i < 15; i++ {
		m, err := d42ViewFile()
		if err != nil {
			return nil, false, err
		}
		std, plain = nil, false
		if v, ok := m["heights_standard"]; ok {
			if err := json.Unmarshal(v, &std); err != nil {
				return nil, false, err
			}
		}
		_, plain = m["heights_plain"]
		if pred(std, plain) {
			return std, plain, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return std, plain, nil
}

func d42MaxDiff(a, b d42Read) (string, float64) {
	worst, key := 0.0, ""
	for _, k := range d42Keys {
		if d := math.Abs(a.H[k] - b.H[k]); d > worst {
			worst, key = d, k
		}
	}
	return key, worst
}

func init() {
	checks["d42"] = func(hwnd uintptr, args []string) error {
		fail := func(format string, a ...any) error { return fmt.Errorf("d42: "+format, a...) }
		defer func() {
			_, _ = evalRaw(`(window.bdevSetSections({reset_heights: true}), true)`)
			time.Sleep(400 * time.Millisecond)
			_, _ = evalRaw(`(window.__bdevExitFakeMode && window.__bdevExitFakeMode())`)
			_ = ensureWindowSizeWH(hwnd, 1152, 2048)
		}()
		snap, err := d42Snapshot()
		if err != nil {
			return err
		}
		// 1. clean start through the real binding, then the first size that gives a handle room
		if _, err := evalRaw(`(window.bdevSetSections({reset_heights: true}), true)`); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		base := d29Opts{Status: "ok", Snap: snap, Scrollbar: false, StandardOff: d42Off}
		payload := d29Payload(base)
		sizes := [][2]int32{{1280, 800}, {1920, 1080}}
		var r0 d42Read
		var hdl d42Handle
		best := -1
		dragTag, otherTag := "", ""
		var otherW, otherH int32
		for _, sz := range sizes {
			if err := ensureWindowSizeWH(hwnd, sz[0], sz[1]); err != nil {
				return err
			}
			tag := fmt.Sprintf("%dx%d", sz[0], sz[1])
			if windowCapped {
				fmt.Printf("uicheck: d42: SKIPPED %s, this screen is too small\n", tag)
				continue
			}
			time.Sleep(500 * time.Millisecond)
			if _, err := d34Apply(payload, snap); err != nil {
				return err
			}
			r, err := d42Read1()
			if err != nil {
				return err
			}
			if len(r.Handles) == 0 {
				return fail("there are no drag handles on the page at %s", tag)
			}
			bst, bh := -1, d42Handle{}
			for _, h := range r.Handles {
				room := h.Max
				if -h.Min > room {
					room = -h.Min
				}
				if room > bst {
					bst, bh = room, h
				}
				fmt.Printf("uicheck: d42: %s handle %s / %s range %d to %d px\n", tag, h.Above, h.Below, h.Min, h.Max)
			}
			if bst >= 12 {
				r0, hdl, best, dragTag = r, bh, bst, tag
				o := [2]int32{1600, 900} // a size between the two, so the resize leg moves real room
				otherW, otherH, otherTag = o[0], o[1], fmt.Sprintf("%dx%d", o[0], o[1])
				break
			}
			fmt.Printf("uicheck: d42: %s has no handle with 12 px of room (largest %d px), trying the next size\n", tag, bst)
		}
		if best < 12 {
			return fail("no handle has room at any size: a zero-range handle proves nothing")
		}
		// 2. the drag distance
		dy := math.Max(6, math.Min(60, math.Round(float64(best)/2)))
		if -hdl.Min > hdl.Max {
			dy = -dy
		}
		fmt.Printf("uicheck: d42: dragging %s / %s by %+.0f px; before: %s\n", hdl.Above, hdl.Below, dy, d42Heights(r0))
		// 3. the real drag
		bringToFront(hwnd)
		time.Sleep(200 * time.Millisecond)
		sel := fmt.Sprintf(`.zonebody > .hdl[data-above="%s"][data-below="%s"]`, hdl.Above, hdl.Below)
		if err := dragSelector(hwnd, sel, dy); err != nil {
			return err
		}
		time.Sleep(800 * time.Millisecond)
		r1, err := d42Read1()
		if err != nil {
			return err
		}
		var p1 d34Page
		if err := evalInto(d34ReadJS, &p1); err != nil {
			return err
		}
		if _, err := screenshot(hwnd, "d42-drag-"+dragTag); err != nil {
			return err
		}
		fmt.Printf("uicheck: d42: after the drag: %s\n", d42Heights(r1))
		var bad []string
		if d := r1.H[hdl.Above] - r0.H[hdl.Above]; math.Abs(d-dy) > 2 {
			bad = append(bad, fmt.Sprintf("%s changed by %.1f px, want %+.0f", hdl.Above, d, dy))
		}
		if d := r1.H[hdl.Below] - r0.H[hdl.Below]; math.Abs(d+dy) > 2 {
			bad = append(bad, fmt.Sprintf("%s changed by %.1f px, want %+.0f", hdl.Below, d, -dy))
		}
		for _, k := range d42Keys {
			if k == hdl.Above || k == hdl.Below {
				continue
			}
			if d := math.Abs(r1.H[k] - r0.H[k]); d > 1 {
				bad = append(bad, fmt.Sprintf("%s moved %.1f px but is not part of the drag", k, d))
			}
		}
		if p1.BodyOver > 1 {
			bad = append(bad, fmt.Sprintf("the page overflows by %.1f px", p1.BodyOver))
		}
		if len(bad) > 0 {
			return fail("drag: %s", strings.Join(bad, "; "))
		}
		fmt.Printf("uicheck: d42: PASS drag (%s by %+.0f, %s by %+.0f, others within 1 px, overflow %.1f px)\n", hdl.Above, r1.H[hdl.Above]-r0.H[hdl.Above], hdl.Below, r1.H[hdl.Below]-r0.H[hdl.Below], p1.BodyOver)
		// 4. on disk and through the binding
		file, plain, err := d42FileHeights(func(s map[string]float64, _ bool) bool { return s != nil })
		if err != nil {
			return err
		}
		if file == nil {
			return fail("the view file has no heights_standard after the drag")
		}
		for _, k := range []string{hdl.Above, hdl.Below} {
			if v, ok := file[k]; !ok || !(v > 0 && v <= 1) {
				return fail("heights_standard[%s] = %v, want a value in (0, 1]", k, v)
			}
		}
		if plain {
			return fail("the view file has heights_plain, a Standard drag must not write it")
		}
		if _, err := evalRaw(`(window.__d42 = null, window.bdevSections().then(function(p){ window.__d42 = p; }), true)`); err != nil {
			return err
		}
		var viaGo map[string]float64
		for i := 0; i < 15; i++ {
			var got struct {
				Done bool               `json:"done"`
				H    map[string]float64 `json:"h"`
			}
			if err := evalInto(`(function(){ var p = window.__d42; return {done: !!p, h: p ? (p.heights_standard || null) : null}; })()`, &got); err != nil {
				return err
			}
			if got.Done {
				viaGo = got.H
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if len(viaGo) != len(file) {
			return fail("bdevSections returned %v, the file holds %v", viaGo, file)
		}
		for k, v := range file {
			if math.Abs(viaGo[k]-v) > 1e-9 {
				return fail("bdevSections heights_standard[%s] = %v, the file %v", k, viaGo[k], v)
			}
		}
		fmt.Printf("uicheck: d42: PASS saved (file and bdevSections agree: %v, no heights_plain)\n", file)
		// 5. "reload": the saved map applied again
		withSaved, err := d41Payload(base, map[string]any{"heights_standard": viaGo})
		if err != nil {
			return err
		}
		if _, err := d34Apply(withSaved, snap); err != nil {
			return err
		}
		r2, err := d42Read1()
		if err != nil {
			return err
		}
		if k, d := d42MaxDiff(r1, r2); d > 2 {
			return fail("after the reload %s differs by %.1f px from the dragged height", k, d)
		}
		fmt.Printf("uicheck: d42: PASS reload (%s)\n", d42Heights(r2))
		// 6. resize to the other size and back
		sizeW := func(t string) int32 { var w, h int32; fmt.Sscanf(t, "%dx%d", &w, &h); return w }
		sizeH := func(t string) int32 { var w, h int32; fmt.Sscanf(t, "%dx%d", &w, &h); return h }
		if err := ensureWindowSizeWH(hwnd, otherW, otherH); err != nil {
			return err
		}
		if windowCapped {
			fmt.Printf("uicheck: d42: SKIPPED the %s resize leg, this screen is too small\n", otherTag)
		} else {
			time.Sleep(800 * time.Millisecond)
			r3, err := d42Read1()
			if err != nil {
				return err
			}
			sum := 0.0
			for _, k := range d42Keys {
				if d42Column[k] == d42Column[hdl.Above] && r3.H[k] > 0 {
					sum += r3.H[k]
				}
			}
			share := r3.H[hdl.Above] / sum
			saved := viaGo[hdl.Above]
			ratio := r3.H[hdl.Above] / r3.H[hdl.Below]
			savedRatio := viaGo[hdl.Above] / viaGo[hdl.Below]
			bound := ""
			for _, h := range r3.Handles {
				if h.Above == hdl.Above && h.Below == hdl.Below && (h.Min == 0 || h.Max == 0) {
					bound = "a floor or cap binds (a handle range edge is 0)"
				}
			}
			if hdl.Above == "vendor_strip" || hdl.Above == "process_groups" || hdl.Below == "vendor_strip" || hdl.Below == "process_groups" {
				if bound == "" {
					bound = "a list part is in the pair and its cap can bind"
				}
			}
			okShare := math.Abs(share-saved) <= 0.03
			okRatio := math.Abs(ratio-savedRatio) <= 0.05*savedRatio
			fmt.Printf("uicheck: d42: %s: %s share %.4f (saved %.4f), ratio %.3f (saved %.3f), %s\n", otherTag, hdl.Above, share, saved, ratio, savedRatio, d42Heights(r3))
			if (!okShare || !okRatio) && bound == "" {
				return fail("after the resize the share is %.4f (saved %.4f) and the ratio %.3f (saved %.3f), and no floor or cap binds", share, saved, ratio, savedRatio)
			}
			if okShare && okRatio {
				fmt.Printf("uicheck: d42: PASS resize (share and ratio kept)\n")
			} else {
				fmt.Printf("uicheck: d42: NOTE resize leg is not a pass: %s\n", bound)
			}
			if err := ensureWindowSizeWH(hwnd, sizeW(dragTag), sizeH(dragTag)); err != nil {
				return err
			}
			time.Sleep(800 * time.Millisecond)
			r4, err := d42Read1()
			if err != nil {
				return err
			}
			if k, d := d42MaxDiff(r1, r4); d > 2 {
				return fail("back at %s %s differs by %.1f px from the dragged height", dragTag, k, d)
			}
			fmt.Printf("uicheck: d42: PASS back at %s (%s)\n", dragTag, d42Heights(r4))
		}
		// 7. Reset heights with real clicks
		bringToFront(hwnd)
		time.Sleep(200 * time.Millisecond)
		if err := clickSelector(hwnd, "#secGear"); err != nil {
			return err
		}
		time.Sleep(400 * time.Millisecond)
		if err := clickSelector(hwnd, "#secResetHeights"); err != nil {
			return err
		}
		time.Sleep(800 * time.Millisecond)
		r5, err := d42Read1()
		if err != nil {
			return err
		}
		bringToFront(hwnd)
		_ = pressKey(vkEscape)
		if k, d := d42MaxDiff(r0, r5); d > 2 {
			return fail("after Reset heights %s differs by %.1f px from the default", k, d)
		}
		after, plainAfter, err := d42FileHeights(func(s map[string]float64, _ bool) bool { return s == nil })
		if err != nil {
			return err
		}
		if after != nil || plainAfter {
			return fail("after Reset heights the view file still has heights (standard %v, plain %v)", after, plainAfter)
		}
		if r5.State != nil {
			return fail("after Reset heights the page state still holds %v", r5.State)
		}
		fmt.Printf("uicheck: d42: PASS reset (%s, file and page state clear)\n", d42Heights(r5))
		return nil
	}
}

func d42Heights(r d42Read) string {
	var b []string
	for _, k := range d42Keys {
		b = append(b, fmt.Sprintf("%s %.0f", k, r.H[k]))
	}
	return strings.Join(b, ", ")
}

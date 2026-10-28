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

// d42: the drag handles (B7.1, spec 2026-10-28, item 5 and the iteration 2
// decision), through real mouse input and a real page reload. Standard,
// Overall scrollbar off, the frozen example with long lists (d42Snapshot:
// d34Snapshot(true) plus 24 turns for the ticker), EVERY part on, at
// 1920x1080. A drag may take height from its partner down to the partner's
// floor and then from the Waits, Outcomes and Sessions lists down to their
// three-row floor; a handle with no room even then is hidden (display none).
// This check never switches a part off to make room, and it never skips: a
// screen that cannot hold 1920x1080 or 1800x1050 is a failure, not a note.
//   1  reset the saved heights through the real binding, apply the payload,
//      read every keyed part, the three lists and every handle (R0). Every
//      handle that is shown must have 12 px or more of room somewhere; the
//      handles reported hidden must be hidden in the DOM and the shown ones
//      drawn; at least one handle must be shown
//   2  pick the shown handle with the most room; dy is half of it (6 to 60 px)
//   3  drag the handle with the real mouse (dragSelector): the growing part
//      changes by dy within 2 px; its partner loses what it could give and the
//      host lists lose the rest (partner plus lists add up to dy within 2 px);
//      the other keyed parts of the column stay within 2 px; the other column
//      takes the height the lists gave; nothing overflows (R1)
//   4  burnmon-dev-view.json has heights_standard with the column's keys in
//      (0, 1], host_lists when the lists gave, and no heights_plain; the real
//      binding bdevSections returns the same map
//   5  reload: the page is reloaded for real (evalReload loads the embedded page again; a marker set on
//      window before it must be gone after). The reloaded page loads its own
//      state through bdevSections and that state must equal the file; then the
//      payload is applied with the heights the reloaded page holds. The live
//      sampler parts (Processes, Process groups) are not replaced by the frozen
//      example, so the room can differ from the dragged page and pixels cannot be
//      compared. What survives is proportions: the share of the dragged pair in
//      its column is within 0.03 of the saved share, and the lists give
//      host_lists times their shrink range (or what they have above the floor),
//      within 3 px, measured against the same page's own default layout (R2a)
//   6  resize to 1800x1050: the saved map is unchanged in the file and the
//      page; the share of the dragged part is within 0.03 of the saved share,
//      or every list sits at its floor (says which); back at
//      1920x1080 the heights equal the reloaded layout (R2) within 2 px (R4)
//   7  real clicks on the gear and on Reset heights: the heights equal the default
//      layout of the reloaded page (R2a) within 2 px (R5), the file has no heights_standard and the page state is null
// The heights are written into the real view file; the wrapper script restores
// it byte for byte afterwards, and the check sends reset_heights on the way out.
// Screenshot d42-drag-<size> after step 3.

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
  var lists = {};
  Array.prototype.forEach.call(document.querySelectorAll('#secHost .seclist'), function(e){
    if(e.getClientRects().length > 0 && !e.closest('.sec-off')) lists[(e.closest('[data-sec]') || e).getAttribute('data-sec') || 'list'] = e.getBoundingClientRect().height;
  });
  var ps = document.getElementById('pageScroll'), f = window.__bdevFit || {};
  var drawn = 0;
  Array.prototype.forEach.call(document.querySelectorAll('.zonebody > .hdl'), function(e){ if(e.getClientRects().length > 0 && getComputedStyle(e).display !== 'none') drawn++; });
  return {handles: window.__bdevFitHandles ? window.__bdevFitHandles() : [], drawn: drawn, h: h, lists: lists,
    state: window.__bdevSections.state().heightsStandard, fitScroll: !!f.fitScroll, items: f.items || [],
    bodyOver: document.body.scrollHeight - document.body.clientHeight, pageOver: ps.scrollHeight - ps.clientHeight,
    vw: window.innerWidth, vh: window.innerHeight, psh: ps.clientHeight};
})()`

type d42Handle struct {
	Above  string `json:"above"`
	Below  string `json:"below"`
	Min    int    `json:"min"`
	Max    int    `json:"max"`
	Hidden bool   `json:"hidden"`
}

type d42Item struct {
	ID    string  `json:"id"`
	Kind  string  `json:"kind"`
	H     float64 `json:"h"`
	Floor float64 `json:"floor"`
	Nat   float64 `json:"nat"`
}

type d42Read struct {
	Handles   []d42Handle        `json:"handles"`
	Drawn     int                `json:"drawn"`
	H         map[string]float64 `json:"h"`
	Lists     map[string]float64 `json:"lists"`
	State     map[string]float64 `json:"state"`
	FitScroll bool               `json:"fitScroll"`
	Items     []d42Item          `json:"items"`
	BodyOver  float64            `json:"bodyOver"`
	PageOver  float64            `json:"pageOver"`
	VW        float64            `json:"vw"`
	VH        float64            `json:"vh"`
	PSH       float64            `json:"psh"`
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
	for k, v := range a.Lists {
		if d := math.Abs(v - b.Lists[k]); d > worst {
			worst, key = d, "list "+k
		}
	}
	return key, worst
}

func d42Sum(m map[string]float64) float64 {
	s := 0.0
	for _, v := range m {
		s += v
	}
	return s
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
		// 1. clean start through the real binding, every part on, at the size Wilco uses
		if _, err := evalRaw(`(window.bdevSetSections({reset_heights: true}), true)`); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		base := d29Opts{Status: "ok", Snap: snap, Scrollbar: false}
		payload := d29Payload(base)
		const dragW, dragH, otherW, otherH = 1920, 1080, 1800, 1050
		dragTag, otherTag := fmt.Sprintf("%dx%d", dragW, dragH), fmt.Sprintf("%dx%d", otherW, otherH)
		if err := ensureWindowSizeWH(hwnd, dragW, dragH); err != nil {
			return err
		}
		if windowCapped {
			return fail("this screen cannot hold %s, so the drag cannot be proven here (a skip would hide that)", dragTag)
		}
		time.Sleep(500 * time.Millisecond)
		if _, err := d34Apply(payload, snap); err != nil {
			return err
		}
		r0, err := d42Read1()
		if err != nil {
			return err
		}
		if len(r0.H) == 0 || r0.H["burn_30min"] < 0 || r0.H["vendor_strip"] < 0 || r0.H["activity_heatmap"] < 0 || r0.H["turn_ticker"] < 0 || r0.H["system_chart"] < 0 || r0.H["process_groups"] < 0 {
			return fail("not every part is drawn at %s: %s", dragTag, d42Heights(r0))
		}
		if len(r0.Lists) < 3 {
			return fail("the Waits, Outcomes and Sessions lists are not all drawn (%v)", r0.Lists)
		}
		shown, best := 0, -1
		var hdl d42Handle
		for _, h := range r0.Handles {
			room := h.Max
			if -h.Min > room {
				room = -h.Min
			}
			fmt.Printf("uicheck: d42: %s handle %s / %s range %d to %d px, hidden %v\n", dragTag, h.Above, h.Below, h.Min, h.Max, h.Hidden)
			if h.Hidden {
				if h.Max-h.Min >= 1 {
					return fail("handle %s / %s is hidden but has a range of %d to %d px", h.Above, h.Below, h.Min, h.Max)
				}
				continue
			}
			shown++
			if h.Max-h.Min < 1 {
				return fail("handle %s / %s is drawn as a dead bar (range %d to %d px)", h.Above, h.Below, h.Min, h.Max)
			}
			if room > best {
				best, hdl = room, h
			}
		}
		if r0.Drawn != shown {
			return fail("the page draws %d handles, __bdevFitHandles reports %d shown", r0.Drawn, shown)
		}
		if shown == 0 || best < 12 {
			return fail("with every part on at %s no handle has 12 px of room (shown %d, largest %d px)", dragTag, shown, best)
		}
		fmt.Printf("uicheck: d42: PASS handles at %s with every part on: %d shown, %d hidden, widest room %d px; %s\n", dragTag, shown, len(r0.Handles)-shown, best, d42Heights(r0))
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
		if _, err := screenshot(hwnd, "d42-drag-"+dragTag); err != nil {
			return err
		}
		fmt.Printf("uicheck: d42: after the drag: %s\n", d42Heights(r1))
		grow, part := hdl.Above, hdl.Below
		if dy < 0 {
			grow, part = hdl.Below, hdl.Above
		}
		g := math.Abs(dy)
		dGrow := r1.H[grow] - r0.H[grow]
		dPart := r1.H[part] - r0.H[part]
		dLists := d42Sum(r1.Lists) - d42Sum(r0.Lists)
		var bad []string
		if math.Abs(dGrow-g) > 2 {
			bad = append(bad, fmt.Sprintf("%s grew by %.1f px, want %.0f", grow, dGrow, g))
		}
		if dPart > 1 || dPart < -g-2 {
			bad = append(bad, fmt.Sprintf("%s changed by %.1f px, want between %.0f and 0", part, dPart, -g))
		}
		if math.Abs((-dPart)+(-dLists)-g) > 2 {
			bad = append(bad, fmt.Sprintf("partner %.1f px and lists %.1f px do not add up to %.0f", -dPart, -dLists, g))
		}
		if dLists > 1 {
			bad = append(bad, fmt.Sprintf("the lists grew by %.1f px during a drag", dLists))
		}
		otherCol := 0.0
		for _, k := range d42Keys {
			if k == hdl.Above || k == hdl.Below {
				continue
			}
			d := r1.H[k] - r0.H[k]
			if d42Column[k] == d42Column[hdl.Above] {
				if math.Abs(d) > 2 {
					bad = append(bad, fmt.Sprintf("%s moved %.1f px but is not part of the drag", k, d))
				}
			} else {
				otherCol += d
			}
		}
		if math.Abs(otherCol-(-dLists)) > 3 {
			bad = append(bad, fmt.Sprintf("the other column changed by %.1f px but the lists gave %.1f px", otherCol, -dLists))
		}
		if r1.BodyOver > 1 {
			bad = append(bad, fmt.Sprintf("the page overflows by %.1f px", r1.BodyOver))
		}
		if len(bad) > 0 {
			return fail("drag: %s", strings.Join(bad, "; "))
		}
		fmt.Printf("uicheck: d42: PASS drag at %s (%s +%.1f, %s %.1f, lists %.1f, other column %+.1f, overflow %.1f px)\n", dragTag, grow, dGrow, part, dPart, dLists, otherCol, r1.BodyOver)
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
		if dLists < -1 {
			if v, ok := file["host_lists"]; !ok || !(v > 0 && v <= 1) {
				return fail("the lists gave %.1f px but heights_standard[host_lists] = %v", -dLists, v)
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
		// 5. a real reload of the page; the reloaded page must load the saved state by itself
		if _, err := evalRaw(`(window.__d42Marker = 'before the reload', true)`); err != nil {
			return err
		}
		if err := evalReload(); err != nil {
			return err
		}
		var reloaded struct {
			Marker string             `json:"marker"`
			Ready  bool               `json:"ready"`
			State  map[string]float64 `json:"state"`
		}
		deadline := time.Now().Add(20 * time.Second)
		time.Sleep(600 * time.Millisecond)
		var lastErr error
		for time.Now().Before(deadline) {
			reloaded.Ready = false
			lastErr = evalInto(`(function(){ var s = window.__bdevSections ? window.__bdevSections.state() : null; return {marker: window.__d42Marker || '', ready: !!s, state: s ? s.heightsStandard : null}; })()`, &reloaded)
			if lastErr == nil && reloaded.Ready && reloaded.State != nil {
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
		if reloaded.Marker != "" {
			return fail("the page was not reloaded: window.__d42Marker is still %q", reloaded.Marker)
		}
		if !reloaded.Ready {
			var diag string
			_ = evalInto(`(location.href.slice(0, 40) + ' | ' + document.documentElement.outerHTML.length + ' | ' + typeof window.__bdevSections)`, &diag)
			return fail("the page did not come back after location.reload within 20 s (last eval error %v, page says %q)", lastErr, diag)
		}
		if reloaded.State == nil {
			return fail("the reloaded page loaded no heights_standard from the view file")
		}
		if len(reloaded.State) != len(file) {
			return fail("the reloaded page holds %v, the file %v", reloaded.State, file)
		}
		for k, v := range file {
			if math.Abs(reloaded.State[k]-v) > 1e-9 {
				return fail("the reloaded page holds heights[%s] = %v, the file %v", k, reloaded.State[k], v)
			}
		}
		fileAfter, _, err := d42FileHeights(func(s map[string]float64, _ bool) bool { return s != nil })
		if err != nil {
			return err
		}
		for k, v := range file {
			if math.Abs(fileAfter[k]-v) > 1e-9 {
				return fail("the file changed by the reload: %v, was %v", fileAfter, file)
			}
		}
		// The Processes and Process groups parts show the live sampler, which the frozen
		// example does not replace, so their rows (and with them the room the columns
		// have) can differ between the page before and the page after the reload. The
		// reloaded page is therefore compared with its own default layout (r2a, no
		// saved heights), not with the pixels of the page that was dragged.
		noHeights, err := d41Payload(base, map[string]any{})
		if err != nil {
			return err
		}
		if _, err := d34Apply(noHeights, snap); err != nil {
			return err
		}
		r2a, err := d42Read1()
		if err != nil {
			return err
		}
		withSaved, err := d41Payload(base, map[string]any{"heights_standard": reloaded.State})
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
		// What survives a reload is the saved proportions, not pixels: the share of
		// each part in its column and the fraction of the lists' shrink range the
		// drag took. So the checks are on those, against this page's own default.
		colSum := func(r d42Read) float64 {
			s := 0.0
			for _, k := range d42Keys {
				if d42Column[k] == d42Column[hdl.Above] && r.H[k] > 0 {
					s += r.H[k]
				}
			}
			return s
		}
		savedColSum := 0.0
		for _, k := range d42Keys {
			if d42Column[k] == d42Column[hdl.Above] {
				savedColSum += file[k]
			}
		}
		var badReload []string
		for _, k := range []string{hdl.Above, hdl.Below} {
			want, got := file[k]/savedColSum, r2.H[k]/colSum(r2)
			if math.Abs(got-want) > 0.03 {
				badReload = append(badReload, fmt.Sprintf("%s has share %.4f of its column, saved %.4f", k, got, want))
			}
		}
		rng, availDefault := 0.0, 0.0
		for _, it := range r2a.Items {
			if it.Kind == "seclist" {
				rng += it.Nat - it.Floor
				availDefault += math.Max(0, it.H-it.Floor)
			}
		}
		wantLists := 0.0
		if f, ok := file["host_lists"]; ok {
			wantLists = math.Min(f*rng, availDefault)
		}
		gotLists := d42Sum(r2a.Lists) - d42Sum(r2.Lists)
		if math.Abs(gotLists-wantLists) > 3 {
			badReload = append(badReload, fmt.Sprintf("the lists gave %.1f px relative to the default, host_lists %.4f of a %.0f px range with %.0f px available gives %.1f px", gotLists, file["host_lists"], rng, availDefault, wantLists))
		}
		if gotGrow := r2.H[grow] - r2a.H[grow]; gotGrow < 3 {
			badReload = append(badReload, fmt.Sprintf("%s is only %.1f px taller than its default, the saved layout is not visible", grow, gotGrow))
		}
		if len(badReload) > 0 {
			return fail("after the reload: %s (default: %s; saved layout: %s; saved %v; list items default %s, saved %s; viewport %.0fx%.0f)", strings.Join(badReload, "; "), d42Heights(r2a), d42Heights(r2), reloaded.State, d42Items(r2a), d42Items(r2), r2.VW, r2.VH)
		}
		_, drift := d42MaxDiff(r1, r2)
		fmt.Printf("uicheck: d42: PASS reload (page reloaded, state read from the reloaded page equals the file; shares kept within 0.03, lists gave %.1f px (expected %.1f); %s; largest difference to the dragged page %.1f px)\n", gotLists, wantLists, d42Heights(r2), drift)
		// 6. resize to a smaller size and back
		if err := ensureWindowSizeWH(hwnd, otherW, otherH); err != nil {
			return fail("the resize leg could not run: %v", err)
		}
		if windowCapped {
			return fail("the resize leg could not run: this screen cannot hold %s", otherTag)
		}
		time.Sleep(800 * time.Millisecond)
		r3, err := d42Read1()
		if err != nil {
			return err
		}
		var onDisk map[string]float64
		if onDisk, _, err = d42FileHeights(func(s map[string]float64, _ bool) bool { return s != nil }); err != nil {
			return err
		}
		for k, v := range file {
			if math.Abs(onDisk[k]-v) > 1e-9 || math.Abs(r3.State[k]-v) > 1e-9 {
				return fail("the resize changed the saved heights: file %v, page %v, saved %v", onDisk, r3.State, file)
			}
		}
		sum := 0.0
		for _, k := range d42Keys {
			if d42Column[k] == d42Column[hdl.Above] && r3.H[k] > 0 {
				sum += r3.H[k]
			}
		}
		share, savedSum := r3.H[hdl.Above]/sum, 0.0
		for _, k := range d42Keys {
			if d42Column[k] == d42Column[hdl.Above] {
				savedSum += file[k]
			}
		}
		saved := file[hdl.Above] / savedSum
		listsAtFloor, nLists := true, 0
		for _, it := range r3.Items {
			if it.Kind == "seclist" {
				nLists++
				if it.H > it.Floor+1 {
					listsAtFloor = false
				}
			}
		}
		fmt.Printf("uicheck: d42: %s: %s share %.4f (saved %.4f), %s\n", otherTag, hdl.Above, share, saved, d42Heights(r3))
		switch {
		case math.Abs(share-saved) <= 0.03:
			fmt.Printf("uicheck: d42: PASS resize to %s (share kept within 0.03)\n", otherTag)
		case nLists > 0 && listsAtFloor:
			fmt.Printf("uicheck: d42: PASS resize to %s (share %.4f differs from %.4f because every list sits at its floor and the saved draw cannot be honored in full)\n", otherTag, share, saved)
		default:
			return fail("after the resize the share is %.4f (saved %.4f) and the lists are not at their floor", share, saved)
		}
		if err := ensureWindowSizeWH(hwnd, dragW, dragH); err != nil {
			return err
		}
		time.Sleep(800 * time.Millisecond)
		r4, err := d42Read1()
		if err != nil {
			return err
		}
		if k, d := d42MaxDiff(r2, r4); d > 2 {
			return fail("back at %s %s differs by %.1f px from the reloaded layout", dragTag, k, d)
		}
		fmt.Printf("uicheck: d42: PASS back at %s (%s)\n", dragTag, d42Heights(r4))
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
		if k, d := d42MaxDiff(r2a, r5); d > 2 {
			return fail("after Reset heights %s differs by %.1f px from the default layout (default %s, after Reset %s)", k, d, d42Heights(r2a), d42Heights(r5))
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
	var l []string
	for _, k := range []string{"waits", "outcomes", "sessions"} {
		if v, ok := r.Lists[k]; ok {
			l = append(l, fmt.Sprintf("%s %.0f", k, v))
		}
	}
	return strings.Join(b, ", ") + " | lists " + strings.Join(l, ", ")
}

// d42Items prints the fit items (id h/floor/nat) of the host lists, for a failure message.
func d42Items(r d42Read) string {
	var b []string
	for _, it := range r.Items {
		if it.Kind == "seclist" {
			b = append(b, fmt.Sprintf("%s %.0f/%.0f/%.0f", it.ID, it.H, it.Floor, it.Nat))
		}
	}
	return strings.Join(b, ", ")
}

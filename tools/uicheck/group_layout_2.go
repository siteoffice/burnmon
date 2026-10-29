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

// layout/d42: moved from check_d42.go.

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
//   8  save shares that put activity_heatmap well above its floor (and
//      host_lists 1) through bdevSetSections (the binding a drag saves with),
//      apply them, drag the
//      activity_heatmap / turn_ticker handle up: the partner shrinks by the
//      drag within 2 px and the lists move under 2 px, then step 3's checks
// The heights are written into the run's own view file (scripts\uicheck-dev.ps1
// makes a temp settings folder); applyStart starts each case with none saved.
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

// d42TurnsBase is the fixed time the invented turns count back from: two
// minutes before the newest time in testdata\snapshot_example.json.
const d42TurnsBase = "2026-10-23T09:58:00Z"

// d42Snapshot is d34Snapshot(true) plus 24 invented turns in burn.turns, so the
// turn ticker holds more rows than its three-row floor and a handle next to it
// has room to move. Without the turns the ticker cap equals its floor. The turn
// times count back from d42TurnsBase, never the clock, so two builds are the
// same bytes (the page shows the turns it is given; it does not filter by age).
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
	base, err := time.Parse(time.RFC3339, d42TurnsBase)
	if err != nil {
		return nil, err
	}
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
	sd, err := devSettingsDir()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(sd, "burnmon-dev-view.json"))
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

// The pass/fail decisions of d42 below are pure functions over what the live
// case measured (a measure struct in, an error out), so check_d42_test.go can
// feed each one a stubbed measure that must fail and prove every branch fires.
// The thresholds and messages are the ones the live case always used.

func d42Fail(format string, a ...any) error { return fmt.Errorf("d42: "+format, a...) }

// d42StartMeasure is step 1: the first read (R0) at the drag size.
type d42StartMeasure struct {
	Tag string
	R   d42Read
}

// d42Pick is the handle step 2 drags: the shown one with the most room.
type d42Pick struct {
	Handle d42Handle
	Room   int
	Shown  int
}

func d42CheckStart(m d42StartMeasure) (d42Pick, error) {
	r0 := m.R
	if len(r0.H) == 0 || r0.H["burn_30min"] < 0 || r0.H["vendor_strip"] < 0 || r0.H["activity_heatmap"] < 0 || r0.H["turn_ticker"] < 0 || r0.H["system_chart"] < 0 || r0.H["process_groups"] < 0 {
		return d42Pick{}, d42Fail("not every part is drawn at %s: %s", m.Tag, d42Heights(r0))
	}
	if len(r0.Lists) < 3 {
		return d42Pick{}, d42Fail("the Waits, Outcomes and Sessions lists are not all drawn (%v)", r0.Lists)
	}
	p := d42Pick{Room: -1}
	for _, h := range r0.Handles {
		room := h.Max
		if -h.Min > room {
			room = -h.Min
		}
		if h.Hidden {
			if h.Max-h.Min >= 1 {
				return d42Pick{}, d42Fail("handle %s / %s is hidden but has a range of %d to %d px", h.Above, h.Below, h.Min, h.Max)
			}
			continue
		}
		p.Shown++
		if h.Max-h.Min < 1 {
			return d42Pick{}, d42Fail("handle %s / %s is drawn as a dead bar (range %d to %d px)", h.Above, h.Below, h.Min, h.Max)
		}
		if room > p.Room {
			p.Room, p.Handle = room, h
		}
	}
	if r0.Drawn != p.Shown {
		return d42Pick{}, d42Fail("the page draws %d handles, __bdevFitHandles reports %d shown", r0.Drawn, p.Shown)
	}
	if p.Shown == 0 || p.Room < 12 {
		return d42Pick{}, d42Fail("with every part on at %s no handle has 12 px of room (shown %d, largest %d px)", m.Tag, p.Shown, p.Room)
	}
	return p, nil
}

// d42DragMeasure is step 3: the handle, the drag distance and the reads
// before (R0) and after (R1) the drag.
type d42DragMeasure struct {
	Hdl           d42Handle
	Dy            float64
	Before, After d42Read
}

// d42DragDelta is what the drag changed: the part that grows, its partner,
// the host lists and the other column.
type d42DragDelta struct {
	Grow, Part                     string
	DGrow, DPart, DLists, OtherCol float64
}

func (m d42DragMeasure) deltas() d42DragDelta {
	d := d42DragDelta{Grow: m.Hdl.Above, Part: m.Hdl.Below}
	if m.Dy < 0 {
		d.Grow, d.Part = m.Hdl.Below, m.Hdl.Above
	}
	d.DGrow = m.After.H[d.Grow] - m.Before.H[d.Grow]
	d.DPart = m.After.H[d.Part] - m.Before.H[d.Part]
	d.DLists = d42Sum(m.After.Lists) - d42Sum(m.Before.Lists)
	for _, k := range d42Keys {
		if k == m.Hdl.Above || k == m.Hdl.Below || d42Column[k] == d42Column[m.Hdl.Above] {
			continue
		}
		d.OtherCol += m.After.H[k] - m.Before.H[k]
	}
	return d
}

func d42CheckDrag(m d42DragMeasure) error {
	d := m.deltas()
	g := math.Abs(m.Dy)
	var bad []string
	if math.Abs(d.DGrow-g) > 2 {
		bad = append(bad, fmt.Sprintf("%s grew by %.1f px, want %.0f", d.Grow, d.DGrow, g))
	}
	if d.DPart > 1 || d.DPart < -g-2 {
		bad = append(bad, fmt.Sprintf("%s changed by %.1f px, want between %.0f and 0", d.Part, d.DPart, -g))
	}
	if math.Abs((-d.DPart)+(-d.DLists)-g) > 2 {
		bad = append(bad, fmt.Sprintf("partner %.1f px and lists %.1f px do not add up to %.0f", -d.DPart, -d.DLists, g))
	}
	if d.DLists > 1 {
		bad = append(bad, fmt.Sprintf("the lists grew by %.1f px during a drag", d.DLists))
	}
	for _, k := range d42Keys {
		if k == m.Hdl.Above || k == m.Hdl.Below || d42Column[k] != d42Column[m.Hdl.Above] {
			continue
		}
		if v := m.After.H[k] - m.Before.H[k]; math.Abs(v) > 2 {
			bad = append(bad, fmt.Sprintf("%s moved %.1f px but is not part of the drag", k, v))
		}
	}
	if math.Abs(d.OtherCol-(-d.DLists)) > 3 {
		bad = append(bad, fmt.Sprintf("the other column changed by %.1f px but the lists gave %.1f px", d.OtherCol, -d.DLists))
	}
	if m.After.BodyOver > 1 {
		bad = append(bad, fmt.Sprintf("the page overflows by %.1f px", m.After.BodyOver))
	}
	if len(bad) > 0 {
		return d42Fail("drag: %s", strings.Join(bad, "; "))
	}
	return nil
}

// d42SavedMeasure is step 4: what the view file holds after the drag.
type d42SavedMeasure struct {
	File   map[string]float64
	Plain  bool
	Hdl    d42Handle
	DLists float64
}

func d42CheckSaved(m d42SavedMeasure) error {
	if m.File == nil {
		return d42Fail("the view file has no heights_standard after the drag")
	}
	for _, k := range []string{m.Hdl.Above, m.Hdl.Below} {
		if v, ok := m.File[k]; !ok || !(v > 0 && v <= 1) {
			return d42Fail("heights_standard[%s] = %v, want a value in (0, 1]", k, v)
		}
	}
	if m.DLists < -1 {
		if v, ok := m.File["host_lists"]; !ok || !(v > 0 && v <= 1) {
			return d42Fail("the lists gave %.1f px but heights_standard[host_lists] = %v", -m.DLists, v)
		}
	}
	if m.Plain {
		return d42Fail("the view file has heights_plain, a Standard drag must not write it")
	}
	return nil
}

// d42CheckBinding is step 4: bdevSections must return what the file holds.
func d42CheckBinding(viaGo, file map[string]float64) error {
	if len(viaGo) != len(file) {
		return d42Fail("bdevSections returned %v, the file holds %v", viaGo, file)
	}
	for k, v := range file {
		if math.Abs(viaGo[k]-v) > 1e-9 {
			return d42Fail("bdevSections heights_standard[%s] = %v, the file %v", k, viaGo[k], v)
		}
	}
	return nil
}

// d42ReloadMeasure is step 5: what the page says after the real reload. Diag
// is read only when the page did not come back.
type d42ReloadMeasure struct {
	Marker  string
	Ready   bool
	State   map[string]float64
	File    map[string]float64
	LastErr error
	Diag    string
}

func d42CheckReloaded(m d42ReloadMeasure) error {
	if m.Marker != "" {
		return d42Fail("the page was not reloaded: window.__d42Marker is still %q", m.Marker)
	}
	if !m.Ready {
		return d42Fail("the page did not come back after location.reload within 20 s (last eval error %v, page says %q)", m.LastErr, m.Diag)
	}
	if m.State == nil {
		return d42Fail("the reloaded page loaded no heights_standard from the view file")
	}
	if len(m.State) != len(m.File) {
		return d42Fail("the reloaded page holds %v, the file %v", m.State, m.File)
	}
	for k, v := range m.File {
		if math.Abs(m.State[k]-v) > 1e-9 {
			return d42Fail("the reloaded page holds heights[%s] = %v, the file %v", k, m.State[k], v)
		}
	}
	return nil
}

// d42CheckFileKept is step 5: the reload must not change the view file.
func d42CheckFileKept(after, file map[string]float64) error {
	for k, v := range file {
		if math.Abs(after[k]-v) > 1e-9 {
			return d42Fail("the file changed by the reload: %v, was %v", after, file)
		}
	}
	return nil
}

// d42LayoutMeasure is step 5 after the reload: the reloaded page's own
// default layout (R2a, no saved heights) and the saved layout (R2).
type d42LayoutMeasure struct {
	Hdl            d42Handle
	Grow           string
	File, State    map[string]float64
	Default, Saved d42Read
}

// d42ListDraw is what the lists gave against the default, and what the saved
// host_lists says they should give.
type d42ListDraw struct{ Got, Want float64 }

func d42CheckReloadLayout(m d42LayoutMeasure) (d42ListDraw, error) {
	col := d42Column[m.Hdl.Above]
	colSum := func(r d42Read) float64 {
		s := 0.0
		for _, k := range d42Keys {
			if d42Column[k] == col && r.H[k] > 0 {
				s += r.H[k]
			}
		}
		return s
	}
	savedColSum := 0.0
	for _, k := range d42Keys {
		if d42Column[k] == col {
			savedColSum += m.File[k]
		}
	}
	var bad []string
	for _, k := range []string{m.Hdl.Above, m.Hdl.Below} {
		want, got := m.File[k]/savedColSum, m.Saved.H[k]/colSum(m.Saved)
		if math.Abs(got-want) > 0.03 {
			bad = append(bad, fmt.Sprintf("%s has share %.4f of its column, saved %.4f", k, got, want))
		}
	}
	rng, availDefault := 0.0, 0.0
	for _, it := range m.Default.Items {
		if it.Kind == "seclist" {
			rng += it.Nat - it.Floor
			availDefault += math.Max(0, it.H-it.Floor)
		}
	}
	var lists d42ListDraw
	if f, ok := m.File["host_lists"]; ok {
		lists.Want = math.Min(f*rng, availDefault)
	}
	lists.Got = d42Sum(m.Default.Lists) - d42Sum(m.Saved.Lists)
	if math.Abs(lists.Got-lists.Want) > 3 {
		bad = append(bad, fmt.Sprintf("the lists gave %.1f px relative to the default, host_lists %.4f of a %.0f px range with %.0f px available gives %.1f px", lists.Got, m.File["host_lists"], rng, availDefault, lists.Want))
	}
	if gotGrow := m.Saved.H[m.Grow] - m.Default.H[m.Grow]; gotGrow < 3 {
		bad = append(bad, fmt.Sprintf("%s is only %.1f px taller than its default, the saved layout is not visible", m.Grow, gotGrow))
	}
	if len(bad) > 0 {
		return lists, d42Fail("after the reload: %s (default: %s; saved layout: %s; saved %v; list items default %s, saved %s; viewport %.0fx%.0f)", strings.Join(bad, "; "), d42Heights(m.Default), d42Heights(m.Saved), m.State, d42Items(m.Default), d42Items(m.Saved), m.Saved.VW, m.Saved.VH)
	}
	return lists, nil
}

// d42ResizeMeasure is step 6: the read at the smaller size (R3) and the file.
type d42ResizeMeasure struct {
	Hdl          d42Handle
	File, OnDisk map[string]float64
	R            d42Read
}

// d42Resize is the dragged part's share at the smaller size against the saved
// share, and whether a difference is excused because every list is at its floor.
type d42Resize struct {
	Share, Saved float64
	ByFloor      bool
}

func d42CheckResize(m d42ResizeMeasure) (d42Resize, error) {
	for k, v := range m.File {
		if math.Abs(m.OnDisk[k]-v) > 1e-9 || math.Abs(m.R.State[k]-v) > 1e-9 {
			return d42Resize{}, d42Fail("the resize changed the saved heights: file %v, page %v, saved %v", m.OnDisk, m.R.State, m.File)
		}
	}
	col := d42Column[m.Hdl.Above]
	sum, savedSum := 0.0, 0.0
	for _, k := range d42Keys {
		if d42Column[k] != col {
			continue
		}
		if m.R.H[k] > 0 {
			sum += m.R.H[k]
		}
		savedSum += m.File[k]
	}
	res := d42Resize{Share: m.R.H[m.Hdl.Above] / sum, Saved: m.File[m.Hdl.Above] / savedSum}
	listsAtFloor, nLists := true, 0
	for _, it := range m.R.Items {
		if it.Kind == "seclist" {
			nLists++
			if it.H > it.Floor+1 {
				listsAtFloor = false
			}
		}
	}
	switch {
	case math.Abs(res.Share-res.Saved) <= 0.03:
		return res, nil
	case nLists > 0 && listsAtFloor:
		res.ByFloor = true
		return res, nil
	}
	return res, d42Fail("after the resize the share is %.4f (saved %.4f) and the lists are not at their floor", res.Share, res.Saved)
}

// d42CheckBack is step 6: back at the drag size the heights equal the
// reloaded layout (R2) within 2 px.
func d42CheckBack(reloaded, back d42Read, tag string) error {
	if k, d := d42MaxDiff(reloaded, back); d > 2 {
		return d42Fail("back at %s %s differs by %.1f px from the reloaded layout", tag, k, d)
	}
	return nil
}

// d42CheckResetLayout is step 7: after Reset heights the heights equal the
// default layout of the reloaded page (R2a) within 2 px.
func d42CheckResetLayout(def, after d42Read) error {
	if k, d := d42MaxDiff(def, after); d > 2 {
		return d42Fail("after Reset heights %s differs by %.1f px from the default layout (default %s, after Reset %s)", k, d, d42Heights(def), d42Heights(after))
	}
	return nil
}

// d42CheckResetCleared is step 7: the file and the page state hold no heights.
func d42CheckResetCleared(file map[string]float64, plain bool, state map[string]float64) error {
	if file != nil || plain {
		return d42Fail("after Reset heights the view file still has heights (standard %v, plain %v)", file, plain)
	}
	if state != nil {
		return d42Fail("after Reset heights the page state still holds %v", state)
	}
	return nil
}

// Step 8 (the partner leg): at the default layout the partner of the step 3
// handle sits at its floor, so step 3 passes with every px taken from the host
// lists. Step 8 saves shares that put activity_heatmap well above its floor and
// drags the activity_heatmap / turn_ticker handle up (turn_ticker grows): the
// partner must give the whole drag and the lists nothing.
const (
	d42PartnerAbove = "activity_heatmap"
	d42PartnerBelow = "turn_ticker"
	d42PartnerItem  = "activityHeatmapRow"
)

// d42PartnerShares is the heights_standard map step 8 saves: the partner takes
// half of the burn column, the other keyed parts share the rest in proportion
// to their heights in r, rounded to 4 places as the page rounds a drag. The
// live sampler parts can leave every keyed part of the column at its floor with
// the spare room in the host lists, so host_lists 1 lets the lists give that
// room to the columns first; without it the shares have nothing to move.
func d42PartnerShares(r d42Read) map[string]float64 {
	other := 0.0
	for _, k := range d42Keys {
		if d42Column[k] == d42Column[d42PartnerAbove] && k != d42PartnerAbove && r.H[k] > 0 {
			other += r.H[k]
		}
	}
	out := map[string]float64{d42PartnerAbove: 0.5, "host_lists": 1}
	for _, k := range d42Keys {
		if d42Column[k] == d42Column[d42PartnerAbove] && k != d42PartnerAbove && r.H[k] > 0 && other > 0 {
			out[k] = math.Round(0.5*r.H[k]/other*10000) / 10000
		}
	}
	return out
}

// d42CheckSharesSaved is step 8: bdevSetSections answered "ok" (res) and the
// view file holds every share that was sent.
func d42CheckSharesSaved(res string, sent, file map[string]float64) error {
	if res != "ok" {
		return d42Fail("partner leg: bdevSetSections did not save the shares within 10 s (%q)", res)
	}
	for k, v := range sent {
		if f, ok := file[k]; !ok || math.Abs(f-v) > 1e-9 {
			return d42Fail("partner leg: the view file holds %v, sent %v", file, sent)
		}
	}
	return nil
}

// d42CheckPartnerRoom is step 8 before the drag: the handle is drawn and the
// partner has room above its floor. dy is negative (a drag up), at most 40 px
// and 2 px short of the room, so the partner can give all of it.
func d42CheckPartnerRoom(r d42Read) (d42Handle, float64, error) {
	var hdl d42Handle
	found := false
	for _, h := range r.Handles {
		if h.Above == d42PartnerAbove && h.Below == d42PartnerBelow && !h.Hidden {
			hdl, found = h, true
		}
	}
	if !found {
		return hdl, 0, d42Fail("partner leg: the %s / %s handle is not drawn with the saved shares (%v)", d42PartnerAbove, d42PartnerBelow, r.Handles)
	}
	floor := -1.0
	for _, it := range r.Items {
		if it.ID == d42PartnerItem {
			floor = it.Floor
		}
	}
	if floor < 0 {
		return hdl, 0, d42Fail("partner leg: the fit pass has no item %s", d42PartnerItem)
	}
	room := r.H[d42PartnerAbove] - floor
	if room < 8 {
		return hdl, 0, d42Fail("partner leg: the saved shares left %s %.1f px above its floor of %.0f px, want 8 or more (%s)", d42PartnerAbove, room, floor, d42Heights(r))
	}
	return hdl, -math.Min(40, math.Floor(room-2)), nil
}

// d42CheckPartnerDrag is step 8 after the drag: the partner shrank by the drag
// within 2 px (the tolerance step 3 uses) and the lists moved under 2 px.
func d42CheckPartnerDrag(m d42DragMeasure) error {
	d := m.deltas()
	g := math.Abs(m.Dy)
	var bad []string
	if math.Abs(-d.DPart-g) > 2 {
		bad = append(bad, fmt.Sprintf("%s gave %.1f px, want %.0f (the whole drag)", d.Part, -d.DPart, g))
	}
	if math.Abs(d.DLists) >= 2 {
		bad = append(bad, fmt.Sprintf("the lists changed by %.1f px, want under 2 (the partner had room)", d.DLists))
	}
	if len(bad) > 0 {
		return d42Fail("partner drag: %s", strings.Join(bad, "; "))
	}
	return nil
}

// d42ViaBinding asks the real binding bdevSections for heights_standard.
func d42ViaBinding() (map[string]float64, error) {
	if _, err := evalRaw(`(window.__d42 = null, window.bdevSections().then(function(p){ window.__d42 = p; }), true)`); err != nil {
		return nil, err
	}
	for i := 0; i < 15; i++ {
		var got struct {
			Done bool               `json:"done"`
			H    map[string]float64 `json:"h"`
		}
		if err := evalInto(`(function(){ var p = window.__d42; return {done: !!p, h: p ? (p.heights_standard || null) : null}; })()`, &got); err != nil {
			return nil, err
		}
		if got.Done {
			return got.H, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil, nil
}

func caseD42(hwnd uintptr, args []string) error {
	snap, err := d42Snapshot()
	if err != nil {
		return err
	}
	// 1. clean start (applyStart: no saved heights, every part on), at the size Wilco uses
	base := d29Opts{Status: "ok", Snap: snap, Scrollbar: false}
	payload := d29Payload(base)
	const dragW, dragH, otherW, otherH = 1920, 1080, 1800, 1050
	dragTag, otherTag := fmt.Sprintf("%dx%d", dragW, dragH), fmt.Sprintf("%dx%d", otherW, otherH)
	if err := ensureWindowSizeWH(hwnd, dragW, dragH); err != nil {
		return err
	}
	if windowCapped {
		return d42Fail("this screen cannot hold %s, so the drag cannot be proven here (a skip would hide that)", dragTag)
	}
	time.Sleep(500 * time.Millisecond)
	if _, err := d34Apply(payload, snap); err != nil {
		return err
	}
	r0, err := d42Read1()
	if err != nil {
		return err
	}
	for _, h := range r0.Handles {
		fmt.Printf("uicheck: d42: %s handle %s / %s range %d to %d px, hidden %v\n", dragTag, h.Above, h.Below, h.Min, h.Max, h.Hidden)
	}
	pick, err := d42CheckStart(d42StartMeasure{Tag: dragTag, R: r0})
	if err != nil {
		return err
	}
	hdl, best, shown := pick.Handle, pick.Room, pick.Shown
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
	drag := d42DragMeasure{Hdl: hdl, Dy: dy, Before: r0, After: r1}
	if err := d42CheckDrag(drag); err != nil {
		return err
	}
	dd := drag.deltas()
	grow := dd.Grow
	fmt.Printf("uicheck: d42: PASS drag at %s (%s +%.1f, %s %.1f, lists %.1f, other column %+.1f, overflow %.1f px)\n", dragTag, grow, dd.DGrow, dd.Part, dd.DPart, dd.DLists, dd.OtherCol, r1.BodyOver)
	// 4. on disk and through the binding
	file, plain, err := d42FileHeights(func(s map[string]float64, _ bool) bool { return s != nil })
	if err != nil {
		return err
	}
	if err := d42CheckSaved(d42SavedMeasure{File: file, Plain: plain, Hdl: hdl, DLists: dd.DLists}); err != nil {
		return err
	}
	viaGo, err := d42ViaBinding()
	if err != nil {
		return err
	}
	if err := d42CheckBinding(viaGo, file); err != nil {
		return err
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
	var diag string
	if reloaded.Marker == "" && !reloaded.Ready {
		_ = evalInto(`(location.href.slice(0, 40) + ' | ' + document.documentElement.outerHTML.length + ' | ' + typeof window.__bdevSections)`, &diag)
	}
	if err := d42CheckReloaded(d42ReloadMeasure{Marker: reloaded.Marker, Ready: reloaded.Ready, State: reloaded.State, File: file, LastErr: lastErr, Diag: diag}); err != nil {
		return err
	}
	fileAfter, _, err := d42FileHeights(func(s map[string]float64, _ bool) bool { return s != nil })
	if err != nil {
		return err
	}
	if err := d42CheckFileKept(fileAfter, file); err != nil {
		return err
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
	lists, err := d42CheckReloadLayout(d42LayoutMeasure{Hdl: hdl, Grow: grow, File: file, State: reloaded.State, Default: r2a, Saved: r2})
	if err != nil {
		return err
	}
	_, drift := d42MaxDiff(r1, r2)
	fmt.Printf("uicheck: d42: PASS reload (page reloaded, state read from the reloaded page equals the file; shares kept within 0.03, lists gave %.1f px (expected %.1f); %s; largest difference to the dragged page %.1f px)\n", lists.Got, lists.Want, d42Heights(r2), drift)
	// 6. resize to a smaller size and back
	if err := ensureWindowSizeWH(hwnd, otherW, otherH); err != nil {
		return d42Fail("the resize leg could not run: %v", err)
	}
	if windowCapped {
		return d42Fail("the resize leg could not run: this screen cannot hold %s", otherTag)
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
	rs, err := d42CheckResize(d42ResizeMeasure{Hdl: hdl, File: file, OnDisk: onDisk, R: r3})
	if err != nil {
		return err
	}
	fmt.Printf("uicheck: d42: %s: %s share %.4f (saved %.4f), %s\n", otherTag, hdl.Above, rs.Share, rs.Saved, d42Heights(r3))
	if rs.ByFloor {
		fmt.Printf("uicheck: d42: PASS resize to %s (share %.4f differs from %.4f because every list sits at its floor and the saved draw cannot be honored in full)\n", otherTag, rs.Share, rs.Saved)
	} else {
		fmt.Printf("uicheck: d42: PASS resize to %s (share kept within 0.03)\n", otherTag)
	}
	if err := ensureWindowSizeWH(hwnd, dragW, dragH); err != nil {
		return err
	}
	time.Sleep(800 * time.Millisecond)
	r4, err := d42Read1()
	if err != nil {
		return err
	}
	if err := d42CheckBack(r2, r4, dragTag); err != nil {
		return err
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
	if err := d42CheckResetLayout(r2a, r5); err != nil {
		return err
	}
	after, plainAfter, err := d42FileHeights(func(s map[string]float64, _ bool) bool { return s == nil })
	if err != nil {
		return err
	}
	if err := d42CheckResetCleared(after, plainAfter, r5.State); err != nil {
		return err
	}
	fmt.Printf("uicheck: d42: PASS reset (%s, file and page state clear)\n", d42Heights(r5))
	// 8. the partner leg: shares saved through the binding the page saves a drag with
	shares := d42PartnerShares(r5)
	change, err := json.Marshal(map[string]any{"heights_mode": "standard", "heights": shares})
	if err != nil {
		return err
	}
	if _, err := evalRaw(`(window.__d42Set = '', window.bdevSetSections(` + string(change) + `).then(function(){ window.__d42Set = 'ok'; }, function(e){ window.__d42Set = 'error ' + e; }), true)`); err != nil {
		return err
	}
	setRes := ""
	for i := 0; i < 50 && setRes == ""; i++ {
		time.Sleep(200 * time.Millisecond)
		if setRes, err = evalString(`window.__d42Set || ''`); err != nil {
			return err
		}
	}
	file8, _, err := d42FileHeights(func(s map[string]float64, _ bool) bool { return s != nil })
	if err != nil {
		return err
	}
	if err := d42CheckSharesSaved(setRes, shares, file8); err != nil {
		return err
	}
	via8, err := d42ViaBinding()
	if err != nil {
		return err
	}
	if err := d42CheckBinding(via8, file8); err != nil {
		return err
	}
	withShares, err := d41Payload(base, map[string]any{"heights_standard": via8})
	if err != nil {
		return err
	}
	if _, err := d34Apply(withShares, snap); err != nil {
		return err
	}
	r6, err := d42Read1()
	if err != nil {
		return err
	}
	hdl8, dy8, err := d42CheckPartnerRoom(r6)
	if err != nil {
		return err
	}
	fmt.Printf("uicheck: d42: partner leg: saved %v; dragging %s / %s by %+.0f px; before: %s\n", via8, hdl8.Above, hdl8.Below, dy8, d42Heights(r6))
	bringToFront(hwnd)
	time.Sleep(200 * time.Millisecond)
	sel8 := fmt.Sprintf(`.zonebody > .hdl[data-above="%s"][data-below="%s"]`, hdl8.Above, hdl8.Below)
	if err := dragSelector(hwnd, sel8, dy8); err != nil {
		return err
	}
	time.Sleep(800 * time.Millisecond)
	r7, err := d42Read1()
	if err != nil {
		return err
	}
	drag8 := d42DragMeasure{Hdl: hdl8, Dy: dy8, Before: r6, After: r7}
	if err := d42CheckPartnerDrag(drag8); err != nil {
		return err
	}
	if err := d42CheckDrag(drag8); err != nil {
		return err
	}
	d8 := drag8.deltas()
	fmt.Printf("uicheck: d42: PASS partner drag (%s +%.1f, %s %.1f, lists %.1f, other column %+.1f px)\n", d8.Grow, d8.DGrow, d8.Part, d8.DPart, d8.DLists, d8.OtherCol)
	return nil
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

// layout/d43: moved from check_d43.go.

// d43: the gear Controls text spans the gear (B7.1, spec 2026-10-28, item 3).
// At 1280x800 and 1920x1080 (a size the screen cannot hold prints SKIPPED and
// is not a pass), Standard, with invented "needs ..." lines for all five
// controls, a real click opens the gear. Asserted: the Controls explanation
// and every non-empty "needs ..." line are within 2 px of the width of the
// History block (#histAdmin, which spans the gear grid); each is wider than
// 320 px when the History block is; the dialog is narrower than the window
// minus 20 px; no text is clipped (scrollWidth within 1 px of clientWidth).
// Screenshot d43-gear-<size>; Esc closes the gear.

const d43ReadJS = `(function(){
  var w = function(e){ return e ? e.getBoundingClientRect().width : 0; };
  var clip = function(e){ return e.scrollWidth <= e.clientWidth + 1; };
  var hist = document.getElementById('histAdmin');
  var texts = [], e0 = document.querySelector('#secControls > .ctlwhy');
  if(e0) texts.push({name: 'explanation', w: w(e0), clip: clip(e0)});
  Array.prototype.forEach.call(document.querySelectorAll('#secControls .ctlwhy[data-ctl-why]'), function(e){
    if(!(e.textContent || '').trim()) return;
    texts.push({name: e.getAttribute('data-ctl-why'), w: w(e), clip: clip(e)});
  });
  return {open: document.getElementById('secSettings').classList.contains('open'), hist: w(hist), dialog: w(document.getElementById('secSettings')),
    win: window.innerWidth, texts: texts};
})()`

type d43Text struct {
	Name string  `json:"name"`
	W    float64 `json:"w"`
	Clip bool    `json:"clip"`
}

type d43Read struct {
	Open   bool      `json:"open"`
	Hist   float64   `json:"hist"`
	Dialog float64   `json:"dialog"`
	Win    float64   `json:"win"`
	Texts  []d43Text `json:"texts"`
}

func caseD43(hwnd uintptr, args []string) error {
	extra := map[string]any{"controls": map[string]any{
		"on": map[string]bool{}, "ready": map[string]bool{},
		"why": map[string]string{
			"stop": "needs claude_path in burnmon.json", "attach": "needs agent_view in burnmon.json",
			"approve": "needs python on PATH", "send_back": "needs python on PATH", "close_sprint": "needs foreman_root",
		},
		"verdicts": map[string]any{}, "foreman": true}}
	payload, err := d41Payload(d29Opts{Status: "ok"}, extra)
	if err != nil {
		return err
	}
	var fails []string
	ran := 0
	for _, sz := range [][2]int32{{1280, 800}, {1920, 1080}} {
		tag := fmt.Sprintf("%dx%d", sz[0], sz[1])
		if err := ensureWindowSizeWH(hwnd, sz[0], sz[1]); err != nil {
			return err
		}
		if windowCapped {
			fmt.Printf("uicheck: d43: SKIPPED %s, this screen is too small\n", tag)
			continue
		}
		time.Sleep(500 * time.Millisecond)
		if _, err := evalRaw(`(document.getElementById('secSettings').classList.remove('open'), window.__bdevSections.apply(` + payload + `), true)`); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		if err := clickSelector(hwnd, "#secGear"); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		var r d43Read
		err := evalInto(d43ReadJS, &r)
		_, shotErr := screenshot(hwnd, "d43-gear-"+tag)
		bringToFront(hwnd)
		_ = pressKey(vkEscape)
		time.Sleep(300 * time.Millisecond)
		if err != nil {
			return err
		}
		if shotErr != nil {
			return shotErr
		}
		var bad []string
		if !r.Open {
			bad = append(bad, "the gear did not open")
		}
		if len(r.Texts) != 6 {
			bad = append(bad, fmt.Sprintf("%d text elements, want 6 (explanation and five needs lines)", len(r.Texts)))
		}
		var ws []string
		for _, t := range r.Texts {
			ws = append(ws, fmt.Sprintf("%s %.0f", t.Name, t.W))
			if t.W < r.Hist-2 || t.W > r.Hist+2 {
				bad = append(bad, fmt.Sprintf("%s is %.1f px wide, the History block %.1f px", t.Name, t.W, r.Hist))
			}
			if r.Hist > 320 && t.W <= 320 {
				bad = append(bad, fmt.Sprintf("%s is %.1f px wide, not over 320", t.Name, t.W))
			}
			if !t.Clip {
				bad = append(bad, t.Name+" is clipped")
			}
		}
		if r.Dialog >= r.Win-20 {
			bad = append(bad, fmt.Sprintf("dialog %.0f px is not narrower than window %.0f minus 20", r.Dialog, r.Win))
		}
		ran++
		if len(bad) == 0 {
			fmt.Printf("uicheck: d43: PASS %s (grid width %.0f px, dialog %.0f px, text widths: %s)\n", tag, r.Hist, r.Dialog, strings.Join(ws, ", "))
		} else {
			msg := fmt.Sprintf("%s: %s", tag, strings.Join(bad, "; "))
			fmt.Println("uicheck: d43: FAIL " + msg)
			fails = append(fails, msg)
		}
	}
	if err := ensureWindowSizeWH(hwnd, 1152, 2048); err != nil {
		return err
	}
	if len(fails) > 0 {
		return fmt.Errorf("d43: %d case(s) failed, first: %s", len(fails), fails[0])
	}
	if ran == 0 {
		return fmt.Errorf("d43: no size ran")
	}
	return nil
}

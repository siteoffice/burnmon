package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// d29: the sections frame (agent control plan, row R8) with B2's parts: four
// groups of 15 parts, one tick per part per mode, and the Overall scrollbar
// switch. Fed lane A's frozen example (testdata\snapshot_example.json, the
// file docs\snapshot_schema.md describes) it proves:
//   - Standard with every part on shows all of them, drops none and the page
//     scrolls at the window the screen grants (1152x2048 here);
//   - each part switched off alone hides only that part; the older zones follow
//     their parts; with all 15 off the host is empty and nothing breaks;
//   - Plain shows its own list (the five R8 panels), in plain words, and fits
//     the window; its list can take a burn-zone part; at 1024x480 with all
//     on it hides nothing and the page scrolls (B7);
//   - the Plain toggle's text flips between "Plain: off" and "Plain: on";
//   - the gear lists four groups of parts, Overall scrollbar on top, the
//     "Station starts as" pair below; choosing a Station theme calls the
//     binding and does not switch the Station that is open;
//   - a stale or missing snapshot shows the payload's snapshot_line in the
//     header and the notice (never "not running") and draws no old numbers;
//   - the toggle, the scrollbar and a tick saved through the real bindings land
//     where they belong. Survival of a restart is the Go test of
//     internal\sections, not a real restart of the exe.
//
// The page is driven through window.__bdevSections.apply, which draws without
// saving. The persistence step at the end uses the real bindings and puts back
// what it found. Screenshots: d29-standard, d29-plain, d29-plain-small,
// d29-gear, d29-not-running.
var d29Parts = []string{
	"waits", "outcomes", "burn", "git", "agents_map", "sessions",
	"burn_30min", "vendor_strip", "activity", "heatmap", "turn_ticker",
	"system", "process_groups", "processes",
	"todo",
}

var d29PlainDefault = map[string]bool{"waits": true, "outcomes": true, "burn": true, "agents_map": true, "sessions": true}

type d29Opts struct {
	Plain, Scrollbar bool
	StandardOff      map[string]bool
	PlainOn          map[string]bool
	PlainAll         bool
	PlainOnly        map[string]bool
	Status           string
	Snap             json.RawMessage
	StationTheme     string
}

func d29Payload(o d29Opts) string {
	standard, plainSet := map[string]bool{}, map[string]bool{}
	for _, id := range d29Parts {
		standard[id] = !o.StandardOff[id]
		plainSet[id] = o.PlainAll || d29PlainDefault[id] || o.PlainOn[id]
		if o.PlainOnly != nil {
			plainSet[id] = o.PlainOnly[id]
		}
	}
	current := standard
	if o.Plain {
		current = plainSet
	}
	reading := map[string]any{"status": o.Status, "age_seconds": 7}
	if o.Snap != nil {
		reading["snapshot"] = o.Snap
	}
	// The line Go sends for an unknown writer state (B7.3 sentence table),
	// with the age of 7 s used above.
	line := map[string]any{"text": "snapshot 7 s old", "warn": false}
	switch o.Status {
	case "stale":
		line = map[string]any{"text": "No process has written the snapshot for 7 s.", "warn": true}
	case "missing":
		line = map[string]any{"text": "No snapshot file yet.", "warn": true}
	}
	theme := o.StationTheme
	if theme == "" {
		theme = "space"
	}
	part := func(id, label string) map[string]string { return map[string]string{"id": id, "label": label} }
	groups := []map[string]any{
		{"id": "status", "label": "Status", "parts": []map[string]string{
			part("waits", "Waits on you"), part("outcomes", "Outcomes"), part("burn", "Burn"),
			part("git", "Git"), part("agents_map", "Agents map"), part("sessions", "Sessions")}},
		{"id": "sessions_30min", "label": "Sessions last 30 min", "parts": []map[string]string{
			part("burn_30min", "Burn last 30 min"), part("vendor_strip", "Vendor strip"),
			part("activity", "Activity"), part("heatmap", "Heatmap harness x minute"),
			part("turn_ticker", "Turn ticker")}},
		{"id": "processes_30min", "label": "Processes last 30 min", "parts": []map[string]string{
			part("system", "System"), part("process_groups", "Process groups (with memory, disks and network)"),
			part("processes", "Processes")}},
		{"id": "todo", "label": "Microsoft To Do", "parts": []map[string]string{part("todo", "To Do")}},
	}
	b, _ := json.Marshal(map[string]any{
		"plain_mode": o.Plain, "overall_scrollbar": o.Scrollbar,
		"mode":  map[bool]string{true: "plain", false: "standard"}[o.Plain],
		"parts": current, "parts_standard": standard, "parts_plain": plainSet,
		"groups": groups, "station_theme": theme, "reading": reading,
		"writer": map[string]any{"state": ""}, "snapshot_line": line,
	})
	return string(b)
}

type d29Page struct {
	Parts        []string `json:"parts"`
	TodoOn       bool     `json:"todoOn"`
	Plain        bool     `json:"plain"`
	Toggle       string   `json:"toggle"`
	ToggleOn     bool     `json:"toggleOn"`
	BodyStandard bool     `json:"bodyStandard"`
	BurnZone     bool     `json:"burnZone"`
	SystemZone   bool     `json:"systemZone"`
	MainShown    bool     `json:"mainShown"`
	Header       string   `json:"header"`
	Notice       string   `json:"notice"`
	Text         string   `json:"text"`
	Scrolls      bool     `json:"scrolls"`
	OverflowY    string   `json:"overflowY"`
	HostOver     float64  `json:"hostOver"`
	BodyOver     float64  `json:"bodyOver"`
	AgentsSlot   bool     `json:"agentsSlot"`
	StripClip    float64  `json:"stripClip"`
	StripShrunk  bool     `json:"stripShrunk"`
	StripRows    int      `json:"stripRows"`
	StripWhole   int      `json:"stripWhole"`
	GearOpen     bool     `json:"gearOpen"`
	Ticks        []string `json:"ticks"`
	Headings     []string `json:"headings"`
	LabelRows    int      `json:"labelRows"`
	Checkboxes   int      `json:"checkboxes"`
	ScrollFirst  bool     `json:"scrollFirst"`
	ScrollTick   bool     `json:"scrollTick"`
	Radio        string   `json:"radio"`
}

// shown lists the parts on the page, in d29Parts order. To Do counts when its
// panel lacks .sec-off, since uicheck runs with To Do disabled.
func (p d29Page) shown() []string {
	var ids []string
	for _, id := range d29Parts {
		if id == "todo" {
			if p.TodoOn {
				ids = append(ids, id)
			}
			continue
		}
		for _, s := range p.Parts {
			if s == id {
				ids = append(ids, id)
				break
			}
		}
	}
	return ids
}

const d29ReadJS = `(function(){
  var disp = function(e){ return !!e && e.getClientRects().length > 0; };
  var q = function(s){ return document.querySelector(s); };
  var ps = document.getElementById('pageScroll'), host = document.getElementById('secHost');
  var strip = document.getElementById('vendorStripPanel'), tg = document.getElementById('plainToggle');
  var ticks = [];
  Array.prototype.forEach.call(document.querySelectorAll('#secSettingsCols input[data-sw]'), function(i){
    if(i.checked) ticks.push(i.getAttribute('data-mode') + ':' + i.getAttribute('data-sw'));
  });
  var seen = {};
  Array.prototype.forEach.call(document.querySelectorAll('[data-part]'), function(e){
    if(disp(e)) seen[e.getAttribute('data-part')] = true;
  });
  var sb = document.getElementById('secScrollbar'), firstH = q('#secSettingsCols h4'), radio = q('input[name="secStationStart"]:checked');
  return {
    parts: Object.keys(seen),
    todoOn: !document.getElementById('todoPanel').classList.contains('sec-off'),
    plain: !!window.__bdevSections.state().plain,
    toggle: tg.textContent, toggleOn: tg.classList.contains('on'),
    bodyStandard: document.body.classList.contains('scroll-on'),
    burnZone: disp(q('.zone.burn')), systemZone: disp(q('.zone.system')), mainShown: disp(q('main')),
    header: document.getElementById('hdrSnapAge').textContent,
    notice: document.getElementById('secNotice').textContent,
    text: document.getElementById('secPanels').textContent,
    scrolls: ps.scrollHeight > ps.clientHeight + 1, overflowY: getComputedStyle(ps).overflowY,
    hostOver: host.scrollHeight - host.clientHeight, bodyOver: document.body.scrollHeight - document.body.clientHeight,
    agentsSlot: !!document.getElementById('secAgentsSlot'),
    stripClip: strip ? strip.scrollHeight - strip.clientHeight : 0,
    stripShrunk: !!(window.__bdevFit && window.__bdevFit.items || []).filter(function(it){ return it.id === 'vendorStripPanel' && it.h < it.nat; }).length,
    stripRows: strip ? strip.querySelectorAll('tbody tr').length : 0,
    stripWhole: strip ? Array.prototype.filter.call(strip.querySelectorAll('tbody tr'), function(r){ return r.getBoundingClientRect().bottom <= strip.getBoundingClientRect().bottom + 1; }).length : 0,
    gearOpen: document.getElementById('secSettings').classList.contains('open'),
    ticks: ticks,
    headings: Array.prototype.map.call(document.querySelectorAll('#secSettingsCols h4'), function(h){ return h.textContent; }),
    labelRows: document.querySelectorAll('#secSettingsCols .setlbl').length,
    checkboxes: document.querySelectorAll('#secSettingsCols input[data-sw]').length,
    scrollFirst: !!sb && !!firstH && !!(sb.compareDocumentPosition(firstH) & Node.DOCUMENT_POSITION_FOLLOWING),
    scrollTick: !!sb && sb.checked,
    radio: radio ? radio.value : ''
  };
})()`

func d29Apply(payload string) (d29Page, error) {
	var p d29Page
	if _, err := evalRaw(`window.__bdevSections.apply(` + payload + `)`); err != nil {
		return p, err
	}
	return d29Read()
}

func d29Read() (d29Page, error) {
	var p d29Page
	time.Sleep(500 * time.Millisecond)
	err := evalInto(d29ReadJS, &p)
	return p, err
}

func d29Eval(js string) error {
	_, err := evalRaw(js)
	time.Sleep(300 * time.Millisecond)
	return err
}

func d29Fail(format string, args ...any) error { return fmt.Errorf("d29: "+format, args...) }

// d29StripCheck: the vendor strip's last row must not be cut by its own
// max-height cap (R8). Since B7 the fit pass may shrink it as a list toward its
// floor, a table's header plus two rows, also with the scrollbar on (the
// legacy block is one window tall); the rows below the cut then clip, as the
// comment above #vendorStripPanel in page.html accepts. So a cut is only fine
// when the fit pass shrank the strip, and even then two rows stay whole.
func d29StripCheck(p d29Page) error {
	if !p.StripShrunk {
		if p.StripClip > 1 {
			return d29Fail("standard: the vendor strip is clipped by %.0f px", p.StripClip)
		}
		return nil
	}
	want := 2
	if p.StripRows < want {
		want = p.StripRows
	}
	if p.StripWhole < want {
		return d29Fail("standard: the fit pass shrank the vendor strip below its floor: %d of %d rows whole, want %d", p.StripWhole, p.StripRows, want)
	}
	return nil
}

func caseD29(hwnd uintptr, args []string) error {
	snap, err := readFixture("snapshot_example.json")
	if err != nil {
		return fmt.Errorf("read the frozen example (uicheck runs from the tools folder): %w", err)
	}
	if err := ensureWindowSizeWH(hwnd, 1152, 2048); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)
	std := func(off map[string]bool) d29Opts {
		return d29Opts{Scrollbar: true, StandardOff: off, Status: "ok", Snap: snap}
	}
	wantAll := strings.Join(d29Parts, ",")

	// Standard, every part on, scrollbar on: all shown, none dropped, the page scrolls.
	p, err := d29Apply(d29Payload(std(nil)))
	if err != nil {
		return err
	}
	if got := strings.Join(p.shown(), ","); got != wantAll {
		return d29Fail("standard shows %v, want all 15", p.shown())
	}
	if p.Plain || p.Toggle != "Plain: off" || p.ToggleOn || !p.BodyStandard || !p.BurnZone || !p.SystemZone {
		return d29Fail("standard shape wrong: %+v", p)
	}
	if !p.Scrolls || p.OverflowY != "auto" {
		return d29Fail("standard page does not scroll (scrolls %v, overflow-y %s)", p.Scrolls, p.OverflowY)
	}
	if p.BodyOver > 1 {
		return d29Fail("standard: the body itself overflows by %.0f px", p.BodyOver)
	}
	if !strings.Contains(p.Header, "snapshot 7 s old") {
		return d29Fail("header %q does not show the snapshot age", p.Header)
	}
	for _, want := range []string{"project", "API price equivalent", "Plan limits", "Not built yet", "G4", "120"} {
		if !strings.Contains(p.Text, want) {
			return d29Fail("panels do not show %q from the example", want)
		}
	}
	if !p.AgentsSlot {
		return d29Fail("agents map has no slot for R7")
	}
	if err := d29StripCheck(p); err != nil {
		return err
	}
	if _, err := screenshot(hwnd, "d29-standard"); err != nil {
		return err
	}

	// One part off at a time: only that part goes.
	for _, id := range d29Parts {
		p, err := d29Apply(d29Payload(std(map[string]bool{id: true})))
		if err != nil {
			return err
		}
		var want []string
		for _, x := range d29Parts {
			if x != id {
				want = append(want, x)
			}
		}
		if got := strings.Join(p.shown(), ","); got != strings.Join(want, ",") {
			return d29Fail("switching %s off left parts %v", id, p.shown())
		}
	}
	// The older zones follow their parts.
	burnOff := map[string]bool{"burn_30min": true, "vendor_strip": true, "activity": true, "heatmap": true, "turn_ticker": true}
	if p, err = d29Apply(d29Payload(std(burnOff))); err != nil {
		return err
	}
	if p.BurnZone || !p.SystemZone {
		return d29Fail("five burn-zone parts off: burn zone %v, system zone %v", p.BurnZone, p.SystemZone)
	}
	sysOff := map[string]bool{"system": true, "process_groups": true}
	if p, err = d29Apply(d29Payload(std(sysOff))); err != nil {
		return err
	}
	if p.SystemZone || !p.BurnZone {
		return d29Fail("system and process_groups off: burn zone %v, system zone %v", p.BurnZone, p.SystemZone)
	}

	// Every part off: an empty host, no zones, no error.
	allOff := map[string]bool{}
	for _, id := range d29Parts {
		allOff[id] = true
	}
	if p, err = d29Apply(d29Payload(std(allOff))); err != nil {
		return d29Fail("all 15 parts off raised: %v", err)
	}
	if len(p.shown()) != 0 || p.MainShown || p.BurnZone || p.SystemZone {
		return d29Fail("all parts off: shown %v, main %v", p.shown(), p.MainShown)
	}

	// Plain, scrollbar off: the Plain list, plain words, no older zones, fits the window.
	plain := func(o d29Opts) d29Opts {
		o.Plain, o.Status, o.Snap = true, "ok", snap
		return o
	}
	if p, err = d29Apply(d29Payload(plain(d29Opts{}))); err != nil {
		return err
	}
	if got := strings.Join(p.shown(), ","); got != "waits,outcomes,burn,agents_map,sessions" {
		return d29Fail("plain shows %v", p.shown())
	}
	if !p.Plain || p.Toggle != "Plain: on" || !p.ToggleOn || p.BodyStandard || p.BurnZone || p.SystemZone {
		return d29Fail("plain shape wrong: %+v", p)
	}
	if p.OverflowY != "hidden" {
		return d29Fail("plain page can scroll (overflow-y %s)", p.OverflowY)
	}
	if p.HostOver > 1 {
		return d29Fail("plain at this size: host overflow %.0f px", p.HostOver)
	}
	for _, jargon := range []string{"tokens/min", "API price equiv."} {
		if strings.Contains(p.Text, jargon) {
			return d29Fail("plain mode still shows %q", jargon)
		}
	}
	if !strings.Contains(p.Text, "waiting for you") {
		return d29Fail("plain mode did not put the state in plain words")
	}
	if _, err := screenshot(hwnd, "d29-plain"); err != nil {
		return err
	}

	// A burn-zone part in the Plain list shows the zone with only those parts.
	if p, err = d29Apply(d29Payload(plain(d29Opts{PlainOn: map[string]bool{"burn_30min": true, "turn_ticker": true}}))); err != nil {
		return err
	}
	if got := strings.Join(p.shown(), ","); got != "waits,outcomes,burn,agents_map,sessions,burn_30min,turn_ticker" || !p.BurnZone || p.SystemZone {
		return d29Fail("plain with two burn parts: shown %v, burn zone %v, system zone %v", p.shown(), p.BurnZone, p.SystemZone)
	}

	// B7: everything on in a small window hides nothing. Parts shrink to their floors and the page scrolls.
	if err := ensureWindowSizeWH(hwnd, 1024, 480); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)
	if p, err = d29Apply(d29Payload(plain(d29Opts{PlainAll: true}))); err != nil {
		return err
	}
	if len(p.shown()) < len(d29Parts) {
		return d29Fail("plain at 1024x480 with all parts hid some: shown %v", p.shown())
	}
	if p.BodyOver > 1 || p.OverflowY != "auto" {
		return d29Fail("plain at 1024x480: body overflow %.0f px, overflow-y %s (want the page to scroll)", p.BodyOver, p.OverflowY)
	}
	if _, err := screenshot(hwnd, "d29-plain-small"); err != nil {
		return err
	}
	// Standard at the same size drops nothing either: it scrolls.
	if p, err = d29Apply(d29Payload(std(nil))); err != nil {
		return err
	}
	if strings.Join(p.shown(), ",") != wantAll || !p.Scrolls {
		return d29Fail("standard at 1024x480: shown %v, scrolls %v", p.shown(), p.Scrolls)
	}
	if err := ensureWindowSizeWH(hwnd, 1152, 2048); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)

	// The toggle's text flips with each press.
	if _, err = d29Apply(d29Payload(std(nil))); err != nil {
		return err
	}
	for i, want := range []string{"Plain: on", "Plain: off", "Plain: on"} {
		if err := d29Eval(`document.getElementById('plainToggle').click()`); err != nil {
			return err
		}
		if p, err = d29Read(); err != nil {
			return err
		}
		if p.Toggle != want || p.ToggleOn != (want == "Plain: on") {
			return d29Fail("toggle press %d reads %q (on %v), want %q", i+1, p.Toggle, p.ToggleOn, want)
		}
	}

	// The gear: four groups, Overall scrollbar on top, ticks from the payload, the Station pair.
	if p, err = d29Apply(d29Payload(std(map[string]bool{"git": true}))); err != nil {
		return err
	}
	if err := d29Eval(`document.getElementById('secGear').click()`); err != nil {
		return err
	}
	if p, err = d29Read(); err != nil {
		return err
	}
	wantHeads := "Status,Sessions last 30 min,Processes last 30 min,Microsoft To Do"
	if !p.GearOpen || strings.Join(p.Headings, ",") != wantHeads {
		return d29Fail("gear: open %v, group headings %v", p.GearOpen, p.Headings)
	}
	if p.LabelRows != 15 || p.Checkboxes != 30 {
		return d29Fail("gear: %d part rows and %d checkboxes, want 15 and 30", p.LabelRows, p.Checkboxes)
	}
	if !p.ScrollFirst || !p.ScrollTick {
		return d29Fail("gear: Overall scrollbar first %v, ticked %v", p.ScrollFirst, p.ScrollTick)
	}
	if len(p.Ticks) != 19 {
		return d29Fail("gear: %d ticks, want 19 (14 standard without git, 5 plain)", len(p.Ticks))
	}
	for _, t := range p.Ticks {
		if t == "standard:git" || t == "plain:git" || t == "plain:processes" {
			return d29Fail("gear ticked %s, which is off in the payload", t)
		}
	}
	if p.Radio != "space" {
		return d29Fail("gear: Station starts as %q, want space", p.Radio)
	}
	siteOpts := std(nil)
	siteOpts.StationTheme = "site"
	if p, err = d29Apply(d29Payload(siteOpts)); err != nil {
		return err
	}
	if p.Radio != "site" {
		return d29Fail("gear: payload says site, radio reads %q", p.Radio)
	}
	if _, err := screenshot(hwnd, "d29-gear"); err != nil {
		return err
	}
	// Choosing a Station theme calls the binding and leaves the open Station alone.
	if _, err = d29Apply(d29Payload(std(nil))); err != nil {
		return err
	}
	if err := d29Eval(`(function(){
  window.__d29Real = window.bdevSetStationTheme; window.__d29Calls = []; window.__d29Start = window.BM_STATION_THEME;
  window.bdevSetStationTheme = function(v){ window.__d29Calls.push(v); return Promise.resolve(); };
  document.querySelector('input[name="secStationStart"][value="site"]').click();
  return 1;
})()`); err != nil {
		return err
	}
	calls, cerr := evalString(`JSON.stringify(window.__d29Calls)`)
	same, serr := evalString(`String(window.BM_STATION_THEME === window.__d29Start)`)
	_ = d29Eval(`(window.bdevSetStationTheme = window.__d29Real, 1)`)
	if cerr != nil || serr != nil {
		return d29Fail("radio step could not read back: %v %v", cerr, serr)
	}
	if calls != `["site"]` || same != "true" {
		return d29Fail("radio: binding calls %s, Station theme unchanged %s, want [\"site\"] and true", calls, same)
	}
	if err := d29Eval(`document.getElementById('secGear').click()`); err != nil {
		return err
	}

	// Stale and missing: the header and the notice carry Go's line, never
	// "not running", and no old numbers are drawn.
	lines := map[string]string{"stale": "No process has written the snapshot for 7 s.", "missing": "No snapshot file yet."}
	for _, status := range []string{"stale", "missing"} {
		p, err = d29Apply(d29Payload(d29Opts{Plain: true, Status: status}))
		if err != nil {
			return err
		}
		want := lines[status]
		if p.Header != want || !strings.Contains(p.Notice, want) {
			return d29Fail("%s: header %q notice %q do not carry the line %q", status, p.Header, p.Notice, want)
		}
		if strings.Contains(p.Header, "not running") || strings.Contains(p.Notice, "not running") {
			return d29Fail("%s: header %q notice %q still say not running", status, p.Header, p.Notice)
		}
		if strings.Contains(p.Text, "tokens") && strings.Contains(p.Text, "120") {
			return d29Fail("%s: old numbers are still drawn", status)
		}
	}
	if _, err := screenshot(hwnd, "d29-not-running"); err != nil {
		return err
	}

	return d29Persist()
}

type d29Saved struct {
	Plain     bool            `json:"plain_mode"`
	Scrollbar bool            `json:"overall_scrollbar"`
	Standard  map[string]bool `json:"parts_standard"`
	PlainSet  map[string]bool `json:"parts_plain"`
	Parts     map[string]bool `json:"parts"`
}

// d29Persist drives the real Go side: the toggle, one part in each mode and the
// scrollbar through the bindings, and one tick through the gear's own checkbox.
// Each must land where it belongs only. The next case's applyStart resets them.
func d29Persist() error {
	read := func() (d29Saved, error) {
		var st d29Saved
		if _, err := evalRaw(`(window.__d29 = null, window.bdevSections().then(function(p){ window.__d29 = JSON.stringify(p); }), 1)`); err != nil {
			return st, err
		}
		for i := 0; i < 20; i++ {
			time.Sleep(150 * time.Millisecond)
			s, err := evalString(`window.__d29 || ''`)
			if err != nil {
				return st, err
			}
			if s != "" {
				return st, json.Unmarshal([]byte(s), &st)
			}
		}
		return st, fmt.Errorf("bdevSections never answered")
	}
	send := func(change string) error {
		_, err := evalRaw(`(window.bdevSetSections(` + change + `), 1)`)
		time.Sleep(400 * time.Millisecond)
		return err
	}
	// Start from Standard: git on in Standard and off in Plain, turn_ticker on in Standard and off in Plain.
	for _, c := range []string{
		`{plain:false}`,
		`{mode:'standard',section:'git',on:true}`, `{mode:'plain',section:'git',on:false}`,
		`{mode:'standard',section:'turn_ticker',on:true}`, `{mode:'plain',section:'turn_ticker',on:false}`,
		`{mode:'standard',section:'burn',on:true}`, `{mode:'plain',section:'burn',on:true}`,
		`{scrollbar:true}`,
	} {
		if err := send(c); err != nil {
			return err
		}
	}
	// The toggle.
	if err := send(`{plain:true}`); err != nil {
		return err
	}
	st, err := read()
	if err != nil {
		return err
	}
	if !st.Plain || st.Parts["git"] || !st.Standard["git"] {
		return d29Fail("toggle saved wrong: %+v", st)
	}
	// One part in Plain only.
	if err := send(`{mode:'plain',section:'turn_ticker',on:true}`); err != nil {
		return err
	}
	if st, err = read(); err != nil {
		return err
	}
	if !st.PlainSet["turn_ticker"] || !st.Standard["turn_ticker"] || !st.Parts["turn_ticker"] {
		return d29Fail("plain turn_ticker switch saved wrong: %+v", st)
	}
	// One part in Standard only, with the page in Plain: the current list does not move.
	if err := send(`{mode:'standard',section:'burn',on:false}`); err != nil {
		return err
	}
	if st, err = read(); err != nil {
		return err
	}
	if st.Standard["burn"] || !st.PlainSet["burn"] || !st.Parts["burn"] {
		return d29Fail("standard burn switch saved wrong: %+v", st)
	}
	// The scrollbar switch saves alone.
	before := st
	if err := send(`{scrollbar:false}`); err != nil {
		return err
	}
	if st, err = read(); err != nil {
		return err
	}
	if st.Scrollbar || !st.Plain || !reflect.DeepEqual(st.Standard, before.Standard) || !reflect.DeepEqual(st.PlainSet, before.PlainSet) {
		return d29Fail("scrollbar switch saved wrong: %+v", st)
	}
	// The gear's own checkbox saves through the same path, to its own mode.
	if err := d29Eval(`(function(){ var i = document.querySelector('#secSettingsCols input[data-mode="plain"][data-sw="sessions"]'); i.checked = false; i.dispatchEvent(new Event('change', {bubbles: true})); })()`); err != nil {
		return err
	}
	time.Sleep(400 * time.Millisecond)
	if st, err = read(); err != nil {
		return err
	}
	if st.PlainSet["sessions"] || !st.Standard["sessions"] {
		return d29Fail("gear tick saved wrong: %+v", st)
	}
	return nil
}

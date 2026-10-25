package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// d29: the sections frame (agent control plan, row R8) and its follow-up, the
// mode toggle. Fed lane A's frozen example (testdata\snapshot_example.json,
// the file docs\snapshot_schema.md describes) it proves:
//   - Standard shows all seven sections, drops none at any size, and the page
//     scrolls at the window the screen grants (1152x1301 on this laptop);
//   - Plain keeps the fit: no scrollbar, and at 1024x480 with all seven
//     sections on it drops whole sections and says how many;
//   - the Plain toggle's text flips between "Plain: off" and "Plain: on";
//   - each switch removes its panel, and a stale or missing snapshot says
//     BurnMon is not running and draws no old numbers;
//   - the gear lists one column per mode, and a tick saved through it, and
//     through the real bdevSetSections, lands in that mode only. Survival of
//     a restart is the Go test of internal\sections (a fresh Load of the
//     saved file), not a real restart of the exe.
//
// The page is driven through window.__bdevSections.apply, which draws without
// saving. The persistence step at the end uses the real bindings and puts back
// what it found. Screenshots: d29-standard, d29-plain, d29-plain-small,
// d29-gear, d29-not-running.
var d29Order = []string{"waits", "outcomes", "burn", "agents_map", "sessions", "processes", "git"}

func d29Payload(plain bool, standardOff map[string]bool, plainAll bool, status string, snap json.RawMessage) string {
	standard, plainSet := map[string]bool{}, map[string]bool{}
	for _, id := range d29Order {
		standard[id] = !standardOff[id]
		plainSet[id] = plainAll || (id != "processes" && id != "git")
	}
	current := standard
	if plain {
		current = plainSet
	}
	reading := map[string]any{"status": status, "age_seconds": 7}
	if snap != nil {
		reading["snapshot"] = snap
	}
	b, _ := json.Marshal(map[string]any{
		"plain_mode": plain, "mode": map[bool]string{true: "plain", false: "standard"}[plain],
		"sections": current, "sections_standard": standard, "sections_plain": plainSet,
		"order": d29Order, "reading": reading,
	})
	return string(b)
}

type d29Panel struct {
	ID    string `json:"id"`
	Shown bool   `json:"shown"`
}

type d29Page struct {
	Panels       []d29Panel `json:"panels"`
	Plain        bool       `json:"plain"`
	Toggle       string     `json:"toggle"`
	ToggleOn     bool       `json:"toggleOn"`
	BodyStandard bool       `json:"bodyStandard"`
	BurnZone     bool       `json:"burnZone"`
	SystemZone   bool       `json:"systemZone"`
	Header       string     `json:"header"`
	Notice       string     `json:"notice"`
	Text         string     `json:"text"`
	Note         string     `json:"note"`
	Scrolls      bool       `json:"scrolls"`
	OverflowY    string     `json:"overflowY"`
	HostOver     float64    `json:"hostOver"`
	BodyOver     float64    `json:"bodyOver"`
	AgentsSlot   bool       `json:"agentsSlot"`
	StripClip    float64    `json:"stripClip"`
	GearOpen     bool       `json:"gearOpen"`
	Ticks        []string   `json:"ticks"`
}

func (p d29Page) shown() []string {
	var ids []string
	for _, x := range p.Panels {
		if x.Shown {
			ids = append(ids, x.ID)
		}
	}
	return ids
}

const d29ReadJS = `(function(){
  var disp = function(e){ return !!e && getComputedStyle(e).display !== 'none'; };
  var q = function(s){ return document.querySelector(s); };
  var ps = document.getElementById('pageScroll'), host = document.getElementById('secHost');
  var strip = document.getElementById('vendorStripPanel'), tg = document.getElementById('plainToggle');
  var ticks = [];
  Array.prototype.forEach.call(document.querySelectorAll('#secSettingsCols input[data-sw]'), function(i){
    if(i.checked) ticks.push(i.getAttribute('data-mode') + ':' + i.getAttribute('data-sw'));
  });
  return {
    panels: Array.prototype.map.call(document.querySelectorAll('#secPanels [data-sec]'), function(e){ return { id: e.getAttribute('data-sec'), shown: disp(e) }; }),
    plain: !!window.__bdevSections.state().plain,
    toggle: tg.textContent, toggleOn: tg.classList.contains('on'),
    bodyStandard: document.body.classList.contains('sec-standard'),
    burnZone: disp(q('.zone.burn')), systemZone: disp(q('.zone.system')),
    header: document.getElementById('hdrSnapAge').textContent,
    notice: document.getElementById('secNotice').textContent,
    text: document.getElementById('secPanels').textContent,
    note: document.getElementById('secCutNote').textContent,
    scrolls: ps.scrollHeight > ps.clientHeight + 1, overflowY: getComputedStyle(ps).overflowY,
    hostOver: host.scrollHeight - host.clientHeight, bodyOver: document.body.scrollHeight - document.body.clientHeight,
    agentsSlot: !!document.getElementById('secAgentsSlot'),
    stripClip: strip ? strip.scrollHeight - strip.clientHeight : 0,
    gearOpen: document.getElementById('secSettings').classList.contains('open'),
    ticks: ticks
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

func init() {
	checks["d29"] = func(hwnd uintptr, args []string) error {
		snap, err := os.ReadFile(filepath.Join("..", "..", "testdata", "snapshot_example.json"))
		if err != nil {
			return fmt.Errorf("read the frozen example (uicheck runs from the tools folder): %w", err)
		}
		if err := ensureWindowSizeWH(hwnd, 1152, 2048); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		all := strings.Join(d29Order, ",")

		// Standard: every section shown, none dropped, the page scrolls, no hidden note.
		p, err := d29Apply(d29Payload(false, nil, false, "ok", snap))
		if err != nil {
			return err
		}
		if strings.Join(p.shown(), ",") != all {
			return d29Fail("standard shows %v, want all seven", p.shown())
		}
		if p.Plain || p.Toggle != "Plain: off" || p.ToggleOn || !p.BodyStandard || !p.BurnZone || !p.SystemZone {
			return d29Fail("standard shape wrong: %+v", p)
		}
		if !p.Scrolls || p.OverflowY != "auto" {
			return d29Fail("standard page does not scroll (scrolls %v, overflow-y %s)", p.Scrolls, p.OverflowY)
		}
		if p.Note != "" {
			return d29Fail("standard shows a hidden note: %q", p.Note)
		}
		if p.BodyOver > 1 {
			return d29Fail("standard: the body itself overflows by %.0f px", p.BodyOver)
		}
		if !strings.Contains(p.Header, "snapshot 7 s old") {
			return d29Fail("header %q does not show the snapshot age", p.Header)
		}
		for _, want := range []string{"project", "API price equivalent", "Plan limits", "Not built yet", "coming (R9)", "120"} {
			if !strings.Contains(p.Text, want) {
				return d29Fail("panels do not show %q from the example", want)
			}
		}
		if !p.AgentsSlot {
			return d29Fail("agents map has no slot for R7")
		}
		if p.StripClip > 1 {
			return d29Fail("standard: the vendor strip is clipped by %.0f px", p.StripClip)
		}
		if _, err := screenshot(hwnd, "d29-standard"); err != nil {
			return err
		}

		// One switch at a time: that panel goes, the other six stay, nothing is dropped for room.
		for _, id := range d29Order {
			p, err := d29Apply(d29Payload(false, map[string]bool{id: true}, false, "ok", snap))
			if err != nil {
				return err
			}
			if len(p.shown()) != 6 || strings.Contains(strings.Join(p.shown(), ","), id) {
				return d29Fail("switching %s off left panels %v", id, p.shown())
			}
		}
		// The older zones follow Burn and Processes.
		p, err = d29Apply(d29Payload(false, map[string]bool{"burn": true, "processes": true}, false, "ok", snap))
		if err != nil {
			return err
		}
		if p.BurnZone || p.SystemZone {
			return d29Fail("burn and processes off: burn zone %v, system zone %v", p.BurnZone, p.SystemZone)
		}

		// Plain: the Plain list, plain words, the older zones gone, fits the window.
		p, err = d29Apply(d29Payload(true, nil, false, "ok", snap))
		if err != nil {
			return err
		}
		if strings.Join(p.shown(), ",") != "waits,outcomes,burn,agents_map,sessions" {
			return d29Fail("plain shows %v", p.shown())
		}
		if !p.Plain || p.Toggle != "Plain: on" || !p.ToggleOn || p.BodyStandard || p.BurnZone || p.SystemZone {
			return d29Fail("plain shape wrong: %+v", p)
		}
		if p.OverflowY != "hidden" {
			return d29Fail("plain page can scroll (overflow-y %s)", p.OverflowY)
		}
		if p.Note != "" || p.HostOver > 1 {
			return d29Fail("plain at this size: note %q, host overflow %.0f px", p.Note, p.HostOver)
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

		// Plain keeps the fit: all seven on in a small window drops whole sections and says so.
		if err := ensureWindowSizeWH(hwnd, 1024, 480); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		p, err = d29Apply(d29Payload(true, nil, true, "ok", snap))
		if err != nil {
			return err
		}
		if len(p.shown()) >= 7 || !strings.Contains(p.Note, "hidden, the window is too small") {
			return d29Fail("plain at 1024x480 with seven sections: shown %v, note %q", p.shown(), p.Note)
		}
		if p.HostOver > 1 || p.BodyOver > 1 || p.OverflowY != "hidden" {
			return d29Fail("plain at 1024x480 overflows: host %.0f px, body %.0f px, overflow-y %s", p.HostOver, p.BodyOver, p.OverflowY)
		}
		if _, err := screenshot(hwnd, "d29-plain-small"); err != nil {
			return err
		}
		// Standard at the same size drops nothing and has no note: it scrolls instead.
		p, err = d29Apply(d29Payload(false, nil, false, "ok", snap))
		if err != nil {
			return err
		}
		if strings.Join(p.shown(), ",") != all || p.Note != "" || !p.Scrolls {
			return d29Fail("standard at 1024x480: shown %v, note %q, scrolls %v", p.shown(), p.Note, p.Scrolls)
		}
		if err := ensureWindowSizeWH(hwnd, 1152, 2048); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)

		// The toggle's text flips with each press.
		if _, err = d29Apply(d29Payload(false, nil, false, "ok", snap)); err != nil {
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

		// The gear lists one column per mode, ticked from the saved lists.
		if p, err = d29Apply(d29Payload(false, map[string]bool{"git": true}, false, "ok", snap)); err != nil {
			return err
		}
		if err := d29Eval(`document.getElementById('secGear').click()`); err != nil {
			return err
		}
		if p, err = d29Read(); err != nil {
			return err
		}
		if !p.GearOpen || len(p.Ticks) != 11 {
			return d29Fail("gear: open %v, %d ticks, want 11 (6 standard without git, 5 plain)", p.GearOpen, len(p.Ticks))
		}
		for _, t := range p.Ticks {
			if t == "standard:git" || t == "plain:git" || t == "plain:processes" {
				return d29Fail("gear ticked %s, which is off in the payload", t)
			}
		}
		if _, err := screenshot(hwnd, "d29-gear"); err != nil {
			return err
		}
		if err := d29Eval(`document.getElementById('secGear').click()`); err != nil {
			return err
		}

		// Stale and missing: say so, draw no old numbers.
		for _, status := range []string{"stale", "missing"} {
			p, err = d29Apply(d29Payload(true, nil, false, status, nil))
			if err != nil {
				return err
			}
			if !strings.Contains(p.Notice, "not running") || !strings.Contains(p.Header, "not running") {
				return d29Fail("%s: header %q notice %q do not say BurnMon is not running", status, p.Header, p.Notice)
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
}

type d29Saved struct {
	Plain    bool            `json:"plain_mode"`
	Standard map[string]bool `json:"sections_standard"`
	PlainSet map[string]bool `json:"sections_plain"`
	Sections map[string]bool `json:"sections"`
}

// d29Persist drives the real Go side: the toggle, one switch in each mode
// through bdevSetSections, and one tick through the gear's own checkbox. Each
// must land in its own mode only. It puts back every value it found.
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
	orig, err := read()
	if err != nil {
		return err
	}
	defer func() {
		for _, id := range d29Order {
			_ = send(fmt.Sprintf(`{mode:'standard',section:%s,on:%t}`, jsString(id), orig.Standard[id]))
			_ = send(fmt.Sprintf(`{mode:'plain',section:%s,on:%t}`, jsString(id), orig.PlainSet[id]))
		}
		_ = send(fmt.Sprintf(`{plain:%t}`, orig.Plain))
	}()

	// Start from Standard, git on in Standard, off in Plain.
	if err := send(`{plain:false}`); err != nil {
		return err
	}
	if err := send(`{mode:'standard',section:'git',on:true}`); err != nil {
		return err
	}
	if err := send(`{mode:'plain',section:'git',on:false}`); err != nil {
		return err
	}
	// The toggle.
	if err := send(`{plain:true}`); err != nil {
		return err
	}
	st, err := read()
	if err != nil {
		return err
	}
	if !st.Plain || st.Sections["git"] || !st.Standard["git"] {
		return d29Fail("toggle saved wrong: %+v", st)
	}
	// One switch in Plain only.
	if err := send(`{mode:'plain',section:'git',on:true}`); err != nil {
		return err
	}
	if st, err = read(); err != nil {
		return err
	}
	if !st.PlainSet["git"] || !st.Standard["git"] || !st.Sections["git"] {
		return d29Fail("plain git switch saved wrong: %+v", st)
	}
	// One switch in Standard only, with the page in Plain: the current list does not move.
	if err := send(`{mode:'standard',section:'burn',on:false}`); err != nil {
		return err
	}
	if st, err = read(); err != nil {
		return err
	}
	if st.Standard["burn"] || !st.PlainSet["burn"] || !st.Sections["burn"] {
		return d29Fail("standard burn switch saved wrong: %+v", st)
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

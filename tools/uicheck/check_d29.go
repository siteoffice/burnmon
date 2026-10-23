package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// d29: the sections frame (agent control plan, row R8). Fed lane A's frozen
// example (testdata\snapshot_example.json, the file docs\snapshot_schema.md
// describes), all seven sections render, each one's switch removes its panel,
// Simple and Developer both draw and Plain Mode is on in Simple, a stale or
// missing snapshot says BurnMon is not running and draws no old numbers, and
// the preset the Go side saved reads back. The page is driven through
// window.__bdevSections.apply, which draws without saving; only the last step
// uses the real bdevSetSections and it restores the preset it found.
// Screenshots: d29-developer, d29-simple, d29-not-running.

var d29Order = []string{"waits", "outcomes", "burn", "agents_map", "sessions", "processes", "git"}

func d29Payload(preset string, plain bool, off map[string]bool, status string, snap json.RawMessage) string {
	// Go names a state by its switches; with any switch off it is custom.
	if len(off) > 0 {
		preset = "custom"
	}
	sections := map[string]bool{}
	for _, id := range d29Order {
		on := !off[id]
		if preset == "simple" && (id == "processes" || id == "git") {
			on = false
		}
		sections[id] = on
	}
	reading := map[string]any{"status": status, "age_seconds": 7}
	if snap != nil {
		reading["snapshot"] = snap
	}
	b, _ := json.Marshal(map[string]any{
		"sections": sections, "plain_mode": plain, "preset": preset, "order": d29Order, "reading": reading,
	})
	return string(b)
}

type d29Page struct {
	Panels      []string `json:"panels"`
	Plain       bool     `json:"plain"`
	BurnZone    bool     `json:"burnZone"`
	SystemZone  bool     `json:"systemZone"`
	Header      string   `json:"header"`
	Notice      string   `json:"notice"`
	Text        string   `json:"text"`
	Switches    int      `json:"switches"`
	ScrollH     float64  `json:"scrollH"`
	InnerH      float64  `json:"innerH"`
	AgentsSlot  bool     `json:"agentsSlot"`
	PresetBtnOn string   `json:"presetBtnOn"`
	StripClip   float64  `json:"stripClip"`
	Cut         int      `json:"cut"`
}

const d29ReadJS = `(function(){
  var vis = function(sel){ var e = document.querySelector(sel); return !!e && getComputedStyle(e).display !== 'none'; };
  var s = window.__bdevSections.state();
  return {
    panels: Array.prototype.map.call(document.querySelectorAll('#secPanels [data-sec]'), function(e){ return e.getAttribute('data-sec'); }),
    plain: !!s.plain,
    burnZone: vis('.zone.burn'), systemZone: vis('.zone.system'),
    header: document.getElementById('hdrSnapAge').textContent,
    notice: document.getElementById('secNotice').textContent,
    text: document.getElementById('secPanels').textContent,
    switches: document.querySelectorAll('#secBar input[data-sw]').length,
    scrollH: document.body.scrollHeight, innerH: window.innerHeight,
    agentsSlot: !!document.getElementById('secAgentsSlot'),
    cut: document.querySelectorAll('#secPanels .sec-cut').length,
    stripClip: (function(){ var e = document.getElementById('vendorStripPanel'); return e ? e.scrollHeight - e.clientHeight : 0; })(),
    presetBtnOn: (document.querySelector('#secBar [data-preset].on') || {getAttribute:function(){return ''}}).getAttribute('data-preset')
  };
})()`

func d29Apply(payload string) (d29Page, error) {
	var p d29Page
	if _, err := evalRaw(`window.__bdevSections.apply(` + payload + `)`); err != nil {
		return p, err
	}
	time.Sleep(400 * time.Millisecond)
	err := evalInto(d29ReadJS, &p)
	return p, err
}

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

		// Developer: all seven panels, both older zones, Plain Mode off.
		p, err := d29Apply(d29Payload("developer", false, nil, "ok", snap))
		if err != nil {
			return err
		}
		if strings.Join(p.Panels, ",") != strings.Join(d29Order, ",") {
			return fmt.Errorf("developer panels %v, want %v", p.Panels, d29Order)
		}
		if p.Plain || !p.BurnZone || !p.SystemZone || p.Switches != 7 || p.PresetBtnOn != "developer" {
			return fmt.Errorf("developer shape wrong: %+v", p)
		}
		if !strings.Contains(p.Header, "snapshot 7 s old") {
			return fmt.Errorf("header %q does not show the snapshot age", p.Header)
		}
		for _, want := range []string{"project", "API price equivalent", "Plan limits", "Not built yet", "coming (R9)", "120"} {
			if !strings.Contains(p.Text, want) {
				return fmt.Errorf("panels do not show %q from the example", want)
			}
		}
		// The vendor strip has a height cap; its last row must not be cut off.
		if p.StripClip > 1 {
			return fmt.Errorf("developer: the vendor strip is clipped by %.0f px", p.StripClip)
		}
		if !p.AgentsSlot {
			return fmt.Errorf("agents map has no slot for R7")
		}
		if p.ScrollH > p.InnerH+4 {
			return fmt.Errorf("developer: page %.0f px is taller than the window %.0f px", p.ScrollH, p.InnerH)
		}
		if _, err := screenshot(hwnd, "d29-developer"); err != nil {
			return err
		}

		// One switch at a time: that panel goes, the other six stay.
		for _, id := range d29Order {
			p, err := d29Apply(d29Payload("developer", false, map[string]bool{id: true}, "ok", snap))
			if err != nil {
				return err
			}
			if len(p.Panels) != 6 || strings.Contains(strings.Join(p.Panels, ","), id) {
				return fmt.Errorf("switching %s off left panels %v", id, p.Panels)
			}
		}
		// The older zones follow Burn and Processes.
		p, err = d29Apply(d29Payload("developer", false, map[string]bool{"burn": true, "processes": true}, "ok", snap))
		if err != nil {
			return err
		}
		if p.BurnZone || p.SystemZone || p.PresetBtnOn != "" {
			return fmt.Errorf("burn and processes off: burn zone %v, system zone %v, preset button %q", p.BurnZone, p.SystemZone, p.PresetBtnOn)
		}

		// Simple: five panels, Plain Mode on, the older zones gone.
		p, err = d29Apply(d29Payload("simple", true, nil, "ok", snap))
		if err != nil {
			return err
		}
		if strings.Join(p.Panels, ",") != "waits,outcomes,burn,agents_map,sessions" {
			return fmt.Errorf("simple panels %v", p.Panels)
		}
		if p.Cut != 0 {
			return fmt.Errorf("simple: %d panels dropped for lack of room", p.Cut)
		}
		if !p.Plain || p.BurnZone || p.SystemZone || p.PresetBtnOn != "simple" {
			return fmt.Errorf("simple shape wrong: %+v", p)
		}
		for _, jargon := range []string{"tokens/min", "API price equiv."} {
			if strings.Contains(p.Text, jargon) {
				return fmt.Errorf("plain mode still shows %q", jargon)
			}
		}
		if !strings.Contains(p.Text, "waiting for you") {
			return fmt.Errorf("plain mode did not put the state in plain words")
		}
		if _, err := screenshot(hwnd, "d29-simple"); err != nil {
			return err
		}

		// Stale and missing: say so, draw no old numbers.
		for _, status := range []string{"stale", "missing"} {
			p, err = d29Apply(d29Payload("simple", true, nil, status, nil))
			if err != nil {
				return err
			}
			if !strings.Contains(p.Notice, "not running") || !strings.Contains(p.Header, "not running") {
				return fmt.Errorf("%s: header %q notice %q do not say BurnMon is not running", status, p.Header, p.Notice)
			}
			if strings.Contains(p.Text, "tokens") && strings.Contains(p.Text, "120") {
				return fmt.Errorf("%s: old numbers are still drawn", status)
			}
		}
		if _, err := screenshot(hwnd, "d29-not-running"); err != nil {
			return err
		}

		return d29Persist()
	}
}

// d29Persist asks the real Go side to save Simple, reads it back, and puts
// back whatever preset it found.
func d29Persist() error {
	read := func() (string, error) {
		if _, err := evalRaw(`(window.__d29 = null, window.bdevSections().then(function(p){ window.__d29 = p.preset + '|' + p.plain_mode; }), 1)`); err != nil {
			return "", err
		}
		for i := 0; i < 20; i++ {
			time.Sleep(150 * time.Millisecond)
			s, err := evalString(`window.__d29 || ''`)
			if err != nil {
				return "", err
			}
			if s != "" {
				return s, nil
			}
		}
		return "", fmt.Errorf("bdevSections never answered")
	}
	set := func(preset string) error {
		_, err := evalRaw(`(window.bdevSetSections({preset: ` + jsString(preset) + `}), 1)`)
		time.Sleep(400 * time.Millisecond)
		return err
	}
	before, err := read()
	if err != nil {
		return err
	}
	restore := strings.SplitN(before, "|", 2)[0]
	if restore != "simple" && restore != "developer" {
		restore = "developer"
	}
	defer set(restore)

	for _, want := range []string{"simple", "developer", "simple"} {
		if err := set(want); err != nil {
			return err
		}
		got, err := read()
		if err != nil {
			return err
		}
		wantPlain := "false"
		if want == "simple" {
			wantPlain = "true"
		}
		if got != want+"|"+wantPlain {
			return fmt.Errorf("saved preset %s, read back %q", want, got)
		}
	}
	return nil
}

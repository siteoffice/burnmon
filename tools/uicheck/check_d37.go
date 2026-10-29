package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// d37: control chips, the preview line, Confirm, expiry and the result (agent
// control plan row R10, B4). The two Go bindings are REPLACED by recorders in
// the page, so no real control is ever pressed. Fed the unmodified frozen
// example it proves:
//   - chips show only when the switch is on AND ready: all off, or on but not
//     ready, show none; the gear shows the reason text;
//   - the agentview:ab12 and session:anthropic:example-sub wait rows have stop
//     and attach, the G2 gate row has approve and send back, the G4 gate row,
//     the outcome rows and the interactive session have none;
//   - one chip press calls the preview recorder with exactly {control, target},
//     never the run recorder, and shows the command as text with a Confirm
//     button that is disabled for the first 400 ms;
//   - two Confirm presses 50 ms apart send one run request, with the token only;
//   - a preview left alone for 5 s loses its Confirm button and says it expired;
//   - two previews survive a re-apply (the poll) and each Confirm sends its own
//     token;
//   - a refusal from the run binding is shown as text;
//   - Plain mode uses Stop, Open, Approve, Send back;
//   - hostile text in a label shows literally and creates no element.
//
// Screenshots: d37-chips, d37-preview.
const d37Recorders = `(function(){
  window.__d37prev = []; window.__d37run = []; window.__d37label = null; window.__d37fail = null;
  window.bdevControlPreview = function(r){
    window.__d37prev.push(JSON.parse(JSON.stringify(r)));
    return Promise.resolve({token:'tok-' + r.target, command:'C:\\example\\claude.exe stop ' + r.target,
      label: window.__d37label || ('other (' + r.target + ')'), expires_in_ms:5000});
  };
  window.bdevControlRun = function(t){
    window.__d37run.push(t);
    if(window.__d37fail) return Promise.reject(new Error(window.__d37fail));
    return Promise.resolve({exit_code:0, line:'stopped ab12'});
  };
  return true;
})()`

const d37ReadJS = `(function(){
  var rows = {};
  Array.prototype.forEach.call(document.querySelectorAll('[data-sec=waits] .secitem'), function(r){
    rows[r.getAttribute('data-id')] = Array.prototype.map.call(r.querySelectorAll('.ctl'), function(b){ return b.textContent; }).join(',');
  });
  var gear = {};
  Array.prototype.forEach.call(document.querySelectorAll('[data-ctl-why]'), function(s){ gear[s.getAttribute('data-ctl-why')] = s.textContent; });
  var prev = document.querySelector('[data-sec=waits] .ctlprev');
  var go = document.querySelector('[data-sec=waits] .ctlgo');
  var prevBy = {};
  Array.prototype.forEach.call(document.querySelectorAll('[data-sec=waits] .ctlprev'), function(e){
    var k = e.getAttribute('data-ctlid'); if(!(k in prevBy)) prevBy[k] = e.textContent; });
  var results = {};
  Array.prototype.forEach.call(document.querySelectorAll('[data-sec=waits] .ctlres'), function(e){
    var k = e.getAttribute('data-ctlid'); if(!(k in results)) results[k] = e.textContent; });
  return { rows: rows, all: document.querySelectorAll('#secPanels .ctl').length,
    outcome: document.querySelectorAll('[data-sec=outcomes] .ctl').length, gear: gear,
    prevText: prev ? prev.textContent : '', cmd: prev ? prev.querySelector('.ctlcmd').textContent : '',
    cmdKids: prev ? prev.querySelector('.ctlcmd').children.length : 0,
    goCount: document.querySelectorAll('#secPanels .ctlgo').length, goDisabled: go ? go.disabled : false,
    prevBy: prevBy, results: results, imgs: document.querySelectorAll('#secPanels img').length,
    runs: window.__d37run.slice(), prevs: window.__d37prev.slice() };
})()`

type d37Page struct {
	Rows     map[string]string `json:"rows"`
	All      int               `json:"all"`
	Outcome  int               `json:"outcome"`
	Gear     map[string]string `json:"gear"`
	PrevText string            `json:"prevText"`
	Cmd      string            `json:"cmd"`
	CmdKids  int               `json:"cmdKids"`
	GoCount  int               `json:"goCount"`
	GoDis    bool              `json:"goDisabled"`
	PrevBy   map[string]string `json:"prevBy"`  // preview text per control|target
	Results  map[string]string `json:"results"` // result text per control|target
	Imgs     int               `json:"imgs"`
	Runs     []string          `json:"runs"`
	Prevs    []map[string]any  `json:"prevs"`
}

func caseD37(hwnd uintptr, args []string) error {
	raw, err := readFixture("snapshot_example.json")
	if err != nil {
		return fmt.Errorf("read the frozen example (uicheck runs from the tools folder): %w", err)
	}
	fail := func(format string, a ...any) error { return fmt.Errorf("d37: "+format, a...) }
	if err := ensureWindowSizeWH(hwnd, 1920, 1080); err != nil {
		return err
	}
	all := map[string]bool{"stop": true, "attach": true, "approve": true, "send_back": true}
	none := map[string]bool{"stop": false, "attach": false, "approve": false, "send_back": false}
	verdicts := map[string]map[string]string{
		"approve":   {"G1": "approve", "G2": "approve", "G3": "proceed"},
		"send_back": {"G1": "reject", "G2": "reject"},
	}
	payload := func(plain bool, on, ready map[string]bool, why map[string]string) string {
		var m map[string]any
		if err := json.Unmarshal([]byte(d29Payload(d29Opts{Plain: plain, Scrollbar: true, Status: "ok", Snap: raw})), &m); err != nil {
			panic(err)
		}
		m["controls"] = map[string]any{"on": on, "ready": ready, "why": why, "verdicts": verdicts}
		b, _ := json.Marshal(m)
		return string(b)
	}
	read := func() (d37Page, error) {
		var p d37Page
		time.Sleep(150 * time.Millisecond)
		err := evalInto(d37ReadJS, &p)
		return p, err
	}
	apply := func(pl string) (d37Page, error) {
		if _, err := d29Apply(pl); err != nil {
			return d37Page{}, err
		}
		return read()
	}
	click := func(js string) error { _, err := evalRaw(js); time.Sleep(80 * time.Millisecond); return err }
	chip := func(c, t string) string {
		return `document.querySelector('[data-sec=waits] .ctl[data-ctl="` + c + `"][data-target="` + t + `"]').click()`
	}
	if _, err := evalRaw(d37Recorders); err != nil {
		return err
	}

	// Off, or on but not ready: no chip; the gear says why.
	p, err := apply(payload(false, none, none, nil))
	if err != nil {
		return err
	}
	if p.All != 0 {
		return fail("%d chips with every switch off", p.All)
	}
	why := map[string]string{"stop": "needs claude.exe: set claude_path in burnmon.json", "approve": "BurnMon Dev runs as administrator: controls are off"}
	if p, err = apply(payload(false, all, none, why)); err != nil {
		return err
	}
	if p.All != 0 {
		return fail("%d chips when on but not ready", p.All)
	}
	if p.Gear["stop"] != why["stop"] || p.Gear["approve"] != why["approve"] {
		return fail("gear reasons %v, want %v", p.Gear, why)
	}

	// On and ready: chips only where offered.
	if p, err = apply(payload(false, all, all, nil)); err != nil {
		return err
	}
	want := map[string]string{"agentview:ab12": "stop,attach", "session:anthropic:example-sub": "stop,attach",
		"gate:example__flow__2:G2": "approve,send back", "gate:example__flow__1:G4": "", "session:anthropic:example-parent": ""}
	for id, w := range want {
		if p.Rows[id] != w {
			return fail("row %q has chips %q, want %q", id, p.Rows[id], w)
		}
	}
	if p.Outcome != 0 {
		return fail("%d chips on outcome rows", p.Outcome)
	}
	if _, err := screenshot(hwnd, "d37-chips"); err != nil {
		return err
	}

	// One press previews only; the command is text; Confirm is disabled at first.
	if err := click(chip("stop", "ab12")); err != nil {
		return err
	}
	if p, err = read(); err != nil {
		return err
	}
	if len(p.Prevs) != 1 || len(p.Prevs[0]) != 2 || p.Prevs[0]["control"] != "stop" || p.Prevs[0]["target"] != "ab12" || len(p.Runs) != 0 {
		return fail("first press sent preview %v and runs %v, want exactly {control, target} and no run", p.Prevs, p.Runs)
	}
	if !strings.Contains(p.Cmd, "claude.exe stop ab12") || !strings.Contains(p.PrevText, "again within 5 s") || p.CmdKids != 0 {
		return fail("preview line %q / %q", p.Cmd, p.PrevText)
	}
	if !p.GoDis {
		return fail("Confirm is enabled in the first 400 ms")
	}
	if _, err := screenshot(hwnd, "d37-preview"); err != nil {
		return err
	}
	time.Sleep(450 * time.Millisecond)

	// Two Confirm presses 50 ms apart: one run, the token only.
	if _, err := evalRaw(`(function(){ var b = document.querySelector('[data-sec=waits] .ctlgo'); b.click();
			setTimeout(function(){ var c = document.querySelector('[data-sec=waits] .ctlgo'); if(c) c.click(); }, 50); return true; })()`); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	if p, err = read(); err != nil {
		return err
	}
	if len(p.Runs) != 1 || p.Runs[0] != "tok-ab12" {
		return fail("double press sent runs %v, want exactly [tok-ab12]", p.Runs)
	}
	if p.Results["stop|ab12"] != "stop: exit 0, stopped ab12" || p.GoCount != 0 {
		return fail("result %q, confirm buttons %d", p.Results["stop|ab12"], p.GoCount)
	}

	// A preview left for 5 s expires and loses Confirm. Results are read per
	// control and target: the stop result above stays 10 s (page.html
	// CTL_RESULT_MS) and renders before attach's in the same row.
	if err := click(chip("attach", "ab12")); err != nil {
		return err
	}
	time.Sleep(5400 * time.Millisecond)
	if p, err = read(); err != nil {
		return err
	}
	if p.GoCount != 0 || !strings.Contains(p.Results["attach|ab12"], "expired") {
		return fail("after 5 s: %d confirm buttons, results %q", p.GoCount, p.Results)
	}

	// Two previews survive a re-apply; each Confirm sends its own token.
	if err := click(chip("stop", "ab12")); err != nil {
		return err
	}
	if err := click(chip("approve", "gate:example__flow__2:G2")); err != nil {
		return err
	}
	time.Sleep(450 * time.Millisecond)
	if p, err = apply(payload(false, all, all, nil)); err != nil {
		return err
	}
	if p.GoCount != 2 {
		return fail("%d confirm buttons after a poll, want 2", p.GoCount)
	}
	// One expression: the eval server wraps the script as return (...).
	if _, err := evalRaw(`(function(){ window.__d37run.length = 0; document.querySelector('.ctlgo[data-ctlgo=approve]').click(); return true; })()`); err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)
	if p, err = read(); err != nil {
		return err
	}
	if len(p.Runs) != 1 || p.Runs[0] != "tok-gate:example__flow__2:G2" {
		return fail("approve sent runs %v", p.Runs)
	}

	// A refusal is shown as text. Confirm is ignored within 400 ms of the last
	// run (page.html ctlConfirm, CTL_GAP), so wait past the approve run first.
	if _, err := evalRaw(`window.__d37fail = 'the session changed since the preview, press again'`); err != nil {
		return err
	}
	time.Sleep(450 * time.Millisecond)
	if err := click(`document.querySelector('.ctlgo[data-ctlgo=stop]').click()`); err != nil {
		return err
	}
	if p, err = read(); err != nil {
		return err
	}
	if !strings.Contains(p.Results["stop|ab12"], "refused: the session changed since the preview") {
		return fail("refusal shown as %q", p.Results["stop|ab12"])
	}
	if _, err := evalRaw(`window.__d37fail = null`); err != nil {
		return err
	}

	// Plain words.
	if p, err = apply(payload(true, all, all, nil)); err != nil {
		return err
	}
	if p.Rows["agentview:ab12"] != "Stop,Open" || p.Rows["gate:example__flow__2:G2"] != "Approve,Send back" {
		return fail("plain chips %q / %q", p.Rows["agentview:ab12"], p.Rows["gate:example__flow__2:G2"])
	}
	time.Sleep(500 * time.Millisecond)
	if err := click(chip("stop", "ab12")); err != nil {
		return err
	}
	if p, err = read(); err != nil {
		return err
	}
	if !strings.Contains(p.PrevText, "again within 5 seconds") {
		return fail("plain preview text %q", p.PrevText)
	}

	// Hostile label: literal text, no element. Read by key: the Plain stop
	// preview above is still open and renders first in the row.
	if _, err := evalRaw(`window.__d37label = 'a<b "c" & d \\ <img src=x>'`); err != nil {
		return err
	}
	if p, err = apply(payload(false, all, all, nil)); err != nil {
		return err
	}
	if err := click(chip("attach", "ab12")); err != nil {
		return err
	}
	if p, err = read(); err != nil {
		return err
	}
	if !strings.Contains(p.PrevBy["attach|ab12"], `a<b "c" & d \ <img src=x>`) || p.Imgs != 0 {
		return fail("hostile label shown as %q with %d images", p.PrevBy["attach|ab12"], p.Imgs)
	}
	return nil
}

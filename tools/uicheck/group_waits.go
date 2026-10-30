package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// waitsOpenDoneJS opens the Done group with a click on its header when it is
// closed (it starts closed at every page load, d44) and returns its
// aria-expanded after, or "" when the list has no Done group.
const waitsOpenDoneJS = `(function(){
  var h = document.querySelector('[data-sec=waits] .sectier.done');
  if(!h) return '';
  if(h.getAttribute('aria-expanded') === 'false') h.click();
  h = document.querySelector('[data-sec=waits] .sectier.done');
  return h ? h.getAttribute('aria-expanded') || '' : '';
})()`

// waitsOpenDone is for the cases that read every wait row: it opens the Done
// group the way a user does and fails when it stays closed.
func waitsOpenDone() error {
	var got string
	if err := evalInto(waitsOpenDoneJS, &got); err != nil {
		return err
	}
	if got != "" && got != "true" {
		return fmt.Errorf("the Done group did not open: aria-expanded %q", got)
	}
	return nil
}

// waits/d36: moved from check_d36.go.

// d36: Waits on you and Outcomes list the items (agent control plan row R9).
// Fed the UNMODIFIED frozen example (testdata\snapshot_example.json, the gate is not moved
// to the front) it proves:
//   - with the Done group opened (it starts collapsed, d44), every wait item is
//     a row in the DOM (the list scrolls inside the panel, none is
//     cut off), no +k more line; the header carries the Blocked count from
//     wait_counts, not the row total (B6 scope item 8, one count everywhere):
//     "Waits on you (7)" plus "1 your turn";
//   - Standard: the Waits rows are the example's wait_items and the Outcomes
//     rows its outcome_items, in order; the gate row reads "gate G4", the
//     prompt row "permission prompt", the sprint row "2 tasks parked"; every
//     row is one line; each spec or result link carries the item's exact path
//     and a missing result reads "no result yet";
//   - Plain: the same rows in plain words, links read "request" and "result";
//   - a reading older than two ticks marks both panels aged with the age;
//     at age 7 neither is;
//   - empty lists read the empty texts;
//   - a click on the first gate's spec link calls bdevOpenItemFile with that path
//     (the binding is replaced by a recorder and put back).
//
// Screenshots: d36-standard, d36-plain, d36-aged.
type d36Row struct {
	ID       string   `json:"id"`
	Text     string   `json:"text"`
	Height   float64  `json:"height"`
	LineH    float64  `json:"lineH"`
	Links    []string `json:"links"`
	LinkText []string `json:"linkText"`
}

type d36Page struct {
	Waits    []d36Row `json:"waits"`
	Outcomes []d36Row `json:"outcomes"`
	WaitsTxt string   `json:"waitsTxt"`
	OutTxt   string   `json:"outTxt"`
	WAged    bool     `json:"wAged"`
	OAged    bool     `json:"oAged"`
}

type d36Item struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	SpecPath   string `json:"spec_path"`
	ResultPath string `json:"result_path"`
}

const d36ReadJS = `(function(){
  var rows = function(sec){
    return Array.prototype.map.call(document.querySelectorAll('[data-sec=' + sec + '] .secitem'), function(r){
      var links = Array.prototype.map.call(r.querySelectorAll('.secfile'), function(a){ return a.getAttribute('data-path'); });
      var lt = Array.prototype.map.call(r.querySelectorAll('.secfile'), function(a){ return a.textContent; });
      return { id: r.getAttribute('data-id'), text: r.textContent, height: r.getBoundingClientRect().height,
        lineH: parseFloat(getComputedStyle(r).lineHeight) || parseFloat(getComputedStyle(r).fontSize) * 1.3, links: links, linkText: lt };
    });
  };
  var w = document.querySelector('[data-sec=waits]'), o = document.querySelector('[data-sec=outcomes]');
  return { waits: rows('waits'), outcomes: rows('outcomes'), waitsTxt: w ? w.textContent : '', outTxt: o ? o.textContent : '',
    wAged: !!w && w.classList.contains('sec-aged'), oAged: !!o && o.classList.contains('sec-aged') };
})()`

// d36ClickJS clicks the first link (the spec) of the row with data-id id. The
// example holds two gates (G4 and the B4 G2 row), so the row is picked by id.
func d36ClickJS(id string) string {
	q, _ := json.Marshal(id)
	return `(function(){
  var old = window.bdevOpenItemFile, got = [];
  window.bdevOpenItemFile = function(p){ got.push(p); return Promise.resolve(); };
  var a = document.querySelector('[data-sec=waits] .secitem[data-id="' + ` + string(q) + ` + '"] .secfile');
  if(a) a.click();
  window.bdevOpenItemFile = old;
  return got;
})()`
}

func caseD36(hwnd uintptr, args []string) error {
	raw, err := readSnapshotExample()
	if err != nil {
		return err
	}
	var ex struct {
		Wait    []d36Item `json:"wait_items"`
		Outcome []d36Item `json:"outcome_items"`
		Counts  *struct {
			Blocked  int `json:"blocked"`
			YourTurn int `json:"your_turn"`
		} `json:"wait_counts"`
	}
	if err := json.Unmarshal(raw, &ex); err != nil {
		return err
	}
	fail := func(format string, a ...any) error { return fmt.Errorf("d36: "+format, a...) }
	if ex.Counts == nil || ex.Counts.Blocked == 0 {
		return fail("the example has no wait_counts with a Blocked count")
	}
	apply := func(plain bool, snap []byte, age string) (d36Page, error) {
		var p d36Page
		payload := d29Payload(d29Opts{Plain: plain, Scrollbar: true, Status: "ok", Snap: snap})
		payload = strings.Replace(payload, `"age_seconds":7`, `"age_seconds":`+age, 1)
		if _, err := d29Apply(payload); err != nil {
			return p, err
		}
		if err := waitsOpenDone(); err != nil {
			return p, fail("%v", err)
		}
		err := evalInto(d36ReadJS, &p)
		return p, err
	}
	sameRows := func(label string, rows []d36Row, items []d36Item) error {
		if len(rows) != len(items) {
			return fail("%s: %d rows, want %d", label, len(rows), len(items))
		}
		for i, it := range items {
			r := rows[i]
			if r.ID != it.ID {
				return fail("%s row %d is %q, want %q", label, i, r.ID, it.ID)
			}
			if r.Height > r.LineH*1.6 {
				return fail("%s row %q is %.0f px high for a %.0f px line", label, it.ID, r.Height, r.LineH)
			}
			var want []string
			for _, p := range []string{it.SpecPath, it.ResultPath} {
				if p != "" {
					want = append(want, p)
				}
			}
			if strings.Join(r.Links, "|") != strings.Join(want, "|") {
				return fail("%s row %q links %v, want %v", label, it.ID, r.Links, want)
			}
		}
		return nil
	}

	p, err := apply(false, raw, "7")
	if err != nil {
		return err
	}
	if err := sameRows("standard waits", p.Waits, ex.Wait); err != nil {
		return err
	}
	if err := sameRows("standard outcomes", p.Outcomes, ex.Outcome); err != nil {
		return err
	}
	// Nothing is cut off: every wait item is a row (sameRows above) and there is
	// no +k more line. The header count is the Blocked count (B6 item 8), with
	// Your turn beside it.
	if want := fmt.Sprintf("Waits on you (%d)", ex.Counts.Blocked); !strings.Contains(p.WaitsTxt, want) {
		return fail("standard Waits header lacks %q", want)
	}
	if want := fmt.Sprintf("%d your turn", ex.Counts.YourTurn); ex.Counts.YourTurn > 0 && !strings.Contains(p.WaitsTxt, want) {
		return fail("standard Waits header lacks %q", want)
	}
	if strings.Contains(p.WaitsTxt, " more") || strings.Contains(p.OutTxt, " more") {
		return fail("a list still ends in a more line")
	}
	if strings.Contains(p.WaitsTxt, "permission prompt project permission prompt") {
		return fail("standard prompt row repeats its kind word")
	}
	for _, want := range []string{"gate G4", "permission prompt", "2 tasks parked", "no result yet"} {
		if !strings.Contains(p.WaitsTxt, want) {
			return fail("standard Waits lacks %q", want)
		}
	}
	if p.WAged || p.OAged {
		return fail("age 7 marks the panels aged")
	}
	if _, err := screenshot(hwnd, "d36-standard"); err != nil {
		return err
	}

	if p, err = apply(true, raw, "7"); err != nil {
		return err
	}
	if err := sameRows("plain waits", p.Waits, ex.Wait); err != nil {
		return err
	}
	if want := fmt.Sprintf("Needs you (%d)", ex.Counts.Blocked); !strings.Contains(p.WaitsTxt, want) {
		return fail("plain Waits header lacks %q", want)
	}
	for _, want := range []string{"Decision needed", "Approval needed", "Question for you", "Job paused, needs you", "2 tasks need you"} {
		if !strings.Contains(p.WaitsTxt, want) {
			return fail("plain Waits lacks %q", want)
		}
	}
	if !strings.Contains(p.OutTxt, "Done") {
		return fail("plain Outcomes lacks Done")
	}
	for _, bad := range []string{"permission prompt", "failed check"} {
		if strings.Contains(p.WaitsTxt, bad) {
			return fail("plain Waits shows %q", bad)
		}
	}
	for _, r := range append(append([]d36Row{}, p.Waits...), p.Outcomes...) {
		for _, lt := range r.LinkText {
			if lt != "request" && lt != "result" {
				return fail("plain link text %q", lt)
			}
		}
	}
	if _, err := screenshot(hwnd, "d36-plain"); err != nil {
		return err
	}

	if p, err = apply(false, raw, "12"); err != nil {
		return err
	}
	const aged = "snapshot 12 s old, this list may be behind"
	if !p.WAged || !p.OAged || !strings.Contains(p.WaitsTxt, aged) || !strings.Contains(p.OutTxt, aged) {
		return fail("age 12 does not mark both panels aged: %+v", p)
	}
	if _, err := screenshot(hwnd, "d36-aged"); err != nil {
		return err
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	m["wait_items"], m["outcome_items"] = json.RawMessage("[]"), json.RawMessage("[]")
	empty, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if p, err = apply(false, empty, "7"); err != nil {
		return err
	}
	if !strings.Contains(p.WaitsTxt, "Nothing waits on you.") || !strings.Contains(p.OutTxt, "Nothing finished or failed in the last 24 hours.") {
		return fail("standard empty texts wrong: %q / %q", p.WaitsTxt, p.OutTxt)
	}
	if p, err = apply(true, empty, "7"); err != nil {
		return err
	}
	if !strings.Contains(p.WaitsTxt, "Nothing is waiting for you.") || !strings.Contains(p.OutTxt, "Nothing finished in the last 24 hours.") {
		return fail("plain empty texts wrong: %q / %q", p.WaitsTxt, p.OutTxt)
	}

	if _, err := apply(false, raw, "7"); err != nil {
		return err
	}
	var gate d36Item
	for _, it := range ex.Wait {
		if it.Kind == "gate" && it.SpecPath != "" {
			gate = it
			break
		}
	}
	if gate.SpecPath == "" {
		return fail("the example has no gate with a spec path")
	}
	var got []string
	if err := evalInto(d36ClickJS(gate.ID), &got); err != nil {
		return err
	}
	if len(got) != 1 || got[0] != gate.SpecPath {
		return fail("click sent %v, want [%s]", got, gate.SpecPath)
	}
	return nil
}

// waits/d37: moved from check_d37.go.

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
	raw, err := readSnapshotExample()
	if err != nil {
		return err
	}
	fail := func(format string, a ...any) error { return fmt.Errorf("d37: "+format, a...) }
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

// waits/d39: moved from check_d39.go.

// d39: Waits on you is the ranked your-move queue (B6). The two control
// bindings are REPLACED by recorders, so no real control is ever pressed. Fed
// the frozen example (wait_counts blocked 7, your_turn 1, done 2), its
// close_out row marked close_ok (Foreman would accept), it proves:
//   - the tier headers come in the order Blocked, Your turn, Done needs
//     close-out (one Done tier holding the close_out and the can_close row),
//     and the panel header count is 7;
//   - with the Done group opened (it starts collapsed, d44), the Close chip
//     is on the close_out row only, and one press previews
//     {control: close_sprint, target: closeout:example__sprint__9};
//   - with controls.foreman false and the close_out row removed: no Close
//     chip, the Done tier is headed Done, can be closed, every gear line
//     marked data-ctl-needs=foreman is hidden, and no visible element in the
//     panel or the gear carries foreman_root error text (the payload gives
//     the Foreman controls a not-ready reason, so the check is not vacuous).
//
// Screenshot: d39-waits.
const d39Recorders = `(function(){
  window.__d39prev = [];
  window.bdevControlPreview = function(r){
    window.__d39prev.push(JSON.parse(JSON.stringify(r)));
    return Promise.resolve({token:'tok-' + r.target, command:'C:\\example\\python.exe sprint_step.py', label:r.target, expires_in_ms:5000});
  };
  window.bdevControlRun = function(t){ return Promise.resolve({exit_code:0, line:'ok'}); };
  return true;
})()`

const d39ReadJS = `(function(){
  var tiers = Array.prototype.map.call(document.querySelectorAll('[data-sec=waits] .sectier'), function(e){ return e.textContent; });
  var chips = Array.prototype.map.call(document.querySelectorAll('[data-sec=waits] .ctl[data-ctl=close_sprint]'), function(e){
    return e.closest('.secitem').getAttribute('data-id'); });
  var needsAll = Array.prototype.slice.call(document.querySelectorAll('[data-ctl-needs=foreman]'));
  var needs = needsAll[0];
  var errs = [];
  Array.prototype.forEach.call(document.querySelectorAll('#secControls [data-ctl-why]'), function(e){
    var h = e.closest('[data-ctl-needs]'), hid = h && getComputedStyle(h).display === 'none';
    if(!hid && /foreman_root/.test(e.textContent)) errs.push(e.textContent);
  });
  var wp = document.querySelector('[data-sec=waits]');
  if(wp && /foreman_root/.test(wp.textContent)) errs.push('waits panel: ' + wp.textContent.slice(0, 80));
  var head = document.querySelector('[data-sec=waits] .panelhead');
  return { tiers: tiers, chips: chips, head: head ? head.textContent : '',
    gearHidden: needsAll.length ? needsAll.every(function(e){ return getComputedStyle(e).display === 'none'; }) : null,
    gearShown: needsAll.length ? needsAll.every(function(e){ return getComputedStyle(e).display !== 'none'; }) : null,
    gearCount: needsAll.length, errs: errs,
    gearText: needs ? needs.textContent.trim() : '',
    prevs: window.__d39prev.slice() };
})()`

type d39Page struct {
	Tiers      []string         `json:"tiers"`
	Chips      []string         `json:"chips"`
	Head       string           `json:"head"`
	GearHidden *bool            `json:"gearHidden"`
	GearShown  *bool            `json:"gearShown"`
	GearCount  int              `json:"gearCount"`
	Errs       []string         `json:"errs"`
	GearText   string           `json:"gearText"`
	Prevs      []map[string]any `json:"prevs"`
}

func caseD39(hwnd uintptr, args []string) error {
	raw, err := readSnapshotExample()
	if err != nil {
		return err
	}
	fail := func(format string, a ...any) error { return fmt.Errorf("d39: "+format, a...) }
	on := map[string]bool{"close_sprint": true}
	payload := func(foreman bool, drop string) string {
		var m map[string]any
		if err := json.Unmarshal([]byte(d29Payload(d29Opts{Plain: false, Scrollbar: true, Status: "ok", Snap: raw})), &m); err != nil {
			panic(err)
		}
		snap, _ := m["reading"].(map[string]any)["snapshot"].(map[string]any)
		var keep []any
		for _, it := range snap["wait_items"].([]any) {
			item := it.(map[string]any)
			if item["kind"] == drop {
				continue
			}
			// The Close chip shows only when Foreman would accept the close
			// (page.html secItemRow closeable, since 087452c); the frozen
			// example's close_out row has a report but no close_ok.
			if item["kind"] == "close_out" {
				item["close_ok"] = true
			}
			keep = append(keep, it)
		}
		snap["wait_items"] = keep
		ready, why := on, map[string]string{}
		if !foreman {
			ready = map[string]bool{}
			for _, c := range []string{"approve", "send_back", "close_sprint"} {
				why[c] = "needs foreman_root in burnmon.json"
			}
		}
		m["controls"] = map[string]any{"foreman": foreman, "on": on, "ready": ready, "why": why}
		b, _ := json.Marshal(m)
		return string(b)
	}
	read := func() (d39Page, error) {
		var p d39Page
		time.Sleep(150 * time.Millisecond)
		err := evalInto(d39ReadJS, &p)
		return p, err
	}
	if _, err := evalRaw(d39Recorders); err != nil {
		return err
	}
	if _, err := d29Apply(payload(true, "")); err != nil {
		return err
	}
	if err := waitsOpenDone(); err != nil {
		return fail("%v", err)
	}
	p, err := read()
	if err != nil {
		return err
	}
	wantTiers := []string{"Blocked (7)", "Your turn (1)", "Done, needs close-out (2)"}
	if strings.Join(p.Tiers, "|") != strings.Join(wantTiers, "|") {
		return fail("tiers %v, want %v", p.Tiers, wantTiers)
	}
	if !strings.Contains(p.Head, "(7)") || !strings.Contains(p.Head, "1 your turn") {
		return fail("panel header %q, want the count 7 and 1 your turn", p.Head)
	}
	if len(p.Chips) != 1 || p.Chips[0] != "closeout:example__sprint__9" {
		return fail("Close chips on %v, want only closeout:example__sprint__9", p.Chips)
	}
	if p.GearShown == nil || !*p.GearShown || p.GearCount != 3 {
		return fail("the Foreman gear lines (%d) are not all shown with foreman true", p.GearCount)
	}
	if _, err := screenshot(hwnd, "d39-waits"); err != nil {
		return err
	}
	if _, err := evalRaw(`document.querySelector('[data-sec=waits] .ctl[data-ctl=close_sprint]').click()`); err != nil {
		return err
	}
	if p, err = read(); err != nil {
		return err
	}
	if len(p.Prevs) != 1 || len(p.Prevs[0]) != 2 || p.Prevs[0]["control"] != "close_sprint" || p.Prevs[0]["target"] != "closeout:example__sprint__9" {
		return fail("press sent %v, want exactly {close_sprint, closeout:example__sprint__9}", p.Prevs)
	}

	// No Foreman: no Close chip, no close-out tier, the gear line hidden, no error text.
	// The Done group is open, so the chip check sees the can_close row.
	if _, err := d29Apply(payload(false, "close_out")); err != nil {
		return err
	}
	if err := waitsOpenDone(); err != nil {
		return fail("%v", err)
	}
	if p, err = read(); err != nil {
		return err
	}
	if len(p.Chips) != 0 {
		return fail("Close chips %v without foreman_root", p.Chips)
	}
	if len(p.Tiers) == 0 || p.Tiers[len(p.Tiers)-1] != "Done, can be closed (1)" {
		return fail("tiers %v without foreman_root, want the last to be Done, can be closed (1)", p.Tiers)
	}
	for _, t := range p.Tiers {
		if strings.Contains(t, "close-out") {
			return fail("close-out tier %q without foreman_root", t)
		}
	}
	if p.GearHidden == nil || !*p.GearHidden || p.GearCount != 3 {
		return fail("a Foreman gear line (%d) is shown without foreman_root", p.GearCount)
	}
	if len(p.Errs) != 0 {
		return fail("error text visible without foreman_root: %v", p.Errs)
	}
	return nil
}

// waits/d44: the Done group starts collapsed (B8 spec item 3).

// d44: the Done group (close-out and branch-gone items) starts collapsed with
// its count, so Blocked and Your turn are what shows first. Fed the frozen
// example cut to 2 Blocked, 1 Your turn and 3 Done items (its close_out and
// can_close rows plus a branch_gone row) it proves:
//   - the Blocked and Your turn rows are in the DOM, the Done header reads
//     "Done, needs close-out (3)" as a button (role, tabindex 0) with
//     aria-expanded false, and no Done row is rendered;
//   - a real click on the header shows the 3 Done rows, aria-expanded true;
//   - the same payload applied again (what a poll does) leaves it open;
//   - Enter on the focused header closes it, Space opens it again, and focus
//     stays on the header although the render replaces it;
//   - the panel header, with its amber number (wait_counts.blocked), reads
//     the same throughout.
//
// Screenshot: d44-collapsed.

// VK_RETURN and VK_SPACE, the keys a button answers to.
const (
	vkReturn = 0x0D
	vkSpace  = 0x20
)

const d44ReadJS = `(function(){
  var p = document.querySelector('[data-sec=waits]');
  if(!p) return { rows: [], tiers: [], head: '' };
  var h = p.querySelector('.sectier.done');
  return { rows: Array.prototype.map.call(p.querySelectorAll('.secitem'), function(r){ return r.getAttribute('data-id'); }),
    tiers: Array.prototype.map.call(p.querySelectorAll('.sectier'), function(e){ return e.textContent; }),
    head: p.querySelector('.panelhead').textContent,
    role: h ? h.getAttribute('role') || '' : '', tab: h ? h.getAttribute('tabindex') || '' : '',
    expanded: h ? h.getAttribute('aria-expanded') || '' : '', focused: !!h && document.activeElement === h };
})()`

type d44Page struct {
	Rows     []string `json:"rows"`
	Tiers    []string `json:"tiers"`
	Head     string   `json:"head"`
	Role     string   `json:"role"`
	Tab      string   `json:"tab"`
	Expanded string   `json:"expanded"`
	Focused  bool     `json:"focused"`
}

func caseD44(hwnd uintptr, args []string) error {
	raw, err := readSnapshotExample()
	if err != nil {
		return err
	}
	fail := func(format string, a ...any) error { return fmt.Errorf("d44: "+format, a...) }
	var snap map[string]any
	if err := json.Unmarshal(raw, &snap); err != nil {
		return err
	}
	byID := map[string]any{}
	for _, it := range snap["wait_items"].([]any) {
		byID[it.(map[string]any)["id"].(string)] = it
	}
	first := []string{"gate:example__flow__1:G4", "sprint:example__sprint-example__1", "session:anthropic:example-idle"}
	done := []string{"closeout:example__sprint__9", "canclose:session:anthropic:example-merged", "branchgone:example__sprint__8"}
	all := append(append([]string{}, first...), done...)
	var items []any
	for _, id := range all[:5] {
		it, ok := byID[id]
		if !ok {
			return fail("the example has no wait item %q", id)
		}
		items = append(items, it)
	}
	// The example has no branch_gone row; this one has the shape waits.go gives it.
	items = append(items, map[string]any{"kind": "branch_gone", "id": done[2], "title": "example-gone", "project": "example",
		"detail": "branch gone, check by hand", "since": "2026-10-23T07:00:00Z", "age_seconds": 9000, "source": "foreman",
		"group": "done", "holds": 1})
	snap["wait_items"] = items
	snap["wait_counts"] = map[string]int{"blocked": 2, "your_turn": 1, "done": 3}
	sb, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	payload := d29Payload(d29Opts{Plain: false, Scrollbar: true, Status: "ok", Snap: sb})
	read := func() (d44Page, error) {
		var p d44Page
		err := evalInto(d44ReadJS, &p)
		return p, err
	}
	// waitFor reads until the Done header has aria-expanded want, for up to 2 s.
	waitFor := func(want string) (d44Page, error) {
		for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(100 * time.Millisecond) {
			p, err := read()
			if err != nil || p.Expanded == want || time.Now().After(deadline) {
				return p, err
			}
		}
	}
	var head string
	check := func(label string, p d44Page, expanded string, rows []string) error {
		if p.Expanded != expanded {
			return fail("%s: aria-expanded %q, want %q", label, p.Expanded, expanded)
		}
		if strings.Join(p.Rows, "|") != strings.Join(rows, "|") {
			return fail("%s: rows %v, want %v", label, p.Rows, rows)
		}
		if p.Head != head {
			return fail("%s: panel header %q, was %q", label, p.Head, head)
		}
		return nil
	}

	if _, err := d29Apply(payload); err != nil {
		return err
	}
	p, err := read()
	if err != nil {
		return err
	}
	head = p.Head
	if !strings.Contains(head, "Waits on you (2)") || !strings.Contains(head, "1 your turn") {
		return fail("panel header %q, want the amber number 2 and 1 your turn", head)
	}
	wantTiers := []string{"Blocked (2)", "Your turn (1)", "Done, needs close-out (3)"}
	if strings.Join(p.Tiers, "|") != strings.Join(wantTiers, "|") {
		return fail("tiers %v, want %v", p.Tiers, wantTiers)
	}
	if p.Role != "button" || p.Tab != "0" {
		return fail("Done header role %q, tabindex %q, want button and 0", p.Role, p.Tab)
	}
	if err := check("at the start", p, "false", first); err != nil {
		return err
	}
	if _, err := screenshot(hwnd, "d44-collapsed"); err != nil {
		return err
	}

	if err := clickSelector(hwnd, "[data-sec=waits] .sectier.done"); err != nil {
		return fail("click the Done header: %v", err)
	}
	if p, err = waitFor("true"); err != nil {
		return err
	}
	if err := check("after a click", p, "true", all); err != nil {
		return err
	}

	if _, err := d29Apply(payload); err != nil {
		return err
	}
	if p, err = read(); err != nil {
		return err
	}
	if err := check("after the payload again", p, "true", all); err != nil {
		return err
	}

	// Keys go to the foreground window: put it in front before each one.
	key := func(name string, vk uint16, expanded string, rows []string) error {
		if _, err := evalRaw(`(document.querySelector('[data-sec=waits] .sectier.done').focus(), 1)`); err != nil {
			return err
		}
		bringToFront(hwnd)
		if err := pressKey(vk); err != nil {
			return fail("%s: press: %v", name, err)
		}
		p, err := waitFor(expanded)
		if err != nil {
			return err
		}
		if err := check("after "+name, p, expanded, rows); err != nil {
			return err
		}
		if !p.Focused {
			return fail("after %s the Done header does not have focus", name)
		}
		return nil
	}
	if err := key("Enter", vkReturn, "false", first); err != nil {
		return err
	}
	return key("Space", vkSpace, "true", all)
}

package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// d36: Waits on you and Outcomes list the items (agent control plan row R9).
// Fed the UNMODIFIED frozen example (testdata\snapshot_example.json, the gate is not moved
// to the front) it proves:
//   - every wait item is a row in the DOM (the list scrolls inside the panel, none is
//     cut off), the header carries the count ("Waits on you (6)"), no +k more line;
//   - Standard: the Waits rows are the example's wait_items and the Outcomes
//     rows its outcome_items, in order; the gate row reads "gate G4", the
//     prompt row "permission prompt", the sprint row "2 tasks parked"; every
//     row is one line; each spec or result link carries the item's exact path
//     and a missing result reads "no result yet";
//   - Plain: the same rows in plain words, links read "request" and "result";
//   - a reading older than two ticks marks both panels aged with the age;
//     at age 7 neither is;
//   - empty lists read the empty texts;
//   - a click on the gate's spec link calls bdevOpenItemFile with that path
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

const d36ClickJS = `(function(){
  var old = window.bdevOpenItemFile, got = [];
  window.bdevOpenItemFile = function(p){ got.push(p); return Promise.resolve(); };
  var a = document.querySelector('[data-sec=waits] .secitem[data-kind=gate] .secfile');
  if(a) a.click();
  window.bdevOpenItemFile = old;
  return got;
})()`

func caseD36(hwnd uintptr, args []string) error {
	raw, err := readFixture("snapshot_example.json")
	if err != nil {
		return fmt.Errorf("read the frozen example (uicheck runs from the tools folder): %w", err)
	}
	var ex struct {
		Wait    []d36Item `json:"wait_items"`
		Outcome []d36Item `json:"outcome_items"`
	}
	if err := json.Unmarshal(raw, &ex); err != nil {
		return err
	}
	fail := func(format string, a ...any) error { return fmt.Errorf("d36: "+format, a...) }
	if err := ensureWindowSizeWH(hwnd, 1920, 1080); err != nil {
		return err
	}
	apply := func(plain bool, snap []byte, age string) (d36Page, error) {
		var p d36Page
		payload := d29Payload(d29Opts{Plain: plain, Scrollbar: true, Status: "ok", Snap: snap})
		payload = strings.Replace(payload, `"age_seconds":7`, `"age_seconds":`+age, 1)
		if _, err := d29Apply(payload); err != nil {
			return p, err
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
	// Nothing is cut off: every wait item is a row (sameRows above), the header
	// carries the total and there is no +k more line.
	if want := fmt.Sprintf("Waits on you (%d)", len(ex.Wait)); !strings.Contains(p.WaitsTxt, want) {
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
	if want := fmt.Sprintf("Needs you (%d)", len(ex.Wait)); !strings.Contains(p.WaitsTxt, want) {
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
		if it.Kind == "gate" {
			gate = it
		}
	}
	if gate.SpecPath == "" {
		return fail("the example has no gate with a spec path")
	}
	var got []string
	if err := evalInto(d36ClickJS, &got); err != nil {
		return err
	}
	if len(got) != 1 || got[0] != gate.SpecPath {
		return fail("click sent %v, want [%s]", got, gate.SpecPath)
	}
	return nil
}

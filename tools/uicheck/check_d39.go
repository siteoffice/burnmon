package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// d39: Waits on you is the ranked your-move queue (B6). The two control
// bindings are REPLACED by recorders, so no real control is ever pressed. Fed
// the frozen example (wait_counts blocked 7, your_turn 1, done 2) it proves:
//   - the tier headers come in the order Blocked, Your turn, Done needs
//     close-out (one Done tier holding the close_out and the can_close row),
//     and the panel header count is 7;
//   - the Close chip is on the close_out row only, and one press previews
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
	raw, err := readFixture("snapshot_example.json")
	if err != nil {
		return fmt.Errorf("read the frozen example (uicheck runs from the tools folder): %w", err)
	}
	fail := func(format string, a ...any) error { return fmt.Errorf("d39: "+format, a...) }
	if err := ensureWindowSizeWH(hwnd, 1920, 1080); err != nil {
		return err
	}
	on := map[string]bool{"close_sprint": true}
	payload := func(foreman bool, drop string) string {
		var m map[string]any
		if err := json.Unmarshal([]byte(d29Payload(d29Opts{Plain: false, Scrollbar: true, Status: "ok", Snap: raw})), &m); err != nil {
			panic(err)
		}
		if drop != "" {
			snap, _ := m["reading"].(map[string]any)["snapshot"].(map[string]any)
			var keep []any
			for _, it := range snap["wait_items"].([]any) {
				if it.(map[string]any)["kind"] != drop {
					keep = append(keep, it)
				}
			}
			snap["wait_items"] = keep
		}
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
	if _, err := d29Apply(payload(false, "close_out")); err != nil {
		return err
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

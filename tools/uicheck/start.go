package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// startState is what a burnmon-dev.exe case starts from (B7.2 spec item 3:
// each check sets its own starting state, so check order never matters).
// applyStart sets it through the page's own bindings, then reloads the page,
// so fake mode, a frozen __bdevSections payload, an open map or gear dialog
// and an in-page theme toggle from an earlier case are gone too.
type startState struct {
	// Width and Height are the window's CSS size (ensureWindowSizeWH).
	Width, Height int32
	// Plain is the mode: false Standard, true Plain.
	Plain bool
	// Scrollbar is the Overall scrollbar flag.
	Scrollbar bool
	// Standard and PlainSet are the part switches per mode, part id to on.
	Standard, PlainSet map[string]bool
	// Theme is the L key's choice, "dark" or "light".
	Theme string
}

// devPartIDs mirrors sections.IDs (internal\sections\sections.go Groups), in
// group order. uicheck is its own module and cannot import internal\, so
// applyStart compares this list with the live bdevSections groups and stops
// on any difference instead of silently skipping a new part.
var devPartIDs = []string{
	"waits", "outcomes", "burn", "git", "agents_map", "sessions",
	"burn_30min", "vendor_strip", "activity", "heatmap", "turn_ticker",
	"system", "process_groups", "processes",
	"todo",
}

// devControlIDs mirrors sections.ControlIDs.
var devControlIDs = []string{"stop", "attach", "approve", "send_back", "close_sprint"}

// appDefaultPlain is the Plain list a fresh settings folder gives
// (sections.Default, internal\sections\sections.go:170).
var appDefaultPlain = map[string]bool{"waits": true, "outcomes": true, "burn": true, "agents_map": true, "sessions": true}

// appStart is what a fresh settings folder gives, which is what every run
// starts with since scripts\uicheck-dev.ps1 makes one per launch:
// sections.Default (internal\sections\sections.go:169) is Standard, scrollbar
// on, every Standard part on, the five Plain parts, every control off and no
// saved heights; loadUITheme (cmd\burnmon-dev\viewstate.go:29) is dark. The
// size is the primary viewport (1152x2048, check_d2.go:16); groups override it.
func appStart() startState {
	s := startState{Width: 1152, Height: 2048, Scrollbar: true, Theme: "dark",
		Standard: map[string]bool{}, PlainSet: map[string]bool{}}
	for _, id := range devPartIDs {
		s.Standard[id] = true
		s.PlainSet[id] = appDefaultPlain[id]
	}
	return s
}

// sizedStart is appStart at another window size, a group's default.
func sizedStart(w, h int32) startState {
	s := appStart()
	s.Width, s.Height = w, h
	return s
}

// clone copies s with its own maps, so a case override never edits the
// group default.
func (s startState) clone() startState {
	out := s
	out.Standard, out.PlainSet = map[string]bool{}, map[string]bool{}
	for k, v := range s.Standard {
		out.Standard[k] = v
	}
	for k, v := range s.PlainSet {
		out.PlainSet[k] = v
	}
	return out
}

// devCaseStart holds the cases that need something other than their group's
// default; each edits its own copy. d24 sets its size here rather than
// resizing itself, so the focus applyStart gives is at the size its key
// presses run at.
var devCaseStart = map[string]func(*startState){
	"d24": func(s *startState) { s.Width, s.Height = 1600, 900 },
}

// caseStart is g's default with c's override applied.
func caseStart(g devGroup, c devCase) startState {
	s := g.Start.clone()
	if f := devCaseStart[c.Name]; f != nil {
		f(&s)
	}
	return s
}

// startSaved is the part of a bdevSections payload applyStart compares.
type startSaved struct {
	Plain     bool            `json:"plain"`
	Scrollbar bool            `json:"scrollbar"`
	Standard  map[string]bool `json:"standard"`
	PlainSet  map[string]bool `json:"plainSet"`
	Controls  map[string]bool `json:"controls"`
	Parts     []string        `json:"parts"`
}

// startChanges lists the bdevSetSections changes (as JS object literals) that
// take cur to want, in a fixed order: heights reset, controls off, part
// switches, scrollbar, mode. The heights reset is always sent: it is one
// cheap write, and the saved split is not worth comparing key by key.
func startChanges(cur startSaved, want startState) []string {
	ch := []string{`{reset_heights:true}`}
	for _, id := range devControlIDs {
		if cur.Controls[id] {
			ch = append(ch, fmt.Sprintf(`{control:%s,on:false}`, jsString(id)))
		}
	}
	for _, id := range devPartIDs {
		if cur.Standard[id] != want.Standard[id] {
			ch = append(ch, fmt.Sprintf(`{mode:'standard',section:%s,on:%t}`, jsString(id), want.Standard[id]))
		}
		if cur.PlainSet[id] != want.PlainSet[id] {
			ch = append(ch, fmt.Sprintf(`{mode:'plain',section:%s,on:%t}`, jsString(id), want.PlainSet[id]))
		}
	}
	if cur.Scrollbar != want.Scrollbar {
		ch = append(ch, fmt.Sprintf(`{scrollbar:%t}`, want.Scrollbar))
	}
	if cur.Plain != want.Plain {
		ch = append(ch, fmt.Sprintf(`{plain:%t}`, want.Plain))
	}
	return ch
}

// samePartIDs reports whether live (the bdevSections groups) holds exactly
// devPartIDs.
func samePartIDs(live []string) bool {
	a := append([]string(nil), live...)
	b := append([]string(nil), devPartIDs...)
	sort.Strings(a)
	sort.Strings(b)
	return strings.Join(a, ",") == strings.Join(b, ",")
}

// evalAwait runs js, an expression that may return a promise, and waits up to
// 15 s for it to settle; the result is JSON-decoded into v when v is not nil.
func evalAwait(js string, v any) error {
	start := `(window.__uicheckAwait = null, Promise.resolve().then(function(){ return (` + js + `); }).then(` +
		`function(r){ window.__uicheckAwait = {ok:true, v:JSON.stringify(r === undefined ? null : r)}; },` +
		`function(e){ window.__uicheckAwait = {ok:false, e:String(e)}; }), 1)`
	if _, err := evalRaw(start); err != nil {
		return err
	}
	var res struct {
		OK bool   `json:"ok"`
		V  string `json:"v"`
		E  string `json:"e"`
	}
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		raw, err := evalRaw(`window.__uicheckAwait`)
		if err != nil {
			return err
		}
		if string(raw) == "null" {
			continue
		}
		if err := json.Unmarshal(raw, &res); err != nil {
			return err
		}
		if !res.OK {
			return fmt.Errorf("%s", res.E)
		}
		if v == nil {
			return nil
		}
		return json.Unmarshal([]byte(res.V), v)
	}
	return fmt.Errorf("no answer in 15 s")
}

const startReadJS = `window.bdevSections().then(function(p){
  var parts = [];
  (p.groups || []).forEach(function(g){ (g.parts || []).forEach(function(x){ parts.push(x.id); }); });
  return {plain: !!p.plain_mode, scrollbar: !!p.overall_scrollbar, standard: p.parts_standard || {}, plainSet: p.parts_plain || {},
    controls: (p.controls && p.controls.on) || {}, parts: parts};
})`

// startReadyJS reads the reloaded page: its first tick has landed (the
// loading overlay is hidden), the sections poll has applied, and the theme.
const startReadyJS = `(function(){
  var o = document.getElementById('loadingOverlay');
  var s = window.__bdevSections ? window.__bdevSections.state() : null;
  return {painted: !!o && o.classList.contains('hidden'), sections: !!s && (s.groups || []).length > 0,
    plain: !!s && s.plain, scrollbar: !!s && s.scrollbar, light: document.documentElement.getAttribute('data-theme') === 'light',
    marker: window.__uicheckStart || ''};
})()`

// applyStartFn is what the runner calls; tests swap it for a recorder.
var applyStartFn = applyStart

// applyStart puts the app and the window into s: settings through
// bdevSetSections and bdevSetUITheme (the bindings the gear dialog and the L
// key use), a real reload of the page, the window size, then the window in
// front with keyboard focus in the page. It writes settings, so it refuses to
// run unless BURNMON_DEV_SETTINGS_DIR points burnmon-dev.exe at a temp folder.
func applyStart(hwnd uintptr, s startState) error {
	if _, err := devSettingsDir(); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	var cur startSaved
	if err := evalAwait(startReadJS, &cur); err != nil {
		return fmt.Errorf("start: read bdevSections: %w", err)
	}
	if !samePartIDs(cur.Parts) {
		return fmt.Errorf("start: the app's parts %v differ from devPartIDs %v; update start.go", cur.Parts, devPartIDs)
	}
	chain := "Promise.resolve()"
	for _, c := range startChanges(cur, s) {
		chain += ".then(function(){ return window.bdevSetSections(" + c + "); })"
	}
	chain += ".then(function(){ return window.bdevSetUITheme(" + jsString(s.Theme) + "); })"
	if err := evalAwait(chain, nil); err != nil {
		return fmt.Errorf("start: save settings: %w", err)
	}

	// The marker proves the page answering below is the reloaded one.
	if _, err := evalRaw(`(window.__uicheckStart = 'before the reload', 1)`); err != nil {
		return fmt.Errorf("start: mark the page: %w", err)
	}
	if err := evalReload(); err != nil {
		return fmt.Errorf("start: reload: %w", err)
	}
	var ready struct {
		Painted, Sections, Plain, Scrollbar, Light bool
		Marker                                     string
	}
	ok := false
	var lastErr error
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		if lastErr = evalInto(startReadyJS, &ready); lastErr != nil {
			continue // the old page may be going away mid-call
		}
		if ready.Marker == "" && ready.Painted && ready.Sections && ready.Plain == s.Plain && ready.Scrollbar == s.Scrollbar &&
			ready.Light == (s.Theme == "light") {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("start: the reloaded page never reached the start state in 30 s (last %+v, err %v)", ready, lastErr)
	}

	if err := ensureWindowSizeWH(hwnd, s.Width, s.Height); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	time.Sleep(300 * time.Millisecond)
	if err := focusPage(hwnd); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	return nil
}

// focusPage brings the window to the front and gives the page keyboard focus
// with a real click on the header logo, then waits until the page says it
// has focus. Key presses (d7, d23, d24, d25, d28) go to whatever has focus,
// so a case must not start before this holds. "header .logo", not ".logo":
// the loading overlay's own logo comes first in the page and is display:none
// once the page has painted, so ".logo" gives a zero rect at the window's
// top left corner.
func focusPage(hwnd uintptr) error {
	var lastErr error
	for try := 1; try <= 3; try++ {
		bringToFront(hwnd)
		if err := clickSelectorAt(hwnd, "header .logo", 0.5, 0.5); err != nil {
			lastErr = fmt.Errorf("click header .logo: %w", err)
		} else {
			for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
				has, err := evalBool(`document.hasFocus()`)
				if err != nil {
					lastErr = err
					break
				}
				if has {
					return nil
				}
				lastErr = fmt.Errorf("the page does not have focus after the click")
			}
		}
		fmt.Printf("uicheck: focus try %d: %v\n", try, lastErr)
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("could not give the page keyboard focus (%v); is the desktop locked or another window holding the foreground?", lastErr)
}

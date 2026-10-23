package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// d30: the agent map (agent control plan, row R7). Fed lane A's frozen
// example and the view internal\agentmap builds from it
// (testdata\agentmap_example.json, kept in step by a Go test) it proves:
//   - the Agents map section is a small read-only overview: counts, the sprint
//     and its agent total, and no animation anywhere inside it;
//   - a click on it opens the full map in the Station's layer, with the sprint,
//     the session, its subagent nested under it, the MCP server with its port,
//     the repo, the branch and the "waits on you" mark;
//   - a card click shows its detail line (a live session with turns opens the
//     Station's compact turn card instead, not testable on a frozen example);
//   - the past-window tabs draw a view handed in from the store's side;
//   - M and P swap the map and the Station, Esc closes the map, and the map
//     animates nothing;
//   - Plain Mode uses plain words, and a missing snapshot offers the last 24
//     hours instead of drawing old numbers.
//
// The page is driven through window.__bdevSections.apply and window.__bdevMap,
// which draw without saving or calling the store. Screenshots: d30-overview,
// d30-map, d30-map-plain, d30-map-history.
type d30Page struct {
	Overview  string   `json:"overview"`
	HasNodes  bool     `json:"hasNodes"`
	Anims     int      `json:"anims"`
	MapOn     bool     `json:"mapOn"`
	StationOn bool     `json:"stationOn"`
	Text      string   `json:"text"`
	Src       string   `json:"src"`
	Cards     int      `json:"cards"`
	Nested    int      `json:"nested"`
	Details   int      `json:"details"`
	Waits     int      `json:"waits"`
	TabsOn    []string `json:"tabsOn"`
	BodyOver  float64  `json:"bodyOver"`
}

const d30ReadJS = `(function(){
  var q = function(s){ return document.querySelector(s); };
  var slot = document.getElementById('secAgentsSlot'), ov = document.getElementById('mapOverlay');
  var body = document.getElementById('mapBody');
  var tabs = [];
  Array.prototype.forEach.call(document.querySelectorAll('#mapTabs button[aria-pressed="true"]'), function(b){ tabs.push(b.getAttribute('data-src')); });
  return {
    overview: slot ? slot.textContent : '',
    hasNodes: !!(slot && slot.querySelector('.mapcard')),
    anims: slot && slot.getAnimations ? slot.getAnimations({subtree:true}).length + (ov.getAnimations ? ov.getAnimations({subtree:true}).length : 0) : -1,
    mapOn: ov.classList.contains('on'),
    stationOn: document.getElementById('stationOverlay').classList.contains('on'),
    text: body.textContent,
    src: document.getElementById('mapSrc').textContent,
    cards: body.querySelectorAll('.mapcard').length,
    nested: body.querySelectorAll('.mapkids .mapcard').length,
    details: body.querySelectorAll('.mapdetail').length,
    waits: body.querySelectorAll('.mapcard.wait').length,
    tabsOn: tabs,
    bodyOver: document.body.scrollHeight - document.body.clientHeight
  };
})()`

func d30Fail(format string, args ...any) error { return fmt.Errorf("d30: "+format, args...) }

func d30Read() (d30Page, error) {
	var p d30Page
	time.Sleep(400 * time.Millisecond)
	err := evalInto(d30ReadJS, &p)
	return p, err
}

func d30Eval(js string) error {
	_, err := evalRaw(js)
	time.Sleep(300 * time.Millisecond)
	return err
}

func d30Key(key string) error {
	return d30Eval(`document.dispatchEvent(new KeyboardEvent('keydown', {key:` + fmt.Sprintf("%q", key) + `, bubbles:true}))`)
}

func init() {
	checks["d30"] = func(hwnd uintptr, args []string) error {
		root := filepath.Join("..", "..", "testdata")
		snap, err := os.ReadFile(filepath.Join(root, "snapshot_example.json"))
		if err != nil {
			return fmt.Errorf("read the frozen example (uicheck runs from the tools folder): %w", err)
		}
		view, err := os.ReadFile(filepath.Join(root, "agentmap_example.json"))
		if err != nil {
			return fmt.Errorf("read the map example: %w", err)
		}
		if err := ensureWindowSizeWH(hwnd, 1152, 2048); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)

		// payload is d29's, with the map added as the Go side adds it.
		payload := func(plain bool, status string, withSnap bool) (string, error) {
			var m map[string]any
			var s json.RawMessage
			if withSnap {
				s = snap
			}
			if err := json.Unmarshal([]byte(d29Payload(plain, nil, false, status, s)), &m); err != nil {
				return "", err
			}
			if withSnap {
				m["agent_map"] = json.RawMessage(view)
			} else {
				m["agent_map"] = nil
			}
			b, err := json.Marshal(m)
			return string(b), err
		}
		apply := func(plain bool, status string, withSnap bool) (d30Page, error) {
			var p d30Page
			js, err := payload(plain, status, withSnap)
			if err != nil {
				return p, err
			}
			// The eval channel takes one expression, so the two calls share a function.
			if err := d30Eval(`(function(){ window.__bdevMap.close(); window.__bdevSections.apply(` + js + `); })()`); err != nil {
				return p, err
			}
			return d30Read()
		}

		// The overview: counts and the sprint, read-only, nothing animating.
		p, err := apply(false, "ok", true)
		if err != nil {
			return err
		}
		for _, want := range []string{"1 session,", "1 subagent,", "1 waiting on you", "sprint-example", "Open the full map"} {
			if !strings.Contains(p.Overview, want) {
				return d30Fail("overview lacks %q: %q", want, p.Overview)
			}
		}
		if p.HasNodes || p.Anims > 0 || p.MapOn {
			return d30Fail("overview is not a still, read-only summary: cards %v, animations %d, map open %v", p.HasNodes, p.Anims, p.MapOn)
		}
		if _, err := screenshot(hwnd, "d30-overview"); err != nil {
			return err
		}

		// A click opens the full map in the Station's layer.
		if err := d30Eval(`document.getElementById('secAgentsSlot').click()`); err != nil {
			return err
		}
		if p, err = d30Read(); err != nil {
			return err
		}
		if !p.MapOn || p.StationOn {
			return d30Fail("click on the overview: map open %v, station open %v", p.MapOn, p.StationOn)
		}
		for _, want := range []string{"sprint-example", "running", "Explore", "subagent", "waits on you", "example-server", "port 8765", "repo project", "branch sprint/example", "messages between agents"} {
			if !strings.Contains(p.Text, want) {
				return d30Fail("map lacks %q: %q", want, p.Text)
			}
		}
		if p.Cards != 2 || p.Nested != 1 || p.Waits != 1 {
			return d30Fail("map cards %d (want 2), nested %d (want 1), waiting %d (want 1)", p.Cards, p.Nested, p.Waits)
		}
		if len(p.TabsOn) != 1 || p.TabsOn[0] != "live" || !strings.Contains(p.Src, "snapshot 7 s old") {
			return d30Fail("live tab %v, source line %q", p.TabsOn, p.Src)
		}
		if p.Anims > 0 {
			return d30Fail("the open map animates (%d animations)", p.Anims)
		}
		if _, err := screenshot(hwnd, "d30-map"); err != nil {
			return err
		}

		// A card click shows its detail line, a second click hides it.
		for i, want := range []int{1, 0} {
			if err := d30Eval(`document.querySelector('#mapBody .mapcard').click()`); err != nil {
				return err
			}
			if p, err = d30Read(); err != nil {
				return err
			}
			if p.Details != want {
				return d30Fail("card click %d: %d detail lines, want %d", i+1, p.Details, want)
			}
		}

		// A past window handed in from the store's side.
		if err := d30Eval(`window.__bdevMap.setHistory({source:'history', hours:24, note:'', view:` + string(view) + `})`); err != nil {
			return err
		}
		if p, err = d30Read(); err != nil {
			return err
		}
		if !strings.Contains(p.Src, "last 24 h") || p.Cards != 2 || !strings.Contains(p.Text, "last seen") {
			return d30Fail("history draw: source %q, cards %d", p.Src, p.Cards)
		}
		if _, err := screenshot(hwnd, "d30-map-history"); err != nil {
			return err
		}

		// The real binding on the real store: no error, a view that draws.
		// Which sessions it holds depends on the machine, so this asserts the
		// shape and reports the counts. The screenshot holds real project
		// names: it stays in testdata\uicheck\out and is not for the README.
		if err := d30Eval(`(function(){ window.__d30 = null; window.bdevAgentMapHistory(168).then(function(r){ window.__d30 = r; }, function(e){ window.__d30 = {err: String(e)}; }); })()`); err != nil {
			return err
		}
		var real struct {
			Err  string `json:"err"`
			View struct {
				Groups []json.RawMessage `json:"groups"`
				Counts struct {
					Sessions  int `json:"sessions"`
					Subagents int `json:"subagents"`
					Sprints   int `json:"sprints"`
				} `json:"counts"`
			} `json:"view"`
			Hours int `json:"hours"`
		}
		for i := 0; ; i++ {
			raw, err := evalRaw(`window.__d30`)
			if err != nil {
				return err
			}
			if string(raw) != "null" {
				if err := json.Unmarshal(raw, &real); err != nil {
					return d30Fail("real history payload: %w", err)
				}
				break
			}
			if i >= 30 {
				return d30Fail("bdevAgentMapHistory(168) gave no answer in 30 s")
			}
			time.Sleep(time.Second)
		}
		if real.Err != "" || real.Hours != 168 {
			return d30Fail("real history: error %q, hours %d", real.Err, real.Hours)
		}
		fmt.Printf("d30: real store, last 168 h: %d groups, %d sessions, %d subagents, %d sprints\n",
			len(real.View.Groups), real.View.Counts.Sessions, real.View.Counts.Subagents, real.View.Counts.Sprints)
		if real.View.Counts.Sessions+real.View.Counts.Subagents == 0 {
			return d30Fail("the real store gave an empty map for 168 h; was the store copy empty?")
		}
		if err := d30Eval(`window.__bdevMap.setHistory({source:'history', hours:168, note:'', view: window.__d30.view})`); err != nil {
			return err
		}
		if p, err = d30Read(); err != nil {
			return err
		}
		if p.Cards == 0 || p.BodyOver > 1 {
			return d30Fail("real history draws %d cards, body overflow %.0f px", p.Cards, p.BodyOver)
		}
		if _, err := screenshot(hwnd, "d30-map-real-history"); err != nil {
			return err
		}

		// Esc closes the map. M opens it again; P (the Station) swaps it out.
		if err := d30Key("Escape"); err != nil {
			return err
		}
		if p, err = d30Read(); err != nil {
			return err
		}
		if p.MapOn {
			return d30Fail("Esc left the map open")
		}
		if err := d30Key("m"); err != nil {
			return err
		}
		if p, err = d30Read(); err != nil {
			return err
		}
		if !p.MapOn {
			return d30Fail("M did not open the map")
		}
		if err := d30Key("p"); err != nil {
			return err
		}
		if p, err = d30Read(); err != nil {
			return err
		}
		if p.MapOn || !p.StationOn {
			return d30Fail("P with the map open: map %v, station %v, want the Station only", p.MapOn, p.StationOn)
		}
		if err := d30Key("m"); err != nil {
			return err
		}
		if p, err = d30Read(); err != nil {
			return err
		}
		if !p.MapOn || p.StationOn {
			return d30Fail("M with the Station open: map %v, station %v, want the map only", p.MapOn, p.StationOn)
		}
		if err := d30Key("Escape"); err != nil {
			return err
		}

		// Plain Mode: plain words, no jargon.
		if p, err = apply(true, "ok", true); err != nil {
			return err
		}
		for _, want := range []string{"1 helper,", "waiting for you"} {
			if !strings.Contains(p.Overview, want) {
				return d30Fail("plain overview lacks %q: %q", want, p.Overview)
			}
		}
		if err := d30Eval(`document.getElementById('secAgentsSlot').click()`); err != nil {
			return err
		}
		if p, err = d30Read(); err != nil {
			return err
		}
		for _, want := range []string{"helper", "Foreman job", "waiting for you", "tool server example-server", "project project"} {
			if !strings.Contains(p.Text, want) {
				return d30Fail("plain map lacks %q: %q", want, p.Text)
			}
		}
		for _, jargon := range []string{"subagent", "MCP", "repo ", "waits on you"} {
			if strings.Contains(p.Text, jargon) {
				return d30Fail("plain map still shows %q", jargon)
			}
		}
		if _, err := screenshot(hwnd, "d30-map-plain"); err != nil {
			return err
		}

		// No current snapshot: no old numbers, and the overview offers the last 24 hours.
		if p, err = apply(false, "missing", false); err != nil {
			return err
		}
		if !strings.Contains(p.Overview, "Open the map for the last 24 hours") || strings.Contains(p.Overview, "sprint-example") {
			return d30Fail("missing snapshot overview: %q", p.Overview)
		}
		if err := d30Eval(`document.getElementById('secAgentsSlot').click()`); err != nil {
			return err
		}
		if p, err = d30Read(); err != nil {
			return err
		}
		if !p.MapOn || len(p.TabsOn) != 1 || p.TabsOn[0] != "24" {
			return d30Fail("missing snapshot map: open %v, tab %v, source %q", p.MapOn, p.TabsOn, p.Src)
		}
		if err := d30Eval(`window.__bdevMap.close()`); err != nil {
			return err
		}
		fmt.Println("d30: ok")
		return nil
	}
}

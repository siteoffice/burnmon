package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// station/d23: moved from check_d23.go.

// d23: the Station, BurnMon Dev's secret screen
// (02_roadmap\2026-10-01_station_secret_screen.md, step 6). Fake mode feeds
// three sessions with stages (never live data), then real key presses:
// P opens the full overlay and its canvas draws, a Station click opens the
// turn popup on top, Esc closes the popup first and the Station on the
// next press; O replaces the parts of the burn zone (chart to turn ticker) with
// the plan view at the height of those parts (B7.1), and it draws; O
// brings the zone back; I does the same with the 3D view, and Esc closes O and
// I after a turn popup and before fullscreen (2026-10-05). Screenshots of the
// views in both themes (P, O and I, site and space). Station themes (alpha.10):
// the startup theme is read from window.BM_STATION_THEME (space by default,
// HUD "STATION ... on board"; site shows "SITE ... on site"), T flips it to
// the other theme and back while the Station is open, and does nothing once it
// is closed.
const (
	vkP = 0x50
	vkO = 0x4F
	vkT = 0x54
)

// d23State is one read of the Station's DOM. Lit counts canvas pixels
// (sampled every 4th pixel each way) brighter than the deck background, so
// an empty or never-drawn canvas reads near 0.
type d23State struct {
	Overlay    bool `json:"overlay"`
	FullCanvas bool `json:"fullCanvas"`
	Panel      bool `json:"panel"`
	BarsShown  bool `json:"barsShown"`
	// TickerShown is the turn ticker, the burn zone's last panel above System.
	TickerShown bool    `json:"tickerShown"`
	AxisShown   bool    `json:"axisShown"`
	Lit         int     `json:"lit"`
	Hud         string  `json:"hud"`
	Popup       bool    `json:"popup"`
	PopupZ      float64 `json:"popupZ"`
	OverlayZ    float64 `json:"overlayZ"`
	PanelH      float64 `json:"panelH"`
	ZoneH       float64 `json:"zoneH"`
	Theme       string  `json:"theme"`
	// View is the mounted Station's projection: "iso" (3D) or "top" (plan).
	View string `json:"view"`
	// ExitCalls counts bdevExitFullscreen calls since the spy went in.
	ExitCalls int `json:"exitCalls"`
}

const d23ReadJS = `(function(){
  var ov = document.getElementById('stationOverlay');
  var box = document.getElementById('stationPanel');
  var cv = (ov && ov.classList.contains('on') ? ov.querySelector('canvas') : null) || (box ? box.querySelector('canvas') : null);
  var lit = 0;
  if(cv && cv.width && cv.height){
    var d = cv.getContext('2d').getImageData(0, 0, cv.width, cv.height).data;
    for(var y = 0; y < cv.height; y += 4) for(var x = 0; x < cv.width; x += 4){
      var i = (y * cv.width + x) * 4;
      if(d[i] + d[i + 1] + d[i + 2] > 90) lit++;
    }
  }
  var hud = document.querySelector('.bms-hud');
  var pop = document.getElementById('turnPopupOverlay');
  // Shown means it has a box: a child of a display:none parent keeps its
  // own computed display, so check the rendered size instead.
  function shown(id){ var el = document.getElementById(id); return !!el && el.getClientRects().length > 0; }
  return {
    overlay: !!ov && ov.classList.contains('on'), fullCanvas: !!(ov && ov.querySelector('canvas')),
    panel: !!box, barsShown: shown('burnBars'), tickerShown: shown('turnTicker'), axisShown: shown('burnAxis'), lit: lit,
    hud: hud ? hud.textContent : '', popup: !pop.classList.contains('hidden'),
    popupZ: parseFloat(getComputedStyle(pop).zIndex) || 0, overlayZ: parseFloat(getComputedStyle(ov).zIndex) || 0,
    panelH: box ? box.getBoundingClientRect().height : 0,
    zoneH: document.querySelector('.zone.burn').getBoundingClientRect().height,
    theme: window.__bdevStationTheme ? (window.__bdevStationTheme() || '') : '',
    view: window.__bdevStationView ? (window.__bdevStationView() || '') : '',
    exitCalls: window.__bdevExitCalls || 0
  };
})()`

const d23FakeJS = `(function(){
  var nowMs = Date.now(), ws = nowMs - 30 * 60000, chart = [];
  function iso(ms){ return new Date(ms).toISOString(); }
  for(var i = 0; i < 30; i++) chart.push({ at: iso(ws + i * 60000), by_session: {}, cost: 0 });
  function s(id, vendor, agent, stage, tool){
    return {session_id: id, vendor: vendor, agent: agent, surface: agent, model: '', project: 'proj-' + id, start: iso(nowMs - 600000),
      last_turn: iso(nowMs - 5000), turn_count: 4, context: 1000, context_window: 200000, tokens: 5000, cost: 0.1, cache_hit_ratio: 0,
      stage: stage, stage_since: iso(nowMs - 5000), stage_tool: tool};
  }
  var sessions = [s('fake-st-1', 'anthropic', 'claude-code', 'reading', 'Read'),
                  s('fake-st-2', 'openai', 'codex', 'running', 'exec'),
                  s('fake-st-3', 'anthropic', 'claude-code', 'waiting', '')];
  var snap = { now: nowMs, hist_gap_ms: 25000, sysmon: { Ts: iso(nowMs), pressure_score: 10, CPUPct: 10, Cores: [10, 10], MemUsedMB: 1000, MemTotalMB: 32000,
      SwapUsedMB: 0, SwapTotalMB: 1000, NetDownBps: 0, NetUpBps: 0, Disks: [], Wifi: { OK: true, Connected: false } },
    sysmon_history: [], process_groups_now: [], process_groups_history: [], vendor_strip: { rows: [] }, activity_heatmap: { rows: [] },
    todo: { enabled: false }, todo_tasks: { items: [] }, headline_today: 0,
    burn: { generated_at: iso(nowMs), running_window_seconds: 600, sessions: sessions, window_start: iso(ws), bucket_seconds: 60, chart: chart, turns: [] } };
  window.__bdevEnterFakeMode();
  window.__bdevPaintFake(snap);
  return true;
})()`

// d23Theme says the Station shows theme: the page's theme name and the HUD word.
func d23Theme(s d23State, theme string) bool {
	word := "STATION"
	if theme == "site" {
		word = "SITE"
	}
	return s.Theme == theme && strings.Contains(s.Hud, word)
}

func d23Other(theme string) string {
	if theme == "site" {
		return "space"
	}
	return "site"
}

// d23Shot waits for the canvas to repaint before the capture: the DOM read
// that precedes it can already show the new theme while the window still holds
// the previous frame (found 2026-10-03, two "different" shots were identical).
func d23Shot(hwnd uintptr, name string) error {
	time.Sleep(900 * time.Millisecond)
	_, err := screenshot(hwnd, name)
	return err
}

func d23Read() (d23State, error) {
	var s d23State
	err := evalInto(d23ReadJS, &s)
	return s, err
}

// d23Wait polls until ok holds or 4 s pass, returning the last read.
func d23Wait(ok func(d23State) bool) (d23State, bool, error) {
	var s d23State
	var err error
	for deadline := time.Now().Add(4 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if s, err = d23Read(); err != nil {
			return s, false, err
		}
		if ok(s) {
			return s, true, nil
		}
	}
	return s, false, nil
}

func caseD23(hwnd uintptr, args []string) error {
	time.Sleep(300 * time.Millisecond)
	if _, err := evalRaw(d23FakeJS); err != nil {
		return fmt.Errorf("d23: fake paint: %w", err)
	}
	start, err := evalString(`window.BM_STATION_THEME`)
	if err != nil || (start != "site" && start != "space") {
		return fmt.Errorf("d23: startup theme read %q: %v", start, err)
	}
	other := d23Other(start)
	bringToFront(hwnd)
	// A real click grants the window input focus before the key presses
	// (d7's own finding).
	if err := clickSelectorAt(hwnd, "header .logo", 0.5, 0.5); err != nil {
		fmt.Println("uicheck: d23: could not click header .logo to establish focus:", err)
	}
	var errs []string
	step := func(name string, vk uint16, ok func(d23State) bool) d23State {
		if vk != 0 {
			if err := pressKey(vk); err != nil {
				errs = append(errs, fmt.Sprintf("%s: press: %v", name, err))
				return d23State{}
			}
		}
		s, good, err := d23Wait(ok)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: eval: %v", name, err))
		} else if !good {
			errs = append(errs, fmt.Sprintf("%s: got %+v", name, s))
		}
		fmt.Printf("uicheck: d23 %s: %+v\n", name, s)
		return s
	}

	step("P opens the full Station and it draws", vkP, func(s d23State) bool {
		return s.Overlay && s.FullCanvas && s.Lit > 500 && strings.Contains(s.Hud, "3 agents") &&
			d23Theme(s, start)
	})
	// The fake agents walk in from the airlock; give them time to reach their
	// rooms and sit, so the shots show the settled pose (alpha.10 follow-up).
	time.Sleep(12 * time.Second)
	bringToFront(hwnd) // focus can drift during a long wait; the key presses below need it
	if err := d23Shot(hwnd, "d23-station-full"); err != nil {
		return err
	}
	step("T switches the Station to the space theme", vkT, func(s d23State) bool {
		return s.Overlay && s.Lit > 500 && d23Theme(s, other)
	})
	if err := d23Shot(hwnd, "d23-station-space"); err != nil {
		return err
	}
	step("T again switches back to the site theme", vkT, func(s d23State) bool {
		return s.Overlay && s.Lit > 500 && d23Theme(s, start)
	})
	if _, err := evalRaw(`(window.__bdevStationClick('fake-st-1'), true)`); err != nil {
		errs = append(errs, fmt.Sprintf("station click: %v", err))
	}
	step("a Station click opens the turn popup above it", 0, func(s d23State) bool {
		return s.Popup && s.Overlay && s.PopupZ > s.OverlayZ
	})
	step("Esc closes the popup first, the Station stays", vkEscape, func(s d23State) bool {
		return !s.Popup && s.Overlay
	})
	step("Esc again closes the Station", vkEscape, func(s d23State) bool {
		return !s.Overlay && !s.FullCanvas && s.BarsShown
	})
	// A "nothing happens" check must wait before it reads, or it passes
	// before the key press has even been handled.
	if err := pressKey(vkT); err != nil {
		errs = append(errs, fmt.Sprintf("T closed: press: %v", err))
	}
	time.Sleep(500 * time.Millisecond)
	step("T with the Station closed changes nothing", 0, func(s d23State) bool {
		return !s.Overlay && s.Theme == start
	})
	step("O swaps the chart for the plan view and it draws", vkO, func(s d23State) bool {
		return s.Panel && !s.BarsShown && !s.AxisShown && !s.TickerShown && s.PanelH > 0 && s.PanelH <= s.ZoneH && s.Lit > 500 && strings.Contains(s.Hud, "3 agents")
	})
	if err := d23Shot(hwnd, "d23-station-panel"); err != nil {
		return err
	}
	// T works in the plan view too (the Station is mounted in the burn zone).
	step("T switches the plan view to the space theme", vkT, func(s d23State) bool {
		return s.Panel && s.Lit > 500 && d23Theme(s, other)
	})
	if err := d23Shot(hwnd, "d23-station-panel-space"); err != nil {
		return err
	}
	step("T again switches the plan view back to the site theme", vkT, func(s d23State) bool {
		return s.Panel && s.Lit > 500 && d23Theme(s, start)
	})
	// Esc closes the plan view (O) and the 3D view (I) after a turn popup
	// and before fullscreen: a spy on bdevExitFullscreen counts the calls
	// (F11 itself is a global hotkey, so the Esc path is the only one a
	// key press in the page can reach).
	if _, err := evalRaw(`(window.__bdevExitCalls = 0, window.__bdevExitOrig = window.bdevExitFullscreen, window.bdevExitFullscreen = function(){ window.__bdevExitCalls++; }, true)`); err != nil {
		errs = append(errs, fmt.Sprintf("exit spy: %v", err))
	}
	defer evalRaw(`(window.__bdevExitOrig && (window.bdevExitFullscreen = window.__bdevExitOrig), true)`)
	escOrder := func(name string, open func(d23State) bool) {
		if _, err := evalRaw(`(window.__bdevStationClick('fake-st-1'), true)`); err != nil {
			errs = append(errs, fmt.Sprintf("%s: station click: %v", name, err))
		}
		step(name+": a Station click opens the turn popup", 0, func(s d23State) bool { return s.Popup && open(s) })
		step(name+": Esc closes the popup first, the view stays, fullscreen is left alone", vkEscape, func(s d23State) bool {
			return !s.Popup && open(s) && s.ExitCalls == 0
		})
		step(name+": Esc again closes the view, the chart is back, fullscreen is still left alone", vkEscape, func(s d23State) bool {
			return !s.Popup && !s.Panel && !s.Overlay && s.BarsShown && s.TickerShown && s.ExitCalls == 0
		})
		step(name+": Esc with nothing open reaches fullscreen", vkEscape, func(s d23State) bool { return s.ExitCalls == 1 })
		_, _ = evalRaw(`(window.__bdevExitCalls = 0, true)`)
	}
	step("O again brings the chart back", vkO, func(s d23State) bool {
		return !s.Panel && s.BarsShown && s.AxisShown && s.TickerShown && !s.Overlay
	})
	step("O opens the plan view for the Esc check", vkO, func(s d23State) bool { return s.Panel && s.View == "top" && s.Lit > 500 })
	escOrder("O", func(s d23State) bool { return s.Panel && s.View == "top" })

	// I (3D Station in the burn zone, 2026-10-05): the same zone and
	// frame as O, the isometric projection instead of the plan.
	step("I swaps the chart for the 3D Station and it draws", vkI, func(s d23State) bool {
		return s.Panel && s.View == "iso" && !s.BarsShown && !s.AxisShown && !s.TickerShown && s.PanelH > 0 && s.PanelH <= s.ZoneH && s.Lit > 500 &&
			strings.Contains(s.Hud, "3 agents") && !s.Overlay
	})
	if err := d23Shot(hwnd, "d23-station-iso"); err != nil {
		return err
	}
	step("T switches the 3D view to the space theme", vkT, func(s d23State) bool {
		return s.Panel && s.View == "iso" && s.Lit > 500 && d23Theme(s, other)
	})
	if err := d23Shot(hwnd, "d23-station-iso-space"); err != nil {
		return err
	}
	step("T again switches the 3D view back to the site theme", vkT, func(s d23State) bool {
		return s.Panel && s.View == "iso" && s.Lit > 500 && d23Theme(s, start)
	})
	step("O while the 3D view is open swaps to the plan view", vkO, func(s d23State) bool {
		return s.Panel && s.View == "top" && s.Lit > 500
	})
	step("I swaps back to the 3D view", vkI, func(s d23State) bool { return s.Panel && s.View == "iso" && s.Lit > 500 })
	escOrder("I", func(s d23State) bool { return s.Panel && s.View == "iso" })
	step("I opens the 3D view once more", vkI, func(s d23State) bool { return s.Panel && s.View == "iso" && s.Lit > 500 })
	step("I again brings the chart back", vkI, func(s d23State) bool {
		return !s.Panel && s.BarsShown && s.AxisShown && s.TickerShown && !s.Overlay
	})
	if len(errs) > 0 {
		return fmt.Errorf("d23: %d problem(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}
	fmt.Println("uicheck: d23: Station opens on P, O and I, draws in all three, popup above it, Esc order popup then Station then fullscreen, T flips site and space only while open")
	return nil
}

// station/d25: moved from check_d25.go.

// d25: the Station's click popup is the compact card
// (02_roadmap\2026-10-03_station_ui_and_backlog.md, item A). A click on an
// agent in P, O or I opens the latest turn as a small card next to the click,
// not the 480 px modal in the middle of the window. A long value (a Cowork
// working directory on one unbroken line was the original bug) wraps inside
// the card, and the card, with everything in it, stays inside the window.
// Fake mode feeds three sessions; the turn detail is injected through
// __bdevTurnDetailResolve with long synthetic paths, so nothing real is read.
// Each view is checked on the site and space Station themes, then again with
// the page in light mode (L). Screenshots: d25-<view>-<theme>.
const (
	vkI = 0x49
	// d25MaxCardW is the widest a "compact card" may be in CSS px: the hover
	// card measures about 230 px, so 340 leaves room for the turn table.
	d25MaxCardW = 340
)

type d25Card struct {
	Shown    bool    `json:"shown"`
	Rendered bool    `json:"rendered"`
	Left     float64 `json:"left"`
	Top      float64 `json:"top"`
	Right    float64 `json:"right"`
	Bottom   float64 `json:"bottom"`
	W        float64 `json:"w"`
	ScrollW  float64 `json:"scrollW"`
	ClientW  float64 `json:"clientW"`
	Clipped  int     `json:"clipped"`
	IW       float64 `json:"iw"`
	IH       float64 `json:"ih"`
}

// d25ClickJS clicks fake-st-1 at (ax, ay) and answers the turn detail request
// with long values: a path with no break opportunity, a long model name and
// a long file list entry. The pending promise exists as soon as the click
// returns, so this resolve wins over the Go side's own answer.
const d25ClickJS = `(function(ax, ay){
  var sep = String.fromCharCode(92), seg = [];
  for(var i = 0; i < 14; i++) seg.push('local-agent-mode-sessions-segment-' + i);
  var long = 'C:' + sep + seg.join(sep);
  window.__bdevStationClick('fake-st-1', null, {x: ax, y: ay});
  window.__bdevTurnDetailResolve('fake-st-1:4', {
    turn: 4, session_id: 'fake-st-1', model: 'a-very-long-model-name-that-has-no-break-' + 'x'.repeat(60),
    at: new Date().toISOString(), has_gap: true, gap_seconds: 12, cost: 0.12,
    fresh: 1200, cache_write: 0, cache_read: 5000, output: 300, findings: [],
    tool_calls: [{tool: 'Read', path: long, input_bytes: 100, result_bytes: 2000}],
    files: [long]
  });
  return true;
})`

const d25ReadJS = `(function(){
  var p = document.getElementById('turnPopup'), ov = document.getElementById('turnPopupOverlay');
  var r = p.getBoundingClientRect(), clipped = 0;
  p.querySelectorAll('*').forEach(function(el){
    var b = el.getBoundingClientRect();
    if(!b.width && !b.height) return;
    if(b.left < r.left - 1 || b.right > r.right + 1 || b.top < r.top - 1 || b.bottom > r.bottom + 1) clipped++;
  });
  return {shown: !ov.classList.contains('hidden'), rendered: p.textContent.indexOf('Turn 4') >= 0 && p.textContent.indexOf('Token classes') >= 0,
    left: r.left, top: r.top, right: r.right, bottom: r.bottom, w: r.width, scrollW: p.scrollWidth, clientW: p.clientWidth,
    clipped: clipped, iw: innerWidth, ih: innerHeight};
})()`

func d25Read() (d25Card, error) {
	var c d25Card
	err := evalInto(d25ReadJS, &c)
	return c, err
}

// d25Click clicks the agent at (ax, ay) and polls until the card is rendered.
func d25Click(ax, ay int) (d25Card, error) {
	if _, err := evalRaw(fmt.Sprintf("%s(%d, %d)", d25ClickJS, ax, ay)); err != nil {
		return d25Card{}, err
	}
	var c d25Card
	var err error
	for deadline := time.Now().Add(4 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if c, err = d25Read(); err != nil {
			return c, err
		}
		if c.Shown && c.Rendered {
			break
		}
	}
	return c, nil
}

// d25Problems lists what is wrong with a card opened by a click at (ax, ay).
func d25Problems(c d25Card, ax, ay float64) []string {
	var p []string
	if !c.Shown || !c.Rendered {
		return []string{"card not shown or not rendered"}
	}
	if c.Left < 0 || c.Top < 0 || c.Right > c.IW || c.Bottom > c.IH {
		p = append(p, "card outside the window")
	}
	if c.W > d25MaxCardW {
		p = append(p, fmt.Sprintf("card %.0f px wide, compact is at most %d", c.W, d25MaxCardW))
	}
	if c.Clipped > 0 || c.ScrollW > c.ClientW+1 {
		p = append(p, fmt.Sprintf("content clipped: %d elements outside the card, scrollWidth %.0f > clientWidth %.0f", c.Clipped, c.ScrollW, c.ClientW))
	}
	// Next to the click: 14 px right and below it, pushed back only as far as
	// the window edge needs (a tall card slides up, a click near the right
	// edge slides it left).
	wantLeft := math.Max(4, math.Min(ax+14, c.IW-c.W-4))
	wantTop := math.Max(4, math.Min(ay+14, c.IH-(c.Bottom-c.Top)-4))
	if math.Abs(c.Left-wantLeft) > 2 || math.Abs(c.Top-wantTop) > 2 {
		p = append(p, fmt.Sprintf("card at (%.0f,%.0f), want (%.0f,%.0f) for the click at (%.0f,%.0f)", c.Left, c.Top, wantLeft, wantTop, ax, ay))
	}
	return p
}

func caseD25(hwnd uintptr, args []string) error {
	time.Sleep(300 * time.Millisecond)
	if _, err := evalRaw(d23FakeJS); err != nil {
		return fmt.Errorf("d25: fake paint: %w", err)
	}
	bringToFront(hwnd)
	if err := clickSelectorAt(hwnd, "header .logo", 0.5, 0.5); err != nil {
		fmt.Println("uicheck: d25: could not click header .logo to establish focus:", err)
	}
	var errs []string
	fail := func(where string, probs []string) {
		for _, m := range probs {
			errs = append(errs, fmt.Sprintf("%s: %s", where, m))
		}
	}
	// checkCard runs the two clicks of one case: next to the middle of the
	// window, then in the bottom right corner where the card must be pushed back in.
	checkCard := func(where string) error {
		c, err := d25Click(700, 300)
		if err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		fmt.Printf("uicheck: d25 %s middle: %+v\n", where, c)
		fail(where+" middle click", d25Problems(c, 700, 300))
		if err := d23Shot(hwnd, "d25-"+strings.ReplaceAll(where, " ", "-")); err != nil {
			return err
		}
		c, err = d25Click(1595, 895)
		if err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		fmt.Printf("uicheck: d25 %s corner: %+v\n", where, c)
		fail(where+" corner click", d25Problems(c, 1595, 895))
		_, err = evalRaw(`document.getElementById('turnPopupClose').click()`)
		return err
	}
	openView := func(name string, vk uint16, ok func(d23State) bool) bool {
		if err := pressKey(vk); err != nil {
			errs = append(errs, fmt.Sprintf("%s: press: %v", name, err))
			return false
		}
		s, good, err := d23Wait(ok)
		if err != nil || !good {
			errs = append(errs, fmt.Sprintf("%s did not open: %+v %v", name, s, err))
			return false
		}
		time.Sleep(1500 * time.Millisecond) // the first frame, so the shot is not blank
		return true
	}
	views := []struct {
		name string
		vk   uint16
		ok   func(d23State) bool
	}{
		{"P", vkP, func(s d23State) bool { return s.Overlay && s.Lit > 500 }},
		{"O", vkO, func(s d23State) bool { return s.Panel && s.Lit > 500 }},
		{"I", vkI, func(s d23State) bool { return s.Panel && s.Lit > 500 }},
	}
	runViews := func(page string, themes []string) error {
		for _, v := range views {
			if !openView(v.name, v.vk, v.ok) {
				continue
			}
			for i, th := range themes {
				if i > 0 {
					if err := pressKey(vkT); err != nil {
						return err
					}
					time.Sleep(600 * time.Millisecond)
				}
				if err := checkCard(strings.TrimSpace(v.name + " " + th + " " + page)); err != nil {
					return err
				}
			}
			if len(themes) > 1 { // back to site for the next view
				if err := pressKey(vkT); err != nil {
					return err
				}
				time.Sleep(400 * time.Millisecond)
			}
			if err := pressKey(v.vk); err != nil { // the same key closes the view
				return err
			}
			if _, good, err := d23Wait(func(s d23State) bool { return !s.Overlay && !s.Panel }); err != nil || !good {
				errs = append(errs, fmt.Sprintf("%s did not close on its own key", v.name))
				_ = pressKey(vkEscape)
			}
		}
		return nil
	}
	if err := runViews("dark page", []string{"site", "space"}); err != nil {
		return err
	}
	if err := pressKey(vkL); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)
	if err := runViews("light page", []string{"site"}); err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("d25: %d problem(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}
	fmt.Println("uicheck: d25: the Station click card is compact, next to the click, wraps long values and stays inside the window in P, O and I, both Station themes and the light page")
	return nil
}

// station/d26: moved from check_d26.go.

// d26: the space theme's engine glow under the deck has no hard top edge
// (02_roadmap\2026-10-03_station_ui_and_backlog.md, item D, 2026-10-05).
// station.js fills the radial gradient into a rect that started at the deck's
// bottom corner, so the upper part of the glow was cut flat. A spy on the 2D
// context records every glow fill (the gradient whose first stop is
// rgba(56,189,248,0.22)) and the floor canvas that buildFloor bakes, then per
// state it reads
//
//	cover:  the filled rect contains the gradient's whole bounding square
//	cols/maxDelta: along the two floor rows 2 px apart across the old edge,
//	  for every column that is neither deck nor slab (alpha under 250 on both),
//	  the largest alpha step between them (0 to 255). A flat edge shows as a
//	  step of 25 to 40, a clean glow as 0 or 1. cols guards against a
//	  vacuous read.
//	hash:   FNV-1a of every floor pixel, so a before and an after run can be
//	  compared for the views and themes that must not change.
//
// Views are driven with synthetic key events (no window focus needed) and the
// camera with synthetic wheel and mouse events. Only the P view in the space
// theme is asserted; I and the site theme are recorded for comparison, and O
// (drawTop, buildPixel) must not run buildFloor at all: neither the floor
// build counter nor the glow counter may move while it is open.
// UICHECK_TAG names the run: screenshots are d26-<tag>-<state> and the lines
// go to testdata\uicheck\out\d26-<tag>.txt.
const d26SpyJS = `(function(){
  if(window.__glow) return true;
  var g = window.__glow = {calls: [], floor: null, builds: 0};
  var C = CanvasRenderingContext2D.prototype, H = HTMLCanvasElement.prototype;
  var cg = C.createRadialGradient, fr = C.fillRect, gc = H.getContext;
  C.createRadialGradient = function(){
    var gr = cg.apply(this, arguments);
    gr.__a = Array.prototype.slice.call(arguments); gr.__stops = [];
    var as = gr.addColorStop;
    gr.addColorStop = function(o, c){ gr.__stops.push(c); return as.call(gr, o, c); };
    return gr;
  };
  C.fillRect = function(x, y, w, h){
    var fs = this.fillStyle;
    if(fs && fs.__a && fs.__stops[0] && fs.__stops[0].indexOf('56,189,248,0.22') >= 0) g.calls.push({a: fs.__a, rect: [x, y, w, h]});
    return fr.apply(this, arguments);
  };
  H.getContext = function(){
    if(/buildFloor/.test(new Error().stack)){ g.floor = this; g.builds++; }
    return gc.apply(this, arguments);
  };
  return true;
})()`

const d26ReadJS = `(function(){
  var g = window.__glow, cv = g.floor, out = {builds: g.builds, glows: g.calls.length};
  if(!cv) return out;
  var W = cv.width, Hh = cv.height, data = cv.getContext('2d').getImageData(0, 0, W, Hh).data, h = 2166136261, i;
  for(i = 0; i < data.length; i++){ h ^= data[i]; h = Math.imul(h, 16777619); }
  out.hash = (h >>> 0).toString(16); out.w = W; out.h = Hh;
  var c = g.calls[g.calls.length - 1];
  if(!c) return out;
  var cx = c.a[0], cy = c.a[1], r = c.a[5], rc = c.rect, eps = 0.5;
  out.cover = rc[0] <= cx - r + eps && rc[1] <= cy - r + eps && rc[0] + rc[2] >= cx + r - eps && rc[1] + rc[3] >= cy + r - eps;
  var dpr = window.devicePixelRatio || 1, d = 34 * r / 260, b1 = cy - 2 * d;
  var ya = Math.floor(b1 * dpr) - 1, yb = ya + 2, x0 = Math.max(0, Math.floor((cx - r) * dpr)), x1 = Math.min(W - 1, Math.floor((cx + r) * dpr));
  out.cols = 0; out.maxDelta = 0;
  if(ya >= 0 && yb < Hh) for(var x = x0; x <= x1; x++){
    // Glow alone tops out at 0.22 * 255 = 56. A column with anything stronger
    // within 5 rows (deck, slab, or the antialiased pixel at the slab's edge)
    // is not glow-only, so it is left out.
    var strong = false, yy;
    for(yy = ya - 5; yy <= yb + 5 && !strong; yy++) if(yy < 0 || yy >= Hh || data[(yy * W + x) * 4 + 3] > 60) strong = true;
    if(strong) continue;
    var A = data[(ya * W + x) * 4 + 3], B = data[(yb * W + x) * 4 + 3];
    out.cols++;
    if(Math.abs(B - A) > out.maxDelta){
      out.maxDelta = Math.abs(B - A); out.worstX = x - Math.round(cx * dpr); out.profile = [];
      for(yy = ya - 5; yy <= yb + 5; yy++) out.profile.push(data[(yy * W + x) * 4 + 3]);
    }
  }
  return out;
})()`

type d26Read struct {
	Builds   int    `json:"builds"`
	Glows    int    `json:"glows"`
	Hash     string `json:"hash"`
	W        int    `json:"w"`
	H        int    `json:"h"`
	Cover    *bool  `json:"cover"`
	Cols     int    `json:"cols"`
	MaxDelta int    `json:"maxDelta"`
	WorstX   int    `json:"worstX"`
	Profile  []int  `json:"profile"`
}

const d26KeyJS = `(document.dispatchEvent(new KeyboardEvent('keydown', {key: %q, bubbles: true})), true)`

const d26CanvasJS = `(function(){
  var c = document.querySelector('#stationOverlay canvas') || document.querySelector('#stationPanel canvas');
  var r = c.getBoundingClientRect();
  return {c: c, x: r.left + r.width / 2, y: r.top + r.height / 2};
})()`

func caseD26(hwnd uintptr, args []string) error {
	tag := os.Getenv("UICHECK_TAG")
	if tag == "" {
		tag = "run"
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := evalRaw(d23FakeJS); err != nil {
		return fmt.Errorf("d26: fake paint: %w", err)
	}
	if _, err := evalRaw(d26SpyJS); err != nil {
		return fmt.Errorf("d26: spy: %w", err)
	}
	bringToFront(hwnd)
	var errs, lines []string
	key := func(k string) {
		if _, err := evalRaw(fmt.Sprintf(d26KeyJS, k)); err != nil {
			errs = append(errs, fmt.Sprintf("key %s: %v", k, err))
		}
	}
	theme := func(want string) {
		var cur string
		if err := evalInto(`(window.__bdevStationTheme ? (window.__bdevStationTheme() || '') : '')`, &cur); err != nil {
			errs = append(errs, fmt.Sprintf("theme read: %v", err))
			return
		}
		if cur != want {
			key("t")
		}
	}
	// camera drives the Station canvas with synthetic events.
	camera := func(js string) {
		if _, err := evalRaw(`(function(){ var q = ` + d26CanvasJS + `, c = q.c, x = q.x, y = q.y; ` + js + ` return true; })()`); err != nil {
			errs = append(errs, fmt.Sprintf("camera: %v", err))
		}
	}
	// zoomIn anchors the wheel on the deck's bottom corner (read back from
	// the last glow fill), so the corner stays on screen while it scales.
	zoomIn := func() {
		camera(`var k = window.__glow.calls[window.__glow.calls.length - 1].a, r = c.getBoundingClientRect();
				x = r.left + k[0]; y = r.top + k[1] - 2 * 34 * k[5] / 260;
				c.dispatchEvent(new WheelEvent('wheel', {deltaY: -400, clientX: x, clientY: y, bubbles: true, cancelable: true}));`)
	}
	pan := func() {
		camera(`c.dispatchEvent(new MouseEvent('mousedown', {clientX: x, clientY: y, bubbles: true}));
				c.dispatchEvent(new MouseEvent('mousemove', {clientX: x - 170, clientY: y - 110, bubbles: true}));
				window.dispatchEvent(new MouseEvent('mouseup', {clientX: x - 170, clientY: y - 110, bubbles: true}));`)
	}
	// measure waits for the floor to be rebuilt after the last change,
	// then shoots and reads. kind is "assert" (the glow must be whole),
	// "record" (print only) or "same" (the plan view never builds the
	// isometric floor: neither counter may move).
	var prevBuilds, prevGlows int
	measure := func(label, kind string) {
		time.Sleep(900 * time.Millisecond)
		bringToFront(hwnd)
		if _, err := screenshot(hwnd, "d26-"+tag+"-"+label); err != nil {
			errs = append(errs, fmt.Sprintf("%s: screenshot: %v", label, err))
		}
		var r d26Read
		if err := evalInto(d26ReadJS, &r); err != nil {
			errs = append(errs, fmt.Sprintf("%s: read: %v", label, err))
			return
		}
		// The glow numbers belong to the last glow fill; a build with no new
		// fill (site theme, plan view) has none of its own.
		fresh := r.Glows > prevGlows
		cover := "n/a"
		if r.Cover != nil && fresh {
			cover = fmt.Sprint(*r.Cover)
		} else {
			r.Cols, r.MaxDelta = 0, 0
		}
		hash := r.Hash
		if kind == "same" {
			hash = "n/a"
		}
		line := fmt.Sprintf("d26 %-18s hash=%s size=%dx%d builds=%d glows=%d cover=%s cols=%d maxDelta=%d", label, hash, r.W, r.H, r.Builds, r.Glows, cover, r.Cols, r.MaxDelta)
		if r.MaxDelta > 3 {
			line += fmt.Sprintf(" worstDX=%d profile=%v", r.WorstX, r.Profile)
		}
		fmt.Println("uicheck: " + line)
		lines = append(lines, line)
		if kind == "same" {
			if r.Builds != prevBuilds || r.Glows != prevGlows {
				errs = append(errs, fmt.Sprintf("%s: the plan view ran buildFloor (builds %d to %d, glows %d to %d)", label, prevBuilds, r.Builds, prevGlows, r.Glows))
			}
		} else if r.Builds <= prevBuilds {
			errs = append(errs, fmt.Sprintf("%s: the floor was not rebuilt (builds %d)", label, r.Builds))
		}
		prevBuilds, prevGlows = r.Builds, r.Glows
		if kind == "assert" {
			switch {
			case !fresh || r.Cover == nil:
				errs = append(errs, fmt.Sprintf("%s: no glow fill recorded for this build", label))
			case !*r.Cover:
				errs = append(errs, fmt.Sprintf("%s: the glow fill does not cover its gradient square", label))
			}
			if r.Cols < 20 {
				errs = append(errs, fmt.Sprintf("%s: only %d glow-only columns read, the edge measure is vacuous", label, r.Cols))
			} else if r.MaxDelta > 3 {
				errs = append(errs, fmt.Sprintf("%s: hard edge, alpha steps by %d across the old rect top", label, r.MaxDelta))
			}
		}
	}

	// P, the full-window 3D view: site first (no glow), then space.
	key("p")
	time.Sleep(700 * time.Millisecond)
	theme("site")
	measure("P-site-fit", "record")
	theme("space")
	measure("P-space-fit", "assert")
	zoomIn()
	measure("P-space-zoomin", "assert")
	pan()
	measure("P-space-zoomin-pan", "assert")
	key("p")
	time.Sleep(400 * time.Millisecond)

	// O, the plan view in the burn zone: it must not touch the floor code.
	key("o")
	time.Sleep(700 * time.Millisecond)
	theme("space")
	measure("O-space-fit", "same")
	theme("site")
	measure("O-site-fit", "same")
	key("o")
	time.Sleep(400 * time.Millisecond)

	// I, the 3D view in the burn zone: recorded, not asserted.
	key("i")
	time.Sleep(700 * time.Millisecond)
	theme("space")
	measure("I-space-fit", "record")
	theme("site")
	measure("I-site-fit", "record")
	key("i")

	dir := filepath.Join("..", "..", "testdata", "uicheck", "out")
	if err := os.MkdirAll(dir, 0o755); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "d26-"+tag+".txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	}
	if len(errs) > 0 {
		return fmt.Errorf("d26: %d problem(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}
	fmt.Println("uicheck: d26: the engine glow is whole at three camera states in the P view, space theme")
	return nil
}

// station/d27: moved from check_d27.go.

// d27: the site theme's clouds look like clouds
// (02_roadmap\2026-10-03_station_ui_and_backlog.md, item E, 2026-10-05).
// buildSky drew each cloud as four flat ellipses at 50 percent white, so the
// overlaps doubled up and read as stacked discs. The fix draws each cloud on
// its own offscreen canvas as 5 to 8 soft puffs and composites it once at one
// alpha, seeded and baked once. A spy on the 2D context records, per sky
// build (buildStars with the site theme): the offscreen cloud canvases, their
// arc counts and the alpha each is composited at, and the ellipses drawn
// straight onto the sky. Then per state it reads the baked sky canvas:
//
//	step:   the largest luminance step between two neighbouring pixels in the
//	  sky above the horizon. A flat ellipse edge steps by 20 to 40, soft puffs
//	  by a few, the plain gradient and sun by about 1.
//	ground: FNV-1a of the ground below the horizon, which the fix must leave
//	  alone (the seeded random stream after the clouds is unchanged).
//	hash:   FNV-1a of the whole sky, to prove a rebuild at the same size is
//	  identical (seeded).
//
// The baked sky is saved as d27-<tag>-<state>-sky.png (no HUD, no props) so a
// before and an after run can be compared pixel for pixel. Views are driven
// with synthetic key events, the page theme with L (restored on exit, the
// choice is stored by the Go side), the window with ensureWindowSizeWH.
// UICHECK_TAG names the run.
const d27SpyJS = `(function(){
  if(window.__sky) return true;
  var g = window.__sky = {sky: null, builds: 0, clouds: [], ellipses: 0};
  var C = CanvasRenderingContext2D.prototype, H = HTMLCanvasElement.prototype;
  var gc = H.getContext, ar = C.arc, el = C.ellipse, di = C.drawImage;
  H.getContext = function(){
    var st = new Error().stack;
    if(/buildSky/.test(st)){
      if(!this.__cl){ this.__cl = {arcs: 0, alpha: null, draws: 0, w: this.width, h: this.height}; g.clouds.push(this.__cl); }
    } else if(/buildStars/.test(st) && this !== g.sky){ g.sky = this; g.builds++; g.clouds = []; g.ellipses = 0; }
    return gc.apply(this, arguments);
  };
  C.arc = function(){ var cl = this.canvas && this.canvas.__cl; if(cl) cl.arcs++; return ar.apply(this, arguments); };
  C.ellipse = function(){ if(this.canvas === g.sky) g.ellipses++; return el.apply(this, arguments); };
  C.drawImage = function(src){ if(src && src.__cl){ src.__cl.alpha = this.globalAlpha; src.__cl.draws++; } return di.apply(this, arguments); };
  return true;
})()`

const d27ReadJS = `(function(){
  var g = window.__sky, cv = g.sky;
  var out = {builds: g.builds, ellipses: g.ellipses, theme: window.__bdevStationTheme ? (window.__bdevStationTheme() || '') : '',
    page: document.documentElement.getAttribute('data-theme') || '',
    clouds: g.clouds.map(function(c){ return {arcs: c.arcs, alpha: c.alpha, draws: c.draws}; })};
  if(!cv) return out;
  var W = cv.width, H = cv.height, d = cv.getContext('2d').getImageData(0, 0, W, H).data, i, x, y;
  var hz = Math.floor(H * 0.3);
  function fnv(from, to){ var h = 2166136261; for(var k = from; k < to; k++){ h ^= d[k]; h = Math.imul(h, 16777619); } return (h >>> 0).toString(16); }
  function lum(k){ return 0.299 * d[k] + 0.587 * d[k + 1] + 0.114 * d[k + 2]; }
  out.w = W; out.h = H; out.hash = fnv(0, d.length); out.ground = fnv((hz + 10) * W * 4, d.length);
  var rows = hz - 10, step = 0, a, s;
  for(y = 0; y < rows; y++) for(x = 0; x < W - 1; x++){
    i = (y * W + x) * 4; a = lum(i);
    s = Math.abs(a - lum(i + 4)); if(s > step) step = s;
    if(y + 1 < rows){ s = Math.abs(a - lum(i + W * 4)); if(s > step) step = s; }
  }
  out.step = Math.round(step * 10) / 10;
  return out;
})()`

type d27Cloud struct {
	Arcs  int      `json:"arcs"`
	Alpha *float64 `json:"alpha"`
	Draws int      `json:"draws"`
}

type d27Read struct {
	Builds   int        `json:"builds"`
	Ellipses int        `json:"ellipses"`
	Theme    string     `json:"theme"`
	Page     string     `json:"page"`
	Clouds   []d27Cloud `json:"clouds"`
	W        int        `json:"w"`
	H        int        `json:"h"`
	Hash     string     `json:"hash"`
	Ground   string     `json:"ground"`
	Step     float64    `json:"step"`
}

func caseD27(hwnd uintptr, args []string) error {
	tag := os.Getenv("UICHECK_TAG")
	if tag == "" {
		tag = "run"
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := evalRaw(d23FakeJS); err != nil {
		return fmt.Errorf("d27: fake paint: %w", err)
	}
	if _, err := evalRaw(d27SpyJS); err != nil {
		return fmt.Errorf("d27: spy: %w", err)
	}
	bringToFront(hwnd)
	var errs, lines []string
	key := func(k string) {
		if _, err := evalRaw(fmt.Sprintf(d26KeyJS, k)); err != nil {
			errs = append(errs, fmt.Sprintf("key %s: %v", k, err))
		}
	}
	read := func() (r d27Read) {
		if err := evalInto(d27ReadJS, &r); err != nil {
			errs = append(errs, fmt.Sprintf("read: %v", err))
		}
		return r
	}
	// The page starts in the remembered theme; whatever it was is put back.
	startPage := read().Page
	defer func() {
		if read().Page != startPage {
			key("l")
			time.Sleep(400 * time.Millisecond)
		}
	}()
	// measure shoots, saves the baked sky and prints one line. asserted
	// says the clouds must be puffs, one alpha, with no hard step.
	measure := func(label string, asserted bool, shot bool) d27Read {
		time.Sleep(900 * time.Millisecond)
		bringToFront(hwnd)
		if shot {
			if _, err := screenshot(hwnd, "d27-"+tag+"-"+label); err != nil {
				errs = append(errs, fmt.Sprintf("%s: screenshot: %v", label, err))
			}
		}
		r := read()
		if r.Theme != "site" {
			errs = append(errs, fmt.Sprintf("%s: station theme is %q, want site", label, r.Theme))
		}
		arcs := make([]int, len(r.Clouds))
		for i, c := range r.Clouds {
			arcs[i] = c.Arcs
		}
		line := fmt.Sprintf("d27 %-22s size=%dx%d page=%q builds=%d clouds=%d arcs=%v ellipses=%d step=%.1f hash=%s ground=%s",
			label, r.W, r.H, r.Page, r.Builds, len(r.Clouds), arcs, r.Ellipses, r.Step, r.Hash, r.Ground)
		fmt.Println("uicheck: " + line)
		lines = append(lines, line)
		if shot {
			if data, err := evalString(`window.__sky.sky.toDataURL('image/png')`); err != nil {
				errs = append(errs, fmt.Sprintf("%s: sky dump: %v", label, err))
			} else if raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, "data:image/png;base64,")); err != nil {
				errs = append(errs, fmt.Sprintf("%s: sky decode: %v", label, err))
			} else {
				_ = os.WriteFile(filepath.Join("..", "..", "testdata", "uicheck", "out", "d27-"+tag+"-"+label+"-sky.png"), raw, 0o644)
			}
		}
		if !asserted {
			return r
		}
		if len(r.Clouds) != 6 {
			errs = append(errs, fmt.Sprintf("%s: %d offscreen clouds, want 6", label, len(r.Clouds)))
		}
		for i, c := range r.Clouds {
			if c.Arcs < 5 || c.Arcs > 8 {
				errs = append(errs, fmt.Sprintf("%s: cloud %d has %d puffs, want 5 to 8", label, i, c.Arcs))
			}
			if c.Draws != 1 || c.Alpha == nil || *c.Alpha <= 0 || *c.Alpha >= 1 {
				errs = append(errs, fmt.Sprintf("%s: cloud %d is composited %d times at alpha %v, want once below 1", label, i, c.Draws, c.Alpha))
			}
		}
		if r.Ellipses != 0 {
			errs = append(errs, fmt.Sprintf("%s: %d ellipses drawn straight onto the sky", label, r.Ellipses))
		}
		if r.Step > 12 {
			errs = append(errs, fmt.Sprintf("%s: hard edge in the sky, neighbouring pixels step by %.1f", label, r.Step))
		}
		return r
	}

	key("p")
	time.Sleep(900 * time.Millisecond)
	// The Station starts in the saved theme (space by default); the sky
	// being measured is the site one, so one T reaches it.
	if read().Theme == "space" {
		key("t")
		time.Sleep(400 * time.Millisecond)
	}
	a := measure("P-site-1600", true, true)

	// Seeded: a rebuild at the same size is the same sky.
	key("t")
	time.Sleep(700 * time.Millisecond)
	key("t")
	a2 := measure("P-site-1600-rebuilt", true, false)
	if a2.Builds <= a.Builds {
		errs = append(errs, fmt.Sprintf("rebuild: the sky was not rebuilt (builds %d to %d)", a.Builds, a2.Builds))
	}
	if a2.Hash != a.Hash {
		errs = append(errs, fmt.Sprintf("rebuild: sky hash %s then %s, the sky is not seeded", a.Hash, a2.Hash))
	}

	// Baked once: zooming and panning draw frames and never rebuild it.
	if _, err := evalRaw(`(function(){ var c = document.querySelector('#stationOverlay canvas'), r = c.getBoundingClientRect();
			c.dispatchEvent(new WheelEvent('wheel', {deltaY: -300, clientX: r.left + r.width / 2, clientY: r.top + r.height / 2, bubbles: true, cancelable: true})); return true; })()`); err != nil {
		errs = append(errs, fmt.Sprintf("zoom: %v", err))
	}
	time.Sleep(1500 * time.Millisecond)
	if z := read(); z.Builds != a2.Builds || z.Hash != a.Hash {
		errs = append(errs, fmt.Sprintf("zoom: the sky was rebuilt or changed while frames ran (builds %d to %d)", a2.Builds, z.Builds))
	}
	fmt.Printf("uicheck: d27 baked once: builds %d before and %d after a zoom and 1.5 s of frames\n", a2.Builds, read().Builds)

	// A resized window rebuilds the sky for the new canvas.
	if err := ensureWindowSizeWH(hwnd, 1100, 700); err != nil {
		errs = append(errs, fmt.Sprintf("resize: %v", err))
	}
	time.Sleep(600 * time.Millisecond)
	b := measure("P-site-1100x700", true, true)
	if b.Builds <= a2.Builds || b.W == a.W {
		errs = append(errs, fmt.Sprintf("resize: the sky did not follow the window (builds %d to %d, width %d to %d)", a2.Builds, b.Builds, a.W, b.W))
	}

	// The light page theme leaves the sky alone.
	key("l")
	time.Sleep(700 * time.Millisecond)
	c := measure("P-site-1100x700-light", true, true)
	if c.Page == startPage {
		errs = append(errs, fmt.Sprintf("light: the page theme stayed %q after L", c.Page))
	}
	if c.Hash != b.Hash {
		errs = append(errs, fmt.Sprintf("light: the sky changed with the page theme (%s then %s)", b.Hash, c.Hash))
	}
	key("p")

	dir := filepath.Join("..", "..", "testdata", "uicheck", "out")
	if err := os.MkdirAll(dir, 0o755); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "d27-"+tag+".txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	}
	if len(errs) > 0 {
		return fmt.Errorf("d27: %d problem(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}
	fmt.Println("uicheck: d27: six cumulus clouds, one alpha each, no hard step, seeded, baked once, follows the window, page theme leaves the sky alone")
	return nil
}

// station/d28: moved from check_d28.go.

// d28: the Station's hover card keeps one width and stays inside the window
// (02_roadmap\2026-10-03_station_ui_and_backlog.md, item F). The card's own
// left used to feed its shrink-to-fit width, so an unbreakable value (a long
// Cowork working directory was the original bug) made the card as wide as the
// window and a few px of pointer movement changed that width and shifted it.
// Fake mode feeds three sessions, the first with a 200 character project, a
// long model name and a long tool name, none with a break opportunity. The
// pointer is moved with synthetic mousemove events on the Station canvas (the
// real cursor is left alone), first to find the agent, then a few px around
// its centre. Each of P, O and I is checked on the site and space Station
// themes. Before the O and I hovers the page is scrolled so the whole Station
// is in view (scope 8 of the B7.2 spec): the app keeps the card inside the
// part of the Station the window shows, and a Station cut off by the fold is
// not what this case tests. Screenshots: d28-<view>-<theme>.
const (
	// d28MaxCardW matches the click card (d25MaxCardW is its looser bound):
	// "about 320 px", measured as the card's own border box.
	d28MaxCardW = 320.5
	// d28WidthJitter is the most the width may differ between two pointer
	// positions, in CSS px (sub-pixel layout rounding only).
	d28WidthJitter = 1.0
)

type d28Tip struct {
	Shown   bool    `json:"shown"`
	Left    float64 `json:"left"`
	Top     float64 `json:"top"`
	Right   float64 `json:"right"`
	Bottom  float64 `json:"bottom"`
	W       float64 `json:"w"`
	HostL   float64 `json:"hostL"`
	HostT   float64 `json:"hostT"`
	HostR   float64 `json:"hostR"`
	HostB   float64 `json:"hostB"`
	ScrollW float64 `json:"scrollW"`
	ClientW float64 `json:"clientW"`
	Clipped int     `json:"clipped"`
	IW      float64 `json:"iw"`
	IH      float64 `json:"ih"`
}

type d28Spot struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

const d28FakeJS = `(function(){
  var nowMs = Date.now(), ws = nowMs - 30 * 60000, chart = [];
  function iso(ms){ return new Date(ms).toISOString(); }
  for(var i = 0; i < 30; i++) chart.push({ at: iso(ws + i * 60000), by_session: {}, cost: 0 });
  var sep = String.fromCharCode(92), proj = 'C:';
  for(var k = 0; proj.length < 200; k++) proj += sep + 'local-agent-mode-sessions-segment-' + k;
  proj = proj.slice(0, 200);
  function s(id, vendor, agent, stage, tool, project, model){
    return {session_id: id, vendor: vendor, agent: agent, surface: agent, model: model, project: project, start: iso(nowMs - 600000),
      last_turn: iso(nowMs - 5000), turn_count: 4, context: 1000, context_window: 200000, tokens: 5000, cost: 0.1, cache_hit_ratio: 0,
      stage: stage, stage_since: iso(nowMs - 5000), stage_tool: tool};
  }
  var sessions = [s('fake-st-1', 'anthropic', 'claude-code', 'reading', 'mcp__' + 'y'.repeat(75), proj, 'a-very-long-model-name-that-has-no-break-' + 'x'.repeat(60)),
                  s('fake-st-2', 'openai', 'codex', 'running', 'exec', 'proj-fake-st-2', ''),
                  s('fake-st-3', 'anthropic', 'claude-code', 'waiting', '', 'proj-fake-st-3', '')];
  var snap = { now: nowMs, hist_gap_ms: 25000, sysmon: { Ts: iso(nowMs), pressure_score: 10, CPUPct: 10, Cores: [10, 10], MemUsedMB: 1000, MemTotalMB: 32000,
      SwapUsedMB: 0, SwapTotalMB: 1000, NetDownBps: 0, NetUpBps: 0, Disks: [], Wifi: { OK: true, Connected: false } },
    sysmon_history: [], process_groups_now: [], process_groups_history: [], vendor_strip: { rows: [] }, activity_heatmap: { rows: [] },
    todo: { enabled: false }, todo_tasks: { items: [] }, headline_today: 0,
    burn: { generated_at: iso(nowMs), running_window_seconds: 600, sessions: sessions, window_start: iso(ws), bucket_seconds: 60, chart: chart, turns: [] } };
  window.__bdevEnterFakeMode();
  window.__bdevPaintFake(snap);
  return true;
})()`

// d28ScanJS moves the pointer over the mounted Station canvas until the hover
// card shows the long-project agent, then walks out to the edges of its hit
// area and answers with the centre and size of that area, or null.
const d28ScanJS = `(function(){
  var cv = Array.prototype.slice.call(document.querySelectorAll('.bms-canvas')).filter(function(c){ return c.getClientRects().length > 0; })[0];
  if(!cv) return null;
  var r = cv.getBoundingClientRect();
  function at(x, y){
    cv.dispatchEvent(new MouseEvent('mousemove', {clientX: x, clientY: y, bubbles: true}));
    return Array.prototype.some.call(document.querySelectorAll('.bms-tip'), function(t){ return t.style.display === 'block' && t.textContent.indexOf('local-agent-mode') >= 0; });
  }
  for(var y = r.top + 10; y < r.bottom; y += 10){
    for(var x = r.left + 10; x < r.right; x += 10){
      if(!at(x, y)) continue;
      var xs = x, xe = x;
      while(xs - 2 > r.left && at(xs - 2, y)) xs -= 2;
      while(xe + 2 < r.right && at(xe + 2, y)) xe += 2;
      var cx = Math.round((xs + xe) / 2), ys = y, ye = y;
      while(ys - 2 > r.top && at(cx, ys - 2)) ys -= 2;
      while(ye + 2 < r.bottom && at(cx, ye + 2)) ye += 2;
      return {x: cx, y: Math.round((ys + ye) / 2), w: xe - xs, h: ye - ys};
    }
  }
  return null;
})()`

// d28HoverJS moves the pointer to (x, y) and reads the hover card.
const d28HoverJS = `(function(x, y){
  var cv = Array.prototype.slice.call(document.querySelectorAll('.bms-canvas')).filter(function(c){ return c.getClientRects().length > 0; })[0];
  cv.dispatchEvent(new MouseEvent('mousemove', {clientX: x, clientY: y, bubbles: true}));
  var t = Array.prototype.slice.call(document.querySelectorAll('.bms-tip')).filter(function(e){ return e.style.display === 'block'; })[0];
  if(!t) return {shown: false};
  var r = t.getBoundingClientRect(), clipped = 0;
  var hp = t.offsetParent ? t.offsetParent.getBoundingClientRect() : {left: 0, top: 0, right: innerWidth, bottom: innerHeight};
  t.querySelectorAll('*').forEach(function(el){
    var b = el.getBoundingClientRect();
    if(!b.width && !b.height) return;
    if(b.left < r.left - 1 || b.right > r.right + 1 || b.top < r.top - 1 || b.bottom > r.bottom + 1) clipped++;
  });
  return {shown: true, left: r.left, top: r.top, right: r.right, bottom: r.bottom, w: r.width,
    hostL: hp.left, hostT: hp.top, hostR: hp.right, hostB: hp.bottom,
    scrollW: t.scrollWidth, clientW: t.clientWidth, clipped: clipped, iw: innerWidth, ih: innerHeight};
})`

// d28ScrollJS scrolls #pageScroll so the top of #stationPanel sits at its
// top (spec 2026-10-28-burnmon-b7-2-uicheck-hygiene, scope 8, option b) and
// answers the rects. With argument false it only reads, so the caller can
// wait for the scroll to settle. The visible area is #pageScroll's box cut to
// the window.
const d28ScrollJS = `(function(move){
  var ps = document.getElementById('pageScroll'), box = document.getElementById('stationPanel');
  if(!ps || !box) return null;
  if(move) ps.scrollTop += box.getBoundingClientRect().top - ps.getBoundingClientRect().top;
  var b = box.getBoundingClientRect(), p = ps.getBoundingClientRect();
  return {scrollTop: ps.scrollTop, top: b.top, bottom: b.bottom, visTop: Math.max(p.top, 0), visBottom: Math.min(p.bottom, innerHeight)};
})`

type d28Scroll struct {
	ScrollTop float64 `json:"scrollTop"`
	Top       float64 `json:"top"`
	Bottom    float64 `json:"bottom"`
	VisTop    float64 `json:"visTop"`
	VisBottom float64 `json:"visBottom"`
}

// d28StationInView scrolls the Station fully into view, waits until two reads
// 100 ms apart agree and answers "" when the whole Station is in the visible
// area, else why not. The card is then judged against the Station, not
// against however much of it the live page height above it left on screen.
func d28StationInView() (string, error) {
	var last *d28Scroll
	if err := evalInto(d28ScrollJS+"(true)", &last); err != nil {
		return "", err
	}
	if last == nil {
		return "no #pageScroll or #stationPanel to scroll", nil
	}
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		var now *d28Scroll
		if err := evalInto(d28ScrollJS+"(false)", &now); err != nil {
			return "", err
		}
		if now == nil {
			return "#stationPanel went away while scrolling", nil
		}
		settled := *now == *last
		last = now
		if settled {
			break
		}
	}
	r := *last
	fmt.Printf("uicheck: d28: Station scrolled to scrollTop %.0f, Station %.0f..%.0f, visible %.0f..%.0f\n", r.ScrollTop, r.Top, r.Bottom, r.VisTop, r.VisBottom)
	if h, vh := r.Bottom-r.Top, r.VisBottom-r.VisTop; h > vh+0.5 {
		return fmt.Sprintf("the Station is %.0f px tall, taller than the %.0f px of #pageScroll in view, so it cannot be scrolled fully into view (not hovered)", h, vh), nil
	}
	if r.Top < r.VisTop-0.5 || r.Bottom > r.VisBottom+0.5 {
		return fmt.Sprintf("the Station %.0f..%.0f is not fully in the visible %.0f..%.0f after scrolling (not hovered)", r.Top, r.Bottom, r.VisTop, r.VisBottom), nil
	}
	return "", nil
}

// d28Wiggle is the pointer path around the agent's centre, in CSS px.
var d28Wiggle = [][2]float64{{0, 0}, {3, 0}, {6, 0}, {9, 0}, {12, 0}, {0, 3}, {-4, -4}, {-8, 2}, {2, 6}, {0, 0}}

// d28Problems lists what is wrong with the cards read along one wiggle.
func d28Problems(tips []d28Tip) []string {
	var p []string
	minW, maxW := 1e9, 0.0
	for i, t := range tips {
		if !t.Shown {
			p = append(p, fmt.Sprintf("step %d: the card is not shown", i))
			continue
		}
		if t.W < minW {
			minW = t.W
		}
		if t.W > maxW {
			maxW = t.W
		}
		if t.W > d28MaxCardW {
			p = append(p, fmt.Sprintf("step %d: card %.1f px wide, at most %.0f", i, t.W, d28MaxCardW))
		}
		if t.Left < 0 || t.Top < 0 || t.Right > t.IW || t.Bottom > t.IH {
			p = append(p, fmt.Sprintf("step %d: card (%.0f,%.0f)-(%.0f,%.0f) outside the %.0fx%.0f window", i, t.Left, t.Top, t.Right, t.Bottom, t.IW, t.IH))
		}
		if t.Left < t.HostL-0.5 || t.Top < t.HostT-0.5 || t.Right > t.HostR+0.5 || t.Bottom > t.HostB+0.5 {
			p = append(p, fmt.Sprintf("step %d: card (%.0f,%.0f)-(%.0f,%.0f) outside its Station (%.0f,%.0f)-(%.0f,%.0f)", i, t.Left, t.Top, t.Right, t.Bottom, t.HostL, t.HostT, t.HostR, t.HostB))
		}
		if t.Clipped > 0 || t.ScrollW > t.ClientW+1 {
			p = append(p, fmt.Sprintf("step %d: content clipped, %d elements outside the card, scrollWidth %.0f > clientWidth %.0f", i, t.Clipped, t.ScrollW, t.ClientW))
		}
	}
	if maxW-minW > d28WidthJitter {
		p = append(p, fmt.Sprintf("card width changed with the pointer, %.1f to %.1f px over %d tiny moves", minW, maxW, len(tips)))
	}
	return p
}

func caseD28(hwnd uintptr, args []string) error {
	time.Sleep(300 * time.Millisecond)
	if _, err := evalRaw(d28FakeJS); err != nil {
		return fmt.Errorf("d28: fake paint: %w", err)
	}
	bringToFront(hwnd)
	if err := clickSelectorAt(hwnd, "header .logo", 0.5, 0.5); err != nil {
		fmt.Println("uicheck: d28: could not click header .logo to establish focus:", err)
	}
	var errs []string
	// checkHover finds the long-project agent, wiggles the pointer over it
	// and judges the cards read on the way.
	checkHover := func(where string, scroll bool) error {
		if scroll {
			why, err := d28StationInView()
			if err != nil {
				return fmt.Errorf("%s: scroll: %w", where, err)
			}
			if why != "" {
				errs = append(errs, where+": "+why)
				return nil
			}
		}
		var spot *d28Spot
		if err := evalInto(d28ScanJS, &spot); err != nil {
			return fmt.Errorf("%s: scan: %w", where, err)
		}
		if spot == nil {
			errs = append(errs, where+": no agent with the long project found under the pointer")
			return nil
		}
		var tips []d28Tip
		for _, o := range d28Wiggle {
			if o[0] > spot.W/2-2 || -o[0] > spot.W/2-2 || o[1] > spot.H/2-2 || -o[1] > spot.H/2-2 {
				continue // would leave the agent's own hit area, not the card's fault
			}
			var t d28Tip
			if err := evalInto(fmt.Sprintf("%s(%.0f, %.0f)", d28HoverJS, spot.X+o[0], spot.Y+o[1]), &t); err != nil {
				return fmt.Errorf("%s: hover: %w", where, err)
			}
			tips = append(tips, t)
		}
		fmt.Printf("uicheck: d28 %s: agent at (%.0f,%.0f) hit area %.0fx%.0f, %d moves, widths:", where, spot.X, spot.Y, spot.W, spot.H, len(tips))
		for _, t := range tips {
			fmt.Printf(" %.1f", t.W)
		}
		fmt.Println()
		if len(tips) < 4 {
			errs = append(errs, fmt.Sprintf("%s: only %d usable pointer moves, the agent's hit area %.0fx%.0f is too small to wiggle over", where, len(tips), spot.W, spot.H))
			return nil
		}
		for _, m := range d28Problems(tips) {
			errs = append(errs, fmt.Sprintf("%s: %s", where, m))
		}
		// Back on the centre for the shot, so the card shown is the settled one.
		if _, err := evalRaw(fmt.Sprintf("%s(%.0f, %.0f)", d28HoverJS, spot.X, spot.Y)); err != nil {
			return err
		}
		return d23Shot(hwnd, "d28-"+strings.ReplaceAll(where, " ", "-"))
	}
	views := []struct {
		name string
		vk   uint16
		ok   func(d23State) bool
	}{
		{"P", vkP, func(s d23State) bool { return s.Overlay && s.Lit > 500 }},
		{"O", vkO, func(s d23State) bool { return s.Panel && s.View == "top" && s.Lit > 500 }},
		{"I", vkI, func(s d23State) bool { return s.Panel && s.View == "iso" && s.Lit > 500 }},
	}
	// Keys go to the foreground window, and a view runs for some 20 s: put the
	// window in front before every key, so a key lost to another window cannot
	// read as the app ignoring it.
	press := func(vk uint16) error {
		bringToFront(hwnd)
		return pressKey(vk)
	}
	for _, v := range views {
		if err := press(v.vk); err != nil {
			errs = append(errs, fmt.Sprintf("%s: press: %v", v.name, err))
			continue
		}
		s, good, err := d23Wait(v.ok)
		if err != nil || !good {
			errs = append(errs, fmt.Sprintf("%s did not open: %+v %v", v.name, s, err))
			continue
		}
		// The fake agents walk in from the airlock; the hit area moves until they sit.
		time.Sleep(12 * time.Second)
		bringToFront(hwnd)
		for i, th := range []string{"site", "space"} {
			if i > 0 {
				if err := press(vkT); err != nil {
					return err
				}
				time.Sleep(600 * time.Millisecond)
			}
			// O and I put the Station in the page; P is a full-window overlay.
			if err := checkHover(v.name+" "+th, v.name != "P"); err != nil {
				return err
			}
		}
		if err := press(vkT); err != nil { // back to site for the next view
			return err
		}
		time.Sleep(400 * time.Millisecond)
		if err := press(v.vk); err != nil { // the same key closes the view
			return err
		}
		if _, good, err := d23Wait(func(s d23State) bool { return !s.Overlay && !s.Panel }); err != nil || !good {
			fg, _, _ := procGetForegroundWindow.Call()
			focus, _ := evalString(`String(document.hasFocus())`)
			errs = append(errs, fmt.Sprintf("%s did not close on its own key (window in front %v, page focus %s)", v.name, fg == hwnd, focus))
			_ = press(vkEscape)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("d28: %d problem(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}
	fmt.Println("uicheck: d28: the Station hover card keeps one width, at most 320 px, wraps long values and stays inside the window in P, O and I on both Station themes")
	return nil
}

// station/d41: moved from check_d41.go.

// d41: the Station takes exactly the height of the parts it replaces (B7.1,
// spec 2026-10-28, item 4). At 1280x800 and 1920x1080 (a size the screen
// cannot hold prints SKIPPED and is not a pass), Standard, Overall scrollbar
// off, three variants: all parts on; turn ticker off; saved shares injected
// through heights_standard (burn_30min 0.25, vendor_strip 0.15,
// activity_heatmap 0.3, turn_ticker 0.3, system_chart 0.6, process_groups 0.4).
// Each variant applies the payload and the frozen example through fake mode,
// reads the page, presses a real O (the window must be in front for keys),
// reads again, presses O again and reads a third time.
// Asserted: while open the Station box is within 2 px of the span the parts had
// (bottom of the last minus top of the first); the burn zone, the system zone,
// main and the System chart keep their top and height within 1 px; the zone
// label stays; after the second O there is no box and every part has its top
// and height back within 1 px. Screenshot d41-station-<size>-<variant> while
// the Station is open.

const d41ReadJS = `(function(){
  var shown = function(e){ return !!e && e.getClientRects().length > 0 && getComputedStyle(e).display !== 'none'; };
  var zb = document.querySelector('.zone.burn .zonebody');
  var parts = [], top = Infinity, bot = -Infinity, box = 0;
  Array.prototype.forEach.call(zb.children, function(c){
    if(c.classList.contains('hdl')) return;
    var r = c.getBoundingClientRect();
    if(c.id === 'stationPanel'){ box = shown(c) ? r.height : 0; return; }
    if(!shown(c) || r.height <= 0.5) return;
    parts.push({id: c.id || c.className, top: r.top, h: r.height});
    top = Math.min(top, r.top); bot = Math.max(bot, r.bottom);
  });
  var rc = function(s){ var e = document.querySelector(s); if(!e || !shown(e)) return {top: 0, h: 0}; var r = e.getBoundingClientRect(); return {top: r.top, h: r.height}; };
  var hw = document.querySelector('.histwrap');
  return {span: bot > top ? bot - top : 0, parts: parts, box: box, burn: rc('.zone.burn'), sys: rc('.zone.system'), main: rc('main'),
    hist: hw ? hw.getBoundingClientRect().height : 0, label: shown(document.querySelector('.zone.burn .zonelabel'))};
})()`

type d41Box struct {
	Top float64 `json:"top"`
	H   float64 `json:"h"`
}

type d41Part struct {
	ID  string  `json:"id"`
	Top float64 `json:"top"`
	H   float64 `json:"h"`
}

type d41Read struct {
	Span  float64   `json:"span"`
	Parts []d41Part `json:"parts"`
	Box   float64   `json:"box"`
	Burn  d41Box    `json:"burn"`
	Sys   d41Box    `json:"sys"`
	Main  d41Box    `json:"main"`
	Hist  float64   `json:"hist"`
	Label bool      `json:"label"`
}

// d41Payload is d29Payload with extra top-level fields set on the decoded map.
func d41Payload(o d29Opts, extra map[string]any) (string, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(d29Payload(o)), &m); err != nil {
		return "", err
	}
	for k, v := range extra {
		m[k] = v
	}
	b, err := json.Marshal(m)
	return string(b), err
}

func d41Close(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// d41PressO presses a real O and waits (up to 1.5 s) for the Station box to be
// there (want true) or gone (want false). Keys need the window to hold input
// focus, which drifts now and then, so when the state did not change it gets
// focus back with a real click on the logo (d23's way) and presses once more.
// The state is checked before the retry, so a key that did arrive is never
// pressed twice. After four tries it fails the run with the focus as the cause.
func d41PressO(hwnd uintptr, want bool) error {
	for try := 0; try < 4; try++ {
		bringToFront(hwnd)
		if try > 0 {
			time.Sleep(500 * time.Millisecond)
			fmt.Println("uicheck: d41: O had no effect, taking focus with a click and pressing again")
		}
		if err := clickSelectorAt(hwnd, ".logo", 0.5, 0.5); err != nil {
			fmt.Println("uicheck: d41: could not click .logo to establish focus:", err)
		}
		time.Sleep(200 * time.Millisecond)
		if err := pressKey(vkO); err != nil {
			return err
		}
		for i := 0; i < 8; i++ {
			time.Sleep(200 * time.Millisecond)
			var has bool
			if err := evalInto(`!!document.getElementById('stationPanel')`, &has); err != nil {
				return err
			}
			if has == want {
				return nil
			}
		}
	}
	return fmt.Errorf("d41: O had no effect after 4 tries (the window did not take input focus), want the Station box present=%v", want)
}

func caseD41(hwnd uintptr, args []string) error {
	snap, err := d34Snapshot(false)
	if err != nil {
		return err
	}
	shares := map[string]any{"burn_30min": 0.25, "vendor_strip": 0.15, "activity_heatmap": 0.3, "turn_ticker": 0.3, "system_chart": 0.6, "process_groups": 0.4}
	variants := []struct {
		name  string
		off   map[string]bool
		extra map[string]any
	}{
		{"all", nil, nil},
		{"no-ticker", map[string]bool{"turn_ticker": true}, nil},
		{"saved-shares", nil, map[string]any{"heights_standard": shares}},
	}
	var fails []string
	ran := 0
	for _, sz := range [][2]int32{{1280, 800}, {1920, 1080}} {
		tag := fmt.Sprintf("%dx%d", sz[0], sz[1])
		if err := ensureWindowSizeWH(hwnd, sz[0], sz[1]); err != nil {
			return err
		}
		if windowCapped {
			fmt.Printf("uicheck: d41: SKIPPED %s, this screen is too small\n", tag)
			continue
		}
		time.Sleep(500 * time.Millisecond)
		for _, v := range variants {
			payload, err := d41Payload(d29Opts{Status: "ok", Snap: snap, StandardOff: v.off}, v.extra)
			if err != nil {
				return err
			}
			if _, err := d34Apply(payload, snap); err != nil {
				return err
			}
			var r0, r1, r2 d41Read
			if err := evalInto(d41ReadJS, &r0); err != nil {
				return err
			}
			if err := d41PressO(hwnd, true); err != nil {
				return err
			}
			time.Sleep(700 * time.Millisecond)
			err = evalInto(d41ReadJS, &r1)
			_, shotErr := screenshot(hwnd, fmt.Sprintf("d41-station-%s-%s", tag, v.name))
			if err == nil {
				err = shotErr
			}
			// Close the Station even when a read failed.
			if kerr := d41PressO(hwnd, false); err == nil {
				err = kerr
			}
			time.Sleep(700 * time.Millisecond)
			if err != nil {
				return err
			}
			if err := evalInto(d41ReadJS, &r2); err != nil {
				return err
			}
			var bad []string
			if len(r0.Parts) == 0 || r0.Span <= 0 {
				bad = append(bad, "no parts in the burn zone before the swap")
			}
			if r1.Box <= 0 || !d41Close(r1.Box, r0.Span, 2) {
				diag, _ := evalString(`"focus " + document.hasFocus() + ", view " + window.__bdevStationView() + ", active " + (document.activeElement ? document.activeElement.tagName + "#" + document.activeElement.id : "-")`)
				bad = append(bad, fmt.Sprintf("box %.1f px vs span %.1f px (%s)", r1.Box, r0.Span, diag))
			}
			for _, p := range []struct {
				n    string
				a, b d41Box
			}{{"burn zone", r0.Burn, r1.Burn}, {"system zone", r0.Sys, r1.Sys}, {"main", r0.Main, r1.Main}} {
				if !d41Close(p.a.Top, p.b.Top, 1) || !d41Close(p.a.H, p.b.H, 1) {
					bad = append(bad, fmt.Sprintf("%s moved (top %.1f to %.1f, height %.1f to %.1f)", p.n, p.a.Top, p.b.Top, p.a.H, p.b.H))
				}
			}
			if !d41Close(r0.Hist, r1.Hist, 1) {
				bad = append(bad, fmt.Sprintf("System chart height %.1f to %.1f", r0.Hist, r1.Hist))
			}
			if !r1.Label {
				bad = append(bad, "the zone label is hidden")
			}
			if r2.Box != 0 {
				bad = append(bad, "the box is still there after the second O")
			}
			if len(r2.Parts) != len(r0.Parts) {
				bad = append(bad, fmt.Sprintf("%d parts back, want %d", len(r2.Parts), len(r0.Parts)))
			} else {
				for i := range r0.Parts {
					a, b := r0.Parts[i], r2.Parts[i]
					if !d41Close(a.Top, b.Top, 1) || !d41Close(a.H, b.H, 1) {
						bad = append(bad, fmt.Sprintf("part %s back at top %.1f height %.1f, was %.1f and %.1f", a.ID, b.Top, b.H, a.Top, a.H))
					}
				}
			}
			ran++
			if len(bad) == 0 {
				fmt.Printf("uicheck: d41: PASS %s %s (%d parts, span %.1f px, box %.1f px, burn zone %.1f px, main %.1f px, label shown, parts back)\n", tag, v.name, len(r0.Parts), r0.Span, r1.Box, r1.Burn.H, r1.Main.H)
			} else {
				msg := fmt.Sprintf("%s %s: %s", tag, v.name, strings.Join(bad, "; "))
				fmt.Println("uicheck: d41: FAIL " + msg)
				fails = append(fails, msg)
			}
		}
	}
	if len(fails) > 0 {
		return fmt.Errorf("d41: %d case(s) failed, first: %s", len(fails), fails[0])
	}
	if ran == 0 {
		return fmt.Errorf("d41: no size ran")
	}
	return nil
}

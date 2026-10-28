package main

import (
	"fmt"
	"strings"
	"time"
)

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

func init() {
	checks["d23"] = func(hwnd uintptr, args []string) error {
		if err := ensureWindowSizeWH(hwnd, 1600, 900); err != nil {
			return err
		}
		time.Sleep(300 * time.Millisecond)
		defer evalRaw(`(window.__bdevExitFakeMode && window.__bdevExitFakeMode())`)
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
		if err := clickSelectorAt(hwnd, ".logo", 0.5, 0.5); err != nil {
			fmt.Println("uicheck: d23: could not click .logo to establish focus:", err)
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
}

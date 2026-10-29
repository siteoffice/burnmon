package main

import (
	"fmt"
	"strings"
	"time"
)

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
	if err := ensureWindowSizeWH(hwnd, 1600, 900); err != nil {
		return err
	}
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

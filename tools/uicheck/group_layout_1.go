package main

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"
)

// layout/d19: moved from check_d19.go.

// d19: with the To Do panel visible it is never hidden (B7 replaced the old
// 70 percent chart rule and the drop order): the fit pass shares the free
// height, To Do reaches the window bottom edge (the end of the scrolled
// page when every part sits at its floor) and no zone overflows. At a window
// too small for To Do in the plain layout (1280x860 today) it sits at its
// floor of three rows and the page scrolls. The three To Do
// on sizes are ones that fit this laptop's primary screen: uicheck places
// the window at (100,100) there and caps it to that screen, so the portrait
// 1152x2048 size cannot be reached from here. Fake mode only, with made-up
// task titles: a uicheck run never shows real To Do rows (todo_gate.go).
// One screenshot per case.
func caseD19(hwnd uintptr, args []string) error {
	cases := []struct {
		w, h       int32
		todo       bool
		expectDrop bool
	}{{1600, 1000, true, false}, {1920, 1080, true, false}, {2560, 1300, true, false}, {1920, 1080, false, false}, {1280, 860, true, true}}
	var errs []string
	skipped := 0
	// Own starting state, so the order of checks does not matter: d29 leaves
	// the page in Plain with a stale reading. Standard, every part on,
	// Overall scrollbar off, a fresh reading.
	if _, err := evalRaw(`window.__bdevSections.apply(` + d29Payload(d29Opts{Status: "ok"}) + `)`); err != nil {
		return fmt.Errorf("d19: set the starting state: %w", err)
	}
	for _, c := range cases {
		if err := ensureWindowSizeWH(hwnd, c.w, c.h); err != nil {
			return err
		}
		// A window the screen could not hold was never this size, so its
		// numbers say nothing about the layout at it (a laptop-only
		// 1600x1000 screen failed this check for that reason).
		if windowCapped {
			fmt.Printf("uicheck: d19 %dx%d todo=%v: SKIPPED, this screen is too small for that window\n", c.w, c.h, c.todo)
			skipped++
			continue
		}
		time.Sleep(500 * time.Millisecond)
		script := fmt.Sprintf(`(function(todoOn){
  var nowMs = Date.now(), windowMs = 30 * 60 * 1000, ws = nowMs - windowMs;
  var sys = [], groups = [], chart = [];
  for(var t = nowMs - windowMs; t <= nowMs; t += 1000){
    var s = t / 1000;
    sys.push({ Ts: new Date(t).toISOString(), CPUPct: 35 + 25 * Math.sin(s / 90), MemUsedMB: 9000, MemTotalMB: 16000,
      DiskReadBps: 1e6, DiskWriteBps: 4e5, NetDownBps: 2e5, NetUpBps: 1e5, GPUPct: 12 });
    ['claude', 'codex'].forEach(function(h){ groups.push({ Ts: new Date(t).toISOString(), Harness: h, CPUPct: 4, MemMB: 400, IOBps: 100 }); });
  }
  for(var i = 0; i < 30; i++) chart.push({ at: new Date(ws + i * 60000).toISOString(), by_session: {}, cost: 0 });
  var items = [];
  for(var k = 1; k <= 14; k++) items.push({ Title: 'Example task ' + k, List: 'Sample list', DueDate: '2026-09-29', Overdue: k %% 5 === 0 });
  var nowIso = new Date(nowMs).toISOString();
  var snap = {
    now: nowMs, hist_gap_ms: 25000, sysmon_history: sys,
    process_groups_now: [{ Ts: nowIso, Harness: 'claude', CPUPct: 4, MemMB: 400, IOBps: 100 }, { Ts: nowIso, Harness: 'codex', CPUPct: 4, MemMB: 400, IOBps: 100 }],
    process_groups_history: groups,
    vendor_strip: { rows: [] }, activity_heatmap: { rows: [] },
    todo: todoOn ? { enabled: true, signed_in: true } : { enabled: false },
    todo_tasks: { items: todoOn ? items : [] }, headline_today: 0,
    burn: { generated_at: nowIso, running_window_seconds: 1800, sessions: [],
      window_start: new Date(ws).toISOString(), bucket_seconds: 60, chart: chart, turns: [] }
  };
  window.__bdevEnterFakeMode();
  window.__bdevPaintFake(snap);
  // Standard scrolls (R8 follow-up): the older zones, To Do included, fill the window's height below the
  // sections, so scroll to the end before measuring where To Do sits.
  document.getElementById('pageScroll').scrollTop = 1e9;
  var todo = document.getElementById('todoPanel');
  var tr = todo.getBoundingClientRect();
  var zones = document.querySelectorAll('.zonebody'), zoneOver = false;
  for(var z = 0; z < zones.length; z++) if(zones[z].scrollHeight > zones[z].clientHeight + 1) zoneOver = true;
  return {
    layout: window.__bdevTodoLayout,
    todoDropped: todo.classList.contains('fit-dropped'),
    todoBottom: tr.bottom, innerH: window.innerHeight,
    bodyOver: document.body.scrollHeight > document.body.clientHeight + 1 || document.body.scrollWidth > document.body.clientWidth + 1,
    zoneOver: zoneOver
  };
})(%v)`, c.todo)
		var out struct {
			Layout struct {
				Applied    bool    `json:"applied"`
				PlainHistH float64 `json:"plainHistH"`
				HistH      float64 `json:"histH"`
			} `json:"layout"`
			TodoDropped bool    `json:"todoDropped"`
			TodoBottom  float64 `json:"todoBottom"`
			InnerH      float64 `json:"innerH"`
			BodyOver    bool    `json:"bodyOver"`
			ZoneOver    bool    `json:"zoneOver"`
		}
		if err := evalInto(script, &out); err != nil {
			return fmt.Errorf("d19 %dx%d: eval layout: %w", c.w, c.h, err)
		}
		time.Sleep(400 * time.Millisecond)
		state := "off"
		if c.todo {
			state = "on"
		}
		if _, err := screenshot(hwnd, fmt.Sprintf("d19-todo-%s-%dx%d", state, c.w, c.h)); err != nil {
			return err
		}
		l := out.Layout
		ratio := 0.0
		if l.PlainHistH > 0 {
			ratio = l.HistH / l.PlainHistH
		}
		fmt.Printf("uicheck: d19 %dx%d todo=%s: applied=%v chart %.0fpx of plain %.0fpx (ratio %.3f), To Do bottom %.0f of %.0f, dropped=%v\n",
			c.w, c.h, state, l.Applied, l.HistH, l.PlainHistH, ratio, out.TodoBottom, out.InnerH, out.TodoDropped)
		if out.BodyOver || out.ZoneOver {
			errs = append(errs, fmt.Sprintf("%dx%d todo=%s: page or a zone overflows", c.w, c.h, state))
		}
		if !c.todo {
			continue
		}
		if out.TodoDropped {
			errs = append(errs, fmt.Sprintf("%dx%d: To Do is hidden, B7 never hides a part", c.w, c.h))
			continue
		}
		// .todopanel's own 4px bottom margin.
		if math.Abs(out.InnerH-4-out.TodoBottom) > 2 {
			errs = append(errs, fmt.Sprintf("%dx%d: To Do ends at %.0f, want the window bottom (%.0f)", c.w, c.h, out.TodoBottom, out.InnerH-4))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("d19: %d problem(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}
	if skipped > 0 {
		fmt.Printf("uicheck: d19: PARTIAL, %d of %d cases skipped because the screen is too small; run on a screen that holds 2560x1300 CSS px for a full pass\n", skipped, len(cases))
		return nil
	}
	fmt.Println("uicheck: d19: To Do layout clean (To Do never hidden, fills the bottom, no overflow)")
	return nil
}

// layout/d29: moved from check_d29.go.

// d29: the sections frame (agent control plan, row R8) with B2's parts: four
// groups of 15 parts, one tick per part per mode, and the Overall scrollbar
// switch. Fed lane A's frozen example (testdata\snapshot_example.json, the
// file docs\snapshot_schema.md describes) it proves:
//   - Standard with every part on shows all of them, drops none and the page
//     scrolls at the window the screen grants (1152x2048 here);
//   - each part switched off alone hides only that part; the older zones follow
//     their parts; with all 15 off the host is empty and nothing breaks;
//   - Plain shows its own list (the five R8 panels), in plain words, and fits
//     the window; its list can take a burn-zone part; at 1024x480 with all
//     on it hides nothing and the page scrolls (B7);
//   - the Plain toggle's text flips between "Plain: off" and "Plain: on";
//   - the gear lists four groups of parts, Overall scrollbar on top, the
//     "Station starts as" pair below; choosing a Station theme calls the
//     binding and does not switch the Station that is open;
//   - a stale or missing snapshot shows the payload's snapshot_line in the
//     header and the notice (never "not running") and draws no old numbers;
//   - the toggle, the scrollbar and a tick saved through the real bindings land
//     where they belong. Survival of a restart is the Go test of
//     internal\sections, not a real restart of the exe.
//
// The page is driven through window.__bdevSections.apply, which draws without
// saving. The persistence step at the end uses the real bindings and puts back
// what it found. Screenshots: d29-standard, d29-plain, d29-plain-small,
// d29-gear, d29-not-running.
var d29Parts = []string{
	"waits", "outcomes", "burn", "git", "agents_map", "sessions",
	"burn_30min", "vendor_strip", "activity", "heatmap", "turn_ticker",
	"system", "process_groups", "processes",
	"todo",
}

var d29PlainDefault = map[string]bool{"waits": true, "outcomes": true, "burn": true, "agents_map": true, "sessions": true}

type d29Opts struct {
	Plain, Scrollbar bool
	StandardOff      map[string]bool
	PlainOn          map[string]bool
	PlainAll         bool
	PlainOnly        map[string]bool
	Status           string
	Snap             json.RawMessage
	StationTheme     string
}

func d29Payload(o d29Opts) string {
	standard, plainSet := map[string]bool{}, map[string]bool{}
	for _, id := range d29Parts {
		standard[id] = !o.StandardOff[id]
		plainSet[id] = o.PlainAll || d29PlainDefault[id] || o.PlainOn[id]
		if o.PlainOnly != nil {
			plainSet[id] = o.PlainOnly[id]
		}
	}
	current := standard
	if o.Plain {
		current = plainSet
	}
	reading := map[string]any{"status": o.Status, "age_seconds": 7}
	if o.Snap != nil {
		reading["snapshot"] = o.Snap
	}
	// The line Go sends for an unknown writer state (B7.3 sentence table),
	// with the age of 7 s used above.
	line := map[string]any{"text": "snapshot 7 s old", "warn": false}
	switch o.Status {
	case "stale":
		line = map[string]any{"text": "No process has written the snapshot for 7 s.", "warn": true}
	case "missing":
		line = map[string]any{"text": "No snapshot file yet.", "warn": true}
	}
	theme := o.StationTheme
	if theme == "" {
		theme = "space"
	}
	part := func(id, label string) map[string]string { return map[string]string{"id": id, "label": label} }
	groups := []map[string]any{
		{"id": "status", "label": "Status", "parts": []map[string]string{
			part("waits", "Waits on you"), part("outcomes", "Outcomes"), part("burn", "Burn"),
			part("git", "Git"), part("agents_map", "Agents map"), part("sessions", "Sessions")}},
		{"id": "sessions_30min", "label": "Sessions last 30 min", "parts": []map[string]string{
			part("burn_30min", "Burn last 30 min"), part("vendor_strip", "Vendor strip"),
			part("activity", "Activity"), part("heatmap", "Heatmap harness x minute"),
			part("turn_ticker", "Turn ticker")}},
		{"id": "processes_30min", "label": "Processes last 30 min", "parts": []map[string]string{
			part("system", "System"), part("process_groups", "Process groups (with memory, disks and network)"),
			part("processes", "Processes")}},
		{"id": "todo", "label": "Microsoft To Do", "parts": []map[string]string{part("todo", "To Do")}},
	}
	b, _ := json.Marshal(map[string]any{
		"plain_mode": o.Plain, "overall_scrollbar": o.Scrollbar,
		"mode":  map[bool]string{true: "plain", false: "standard"}[o.Plain],
		"parts": current, "parts_standard": standard, "parts_plain": plainSet,
		"groups": groups, "station_theme": theme, "reading": reading,
		"writer": map[string]any{"state": ""}, "snapshot_line": line,
	})
	return string(b)
}

type d29Page struct {
	Parts        []string `json:"parts"`
	TodoOn       bool     `json:"todoOn"`
	Plain        bool     `json:"plain"`
	Toggle       string   `json:"toggle"`
	ToggleOn     bool     `json:"toggleOn"`
	BodyStandard bool     `json:"bodyStandard"`
	BurnZone     bool     `json:"burnZone"`
	SystemZone   bool     `json:"systemZone"`
	MainShown    bool     `json:"mainShown"`
	Header       string   `json:"header"`
	Notice       string   `json:"notice"`
	Text         string   `json:"text"`
	Scrolls      bool     `json:"scrolls"`
	OverflowY    string   `json:"overflowY"`
	HostOver     float64  `json:"hostOver"`
	BodyOver     float64  `json:"bodyOver"`
	AgentsSlot   bool     `json:"agentsSlot"`
	StripClip    float64  `json:"stripClip"`
	StripShrunk  bool     `json:"stripShrunk"`
	StripRows    int      `json:"stripRows"`
	StripWhole   int      `json:"stripWhole"`
	GearOpen     bool     `json:"gearOpen"`
	Ticks        []string `json:"ticks"`
	Headings     []string `json:"headings"`
	LabelRows    int      `json:"labelRows"`
	Checkboxes   int      `json:"checkboxes"`
	ScrollFirst  bool     `json:"scrollFirst"`
	ScrollTick   bool     `json:"scrollTick"`
	Radio        string   `json:"radio"`
}

// shown lists the parts on the page, in d29Parts order. To Do counts when its
// panel lacks .sec-off, since uicheck runs with To Do disabled.
func (p d29Page) shown() []string {
	var ids []string
	for _, id := range d29Parts {
		if id == "todo" {
			if p.TodoOn {
				ids = append(ids, id)
			}
			continue
		}
		for _, s := range p.Parts {
			if s == id {
				ids = append(ids, id)
				break
			}
		}
	}
	return ids
}

const d29ReadJS = `(function(){
  var disp = function(e){ return !!e && e.getClientRects().length > 0; };
  var q = function(s){ return document.querySelector(s); };
  var ps = document.getElementById('pageScroll'), host = document.getElementById('secHost');
  var strip = document.getElementById('vendorStripPanel'), tg = document.getElementById('plainToggle');
  var ticks = [];
  Array.prototype.forEach.call(document.querySelectorAll('#secSettingsCols input[data-sw]'), function(i){
    if(i.checked) ticks.push(i.getAttribute('data-mode') + ':' + i.getAttribute('data-sw'));
  });
  var seen = {};
  Array.prototype.forEach.call(document.querySelectorAll('[data-part]'), function(e){
    if(disp(e)) seen[e.getAttribute('data-part')] = true;
  });
  var sb = document.getElementById('secScrollbar'), firstH = q('#secSettingsCols h4'), radio = q('input[name="secStationStart"]:checked');
  return {
    parts: Object.keys(seen),
    todoOn: !document.getElementById('todoPanel').classList.contains('sec-off'),
    plain: !!window.__bdevSections.state().plain,
    toggle: tg.textContent, toggleOn: tg.classList.contains('on'),
    bodyStandard: document.body.classList.contains('scroll-on'),
    burnZone: disp(q('.zone.burn')), systemZone: disp(q('.zone.system')), mainShown: disp(q('main')),
    header: document.getElementById('hdrSnapAge').textContent,
    notice: document.getElementById('secNotice').textContent,
    text: document.getElementById('secPanels').textContent,
    scrolls: ps.scrollHeight > ps.clientHeight + 1, overflowY: getComputedStyle(ps).overflowY,
    hostOver: host.scrollHeight - host.clientHeight, bodyOver: document.body.scrollHeight - document.body.clientHeight,
    agentsSlot: !!document.getElementById('secAgentsSlot'),
    stripClip: strip ? strip.scrollHeight - strip.clientHeight : 0,
    stripShrunk: !!(window.__bdevFit && window.__bdevFit.items || []).filter(function(it){ return it.id === 'vendorStripPanel' && it.h < it.nat; }).length,
    stripRows: strip ? strip.querySelectorAll('tbody tr').length : 0,
    stripWhole: strip ? Array.prototype.filter.call(strip.querySelectorAll('tbody tr'), function(r){ return r.getBoundingClientRect().bottom <= strip.getBoundingClientRect().bottom + 1; }).length : 0,
    gearOpen: document.getElementById('secSettings').classList.contains('open'),
    ticks: ticks,
    headings: Array.prototype.map.call(document.querySelectorAll('#secSettingsCols h4'), function(h){ return h.textContent; }),
    labelRows: document.querySelectorAll('#secSettingsCols .setlbl').length,
    checkboxes: document.querySelectorAll('#secSettingsCols input[data-sw]').length,
    scrollFirst: !!sb && !!firstH && !!(sb.compareDocumentPosition(firstH) & Node.DOCUMENT_POSITION_FOLLOWING),
    scrollTick: !!sb && sb.checked,
    radio: radio ? radio.value : ''
  };
})()`

func d29Apply(payload string) (d29Page, error) {
	var p d29Page
	if _, err := evalRaw(`window.__bdevSections.apply(` + payload + `)`); err != nil {
		return p, err
	}
	return d29Read()
}

func d29Read() (d29Page, error) {
	var p d29Page
	time.Sleep(500 * time.Millisecond)
	err := evalInto(d29ReadJS, &p)
	return p, err
}

func d29Eval(js string) error {
	_, err := evalRaw(js)
	time.Sleep(300 * time.Millisecond)
	return err
}

func d29Fail(format string, args ...any) error { return fmt.Errorf("d29: "+format, args...) }

// d29StripCheck: the vendor strip's last row must not be cut by its own
// max-height cap (R8). Since B7 the fit pass may shrink it as a list toward its
// floor, a table's header plus two rows, also with the scrollbar on (the
// legacy block is one window tall); the rows below the cut then clip, as the
// comment above #vendorStripPanel in page.html accepts. So a cut is only fine
// when the fit pass shrank the strip, and even then two rows stay whole.
func d29StripCheck(p d29Page) error {
	if !p.StripShrunk {
		if p.StripClip > 1 {
			return d29Fail("standard: the vendor strip is clipped by %.0f px", p.StripClip)
		}
		return nil
	}
	want := 2
	if p.StripRows < want {
		want = p.StripRows
	}
	if p.StripWhole < want {
		return d29Fail("standard: the fit pass shrank the vendor strip below its floor: %d of %d rows whole, want %d", p.StripWhole, p.StripRows, want)
	}
	return nil
}

func caseD29(hwnd uintptr, args []string) error {
	snap, err := readFixture("snapshot_example.json")
	if err != nil {
		return fmt.Errorf("read the frozen example (uicheck runs from the tools folder): %w", err)
	}
	if err := ensureWindowSizeWH(hwnd, 1152, 2048); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)
	std := func(off map[string]bool) d29Opts {
		return d29Opts{Scrollbar: true, StandardOff: off, Status: "ok", Snap: snap}
	}
	wantAll := strings.Join(d29Parts, ",")

	// Standard, every part on, scrollbar on: all shown, none dropped, the page scrolls.
	p, err := d29Apply(d29Payload(std(nil)))
	if err != nil {
		return err
	}
	if got := strings.Join(p.shown(), ","); got != wantAll {
		return d29Fail("standard shows %v, want all 15", p.shown())
	}
	if p.Plain || p.Toggle != "Plain: off" || p.ToggleOn || !p.BodyStandard || !p.BurnZone || !p.SystemZone {
		return d29Fail("standard shape wrong: %+v", p)
	}
	if !p.Scrolls || p.OverflowY != "auto" {
		return d29Fail("standard page does not scroll (scrolls %v, overflow-y %s)", p.Scrolls, p.OverflowY)
	}
	if p.BodyOver > 1 {
		return d29Fail("standard: the body itself overflows by %.0f px", p.BodyOver)
	}
	if !strings.Contains(p.Header, "snapshot 7 s old") {
		return d29Fail("header %q does not show the snapshot age", p.Header)
	}
	for _, want := range []string{"project", "API price equivalent", "Plan limits", "Not built yet", "G4", "120"} {
		if !strings.Contains(p.Text, want) {
			return d29Fail("panels do not show %q from the example", want)
		}
	}
	if !p.AgentsSlot {
		return d29Fail("agents map has no slot for R7")
	}
	if err := d29StripCheck(p); err != nil {
		return err
	}
	if _, err := screenshot(hwnd, "d29-standard"); err != nil {
		return err
	}

	// One part off at a time: only that part goes.
	for _, id := range d29Parts {
		p, err := d29Apply(d29Payload(std(map[string]bool{id: true})))
		if err != nil {
			return err
		}
		var want []string
		for _, x := range d29Parts {
			if x != id {
				want = append(want, x)
			}
		}
		if got := strings.Join(p.shown(), ","); got != strings.Join(want, ",") {
			return d29Fail("switching %s off left parts %v", id, p.shown())
		}
	}
	// The older zones follow their parts.
	burnOff := map[string]bool{"burn_30min": true, "vendor_strip": true, "activity": true, "heatmap": true, "turn_ticker": true}
	if p, err = d29Apply(d29Payload(std(burnOff))); err != nil {
		return err
	}
	if p.BurnZone || !p.SystemZone {
		return d29Fail("five burn-zone parts off: burn zone %v, system zone %v", p.BurnZone, p.SystemZone)
	}
	sysOff := map[string]bool{"system": true, "process_groups": true}
	if p, err = d29Apply(d29Payload(std(sysOff))); err != nil {
		return err
	}
	if p.SystemZone || !p.BurnZone {
		return d29Fail("system and process_groups off: burn zone %v, system zone %v", p.BurnZone, p.SystemZone)
	}

	// Every part off: an empty host, no zones, no error.
	allOff := map[string]bool{}
	for _, id := range d29Parts {
		allOff[id] = true
	}
	if p, err = d29Apply(d29Payload(std(allOff))); err != nil {
		return d29Fail("all 15 parts off raised: %v", err)
	}
	if len(p.shown()) != 0 || p.MainShown || p.BurnZone || p.SystemZone {
		return d29Fail("all parts off: shown %v, main %v", p.shown(), p.MainShown)
	}

	// Plain, scrollbar off: the Plain list, plain words, no older zones, fits the window.
	plain := func(o d29Opts) d29Opts {
		o.Plain, o.Status, o.Snap = true, "ok", snap
		return o
	}
	if p, err = d29Apply(d29Payload(plain(d29Opts{}))); err != nil {
		return err
	}
	if got := strings.Join(p.shown(), ","); got != "waits,outcomes,burn,agents_map,sessions" {
		return d29Fail("plain shows %v", p.shown())
	}
	if !p.Plain || p.Toggle != "Plain: on" || !p.ToggleOn || p.BodyStandard || p.BurnZone || p.SystemZone {
		return d29Fail("plain shape wrong: %+v", p)
	}
	if p.OverflowY != "hidden" {
		return d29Fail("plain page can scroll (overflow-y %s)", p.OverflowY)
	}
	if p.HostOver > 1 {
		return d29Fail("plain at this size: host overflow %.0f px", p.HostOver)
	}
	for _, jargon := range []string{"tokens/min", "API price equiv."} {
		if strings.Contains(p.Text, jargon) {
			return d29Fail("plain mode still shows %q", jargon)
		}
	}
	if !strings.Contains(p.Text, "waiting for you") {
		return d29Fail("plain mode did not put the state in plain words")
	}
	if _, err := screenshot(hwnd, "d29-plain"); err != nil {
		return err
	}

	// A burn-zone part in the Plain list shows the zone with only those parts.
	if p, err = d29Apply(d29Payload(plain(d29Opts{PlainOn: map[string]bool{"burn_30min": true, "turn_ticker": true}}))); err != nil {
		return err
	}
	if got := strings.Join(p.shown(), ","); got != "waits,outcomes,burn,agents_map,sessions,burn_30min,turn_ticker" || !p.BurnZone || p.SystemZone {
		return d29Fail("plain with two burn parts: shown %v, burn zone %v, system zone %v", p.shown(), p.BurnZone, p.SystemZone)
	}

	// B7: everything on in a small window hides nothing. Parts shrink to their floors and the page scrolls.
	if err := ensureWindowSizeWH(hwnd, 1024, 480); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)
	if p, err = d29Apply(d29Payload(plain(d29Opts{PlainAll: true}))); err != nil {
		return err
	}
	if len(p.shown()) < len(d29Parts) {
		return d29Fail("plain at 1024x480 with all parts hid some: shown %v", p.shown())
	}
	if p.BodyOver > 1 || p.OverflowY != "auto" {
		return d29Fail("plain at 1024x480: body overflow %.0f px, overflow-y %s (want the page to scroll)", p.BodyOver, p.OverflowY)
	}
	if _, err := screenshot(hwnd, "d29-plain-small"); err != nil {
		return err
	}
	// Standard at the same size drops nothing either: it scrolls.
	if p, err = d29Apply(d29Payload(std(nil))); err != nil {
		return err
	}
	if strings.Join(p.shown(), ",") != wantAll || !p.Scrolls {
		return d29Fail("standard at 1024x480: shown %v, scrolls %v", p.shown(), p.Scrolls)
	}
	if err := ensureWindowSizeWH(hwnd, 1152, 2048); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)

	// The toggle's text flips with each press.
	if _, err = d29Apply(d29Payload(std(nil))); err != nil {
		return err
	}
	for i, want := range []string{"Plain: on", "Plain: off", "Plain: on"} {
		if err := d29Eval(`document.getElementById('plainToggle').click()`); err != nil {
			return err
		}
		if p, err = d29Read(); err != nil {
			return err
		}
		if p.Toggle != want || p.ToggleOn != (want == "Plain: on") {
			return d29Fail("toggle press %d reads %q (on %v), want %q", i+1, p.Toggle, p.ToggleOn, want)
		}
	}

	// The gear: four groups, Overall scrollbar on top, ticks from the payload, the Station pair.
	if p, err = d29Apply(d29Payload(std(map[string]bool{"git": true}))); err != nil {
		return err
	}
	if err := d29Eval(`document.getElementById('secGear').click()`); err != nil {
		return err
	}
	if p, err = d29Read(); err != nil {
		return err
	}
	wantHeads := "Status,Sessions last 30 min,Processes last 30 min,Microsoft To Do"
	if !p.GearOpen || strings.Join(p.Headings, ",") != wantHeads {
		return d29Fail("gear: open %v, group headings %v", p.GearOpen, p.Headings)
	}
	if p.LabelRows != 15 || p.Checkboxes != 30 {
		return d29Fail("gear: %d part rows and %d checkboxes, want 15 and 30", p.LabelRows, p.Checkboxes)
	}
	if !p.ScrollFirst || !p.ScrollTick {
		return d29Fail("gear: Overall scrollbar first %v, ticked %v", p.ScrollFirst, p.ScrollTick)
	}
	if len(p.Ticks) != 19 {
		return d29Fail("gear: %d ticks, want 19 (14 standard without git, 5 plain)", len(p.Ticks))
	}
	for _, t := range p.Ticks {
		if t == "standard:git" || t == "plain:git" || t == "plain:processes" {
			return d29Fail("gear ticked %s, which is off in the payload", t)
		}
	}
	if p.Radio != "space" {
		return d29Fail("gear: Station starts as %q, want space", p.Radio)
	}
	siteOpts := std(nil)
	siteOpts.StationTheme = "site"
	if p, err = d29Apply(d29Payload(siteOpts)); err != nil {
		return err
	}
	if p.Radio != "site" {
		return d29Fail("gear: payload says site, radio reads %q", p.Radio)
	}
	if _, err := screenshot(hwnd, "d29-gear"); err != nil {
		return err
	}
	// Choosing a Station theme calls the binding and leaves the open Station alone.
	if _, err = d29Apply(d29Payload(std(nil))); err != nil {
		return err
	}
	if err := d29Eval(`(function(){
  window.__d29Real = window.bdevSetStationTheme; window.__d29Calls = []; window.__d29Start = window.BM_STATION_THEME;
  window.bdevSetStationTheme = function(v){ window.__d29Calls.push(v); return Promise.resolve(); };
  document.querySelector('input[name="secStationStart"][value="site"]').click();
  return 1;
})()`); err != nil {
		return err
	}
	calls, cerr := evalString(`JSON.stringify(window.__d29Calls)`)
	same, serr := evalString(`String(window.BM_STATION_THEME === window.__d29Start)`)
	_ = d29Eval(`(window.bdevSetStationTheme = window.__d29Real, 1)`)
	if cerr != nil || serr != nil {
		return d29Fail("radio step could not read back: %v %v", cerr, serr)
	}
	if calls != `["site"]` || same != "true" {
		return d29Fail("radio: binding calls %s, Station theme unchanged %s, want [\"site\"] and true", calls, same)
	}
	if err := d29Eval(`document.getElementById('secGear').click()`); err != nil {
		return err
	}

	// Stale and missing: the header and the notice carry Go's line, never
	// "not running", and no old numbers are drawn.
	lines := map[string]string{"stale": "No process has written the snapshot for 7 s.", "missing": "No snapshot file yet."}
	for _, status := range []string{"stale", "missing"} {
		p, err = d29Apply(d29Payload(d29Opts{Plain: true, Status: status}))
		if err != nil {
			return err
		}
		want := lines[status]
		if p.Header != want || !strings.Contains(p.Notice, want) {
			return d29Fail("%s: header %q notice %q do not carry the line %q", status, p.Header, p.Notice, want)
		}
		if strings.Contains(p.Header, "not running") || strings.Contains(p.Notice, "not running") {
			return d29Fail("%s: header %q notice %q still say not running", status, p.Header, p.Notice)
		}
		if strings.Contains(p.Text, "tokens") && strings.Contains(p.Text, "120") {
			return d29Fail("%s: old numbers are still drawn", status)
		}
	}
	if _, err := screenshot(hwnd, "d29-not-running"); err != nil {
		return err
	}

	return d29Persist()
}

type d29Saved struct {
	Plain     bool            `json:"plain_mode"`
	Scrollbar bool            `json:"overall_scrollbar"`
	Standard  map[string]bool `json:"parts_standard"`
	PlainSet  map[string]bool `json:"parts_plain"`
	Parts     map[string]bool `json:"parts"`
}

// d29Persist drives the real Go side: the toggle, one part in each mode and the
// scrollbar through the bindings, and one tick through the gear's own checkbox.
// Each must land where it belongs only. The next case's applyStart resets them.
func d29Persist() error {
	read := func() (d29Saved, error) {
		var st d29Saved
		if _, err := evalRaw(`(window.__d29 = null, window.bdevSections().then(function(p){ window.__d29 = JSON.stringify(p); }), 1)`); err != nil {
			return st, err
		}
		for i := 0; i < 20; i++ {
			time.Sleep(150 * time.Millisecond)
			s, err := evalString(`window.__d29 || ''`)
			if err != nil {
				return st, err
			}
			if s != "" {
				return st, json.Unmarshal([]byte(s), &st)
			}
		}
		return st, fmt.Errorf("bdevSections never answered")
	}
	send := func(change string) error {
		_, err := evalRaw(`(window.bdevSetSections(` + change + `), 1)`)
		time.Sleep(400 * time.Millisecond)
		return err
	}
	// Start from Standard: git on in Standard and off in Plain, turn_ticker on in Standard and off in Plain.
	for _, c := range []string{
		`{plain:false}`,
		`{mode:'standard',section:'git',on:true}`, `{mode:'plain',section:'git',on:false}`,
		`{mode:'standard',section:'turn_ticker',on:true}`, `{mode:'plain',section:'turn_ticker',on:false}`,
		`{mode:'standard',section:'burn',on:true}`, `{mode:'plain',section:'burn',on:true}`,
		`{scrollbar:true}`,
	} {
		if err := send(c); err != nil {
			return err
		}
	}
	// The toggle.
	if err := send(`{plain:true}`); err != nil {
		return err
	}
	st, err := read()
	if err != nil {
		return err
	}
	if !st.Plain || st.Parts["git"] || !st.Standard["git"] {
		return d29Fail("toggle saved wrong: %+v", st)
	}
	// One part in Plain only.
	if err := send(`{mode:'plain',section:'turn_ticker',on:true}`); err != nil {
		return err
	}
	if st, err = read(); err != nil {
		return err
	}
	if !st.PlainSet["turn_ticker"] || !st.Standard["turn_ticker"] || !st.Parts["turn_ticker"] {
		return d29Fail("plain turn_ticker switch saved wrong: %+v", st)
	}
	// One part in Standard only, with the page in Plain: the current list does not move.
	if err := send(`{mode:'standard',section:'burn',on:false}`); err != nil {
		return err
	}
	if st, err = read(); err != nil {
		return err
	}
	if st.Standard["burn"] || !st.PlainSet["burn"] || !st.Parts["burn"] {
		return d29Fail("standard burn switch saved wrong: %+v", st)
	}
	// The scrollbar switch saves alone.
	before := st
	if err := send(`{scrollbar:false}`); err != nil {
		return err
	}
	if st, err = read(); err != nil {
		return err
	}
	if st.Scrollbar || !st.Plain || !reflect.DeepEqual(st.Standard, before.Standard) || !reflect.DeepEqual(st.PlainSet, before.PlainSet) {
		return d29Fail("scrollbar switch saved wrong: %+v", st)
	}
	// The gear's own checkbox saves through the same path, to its own mode.
	if err := d29Eval(`(function(){ var i = document.querySelector('#secSettingsCols input[data-mode="plain"][data-sw="sessions"]'); i.checked = false; i.dispatchEvent(new Event('change', {bubbles: true})); })()`); err != nil {
		return err
	}
	time.Sleep(400 * time.Millisecond)
	if st, err = read(); err != nil {
		return err
	}
	if st.PlainSet["sessions"] || !st.Standard["sessions"] {
		return d29Fail("gear tick saved wrong: %+v", st)
	}
	return nil
}

// layout/d33: moved from check_d33.go.

// d33: the row rules (B2). Driven by window.__bdevSections.apply with the
// frozen example, Standard, Overall scrollbar on. It proves:
//   - the first status row (#secRow1) is one row: the shown panels keep the
//     order waits, outcomes, burn, git, agents_map, share one top and one
//     width, and fill the row from edge to edge, with five, four or two on;
//   - the Activity and Heatmap row: both shown share one top and fill the
//     row; one shown takes the whole row;
//   - with the System and Processes parts off, the turn ticker panel is as
//     tall as its rows, or the room left in the burn zone when that is less.
//
// Screenshots: d33-row1-<width>-5, d33-row1-<width>-4, d33-row1-<width>-2,
// d33-ticker-noproc-<width>.

type d33Rect struct {
	ID string  `json:"id"`
	L  float64 `json:"left"`
	R  float64 `json:"right"`
	T  float64 `json:"top"`
	W  float64 `json:"width"`
}

type d33Tick struct {
	Top    float64 `json:"top"`
	Height float64 `json:"height"`
	Head   float64 `json:"head"`
	Scroll float64 `json:"scroll"`
	Bottom float64 `json:"zoneBottom"`
	Shown  bool    `json:"shown"`
}

type d33Page struct {
	RowL   float64   `json:"rowL"`
	RowR   float64   `json:"rowR"`
	Panels []d33Rect `json:"panels"`
	Split  []d33Rect `json:"split"`
	SplitL float64   `json:"splitL"`
	SplitR float64   `json:"splitR"`
	Gap    float64   `json:"gap"`
	Tick   d33Tick   `json:"tick"`
}

const d33ReadJS = `(function(){
  function vis(e){ return !!e && e.getClientRects().length > 0 && getComputedStyle(e).display !== 'none'; }
  function rect(e, id){ var r = e.getBoundingClientRect(); return {id: id, left: r.left, right: r.right, top: r.top, width: r.width}; }
  var row = document.getElementById('secRow1'), rr = row ? row.getBoundingClientRect() : {left: 0, right: 0};
  var panels = [];
  if(row) Array.prototype.forEach.call(row.querySelectorAll('.secpanel'), function(p){ if(vis(p)) panels.push(rect(p, p.getAttribute('data-sec'))); });
  var sp = document.getElementById('activityHeatmapRow'), split = [], sl = 0, sr = 0, gap = 0;
  if(sp && vis(sp)){
    var r2 = sp.getBoundingClientRect(); sl = r2.left; sr = r2.right;
    gap = parseFloat(getComputedStyle(sp).columnGap) || 0;
    ['activityPanel', 'harnessHeatmapPanel'].forEach(function(id){ var e = document.getElementById(id); if(vis(e)) split.push(rect(e, id)); });
  }
  var tp = document.getElementById('turnTickerPanel'), tk = document.getElementById('turnTicker');
  var zb = document.querySelector('.zone.burn .zonebody');
  var tick = {top: 0, height: 0, head: 0, scroll: 0, zoneBottom: 0, shown: false};
  if(vis(tp)){
    var tr = tp.getBoundingClientRect(), hd = tp.querySelector('.panelhead');
    tick = {top: tr.top, height: tr.height, head: hd ? hd.getBoundingClientRect().height : 0, scroll: tk.scrollHeight,
      zoneBottom: zb ? zb.getBoundingClientRect().bottom : 0, shown: true};
  }
  return {rowL: rr.left, rowR: rr.right, panels: panels, split: split, splitL: sl, splitR: sr, gap: gap, tick: tick};
})()`

func d33Fail(format string, args ...any) error { return fmt.Errorf("d33: "+format, args...) }

func d33Apply(payload string) (d33Page, error) {
	var p d33Page
	if _, err := evalRaw(`window.__bdevSections.apply(` + payload + `)`); err != nil {
		return p, err
	}
	time.Sleep(600 * time.Millisecond)
	err := evalInto(d33ReadJS, &p)
	return p, err
}

func caseD33(hwnd uintptr, args []string) error {
	snap, err := readFixture("snapshot_example.json")
	if err != nil {
		return fmt.Errorf("read the frozen example (uicheck runs from the tools folder): %w", err)
	}
	std := func(off ...string) string {
		m := map[string]bool{}
		for _, id := range off {
			m[id] = true
		}
		return d29Payload(d29Opts{Scrollbar: true, StandardOff: m, Status: "ok", Snap: snap})
	}
	order := []string{"waits", "outcomes", "burn", "git", "agents_map"}

	// First row: the shown panels fill one row, in order, equal widths.
	row1 := func(w, h int32, tag string, off ...string) error {
		p, err := d33Apply(std(off...))
		if err != nil {
			return err
		}
		skip := map[string]bool{}
		for _, o := range off {
			skip[o] = true
		}
		var want, got []string
		for _, id := range order {
			if !skip[id] {
				want = append(want, id)
			}
		}
		for _, r := range p.Panels {
			got = append(got, r.ID)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			return d33Fail("%dx%d first row shows %v, want %v", w, h, got, want)
		}
		for i, r := range p.Panels {
			if math.Abs(r.T-p.Panels[0].T) > 1 {
				return d33Fail("%dx%d first row wraps: %s top %.0f, %s top %.0f", w, h, r.ID, r.T, p.Panels[0].ID, p.Panels[0].T)
			}
			if math.Abs(r.W-p.Panels[0].W) > 2 {
				return d33Fail("%dx%d first row widths differ: %s %.0f, %s %.0f", w, h, r.ID, r.W, p.Panels[0].ID, p.Panels[0].W)
			}
			if i > 0 && r.L < p.Panels[i-1].L {
				return d33Fail("%dx%d first row order wrong at %s", w, h, r.ID)
			}
		}
		first, last := p.Panels[0], p.Panels[len(p.Panels)-1]
		if math.Abs(first.L-p.RowL) > 2 || math.Abs(last.R-p.RowR) > 2 {
			return d33Fail("%dx%d first row does not fill the row: %.0f..%.0f of %.0f..%.0f", w, h, first.L, last.R, p.RowL, p.RowR)
		}
		if _, err := screenshot(hwnd, tag); err != nil {
			return err
		}
		return nil
	}
	for _, size := range [][2]int32{{1280, 860}, {1920, 1080}} {
		w, h := size[0], size[1]
		if err := ensureWindowSizeWH(hwnd, w, h); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		if err := row1(w, h, fmt.Sprintf("d33-row1-%d-5", w)); err != nil {
			return err
		}
		if err := row1(w, h, fmt.Sprintf("d33-row1-%d-4", w), "git"); err != nil {
			return err
		}
		if err := row1(w, h, fmt.Sprintf("d33-row1-%d-2", w), "waits", "burn", "agents_map"); err != nil {
			return err
		}
	}

	// Activity and Heatmap: both fill the row, one takes it all.
	for _, size := range [][2]int32{{1920, 1080}, {1152, 2048}} {
		w, h := size[0], size[1]
		if err := ensureWindowSizeWH(hwnd, w, h); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		for _, c := range []struct {
			off  []string
			want int
		}{{nil, 2}, {[]string{"activity"}, 1}, {[]string{"heatmap"}, 1}} {
			p, err := d33Apply(std(c.off...))
			if err != nil {
				return err
			}
			if len(p.Split) != c.want {
				return d33Fail("Activity and Heatmap row dropped at %dx%d, cannot measure (off %v, shown %d)", w, h, c.off, len(p.Split))
			}
			width := p.SplitR - p.SplitL
			if c.want == 2 {
				a, b := p.Split[0], p.Split[1]
				if math.Abs(a.T-b.T) > 1 {
					return d33Fail("%dx%d Activity and Heatmap do not share a top: %.0f, %.0f", w, h, a.T, b.T)
				}
				if math.Abs(a.W+p.Gap+b.W-width) > 2 {
					return d33Fail("%dx%d Activity %.0f + gap %.0f + Heatmap %.0f != row %.0f", w, h, a.W, p.Gap, b.W, width)
				}
			} else if math.Abs(p.Split[0].W-width) > 2 {
				return d33Fail("%dx%d lone %s is %.0f wide, row is %.0f (off %v)", w, h, p.Split[0].ID, p.Split[0].W, width, c.off)
			}
		}
	}

	// Turn ticker: as tall as its rows, at most the room left in the burn zone, but never below the 80 px min-height floor.
	for _, size := range [][2]int32{{1920, 1080}, {1152, 2048}} {
		w, h := size[0], size[1]
		if err := ensureWindowSizeWH(hwnd, w, h); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		p, err := d33Apply(std("system", "process_groups", "processes"))
		if err != nil {
			return err
		}
		if !p.Tick.Shown {
			return d33Fail("%dx%d turn ticker not shown, cannot measure", w, h)
		}
		need := p.Tick.Head + p.Tick.Scroll
		room := p.Tick.Bottom - p.Tick.Top
		if p.Tick.Height > math.Max(80, math.Min(need, room))+6 {
			return d33Fail("%dx%d turn ticker is %.0f px tall, rows need %.0f, room %.0f, 80 px min-height floor", w, h, p.Tick.Height, need, room)
		}
		if _, err := screenshot(hwnd, fmt.Sprintf("d33-ticker-noproc-%d", w)); err != nil {
			return err
		}
	}
	return nil
}

// layout/d34: moved from check_d34.go.

// d34: the layout fill (B7, spec 2026-10-27, items 1, 2, 3, 6). The B2 check
// that asserted the old hide order is replaced; the id stays. Driven by
// window.__bdevSections.apply with the frozen example. For each window size in
// 1280x800, 1920x1080 and 2560x1440, in Standard and in Plain, with the Overall
// scrollbar on and off, these cases run:
//   a  every part on
//   b  turn ticker off
//   c  Plain with only Burn last 30 min on (Plain only)
//   d  every part on at 1280x800 with the scrollbar off
//   e  every part on at 1280x800, scrollbar off, with long lists (12 wait items,
//      8 outcome items, 8 sessions): the page scrolls only when every list sits
//      at three rows
// Each case asserts:
//   - no vertical empty area over 24 px inside a column (the largest gap
//     between a column's children, below the last child, or inside a panel
//     below its last child);
//   - no part hidden: every [data-part] without .sec-off draws, and the old
//     "sections hidden" note is gone;
//   - To Do is drawn in every case that has it on (the page is put in the check
//     fake mode with a snapshot that switches To Do on, the way d19 does, so the
//     real To Do never shows rows) and counts in "no part hidden" and in the
//     24 px measure (gaps between the rows below the columns, and slack inside
//     the Processes and To Do panels);
//   - the Processes table sits above To Do (it comes first in the document and
//     its bottom is at or above To Do's top, compared as rectangles), and below
//     Process groups, Memory, Disks and Network;
//   - scrollbar off: the page scrolls only when window.__bdevFit.fitScroll says
//     every part is at its floor; otherwise nothing overflows, and in two
//     columns both columns end on the same line (within 2 px).
// The drag handles (.hdl, B7.1) are 4 px bars laid over the row gap: the gap
// measures ignore them, and each handle is checked for its own rectangle (4 px
// tall, inside its zonebody, not over a part). A handle with no room (iteration 2:
// even with the host lists at their floor) is hidden, so at fit-scroll, where every
// part and list sits at its floor, no handle may be drawn: a drawn dead bar fails.
// A probe proves the measure can fail: a 100 px gap is pushed into a column
// and the same read must report it (d34Probe), once per run.
// A window size the screen cannot hold prints SKIPPED and is not a pass.
// Screenshots: d34-<size>-<mode>-<case>-<sb>.

const d34MaxEmpty = 24.0

type d34Page struct {
	OverflowY  string    `json:"overflowY"`
	Scrolls    bool      `json:"scrolls"`
	BodyOver   float64   `json:"bodyOver"`
	PageOver   float64   `json:"pageOver"`
	HostOver   float64   `json:"hostOver"`
	ZoneOver   float64   `json:"zoneOver"`
	BodyScroll bool      `json:"bodyScroll"`
	FitScroll  bool      `json:"fitScroll"`
	FitClass   bool      `json:"fitClass"`
	Fit        bool      `json:"fit"`
	FitHidden  []string  `json:"fitHidden"`
	Hidden     []string  `json:"hidden"`
	CutNote    bool      `json:"cutNote"`
	Empty      float64   `json:"empty"`
	EmptyWhere string    `json:"emptyWhere"`
	ProcOn     bool      `json:"procOn"`
	ProcFirst  bool      `json:"procFirst"`
	ProcAbove  bool      `json:"procAbove"`
	ProcBelow  bool      `json:"procBelow"`
	TwoCols    bool      `json:"twoCols"`
	ColDiff    float64   `json:"colDiff"`
	Parts      int       `json:"parts"`
	TodoDrawn  bool      `json:"todoDrawn"`
	AboveSeen  bool      `json:"aboveSeen"`
	Handles    int       `json:"handles"`
	HandleBad  string    `json:"handleBad"`
	Lists      []d34List `json:"lists"`
	Items      []d34Item `json:"items"`
}

type d34List struct {
	Sec  string  `json:"sec"`
	H    float64 `json:"h"`
	SH   float64 `json:"sh"`
	Rows int     `json:"rows"`
	Seen int     `json:"seen"`
}

type d34Item struct {
	ID    string  `json:"id"`
	Kind  string  `json:"kind"`
	H     float64 `json:"h"`
	Floor float64 `json:"floor"`
	Nat   float64 `json:"nat"`
}

const d34ReadJS = `(function(){
  var q = function(s){ return document.querySelector(s); };
  var shown = function(e){ return !!e && e.getClientRects().length > 0 && getComputedStyle(e).display !== 'none'; };
  var ps = document.getElementById('pageScroll'), host = document.getElementById('secHost');
  var empty = 0, where = '', zone = 0;
  var note = function(v, w){ if(v > empty){ empty = v; where = w; } };
  var slack = function(k, name){
    var r = k.getBoundingClientRect();
    var inner = Array.prototype.filter.call(k.children, function(c){ return !c.classList.contains('hdl') && shown(c) && c.getBoundingClientRect().height > 0.5; });
    if(!inner.length) return;
    var ib = 0;
    inner.forEach(function(c){ ib = Math.max(ib, c.getBoundingClientRect().bottom); });
    note(r.bottom - parseFloat(getComputedStyle(k).paddingBottom) - ib, name + ' slack inside ' + (k.id || k.className));
  };
  var zones = Array.prototype.filter.call(document.querySelectorAll('.zone'), shown);
  zones.forEach(function(z){
    var zb = z.querySelector('.zonebody');
    if(!zb || !shown(zb)) return;
    zone = Math.max(zone, zb.scrollHeight - zb.clientHeight);
    var zr = zb.getBoundingClientRect(), cs = getComputedStyle(zb);
    var top = zr.top + parseFloat(cs.paddingTop), bot = zr.bottom - parseFloat(cs.paddingBottom);
    var kids = Array.prototype.filter.call(zb.children, function(c){ return !c.classList.contains('hdl') && shown(c) && c.getBoundingClientRect().height > 0.5; })
      .sort(function(a, b){ return a.getBoundingClientRect().top - b.getBoundingClientRect().top; });
    var name = z.className.replace('zone ', '');
    if(!kids.length) return;
    var prev = top;
    kids.forEach(function(k, i){
      var r = k.getBoundingClientRect();
      note(r.top - prev, name + ' gap before child ' + i + ' (' + (k.id || k.className) + ')');
      prev = Math.max(prev, r.bottom);
      if(k.classList.contains('panel')) slack(k, name);
    });
    note(bot - prev, name + ' gap below last child');
  });
  // The rows below the columns: Processes, then To Do.
  var rows = [q('#processesHost [data-part="processes"]'), q('#todoPanel')].filter(function(e){ return shown(e) && !e.classList.contains('sec-off'); });
  var prevBottom = null, mainEl = q('main');
  if(shown(mainEl)) prevBottom = mainEl.getBoundingClientRect().bottom;
  rows.forEach(function(k){
    var r = k.getBoundingClientRect();
    if(prevBottom !== null) note(r.top - prevBottom, 'gap above ' + (k.id || k.className));
    prevBottom = r.bottom;
    slack(k, 'row');
  });
  var hidden = [], parts = {};
  Array.prototype.forEach.call(document.querySelectorAll('[data-part]'), function(e){
    if(e.classList.contains('sec-off')) return;
    var id = e.getAttribute('data-part');
    if(shown(e)) parts[id] = true;
    else if(hidden.indexOf(id) < 0) hidden.push(id);
  });
  var proc = q('#processesHost [data-part="processes"]'), todo = q('#todoPanel');
  var procOn = shown(proc), procFirst = true, above = true, below = true;
  var host2 = q('#processesHost');
  if(host2 && todo) procFirst = !!(host2.compareDocumentPosition(todo) & Node.DOCUMENT_POSITION_FOLLOWING);
  var seen = false;
  if(procOn){
    var pr = proc.getBoundingClientRect();
    if(shown(todo) && !todo.classList.contains('sec-off')){ above = pr.bottom <= todo.getBoundingClientRect().top + 1; seen = true; }
    ['#processGroupsPanel', '#boxMemory', '#boxDisks', '#boxNetwork'].forEach(function(s){
      var e = q(s);
      if(shown(e) && e.getBoundingClientRect().bottom > pr.top + 1) below = false;
    });
  }
  var two = zones.length === 2 && Math.abs(zones[0].getBoundingClientRect().top - zones[1].getBoundingClientRect().top) < 2;
  var colDiff = two ? Math.abs(zones[0].getBoundingClientRect().bottom - zones[1].getBoundingClientRect().bottom) : 0;
  var handles = 0, handleBad = '';
  Array.prototype.forEach.call(document.querySelectorAll('.zonebody > .hdl'), function(hd){
    if(!shown(hd)) return;
    handles++;
    var hr = hd.getBoundingClientRect(), br = hd.parentNode.getBoundingClientRect();
    if(Math.abs(hr.height - 4) > 0.5) handleBad += 'handle ' + handles + ' is ' + hr.height.toFixed(1) + ' px tall, want 4; ';
    if(hr.top < br.top - 0.5 || hr.bottom > br.bottom + 0.5) handleBad += 'handle ' + handles + ' lies outside its zonebody; ';
    Array.prototype.forEach.call(hd.parentNode.children, function(c){
      if(c === hd || c.classList.contains('hdl') || !shown(c)) return;
      var cr = c.getBoundingClientRect();
      if(cr.height < 0.5) return;
      var ov = Math.min(hr.bottom, cr.bottom) - Math.max(hr.top, cr.top);
      if(ov > 2) handleBad += 'handle ' + handles + ' overlaps ' + (c.id || c.className) + ' by ' + ov.toFixed(1) + ' px; ';
    });
  });
  var lists = [];
  Array.prototype.forEach.call(document.querySelectorAll('#secHost .seclist'), function(el){
    if(!shown(el) || el.closest('.sec-off')) return;
    var rs = Array.prototype.slice.call(el.querySelector('table') ? el.querySelectorAll('tbody tr:not(.ctlrow)') : el.querySelectorAll('.secitem'));
    var lr = el.getBoundingClientRect(), seen = 0;
    rs.forEach(function(r){ var rr = r.getBoundingClientRect(); if(rr.top >= lr.top - 1 && rr.bottom <= lr.bottom + 1) seen++; });
    lists.push({ sec: (el.closest('[data-sec]') || el).getAttribute('data-sec') || '', h: el.clientHeight, sh: el.scrollHeight, rows: rs.length, seen: seen });
  });
  var f = window.__bdevFit, has = !!f;
  f = f || {};
  return {
    overflowY: getComputedStyle(ps).overflowY,
    scrolls: ps.scrollHeight > ps.clientHeight + 1,
    bodyOver: document.body.scrollHeight - document.body.clientHeight,
    pageOver: ps.scrollHeight - ps.clientHeight,
    hostOver: host.scrollHeight - host.clientHeight,
    zoneOver: zone,
    bodyScroll: document.body.classList.contains('scroll-on'),
    fitScroll: !!f.fitScroll, fitClass: document.body.classList.contains('fit-scroll'),
    fit: has, fitHidden: f.hidden || [], hidden: hidden,
    cutNote: !!document.getElementById('secCutNote'),
    empty: Math.max(0, empty), emptyWhere: where,
    procOn: procOn, procFirst: procFirst, procAbove: above, procBelow: below,
    twoCols: two, colDiff: colDiff, parts: Object.keys(parts).length, todoDrawn: shown(todo), aboveSeen: seen,
    handles: handles, handleBad: handleBad, lists: lists, items: f.items || []
  };
})()`

func d34Apply(payload string, snap []byte) (d34Page, error) {
	var p d34Page
	if _, err := evalRaw(`window.__bdevSections.apply(` + payload + `)`); err != nil {
		return p, err
	}
	// Fake mode, then one paint of the snapshot that switches To Do on: the
	// real tick is stopped, so nothing paints over it (the d19 way).
	if _, err := evalRaw(`(function(s){ window.__bdevEnterFakeMode(); window.__bdevPaintFake(s); return true; })(` + string(snap) + `)`); err != nil {
		return p, err
	}
	time.Sleep(800 * time.Millisecond)
	err := evalInto(d34ReadJS, &p)
	return p, err
}

// d34Verify returns the problems found in one read, empty when the case passes.
func d34Verify(p d34Page, scrollbar, wantTodo, caseE bool) []string {
	var bad []string
	if !p.Fit {
		return []string{"window.__bdevFit is missing"}
	}
	if wantTodo && !p.TodoDrawn {
		bad = append(bad, "To Do is on but not drawn")
	}
	if wantTodo && p.ProcOn && !p.AboveSeen {
		bad = append(bad, "Processes and To Do are both on but the rectangles were not compared")
	}
	if p.Empty > d34MaxEmpty+0.5 {
		bad = append(bad, fmt.Sprintf("empty area %.0f px (%s), limit %.0f", p.Empty, p.EmptyWhere, d34MaxEmpty))
	}
	if len(p.Hidden) != 0 || len(p.FitHidden) != 0 {
		bad = append(bad, fmt.Sprintf("hidden parts %v, fit hidden %v", p.Hidden, p.FitHidden))
	}
	if p.CutNote {
		bad = append(bad, "#secCutNote still exists")
	}
	if !p.ProcFirst || !p.ProcAbove || !p.ProcBelow {
		bad = append(bad, fmt.Sprintf("Processes order: before To Do %v, above To Do %v, below groups/Memory/Disks/Network %v", p.ProcFirst, p.ProcAbove, p.ProcBelow))
	}
	if p.BodyOver > 1 {
		bad = append(bad, fmt.Sprintf("the body itself overflows by %.0f px", p.BodyOver))
	}
	if p.HandleBad != "" {
		bad = append(bad, "drag handles: "+p.HandleBad)
	}
	if p.FitScroll && p.Handles != 0 {
		bad = append(bad, fmt.Sprintf("%d drag handle(s) are drawn although every part sits at its floor (a handle without room must be hidden)", p.Handles))
	}
	if caseE {
		long := 0
		for _, l := range p.Lists {
			if l.Rows > 3 {
				long++
			}
		}
		if len(p.Lists) < 3 || long != len(p.Lists) {
			bad = append(bad, fmt.Sprintf("case e needs at least three long lists (more than three rows each), got %d lists, %d long (%s)", len(p.Lists), long, strings.TrimSpace(d34ListNote(p, true)[7:])))
		}
	}
	if scrollbar {
		return bad
	}
	if p.FitScroll != p.FitClass {
		bad = append(bad, fmt.Sprintf("fitScroll %v but body.fit-scroll %v", p.FitScroll, p.FitClass))
	}
	if !p.FitScroll {
		if p.PageOver > 1 || p.HostOver > 1 || p.ZoneOver > 1 || p.Scrolls {
			bad = append(bad, fmt.Sprintf("not at floors yet overflows: page %.0f, host %.0f, zone %.0f, scrolls %v", p.PageOver, p.HostOver, p.ZoneOver, p.Scrolls))
		}
		if p.TwoCols && p.ColDiff > 2 {
			bad = append(bad, fmt.Sprintf("two columns end %.0f px apart, want at most 2", p.ColDiff))
		}
		for _, l := range p.Lists {
			want := l.Rows
			if want > 3 {
				want = 3
			}
			if l.Seen < want {
				bad = append(bad, fmt.Sprintf("list %s shows %d of its first %d rows without the page scrolling", l.Sec, l.Seen, want))
			}
		}
	} else {
		for _, it := range p.Items {
			if it.H > it.Floor+1 {
				bad = append(bad, fmt.Sprintf("page scrolls but %s %s is at %.0f px, above its floor %.0f", it.Kind, it.ID, it.H, it.Floor))
			}
		}
		for _, l := range p.Lists {
			if l.Rows <= 3 {
				continue
			}
			fl := -1.0
			for _, it := range p.Items {
				if it.Kind == "seclist" && it.ID == l.Sec {
					fl = it.Floor
				}
			}
			switch {
			case fl < 0:
				bad = append(bad, fmt.Sprintf("list %s has no fit item", l.Sec))
			case l.H > fl+2:
				bad = append(bad, fmt.Sprintf("page scrolls but list %s is %.0f px, floor %.0f", l.Sec, l.H, fl))
			case l.SH <= l.H:
				bad = append(bad, fmt.Sprintf("page scrolls and list %s does not scroll inside itself (%.0f of %.0f px)", l.Sec, l.H, l.SH))
			case l.Seen < 3:
				bad = append(bad, fmt.Sprintf("list %s shows %d rows at its floor, want 3", l.Sec, l.Seen))
			}
		}
	}
	return bad
}

// d34Probe proves the empty-area measure can fail. It moves the last child of
// the first visible column 100 px down with a transform (a layout change would
// let the flex column squeeze the gap away), reads the page with the same
// script and puts the transform back. The read must report at least 99 px and
// d34Verify must turn that into a failure. Returns an error when it does not.
func d34Probe() error {
	if _, err := evalRaw(`(function(){ var zb = Array.prototype.filter.call(document.querySelectorAll('.zone .zonebody'), function(e){ return e.getClientRects().length > 0; })[0];
		if(!zb) return false;
		var kids = Array.prototype.filter.call(zb.children, function(c){ return !c.classList.contains('hdl') && c.getBoundingClientRect().height > 0.5; });
		var k = kids[kids.length - 1]; if(!k) return false;
		k.setAttribute('data-d34probe', k.style.transform || '-'); k.style.transform = 'translateY(100px)'; return true; })()`); err != nil {
		return err
	}
	var p d34Page
	err := evalInto(d34ReadJS, &p)
	_, _ = evalRaw(`(function(){ var k = document.querySelector('[data-d34probe]'); if(!k) return false;
		var o = k.getAttribute('data-d34probe'); k.style.transform = o === '-' ? '' : o; k.removeAttribute('data-d34probe'); return true; })()`)
	if err != nil {
		return err
	}
	if p.Empty < 99 {
		return fmt.Errorf("probe: a 100 px gap measured as %.0f px (%s), the measure cannot fail", p.Empty, p.EmptyWhere)
	}
	failed := false
	for _, b := range d34Verify(p, true, true, false) {
		if strings.HasPrefix(b, "empty area") {
			failed = true
		}
	}
	if !failed {
		return fmt.Errorf("probe: a %.0f px gap did not fail d34Verify", p.Empty)
	}
	fmt.Printf("uicheck: d34: probe OK, a deliberate 100 px gap measured %.0f px (%s) and failed the check\n", p.Empty, p.EmptyWhere)
	return nil
}

// d34Snapshot returns the frozen example with To Do switched on and 14 tasks.
// With extended true it also grows the lists to 12 wait items, 8 outcome items
// and 8 sessions (clones of the first entry, invented names only).
func d34Snapshot(extended bool) ([]byte, error) {
	snap, err := readFixture("snapshot_example.json")
	if err != nil {
		return nil, fmt.Errorf("read the frozen example (uicheck runs from the tools folder): %w", err)
	}
	var snapMap map[string]any
	if err := json.Unmarshal(snap, &snapMap); err != nil {
		return nil, err
	}
	snapMap["todo"] = map[string]any{"enabled": true, "signed_in": true}
	var items []map[string]any
	for k := 1; k <= 14; k++ {
		items = append(items, map[string]any{"Title": fmt.Sprintf("Example task %d", k), "List": "Sample list", "DueDate": "2026-09-29", "Overdue": k%5 == 0})
	}
	snapMap["todo_tasks"] = map[string]any{"items": items}
	if extended {
		grow := func(key string, n int, idf, titlef, drop string) {
			list, _ := snapMap[key].([]any)
			if len(list) == 0 {
				return
			}
			first, _ := list[0].(map[string]any)
			for len(list) < n {
				c := map[string]any{}
				for k, v := range first {
					c[k] = v
				}
				nn := fmt.Sprintf("%02d", len(list)+1)
				c["id"] = fmt.Sprintf(idf, nn)
				if _, ok := c["title"]; ok {
					c["title"] = fmt.Sprintf(titlef, nn)
				}
				if drop != "" {
					delete(c, drop)
				}
				list = append(list, c)
			}
			snapMap[key] = list
		}
		grow("wait_items", 12, "gate:example__flow__%s:G4", "flow-example-%s", "")
		grow("outcome_items", 8, "example-outcome-%s", "example-outcome-%s", "")
		grow("sessions", 8, "example-session-%s", "example-session-%s", "parent_id")
	}
	return json.Marshal(snapMap)
}
func caseD34(hwnd uintptr, args []string) error {
	snap, err := d34Snapshot(false)
	if err != nil {
		return err
	}
	snapE, err := d34Snapshot(true)
	if err != nil {
		return err
	}
	allBut := func(off string) map[string]bool {
		m := map[string]bool{}
		for _, id := range d29Parts {
			if id != off {
				m[id] = true
			}
		}
		return m
	}
	payload := func(plain bool, c string, scrollbar bool, snap []byte) string {
		o := d29Opts{Plain: plain, Scrollbar: scrollbar, Status: "ok", Snap: snap}
		switch c {
		case "a", "d", "e":
			o.PlainAll = true
		case "b":
			o.StandardOff = map[string]bool{"turn_ticker": true}
			o.PlainOnly = allBut("turn_ticker")
		case "c":
			o.PlainOnly = map[string]bool{"burn_30min": true}
		}
		return d29Payload(o)
	}
	wantTodo := func(plain bool, c string) bool { return !(plain && c == "c") }
	probed := false
	sizes := [][2]int32{{1280, 800}, {1920, 1080}, {2560, 1440}}
	var fails []string
	ran, skipped := 0, 0
	for _, sz := range sizes {
		w, h := sz[0], sz[1]
		tag := fmt.Sprintf("%dx%d", w, h)
		if err := ensureWindowSizeWH(hwnd, w, h); err != nil {
			return err
		}
		if windowCapped {
			fmt.Printf("uicheck: d34: SKIPPED %s, this screen is too small\n", tag)
			skipped++
			continue
		}
		time.Sleep(500 * time.Millisecond)
		for _, plain := range []bool{false, true} {
			mode := map[bool]string{false: "standard", true: "plain"}[plain]
			for _, c := range []string{"a", "b", "c", "d", "e"} {
				if c == "c" && !plain {
					continue
				}
				if (c == "d" || c == "e") && w != 1280 {
					continue
				}
				for _, sb := range []bool{true, false} {
					if (c == "d" || c == "e") && sb {
						continue
					}
					sbName := map[bool]string{true: "on", false: "off"}[sb]
					useSnap := snap
					if c == "e" {
						useSnap = snapE
					}
					p, err := d34Apply(payload(plain, c, sb, useSnap), useSnap)
					if err != nil {
						return err
					}
					ran++
					name := fmt.Sprintf("d34-%s-%s-%s-%s", tag, mode, c, sbName)
					if _, err := screenshot(hwnd, name); err != nil {
						return err
					}
					if !probed {
						probed = true
						if err := d34Probe(); err != nil {
							return err
						}
					}
					bad := d34Verify(p, sb, wantTodo(plain, c), c == "e")
					if len(bad) == 0 {
						fmt.Printf("uicheck: d34: PASS %s %s scrollbar %s case %s (parts %d, largest empty %.0f px, fitScroll %v, two columns %v, To Do drawn %v, handles %d%s)\n", tag, mode, sbName, c, p.Parts, p.Empty, p.FitScroll, p.TwoCols, p.TodoDrawn, p.Handles, d34ListNote(p, c == "e"))
					} else {
						msg := fmt.Sprintf("%s %s scrollbar %s case %s: %s", tag, mode, sbName, c, strings.Join(bad, "; "))
						fmt.Printf("uicheck: d34: FAIL %s\n", msg)
						fails = append(fails, msg)
					}
				}
			}
		}
	}
	if err := ensureWindowSizeWH(hwnd, 1152, 2048); err != nil {
		return err
	}
	fmt.Printf("uicheck: d34: %d cases ran, %d sizes skipped, %d failed\n", ran, skipped, len(fails))
	if len(fails) > 0 {
		return fmt.Errorf("d34: %d case(s) failed, first: %s", len(fails), fails[0])
	}
	if ran == 0 {
		return fmt.Errorf("d34: no size ran")
	}
	return nil
}

// d34ListNote formats the host lists of case e as ", lists <sec> h/floor/nat rows N".
func d34ListNote(p d34Page, on bool) string {
	if !on {
		return ""
	}
	var b strings.Builder
	b.WriteString(", lists")
	for _, l := range p.Lists {
		fl, nat := 0.0, 0.0
		for _, it := range p.Items {
			if it.Kind == "seclist" && it.ID == l.Sec {
				fl, nat = it.Floor, it.Nat
			}
		}
		fmt.Fprintf(&b, " %s %.0f/%.0f/%.0f rows %d", l.Sec, l.H, fl, nat, l.Rows)
	}
	return b.String()
}

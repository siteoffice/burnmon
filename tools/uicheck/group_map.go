package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// map/d30: moved from check_d30.go.

// d30: the agent map (agent control plan, row R7), boxes and lines. Fed lane
// A's frozen example and the layouts internal\agentmap builds from it
// (testdata\agentmap_layout_example.json, the plain and the folded one, each
// kept in step by a Go test) it proves:
//   - the Agents map section is a small read-only overview: counts, the sprint
//     and its agent total, and no animation anywhere inside it;
//   - a click on it opens the full map in the Station's layer as boxes in
//     columns with lines between them, the sprint as a light frame, a You box
//     with a highlighted line, and no two boxes on top of each other;
//   - a click on a box opens its evidence in the right panel and highlights its
//     own lines; a second click, or a click on the background, lets go;
//   - above 40 boxes whole groups fold into one box, and a click on one asks
//     Go to open that frame;
//   - the past-window tabs draw a payload handed in from the store's side;
//   - the real bdevAgentMap binding answers for live and for 168 hours of the
//     real store, and what it returns draws without overlap;
//   - M and P swap the map and the Station, Esc closes the map, and the map
//     animates nothing;
//   - Plain Mode uses plain words, and a missing snapshot offers the last 24
//     hours instead of drawing old numbers.
//
// The page is driven through window.__bdevSections.apply and window.__bdevMap,
// which draw without saving or calling the store. Screenshots: d30-overview,
// d30-map, d30-map-selected, d30-map-folded, d30-map-history, d30-map-real,
// d30-map-plain. The real-store screenshot holds real project names: it stays
// in testdata\uicheck\out and is not for the README.
type d30Box struct {
	ID   string  `json:"id"`
	Cls  string  `json:"cls"`
	Text string  `json:"text"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
}

type d30Page struct {
	Overview   string   `json:"overview"`
	HasBoxes   bool     `json:"hasBoxes"`
	Anims      int      `json:"anims"`
	MapOn      bool     `json:"mapOn"`
	StationOn  bool     `json:"stationOn"`
	Boxes      []d30Box `json:"boxes"`
	Overlap    int      `json:"overlap"`
	Frames     int      `json:"frames"`
	FrameText  string   `json:"frameText"`
	Lines      int      `json:"lines"`
	YouLines   int      `json:"youLines"`
	HlLines    int      `json:"hlLines"`
	DimBoxes   int      `json:"dimBoxes"`
	Words      string   `json:"words"`
	Heads      string   `json:"heads"`
	Panel      string   `json:"panel"`
	PanelOn    bool     `json:"panelOn"`
	Src        string   `json:"src"`
	Count      string   `json:"count"`
	Note       string   `json:"note"`
	TabsOn     []string `json:"tabsOn"`
	Folded     int      `json:"folded"`
	Frames2    []string `json:"openFrames"`
	BodyOver   float64  `json:"bodyOver"`
	ClippedBox int      `json:"clippedBox"`
}

const d30ReadJS = `(function(){
  var ov = document.getElementById('mapOverlay'), st = document.getElementById('mapStage');
  var slot = document.getElementById('secAgentsSlot');
  var boxes = Array.prototype.map.call(st.querySelectorAll('.mapbox'), function(e){
    var r = e.getBoundingClientRect();
    return { id: e.getAttribute('data-box'), cls: e.className, text: e.textContent, x: r.left, y: r.top, w: r.width, h: r.height };
  });
  var overlap = 0;
  for(var i = 0; i < boxes.length; i++) for(var j = i + 1; j < boxes.length; j++){
    var a = boxes[i], b = boxes[j];
    if(!(a.x + a.w <= b.x + .5 || b.x + b.w <= a.x + .5 || a.y + a.h <= b.y + .5 || b.y + b.h <= a.y + .5)) overlap++;
  }
  var tabs = [];
  Array.prototype.forEach.call(document.querySelectorAll('#mapTabs button[aria-pressed="true"]'), function(b){ tabs.push(b.getAttribute('data-src')); });
  var text = function(sel){ return Array.prototype.map.call(st.querySelectorAll(sel), function(e){ return e.textContent; }).join('|'); };
  var anims = 0;
  if(ov.getAnimations) anims += ov.getAnimations({subtree:true}).length;
  if(slot && slot.getAnimations) anims += slot.getAnimations({subtree:true}).length;
  var clipped = 0;
  Array.prototype.forEach.call(st.querySelectorAll('.mapbox .t'), function(e){ if(e.scrollWidth > e.clientWidth + 1) clipped++; });
  return {
    overview: slot ? slot.textContent : '',
    hasBoxes: !!(slot && slot.querySelector('.mapbox')),
    anims: anims,
    mapOn: ov.classList.contains('on'),
    stationOn: document.getElementById('stationOverlay').classList.contains('on'),
    boxes: boxes, overlap: overlap,
    frames: st.querySelectorAll('.mapframe').length, frameText: text('.mapframe'),
    lines: st.querySelectorAll('svg .mapline').length, youLines: st.querySelectorAll('svg .mapline.you').length,
    hlLines: st.querySelectorAll('svg .mapline.hl').length, dimBoxes: st.querySelectorAll('.mapbox.dim').length,
    words: text('.mapword'), heads: text('.mapcolh'),
    panel: document.getElementById('mapPanel').textContent,
    panelOn: document.getElementById('mapPanel').classList.contains('on'),
    src: document.getElementById('mapSrc').textContent, count: document.getElementById('mapCount').textContent,
    note: document.getElementById('mapNote').textContent,
    tabsOn: tabs, folded: st.querySelectorAll('.mapbox.folded').length,
    openFrames: window.__bdevMap.frames(),
    bodyOver: document.body.scrollHeight - document.body.clientHeight,
    clippedBox: clipped
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

func (p d30Page) box(kind string) (d30Box, bool) {
	for _, b := range p.Boxes {
		for _, c := range strings.Fields(b.Cls) {
			if c == kind {
				return b, true
			}
		}
	}
	return d30Box{}, false
}

func d30Has(what, text string, wants ...string) error {
	for _, w := range wants {
		if !strings.Contains(text, w) {
			return d30Fail("%s lacks %q: %q", what, w, text)
		}
	}
	return nil
}

func caseD30(hwnd uintptr, args []string) error {
	root := filepath.Join("..", "..", "testdata")
	read := func(name string) ([]byte, error) {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return nil, fmt.Errorf("read %s (uicheck runs from the tools folder): %w", name, err)
		}
		return b, nil
	}
	snap, err := read("snapshot_example.json")
	if err != nil {
		return err
	}
	view, err := read("agentmap_example.json")
	if err != nil {
		return err
	}
	layout, err := read("agentmap_layout_example.json")
	if err != nil {
		return err
	}
	plainLayout, err := read("agentmap_layout_example_plain.json")
	if err != nil {
		return err
	}
	foldedLayout, err := read("agentmap_layout_folded.json")
	if err != nil {
		return err
	}
	// One count (B6 scope item 8): Go puts the snapshot's wait_counts into the
	// overview's counts (cmd\burnmon-dev\sections_view.go, sectionsPayloadWith),
	// and the page reads wait_counts first, so the overview shows the
	// snapshot's numbers, not the example map's own count.
	var wc struct {
		Counts struct {
			Blocked  int `json:"blocked"`
			YourTurn int `json:"your_turn"`
		} `json:"wait_counts"`
	}
	if err := json.Unmarshal(snap, &wc); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)

	// The section payload is d29's, with the overview's data added as the Go side adds it.
	sectionsJS := func(plain bool, status string, withSnap bool) (string, error) {
		var m map[string]any
		var s json.RawMessage
		if withSnap {
			s = snap
		}
		if err := json.Unmarshal([]byte(d29Payload(d29Opts{Plain: plain, Scrollbar: !plain, Status: status, Snap: s})), &m); err != nil {
			return "", err
		}
		if withSnap {
			var v map[string]any
			if err := json.Unmarshal(view, &v); err != nil {
				return "", err
			}
			c, _ := v["counts"].(map[string]any)
			if c == nil {
				return "", d30Fail("agentmap_example.json has no counts")
			}
			c["waiting"], c["your_turn"] = wc.Counts.Blocked, wc.Counts.YourTurn
			m["agent_map"] = v
		} else {
			m["agent_map"] = nil
		}
		b, err := json.Marshal(m)
		return string(b), err
	}
	apply := func(plain bool, status string, withSnap bool) (d30Page, error) {
		var p d30Page
		js, err := sectionsJS(plain, status, withSnap)
		if err != nil {
			return p, err
		}
		// The eval channel takes one expression, so the two calls share a function.
		if err := d30Eval(`(function(){ window.__bdevMap.close(); window.__bdevSections.apply(` + js + `); })()`); err != nil {
			return p, err
		}
		return d30Read()
	}
	// payload is what bdevAgentMap returns, built by hand around a golden layout.
	payload := func(source string, hours int, note string, lay []byte) string {
		return fmt.Sprintf(`{"source":%q,"hours":%d,"status":"ok","age_seconds":7,"note":%q,"layout":%s}`, source, hours, note, lay)
	}
	show := func(source string, hours int, note string, lay []byte, tab string) (d30Page, error) {
		if err := d30Eval(`window.__bdevMap.setPayload(` + payload(source, hours, note, lay) + `, ` + fmt.Sprintf("%q", tab) + `)`); err != nil {
			return d30Page{}, err
		}
		return d30Read()
	}
	click := func(sel string) (d30Page, error) {
		if err := d30Eval(`document.querySelector(` + fmt.Sprintf("%q", sel) + `).click()`); err != nil {
			return d30Page{}, err
		}
		return d30Read()
	}

	// The overview: counts and the sprint, read-only, nothing animating.
	p, err := apply(false, "ok", true)
	if err != nil {
		return err
	}
	if err := d30Has("overview", p.Overview, "1 session,", "1 subagent,", fmt.Sprintf("%d waiting on you", wc.Counts.Blocked),
		fmt.Sprintf("%d your turn", wc.Counts.YourTurn), "sprint-example", "Open the full map"); err != nil {
		return err
	}
	if p.HasBoxes || p.Anims > 0 || p.MapOn {
		return d30Fail("overview is not a still, read-only summary: boxes %v, animations %d, map open %v", p.HasBoxes, p.Anims, p.MapOn)
	}
	if _, err := screenshot(hwnd, "d30-overview"); err != nil {
		return err
	}

	// A click opens the full map in the Station's layer. The live payload is
	// handed in so the check does not depend on this machine's snapshot.
	if err := d30Eval(`document.getElementById('secAgentsSlot').click()`); err != nil {
		return err
	}
	if p, err = show("live", 0, "", layout, "live"); err != nil {
		return err
	}
	if !p.MapOn || p.StationOn {
		return d30Fail("click on the overview: map open %v, station open %v", p.MapOn, p.StationOn)
	}
	if len(p.Boxes) != 6 || p.Frames != 1 || p.Lines != 5 || p.YouLines != 1 {
		return d30Fail("example map: %d boxes (want 6), %d frames (want 1), %d lines (want 5), %d You lines (want 1)", len(p.Boxes), p.Frames, p.Lines, p.YouLines)
	}
	if p.Overlap != 0 || p.BodyOver > 1 {
		return d30Fail("example map: %d overlapping boxes, body overflow %.0f px", p.Overlap, p.BodyOver)
	}
	if err := d30Has("frame", p.FrameText, "sprint-example", "BurnMon matched them by folder"); err != nil {
		return err
	}
	if err := d30Has("line words", p.Words, "runs", "started", "uses", "waits on"); err != nil {
		return err
	}
	if err := d30Has("column heads", p.Heads, "Tool", "Session", "Subagent", "Programs and servers", "You"); err != nil {
		return err
	}
	you, ok := p.box("you")
	if !ok || !strings.Contains(you.Text, "1 waiting on you") {
		return d30Fail("no You box saying who waits: %+v", you)
	}
	for _, id := range []string{"Claude Code", "example-server", "port 8765", "2 programs", "Explore", "done", "waits on you", "Bash"} {
		all := ""
		for _, b := range p.Boxes {
			all += b.Text + "|"
		}
		if !strings.Contains(all, id) {
			return d30Fail("no box shows %q: %q", id, all)
		}
	}
	if p.PanelOn {
		return d30Fail("the evidence panel is open before any box was clicked")
	}
	if len(p.TabsOn) != 1 || p.TabsOn[0] != "live" || !strings.Contains(p.Src, "snapshot 7 s old") || !strings.Contains(p.Count, "1 session, 1 subagent") {
		return d30Fail("live strip: tab %v, source %q, count %q", p.TabsOn, p.Src, p.Count)
	}
	if p.Anims > 0 {
		return d30Fail("the open map animates (%d animations)", p.Anims)
	}
	if _, err := screenshot(hwnd, "d30-map"); err != nil {
		return err
	}

	// A box click opens its evidence and highlights its own lines.
	if p, err = click(`.mapbox.session`); err != nil {
		return err
	}
	if !p.PanelOn {
		return d30Fail("a box click did not open the evidence panel")
	}
	if err := d30Has("session evidence", p.Panel, "Tokens", "1,200", "API price equivalent", "€2.30", "claude-opus-5-5", "Last active", "Current tool", "store, 20 s old", "matched by BurnMon from the folder", "sprint/example"); err != nil {
		return err
	}
	if p.HlLines != 5 || p.DimBoxes != 0 {
		return d30Fail("session selected: %d highlighted lines (want 5), %d dimmed boxes (want 0)", p.HlLines, p.DimBoxes)
	}
	if _, err := screenshot(hwnd, "d30-map-selected"); err != nil {
		return err
	}
	if p, err = click(`.mapbox.sub`); err != nil {
		return err
	}
	if err := d30Has("subagent evidence", p.Panel, "done", "Explore", "sonnet", "300"); err != nil {
		return err
	}
	if p.HlLines != 1 {
		return d30Fail("subagent selected: %d highlighted lines, want 1 (a finished subagent waits on no one)", p.HlLines)
	}
	if p, err = click(`.mapbox.you`); err != nil {
		return err
	}
	if err := d30Has("You evidence", p.Panel, "agent", "project"); err != nil {
		return err
	}
	if p, err = click(`.mapbox.you`); err != nil {
		return err
	}
	if p.HlLines != 0 || p.PanelOn {
		return d30Fail("second click did not let go: %d highlighted lines, panel open %v", p.HlLines, p.PanelOn)
	}

	// Above 40 boxes whole groups fold; a click asks Go to open that frame.
	if p, err = show("live", 0, "", foldedLayout, "live"); err != nil {
		return err
	}
	if len(p.Boxes) > 40 || p.Folded != 2 || p.Overlap != 0 || p.YouLines < 1 {
		return d30Fail("folded map: %d boxes (want at most 40), %d folded (want 2), %d overlapping, %d You lines", len(p.Boxes), p.Folded, p.Overlap, p.YouLines)
	}
	if _, err := screenshot(hwnd, "d30-map-folded"); err != nil {
		return err
	}
	if p, err = click(`.mapbox.folded`); err != nil {
		return err
	}
	if len(p.Frames2) != 1 || !strings.HasPrefix(p.Frames2[0], "frame:") {
		return d30Fail("clicking a folded box asked to open %v, want one frame id", p.Frames2)
	}

	// A past window handed in from the store's side.
	if p, err = show("history", 24, "Tokens and prices are not kept in the history view, so they read unknown.", layout, "24"); err != nil {
		return err
	}
	if !strings.Contains(p.Src, "last 24 h") || len(p.Boxes) != 6 || len(p.TabsOn) != 1 || p.TabsOn[0] != "24" || !strings.Contains(p.Note, "unknown") {
		return d30Fail("history draw: source %q, %d boxes, tab %v, note %q", p.Src, len(p.Boxes), p.TabsOn, p.Note)
	}
	if _, err := screenshot(hwnd, "d30-map-history"); err != nil {
		return err
	}

	// The real binding, live and 168 hours of the real store. Which sessions
	// these hold depends on the machine, so this asserts the shape and prints
	// the counts. The answer comes back as a promise: stash it, then poll.
	real := func(req string) (struct {
		Err       string          `json:"err"`
		Status    string          `json:"status"`
		Hours     int             `json:"hours"`
		Note      string          `json:"note"`
		Layout    json.RawMessage `json:"layout"`
		Boxes     int             `json:"boxes"`
		Frames    int             `json:"frames"`
		Folded    int             `json:"folded"`
		Session   int             `json:"sessions"`
		Sub       int             `json:"subagents"`
		OpenMulti int             `json:"openMulti"`
	}, error) {
		var out struct {
			Err       string          `json:"err"`
			Status    string          `json:"status"`
			Hours     int             `json:"hours"`
			Note      string          `json:"note"`
			Layout    json.RawMessage `json:"layout"`
			Boxes     int             `json:"boxes"`
			Frames    int             `json:"frames"`
			Folded    int             `json:"folded"`
			Session   int             `json:"sessions"`
			Sub       int             `json:"subagents"`
			OpenMulti int             `json:"openMulti"`
		}
		if err := d30Eval(`(function(){ window.__d30 = null; window.bdevAgentMap(` + req + `).then(function(r){ window.__d30 = r; }, function(e){ window.__d30 = {err: String(e)}; }); })()`); err != nil {
			return out, err
		}
		for i := 0; ; i++ {
			raw, err := evalRaw(`(function(){ var r = window.__d30; if(!r) return null; if(r.err) return {err: r.err};
				  var L = r.layout || {}; return {status: r.status, hours: r.hours, note: r.note || '', layout: L, boxes: (L.boxes || []).length,
				  frames: (L.frames || []).length, folded: (L.frames || []).filter(function(f){ return f.folded; }).length,
				  sessions: (L.counts || {}).sessions || 0, subagents: (L.counts || {}).subagents || 0,
				  openMulti: (L.frames || []).filter(function(f){ return !f.folded && f.count > 1; }).length}; })()`)
			if err != nil {
				return out, err
			}
			if string(raw) != "null" {
				return out, json.Unmarshal(raw, &out)
			}
			if i >= 30 {
				return out, d30Fail("bdevAgentMap gave no answer in 30 s")
			}
			time.Sleep(time.Second)
		}
	}
	liveReal, err := real(`{source:'live', hours:0, open:[]}`)
	if err != nil {
		return err
	}
	if liveReal.Err != "" || liveReal.Status == "" {
		return d30Fail("real live map: error %q, status %q", liveReal.Err, liveReal.Status)
	}
	fmt.Printf("d30: real live map: snapshot %s, %d boxes, %d frames, %d folded, %d sessions, %d subagents\n",
		liveReal.Status, liveReal.Boxes, liveReal.Frames, liveReal.Folded, liveReal.Session, liveReal.Sub)
	histReal, err := real(`{source:'history', hours:168, open:[]}`)
	if err != nil {
		return err
	}
	if histReal.Err != "" || histReal.Hours != 168 || histReal.Status != "ok" {
		return d30Fail("real history: error %q, hours %d, status %q", histReal.Err, histReal.Hours, histReal.Status)
	}
	fmt.Printf("d30: real store, last 168 h: %d boxes, %d frames, %d folded, %d sessions, %d subagents\n",
		histReal.Boxes, histReal.Frames, histReal.Folded, histReal.Session, histReal.Sub)
	if histReal.Session+histReal.Sub == 0 {
		return d30Fail("the real store gave an empty map for 168 h; was the store copy empty?")
	}
	if p, err = show("history", 168, histReal.Note, histReal.Layout, "168"); err != nil {
		return err
	}
	// Above 40 boxes groups fold, largest first, until 40 or until no group
	// of two or more boxes is left open (internal\agentmap\layout.go
	// BuildLayout): a group of one box cannot shrink, and since B7 scope
	// item 8 every folder is its own group, so a week with more than about
	// 38 folders draws more than 40 boxes. Over 40, nothing foldable may stay open.
	if len(p.Boxes) == 0 || p.Overlap != 0 || p.BodyOver > 1 {
		return d30Fail("real history draws %d boxes (want at least 1), %d overlapping, body overflow %.0f px", len(p.Boxes), p.Overlap, p.BodyOver)
	}
	if len(p.Boxes) > 40 && histReal.OpenMulti > 0 {
		return d30Fail("real history draws %d boxes (over 40) with %d group(s) of two or more boxes still open", len(p.Boxes), histReal.OpenMulti)
	}
	if _, err := screenshot(hwnd, "d30-map-real"); err != nil {
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
	if err := d30Has("plain overview", p.Overview, "1 helper,", fmt.Sprintf("%d waiting for you", wc.Counts.Blocked)); err != nil {
		return err
	}
	if err := d30Eval(`document.getElementById('secAgentsSlot').click()`); err != nil {
		return err
	}
	if p, err = show("live", 0, "", plainLayout, "live"); err != nil {
		return err
	}
	all := p.Heads + "|" + p.FrameText + "|" + p.Words
	for _, b := range p.Boxes {
		all += "|" + b.Text
	}
	if err := d30Has("plain map", all, "Helper", "Programs and tool servers", "Foreman job", "waiting for", "tool server", "finished"); err != nil {
		return err
	}
	for _, jargon := range []string{"MCP", "subagent", "Subagent", "waits on"} {
		if strings.Contains(all, jargon) {
			return d30Fail("plain map still shows %q: %q", jargon, all)
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
		return d30Fail("missing snapshot map: open %v, tab %v", p.MapOn, p.TabsOn)
	}
	if err := d30Eval(`window.__bdevMap.close()`); err != nil {
		return err
	}
	fmt.Println("d30: ok")
	return nil
}

// map/d31: moved from check_d31.go.

// d31: the live check of the agent map on a real run, not a fixture. Start it
// while a Foreman sprint with subagents is running. It asks the real
// bdevAgentMap binding for the live map (the snapshot BurnMon core is writing
// now), prints every frame and the line kinds it holds, draws the whole map
// (d31-live-map-all, real project names: stays in testdata\uicheck\out), then
// draws only the frame named by the first argument, with the harness and You
// boxes that belong to it, for the README (d31-live-map-readme). It prints
// every string that shows in that picture so a person can check it for private
// data before it goes anywhere. It asserts nothing about content: what a real
// run shows is the finding. uicheck.ps1 passes no arguments through, so the
// choices are environment variables: D31_FRAME is the frame to draw for the
// README (default znd-skills) and D31_HOURS, when set, asks for that many hours
// of the store instead of the live snapshot.
const d31FilterJS = `(function(label){
  var p = window.__d31, L = p && p.layout;
  if(!L) return {err: 'no layout'};
  var f = (L.frames || []).filter(function(x){ return x.label === label; })[0];
  if(!f) return {err: 'no frame ' + label, frames: (L.frames || []).map(function(x){ return x.label; })};
  var keep = {};
  L.boxes.forEach(function(b){ if(b.group === f.id) keep[b.id] = true; });
  L.lines.forEach(function(l){ if(keep[l.to] && l.from.indexOf('harness:') === 0) keep[l.from] = true; });
  var you = L.lines.filter(function(l){ return l.you && keep[l.from]; });
  if(you.length) keep['you'] = true;
  var boxes = L.boxes.filter(function(b){ return keep[b.id]; }).map(function(b){ return JSON.parse(JSON.stringify(b)); });
  var rows = [];
  boxes.forEach(function(b){ if(rows.indexOf(b.row) < 0) rows.push(b.row); });
  rows.sort(function(a, b){ return a - b; });
  var idx = {}; rows.forEach(function(r, i){ idx[r] = i; });
  boxes.forEach(function(b){
    b.row = idx[b.row];
    if(b.kind === 'harness') b.line = '';
    if(b.kind === 'you') b.line = you.length + ' waiting on you';
  });
  var fr = JSON.parse(JSON.stringify(f));
  fr.first_row = idx[fr.first_row] !== undefined ? idx[fr.first_row] : 0;
  fr.last_row = Math.max.apply(null, boxes.filter(function(b){ return b.group === f.id; }).map(function(b){ return b.row; }));
  var lines = L.lines.filter(function(l){ return keep[l.to] && (keep[l.from] || l.from === f.id); });
  var counts = {sessions: boxes.filter(function(b){ return b.kind === 'session'; }).length, subagents: boxes.filter(function(b){ return b.kind === 'sub'; }).length};
  var layout = {columns: L.columns, boxes: boxes, lines: lines, frames: [fr], idle: [], counts: counts, notes: L.notes};
  window.__bdevMap.setPayload({source: p.source, hours: p.hours || 0, status: p.status, age_seconds: p.age_seconds, note: p.note || '', layout: layout}, p.source === 'live' ? 'live' : String(p.hours));
  var st = document.getElementById('mapStage');
  return {boxes: Array.prototype.map.call(st.querySelectorAll('.mapbox'), function(e){ return e.textContent; }),
    frame: Array.prototype.map.call(st.querySelectorAll('.mapframe'), function(e){ return e.textContent; }),
    words: Array.prototype.map.call(st.querySelectorAll('.mapword'), function(e){ return e.textContent; }),
    heads: Array.prototype.map.call(st.querySelectorAll('.mapcolh'), function(e){ return e.textContent; }),
    src: document.getElementById('mapSrc').textContent, count: document.getElementById('mapCount').textContent,
    note: document.getElementById('mapNote').textContent};
})`

func caseD31(hwnd uintptr, args []string) error {
	label := "znd-skills"
	if v := os.Getenv("D31_FRAME"); v != "" {
		label = v
	}
	source, hours := "live", 0
	if v := os.Getenv("D31_HOURS"); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &hours); err != nil || hours <= 0 {
			return fmt.Errorf("d31: D31_HOURS %q is not a number of hours", v)
		}
		source = "history"
	}
	time.Sleep(500 * time.Millisecond)
	if err := d30Eval(`window.__bdevMap.open('live')`); err != nil {
		return err
	}
	if err := d30Eval(`(function(){ window.__d31 = null; window.bdevAgentMap({source:` + fmt.Sprintf("%q", source) + `, hours:` + fmt.Sprint(hours) + `, open:[]}).then(function(r){ window.__d31 = r; }, function(e){ window.__d31 = {err: String(e)}; }); })()`); err != nil {
		return err
	}
	var raw json.RawMessage
	for i := 0; ; i++ {
		r, err := evalRaw(`window.__d31`)
		if err != nil {
			return err
		}
		if string(r) != "null" {
			raw = r
			break
		}
		if i >= 30 {
			return fmt.Errorf("d31: bdevAgentMap gave no answer in 30 s")
		}
		time.Sleep(time.Second)
	}
	if err := os.WriteFile(filepath.Join("..", "..", "testdata", "uicheck", "out", "d31-live-layout.json"), raw, 0o644); err != nil {
		return err
	}
	var live struct {
		Err    string `json:"err"`
		Status string `json:"status"`
		Layout struct {
			Frames []struct {
				Label  string `json:"label"`
				Kind   string `json:"kind"`
				Status string `json:"status"`
				Count  int    `json:"count"`
			} `json:"frames"`
			Boxes []struct {
				Kind  string `json:"kind"`
				Title string `json:"title"`
			} `json:"boxes"`
			Lines []struct {
				Word  string `json:"word"`
				Basis string `json:"basis"`
			} `json:"lines"`
			Idle []struct {
				Label string `json:"label"`
			} `json:"idle"`
		} `json:"layout"`
	}
	if err := json.Unmarshal(raw, &live); err != nil {
		return err
	}
	if live.Err != "" {
		return fmt.Errorf("d31: real live map: %s", live.Err)
	}
	fmt.Printf("d31: %s map, status %s: %d boxes, %d lines, %d frames, %d idle sprints\n", source, live.Status, len(live.Layout.Boxes), len(live.Layout.Lines), len(live.Layout.Frames), len(live.Layout.Idle))
	words := map[string]int{}
	for _, l := range live.Layout.Lines {
		words[l.Word+"/"+l.Basis]++
	}
	fmt.Printf("d31: line words (word/basis: count): %v\n", words)
	kinds := map[string]int{}
	for _, b := range live.Layout.Boxes {
		kinds[b.Kind]++
	}
	fmt.Printf("d31: box kinds: %v\n", kinds)
	for _, f := range live.Layout.Frames {
		fmt.Printf("d31: frame %q kind %s status %q holds %d boxes\n", f.Label, f.Kind, f.Status, f.Count)
	}
	sprintFrames := 0
	for _, f := range live.Layout.Frames {
		if f.Kind == "sprint" {
			sprintFrames++
		}
	}
	fmt.Printf("d31: %d sprint frames in the live map (a sprint frame needs a works-for link)\n", sprintFrames)
	if _, err := screenshot(hwnd, "d31-live-map-all"); err != nil {
		return err
	}
	var out struct {
		Err    string   `json:"err"`
		Frames []string `json:"frames"`
		Boxes  []string `json:"boxes"`
		Frame  []string `json:"frame"`
		Words  []string `json:"words"`
		Heads  []string `json:"heads"`
		Src    string   `json:"src"`
		Count  string   `json:"count"`
		Note   string   `json:"note"`
	}
	if err := evalInto(d31FilterJS+`(`+fmt.Sprintf("%q", label)+`)`, &out); err != nil {
		return err
	}
	if out.Err != "" {
		fmt.Printf("d31: no README picture: %s, frames %v\n", out.Err, out.Frames)
		return nil
	}
	time.Sleep(500 * time.Millisecond)
	if _, err := screenshot(hwnd, "d31-live-map-readme"); err != nil {
		return err
	}
	fmt.Println("d31: strings in the README picture, check them for private data:")
	fmt.Println("  columns:", strings.Join(out.Heads, " | "))
	fmt.Println("  frame:", strings.Join(out.Frame, " | "))
	for _, b := range out.Boxes {
		fmt.Println("  box:", b)
	}
	fmt.Println("  line words:", strings.Join(out.Words, ", "))
	fmt.Println("  header:", out.Src, "/", out.Count)
	fmt.Println("  strip note:", out.Note)
	if err := d30Eval(`window.__bdevMap.close()`); err != nil {
		return err
	}
	fmt.Println("d31: ok")
	return nil
}

// map/d35: moved from check_d35.go.

// d35: the agent map spreads its columns over the window width (B2). The
// frozen layout example (5 columns) is handed to the open map at two window
// sizes, with and without the side panel. It proves:
//   - the column headers are evenly spaced and the first sits 24 px from the
//     stage's left edge;
//   - when the area is wide enough for 222 px columns, the last box ends 24 px
//     from the right edge of the scroll area and nothing scrolls sideways;
//   - below that width the pitch stays 222 px.
//
// Screenshots: d35-map-1280, d35-map-1920.
type d35Page struct {
	Lefts   []float64 `json:"lefts"`
	Stage   float64   `json:"stage"`
	AvailW  float64   `json:"availW"`
	ScrollW float64   `json:"scrollW"`
	First   string    `json:"first"`
}

const d35ReadJS = `(function(){
  var st = document.getElementById('mapStage'), sc = document.getElementById('mapScroll');
  var s = st.getBoundingClientRect();
  var lefts = Array.prototype.map.call(st.querySelectorAll('.mapcolh'), function(e){ return e.getBoundingClientRect().left - s.left; });
  var f = st.querySelector('.mapbox.session');
  return { lefts: lefts, stage: s.width, availW: sc.clientWidth, scrollW: sc.scrollWidth, first: f ? f.getAttribute('data-box') : '' };
})()`

func d35Check(tag string, p d35Page) error {
	n := len(p.Lefts)
	if n != 5 {
		return fmt.Errorf("d35: %s: %d column headers, want 5", tag, n)
	}
	if math.Abs(p.Lefts[0]-24) > 1 {
		return fmt.Errorf("d35: %s: first column at %.1f px, want 24", tag, p.Lefts[0])
	}
	pitch := p.Lefts[1] - p.Lefts[0]
	for i := 2; i < n; i++ {
		if math.Abs((p.Lefts[i]-p.Lefts[i-1])-pitch) > 1 {
			return fmt.Errorf("d35: %s: columns are not evenly spaced: %v", tag, p.Lefts)
		}
	}
	if 48+4*222+170 <= p.AvailW {
		if math.Abs(p.Lefts[n-1]+170+24-p.AvailW) > 2 {
			return fmt.Errorf("d35: %s: last column ends at %.1f px, scroll area is %.1f px wide", tag, p.Lefts[n-1]+170+24, p.AvailW)
		}
		if p.ScrollW > p.AvailW+1 {
			return fmt.Errorf("d35: %s: map scrolls sideways (%.0f > %.0f)", tag, p.ScrollW, p.AvailW)
		}
	} else if math.Abs(pitch-222) > 1 {
		return fmt.Errorf("d35: %s: pitch %.1f px in a narrow area (%.0f px), want 222", tag, pitch, p.AvailW)
	}
	return nil
}

func caseD35(hwnd uintptr, args []string) error {
	layout, err := readFixture("agentmap_layout_example.json")
	if err != nil {
		return fmt.Errorf("read the layout example (uicheck runs from the tools folder): %w", err)
	}
	payload := fmt.Sprintf(`{"source":"live","hours":0,"status":"ok","age_seconds":7,"note":"","layout":%s}`, layout)
	if !json.Valid([]byte(payload)) {
		return fmt.Errorf("d35: the layout example is not valid JSON")
	}
	read := func() (d35Page, error) {
		var p d35Page
		time.Sleep(400 * time.Millisecond)
		err := evalInto(d35ReadJS, &p)
		return p, err
	}
	for _, sz := range [][2]int32{{1280, 860}, {1920, 1080}} {
		if err := ensureWindowSizeWH(hwnd, sz[0], sz[1]); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		// The dev eval server wraps a script as return (...), so a statement
		// list goes in a function or it never parses and the call times out.
		if _, err := evalRaw(`(function(){ window.__bdevMap.close(); window.__bdevMap.open('live'); window.__bdevMap.setPayload(` + payload + `, 'live'); return true; })()`); err != nil {
			return err
		}
		p, err := read()
		if err != nil {
			return err
		}
		tag := fmt.Sprintf("%dx%d", sz[0], sz[1])
		if err := d35Check(tag, p); err != nil {
			return err
		}
		if _, err := screenshot(hwnd, fmt.Sprintf("d35-map-%d", sz[0])); err != nil {
			return err
		}
		if p.First == "" {
			return fmt.Errorf("d35: %s: no session box in the map", tag)
		}
		if _, err := evalRaw(`window.__bdevMap.select(` + jsString(p.First) + `)`); err != nil {
			return err
		}
		if p, err = read(); err != nil {
			return err
		}
		if err := d35Check(tag+" with the side panel", p); err != nil {
			return err
		}
	}
	_, err = evalRaw(`window.__bdevMap.close()`)
	return err
}

// map/d38: moved from check_d38.go.

// d38: the map draws five fixed columns with visible borders (B6). The map is
// opened through window.__bdevMap and fed layouts built from the frozen
// fixtures; window.bdevAgentMap is REPLACED by a recorder, so nothing is read
// from the store and no control exists on this screen. It proves:
//   - five column headers, and an empty column carries "none in this window";
//   - a session box border is drawn in the --bd2 colour (not the faint --bd);
//   - a dotted line has a stroke-dasharray, a solid one has none;
//   - a click on the fold box asks Go for the frame: open holds subs:<parent>;
//   - the header count ends in the You box's own count (waiting on you).
//
// Screenshot: d38-map.
const d38FoldID = "fold:subs:session:anthropic:example-parent"

const d38ReadJS = `(function(){
  var st = document.getElementById('mapStage');
  var sess = st.querySelector('.mapbox.session');
  var dotted = st.querySelector('svg .mapline.dotted'), solid = st.querySelector('svg .mapline:not(.dotted)');
  var probe = document.createElement('i');
  probe.style.borderTop = '1px solid var(--bd2)';
  document.body.appendChild(probe);
  var bd2 = getComputedStyle(probe).borderTopColor;
  document.body.removeChild(probe);
  var you = st.querySelector('.mapbox.you .l');
  return {
    heads: Array.prototype.map.call(st.querySelectorAll('.mapcolh'), function(e){ return e.textContent; }),
    notes: Array.prototype.map.call(st.querySelectorAll('.mapcoln'), function(e){ return e.textContent; }),
    border: sess ? getComputedStyle(sess).borderTopColor : '',
    bd2: bd2,
    dotted: dotted ? getComputedStyle(dotted).strokeDasharray : '',
    solid: solid ? getComputedStyle(solid).strokeDasharray : '',
    count: document.getElementById('mapCount').textContent,
    you: you ? you.textContent : '',
    calls: window.__d38calls.slice()
  };
})()`

type d38Page struct {
	Heads  []string         `json:"heads"`
	Notes  []string         `json:"notes"`
	Border string           `json:"border"`
	Bd2    string           `json:"bd2"`
	Dotted string           `json:"dotted"`
	Solid  string           `json:"solid"`
	Count  string           `json:"count"`
	You    string           `json:"you"`
	Calls  []map[string]any `json:"calls"`
}

func caseD38(hwnd uintptr, args []string) error {
	fail := func(format string, a ...any) error { return fmt.Errorf("d38: "+format, a...) }
	load := func(name string) (any, error) {
		raw, err := readFixture(name)
		if err != nil {
			return nil, fmt.Errorf("read %s (uicheck runs from the tools folder): %w", name, err)
		}
		var L map[string]any
		if err := json.Unmarshal(raw, &L); err != nil {
			return nil, err
		}
		return L, nil
	}
	example, err := load("agentmap_layout_example.json")
	if err != nil {
		return err
	}
	folded, err := load("agentmap_layout_folded.json")
	if err != nil {
		return err
	}
	// Derive the layout with the subagent fold box and the dotted line from the example.
	ex := example.(map[string]any)
	boxes := ex["boxes"].([]any)
	var sub map[string]any
	for _, b := range boxes {
		if b.(map[string]any)["id"] == "session:anthropic:example-sub" {
			sub = b.(map[string]any)
		}
	}
	if sub == nil {
		return fail("the example layout has no subagent box")
	}
	ex["boxes"] = append(boxes, map[string]any{"id": d38FoldID, "kind": "folded", "title": "11 subagents",
		"line": "click to open", "column": 2, "row": 4, "waits_on_you": false, "facts": []any{}, "tag": "group"})
	ex["lines"] = append(ex["lines"].([]any), map[string]any{"from": sub["id"], "to": "session:anthropic:example-parent",
		"word": "reports to", "basis": "vendor", "you": false, "style": "dotted"})
	wrap := func(L any) string {
		b, _ := json.Marshal(map[string]any{"layout": L, "hours": 24})
		return string(b)
	}
	read := func() (d38Page, error) {
		var p d38Page
		time.Sleep(400 * time.Millisecond)
		err := evalInto(d38ReadJS, &p)
		return p, err
	}
	// The dev eval server wraps a script as return (...), so a statement
	// list goes in a function or it never parses and the call times out.
	if _, err := evalRaw(`(function(){ window.__d38calls = [];
			window.bdevAgentMap = function(r){ window.__d38calls.push(JSON.parse(JSON.stringify(r))); return Promise.resolve(` + wrap(example) + `); };
			window.__bdevMap.open('live'); return true; })()`); err != nil {
		return err
	}

	// Zero subagents: five headers and the note in the empty columns.
	if _, err := evalRaw(`(function(){ window.__bdevMap.setPayload(` + wrap(folded) + `, 'live'); return true; })()`); err != nil {
		return err
	}
	p, err := read()
	if err != nil {
		return err
	}
	if want := "Tool|Session|Subagents|Programs and servers|You"; strings.Join(p.Heads, "|") != want {
		return fail("column headers %v, want %s", p.Heads, want)
	}
	if len(p.Notes) == 0 || p.Notes[0] != "none in this window" {
		return fail("empty column notes %v, want none in this window", p.Notes)
	}

	// Borders, line styles, fold click and the count.
	if _, err := evalRaw(`(function(){ window.__bdevMap.setPayload(` + wrap(example) + `, 'live'); return true; })()`); err != nil {
		return err
	}
	if p, err = read(); err != nil {
		return err
	}
	if p.Bd2 == "" || p.Border != p.Bd2 {
		return fail("session box border %q, want the --bd2 colour %q", p.Border, p.Bd2)
	}
	if p.Dotted == "" || p.Dotted == "none" {
		return fail("the dotted line has stroke-dasharray %q", p.Dotted)
	}
	if p.Solid != "none" {
		return fail("a solid line has stroke-dasharray %q, want none", p.Solid)
	}
	if !strings.Contains(p.Count, "1 waiting on you") || !strings.HasPrefix(p.You, "1 waiting on you") {
		return fail("header count %q and You box %q do not agree on 1 waiting on you", p.Count, p.You)
	}
	if _, err := screenshot(hwnd, "d38-map"); err != nil {
		return err
	}
	if _, err := evalRaw(`(function(){ document.querySelector('.mapbox.folded').click(); return true; })()`); err != nil {
		return err
	}
	if p, err = read(); err != nil {
		return err
	}
	if len(p.Calls) == 0 {
		return fail("the fold click did not ask Go for the map")
	}
	last := p.Calls[len(p.Calls)-1]
	open, _ := last["open"].([]any)
	if len(open) != 1 || open[0] != "subs:session:anthropic:example-parent" {
		return fail("fold click asked for open %v, want [subs:session:anthropic:example-parent]", last["open"])
	}
	return nil
}

// map/d40: moved from check_d40.go.

// d40: the map's idle list is a button and a panel table (B7, spec items 7 and
// 10). The map is opened through window.__bdevMap and fed the frozen example
// layout with 14 idle sprints (invented names) through setPayload, so nothing
// is read from the store. At 1280x800 and 1920x1080 (a size the screen cannot
// hold prints SKIPPED and is not a pass) it proves:
//   - the page has no .mapidle element, only #mapIdleBtn;
//   - a click on the button opens #mapPanel with a table of all 14 rows in the
//     group order Done needs close-out, the rest (two groups), each group
//     longest wait first;
//   - the panel rectangle intersects no .mapbox rectangle, and every .mapbox
//     has a non-zero rectangle inside #mapScroll (nothing is drawn over a box);
//   - Esc closes the idle table first and leaves the map open, a second Esc
//     closes the map;
//   - a click on a box with the table open swaps the panel to that box's
//     evidence and the idle table is gone.
//
// Screenshot: d40-map-idle-<size>.

type d40Idle struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Label      string `json:"label"`
	Project    string `json:"project,omitempty"`
	SpecPath   string `json:"spec_path,omitempty"`
	ResultPath string `json:"result_path,omitempty"`
	WaitsOnYou bool   `json:"waits_on_you"`
	AgeSeconds int64  `json:"age_seconds"`
	CloseOut   bool   `json:"close_out,omitempty"`
}

// d40Fixture builds 14 idle entries: 4 merged sprints to close out and 10
// plain. Ages are distinct so the longest-first order is testable.
func d40Fixture() ([]d40Idle, []string) {
	var idle []d40Idle
	for i := 1; i <= 14; i++ {
		it := d40Idle{
			ID: fmt.Sprintf("sprint:example/example-sprint-%02d", i), Kind: "sprint",
			Label: fmt.Sprintf("example-sprint-%02d", i), Project: "example-project",
			SpecPath: fmt.Sprintf(`C:\example\specs\example-%02d.md`, i), ResultPath: fmt.Sprintf(`C:\example\sprints\example-%02d\report.md`, i),
			AgeSeconds: int64((i*5%14 + 1) * 3600),
		}
		if i <= 4 {
			it.CloseOut = true
		}
		if i%5 == 0 {
			it.Kind = "flow"
		}
		idle = append(idle, it)
	}
	// Expected row order: close-out, then the rest; longest wait first in a group.
	group := func(i d40Idle) int {
		if i.CloseOut {
			return 0
		}
		return 1
	}
	sorted := append([]d40Idle(nil), idle...)
	sort.SliceStable(sorted, func(a, b int) bool {
		if group(sorted[a]) != group(sorted[b]) {
			return group(sorted[a]) < group(sorted[b])
		}
		return sorted[a].AgeSeconds > sorted[b].AgeSeconds
	})
	var want []string
	for _, i := range sorted {
		want = append(want, i.Label)
	}
	return idle, want
}

type d40Page struct {
	OldIdle   int      `json:"oldIdle"`
	BtnText   string   `json:"btnText"`
	PanelOn   bool     `json:"panelOn"`
	Rows      []string `json:"rows"`
	Heads     []string `json:"heads"`
	Group     []string `json:"group"`
	Boxes     int      `json:"boxes"`
	Overlap   []string `json:"overlap"`
	Outside   []string `json:"outside"`
	PanelH3   string   `json:"panelH3"`
	TableOn   bool     `json:"tableOn"`
	MapOpen   bool     `json:"mapOpen"`
	PanelRect string   `json:"panelRect"`
}

const d40ReadJS = `(function(){
  var panel = document.getElementById('mapPanel'), scroll = document.getElementById('mapScroll');
  var pr = panel.getBoundingClientRect(), sr = scroll.getBoundingClientRect();
  var on = panel.classList.contains('on') && pr.width > 0;
  var overlap = [], outside = [], n = 0;
  Array.prototype.forEach.call(document.querySelectorAll('#mapStage .mapbox'), function(b){
    var r = b.getBoundingClientRect();
    n++;
    var id = b.getAttribute('data-box');
    if(r.width <= 0 || r.height <= 0){ outside.push(id + ' has no rectangle'); return; }
    if(on && !(r.right <= pr.left || r.left >= pr.right || r.bottom <= pr.top || r.top >= pr.bottom)) overlap.push(id);
    if(r.left < sr.left - 1 || r.right > sr.right + 1 || r.top < sr.top - 1 || r.bottom > sr.bottom + 1) outside.push(id);
  });
  var h3 = panel.querySelector('h3');
  return {
    oldIdle: document.querySelectorAll('.mapidle').length,
    btnText: (document.getElementById('mapIdleBtn') || {textContent: ''}).textContent,
    panelOn: on,
    rows: Array.prototype.map.call(panel.querySelectorAll('table.mapidletbl tbody tr:not(.grp)'), function(r){ return r.cells[0].textContent; }),
    heads: Array.prototype.map.call(panel.querySelectorAll('table.mapidletbl thead th'), function(t){ return t.textContent; }),
    group: Array.prototype.map.call(panel.querySelectorAll('table.mapidletbl tr.grp'), function(t){ return t.textContent; }),
    boxes: n, overlap: overlap, outside: outside,
    panelH3: h3 ? h3.textContent : '',
    tableOn: !!panel.querySelector('table.mapidletbl'),
    mapOpen: window.__bdevMap.isOpen(),
    panelRect: Math.round(pr.left) + ',' + Math.round(pr.top) + ',' + Math.round(pr.right) + ',' + Math.round(pr.bottom)
  };
})()`

func caseD40(hwnd uintptr, args []string) error {
	fail := func(format string, a ...any) error { return fmt.Errorf("d40: "+format, a...) }
	raw, err := readFixture("agentmap_layout_example.json")
	if err != nil {
		return fmt.Errorf("read the example layout (uicheck runs from the tools folder): %w", err)
	}
	var L map[string]any
	if err := json.Unmarshal(raw, &L); err != nil {
		return err
	}
	// The frozen example, Standard with every part on (applyStart has already left fake mode).
	if _, err := evalRaw(`window.__bdevSections.apply(` + d29Payload(d29Opts{Status: "ok"}) + `)`); err != nil {
		return fail("set the starting state: %w", err)
	}
	idle, want := d40Fixture()
	L["idle"] = idle
	b, _ := json.Marshal(map[string]any{"layout": L, "hours": 24})
	payload := string(b)
	read := func() (d40Page, error) {
		var p d40Page
		time.Sleep(400 * time.Millisecond)
		err := evalInto(d40ReadJS, &p)
		return p, err
	}
	ran := 0
	var fails []string
	for _, sz := range [][2]int32{{1280, 800}, {1920, 1080}} {
		tag := fmt.Sprintf("%dx%d", sz[0], sz[1])
		if err := ensureWindowSizeWH(hwnd, sz[0], sz[1]); err != nil {
			return err
		}
		if windowCapped {
			fmt.Printf("uicheck: d40: SKIPPED %s, this screen is too small\n", tag)
			continue
		}
		time.Sleep(400 * time.Millisecond)
		if _, err := evalRaw(`(function(){ window.__bdevMap.close(); window.bdevAgentMap = function(){ return new Promise(function(){}); };
				window.__bdevMap.open('live'); window.__bdevMap.setPayload(` + payload + `, 'live'); return true; })()`); err != nil {
			return err
		}
		p, err := read()
		if err != nil {
			return err
		}
		if p.OldIdle != 0 || !strings.HasPrefix(p.BtnText, "14 ") {
			return fail("%s: .mapidle elements %d, button text %q, want none and 14 ...", tag, p.OldIdle, p.BtnText)
		}
		if p.PanelOn {
			return fail("%s: the panel is open before the button was clicked", tag)
		}
		if err := clickSelector(hwnd, "#mapIdleBtn"); err != nil {
			return err
		}
		if p, err = read(); err != nil {
			return err
		}
		if !p.PanelOn || !p.TableOn {
			return fail("%s: the button did not open the idle table (panel on %v, table %v)", tag, p.PanelOn, p.TableOn)
		}
		if strings.Join(p.Heads, "|") != "Name|Project|Kind|Waited|Links" {
			return fail("%s: table heads %v", tag, p.Heads)
		}
		if len(p.Rows) != 14 || strings.Join(p.Rows, ",") != strings.Join(want, ",") {
			return fail("%s: %d rows %v, want 14 in the order %v", tag, len(p.Rows), p.Rows, want)
		}
		if len(p.Group) != 2 || p.Group[0] != "Done, needs close-out" || p.Group[1] != "Other sprints and flows" {
			return fail("%s: group headings %v, want exactly two", tag, p.Group)
		}
		if p.Boxes == 0 {
			return fail("%s: the map has no boxes", tag)
		}
		// These two do not stop the run, so each size reports its own result.
		sizeOK := true
		if len(p.Overlap) != 0 {
			msg := fmt.Sprintf("%s: the panel %s overlaps boxes %v", tag, p.PanelRect, p.Overlap)
			fmt.Println("uicheck: d40: FAIL " + msg)
			fails = append(fails, msg)
			sizeOK = false
		}
		if len(p.Outside) != 0 {
			msg := fmt.Sprintf("%s: boxes not fully visible in the map area: %v", tag, p.Outside)
			fmt.Println("uicheck: d40: FAIL " + msg)
			fails = append(fails, msg)
			sizeOK = false
		}
		rect, boxes := p.PanelRect, p.Boxes
		if _, err := screenshot(hwnd, "d40-map-idle-"+tag); err != nil {
			return err
		}
		// Esc closes the table first, then the map.
		bringToFront(hwnd)
		_ = clickSelector(hwnd, "#mapTitle")
		if err := pressKey(vkEscape); err != nil {
			return err
		}
		if p, err = read(); err != nil {
			return err
		}
		if p.TableOn || p.PanelOn || !p.MapOpen {
			return fail("%s: first Esc left table %v, panel %v, map open %v, want closed, closed, open", tag, p.TableOn, p.PanelOn, p.MapOpen)
		}
		if err := pressKey(vkEscape); err != nil {
			return err
		}
		if p, err = read(); err != nil {
			return err
		}
		if p.MapOpen {
			return fail("%s: second Esc did not close the map", tag)
		}
		// A box click with the table open shows that box's evidence.
		if _, err := evalRaw(`(function(){ window.__bdevMap.open('live'); window.__bdevMap.setPayload(` + payload + `, 'live'); window.__bdevMap.idle(true); return true; })()`); err != nil {
			return err
		}
		if p, err = read(); err != nil {
			return err
		}
		if !p.TableOn {
			return fail("%s: the table did not reopen", tag)
		}
		if err := clickSelector(hwnd, "#mapStage .mapbox.session"); err != nil {
			return err
		}
		if p, err = read(); err != nil {
			return err
		}
		if p.TableOn || !p.PanelOn || p.PanelH3 == "" {
			return fail("%s: box click left table %v, panel on %v, title %q", tag, p.TableOn, p.PanelOn, p.PanelH3)
		}
		if _, err := evalRaw(`(window.__bdevMap.close(), true)`); err != nil {
			return err
		}
		ran++
		if !sizeOK {
			continue
		}
		fmt.Printf("uicheck: d40: PASS %s (14 idle rows in order, panel %s overlaps no box, %d boxes visible, Esc twice, box click)\n", tag, rect, boxes)
	}
	if len(fails) > 0 {
		return fail("%d check(s) failed, first: %s", len(fails), fails[0])
	}
	if ran == 0 {
		return fail("no size ran")
	}
	return nil
}

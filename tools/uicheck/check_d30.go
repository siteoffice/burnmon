package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
	if err := ensureWindowSizeWH(hwnd, 1152, 2048); err != nil {
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
			m["agent_map"] = json.RawMessage(view)
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
	if err := d30Has("overview", p.Overview, "1 session,", "1 subagent,", "1 waiting on you", "sprint-example", "Open the full map"); err != nil {
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
		Err     string          `json:"err"`
		Status  string          `json:"status"`
		Hours   int             `json:"hours"`
		Note    string          `json:"note"`
		Layout  json.RawMessage `json:"layout"`
		Boxes   int             `json:"boxes"`
		Frames  int             `json:"frames"`
		Folded  int             `json:"folded"`
		Session int             `json:"sessions"`
		Sub     int             `json:"subagents"`
	}, error) {
		var out struct {
			Err     string          `json:"err"`
			Status  string          `json:"status"`
			Hours   int             `json:"hours"`
			Note    string          `json:"note"`
			Layout  json.RawMessage `json:"layout"`
			Boxes   int             `json:"boxes"`
			Frames  int             `json:"frames"`
			Folded  int             `json:"folded"`
			Session int             `json:"sessions"`
			Sub     int             `json:"subagents"`
		}
		if err := d30Eval(`(function(){ window.__d30 = null; window.bdevAgentMap(` + req + `).then(function(r){ window.__d30 = r; }, function(e){ window.__d30 = {err: String(e)}; }); })()`); err != nil {
			return out, err
		}
		for i := 0; ; i++ {
			raw, err := evalRaw(`(function(){ var r = window.__d30; if(!r) return null; if(r.err) return {err: r.err};
				  var L = r.layout || {}; return {status: r.status, hours: r.hours, note: r.note || '', layout: L, boxes: (L.boxes || []).length,
				  frames: (L.frames || []).length, folded: (L.frames || []).filter(function(f){ return f.folded; }).length,
				  sessions: (L.counts || {}).sessions || 0, subagents: (L.counts || {}).subagents || 0}; })()`)
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
	if len(p.Boxes) == 0 || len(p.Boxes) > 40 || p.Overlap != 0 || p.BodyOver > 1 {
		return d30Fail("real history draws %d boxes (want 1 to 40), %d overlapping, body overflow %.0f px", len(p.Boxes), p.Overlap, p.BodyOver)
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
	if err := d30Has("plain overview", p.Overview, "1 helper,", "waiting for you"); err != nil {
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

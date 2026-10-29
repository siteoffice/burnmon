package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

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
	defer evalRaw(`(window.__bdevExitFakeMode && window.__bdevExitFakeMode())`)
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

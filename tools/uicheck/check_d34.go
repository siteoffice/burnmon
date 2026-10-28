package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
// Each case asserts:
//   - no vertical empty area over 24 px inside a column (the largest gap
//     between a column's children, below the last child, or inside a panel
//     below its last child);
//   - no part hidden: every [data-part] without .sec-off draws, and the old
//     "sections hidden" note is gone;
//   - the Processes table sits above To Do (it comes first in the document and
//     its bottom is at or above To Do's top), and below Process groups, Memory,
//     Disks and Network;
//   - scrollbar off: the page scrolls only when window.__bdevFit.fitScroll says
//     every part is at its floor; otherwise nothing overflows, and in two
//     columns both columns end on the same line (within 2 px).
// A window size the screen cannot hold prints SKIPPED and is not a pass.
// Screenshots: d34-<size>-<mode>-<case>-<sb>.

const d34MaxEmpty = 24.0

type d34Page struct {
	OverflowY  string   `json:"overflowY"`
	Scrolls    bool     `json:"scrolls"`
	BodyOver   float64  `json:"bodyOver"`
	PageOver   float64  `json:"pageOver"`
	HostOver   float64  `json:"hostOver"`
	ZoneOver   float64  `json:"zoneOver"`
	BodyScroll bool     `json:"bodyScroll"`
	FitScroll  bool     `json:"fitScroll"`
	FitClass   bool     `json:"fitClass"`
	Fit        bool     `json:"fit"`
	FitHidden  []string `json:"fitHidden"`
	Hidden     []string `json:"hidden"`
	CutNote    bool     `json:"cutNote"`
	Empty      float64  `json:"empty"`
	EmptyWhere string   `json:"emptyWhere"`
	ProcOn     bool     `json:"procOn"`
	ProcFirst  bool     `json:"procFirst"`
	ProcAbove  bool     `json:"procAbove"`
	ProcBelow  bool     `json:"procBelow"`
	TwoCols    bool     `json:"twoCols"`
	ColDiff    float64  `json:"colDiff"`
	Parts      int      `json:"parts"`
	TodoDrawn  bool     `json:"todoDrawn"`
}

const d34ReadJS = `(function(){
  var q = function(s){ return document.querySelector(s); };
  var shown = function(e){ return !!e && e.getClientRects().length > 0 && getComputedStyle(e).display !== 'none'; };
  var ps = document.getElementById('pageScroll'), host = document.getElementById('secHost');
  var empty = 0, where = '', zone = 0;
  var note = function(v, w){ if(v > empty){ empty = v; where = w; } };
  var zones = Array.prototype.filter.call(document.querySelectorAll('.zone'), shown);
  zones.forEach(function(z){
    var zb = z.querySelector('.zonebody');
    if(!zb || !shown(zb)) return;
    zone = Math.max(zone, zb.scrollHeight - zb.clientHeight);
    var zr = zb.getBoundingClientRect(), cs = getComputedStyle(zb);
    var top = zr.top + parseFloat(cs.paddingTop), bot = zr.bottom - parseFloat(cs.paddingBottom);
    var kids = Array.prototype.filter.call(zb.children, function(c){ return shown(c) && c.getBoundingClientRect().height > 0.5; })
      .sort(function(a, b){ return a.getBoundingClientRect().top - b.getBoundingClientRect().top; });
    var name = z.className.replace('zone ', '');
    if(!kids.length) return;
    var prev = top;
    kids.forEach(function(k, i){
      var r = k.getBoundingClientRect();
      note(r.top - prev, name + ' gap before child ' + i + ' (' + (k.id || k.className) + ')');
      prev = Math.max(prev, r.bottom);
      if(k.classList.contains('panel')){
        var inner = Array.prototype.filter.call(k.children, function(c){ return shown(c) && c.getBoundingClientRect().height > 0.5; });
        if(inner.length){
          var ib = 0;
          inner.forEach(function(c){ ib = Math.max(ib, c.getBoundingClientRect().bottom); });
          var kcs = getComputedStyle(k);
          note(r.bottom - parseFloat(kcs.paddingBottom) - ib, name + ' slack inside ' + (k.id || k.className));
        }
      }
    });
    note(bot - prev, name + ' gap below last child');
  });
  var hidden = [], parts = {};
  Array.prototype.forEach.call(document.querySelectorAll('[data-part]'), function(e){
    if(e.classList.contains('sec-off')) return;
    var id = e.getAttribute('data-part');
    // To Do the app has switched off (todo.enabled false) is not a hidden part.
    if(id === 'todo' && e.style.display === 'none') return;
    if(shown(e)) parts[id] = true;
    else if(hidden.indexOf(id) < 0) hidden.push(id);
  });
  var proc = q('#processesHost [data-part="processes"]'), todo = q('#todoPanel');
  var procOn = shown(proc), procFirst = true, above = true, below = true;
  var host2 = q('#processesHost');
  if(host2 && todo) procFirst = !!(host2.compareDocumentPosition(todo) & Node.DOCUMENT_POSITION_FOLLOWING);
  if(procOn){
    var pr = proc.getBoundingClientRect();
    if(shown(todo) && !todo.classList.contains('sec-off')) above = pr.bottom <= todo.getBoundingClientRect().top + 1;
    ['#processGroupsPanel', '#boxMemory', '#boxDisks', '#boxNetwork'].forEach(function(s){
      var e = q(s);
      if(shown(e) && e.getBoundingClientRect().bottom > pr.top + 1) below = false;
    });
  }
  var two = zones.length === 2 && Math.abs(zones[0].getBoundingClientRect().top - zones[1].getBoundingClientRect().top) < 2;
  var colDiff = two ? Math.abs(zones[0].getBoundingClientRect().bottom - zones[1].getBoundingClientRect().bottom) : 0;
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
    twoCols: two, colDiff: colDiff, parts: Object.keys(parts).length, todoDrawn: shown(todo)
  };
})()`

func d34Apply(payload string) (d34Page, error) {
	var p d34Page
	if _, err := evalRaw(`window.__bdevSections.apply(` + payload + `)`); err != nil {
		return p, err
	}
	time.Sleep(800 * time.Millisecond)
	err := evalInto(d34ReadJS, &p)
	return p, err
}

// d34Verify returns the problems found in one read, empty when the case passes.
func d34Verify(p d34Page, scrollbar bool) []string {
	var bad []string
	if !p.Fit {
		return []string{"window.__bdevFit is missing"}
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
	}
	return bad
}

func init() {
	checks["d34"] = func(hwnd uintptr, args []string) error {
		snap, err := os.ReadFile(filepath.Join("..", "..", "testdata", "snapshot_example.json"))
		if err != nil {
			return fmt.Errorf("read the frozen example (uicheck runs from the tools folder): %w", err)
		}
		// Switch To Do on in the snapshot, so its panel is drawn when the page takes it from there.
		var snapMap map[string]any
		if err := json.Unmarshal(snap, &snapMap); err != nil {
			return err
		}
		snapMap["todo"] = map[string]any{"enabled": true, "signed_in": true}
		var items []map[string]any
		for k := 1; k <= 14; k++ {
			items = append(items, map[string]any{"Title": fmt.Sprintf("Example task %d", k), "List": "Sample list", "DueDate": "2026-09-29", "Overdue": k%5 == 0})
		}
		snapMap["todo_tasks"] = map[string]any{"items": items}
		if snap, err = json.Marshal(snapMap); err != nil {
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
		payload := func(plain bool, c string, scrollbar bool) string {
			o := d29Opts{Plain: plain, Scrollbar: scrollbar, Status: "ok", Snap: snap}
			switch c {
			case "a", "d":
				o.PlainAll = true
			case "b":
				o.StandardOff = map[string]bool{"turn_ticker": true}
				o.PlainOnly = allBut("turn_ticker")
			case "c":
				o.PlainOnly = map[string]bool{"burn_30min": true}
			}
			return d29Payload(o)
		}
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
				for _, c := range []string{"a", "b", "c", "d"} {
					if c == "c" && !plain {
						continue
					}
					if c == "d" && w != 1280 {
						continue
					}
					for _, sb := range []bool{true, false} {
						if c == "d" && sb {
							continue
						}
						sbName := map[bool]string{true: "on", false: "off"}[sb]
						p, err := d34Apply(payload(plain, c, sb))
						if err != nil {
							return err
						}
						ran++
						name := fmt.Sprintf("d34-%s-%s-%s-%s", tag, mode, c, sbName)
						if _, err := screenshot(hwnd, name); err != nil {
							return err
						}
						bad := d34Verify(p, sb)
						if len(bad) == 0 {
							fmt.Printf("uicheck: d34: PASS %s %s scrollbar %s case %s (parts %d, largest empty %.0f px, fitScroll %v, two columns %v, To Do drawn %v)\n", tag, mode, sbName, c, p.Parts, p.Empty, p.FitScroll, p.TwoCols, p.TodoDrawn)
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
}

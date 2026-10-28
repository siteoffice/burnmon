package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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

func init() {
	checks["d40"] = func(hwnd uintptr, args []string) error {
		fail := func(format string, a ...any) error { return fmt.Errorf("d40: "+format, a...) }
		raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "agentmap_layout_example.json"))
		if err != nil {
			return fmt.Errorf("read the example layout (uicheck runs from the tools folder): %w", err)
		}
		var L map[string]any
		if err := json.Unmarshal(raw, &L); err != nil {
			return err
		}
		// Own starting state: d34 leaves the page in Plain. Leave fake mode, then Standard with every part on.
		if _, err := evalRaw(`(window.__bdevExitFakeMode && window.__bdevExitFakeMode())`); err != nil {
			return fail("leave fake mode: %w", err)
		}
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
		if err := ensureWindowSizeWH(hwnd, 1152, 2048); err != nil {
			return err
		}
		if len(fails) > 0 {
			return fail("%d check(s) failed, first: %s", len(fails), fails[0])
		}
		if ran == 0 {
			return fail("no size ran")
		}
		return nil
	}
}

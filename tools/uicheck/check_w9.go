package main

import (
	"fmt"
	"time"
)

// w9: the Sessions table of burnmon.exe at a 960 x 611 and a 1280 x 860 window (D10 of the Mac
// retest, UI standard rules 11 and 12). With one Codex session (two models, client
// "unassigned", one finding) and with a real owner and client, it measures that "What it was"
// keeps at least 200 px, that no header text is cut, that the last column ends inside the pane,
// that nothing scrolls sideways, and that every cell whose text is cut carries a hover title.
// The fixture goes in through window.__bmSetSessions of the page and is taken out at the end;
// nothing is written to disk.
//
// Args: "before" only captures and reports; anything else (the default, "after") also asserts.
func init() {
	checks["w9"] = func(hwnd uintptr, args []string) error {
		phase := "after"
		if len(args) > 0 {
			phase = args[0]
		}

		setup := `(function(){
  var s = {session_id:'w9-codex-0001', start:'2026-11-22T09:30:00Z', end:'2026-11-22T10:10:00Z',
    title:'Fix the sessions table so that every column keeps its room in a narrow window',
    agent:'codex', surface:'cli', models:{'GPT-5.6 Terra':3,'GPT-5.6 Luna':2}, calls:5, tokens:123400000,
    cost:12.34, cost_sub:12.34, unpriced_tokens:0, long:true, owner:'', client:'unassigned'};
  window.__w9Fixture = s;
  return true;
})()`
		if _, err := evalRaw(setup); err != nil {
			return fmt.Errorf("fixture: %w", err)
		}
		defer evalRaw(`(function(){ window.__bmSetSessions(null); return true; })()`)

		measure := `(function(){
  document.querySelector('.tab[data-t="sessions"]').click();
  var tbl = document.getElementById('t_sessions');
  var box = tbl.parentNode;
  var r = {innerWidth: window.innerWidth, wrapClient: box.clientWidth, boxScroll: box.scrollWidth,
    docScroll: document.documentElement.scrollWidth, cutHeaders: [], cutNoTitle: [], numNoTitle: [], what: 0, lastRight: 0, boxRight: 0, shown: []};
  var ths = Array.prototype.filter.call(tbl.querySelectorAll('thead th'), function(th){ return th.offsetParent !== null; });
  var boxRect = box.getBoundingClientRect();
  r.boxRight = boxRect.right;
  ths.forEach(function(th, i){
    var rc = th.getBoundingClientRect();
    r.shown.push(th.textContent);
    if(th.textContent === 'What it was') r.what = rc.width;
    if(th.scrollWidth > th.clientWidth) r.cutHeaders.push(th.textContent + ' ' + th.scrollWidth + '>' + th.clientWidth);
    r.lastRight = Math.max(r.lastRight, rc.right);
  });
  Array.prototype.forEach.call(tbl.querySelectorAll('tbody tr:not(.finding-row) td'), function(td){
    if(td.offsetParent === null) return;
    var inner = td.scrollWidth > td.clientWidth;
    Array.prototype.forEach.call(td.children, function(c){ if(c.scrollWidth > c.clientWidth) inner = true; });
    if(inner && !td.getAttribute('title') && !td.getAttribute('data-full')) r.cutNoTitle.push(td.textContent);
  });
  Array.prototype.forEach.call(tbl.querySelectorAll('tbody tr:not(.finding-row)'), function(tr){
    [4,5,6,7].forEach(function(i){ var td = tr.children[i]; if(td && !td.getAttribute('title')) r.numNoTitle.push(td.textContent); });
  });
  return r;
})()`
		type result struct {
			InnerWidth int      `json:"innerWidth"`
			WrapClient int      `json:"wrapClient"`
			BoxScroll  int      `json:"boxScroll"`
			DocScroll  int      `json:"docScroll"`
			CutHeaders []string `json:"cutHeaders"`
			CutNoTitle []string `json:"cutNoTitle"`
			NumNoTitle []string `json:"numNoTitle"`
			What       float64  `json:"what"`
			LastRight  float64  `json:"lastRight"`
			BoxRight   float64  `json:"boxRight"`
			Shown      []string `json:"shown"`
		}

		var failures []string
		cases := []struct {
			name         string
			w, h         int32
			owner        string
			client       string
			wantClientCol bool
			large        bool // large values: 1,234,567 calls and 9,876.54 EUR (L4)
		}{
			{"960 no client", 960, 611, "", "unassigned", false, false},
			{"1280 no client", 1280, 860, "", "unassigned", false, false},
			{"960 owner and client", 960, 611, "acme-team", "acme-client", false, false},
			{"1280 owner and client", 1280, 860, "acme-team", "acme-client", true, false},
			{"960 large values", 960, 611, "", "unassigned", false, true},
		}
		for _, c := range cases {
			if err := ensureWindowSizeWH(hwnd, c.w, c.h); err != nil {
				return fmt.Errorf("%s: window size: %w", c.name, err)
			}
			time.Sleep(600 * time.Millisecond)
			put := fmt.Sprintf(`(function(){ var s = JSON.parse(JSON.stringify(window.__w9Fixture)); s.owner=%q; s.client=%q;
  if(%t){ s.calls=1234567; s.tokens=9876543210; s.cost=9876.54; s.cost_sub=9876.54; }
  window.__bmSetSessions([s], {'w9-codex-0001': [{Kind:'context-runway'}]}); return true; })()`, c.owner, c.client, c.large)
			if _, err := evalRaw(put); err != nil {
				return fmt.Errorf("%s: set fixture: %w", c.name, err)
			}
			var r result
			if err := evalInto(measure, &r); err != nil {
				return fmt.Errorf("%s: measure: %w", c.name, err)
			}
			time.Sleep(400 * time.Millisecond)
			fmt.Printf("uicheck: w9 %s: measured at %d x %d on Windows (WebView2), inner width %d, What it was %.0f px, last column right %.0f, pane right %.0f, box scroll %d client %d, columns %v, cut headers %v, cut cells without a title %v\n",
				c.name, c.w, c.h, r.InnerWidth, r.What, r.LastRight, r.BoxRight, r.BoxScroll, r.WrapClient, r.Shown, r.CutHeaders, r.CutNoTitle)
			if _, err := screenshot(hwnd, fmt.Sprintf("w9-%s-%dx%d", phase, c.w, c.h)); err != nil {
				fmt.Printf("uicheck: screenshot failed (non-fatal, best effort only): %v\n", err)
			}
			if c.wantClientCol != containsStr(r.Shown, "Client") {
				failures = append(failures, fmt.Sprintf("%s: Client column shown = %v, want %v", c.name, !c.wantClientCol, c.wantClientCol))
			}
			if r.What < 200 {
				failures = append(failures, fmt.Sprintf("%s: What it was is %.0f px, want 200 or more", c.name, r.What))
			}
			if len(r.CutHeaders) > 0 {
				failures = append(failures, fmt.Sprintf("%s: header text is cut: %v", c.name, r.CutHeaders))
			}
			if r.LastRight > r.BoxRight+0.5 {
				failures = append(failures, fmt.Sprintf("%s: the last column ends at %.0f, outside the pane at %.0f", c.name, r.LastRight, r.BoxRight))
			}
			if r.BoxScroll > r.WrapClient || r.DocScroll > r.InnerWidth {
				failures = append(failures, fmt.Sprintf("%s: horizontal scroll (box %d > %d, page %d > %d)", c.name, r.BoxScroll, r.WrapClient, r.DocScroll, r.InnerWidth))
			}
			if len(r.NumNoTitle) > 0 {
				failures = append(failures, fmt.Sprintf("%s: number cells with no hover title: %v", c.name, r.NumNoTitle))
			}
			if len(r.CutNoTitle) > 0 {
				failures = append(failures, fmt.Sprintf("%s: cut cells with no hover title: %v", c.name, r.CutNoTitle))
			}
		}
		if phase == "before" {
			return nil
		}
		if len(failures) > 0 {
			return fmt.Errorf("w9: %d problem(s): %v", len(failures), failures)
		}
		return nil
	}
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

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
	if err := ensureWindowSizeWH(hwnd, 1920, 1080); err != nil {
		return err
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

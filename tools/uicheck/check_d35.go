package main

import (
	"encoding/json"
	"fmt"
	"math"
	"time"
)

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

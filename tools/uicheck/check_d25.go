package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// d25: the Station's click popup is the compact card
// (02_roadmap\2026-10-03_station_ui_and_backlog.md, item A). A click on an
// agent in P, O or I opens the latest turn as a small card next to the click,
// not the 480 px modal in the middle of the window. A long value (a Cowork
// working directory on one unbroken line was the original bug) wraps inside
// the card, and the card, with everything in it, stays inside the window.
// Fake mode feeds three sessions; the turn detail is injected through
// __bdevTurnDetailResolve with long synthetic paths, so nothing real is read.
// Each view is checked on the site and space Station themes, then again with
// the page in light mode (L). Screenshots: d25-<view>-<theme>.
const (
	vkI = 0x49
	// d25MaxCardW is the widest a "compact card" may be in CSS px: the hover
	// card measures about 230 px, so 340 leaves room for the turn table.
	d25MaxCardW = 340
)

type d25Card struct {
	Shown    bool    `json:"shown"`
	Rendered bool    `json:"rendered"`
	Left     float64 `json:"left"`
	Top      float64 `json:"top"`
	Right    float64 `json:"right"`
	Bottom   float64 `json:"bottom"`
	W        float64 `json:"w"`
	ScrollW  float64 `json:"scrollW"`
	ClientW  float64 `json:"clientW"`
	Clipped  int     `json:"clipped"`
	IW       float64 `json:"iw"`
	IH       float64 `json:"ih"`
}

// d25ClickJS clicks fake-st-1 at (ax, ay) and answers the turn detail request
// with long values: a path with no break opportunity, a long model name and
// a long file list entry. The pending promise exists as soon as the click
// returns, so this resolve wins over the Go side's own answer.
const d25ClickJS = `(function(ax, ay){
  var sep = String.fromCharCode(92), seg = [];
  for(var i = 0; i < 14; i++) seg.push('local-agent-mode-sessions-segment-' + i);
  var long = 'C:' + sep + seg.join(sep);
  window.__bdevStationClick('fake-st-1', null, {x: ax, y: ay});
  window.__bdevTurnDetailResolve('fake-st-1:4', {
    turn: 4, session_id: 'fake-st-1', model: 'a-very-long-model-name-that-has-no-break-' + 'x'.repeat(60),
    at: new Date().toISOString(), has_gap: true, gap_seconds: 12, cost: 0.12,
    fresh: 1200, cache_write: 0, cache_read: 5000, output: 300, findings: [],
    tool_calls: [{tool: 'Read', path: long, input_bytes: 100, result_bytes: 2000}],
    files: [long]
  });
  return true;
})`

const d25ReadJS = `(function(){
  var p = document.getElementById('turnPopup'), ov = document.getElementById('turnPopupOverlay');
  var r = p.getBoundingClientRect(), clipped = 0;
  p.querySelectorAll('*').forEach(function(el){
    var b = el.getBoundingClientRect();
    if(!b.width && !b.height) return;
    if(b.left < r.left - 1 || b.right > r.right + 1 || b.top < r.top - 1 || b.bottom > r.bottom + 1) clipped++;
  });
  return {shown: !ov.classList.contains('hidden'), rendered: p.textContent.indexOf('Turn 4') >= 0 && p.textContent.indexOf('Token classes') >= 0,
    left: r.left, top: r.top, right: r.right, bottom: r.bottom, w: r.width, scrollW: p.scrollWidth, clientW: p.clientWidth,
    clipped: clipped, iw: innerWidth, ih: innerHeight};
})()`

func d25Read() (d25Card, error) {
	var c d25Card
	err := evalInto(d25ReadJS, &c)
	return c, err
}

// d25Click clicks the agent at (ax, ay) and polls until the card is rendered.
func d25Click(ax, ay int) (d25Card, error) {
	if _, err := evalRaw(fmt.Sprintf("%s(%d, %d)", d25ClickJS, ax, ay)); err != nil {
		return d25Card{}, err
	}
	var c d25Card
	var err error
	for deadline := time.Now().Add(4 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if c, err = d25Read(); err != nil {
			return c, err
		}
		if c.Shown && c.Rendered {
			break
		}
	}
	return c, nil
}

// d25Problems lists what is wrong with a card opened by a click at (ax, ay).
func d25Problems(c d25Card, ax, ay float64) []string {
	var p []string
	if !c.Shown || !c.Rendered {
		return []string{"card not shown or not rendered"}
	}
	if c.Left < 0 || c.Top < 0 || c.Right > c.IW || c.Bottom > c.IH {
		p = append(p, "card outside the window")
	}
	if c.W > d25MaxCardW {
		p = append(p, fmt.Sprintf("card %.0f px wide, compact is at most %d", c.W, d25MaxCardW))
	}
	if c.Clipped > 0 || c.ScrollW > c.ClientW+1 {
		p = append(p, fmt.Sprintf("content clipped: %d elements outside the card, scrollWidth %.0f > clientWidth %.0f", c.Clipped, c.ScrollW, c.ClientW))
	}
	// Next to the click: 14 px right and below it, pushed back only as far as
	// the window edge needs (a tall card slides up, a click near the right
	// edge slides it left).
	wantLeft := math.Max(4, math.Min(ax+14, c.IW-c.W-4))
	wantTop := math.Max(4, math.Min(ay+14, c.IH-(c.Bottom-c.Top)-4))
	if math.Abs(c.Left-wantLeft) > 2 || math.Abs(c.Top-wantTop) > 2 {
		p = append(p, fmt.Sprintf("card at (%.0f,%.0f), want (%.0f,%.0f) for the click at (%.0f,%.0f)", c.Left, c.Top, wantLeft, wantTop, ax, ay))
	}
	return p
}

func caseD25(hwnd uintptr, args []string) error {
	if err := ensureWindowSizeWH(hwnd, 1600, 900); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	defer evalRaw(`(window.__bdevExitFakeMode && window.__bdevExitFakeMode())`)
	if _, err := evalRaw(d23FakeJS); err != nil {
		return fmt.Errorf("d25: fake paint: %w", err)
	}
	bringToFront(hwnd)
	if err := clickSelectorAt(hwnd, ".logo", 0.5, 0.5); err != nil {
		fmt.Println("uicheck: d25: could not click .logo to establish focus:", err)
	}
	var errs []string
	fail := func(where string, probs []string) {
		for _, m := range probs {
			errs = append(errs, fmt.Sprintf("%s: %s", where, m))
		}
	}
	// The page leaves light mode whatever happens, the stored choice included.
	defer func() {
		if s, err := d24Read(); err == nil && s.Attr == "light" {
			_ = pressKey(vkL)
			time.Sleep(300 * time.Millisecond)
		}
	}()

	// checkCard runs the two clicks of one case: next to the middle of the
	// window, then in the bottom right corner where the card must be pushed back in.
	checkCard := func(where string) error {
		c, err := d25Click(700, 300)
		if err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		fmt.Printf("uicheck: d25 %s middle: %+v\n", where, c)
		fail(where+" middle click", d25Problems(c, 700, 300))
		if err := d23Shot(hwnd, "d25-"+strings.ReplaceAll(where, " ", "-")); err != nil {
			return err
		}
		c, err = d25Click(1595, 895)
		if err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		fmt.Printf("uicheck: d25 %s corner: %+v\n", where, c)
		fail(where+" corner click", d25Problems(c, 1595, 895))
		_, err = evalRaw(`document.getElementById('turnPopupClose').click()`)
		return err
	}
	openView := func(name string, vk uint16, ok func(d23State) bool) bool {
		if err := pressKey(vk); err != nil {
			errs = append(errs, fmt.Sprintf("%s: press: %v", name, err))
			return false
		}
		s, good, err := d23Wait(ok)
		if err != nil || !good {
			errs = append(errs, fmt.Sprintf("%s did not open: %+v %v", name, s, err))
			return false
		}
		time.Sleep(1500 * time.Millisecond) // the first frame, so the shot is not blank
		return true
	}
	views := []struct {
		name string
		vk   uint16
		ok   func(d23State) bool
	}{
		{"P", vkP, func(s d23State) bool { return s.Overlay && s.Lit > 500 }},
		{"O", vkO, func(s d23State) bool { return s.Panel && s.Lit > 500 }},
		{"I", vkI, func(s d23State) bool { return s.Panel && s.Lit > 500 }},
	}
	runViews := func(page string, themes []string) error {
		for _, v := range views {
			if !openView(v.name, v.vk, v.ok) {
				continue
			}
			for i, th := range themes {
				if i > 0 {
					if err := pressKey(vkT); err != nil {
						return err
					}
					time.Sleep(600 * time.Millisecond)
				}
				if err := checkCard(strings.TrimSpace(v.name + " " + th + " " + page)); err != nil {
					return err
				}
			}
			if len(themes) > 1 { // back to site for the next view
				if err := pressKey(vkT); err != nil {
					return err
				}
				time.Sleep(400 * time.Millisecond)
			}
			if err := pressKey(v.vk); err != nil { // the same key closes the view
				return err
			}
			if _, good, err := d23Wait(func(s d23State) bool { return !s.Overlay && !s.Panel }); err != nil || !good {
				errs = append(errs, fmt.Sprintf("%s did not close on its own key", v.name))
				_ = pressKey(vkEscape)
			}
		}
		return nil
	}
	if err := runViews("dark page", []string{"site", "space"}); err != nil {
		return err
	}
	if err := pressKey(vkL); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)
	if err := runViews("light page", []string{"site"}); err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("d25: %d problem(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}
	fmt.Println("uicheck: d25: the Station click card is compact, next to the click, wraps long values and stays inside the window in P, O and I, both Station themes and the light page")
	return nil
}

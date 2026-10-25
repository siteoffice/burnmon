package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// d34: the Overall scrollbar switch both ways (B2). Driven by
// window.__bdevSections.apply with the frozen example. It proves:
//   - scrollbar on: the page scrolls (overflow-y auto), nothing is hidden,
//     no note, and the System chart keeps the height it has in that layout;
//   - scrollbar off: the page does not scroll and nothing overflows (body,
//     page area, sections host, any .zonebody); the System chart shrank to its
//     floor (60 percent of its natural height, at least 60 px) before any
//     part was hidden; the parts hidden are the first ones of the fixed drop
//     order and the note counts them;
//   - a window that grows again brings every hidden part back;
//   - Plain with all its parts on behaves the same way at 1024x480.
//
// To Do is not part of the drop-order prefix: uicheck runs with Microsoft To
// Do disabled, so its panel is never on screen and never counts as hidden.
//
// Screenshots: d34-standard-scroll-on, d34-standard-scroll-off,
// d34-plain-scroll-on, d34-plain-scroll-off.

// d34Order is FIT_OFF_ORDER without todo, which uicheck never shows.
var d34Order = []string{"process_groups", "processes", "heatmap", "activity", "sessions", "git", "outcomes", "waits", "agents_map", "burn", "turn_ticker", "vendor_strip", "system", "burn_30min"}

type d34Page struct {
	OverflowY string   `json:"overflowY"`
	Scrolls   bool     `json:"scrolls"`
	Note      string   `json:"note"`
	BodyOver  float64  `json:"bodyOver"`
	PageOver  float64  `json:"pageOver"`
	HostOver  float64  `json:"hostOver"`
	ZoneOver  float64  `json:"zoneOver"`
	Fit       bool     `json:"fit"`
	Scroll    bool     `json:"scroll"`
	Natural   float64  `json:"natural"`
	Floor     float64  `json:"floor"`
	HistH     float64  `json:"histH"`
	AtHide    float64  `json:"atHide"`
	Hidden    []string `json:"hidden"`
	Dropped   []string `json:"dropped"`
}

const d34ReadJS = `(function(){
  var ps = document.getElementById('pageScroll'), host = document.getElementById('secHost');
  var zone = 0;
  Array.prototype.forEach.call(document.querySelectorAll('.zonebody'), function(z){
    if(z.getClientRects().length > 0) zone = Math.max(zone, z.scrollHeight - z.clientHeight);
  });
  var dropped = [];
  Array.prototype.forEach.call(document.querySelectorAll('[data-part].fit-dropped'), function(e){
    var id = e.getAttribute('data-part');
    if(dropped.indexOf(id) < 0) dropped.push(id);
  });
  var f = window.__bdevFit, has = !!f;
  f = f || {};
  return {
    overflowY: getComputedStyle(ps).overflowY,
    scrolls: ps.scrollHeight > ps.clientHeight + 1,
    note: document.getElementById('secCutNote').textContent,
    bodyOver: document.body.scrollHeight - document.body.clientHeight,
    pageOver: ps.scrollHeight - ps.clientHeight,
    hostOver: host.scrollHeight - host.clientHeight,
    zoneOver: zone,
    fit: has, scroll: !!f.scroll, natural: f.histNatural || 0, floor: f.histFloor || 0, histH: f.histH || 0,
    atHide: f.histAtHide || 0, hidden: f.hidden || [], dropped: dropped
  };
})()`

func d34Fail(format string, args ...any) error { return fmt.Errorf("d34: "+format, args...) }

func d34Apply(payload string) (d34Page, error) {
	var p d34Page
	if _, err := evalRaw(`window.__bdevSections.apply(` + payload + `)`); err != nil {
		return p, err
	}
	time.Sleep(800 * time.Millisecond)
	err := evalInto(d34ReadJS, &p)
	return p, err
}

func d34NoOverflow(tag string, p d34Page) error {
	if p.BodyOver > 1 || p.PageOver > 1 || p.HostOver > 1 || p.ZoneOver > 1 {
		return d34Fail("%s overflows: body %.0f, page area %.0f, host %.0f, zone %.0f", tag, p.BodyOver, p.PageOver, p.HostOver, p.ZoneOver)
	}
	return nil
}

func d34NoteWant(n int) string {
	if n == 0 {
		return ""
	}
	if n == 1 {
		return "1 section hidden, the window is too small"
	}
	return fmt.Sprintf("%d sections hidden, the window is too small", n)
}

// d34On checks the scrollbar-on state: scrolls, nothing hidden, no note, chart natural.
func d34On(tag string, p d34Page) error {
	if !p.Fit {
		return d34Fail("%s: window.__bdevFit is missing", tag)
	}
	if !p.Scroll || p.OverflowY != "auto" {
		return d34Fail("%s: scrollbar on, but scroll %v and overflow-y %q", tag, p.Scroll, p.OverflowY)
	}
	if !p.Scrolls {
		return d34Fail("%s: scrollbar on, but the page does not scroll", tag)
	}
	if p.Note != "" || len(p.Hidden) != 0 {
		return d34Fail("%s: scrollbar on hides %v with note %q", tag, p.Hidden, p.Note)
	}
	if p.Natural <= 0 || p.HistH < p.Natural-1 || p.HistH > p.Natural+1 {
		return d34Fail("%s: System chart is %.1f px, natural %.1f px", tag, p.HistH, p.Natural)
	}
	return nil
}

// d34Off checks the scrollbar-off state at a window too small for everything.
func d34Off(tag string, p d34Page, wantHidden bool) error {
	if !p.Fit {
		return d34Fail("%s: window.__bdevFit is missing", tag)
	}
	if p.Scroll || p.OverflowY != "hidden" || p.Scrolls {
		return d34Fail("%s: scrollbar off, but scroll %v, overflow-y %q, scrolls %v", tag, p.Scroll, p.OverflowY, p.Scrolls)
	}
	if err := d34NoOverflow(tag, p); err != nil {
		return err
	}
	if wantHidden && len(p.Hidden) == 0 {
		return d34Fail("%s: nothing was hidden, the window should be too small", tag)
	}
	if len(p.Hidden) > len(d34Order) {
		return d34Fail("%s: hid %v, more than the drop order holds", tag, p.Hidden)
	}
	for i, id := range p.Hidden {
		if id != d34Order[i] {
			return d34Fail("%s: hidden %v is not a prefix of the drop order %v", tag, p.Hidden, d34Order)
		}
	}
	if len(p.Dropped) != len(p.Hidden) {
		return d34Fail("%s: hidden %v but %v carry fit-dropped", tag, p.Hidden, p.Dropped)
	}
	if want := d34NoteWant(len(p.Hidden)); p.Note != want {
		return d34Fail("%s: note %q, want %q", tag, p.Note, want)
	}
	if p.Natural <= 0 || p.Floor < 0.6*p.Natural-1 || p.Floor < 59 {
		return d34Fail("%s: floor %.1f px, natural %.1f px, want at least 60 percent and 60 px", tag, p.Floor, p.Natural)
	}
	systemHidden := false
	for _, id := range p.Hidden {
		if id == "system" {
			systemHidden = true
		}
	}
	// A hidden System panel has no chart height left to hold to the floor.
	if !systemHidden && p.HistH < p.Floor-1 {
		return d34Fail("%s: System chart %.1f px is below its floor %.1f px", tag, p.HistH, p.Floor)
	}
	if len(p.Hidden) > 0 && p.AtHide > p.Floor+2 {
		return d34Fail("%s: a part was hidden while the System chart was %.1f px, floor %.1f px (it must shrink first)", tag, p.AtHide, p.Floor)
	}
	return nil
}

func init() {
	checks["d34"] = func(hwnd uintptr, args []string) error {
		snap, err := os.ReadFile(filepath.Join("..", "..", "testdata", "snapshot_example.json"))
		if err != nil {
			return fmt.Errorf("read the frozen example (uicheck runs from the tools folder): %w", err)
		}
		std := func(scrollbar bool) string {
			return d29Payload(d29Opts{Scrollbar: scrollbar, Status: "ok", Snap: snap})
		}
		plain := func(scrollbar bool) string {
			return d29Payload(d29Opts{Plain: true, PlainAll: true, Scrollbar: scrollbar, Status: "ok", Snap: snap})
		}
		size := func(w, h int32) error {
			if err := ensureWindowSizeWH(hwnd, w, h); err != nil {
				return err
			}
			time.Sleep(500 * time.Millisecond)
			return nil
		}

		if err := size(1024, 600); err != nil {
			return err
		}
		p, err := d34Apply(std(true))
		if err != nil {
			return err
		}
		if err := d34On("standard 1024x600 scrollbar on", p); err != nil {
			return err
		}
		if _, err := screenshot(hwnd, "d34-standard-scroll-on"); err != nil {
			return err
		}
		p, err = d34Apply(std(false))
		if err != nil {
			return err
		}
		if err := d34Off("standard 1024x600 scrollbar off", p, true); err != nil {
			return err
		}
		if _, err := screenshot(hwnd, "d34-standard-scroll-off"); err != nil {
			return err
		}

		// Review focus 4: the window grows again, every hidden part comes back.
		if err := size(1152, 2048); err != nil {
			return err
		}
		p, err = d34Apply(std(false))
		if err != nil {
			return err
		}
		if err := d34Off("standard 1152x2048 scrollbar off", p, false); err != nil {
			return err
		}
		if len(p.Hidden) != 0 || p.Note != "" || len(p.Dropped) != 0 {
			return d34Fail("after growing to 1152x2048 hidden %v, dropped %v, note %q", p.Hidden, p.Dropped, p.Note)
		}

		if err := size(1024, 480); err != nil {
			return err
		}
		p, err = d34Apply(plain(true))
		if err != nil {
			return err
		}
		if err := d34On("plain 1024x480 scrollbar on", p); err != nil {
			return err
		}
		if _, err := screenshot(hwnd, "d34-plain-scroll-on"); err != nil {
			return err
		}
		p, err = d34Apply(plain(false))
		if err != nil {
			return err
		}
		if err := d34Off("plain 1024x480 scrollbar off", p, true); err != nil {
			return err
		}
		if !strings.Contains(p.Note, "hidden, the window is too small") {
			return d34Fail("plain 1024x480 scrollbar off: note %q", p.Note)
		}
		if _, err := screenshot(hwnd, "d34-plain-scroll-off"); err != nil {
			return err
		}
		return size(1152, 2048)
	}
}

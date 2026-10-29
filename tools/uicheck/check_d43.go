package main

import (
	"fmt"
	"strings"
	"time"
)

// d43: the gear Controls text spans the gear (B7.1, spec 2026-10-28, item 3).
// At 1280x800 and 1920x1080 (a size the screen cannot hold prints SKIPPED and
// is not a pass), Standard, with invented "needs ..." lines for all five
// controls, a real click opens the gear. Asserted: the Controls explanation
// and every non-empty "needs ..." line are within 2 px of the width of the
// History block (#histAdmin, which spans the gear grid); each is wider than
// 320 px when the History block is; the dialog is narrower than the window
// minus 20 px; no text is clipped (scrollWidth within 1 px of clientWidth).
// Screenshot d43-gear-<size>; Esc closes the gear.

const d43ReadJS = `(function(){
  var w = function(e){ return e ? e.getBoundingClientRect().width : 0; };
  var clip = function(e){ return e.scrollWidth <= e.clientWidth + 1; };
  var hist = document.getElementById('histAdmin');
  var texts = [], e0 = document.querySelector('#secControls > .ctlwhy');
  if(e0) texts.push({name: 'explanation', w: w(e0), clip: clip(e0)});
  Array.prototype.forEach.call(document.querySelectorAll('#secControls .ctlwhy[data-ctl-why]'), function(e){
    if(!(e.textContent || '').trim()) return;
    texts.push({name: e.getAttribute('data-ctl-why'), w: w(e), clip: clip(e)});
  });
  return {open: document.getElementById('secSettings').classList.contains('open'), hist: w(hist), dialog: w(document.getElementById('secSettings')),
    win: window.innerWidth, texts: texts};
})()`

type d43Text struct {
	Name string  `json:"name"`
	W    float64 `json:"w"`
	Clip bool    `json:"clip"`
}

type d43Read struct {
	Open   bool      `json:"open"`
	Hist   float64   `json:"hist"`
	Dialog float64   `json:"dialog"`
	Win    float64   `json:"win"`
	Texts  []d43Text `json:"texts"`
}

func init() {
	checks["d43"] = func(hwnd uintptr, args []string) error {
		defer evalRaw(`(window.__bdevExitFakeMode && window.__bdevExitFakeMode())`)
		extra := map[string]any{"controls": map[string]any{
			"on": map[string]bool{}, "ready": map[string]bool{},
			"why": map[string]string{
				"stop": "needs claude_path in burnmon.json", "attach": "needs agent_view in burnmon.json",
				"approve": "needs python on PATH", "send_back": "needs python on PATH", "close_sprint": "needs foreman_root",
			},
			"verdicts": map[string]any{}, "foreman": true}}
		payload, err := d41Payload(d29Opts{Status: "ok"}, extra)
		if err != nil {
			return err
		}
		var fails []string
		ran := 0
		for _, sz := range [][2]int32{{1280, 800}, {1920, 1080}} {
			tag := fmt.Sprintf("%dx%d", sz[0], sz[1])
			if err := ensureWindowSizeWH(hwnd, sz[0], sz[1]); err != nil {
				return err
			}
			if windowCapped {
				fmt.Printf("uicheck: d43: SKIPPED %s, this screen is too small\n", tag)
				continue
			}
			time.Sleep(500 * time.Millisecond)
			if _, err := evalRaw(`(document.getElementById('secSettings').classList.remove('open'), window.__bdevSections.apply(` + payload + `), true)`); err != nil {
				return err
			}
			time.Sleep(500 * time.Millisecond)
			if err := clickSelector(hwnd, "#secGear"); err != nil {
				return err
			}
			time.Sleep(500 * time.Millisecond)
			var r d43Read
			err := evalInto(d43ReadJS, &r)
			_, shotErr := screenshot(hwnd, "d43-gear-"+tag)
			bringToFront(hwnd)
			_ = pressKey(vkEscape)
			time.Sleep(300 * time.Millisecond)
			if err != nil {
				return err
			}
			if shotErr != nil {
				return shotErr
			}
			var bad []string
			if !r.Open {
				bad = append(bad, "the gear did not open")
			}
			if len(r.Texts) != 6 {
				bad = append(bad, fmt.Sprintf("%d text elements, want 6 (explanation and five needs lines)", len(r.Texts)))
			}
			var ws []string
			for _, t := range r.Texts {
				ws = append(ws, fmt.Sprintf("%s %.0f", t.Name, t.W))
				if t.W < r.Hist-2 || t.W > r.Hist+2 {
					bad = append(bad, fmt.Sprintf("%s is %.1f px wide, the History block %.1f px", t.Name, t.W, r.Hist))
				}
				if r.Hist > 320 && t.W <= 320 {
					bad = append(bad, fmt.Sprintf("%s is %.1f px wide, not over 320", t.Name, t.W))
				}
				if !t.Clip {
					bad = append(bad, t.Name+" is clipped")
				}
			}
			if r.Dialog >= r.Win-20 {
				bad = append(bad, fmt.Sprintf("dialog %.0f px is not narrower than window %.0f minus 20", r.Dialog, r.Win))
			}
			ran++
			if len(bad) == 0 {
				fmt.Printf("uicheck: d43: PASS %s (grid width %.0f px, dialog %.0f px, text widths: %s)\n", tag, r.Hist, r.Dialog, strings.Join(ws, ", "))
			} else {
				msg := fmt.Sprintf("%s: %s", tag, strings.Join(bad, "; "))
				fmt.Println("uicheck: d43: FAIL " + msg)
				fails = append(fails, msg)
			}
		}
		if err := ensureWindowSizeWH(hwnd, 1152, 2048); err != nil {
			return err
		}
		if len(fails) > 0 {
			return fmt.Errorf("d43: %d case(s) failed, first: %s", len(fails), fails[0])
		}
		if ran == 0 {
			return fmt.Errorf("d43: no size ran")
		}
		return nil
	}
}

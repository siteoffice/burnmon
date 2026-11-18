package main

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The lamp group drives the lamp window of burnmon-dev.exe (`burnmon-dev.exe
// lamp --demo`, started by scripts\uicheck-lamp.ps1 with BURNMON_LAMP_UICHECK=1,
// its own eval port 9336). It freezes the page's own poll, hands the page a
// fixed state per mood in both themes through window.lampRender, and checks
// what the lamp spec asks of the page: the pill and the opened pill have
// their size, nothing scrolls, cut text has a hover title and the text has
// contrast. The lamp needs no window handle: the eval connection proves it
// runs. Nothing here changes a file.

// evalAddrLamp is the lamp's dev eval server (cmd\burnmon-dev\lamp_window_windows.go),
// a fourth port beside 9333, 9334 and 9335.
const evalAddrLamp = "127.0.0.1:9336"

// lampNames holds every name that targets the lamp window.
var lampNames = map[string]bool{}

func isLampCheck(name string) bool { return lampNames[name] }

// lampMoods are the eight moods of the lamp, in the order of its spec.
var lampMoods = []string{"off", "asleep", "working", "your_turn", "blocked", "bored", "sweating", "done"}

// lampSliver moods show a thin line when the pill is closed.
var lampSliver = map[string]bool{"asleep": true, "working": true, "off": true}

// lampRowsOf is how many rows the fixture of a mood carries.
var lampRowsOf = map[string]int{"your_turn": 1, "blocked": 2, "bored": 3}

type lampFixtureRow struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Word      string `json:"word"`
	Title     string `json:"title"`
	TitleFull string `json:"title_full"`
	Wait      string `json:"wait"`
	WaitExact string `json:"wait_exact"`
}

// lampFixture is the payload lampRender takes for one mood and theme. The
// names are invented; the longest line and title are cut at the pill width on
// purpose, so the hover title is tested.
func lampFixture(mood, theme string) string {
	heads := map[string]string{
		"off": "BurnMon is not running", "asleep": "No agents running", "working": "3 agents at work",
		"your_turn": "1 your turn", "blocked": "2 blocked, 1 your turn", "bored": "2 blocked, 1 your turn",
		"sweating": "24k tokens a minute", "done": "4 ready to close",
	}
	lines := map[string]string{
		"your_turn": "Your turn. An agent cannot press Enter for you.",
		"blocked":   "Blocked. Nobody moves until you do, not even me.",
		"bored":     "I built a wall while I waited for you to come back.",
		"working":   "All quiet. Go and get yourself a coffee, you are free.",
	}
	all := []lampFixtureRow{
		{ID: "fx-1", Kind: "gate", Word: "gate", Title: "Ship the fixture release", TitleFull: "Ship the fixture release",
			Wait: "12 min", WaitExact: "12 min 4 s"},
		{ID: "fx-2", Kind: "prompt", Word: "prompt", Title: "Allow the fixture build to run its long test suite", TitleFull: "Allow the fixture build to run its long test suite on the fixture repo",
			Wait: "3 min", WaitExact: "3 min 9 s"},
		{ID: "fx-3", Kind: "ready", Word: "ready", Title: "Fixture plan is ready", TitleFull: "Fixture plan is ready",
			Wait: "45 s", WaitExact: "45 s"},
	}
	n := lampRowsOf[mood]
	rows := []lampFixtureRow{}
	rows = append(rows, all[:n]...)
	st := map[string]any{
		"mood": mood, "headline": heads[mood], "line": lines[mood], "rows": rows,
		"counts": map[string]int{}, "agents": 3, "longest_wait_min": 0, "wall_bricks": 0,
		"wall_over": false, "burn_rate": 24000.0, "demo": false,
		"cost_5min": map[string]any{"usd": 1.25, "eur": 1.15, "label": "at API prices"},
	}
	if mood == "bored" {
		st["longest_wait_min"], st["wall_bricks"] = 15, 15
	}
	b, _ := json.Marshal(map[string]any{
		// No labels: the page falls back to its English words.
		"state": st, "theme": theme,
	})
	return string(b)
}

// lampWantSize is the CSS size the window must have: a 120 by 6 line for a
// sleepy mood, a 320 by 40 pill, and the opened pill 440 wide and 40 high
// plus 28 for every row.
func lampWantSize(mood string, open bool, rows int) (w, h float64) {
	if !open {
		if lampSliver[mood] {
			return 120, 6
		}
		return 320, 40
	}
	return 440, float64(40 + 28*rows)
}

type lampColor struct {
	Name string `json:"name"`
	FG   string `json:"fg"`
	BG   string `json:"bg"`
}

type lampRead struct {
	Mood       string      `json:"mood"`
	Theme      string      `json:"theme"`
	Open       bool        `json:"open"`
	Rows       int         `json:"rows"`
	InnerW     float64     `json:"innerW"`
	InnerH     float64     `json:"innerH"`
	DocW       float64     `json:"docW"`
	DocH       float64     `json:"docH"`
	CutNoTitle []string    `json:"cutNoTitle"`
	Colors     []lampColor `json:"colors"`
}

var lampRGB = regexp.MustCompile(`^rgb\((\d+), (\d+), (\d+)\)$`)

func lampLum(c string) (float64, error) {
	m := lampRGB.FindStringSubmatch(strings.TrimSpace(c))
	if m == nil {
		return 0, fmt.Errorf("cannot read the colour %q", c)
	}
	var l float64
	for i, k := range []float64{0.2126, 0.7152, 0.0722} {
		v, _ := strconv.Atoi(m[i+1])
		s := float64(v) / 255
		if s <= 0.03928 {
			s /= 12.92
		} else {
			s = math.Pow((s+0.055)/1.055, 2.4)
		}
		l += k * s
	}
	return l, nil
}

// lampContrast is the WCAG contrast ratio of two computed rgb() colours.
func lampContrast(fg, bg string) (float64, error) {
	a, err := lampLum(fg)
	if err != nil {
		return 0, err
	}
	b, err := lampLum(bg)
	if err != nil {
		return 0, err
	}
	if a < b {
		a, b = b, a
	}
	return (a + 0.05) / (b + 0.05), nil
}

// lampProblems lists what is wrong with one read: the size, a scrollbar,
// cut text without a hover title and text without contrast (4.5 to 1).
func lampProblems(r lampRead) []string {
	var p []string
	where := fmt.Sprintf("%s/%s open=%v", r.Mood, r.Theme, r.Open)
	w, h := lampWantSize(r.Mood, r.Open, r.Rows)
	if math.Abs(r.InnerW-w) > 2 || math.Abs(r.InnerH-h) > 2 {
		p = append(p, fmt.Sprintf("%s: the window is %.0f x %.0f px, want %.0f x %.0f", where, r.InnerW, r.InnerH, w, h))
	}
	if r.DocW > r.InnerW+1 {
		p = append(p, fmt.Sprintf("%s: the page is %.0f px wide in a %.0f px window (a horizontal scrollbar)", where, r.DocW, r.InnerW))
	}
	if r.DocH > r.InnerH+1 {
		p = append(p, fmt.Sprintf("%s: the page is %.0f px high in a %.0f px window (a vertical scrollbar)", where, r.DocH, r.InnerH))
	}
	for _, c := range r.CutNoTitle {
		p = append(p, fmt.Sprintf("%s: %s is cut and has no hover title", where, c))
	}
	for _, c := range r.Colors {
		ratio, err := lampContrast(c.FG, c.BG)
		if err != nil {
			p = append(p, fmt.Sprintf("%s: %s: %v", where, c.Name, err))
		} else if ratio < 4.5 {
			p = append(p, fmt.Sprintf("%s: %s has contrast %.2f, want 4.5", where, c.Name, ratio))
		}
	}
	return p
}

// lampReadJS reads the page the way a person sees it.
const lampReadJS = `(function(){
  var bgOf = function(e){ while (e) { var c = getComputedStyle(e).backgroundColor; if (c && c !== 'rgba(0, 0, 0, 0)' && c !== 'transparent') return c; e = e.parentElement; } return 'rgb(0, 0, 0)'; };
  var colors = [], cut = [];
  [['head', '.head'], ['sub', '.sub'], ['word', '.lword'], ['title', '.ltitle'], ['wait', '.lwait']].forEach(function(p){
    [].forEach.call(document.querySelectorAll(p[1]), function(e){
      colors.push({name: p[0], fg: getComputedStyle(e).color, bg: bgOf(e)});
      if (e.scrollWidth > e.clientWidth + 1 && !(e.getAttribute('title') || '').trim()) cut.push(p[1]);
    });
  });
  var de = document.documentElement;
  return {innerW: window.innerWidth, innerH: window.innerHeight, docW: de.scrollWidth, docH: de.scrollHeight,
    rows: document.querySelectorAll('.lrow').length, cutNoTitle: cut, colors: colors};
})()`

const lampHoverJS = `(function(){ document.documentElement.dispatchEvent(new Event('%s')); return true; })()`

// lampShow freezes the page's poll and renders one fixed state, then waits
// for the glim picture and the window resize that follow.
func lampShow(mood, theme string) error {
	var ok bool
	if err := evalInto(`(function(){ window.lampFreeze = true; window.lampRender(`+lampFixture(mood, theme)+`); return true; })()`, &ok); err != nil {
		return err
	}
	time.Sleep(450 * time.Millisecond)
	return nil
}

func lampReadNow(mood, theme string, open bool) (lampRead, error) {
	var r lampRead
	if err := evalInto(lampReadJS, &r); err != nil {
		return r, err
	}
	r.Mood, r.Theme, r.Open = mood, theme, open
	return r, nil
}

func init() {
	checks["lamp"] = runLampGroup
	checks["lamp/states"] = runLampStates
	lampNames["lamp"] = true
	lampNames["lamp/states"] = true
}

func runLampGroup(hwnd uintptr, args []string) error {
	fmt.Println("uicheck: group lamp: states")
	fmt.Println("--- case lamp/states ---")
	if err := runLampStates(hwnd, args); err != nil {
		return fmt.Errorf("lamp/states: %w", err)
	}
	fmt.Println("uicheck: lamp/states OK")
	return nil
}

// runLampStates shows every mood in both themes closed, then hovers the lamp
// and shows them all again opened, and fails on the first problem list.
func runLampStates(_ uintptr, _ []string) error {
	var problems []string
	var ok bool
	if err := evalReload(); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)
	for _, open := range []bool{false, true} {
		if open {
			if err := evalInto(fmt.Sprintf(lampHoverJS, "mouseenter"), &ok); err != nil {
				return err
			}
		}
		for _, theme := range []string{"dark", "light"} {
			for _, mood := range lampMoods {
				if err := lampShow(mood, theme); err != nil {
					return err
				}
				r, err := lampReadNow(mood, theme, open)
				if err != nil {
					return err
				}
				// The fixture decides how many rows the opened pill has; the
				// page's rows are read back to prove the fixture reached it.
				if r.Rows != lampRowsOf[mood] && open {
					problems = append(problems, fmt.Sprintf("%s/%s: the page has %d rows, the fixture has %d", mood, theme, r.Rows, lampRowsOf[mood]))
				}
				r.Rows = lampRowsOf[mood]
				problems = append(problems, lampProblems(r)...)
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("%d problems:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
	return nil
}

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
// its own eval port 9336). It freezes the page's own poll and hands the page a
// fixed state through window.lampRender. It checks the slim ticker layout of
// the glim finish lane: the ticker is 300 px wide with 26 px rows (1 to 4, one
// more row when opened), Glim hangs under it at the lamp_size scale, nothing
// scrolls, cut text has a hover title, the text has contrast in both themes,
// and each act, frozen at its middle, runs the animation the page owns for it.
// The lamp needs no window handle: the eval connection proves it runs. Nothing
// here changes a file.

// evalAddrLamp is the lamp's dev eval server (cmd\burnmon-dev\lamp_window_windows.go),
// a fourth port beside 9333, 9334 and 9335.
const evalAddrLamp = "127.0.0.1:9336"

// lampNames holds every name that targets the lamp window.
var lampNames = map[string]bool{}

func isLampCheck(name string) bool { return lampNames[name] }

// lampMoods are the eight moods of the lamp, in the order of its spec.
var lampMoods = []string{"off", "asleep", "working", "your_turn", "blocked", "bored", "sweating", "done"}

// The geometry of the lane's Definitions: the ticker, a row, the scale of
// each lamp_size, and the box Glim hangs in (132 units, 211 beside the wall).
const (
	lampTickerW = 300.0
	lampRowH    = 26.0
	lampOpenMax = 6
)

var lampScale = map[string]float64{"small": 1.2, "medium": 1.5, "large": 1.9}

// lampActMs is the length of each act, from the Definitions table.
var lampActMs = map[string]float64{
	"alarm": 2600, "knock": 1600, "stamp": 2400, "welcome": 2400,
	"wake": 1600, "slip": 6400, "coffee": 4000, "peek": 4400,
}

// lampActAnim is the keyframes name the page runs for each act.
var lampActAnim = map[string]string{
	"alarm": "alarm", "knock": "knock", "stamp": "thunk", "welcome": "welcome",
	"wake": "wake", "slip": "slip", "coffee": "sip", "peek": "peek",
}

// lampActMood is the mood each act is frozen in. The quiet moods bring Glim
// down only for peek, coffee and wake, so those three use a quiet mood.
var lampActMood = map[string]string{
	"alarm": "blocked", "knock": "your_turn", "stamp": "done", "welcome": "your_turn",
	"wake": "working", "slip": "bored", "coffee": "asleep", "peek": "asleep",
}

// lampCase is one fixed state the page renders and the way it is looked at.
type lampCase struct {
	Mood  string
	Theme string
	Size  string
	Rows  int // lamp_ticker_rows
	Items int // counted items the fixture carries
	Open  bool
	Act   string
}

func lampQuiet(mood string) bool { return mood == "asleep" || mood == "working" || mood == "off" }

// lampDown is whether Glim hangs: always, but for a quiet lamp that hides,
// where only peek, coffee and wake bring him down.
func lampDown(c lampCase) bool {
	return !lampQuiet(c.Mood) || c.Act == "peek" || c.Act == "coffee" || c.Act == "wake"
}

// lampSliverOnly is the quiet lamp that shows only the 120 by 6 sliver.
func lampSliverOnly(c lampCase) bool { return lampQuiet(c.Mood) && !lampDown(c) && !c.Open }

func lampWall(c lampCase) bool { return c.Mood == "bored" }

// lampWantTickerRows is 1 + min(items, rows - 1); opened, the header, the
// items (up to 6) and the burn row.
func lampWantTickerRows(c lampCase) int {
	if c.Open {
		return 1 + min(c.Items, lampOpenMax) + 1
	}
	return 1 + min(c.Items, c.Rows-1)
}

func lampBoxW(c lampCase) float64 {
	if lampWall(c) {
		return 211
	}
	return 132
}

// lampWantGlim is the box of Glim: width, height and top in CSS pixels.
func lampWantGlim(c lampCase) (w, h, top float64) {
	s := lampScale[c.Size]
	return lampBoxW(c) * s, 132 * s, lampRowH*float64(lampWantTickerRows(c)) - 6*s
}

// lampWantWindow is the CSS size the window must have. The act and the wall
// make it grow the way the lane's window area says.
func lampWantWindow(c lampCase) (w, h float64) {
	if lampSliverOnly(c) {
		return 120, 6
	}
	s := lampScale[c.Size]
	tH := lampRowH * float64(lampWantTickerRows(c))
	pad := math.Max(0, math.Ceil(150-66*s+lampBoxW(c)*s-lampTickerW))
	extra := 0.0
	if c.Act == "slip" {
		pad = math.Max(pad, math.Ceil(90*s))
		extra = 26 * s
	}
	if c.Act == "alarm" {
		pad = math.Max(pad, 24)
	}
	return lampTickerW + 2*pad, math.Round(tH + 126*s + extra)
}

// lampItemsOf is how many items the fixture of a mood carries by default.
var lampItemsOf = map[string]int{"your_turn": 1, "blocked": 2, "bored": 3}

// lampCases is every fixed state the group looks at: every mood in both
// themes at rows 1 and 4, then five items at the three sizes, then every act.
func lampCases() []lampCase {
	var out []lampCase
	for _, rows := range []int{1, 4} {
		for _, theme := range []string{"dark", "light"} {
			for _, mood := range lampMoods {
				out = append(out, lampCase{Mood: mood, Theme: theme, Size: "medium", Rows: rows, Items: lampItemsOf[mood]})
			}
			for _, size := range []string{"small", "medium", "large"} {
				out = append(out, lampCase{Mood: "blocked", Theme: theme, Size: size, Rows: rows, Items: 5})
			}
		}
	}
	for _, theme := range []string{"dark", "light"} {
		for _, act := range []string{"alarm", "knock", "stamp", "welcome", "wake", "slip", "coffee", "peek"} {
			mood := lampActMood[act]
			out = append(out, lampCase{Mood: mood, Theme: theme, Size: "medium", Rows: 1, Items: lampItemsOf[mood], Act: act})
		}
	}
	return out
}

type lampFixtureRow struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Word      string `json:"word"`
	Title     string `json:"title"`
	TitleFull string `json:"title_full"`
	Wait      string `json:"wait"`
	WaitExact string `json:"wait_exact"`
}

// lampPayload is what window.lampRender takes for one case. The names are
// invented; the longest line and title are cut at the ticker width on purpose,
// so the hover title is tested. seq is the act sequence number.
func lampPayload(c lampCase, seq int) string {
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
	words := []string{"gate", "prompt", "ready", "gate", "prompt"}
	titles := []string{
		"Ship the fixture release",
		"Allow the fixture build to run its long test suite on the fixture repo",
		"Fixture plan is ready", "Review the fixture migration", "Approve the fixture rename",
	}
	waits := []string{"12 min", "3 min", "45 s", "31 s", "9 s"}
	rows := []lampFixtureRow{}
	for i := 0; i < c.Items && i < len(titles); i++ {
		full := titles[i]
		cut := full
		if r := []rune(full); len(r) > 28 {
			cut = string(r[:27]) + "…"
		}
		rows = append(rows, lampFixtureRow{ID: fmt.Sprintf("fx-%d", i+1), Kind: words[i], Word: words[i],
			Title: cut, TitleFull: full, Wait: waits[i], WaitExact: waits[i] + " 4 s"})
	}
	st := map[string]any{
		"mood": c.Mood, "headline": heads[c.Mood], "line": lines[c.Mood], "rows": rows,
		"ticker_rows": 1 + min(c.Items, c.Rows-1), "counted": c.Items,
		"counts": map[string]int{}, "agents": 3, "longest_wait_min": 0, "wall_bricks": 0,
		"wall_over": false, "burn_rate": 24000.0, "demo": false,
		"cost_5min": map[string]any{"usd": 1.25, "eur": 1.15, "label": "API price equivalent"},
	}
	if c.Mood == "bored" {
		st["longest_wait_min"], st["wall_bricks"] = 15, 15
	}
	p := map[string]any{
		"state": st,
		// No labels and no act lines: the page falls back to its English words.
		"look":        map[string]any{"size": c.Size, "theme": c.Theme, "motion": "lively", "quiet": "hide", "backdrop": "clear", "rows": c.Rows},
		"act":         c.Act,
		"act_seq":     seq,
		"holds":       []string{},
		"dot_seconds": 2,
		"quiet":       lampQuiet(c.Mood),
	}
	b, _ := json.Marshal(p)
	return string(b)
}

type lampColor struct {
	Name string `json:"name"`
	FG   string `json:"fg"`
	BG   string `json:"bg"`
}

type lampRead struct {
	InnerW     float64     `json:"innerW"`
	InnerH     float64     `json:"innerH"`
	DocW       float64     `json:"docW"`
	DocH       float64     `json:"docH"`
	Sliver     bool        `json:"sliver"`
	TickW      float64     `json:"tickW"`
	TickH      float64     `json:"tickH"`
	TickRows   int         `json:"tickRows"`
	HangShown  bool        `json:"hangShown"`
	HangW      float64     `json:"hangW"`
	HangH      float64     `json:"hangH"`
	HangTop    float64     `json:"hangTop"`
	Anims      []string    `json:"anims"`
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

func near(a, b float64) bool { return math.Abs(a-b) <= 2 }

// lampProblems lists what is wrong with one read: the window and ticker size,
// the rows, Glim's box, a scrollbar, cut text without a hover title and text
// without contrast (4.5 to 1). An act only checks its animation and sizes: the
// window grows for it on purpose, so no scrollbar is looked for.
func lampProblems(c lampCase, r lampRead) []string {
	var p []string
	where := fmt.Sprintf("%s/%s/%s rows=%d items=%d open=%v act=%q", c.Mood, c.Theme, c.Size, c.Rows, c.Items, c.Open, c.Act)
	w, h := lampWantWindow(c)
	if !near(r.InnerW, w) || !near(r.InnerH, h) {
		p = append(p, fmt.Sprintf("%s: the window is %.0f x %.0f px, want %.0f x %.0f", where, r.InnerW, r.InnerH, w, h))
	}
	if c.Act == "" {
		if r.DocW > r.InnerW+1 {
			p = append(p, fmt.Sprintf("%s: the page is %.0f px wide in a %.0f px window (a horizontal scrollbar)", where, r.DocW, r.InnerW))
		}
		if r.DocH > r.InnerH+1 {
			p = append(p, fmt.Sprintf("%s: the page is %.0f px high in a %.0f px window (a vertical scrollbar)", where, r.DocH, r.InnerH))
		}
	}
	if lampSliverOnly(c) {
		if !r.Sliver {
			p = append(p, where+": the sliver is not shown")
		}
		return p
	}
	if r.Sliver {
		p = append(p, where+": the sliver is shown where the ticker belongs")
	}
	rows := lampWantTickerRows(c)
	if r.TickRows != rows {
		p = append(p, fmt.Sprintf("%s: the ticker has %d rows, want %d", where, r.TickRows, rows))
	}
	if !near(r.TickW, lampTickerW) {
		p = append(p, fmt.Sprintf("%s: the ticker is %.0f px wide, want %.0f", where, r.TickW, lampTickerW))
	}
	if !near(r.TickH, lampRowH*float64(rows)) {
		p = append(p, fmt.Sprintf("%s: the ticker is %.0f px high, want %.0f", where, r.TickH, lampRowH*float64(rows)))
	}
	if lampDown(c) {
		gw, gh, gt := lampWantGlim(c)
		switch {
		case !r.HangShown:
			p = append(p, where+": Glim is not shown")
		case !near(r.HangW, gw) || !near(r.HangH, gh) || !near(r.HangTop, gt):
			p = append(p, fmt.Sprintf("%s: Glim's box is %.0f x %.0f at %.0f, want %.0f x %.0f at %.0f", where, r.HangW, r.HangH, r.HangTop, gw, gh, gt))
		}
	} else if r.HangShown {
		p = append(p, where+": Glim is shown while the lamp is quiet")
	}
	if c.Act != "" {
		want, found := lampActAnim[c.Act], false
		for _, a := range r.Anims {
			if a == want {
				found = true
			}
		}
		if !found {
			p = append(p, fmt.Sprintf("%s: no animation %q runs at the middle of the act (running: %v)", where, want, r.Anims))
		}
	}
	for _, t := range r.CutNoTitle {
		p = append(p, fmt.Sprintf("%s: %s is cut and has no hover title", where, t))
	}
	for _, col := range r.Colors {
		ratio, err := lampContrast(col.FG, col.BG)
		if err != nil {
			p = append(p, fmt.Sprintf("%s: %s: %v", where, col.Name, err))
		} else if ratio < 4.5 {
			p = append(p, fmt.Sprintf("%s: %s has contrast %.2f, want 4.5", where, col.Name, ratio))
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
  var vis = function(e){ return e && e.style.display !== 'none'; };
  var tk = document.getElementById('tick'), hg = document.getElementById('hang'), sl = document.getElementById('sliver');
  var tr = vis(tk) ? tk.getBoundingClientRect() : null, hr = vis(hg) ? hg.getBoundingClientRect() : null;
  var anims = hg && hg.getAnimations ? hg.getAnimations({subtree: true}).map(function(a){ return a.animationName || ''; }) : [];
  var de = document.documentElement;
  return {innerW: window.innerWidth, innerH: window.innerHeight, docW: de.scrollWidth, docH: de.scrollHeight,
    sliver: vis(sl), tickW: tr ? tr.width : 0, tickH: tr ? tr.height : 0, tickRows: vis(tk) ? tk.querySelectorAll('.trow').length : 0,
    hangShown: !!hr, hangW: hr ? hr.width : 0, hangH: hr ? hr.height : 0, hangTop: hr ? hr.top : 0,
    anims: anims, cutNoTitle: cut, colors: colors};
})()`

// lampFreezeJS pauses every animation of Glim at the middle of its act.
const lampFreezeJS = `(function(){ var hg = document.getElementById('hang'); if (!hg || !hg.getAnimations) return false;
  hg.getAnimations({subtree: true}).forEach(function(a){ a.pause(); a.currentTime = %d; }); return true; })()`

// lampHoverJS opens the ticker: the page listens on the ticker, Glim and the sliver.
const lampHoverJS = `(function(){ ['tick','hang','sliver'].forEach(function(id){ var e = document.getElementById(id); if (e) e.dispatchEvent(new Event('mouseenter')); }); return true; })()`

// lampShow freezes the page's poll and renders one fixed state, then waits
// for the glim picture and the window resize that follow.
func lampShow(c lampCase, seq int) error {
	var ok bool
	if err := evalInto(`(function(){ window.lampFreeze = true; window.lampRender(`+lampPayload(c, seq)+`); return true; })()`, &ok); err != nil {
		return err
	}
	time.Sleep(450 * time.Millisecond)
	if c.Act != "" {
		return evalInto(fmt.Sprintf(lampFreezeJS, int(lampActMs[c.Act]/2)), &ok)
	}
	return nil
}

func lampReadNow() (lampRead, error) {
	var r lampRead
	err := evalInto(lampReadJS, &r)
	return r, err
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

// runLampStates shows every case closed, then hovers the lamp and shows the
// non-act cases again opened, and fails on the first problem list.
func runLampStates(_ uintptr, _ []string) error {
	var problems []string
	var ok bool
	if err := evalReload(); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)
	seq := 0
	for _, open := range []bool{false, true} {
		if open {
			if err := evalInto(lampHoverJS, &ok); err != nil {
				return err
			}
		}
		for _, c := range lampCases() {
			if open && c.Act != "" {
				continue
			}
			c.Open = open
			seq++
			if err := lampShow(c, seq); err != nil {
				return err
			}
			r, err := lampReadNow()
			if err != nil {
				return err
			}
			problems = append(problems, lampProblems(c, r)...)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("%d problems:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
	return nil
}

package main

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// goodLampRead is a read that matches its case exactly.
func goodLampRead(c lampCase) lampRead {
	tw := goodTickW(c)
	w, h := lampWantWindow(c, tw)
	r := lampRead{
		InnerW: w, InnerH: h, DocW: w, DocH: h,
		Colors: []lampColor{{Name: "head", FG: "rgb(250, 199, 117)", BG: "rgb(20, 17, 13)"}},
	}
	if c.Mood == "off" {
		r.Row1Head = "BurnMon is not running"
	}
	if lampSliverOnly(c) {
		r.Sliver = true
		return r
	}
	tr := lampWantTickerRows(c)
	r.TickW, r.TickH, r.TickRows = tw, float64(lampRowH*tr), tr
	for i := 0; i < tr; i++ {
		r.Rows = append(r.Rows, lampRowBox{L: 1, W: tw - 2})
	}
	if lampDown(c) {
		gw, gh, gt := lampWantGlim(c)
		r.HangShown, r.HangW, r.HangH, r.HangTop = true, gw, gh, gt
		r.HangClass = "glim mood-" + lampWantPose(c)
	}
	return r
}

// goodTickW is a ticker width that fits the text of the case.
func goodTickW(c lampCase) float64 {
	switch c.Text {
	case "short":
		return 300
	case "huge":
		return 440
	}
	return 372
}

func caseOf(mood string, rows, items int) lampCase {
	return lampCase{Mood: mood, Theme: "dark", Size: 150, Rows: rows, Items: items}
}

// The lamp checks target burnmon-dev.exe's lamp window, not another exe.
func TestLampChecksAreRouted(t *testing.T) {
	for _, name := range []string{"lamp", "lamp/states"} {
		if checks[name] == nil {
			t.Errorf("%s is not registered", name)
		}
		if !isLampCheck(name) {
			t.Errorf("%s is not a lamp check", name)
		}
		if isDevCheck(name) || isCentreCheck(name) {
			t.Errorf("%s is counted as another exe's check", name)
		}
	}
	for _, name := range []string{"w0", "d2", "waits", "gitpage"} {
		if isLampCheck(name) {
			t.Errorf("%s is counted as a lamp check", name)
		}
	}
	if evalAddrLamp != "127.0.0.1:9336" {
		t.Errorf("the lamp eval port is %s, want 9336", evalAddrLamp)
	}
}

// The case list covers rows 1 and 4, three sizes, both themes, every mood and
// every act of the lamp spec, each act frozen at its middle.
func TestLampCasesCoverTheNewLayout(t *testing.T) {
	cases := lampCases()
	rows, sizes, themes, moods, acts := map[int]bool{}, map[int]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, c := range cases {
		if c.Act == "" {
			rows[c.Rows], sizes[c.Size], themes[c.Theme], moods[c.Mood] = true, true, true, true
		} else {
			acts[c.Act] = true
		}
	}
	if !rows[1] || !rows[4] || len(rows) != 2 {
		t.Errorf("rows covered: %v, want 1 and 4", rows)
	}
	for _, s := range []int{100, 150, 200} {
		if !sizes[s] {
			t.Errorf("size %d is not covered", s)
		}
	}
	texts := map[string]bool{}
	for _, c := range cases {
		texts[c.Text] = true
	}
	for _, x := range []string{"short", "huge"} {
		if !texts[x] {
			t.Errorf("the text length %q is not covered", x)
		}
	}
	if !themes["dark"] || !themes["light"] {
		t.Errorf("themes covered: %v", themes)
	}
	for _, m := range lampMoods {
		if !moods[m] {
			t.Errorf("mood %s is not covered", m)
		}
	}
	for _, a := range []string{"alarm", "knock", "stamp", "welcome", "wake", "slip", "coffee", "peek"} {
		if !acts[a] {
			t.Errorf("act %s is not covered", a)
		}
		if lampActMs[a] <= 0 || lampActAnim[a] == "" {
			t.Errorf("act %s has no length or animation name", a)
		}
	}
	if len(lampMoods) != 8 {
		t.Errorf("%d moods, want 8", len(lampMoods))
	}
}

// Every case has a fixed state the page's lampRender takes.
func TestLampFixturesCoverEveryCase(t *testing.T) {
	for _, c := range lampCases() {
		raw := lampPayload(c, 1)
		var p struct {
			State struct {
				Mood       string            `json:"mood"`
				Headline   string            `json:"headline"`
				TickerRows int               `json:"ticker_rows"`
				Rows       []json.RawMessage `json:"rows"`
			} `json:"state"`
			Look struct {
				Size   int    `json:"size"`
				Theme  string `json:"theme"`
				Motion string `json:"motion"`
			} `json:"look"`
			Act  string `json:"act"`
			Pose string `json:"pose"`
		}
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			t.Fatalf("%+v: %v", c, err)
		}
		if p.State.Mood != c.Mood || p.State.Headline == "" || p.Look.Theme != c.Theme || p.Look.Size != c.Size || p.Look.Motion != "lively" || p.Act != c.Act || p.Pose != lampWantPose(c) {
			t.Errorf("%+v: got mood %q headline %q theme %q size %d motion %q act %q pose %q", c, p.State.Mood, p.State.Headline, p.Look.Theme, p.Look.Size, p.Look.Motion, p.Act, p.Pose)
		}
		if p.State.TickerRows != 1+min(c.Items, c.Rows-1) || len(p.State.Rows) != c.Items {
			t.Errorf("%+v: ticker_rows %d, rows %d", c, p.State.TickerRows, len(p.State.Rows))
		}
		if strings.ContainsRune(raw, 0x2014) {
			t.Errorf("%+v: the fixture has an em dash", c)
		}
	}
}

// The ticker is 300 px wide, 26 px a row, and shows 1 + min(items, rows - 1)
// rows; opened it shows the items and the burn row.
func TestLampWantTickerRows(t *testing.T) {
	for _, tc := range []struct {
		rows, items, want int
		open              bool
	}{
		{1, 0, 1, false}, {1, 1, 1, false}, {1, 5, 1, false},
		{4, 0, 1, false}, {4, 1, 2, false}, {4, 5, 4, false},
		{1, 5, 7, true}, {4, 0, 2, true}, {4, 2, 4, true},
	} {
		c := caseOf("blocked", tc.rows, tc.items)
		c.Open = tc.open
		if got := lampWantTickerRows(c); got != tc.want {
			t.Errorf("rows %d items %d open %v: %d ticker rows, want %d", tc.rows, tc.items, tc.open, got, tc.want)
		}
	}
	if lampTickerW != 300 || lampRowH != 26 {
		t.Errorf("ticker %v wide, rows %v high, want 300 and 26", lampTickerW, lampRowH)
	}
}

func TestLampWantWindow(t *testing.T) {
	for _, tc := range []struct {
		c            lampCase
		wantW, wantH float64
	}{
		{caseOf("asleep", 1, 0), 120, 6},
		{caseOf("off", 4, 0), 120, 6},
		{caseOf("working", 4, 0), 120, 6},
		// 150 percent: one row 26 + 126*1.5 = 215.
		{caseOf("done", 1, 0), 300, 215},
		// four rows: 104 + 189 = 293.
		{caseOf("blocked", 4, 5), 300, 293},
		{lampCase{Mood: "blocked", Theme: "dark", Size: 100, Rows: 1, Items: 5}, 300, 26 + 126},
		{lampCase{Mood: "blocked", Theme: "dark", Size: 200, Rows: 4, Items: 5}, 300, 104 + 252},
		// opened: header, five items and the burn row.
		{lampCase{Mood: "blocked", Theme: "dark", Size: 150, Rows: 1, Items: 5, Open: true}, 300, 26*7 + 189},
	} {
		w, h := lampWantWindow(tc.c, 300)
		if w != tc.wantW || math.Abs(h-tc.wantH) > 1 {
			t.Errorf("%+v: %v x %v, want %v x %v", tc.c, w, h, tc.wantW, tc.wantH)
		}
	}
	// The wall widens the window, slip and alarm too.
	if w, _ := lampWantWindow(caseOf("bored", 1, 3), 300); w <= 300 {
		t.Errorf("the wall did not widen the window: %v", w)
	}
	if w, _ := lampWantWindow(lampCase{Mood: "bored", Theme: "dark", Size: 150, Rows: 1, Act: "slip"}, 300); w < 300+2*135 {
		t.Errorf("slip window %v wide", w)
	}
	if w, _ := lampWantWindow(lampCase{Mood: "blocked", Theme: "dark", Size: 190, Rows: 1, Act: "alarm"}, 300); w != 300+48 {
		t.Errorf("alarm window %v wide, want 348", w)
	}
	// A 440 px ticker is a window as wide as the ticker.
	if w, _ := lampWantWindow(caseOf("done", 1, 0), 440); w != 440 {
		t.Errorf("a 440 px ticker gives a %v px window, want 440", w)
	}
}

func TestLampWantGlim(t *testing.T) {
	w, h, top := lampWantGlim(caseOf("done", 1, 0))
	if w != 132*1.5 || h != 132*1.5 || top != 26-6*1.5 {
		t.Errorf("medium glim box %v x %v at %v", w, h, top)
	}
	c := caseOf("bored", 1, 3)
	if w, _, _ := lampWantGlim(c); w != 211*1.5 {
		t.Errorf("the wall glim box is %v wide, want %v", w, 211*1.5)
	}
}

func TestLampProblems(t *testing.T) {
	for _, c := range []lampCase{caseOf("blocked", 4, 5), caseOf("done", 1, 0), caseOf("asleep", 1, 0), caseOf("bored", 4, 3)} {
		if p := lampProblems(c, goodLampRead(c)); len(p) != 0 {
			t.Fatalf("%+v: a good read has problems: %v", c, p)
		}
		c.Open = true
		if p := lampProblems(c, goodLampRead(c)); len(p) != 0 {
			t.Fatalf("%+v: a good opened read has problems: %v", c, p)
		}
	}
	c := caseOf("blocked", 4, 5)
	for name, mut := range map[string]func(*lampRead){
		"ticker under 300":    func(r *lampRead) { r.TickW = 290 },
		"ticker over 440":     func(r *lampRead) { r.TickW = 460 },
		"opened row too left": func(r *lampRead) { r.Rows[len(r.Rows)-1].L = -4 },
		"opened row too wide": func(r *lampRead) { r.Rows[len(r.Rows)-1].W += 12 },
		"wrong pose":          func(r *lampRead) { r.HangClass = "glim mood-asleep" },
		"wrong ticker height": func(r *lampRead) { r.TickH = 40 },
		"wrong row count":     func(r *lampRead) { r.TickRows = 2 },
		"wrong window":        func(r *lampRead) { r.InnerW = 440 },
		"glim missing":        func(r *lampRead) { r.HangShown = false },
		"wrong glim size":     func(r *lampRead) { r.HangW = 90 },
		"glim not under":      func(r *lampRead) { r.HangTop = 80 },
		"horizontal scroll":   func(r *lampRead) { r.DocW = r.InnerW + 12 },
		"vertical scroll":     func(r *lampRead) { r.DocH = r.InnerH + 5 },
		"cut without title":   func(r *lampRead) { r.CutNoTitle = []string{".sub"} },
		"low contrast": func(r *lampRead) {
			r.Colors = []lampColor{{Name: "sub", FG: "rgb(90, 68, 22)", BG: "rgb(21, 17, 10)"}}
		},
		"unreadable colour": func(r *lampRead) { r.Colors = []lampColor{{Name: "head", FG: "not a colour", BG: "rgb(0, 0, 0)"}} },
	} {
		r := goodLampRead(c)
		mut(&r)
		if p := lampProblems(c, r); len(p) == 0 {
			t.Errorf("%s was not reported", name)
		}
	}
	// A quiet lamp that shows a ticker is wrong, a sliver where a ticker belongs too.
	q := caseOf("asleep", 1, 0)
	r := goodLampRead(q)
	r.Sliver = false
	if p := lampProblems(q, r); len(p) == 0 {
		t.Error("a missing sliver was not reported")
	}
	r = goodLampRead(c)
	r.Sliver = true
	if p := lampProblems(c, r); len(p) == 0 {
		t.Error("a sliver where the ticker belongs was not reported")
	}
}

// An act frozen at its middle must run the animation the page owns for it,
// and a scrollbar is not checked there (the window grows by design).
func TestLampActProblems(t *testing.T) {
	c := lampCase{Mood: "blocked", Theme: "dark", Size: 150, Rows: 1, Items: 2, Act: "alarm"}
	r := goodLampRead(c)
	r.Anims = []string{"alarm", "pulse", "sway"}
	r.DocW = r.InnerW + 40
	if p := lampProblems(c, r); len(p) != 0 {
		t.Fatalf("a good act read has problems: %v", p)
	}
	r.Anims = []string{"sway"}
	if p := lampProblems(c, r); len(p) == 0 {
		t.Error("a missing act animation was not reported")
	}
	if lampActMs["alarm"] != 2600 || lampActMs["slip"] != 6400 || lampActMs["peek"] != 4400 {
		t.Errorf("act lengths drifted from the lane: %v", lampActMs)
	}
}

// An act case must show Glim, or its animation is never read (a quiet lamp that
// hides shows the sliver except for peek, coffee and wake). The stamp act ends
// its body animation before its middle, so the page's thunk (fill both) is the
// animation that is still there.
func TestLampActCasesShowGlim(t *testing.T) {
	for _, c := range lampCases() {
		if c.Act != "" && lampSliverOnly(c) {
			t.Errorf("act %s in mood %s shows only the sliver, its animation is never checked", c.Act, c.Mood)
		}
	}
	if lampActAnim["stamp"] != "thunk" {
		t.Errorf("stamp is read through %q, want thunk", lampActAnim["stamp"])
	}
}

func TestLampContrast(t *testing.T) {
	c, err := lampContrast("rgb(255, 255, 255)", "rgb(0, 0, 0)")
	if err != nil || math.Abs(c-21) > 0.01 {
		t.Fatalf("white on black is %v, %v; want 21", c, err)
	}
	if c, _ := lampContrast("rgb(10, 10, 10)", "rgb(10, 10, 10)"); math.Abs(c-1) > 0.01 {
		t.Errorf("same colour is %v, want 1", c)
	}
	if _, err := lampContrast("rgba(0, 0, 0, 0)", "rgb(0, 0, 0)"); err == nil {
		t.Error("a transparent colour was accepted")
	}
}

// A short line keeps the ticker at 300, a huge one stops it at 440, and the
// coffee act draws Glim awake (the working pose) in the asleep mood.
func TestLampTextWidthAndCoffeePose(t *testing.T) {
	short := caseOf("working", 1, 0)
	short.Text = "short"
	short.Act = "wake"
	r := goodLampRead(short)
	r.TickW = 340
	if p := lampProblems(short, r); len(p) == 0 {
		t.Error("a short line with a 340 px ticker was not reported")
	}
	huge := caseOf("blocked", 1, 2)
	huge.Text = "huge"
	r = goodLampRead(huge)
	r.TickW = 400
	if p := lampProblems(huge, r); len(p) == 0 {
		t.Error("a huge line with a 400 px ticker was not reported")
	}
	if lampWantPose(lampCase{Mood: "asleep", Act: "coffee"}) != "working" {
		t.Error("the coffee pose is not working")
	}
	if lampWantPose(lampCase{Mood: "asleep", Act: "peek"}) != "asleep" {
		t.Error("the peek pose is not the mood")
	}
	c := lampCase{Mood: "asleep", Theme: "dark", Size: 150, Rows: 1, Act: "coffee"}
	r = goodLampRead(c)
	r.HangClass = "glim mood-asleep"
	if p := lampProblems(c, r); len(p) == 0 {
		t.Error("Glim asleep at the coffee was not reported")
	}
}

// N6 (Mac check 2026-11-22): when BurnMon is not running the Go headline has no
// full stop and the Go line has one, and row 1 printed the sentence twice. The
// off case carries both real strings, and the read fails when row 1 shows a
// second span or no headline at all (an empty read would pass vacuously).
func TestLampOffCaseCarriesTheRealHeadlineAndLine(t *testing.T) {
	var p struct {
		State struct {
			Headline string `json:"headline"`
			Line     string `json:"line"`
		} `json:"state"`
	}
	if err := json.Unmarshal([]byte(lampPayload(caseOf("off", 1, 0), 1)), &p); err != nil {
		t.Fatal(err)
	}
	if p.State.Headline != "BurnMon is not running" || p.State.Line != "BurnMon is not running." {
		t.Fatalf("off payload headline %q, line %q; want the strings internal\\lamp sends", p.State.Headline, p.State.Line)
	}
}

func TestLampOffRow1ShowsTheSentenceOnce(t *testing.T) {
	// Opened, the off lamp draws its ticker; closed it is only the sliver (see below).
	c := caseOf("off", 1, 0)
	c.Open = true
	r := goodLampRead(c)
	if p := lampProblems(c, r); len(p) != 0 {
		t.Fatalf("a good off read has problems: %v", p)
	}
	twice := r
	twice.Row1Subs = 1
	if p := lampProblems(c, twice); len(p) == 0 {
		t.Error("a second span in row 1 of the off state was not reported")
	}
	empty := r
	empty.Row1Head = ""
	if p := lampProblems(c, empty); len(p) == 0 {
		t.Error("an off state with no headline in row 1 was not reported")
	}
	// Closed, the ticker is hidden and keeps the previous case's row: a stale read must not fail.
	closed := caseOf("off", 1, 0)
	cr := goodLampRead(closed)
	cr.Row1Head, cr.Row1Subs = "1 blocked", 1
	if p := lampProblems(closed, cr); len(p) != 0 {
		t.Errorf("a stale hidden ticker of the closed off lamp was reported: %v", p)
	}
	// Other moods keep their line as a second span on purpose.
	y := caseOf("your_turn", 1, 1)
	yr := goodLampRead(y)
	yr.Row1Subs = 1
	if p := lampProblems(y, yr); len(p) != 0 {
		t.Errorf("a your_turn row 1 with its line was reported: %v", p)
	}
}

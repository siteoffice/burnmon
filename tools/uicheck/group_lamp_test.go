package main

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// goodLampRead is a read that matches its case exactly.
func goodLampRead(c lampCase) lampRead {
	w, h := lampWantWindow(c)
	r := lampRead{
		InnerW: w, InnerH: h, DocW: w, DocH: h,
		Colors: []lampColor{{Name: "head", FG: "rgb(250, 199, 117)", BG: "rgb(20, 17, 13)"}},
	}
	if lampSliverOnly(c) {
		r.Sliver = true
		return r
	}
	tr := lampWantTickerRows(c)
	r.TickW, r.TickH, r.TickRows = lampTickerW, float64(lampRowH*tr), tr
	if lampDown(c) {
		gw, gh, gt := lampWantGlim(c)
		r.HangShown, r.HangW, r.HangH, r.HangTop = true, gw, gh, gt
	}
	return r
}

func caseOf(mood string, rows, items int) lampCase {
	return lampCase{Mood: mood, Theme: "dark", Size: "medium", Rows: rows, Items: items}
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
	rows, sizes, themes, moods, acts := map[int]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
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
	for _, s := range []string{"small", "medium", "large"} {
		if !sizes[s] {
			t.Errorf("size %s is not covered", s)
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
				Size   string `json:"size"`
				Theme  string `json:"theme"`
				Motion string `json:"motion"`
			} `json:"look"`
			Act string `json:"act"`
		}
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			t.Fatalf("%+v: %v", c, err)
		}
		if p.State.Mood != c.Mood || p.State.Headline == "" || p.Look.Theme != c.Theme || p.Look.Size != c.Size || p.Look.Motion != "lively" || p.Act != c.Act {
			t.Errorf("%+v: got mood %q headline %q theme %q size %q motion %q act %q", c, p.State.Mood, p.State.Headline, p.Look.Theme, p.Look.Size, p.Look.Motion, p.Act)
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
		// medium is 1.5: one row 26 + 126*1.5 = 215.
		{caseOf("done", 1, 0), 300, 215},
		// four rows: 104 + 189 = 293.
		{caseOf("blocked", 4, 5), 300, 293},
		{lampCase{Mood: "blocked", Theme: "dark", Size: "small", Rows: 1, Items: 5}, 300, 26 + 151.2},
		{lampCase{Mood: "blocked", Theme: "dark", Size: "large", Rows: 4, Items: 5}, 300, 104 + 239.4},
		// opened: header, five items and the burn row.
		{lampCase{Mood: "blocked", Theme: "dark", Size: "medium", Rows: 1, Items: 5, Open: true}, 300, 26*7 + 189},
	} {
		w, h := lampWantWindow(tc.c)
		if w != tc.wantW || math.Abs(h-tc.wantH) > 1 {
			t.Errorf("%+v: %v x %v, want %v x %v", tc.c, w, h, tc.wantW, tc.wantH)
		}
	}
	// The wall widens the window, slip and alarm too.
	if w, _ := lampWantWindow(caseOf("bored", 1, 3)); w <= 300 {
		t.Errorf("the wall did not widen the window: %v", w)
	}
	if w, _ := lampWantWindow(lampCase{Mood: "bored", Theme: "dark", Size: "medium", Rows: 1, Act: "slip"}); w < 300+2*135 {
		t.Errorf("slip window %v wide", w)
	}
	if w, _ := lampWantWindow(lampCase{Mood: "blocked", Theme: "dark", Size: "large", Rows: 1, Act: "alarm"}); w != 300+48 {
		t.Errorf("alarm window %v wide, want 348", w)
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
		"wrong ticker width":  func(r *lampRead) { r.TickW = 320 },
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
	c := lampCase{Mood: "blocked", Theme: "dark", Size: "medium", Rows: 1, Items: 2, Act: "alarm"}
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

package main

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func goodLampRead(mood string, open bool, rows int) lampRead {
	w, h := lampWantSize(mood, open, rows)
	return lampRead{
		Mood: mood, Theme: "dark", Open: open, Rows: rows, InnerW: w, InnerH: h, DocW: w, DocH: h,
		Colors: []lampColor{{Name: "head", FG: "rgb(255, 199, 102)", BG: "rgb(21, 17, 10)"}},
	}
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

// Every mood has a fixed state, and the payload is what lampRender takes.
func TestLampFixturesCoverEveryMood(t *testing.T) {
	for _, mood := range lampMoods {
		for _, theme := range []string{"dark", "light"} {
			var p struct {
				State struct {
					Mood     string            `json:"mood"`
					Headline string            `json:"headline"`
					Line     string            `json:"line"`
					Rows     []json.RawMessage `json:"rows"`
				} `json:"state"`
				Theme string `json:"theme"`
			}
			if err := json.Unmarshal([]byte(lampFixture(mood, theme)), &p); err != nil {
				t.Fatalf("%s %s: %v", mood, theme, err)
			}
			if p.State.Mood != mood || p.Theme != theme || p.State.Headline == "" {
				t.Errorf("%s %s: got mood %q theme %q headline %q", mood, theme, p.State.Mood, p.Theme, p.State.Headline)
			}
			if strings.ContainsRune(lampFixture(mood, theme), 0x2014) {
				t.Errorf("%s %s: the fixture has an em dash", mood, theme)
			}
		}
	}
	if len(lampMoods) != 8 {
		t.Errorf("%d moods, want 8", len(lampMoods))
	}
}

func TestLampWantSize(t *testing.T) {
	for _, tc := range []struct {
		mood         string
		open         bool
		rows         int
		wantW, wantH float64
	}{
		{"asleep", false, 0, 120, 6},
		{"off", false, 0, 120, 6},
		{"working", false, 0, 120, 6},
		{"blocked", false, 2, 320, 40},
		{"done", false, 0, 320, 40},
		{"asleep", true, 0, 440, 40},
		{"blocked", true, 2, 440, 96},
		{"bored", true, 3, 440, 124},
	} {
		w, h := lampWantSize(tc.mood, tc.open, tc.rows)
		if w != tc.wantW || h != tc.wantH {
			t.Errorf("%s open=%v rows=%d: %v x %v, want %v x %v", tc.mood, tc.open, tc.rows, w, h, tc.wantW, tc.wantH)
		}
	}
}

func TestLampProblems(t *testing.T) {
	if p := lampProblems(goodLampRead("blocked", false, 2)); len(p) != 0 {
		t.Fatalf("a good read has problems: %v", p)
	}
	if p := lampProblems(goodLampRead("blocked", true, 2)); len(p) != 0 {
		t.Fatalf("a good opened read has problems: %v", p)
	}
	r := goodLampRead("blocked", false, 2)
	r.InnerW = 400
	if p := lampProblems(r); len(p) == 0 {
		t.Error("a wrong pill width was not reported")
	}
	r = goodLampRead("blocked", true, 2)
	r.InnerH = 40
	if p := lampProblems(r); len(p) == 0 {
		t.Error("a wrong opened height was not reported")
	}
	r = goodLampRead("done", false, 0)
	r.DocW = r.InnerW + 12
	if p := lampProblems(r); len(p) == 0 {
		t.Error("a horizontal scrollbar was not reported")
	}
	r = goodLampRead("done", false, 0)
	r.DocH = r.InnerH + 5
	if p := lampProblems(r); len(p) == 0 {
		t.Error("a vertical scrollbar was not reported")
	}
	r = goodLampRead("done", false, 0)
	r.CutNoTitle = []string{".sub"}
	if p := lampProblems(r); len(p) == 0 {
		t.Error("cut text without a hover title was not reported")
	}
	r = goodLampRead("done", false, 0)
	r.Colors = []lampColor{{Name: "sub", FG: "rgb(90, 68, 22)", BG: "rgb(21, 17, 10)"}}
	if p := lampProblems(r); len(p) == 0 {
		t.Error("low contrast was not reported")
	}
	r = goodLampRead("done", false, 0)
	r.Colors = []lampColor{{Name: "head", FG: "not a colour", BG: "rgb(0, 0, 0)"}}
	if p := lampProblems(r); len(p) == 0 {
		t.Error("an unreadable colour was not reported")
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

package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// stubApplyStart swaps applyStartFn for f for the length of the test.
func stubApplyStart(t *testing.T, f func(uintptr, startState) error) {
	t.Helper()
	old := applyStartFn
	applyStartFn = f
	t.Cleanup(func() { applyStartFn = old })
}

// The default is what a fresh settings folder gives: sections.Default and a
// dark theme, at the primary viewport.
func TestAppStartIsAppDefault(t *testing.T) {
	s := appStart()
	if s.Plain || !s.Scrollbar || s.Theme != "dark" || s.Width != 1152 || s.Height != 2048 {
		t.Fatalf("appStart = %+v", s)
	}
	if len(s.Standard) != len(devPartIDs) || len(s.PlainSet) != len(devPartIDs) {
		t.Fatalf("lists have %d and %d parts, want %d", len(s.Standard), len(s.PlainSet), len(devPartIDs))
	}
	var plain []string
	for _, id := range devPartIDs {
		if !s.Standard[id] {
			t.Errorf("standard %s is off", id)
		}
		if s.PlainSet[id] {
			plain = append(plain, id)
		}
	}
	if got := strings.Join(plain, " "); got != "waits outcomes burn agents_map sessions" {
		t.Fatalf("plain list = %s", got)
	}
}

// Every group starts from the app default; only the window size differs.
func TestGroupStartDefaults(t *testing.T) {
	want := map[string][2]int32{
		"fit": {1152, 2048}, "charts": {1280, 860}, "station": {1600, 900}, "layout": {1152, 2048},
		"map": {1152, 2048}, "waits": {1920, 1080}, "shell": {1152, 2048},
	}
	for _, g := range devGroups {
		sz, ok := want[g.Name]
		if !ok {
			t.Errorf("group %s has no expected size", g.Name)
			continue
		}
		if g.Start.Width != sz[0] || g.Start.Height != sz[1] {
			t.Errorf("group %s starts at %dx%d, want %dx%d", g.Name, g.Start.Width, g.Start.Height, sz[0], sz[1])
		}
		base := appStart()
		base.Width, base.Height = g.Start.Width, g.Start.Height
		if !reflect.DeepEqual(g.Start, base) {
			t.Errorf("group %s start %+v is not the app default", g.Name, g.Start)
		}
	}
}

// A case override changes its own copy, never the group default.
func TestCaseStartOverride(t *testing.T) {
	cases := map[string]bool{}
	for _, g := range devGroups {
		for _, c := range g.Cases {
			cases[c.Name] = true
		}
	}
	for name := range devCaseStart {
		if !cases[name] {
			t.Errorf("devCaseStart has %s, which is no case", name)
		}
	}
	shell := devGroups[len(devGroups)-1]
	if shell.Name != "shell" {
		t.Fatalf("last group is %s, want shell", shell.Name)
	}
	before := shell.Start.clone()
	for _, c := range shell.Cases {
		s := caseStart(shell, c)
		if c.Name == "d24" {
			if s.Width != 1600 || s.Height != 900 {
				t.Errorf("d24 starts at %dx%d, want 1600x900", s.Width, s.Height)
			}
		} else if !reflect.DeepEqual(s, shell.Start) {
			t.Errorf("%s start %+v differs from the group default", c.Name, s)
		}
	}
	if !reflect.DeepEqual(before, shell.Start) {
		t.Fatalf("an override changed the group default")
	}
}

// The cases that no longer size their own window read at the size they used
// to set themselves (Task 3 iteration 2), so applyStart must give that size.
func TestCaseStartSizes(t *testing.T) {
	want := map[string][2]int32{
		"d0": {1152, 2048}, "d1": {1152, 2048},
		"d2": {1152, 2048}, "d3": {1024, 1152}, "d4": {1280, 860}, "d5": {1920, 1080}, "d6": {2560, 1440}, "d8": {1024, 768},
		"d10": {1280, 860}, "d11": {1280, 860}, "d13": {1280, 860}, "d14": {1920, 1080}, "d15": {1280, 860},
		"d16": {1280, 860}, "d17": {1280, 860}, "d18": {1280, 860}, "d20": {1920, 1080}, "d22": {1920, 1080},
		"d23": {1600, 900}, "d24": {1600, 900}, "d25": {1600, 900}, "d26": {1600, 900}, "d27": {1600, 900}, "d28": {1600, 900},
		"d29": {1152, 2048}, "d42": {1920, 1080},
		"d30": {1152, 2048}, "d31": {1280, 900}, "d38": {1920, 1080},
		"d36": {1920, 1080}, "d37": {1920, 1080}, "d39": {1920, 1080},
	}
	seen := 0
	for _, g := range devGroups {
		for _, c := range g.Cases {
			w, ok := want[c.Name]
			if !ok {
				continue
			}
			seen++
			if s := caseStart(g, c); s.Width != w[0] || s.Height != w[1] {
				t.Errorf("%s/%s starts at %dx%d, want %dx%d", g.Name, c.Name, s.Width, s.Height, w[0], w[1])
			}
		}
	}
	if seen != len(want) {
		t.Errorf("found %d of the %d cases", seen, len(want))
	}
}

func TestCloneOwnsItsMaps(t *testing.T) {
	a := appStart()
	b := a.clone()
	b.Standard["git"] = false
	b.PlainSet["git"] = true
	if !a.Standard["git"] || a.PlainSet["git"] {
		t.Fatalf("clone shares maps with the original")
	}
}

// A page already at the start state gets only the heights reset; a dirty one
// gets each difference, in the fixed order.
func TestStartChanges(t *testing.T) {
	want := appStart()
	clean := startSaved{Scrollbar: true, Standard: want.Standard, PlainSet: want.PlainSet, Parts: devPartIDs}
	if got := startChanges(clean, want); !reflect.DeepEqual(got, []string{`{reset_heights:true}`}) {
		t.Fatalf("clean page: %v", got)
	}

	dirty := startSaved{Plain: true, Scrollbar: false, Standard: map[string]bool{}, PlainSet: map[string]bool{},
		Controls: map[string]bool{"stop": true, "attach": false}}
	for k, v := range want.Standard {
		dirty.Standard[k] = v
	}
	for k, v := range want.PlainSet {
		dirty.PlainSet[k] = v
	}
	dirty.Standard["git"] = false
	dirty.PlainSet["todo"] = true
	got := startChanges(dirty, want)
	exp := []string{
		`{reset_heights:true}`,
		`{control:"stop",on:false}`,
		`{mode:'standard',section:"git",on:true}`,
		`{mode:'plain',section:"todo",on:false}`,
		`{scrollbar:true}`,
		`{plain:false}`,
	}
	if !reflect.DeepEqual(got, exp) {
		t.Fatalf("dirty page:\n got %v\nwant %v", got, exp)
	}

	// A Plain start is reached from the app default too.
	lp := appStart()
	lp.Plain = true
	if got := startChanges(clean, lp); got[len(got)-1] != `{plain:true}` {
		t.Fatalf("to Plain: %v", got)
	}
}

func TestSamePartIDs(t *testing.T) {
	rev := append([]string(nil), devPartIDs...)
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	if !samePartIDs(rev) {
		t.Errorf("order should not matter")
	}
	if samePartIDs(append(rev, "new_part")) || samePartIDs(rev[1:]) {
		t.Errorf("a new or a missing part must not match")
	}
}

// Every registered dev name runs applyStart before its case, with that case's
// own start: a failing stub stops each one before the case body runs (a real
// case body would need a window). A group stops at its first case.
func TestEveryCaseGetsApplyStart(t *testing.T) {
	sentinel := errors.New("start stub")
	var got []startState
	stubApplyStart(t, func(_ uintptr, s startState) error {
		got = append(got, s)
		return sentinel
	})
	for _, g := range devGroups {
		for _, c := range g.Cases {
			for _, name := range []string{g.Name + "/" + c.Name, c.Name} {
				got = nil
				err := checks[name](0, nil)
				if !errors.Is(err, sentinel) || len(got) != 1 {
					t.Errorf("%s: err %v, %d applyStart calls; want the stub's error after one call", name, err, len(got))
					continue
				}
				if !reflect.DeepEqual(got[0], caseStart(g, c)) {
					t.Errorf("%s started from %+v, want %+v", name, got[0], caseStart(g, c))
				}
			}
		}
		got = nil
		err := checks[g.Name](0, nil)
		if !errors.Is(err, sentinel) || !strings.HasPrefix(err.Error(), g.Name+"/"+g.Cases[0].Name+": ") || len(got) != 1 {
			t.Errorf("group %s: err %v after %d calls", g.Name, err, len(got))
		}
	}
	for alias := range devAliases {
		got = nil
		if err := checks[alias](0, nil); !errors.Is(err, sentinel) || len(got) != 1 {
			t.Errorf("alias %s: err %v after %d calls", alias, err, len(got))
		}
	}
}

// In a group, the start comes before each case, in both directions.
func TestRunDevGroupStartsEachCase(t *testing.T) {
	var log []string
	stubApplyStart(t, func(_ uintptr, s startState) error {
		log = append(log, "start")
		return nil
	})
	mk := func(name string) devCase {
		return devCase{name, func(uintptr, []string) error { log = append(log, name); return nil }}
	}
	g := devGroup{"g", []devCase{mk("a"), mk("b")}, appStart()}
	if err := runDevGroup(0, g, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Join(log, " ") != "start a start b" {
		t.Fatalf("order = %v", log)
	}
	log = nil
	if err := runDevGroup(0, g, []string{"reverse"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(log, " ") != "start b start a" {
		t.Fatalf("reverse order = %v", log)
	}
}

// applyStart writes settings, so without the run's own settings folder it
// must stop before it touches the page.
func TestApplyStartNeedsSettingsDir(t *testing.T) {
	t.Setenv(devSettingsEnv, "")
	if err := applyStart(0, appStart()); err == nil || !strings.Contains(err.Error(), devSettingsEnv) {
		t.Fatalf("applyStart without %s: %v", devSettingsEnv, err)
	}
}

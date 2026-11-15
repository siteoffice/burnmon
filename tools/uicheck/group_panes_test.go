package main

import (
	"strings"
	"testing"
)

// d48Check: a clean read passes; each rule names its own failure.
func TestD48Check(t *testing.T) {
	good := d48Read{HeaderOneLine: true, HeaderKids: 10, Two: true, Room: 800, Status: 240, Sessions: 960, Processes: 960,
		Btns: []d48Btn{{ID: "secSizeSave", Gap: 0, LeftOf: true}}}
	if bad := d48Check(good, true); len(bad) != 0 {
		t.Fatalf("clean read failed: %v", bad)
	}
	r := good
	r.Bad = []string{"cut: #vendorStripPanel tr"}
	r.HeaderOneLine = false
	r.Sessions = 560 // the old "larger number" rule would draw 60 percent
	r.Btns = []d48Btn{{ID: "hist_export", Gap: 40, LeftOf: true}}
	got := strings.Join(d48Check(r, true), "\n")
	for _, want := range []string{"cut: #vendorStripPanel", "header is not one line", "side by side", "button hist_export"} {
		if !strings.Contains(got, want) {
			t.Errorf("d48Check lacks %q in:\n%s", want, got)
		}
	}
	// Below 1600 px the side by side rule does not apply.
	r = good
	r.Two = false
	if bad := d48Check(r, false); len(bad) != 0 {
		t.Errorf("narrow read failed: %v", bad)
	}
}

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

// d51Sizes reads "WxH" and "WxHp" (p: the section and pane percents of Wilco's
// window, finding 9 of the Glim fix lane) and skips anything else.
func TestD51Sizes(t *testing.T) {
	got := d51Sizes("1500x900, 1146x1554p ,bad,12x")
	want := []d51Size{{1500, 900, false}, {1146, 1554, true}}
	if len(got) != len(want) {
		t.Fatalf("d51Sizes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("d51Sizes[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	// The default list holds Wilco's window: a short window alone never reached it.
	found := false
	for _, s := range d51Defaults {
		if s.W == 1136 && s.H == 1554 && s.Pct {
			found = true
		}
	}
	if !found {
		t.Error("d51Defaults lacks 1146x1554 with Wilco's percents")
	}
	for _, want := range []string{"status:15", "sessions_30min:40", "processes_30min:35", "todo:10", "burn_30min:40", "vendor_strip:20", "activity_heatmap:20", "turn_ticker:20"} {
		if !strings.Contains(d51WilcoPct, want) {
			t.Errorf("d51WilcoPct lacks %s", want)
		}
	}
}

// d51Check: a month label inside the visible panel passes; no label with text
// is a data failure; text that exists but lies outside the visible box is the
// Activity bug of 2026-11-17 and its message carries the measured boxes.
func TestD51Check(t *testing.T) {
	good := d51Read{Labels: 4, Visible: 3, VisibleBottom: 140, MonthsBottom: 132, WrapH: 100, GridH: 100}
	if bad := d51Check(good); len(bad) != 0 {
		t.Fatalf("clean read failed: %v", bad)
	}
	cut := d51Read{Labels: 4, Visible: 0, VisibleBottom: 120, MonthsBottom: 133, WrapH: 100, GridH: 100}
	got := strings.Join(d51Check(cut), "\n")
	for _, want := range []string{"no month label", "month line ends 133", "visible panel bottom 120", "wrap 100", "grid 100"} {
		if !strings.Contains(got, want) {
			t.Errorf("d51Check lacks %q in:\n%s", want, got)
		}
	}
	// The month line hidden by the fit pass (class pct-cut) is named as such.
	hidden := d51Read{Labels: 4, Visible: 0, MonthsCut: true, VisibleBottom: 120, MonthsBottom: 0, WrapH: 18, GridH: 88}
	if got := strings.Join(d51Check(hidden), "\n"); !strings.Contains(got, "hidden by the fit pass") {
		t.Errorf("hidden month line not named in:\n%s", got)
	}
	empty := d51Read{}
	if bad := d51Check(empty); len(bad) != 1 || !strings.Contains(bad[0], "no month label has text") {
		t.Errorf("empty read = %v, want the no-text failure", bad)
	}
}

//go:build windows

package main

import (
	"strings"
	"testing"
)

// An Activity month label is as tall as its 10 px month line. With the normal
// line height of an 8 px font the label box is taller than the clipping line,
// and the whole-row pass (pctWhole) hides it as cut: no month name showed in
// Wilco's window (uicheck panes/d51 at 1136x1554 with his percents).
func TestPageMonthLabelFitsItsLine(t *testing.T) {
	line := ".heatmonths{position:relative; height:10px;"
	label := ".heatmonth{position:absolute; top:0; font-size:8px; line-height:10px;"
	for _, want := range []string{line, label} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page.html lacks %q", want)
		}
	}
}

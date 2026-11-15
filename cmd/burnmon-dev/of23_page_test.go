package main

import (
	"strings"
	"testing"
)

// OF23, amended 2026-11-15: the gear names Foreman nowhere. Approve, Send
// back and Close sprint moved to the Control Centre, so the BurnMon Dev hint
// speaks only of Stop and points there.
func TestPageHintNamesForemanOnlyWithRoot(t *testing.T) {
	s := strings.Index(pageHTML, `<div id="secControls">`)
	e := strings.Index(pageHTML, `<label class="setrow"><input type="checkbox" id="secToast">`)
	if s < 0 || e < s {
		t.Fatal("cannot find the gear's Controls block")
	}
	block := pageHTML[s:e]
	for _, bad := range []string{"foreman_root", "python on PATH", "Approve a gate"} {
		if strings.Contains(block, bad) {
			t.Errorf("the Controls block still names %q", bad)
		}
	}
	for _, want := range []string{"Stop a session needs claude_path and agent_view in burnmon.json.", "live in the Control Centre"} {
		if !strings.Contains(block, want) {
			t.Errorf("the Controls block lacks %q", want)
		}
	}
}

package main

import (
	"strings"
	"testing"
)

// ---- B6 fix pass B ---------------------------------------------------------

func TestPageYourTurnPillIsGrey(t *testing.T) {
	body := pageFuncBody(t, "mapPillClass")
	for _, want := range []string{"your turn", "can be closed", "'quiet'"} {
		if !strings.Contains(body, want) {
			t.Errorf("mapPillClass lacks %q", want)
		}
	}
	if !strings.Contains(pageHTML, ".mapbox .p.quiet{") {
		t.Error("page has no .mapbox .p.quiet rule")
	}
	if strings.Contains(pageHTML, ".mapbox.you{border-color:var(--amber)") {
		t.Error("the You box is amber without waits_on_you")
	}
	if !strings.Contains(pageHTML, ".mapbox.you.wait{border-color:var(--amber)") {
		t.Error("the You box is not amber when it waits on you")
	}
}

func TestPageMapIndentNoCap(t *testing.T) {
	if strings.Contains(pageHTML, "Math.min(b.depth") {
		t.Error("the map indent is still capped")
	}
	if !strings.Contains(pageHTML, "16 * (b.depth - 1)") {
		t.Error("the map indent is not 16 px per level")
	}
}

func TestPageOneDoneTier(t *testing.T) {
	if strings.Contains(pageHTML, "['can_close','Done, can be closed'") {
		t.Error("SEC_TIERS still has a separate can_close tier")
	}
	if !strings.Contains(pageHTML, "['done','Done, needs close-out'") {
		t.Error("SEC_TIERS has no done tier headed Done, needs close-out")
	}
	body := pageFuncBody(t, "secBodyWaits")
	if !strings.Contains(body, "Done, can be closed") || !strings.Contains(body, "close_out") {
		t.Error("secBodyWaits does not pick the Done header from the close_out rows")
	}
	if got := pageFuncBody(t, "secTier"); !strings.Contains(got, "return 'done'") {
		t.Error("secTier does not return the done tier")
	}
}

// Second look 2026-11-15: BurnMon Dev keeps one control, Stop. The gear has
// only its switch, and the page draws only Stop chips.
func TestPageGearHasOnlyStop(t *testing.T) {
	if n := strings.Count(pageHTML, `data-ctl-sw="`); n != 1 || !strings.Contains(pageHTML, `data-ctl-sw="stop"> Stop a session</label>`) {
		t.Errorf("the gear has %d control switches, want only Stop a session", n)
	}
	if !strings.Contains(pageHTML, "  var CTL_ORDER = ['stop'];") {
		t.Error("CTL_ORDER is not just stop")
	}
	if strings.Contains(pageHTML, "data-ctl-needs") {
		t.Error("the gear still has Foreman-only parts")
	}
}

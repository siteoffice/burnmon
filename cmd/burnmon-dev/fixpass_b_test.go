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

func TestPageForemanGearLinesNeedForeman(t *testing.T) {
	for _, sw := range []string{"approve", "send_back", "close_sprint"} {
		want := `<div class="ctlsw" data-ctl-needs="foreman"><label><input type="checkbox" data-ctl-sw="` + sw + `">`
		if !strings.Contains(pageHTML, want) {
			t.Errorf("the %s gear line is not marked data-ctl-needs=\"foreman\"", sw)
		}
	}
}

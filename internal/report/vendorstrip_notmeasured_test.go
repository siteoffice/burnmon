package report

import (
	"strings"
	"testing"
)

// The vendor strip cell of BurnMon prints the Go side's note: "not measured"
// for a Cowork (Cloud) period after the last measured row, a + after the number
// for the period that holds it, and the payload's cloud_tip as tooltip on both.
func TestVendorStripCellShowsCloudNote(t *testing.T) {
	s := strings.Index(tpl, "function vsCell(")
	if s < 0 {
		t.Fatal("template.html lacks vsCell")
	}
	body := tpl[s:]
	body = body[:strings.Index(body, "\n}\n")]
	for _, want := range []string{
		"note === 'not_measured' ? 'not measured'",
		"(note === 'partial' ? '+' : '')",
		"var tip = note ? cloudTip : 'Open in History'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("vsCell lacks %q", want)
		}
	}
	r := strings.Index(tpl, "function renderVendorStrip(")
	render := tpl[r : r+strings.Index(tpl[r:], "\n}\n")]
	for _, want := range []string{"r.today_note", "r.week_note", "r.month_note", "total.month_note", "payload.cloud_tip"} {
		if !strings.Contains(render, want) {
			t.Errorf("renderVendorStrip lacks %q", want)
		}
	}
}

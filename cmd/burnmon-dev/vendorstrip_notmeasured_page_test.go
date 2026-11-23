//go:build windows

package main

import (
	"strings"
	"testing"
)

// The Dev page's vendor strip prints the Go side's note on a Cowork (Cloud)
// cell: "not measured" instead of a 0, a + after the number, and the
// payload's cloud_tip as tooltip. Total uses the same helper.
func TestPageVendorStripShowsCloudNote(t *testing.T) {
	s := strings.Index(pageHTML, "function vendorCell(")
	if s < 0 {
		t.Fatal("page.html lacks vendorCell")
	}
	cell := pageHTML[s:]
	cell = cell[:strings.Index(cell, "\n  function renderVendorStrip(")]
	for _, want := range []string{
		"el.textContent = 'not measured'",
		"fmtTokens(n) + '+'",
		"el.title = cloudTip",
	} {
		if !strings.Contains(cell, want) {
			t.Errorf("vendorCell lacks %q", want)
		}
	}
	r := strings.Index(pageHTML, "function renderVendorStrip(")
	render := pageHTML[r:]
	render = render[:strings.Index(render, "function renderHeadline(")]
	for _, want := range []string{
		"vendorCell(rec.today, r.today, r.today_note, cloudTip)",
		"vendorCell(rec.month, r.month, r.month_note, cloudTip)",
		"vendorCell(total.month, tot.month || 0, tot.month_note, cloudTip)",
	} {
		if !strings.Contains(render, want) {
			t.Errorf("renderVendorStrip lacks %q", want)
		}
	}
}

// The reason is on screen, not only in a hover: one dim line under the strip
// says it while the pull is partial or failed, cut with an ellipsis and
// carrying the full text as its title.
func TestPageVendorStripShowsCloudReasonLine(t *testing.T) {
	i := strings.Index(pageHTML, `id="vendorStripNote"`)
	if i < 0 {
		t.Fatal("page.html has no vendorStripNote line")
	}
	line := pageHTML[i : i+strings.Index(pageHTML[i:], "</div>")]
	for _, want := range []string{"text-overflow:ellipsis", "white-space:nowrap", "hidden"} {
		if !strings.Contains(line, want) {
			t.Errorf("vendorStripNote lacks %q", want)
		}
	}
	r := strings.Index(pageHTML, "function renderVendorStrip(")
	render := pageHTML[r:]
	render = render[:strings.Index(render, "function renderHeadline(")]
	for _, want := range []string{"getElementById('vendorStripNote')", "cloud_pull === 'partial'", "cloud_pull === 'failed'", "note.title"} {
		if !strings.Contains(render, want) {
			t.Errorf("renderVendorStrip lacks %q", want)
		}
	}
}

//go:build windows

package main

import (
	"regexp"
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
	// The text and the link are read in full: the line wraps (a long address too) instead of
	// being cut, and it sits right below the panel, outside the panel's height clamp.
	for _, want := range []string{"white-space:normal", "overflow-wrap:anywhere", "hidden"} {
		if !strings.Contains(line, want) {
			t.Errorf("vendorStripNote lacks %q", want)
		}
	}
	for _, bad := range []string{"nowrap", "text-overflow", "overflow:hidden"} {
		if strings.Contains(line, bad) {
			t.Errorf("vendorStripNote still has %q, so it cuts the text", bad)
		}
	}
	if !regexp.MustCompile(`</table>\s*</div>\s*<div[^>]*id="vendorStripNote"`).MatchString(pageHTML) {
		t.Error("vendorStripNote is not right below the closed vendor strip panel")
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

// The burnmon-cloud mod's status rides on the same line (decision 3, placement
// by Wilco 2026-11-24): appended while the reason shows, and a real delivery
// shows on its own too.
func TestPageVendorStripShowsCloudModStatus(t *testing.T) {
	r := strings.Index(pageHTML, "function renderVendorStrip(")
	render := pageHTML[r:]
	render = render[:strings.Index(render, "function renderHeadline(")]
	for _, want := range []string{"payload.cloud_mod", "payload.cloud_mod_delivered"} {
		if !strings.Contains(render, want) {
			t.Errorf("renderVendorStrip lacks %q", want)
		}
	}
}

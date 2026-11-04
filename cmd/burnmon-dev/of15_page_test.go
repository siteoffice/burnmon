//go:build windows

package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestPageHasBurnReportControls(t *testing.T) {
	start := strings.Index(pageHTML, `id="histAdmin"`)
	if start < 0 {
		t.Fatal("histAdmin block missing")
	}
	end := start + strings.Index(pageHTML[start:], `Esc closes this list`)
	block := pageHTML[start:end]
	for _, id := range []string{"hist_burn_report", "hist_burn_names", "hist_burn_result"} {
		if !strings.Contains(block, `id="`+id+`"`) {
			t.Errorf("%s missing inside the histAdmin block", id)
		}
	}
	tag := regexp.MustCompile(`<input[^>]*id="hist_burn_names"[^>]*>`).FindString(block)
	if tag == "" {
		t.Fatal("hist_burn_names input not found")
	}
	if strings.Contains(tag, "checked") {
		t.Errorf("hist_burn_names must start unchecked: %s", tag)
	}
	if !strings.Contains(tag, `type="checkbox"`) {
		t.Errorf("hist_burn_names must be a checkbox: %s", tag)
	}
	if !strings.Contains(block, "show project names") {
		t.Error("label text 'show project names' missing")
	}
	if !regexp.MustCompile(`<button[^>]*id="hist_burn_report"[^>]*>Burn Report</button>`).MatchString(block) {
		t.Error("hist_burn_report button with text Burn Report missing")
	}
	for _, want := range []string{"bdevBurnReport", "__bdevBurnReportResolve"} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page lacks %s", want)
		}
	}
}

func TestPageHeaderExportUnchanged(t *testing.T) {
	if !strings.Contains(pageHTML, `<button class="textbtn" id="exportBtn" title="Export the last 24 hours to an AI-ready bundle">EXPORT</button>`) {
		t.Error("header EXPORT button changed")
	}
	if !strings.Contains(pageHTML, `<button id="hist_export" class="histbtn histend" type="button">Export</button>`) {
		t.Error("card Export button changed")
	}
}

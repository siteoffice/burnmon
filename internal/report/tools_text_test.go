package report

import (
	"regexp"
	"strings"
	"testing"
)

// TestToolsNoDataTextNamesNoVersion checks the empty Tools page text (D14 of
// the Mac test: it said "Tool counting starts with v0.5.0" in a v0.4.0
// build that already counts tools). It names no version, and it says which
// sessions are counted: only the claude adapter fills the tool counts, so
// Claude Code and Cowork.
func TestToolsNoDataTextNamesNoVersion(t *testing.T) {
	line := regexp.MustCompile(`(?m)^\s*tools_no_data_html:'([^']*)'`).FindStringSubmatch(tpl)
	if line == nil {
		t.Fatal("template.html lacks tools_no_data_html")
	}
	text := line[1]
	if v := regexp.MustCompile(`v\d+\.\d+`).FindString(text); v != "" {
		t.Errorf("tools_no_data_html names a version (%s): %q", v, text)
	}
	if !strings.Contains(text, "Claude Code and Cowork") {
		t.Errorf("tools_no_data_html does not say which sessions are counted: %q", text)
	}
}

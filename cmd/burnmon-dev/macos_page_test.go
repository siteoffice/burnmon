package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestMacPageNotOn(t *testing.T) {
	page := readText(t, "page.html")
	for _, want := range []string{
		"renderPressureChip(sm.pressure_score, sm.not_on)",
		"'PRESSURE not on ' + notOn",
		"'wifi not on ' + esc(sm.not_on)",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page.html lacks %q", want)
		}
	}
	main := readText(t, "main.go")
	if !regexp.MustCompile("NotOn\\s+string\\s+`json:\"not_on,omitempty\"`").MatchString(main) {
		t.Error("main.go sysmonNowPayload lacks NotOn with json tag not_on,omitempty")
	}
	if !regexp.MustCompile(`NotOn:\s*platformNotOn`).MatchString(main) {
		t.Error("main.go does not fill NotOn with platformNotOn")
	}
}

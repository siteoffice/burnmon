package main

import (
	"regexp"
	"strings"
	"testing"
)

// TestPageCostOneFunctionOneCurrency checks that BurnMon Dev prints every
// cost through fmtCost, in euro (D12 of the Mac test: the page showed $ in
// some places and the euro sign in others). BurnMon keeps its currency
// choice in its own page storage, which this app cannot read, so the page
// uses euro at BurnMon's default rate, 0.92.
func TestPageCostOneFunctionOneCurrency(t *testing.T) {
	page := readText(t, "page.html")
	if strings.Contains(page, "'$'") || strings.Contains(page, `"$"`) {
		t.Error("page.html still prints a dollar sign")
	}
	if strings.Contains(page, `">$<`) {
		t.Error("page.html still has a dollar sign in its markup (the header burn rate)")
	}
	if !strings.Contains(page, "rateEl.textContent = fmtCost(minutes > 0 ? cost / minutes : 0);") {
		t.Error("the header burn rate does not go through fmtCost")
	}
	if strings.Contains(page, "secEuro(") {
		t.Error("page.html still has secEuro, a second cost formatter")
	}
	if n := strings.Count(page, "€"); n != 1 {
		t.Errorf("page.html has %d euro signs, want 1 (inside fmtCost)", n)
	}
	re := regexp.MustCompile(`function fmtCost\(usd\)\{ return '\x{20ac}' \+ \(\(usd \|\| 0\) \* EUR_PER_USD\)\.toFixed\(2\); \}`)
	if !re.MatchString(page) {
		t.Error("page.html lacks fmtCost(usd) printing euro at EUR_PER_USD")
	}
	if !strings.Contains(page, "var EUR_PER_USD = 0.92;") {
		t.Error("page.html lacks EUR_PER_USD = 0.92, BurnMon's default rate")
	}
}

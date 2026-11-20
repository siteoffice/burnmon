package main

import (
	"strings"
	"testing"
)

// D8 of the Mac test: in the Status section at 15 percent the Agents map card was cut through
// its first line ("sessions, subagents, waiting on you"). The section body clips what does not
// fit (overflow:hidden). The card now lays its lines out as a column that wraps, so a line that
// does not fit moves to a second column outside the card and is hidden whole, never cut through.
// A line wider than the card is shortened with an ellipsis, and the totals line carries its full
// text as a hover title (UI standard rule 12).
func TestAgentsCardNeverCutsALine(t *testing.T) {
	for _, want := range []string{
		".mapopen{all:unset; box-sizing:border-box; display:flex; flex-direction:column; flex-wrap:wrap; align-content:flex-start; overflow:hidden; gap:3px; cursor:pointer; width:100%; flex:1 1 auto; min-height:0}",
		".mapopen > *{width:100%; box-sizing:border-box; flex:0 0 auto; white-space:nowrap; overflow:hidden; text-overflow:ellipsis}",
		`<div class="maptotals" title="`,
	} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page.html lacks %q", want)
		}
	}
}

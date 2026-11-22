package report

import (
	"regexp"
	"strings"
	"testing"
)

// cssRule returns the declarations of the first rule in the template whose selector is exactly sel.
func cssRule(t *testing.T, sel string) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(sel) + `\{([^}]*)\}`).FindStringSubmatch(tpl)
	if m == nil {
		t.Fatalf("template.html has no rule for %q", sel)
	}
	return m[1]
}

// TestSessionsTableColumnsAllHaveAWidth pins D10 of the Mac test. The Sessions table has a fixed
// layout, and the widths were written by column position when the Owner and Client columns
// (hidden unless used) sat before Findings. The Findings column, the 11th, had no width, so it
// shared the free width with "What it was" and both were cut. Every column must now have one,
// "What it was" is the only one left to take the free width, and the table has no minimum width
// that would force a horizontal scrollbar (UI standard rule 11).
func TestSessionsTableColumnsAllHaveAWidth(t *testing.T) {
	for n := 1; n <= 11; n++ {
		sel := `#t_sessions th:nth-child(` + itoa(n) + `),#t_sessions td:nth-child(` + itoa(n) + `)`
		rule := cssRule(t, sel)
		if n == 2 {
			if !strings.Contains(rule, "width:auto") {
				t.Errorf("column 2 (What it was) must take the free width: %q", rule)
			}
			continue
		}
		if !regexp.MustCompile(`width:\d+px`).MatchString(rule) {
			t.Errorf("column %d has no fixed width: %q", n, rule)
		}
	}
	if strings.Contains(cssRule(t, "#t_sessions"), "min-width") {
		t.Error("#t_sessions has a min-width: it forces a horizontal scrollbar in a narrow window (rule 11)")
	}
}

// TestSessionsTableTextsShowInFullOnHover pins rule 12 for the cut texts of D10: the model pills
// are cut with an ellipsis and the cell carries the full list; the Findings cell carries its
// summary as a title.
func TestSessionsTableTextsShowInFullOnHover(t *testing.T) {
	if !strings.Contains(cssRule(t, "#t_sessions td:nth-child(4) .pill"), "text-overflow:ellipsis") {
		t.Error("the model pills are not shortened with an ellipsis")
	}
	if !strings.Contains(tpl, `cell.title = `) {
		t.Error("the Findings cell does not get its summary as a hover title")
	}
}

// TestFilterRowFieldsShareOneHeight pins D11: the "Search title" input was taller than the
// selects next to it.
func TestFilterRowFieldsShareOneHeight(t *testing.T) {
	rule := cssRule(t, "#sessions .controls select,#sessions .controls input[type=text]")
	if !regexp.MustCompile(`height:\d+px`).MatchString(rule) {
		t.Errorf("the filter row selects and text input have no common height: %q", rule)
	}
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// sessionsColumnWidths returns the fixed px width of every Sessions column but "What it was"
// (column 2), keyed by its position.
func sessionsColumnWidths(t *testing.T) map[int]int {
	t.Helper()
	w := map[int]int{}
	for n := 1; n <= 11; n++ {
		if n == 2 {
			continue
		}
		sel := `#t_sessions th:nth-child(` + itoa(n) + `),#t_sessions td:nth-child(` + itoa(n) + `)`
		m := regexp.MustCompile(`width:(\d+)px`).FindStringSubmatch(cssRule(t, sel))
		if m == nil {
			t.Fatalf("column %d has no px width", n)
		}
		v := 0
		for _, c := range m[1] {
			v = v*10 + int(c-'0')
		}
		w[n] = v
	}
	return w
}

// TestSessionsWhatItWasKeepsRoomAtANarrowWindow pins D10 of the Mac retest. In a 960 px window
// the pane is 920 px (.wrap padding is 20 px a side), 905 px with the vertical scrollbar, and "What it was" must keep at least
// 200 px of it with the usual columns (Owner and Client hidden). Owner and Client (columns 9
// and 10) are hidden below 1100 px of window width so that, when both are in use, the pane
// of a window at 1101 px still leaves "What it was" its 200 px.
func TestSessionsWhatItWasKeepsRoomAtANarrowWindow(t *testing.T) {
	w := sessionsColumnWidths(t)
	base := 0
	for n, v := range w {
		if n != 9 && n != 10 {
			base += v
		}
	}
	if free := 905 - base; free < 200 {
		t.Errorf("at a 960 px window (905 px pane, 920 less a 15 px scrollbar) What it was keeps %d px, want 200 or more (fixed columns add up to %d)", free, base)
	}
	if free := (1101 - 40 - 15) - (base + w[9] + w[10]); free < 200 {
		t.Errorf("with Owner and Client shown at a 1101 px window What it was keeps %d px, want 200 or more", free)
	}
	if !regexp.MustCompile(`@media\(max-width:1100px\)\{[^}]*#t_sessions \.col-owner[^}]*display:none`).MatchString(tpl) {
		t.Error("Owner and Client are not hidden at 1100 px of window width and below")
	}
	if !regexp.MustCompile(`#t_sessions th,#t_sessions td\{[^}]*padding:7px 6px`).MatchString(tpl) {
		t.Error("the Sessions table has no tighter cell padding")
	}
}

// TestSessionsClientColumnShowsOnlyForARealClient: the client "unassigned" is what every session
// gets when no owner rule names a client, so it must not make the Client column (and its filter)
// show. The Owner and Client values of a column that is hidden stay in the Harness cell's title.
func TestSessionsClientColumnShowsOnlyForARealClient(t *testing.T) {
	if !strings.Contains(tpl, `var hasClient = (D.sessions||[]).some(function(s){ return !!s.client && s.client !== 'unassigned'; });`) {
		t.Error("hasClient counts the client 'unassigned' as a client")
	}
	if !strings.Contains(tpl, `' / Owner: '+esc(s.owner)`) || !strings.Contains(tpl, `' / Client: '+esc(s.client)`) {
		t.Error("the Harness cell title does not carry Owner and Client, so a hidden column loses them")
	}
}

// TestSessionsUICheckHookExists: tools\uicheck\check_w9.go shows its fixture through this hook,
// because D is private to the page script.
func TestSessionsUICheckHookExists(t *testing.T) {
	if !strings.Contains(tpl, "window.__bmSetSessions = function(list, insights)") {
		t.Error("the page has no window.__bmSetSessions hook for the w9 uicheck case")
	}
}

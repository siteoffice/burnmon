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

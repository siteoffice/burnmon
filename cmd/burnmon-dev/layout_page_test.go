//go:build windows

package main

import (
	"regexp"
	"strings"
	"testing"
)

// 2026-11-15: the Processes table is a pane of the Processes last 30 min
// section, so #processesHost sits inside that zone's body, after the process
// groups and before </main>; it is still not a #secPanels panel.
func TestProcessesHostAboveTodo(t *testing.T) {
	sys := strings.Index(pageHTML, `<section class="zone system">`)
	groups := strings.Index(pageHTML, `id="processGroupsPanel"`)
	host := strings.Index(pageHTML, `id="processesHost"`)
	mainEnd := strings.Index(pageHTML, "</main>")
	todo := strings.Index(pageHTML, `id="todoPanel"`)
	if sys < 0 || host < 0 || mainEnd < 0 || todo < 0 || !(sys < groups && groups < host && host < mainEnd && mainEnd < todo) {
		t.Fatalf("processesHost must sit in the system zone after the process groups: sys=%d groups=%d host=%d main=%d todo=%d", sys, groups, host, mainEnd, todo)
	}
	m := regexp.MustCompile(`var SEC_ORDER = \[([^\]]*)\];`).FindStringSubmatch(pageHTML)
	if m == nil {
		t.Fatal("SEC_ORDER not found")
	}
	if strings.Contains(m[1], "processes") {
		t.Errorf("SEC_ORDER still lists processes: %s", m[1])
	}
}

// B7 item 4: .todobody carries the .ticker house scrollbar.
func TestTodobodyHouseScrollbar(t *testing.T) {
	rule := regexp.MustCompile(`\.todobody\{[^}]*\}`).FindString(pageHTML)
	for _, want := range []string{"scrollbar-width:thin", "scrollbar-color:var(--sb)"} {
		if !strings.Contains(rule, want) {
			t.Errorf(".todobody rule lacks %s: %s", want, rule)
		}
	}
	for _, sfx := range []string{"::-webkit-scrollbar", "::-webkit-scrollbar-track", "::-webkit-scrollbar-thumb", "::-webkit-scrollbar-thumb:hover"} {
		tick := regexp.MustCompile(`\.ticker` + regexp.QuoteMeta(sfx) + `\{([^}]*)\}`).FindStringSubmatch(pageHTML)
		todo := regexp.MustCompile(`\.todobody` + regexp.QuoteMeta(sfx) + `\{([^}]*)\}`).FindStringSubmatch(pageHTML)
		if tick == nil || todo == nil {
			t.Errorf("missing %s rule: ticker=%v todobody=%v", sfx, tick != nil, todo != nil)
			continue
		}
		if tick[1] != todo[1] {
			t.Errorf("%s differs: ticker %q, todobody %q", sfx, tick[1], todo[1])
		}
	}
}

// B7 item 5: the History delete rows are a grid, ids unchanged.
func TestHistGrid(t *testing.T) {
	if !regexp.MustCompile(`#histAdmin \.histgrid\{[^}]*display:grid`).MatchString(pageHTML) {
		t.Error("#histAdmin has no display:grid rule")
	}
	for _, id := range []string{"hist_delete_older", "hist_delete_session", "hist_delete_all", "hist_export", "hist_days", "hist_session"} {
		if !strings.Contains(pageHTML, `id="`+id+`"`) {
			t.Errorf("page lacks id %s", id)
		}
	}
}

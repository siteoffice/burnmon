//go:build windows

package main

import (
	"regexp"
	"strings"
	"testing"
)

// B7 item 3: the Processes table sits in #processesHost inside #legacy, after
// the closing main and before To Do, and is no longer a #secPanels panel.
func TestProcessesHostAboveTodo(t *testing.T) {
	host := strings.Index(pageHTML, `id="processesHost"`)
	mainEnd := strings.Index(pageHTML, "</main>")
	todo := strings.Index(pageHTML, `id="todoPanel"`)
	if host < 0 || mainEnd < 0 || todo < 0 || !(mainEnd < host && host < todo) {
		t.Fatalf("processesHost must sit between </main> and #todoPanel: main=%d host=%d todo=%d", mainEnd, host, todo)
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

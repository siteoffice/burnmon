package main

import (
	"strings"
	"testing"
)

// The gear pane is taller than a small window (1488 CSS px of content,
// measured), so it scrolls inside the window. macOS hides a default scrollbar
// until the user scrolls, so the pane carries the scrollbar of .ticker, which
// stays visible. Only .ticker, .todobody, .seclist, .mapscroll and the pane
// itself get one.
func TestGearPaneHasTheHouseScrollbar(t *testing.T) {
	for _, want := range []string{
		"#secSettings{max-height:calc(100vh - 80px); overflow-y:auto; overflow-x:hidden}",
		"#secSettings{scrollbar-width:thin; scrollbar-color:var(--sb) var(--panel2)}",
		"#secSettings::-webkit-scrollbar{width:8px}",
		"#secSettings::-webkit-scrollbar-track{background:var(--panel2)}",
		"#secSettings::-webkit-scrollbar-thumb{background:var(--sb)}",
		"#secSettings::-webkit-scrollbar-thumb:hover{background:var(--sb-hover)}",
	} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page.html lacks %q", want)
		}
	}
}

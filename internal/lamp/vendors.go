package lamp

import "strings"

// vendorNames is a copy of HARNESS_LABEL in cmd\burnmon-dev\page.html, the
// names BurnMon Dev shows. TestVendorNamesEqualPageHarnessLabel fails when
// the two differ, so the page stays the one source.
var vendorNames = map[string]string{
	"claude":               "Claude Code",
	"claude-desktop":       "Claude Desktop",
	"codex":                "Codex",
	"copilot-cli":          "Copilot CLI",
	"copilot-vscode":       "Copilot (VS Code)",
	"hermes":               "Hermes",
	"wsl":                  "WSL",
	"burnmon-dev":          "BurnMon Dev (Go)",
	"burnmon-dev-webview2": "BurnMon Dev (WebView2)",
	"node":                 "node (unclassified)",
}

const anAgent = "An agent"

// Who is the name a line uses for the item with this id: the vendor's
// display name for "session:<vendor>:...", "An agent" for everything else.
func Who(id string) string {
	rest, ok := strings.CutPrefix(id, "session:")
	if !ok {
		return anAgent
	}
	vendor, _, ok := strings.Cut(rest, ":")
	if !ok {
		return anAgent
	}
	if name, ok := vendorNames[vendor]; ok {
		return name
	}
	return anAgent
}

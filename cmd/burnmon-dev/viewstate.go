//go:build windows

// viewstate.go remembers the one view preference the page has: light or dark
// (the L key); sections_view.go keeps the section switches in the same file. It lives in burnmon-dev-view.json next to burnmon-dev.db and
// is written by the app, never by hand and never into burnmon-dev.json, which
// is the user's own config and is never rewritten. The page is loaded with
// NavigateToString, whose origin is opaque, so localStorage cannot hold it.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type viewState struct {
	UITheme string `json:"ui_theme"`
}

func viewStatePath(dataDir string) string {
	return filepath.Join(dataDir, "burnmon-dev-view.json")
}

// loadUITheme returns "light" or "dark". A missing, unparsable or unknown
// value means dark, the default look.
func loadUITheme(dataDir string) string {
	b, err := os.ReadFile(viewStatePath(dataDir))
	if err != nil {
		return "dark"
	}
	var st viewState
	if json.Unmarshal(b, &st) != nil || st.UITheme != "light" {
		return "dark"
	}
	return "light"
}

// saveUITheme writes theme, which must be exactly "light" or "dark".
func saveUITheme(dataDir, theme string) error {
	if theme != "light" && theme != "dark" {
		return fmt.Errorf("ui theme %q is not light or dark", theme)
	}
	// The file also holds the sections choice (sections_view.go); keep it.
	rest := map[string]json.RawMessage{}
	if b, err := os.ReadFile(viewStatePath(dataDir)); err == nil {
		if json.Unmarshal(b, &rest) != nil || rest == nil {
			rest = map[string]json.RawMessage{}
		}
	}
	rest["ui_theme"] = json.RawMessage(strconv.Quote(theme))
	b, err := json.MarshalIndent(rest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(viewStatePath(dataDir), b, 0o644)
}

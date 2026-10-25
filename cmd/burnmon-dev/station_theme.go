//go:build windows

// station_theme.go saves the "Station starts as" setting. It is the only
// code that writes burnmon-dev.json, and it writes only the station_theme
// key: every other key keeps its raw JSON, so a setting this build does not
// know survives. An unparsable file is refused and left as it is.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func saveStationTheme(dataDir, theme string) error {
	if theme != "space" && theme != "site" {
		return fmt.Errorf("station theme %q is not space or site", theme)
	}
	path := filepath.Join(dataDir, "burnmon-dev.json")
	m := map[string]json.RawMessage{}
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(b, &m); err != nil {
			return fmt.Errorf("burnmon-dev.json is not valid JSON, not touching it: %w", err)
		}
		if m == nil {
			m = map[string]json.RawMessage{}
		}
	case os.IsNotExist(err):
	default:
		return err
	}
	v, _ := json.Marshal(theme)
	m["station_theme"] = v
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

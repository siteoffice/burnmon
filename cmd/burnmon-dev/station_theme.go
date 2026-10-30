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

// renameFile is os.Rename; a test swaps it to prove a failed save leaves the file alone.
var renameFile = os.Rename

func saveStationTheme(dataDir, theme string) error {
	if theme != "space" && theme != "site" {
		return fmt.Errorf("station theme %q is not space or site", theme)
	}
	path := filepath.Join(dataDir, "burnmon-dev.json")
	m := map[string]json.RawMessage{}
	// Read the same file loadDevConfig reads, so a first save into the uicheck
	// settings folder keeps the real file's other keys.
	b, err := os.ReadFile(devConfigReadPath(dataDir))
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
	// Write a temp file in the same directory, then rename it over the target, so a
	// crash mid-write never leaves a half-written burnmon-dev.json.
	tmp, err := os.CreateTemp(dataDir, "burnmon-dev.json.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	_, werr := tmp.Write(out)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(tmpName)
		return werr
	}
	if err := renameFile(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

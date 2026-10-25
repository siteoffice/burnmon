//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeDevJSON drops a burnmon-dev.json holding body into a fresh temp dir
// and returns the dir.
func writeDevJSON(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "burnmon-dev.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestLoadDevConfigStationTheme: "space" is the default, "site" is the only
// other value, anything else falls back to "space".
func TestLoadDevConfigStationTheme(t *testing.T) {
	cases := []struct {
		name string
		body string // empty means no file at all
		want string
	}{
		{"no file", "", "space"},
		{"key absent", `{"retention_days": 3}`, "space"},
		{"site", `{"station_theme": "site"}`, "site"},
		{"space", `{"station_theme": "space"}`, "space"},
		{"unknown", `{"station_theme": "moon"}`, "space"},
		{"wrong case", `{"station_theme": "Space"}`, "space"},
		{"quote injection", `{"station_theme": "x\";alert(1);//"}`, "space"},
		{"parse error", `{"station_theme": `, "space"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		if c.body != "" {
			dir = writeDevJSON(t, c.body)
		}
		if got := loadDevConfig(dir).StationTheme; got != c.want {
			t.Errorf("%s: StationTheme = %q, want %q", c.name, got, c.want)
		}
	}
}

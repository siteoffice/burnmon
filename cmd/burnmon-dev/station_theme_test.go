//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func devJSONPath(dir string) string { return filepath.Join(dir, "burnmon-dev.json") }

// TestSaveStationThemeKeepsOtherKeys: saving the theme writes only that key;
// every other key, known to the struct or not, keeps its value.
func TestSaveStationThemeKeepsOtherKeys(t *testing.T) {
	dir := writeDevJSON(t, `{"retention_days": 3, "microsoft_todo_enabled": true, "refresh_ms": 2000, "future": {"a": [1, 2]}}`)
	if err := saveStationTheme(dir, "site"); err != nil {
		t.Fatal(err)
	}
	cfg := loadDevConfig(dir)
	if cfg.StationTheme != "site" || cfg.RetentionDays != 3 || !cfg.MicrosoftTodoEnabled || cfg.RefreshMs != 2000 {
		t.Fatalf("config after save = %+v", cfg)
	}
	b, err := os.ReadFile(devJSONPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 5 {
		t.Fatalf("raw has %d keys, want 5: %s", len(raw), b)
	}
	var fut struct {
		A []int `json:"a"`
	}
	if err := json.Unmarshal(raw["future"], &fut); err != nil || len(fut.A) != 2 || fut.A[0] != 1 || fut.A[1] != 2 {
		t.Fatalf("future key lost: %s (%v)", raw["future"], err)
	}
}

func TestSaveStationThemeCreatesFile(t *testing.T) {
	dir := t.TempDir()
	if err := saveStationTheme(dir, "site"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(devJSONPath(dir)); err != nil {
		t.Fatal("file not created:", err)
	}
	if got := loadDevConfig(dir).StationTheme; got != "site" {
		t.Fatalf("theme = %q, want site", got)
	}
	if err := saveStationTheme(dir, "space"); err != nil {
		t.Fatal(err)
	}
	if got := loadDevConfig(dir).StationTheme; got != "space" {
		t.Fatalf("theme = %q, want space", got)
	}
}

func TestSaveStationThemeRefusesUnknown(t *testing.T) {
	const body = `{"retention_days": 3}`
	dir := writeDevJSON(t, body)
	for _, theme := range []string{"moon", "Space", ""} {
		if err := saveStationTheme(dir, theme); err == nil || !strings.Contains(err.Error(), "is not space or site") {
			t.Errorf("theme %q: err = %v, want the not-space-or-site refusal", theme, err)
		}
	}
	b, _ := os.ReadFile(devJSONPath(dir))
	if string(b) != body {
		t.Fatalf("file changed: %s", b)
	}
}

func TestSaveStationThemeRefusesUnparsableFile(t *testing.T) {
	const body = `{"retention_days": `
	dir := writeDevJSON(t, body)
	if err := saveStationTheme(dir, "site"); err == nil || !strings.Contains(err.Error(), "is not valid JSON") {
		t.Fatalf("err = %v, want the not-valid-JSON refusal", err)
	}
	noTempLeft(t, dir)
	b, _ := os.ReadFile(devJSONPath(dir))
	if string(b) != body {
		t.Fatalf("file changed: %s", b)
	}
}

func noTempLeft(t *testing.T, dir string) {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

// TestSaveStationThemeFailedRenameKeepsOriginal: when the final rename fails
// the user's file is byte for byte as it was and no temp file stays.
func TestSaveStationThemeFailedRenameKeepsOriginal(t *testing.T) {
	const body = `{
  "retention_days": 3
}`
	dir := writeDevJSON(t, body)
	boom := errors.New("rename refused")
	old := renameFile
	renameFile = func(string, string) error { return boom }
	defer func() { renameFile = old }()
	if err := saveStationTheme(dir, "site"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the rename error", err)
	}
	b, _ := os.ReadFile(devJSONPath(dir))
	if string(b) != body {
		t.Fatalf("file changed: %s", b)
	}
	noTempLeft(t, dir)
}

func TestSaveStationThemeLeavesNoTempOnSuccess(t *testing.T) {
	dir := writeDevJSON(t, `{"retention_days": 3}`)
	if err := saveStationTheme(dir, "site"); err != nil {
		t.Fatal(err)
	}
	noTempLeft(t, dir)
}

func TestSectionsPayloadCarriesStationTheme(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	if got := sectionsPayloadFor(dir, filepath.Join(dir, "none.json"), now).StationTheme; got != "space" {
		t.Fatalf("fresh dir: station_theme = %q, want space", got)
	}
	if err := saveStationTheme(dir, "site"); err != nil {
		t.Fatal(err)
	}
	p := sectionsPayloadFor(dir, filepath.Join(dir, "none.json"), now)
	if p.StationTheme != "site" {
		t.Fatalf("after save: station_theme = %q, want site", p.StationTheme)
	}
	b, _ := json.Marshal(p)
	var m map[string]json.RawMessage
	_ = json.Unmarshal(b, &m)
	if _, ok := m["station_theme"]; !ok {
		t.Fatal("payload JSON lacks station_theme")
	}
}

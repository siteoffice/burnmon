//go:build windows

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// fakeAppDataDir points appDataDir at a temp dir for one test, so no test
// here ever reads or writes the real %LOCALAPPDATA%\burnmon.
func fakeAppDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := appDataDir
	appDataDir = func() string { return dir }
	t.Cleanup(func() { appDataDir = old })
	return dir
}

// TestSettingsDirUnset: without the variable every settings path is the
// appDataDir path, the same as before B7.2.
func TestSettingsDirUnset(t *testing.T) {
	real := fakeAppDataDir(t)
	t.Setenv(settingsDirEnv, "")
	got := settingsDir()
	if got != real {
		t.Fatalf("settingsDir() = %q, want appDataDir %q", got, real)
	}
	if p := viewStatePath(got); p != filepath.Join(real, "burnmon-dev-view.json") {
		t.Fatalf("view path = %q", p)
	}
	if p := windowStatePath(got); p != filepath.Join(real, "burnmon-dev-window.json") {
		t.Fatalf("window path = %q", p)
	}
	if p := devConfigReadPath(got); p != filepath.Join(real, "burnmon-dev.json") {
		t.Fatalf("dev config path = %q", p)
	}
}

// TestSettingsDirCreatesFolder: a set variable naming a missing folder gets
// that folder made, and is returned as is.
func TestSettingsDirCreatesFolder(t *testing.T) {
	fakeAppDataDir(t)
	dir := filepath.Join(t.TempDir(), "a", "b")
	t.Setenv(settingsDirEnv, dir)
	if got := settingsDir(); got != dir {
		t.Fatalf("settingsDir() = %q, want %q", got, dir)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("folder not made: %v", err)
	}
}

// TestSettingsDirSavesLeaveRealFilesAlone: with the variable set, the view,
// window, sections and station theme saves all land in the settings folder,
// and the real files are unchanged byte for byte. burnmon-dev.json is read
// from the real folder until the settings folder has its own copy, and the
// first theme save keeps the real file's other keys.
func TestSettingsDirSavesLeaveRealFilesAlone(t *testing.T) {
	real := fakeAppDataDir(t)
	realFiles := map[string][]byte{
		"burnmon-dev.json":        []byte(`{"retention_days": 3, "microsoft_todo_enabled": true, "station_theme": "space"}`),
		"burnmon-dev-view.json":   []byte(`{"ui_theme": "dark"}`),
		"burnmon-dev-window.json": []byte(`{"X": 1, "Y": 2, "Width": 300, "Height": 200, "Maximized": false, "Monitor": "x"}`),
	}
	for name, b := range realFiles {
		if err := os.WriteFile(filepath.Join(real, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	set := t.TempDir()
	t.Setenv(settingsDirEnv, set)
	dir := settingsDir()
	if dir != set {
		t.Fatalf("settingsDir() = %q, want %q", dir, set)
	}

	if cfg := loadDevConfig(dir); cfg.RetentionDays != 3 || !cfg.MicrosoftTodoEnabled {
		t.Fatalf("before any save, config = %+v, want the real file's values", cfg)
	}
	if err := saveUITheme(dir, "light"); err != nil {
		t.Fatal(err)
	}
	on := true
	if err := applySectionsChange(dir, sectionsChange{Plain: &on}); err != nil {
		t.Fatal(err)
	}
	if err := saveWindowState(dir, windowState{X: 5, Y: 6, Width: 700, Height: 500, Monitor: "y"}); err != nil {
		t.Fatal(err)
	}
	if err := saveStationTheme(dir, "site"); err != nil {
		t.Fatal(err)
	}

	for name, want := range realFiles {
		got, err := os.ReadFile(filepath.Join(real, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("real %s changed: %s", name, got)
		}
		if _, err := os.Stat(filepath.Join(set, name)); err != nil {
			t.Fatalf("%s not written to the settings folder: %v", name, err)
		}
	}
	if entries, err := os.ReadDir(real); err != nil || len(entries) != len(realFiles) {
		t.Fatalf("real folder has %d entries, want %d (%v)", len(entries), len(realFiles), err)
	}
	if got := loadUITheme(dir); got != "light" {
		t.Fatalf("ui theme = %q, want light", got)
	}
	if !loadSections(dir).Plain {
		t.Fatal("plain mode not saved in the settings folder")
	}
	cfg := loadDevConfig(dir)
	if cfg.StationTheme != "site" || cfg.RetentionDays != 3 || !cfg.MicrosoftTodoEnabled {
		t.Fatalf("after save, config = %+v, want site with the real file's other keys", cfg)
	}
}

// TestDevConfigReadPathOnlyFallsBackForTheSettingsFolder: any other folder
// without a burnmon-dev.json reads its own (missing) file, never the real one.
func TestDevConfigReadPathOnlyFallsBackForTheSettingsFolder(t *testing.T) {
	fakeAppDataDir(t)
	t.Setenv(settingsDirEnv, t.TempDir())
	other := t.TempDir()
	if p := devConfigReadPath(other); p != filepath.Join(other, "burnmon-dev.json") {
		t.Fatalf("devConfigReadPath(other) = %q", p)
	}
}

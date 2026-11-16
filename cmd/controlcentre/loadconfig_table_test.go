//go:build windows || darwin

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// loadBurnmonConfig skips a folder that is named like a config file, and
// falls on to the next candidate.
func TestLoadBurnmonConfigSkipsAFolderNamedBurnmonJSON(t *testing.T) {
	data := t.TempDir()
	if err := os.Mkdir(filepath.Join(data, "burnmon.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "claudecost.json"), []byte(`{"foreman_root":"old-name"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadBurnmonConfig("", data).ForemanRoot; got != "old-name" {
		t.Errorf("ForemanRoot = %q, want old-name", got)
	}
}

// Both names in one folder: burnmon.json wins, in the exe folder and in the
// fallback.
func TestLoadBurnmonConfigBothNamesInOneFolder(t *testing.T) {
	data := t.TempDir()
	for name, root := range map[string]string{"burnmon.json": "new", "claudecost.json": "old"} {
		if err := os.WriteFile(filepath.Join(data, name), []byte(`{"foreman_root":"`+root+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := loadBurnmonConfig("", data).ForemanRoot; got != "new" {
		t.Errorf("ForemanRoot = %q, want new", got)
	}
	if got := loadBurnmonConfig(data, t.TempDir()).ForemanRoot; got != "new" {
		t.Errorf("ForemanRoot from the exe folder = %q, want new", got)
	}
}

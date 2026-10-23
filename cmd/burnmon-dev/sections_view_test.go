//go:build windows

package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/sections"
	"burnmon/internal/snapshot"
)

// Saving the theme must keep the sections, and saving the sections must keep
// the theme: both live in burnmon-dev-view.json.
func TestThemeAndSectionsShareTheViewFile(t *testing.T) {
	dir := t.TempDir()
	if err := saveUITheme(dir, "light"); err != nil {
		t.Fatal(err)
	}
	if err := saveSections(dir, sections.Simple()); err != nil {
		t.Fatal(err)
	}
	if got := loadUITheme(dir); got != "light" {
		t.Errorf("theme after saving sections = %q, want light", got)
	}
	if err := saveUITheme(dir, "dark"); err != nil {
		t.Fatal(err)
	}
	if got := loadSections(dir).PresetName(); got != "simple" {
		t.Errorf("sections after saving theme = %q, want simple", got)
	}
}

func TestSectionsPayloadFromFrozenExample(t *testing.T) {
	dir := t.TempDir()
	snapPath := filepath.Join("..", "..", "testdata", "snapshot_example.json")
	now := time.Date(2026, 10, 23, 10, 0, 7, 0, time.UTC)

	p := sectionsPayloadFor(dir, snapPath, now)
	if p.Preset != "developer" {
		t.Errorf("first run preset %q, want developer", p.Preset)
	}
	if p.Reading.Status != snapshot.StatusOK || p.Reading.AgeSeconds != 7 {
		t.Errorf("reading %+v", p.Reading)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"sections", "plain_mode", "preset", "reading", "order"} {
		if _, ok := m[k]; !ok {
			t.Errorf("payload has no %q", k)
		}
	}
}

func TestSetSectionsFromPage(t *testing.T) {
	dir := t.TempDir()
	// A preset name sets the whole shape.
	if err := applySectionsChange(dir, sectionsChange{Preset: "simple"}); err != nil {
		t.Fatal(err)
	}
	if got := loadSections(dir); got.PresetName() != "simple" || !got.Plain {
		t.Errorf("after preset simple: %+v", got)
	}
	// One switch.
	off := false
	if err := applySectionsChange(dir, sectionsChange{Section: "burn", On: &off}); err != nil {
		t.Fatal(err)
	}
	if got := loadSections(dir); got.On("burn") || got.PresetName() != "custom" {
		t.Errorf("after burn off: %+v", got)
	}
	// Plain Mode.
	on := true
	if err := applySectionsChange(dir, sectionsChange{Plain: &on}); err != nil {
		t.Fatal(err)
	}
	if !loadSections(dir).Plain {
		t.Error("plain mode not saved")
	}
	// Garbage is refused and leaves the file alone.
	before := loadSections(dir)
	for _, bad := range []sectionsChange{{Preset: "wild"}, {Section: "nope", On: &on}, {}} {
		if err := applySectionsChange(dir, bad); err == nil {
			t.Errorf("%+v: want an error", bad)
		}
	}
	if got := loadSections(dir); got.PresetName() != before.PresetName() || got.Plain != before.Plain {
		t.Errorf("a refused change altered the file: %+v", got)
	}
}

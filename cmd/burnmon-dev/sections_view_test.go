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
	if err := saveSections(dir, sections.Default().WithPlain(true)); err != nil {
		t.Fatal(err)
	}
	if got := loadUITheme(dir); got != "light" {
		t.Errorf("theme after saving sections = %q, want light", got)
	}
	if err := saveUITheme(dir, "dark"); err != nil {
		t.Fatal(err)
	}
	if got := loadSections(dir).Mode(); got != "plain" {
		t.Errorf("mode after saving theme = %q, want plain", got)
	}
}

func TestSectionsPayloadFromFrozenExample(t *testing.T) {
	dir := t.TempDir()
	snapPath := filepath.Join("..", "..", "testdata", "snapshot_example.json")
	now := time.Date(2026, 10, 23, 10, 0, 7, 0, time.UTC)

	p := sectionsPayloadFor(dir, snapPath, now)
	if p.Mode != "standard" || p.Plain {
		t.Errorf("first run mode %q, want standard", p.Mode)
	}
	if len(p.Sections) != 7 || !p.Sections["git"] {
		t.Errorf("current list %v: standard shows every section", p.Sections)
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
	for _, k := range []string{"mode", "plain_mode", "sections", "sections_standard", "sections_plain", "reading", "order"} {
		if _, ok := m[k]; !ok {
			t.Errorf("payload has no %q", k)
		}
	}
	// In Plain the page's current list is the Plain one.
	if err := saveSections(dir, sections.Default().WithPlain(true)); err != nil {
		t.Fatal(err)
	}
	if q := sectionsPayloadFor(dir, snapPath, now); q.Mode != "plain" || q.Sections["git"] {
		t.Errorf("plain payload: mode %q, git %v", q.Mode, q.Sections["git"])
	}
}

func TestSetSectionsFromPage(t *testing.T) {
	dir := t.TempDir()
	on, off := true, false

	// The toggle.
	if err := applySectionsChange(dir, sectionsChange{Plain: &on}); err != nil {
		t.Fatal(err)
	}
	if got := loadSections(dir); got.Mode() != "plain" {
		t.Errorf("after toggle on: %+v", got)
	}
	// One switch in one mode leaves the other mode alone.
	if err := applySectionsChange(dir, sectionsChange{Mode: "plain", Section: "git", On: &on}); err != nil {
		t.Fatal(err)
	}
	if err := applySectionsChange(dir, sectionsChange{Mode: "standard", Section: "burn", On: &off}); err != nil {
		t.Fatal(err)
	}
	got := loadSections(dir)
	if !got.OnIn("plain", "git") || got.OnIn("standard", "burn") || !got.OnIn("standard", "git") || !got.OnIn("plain", "burn") {
		t.Errorf("per-mode switches wrong: %+v", got)
	}
	if got.Mode() != "plain" {
		t.Error("a switch changed the mode")
	}
	// The toggle back.
	if err := applySectionsChange(dir, sectionsChange{Plain: &off}); err != nil {
		t.Fatal(err)
	}
	if loadSections(dir).Mode() != "standard" {
		t.Error("toggle off not saved")
	}
	// Garbage is refused and leaves the file alone.
	before := loadSections(dir)
	for _, bad := range []sectionsChange{
		{Mode: "wild", Section: "git", On: &on}, {Mode: "plain", Section: "nope", On: &on},
		{Mode: "plain", Section: "git"}, {Section: "git", On: &on}, {},
	} {
		if err := applySectionsChange(dir, bad); err == nil {
			t.Errorf("%+v: want an error", bad)
		}
	}
	after := loadSections(dir)
	if after.Mode() != before.Mode() || after.OnIn("plain", "git") != before.OnIn("plain", "git") || after.OnIn("standard", "burn") != before.OnIn("standard", "burn") {
		t.Errorf("a refused change altered the file: %+v", after)
	}
}

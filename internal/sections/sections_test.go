package sections

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSevenSectionsInOrder(t *testing.T) {
	want := []string{"waits", "outcomes", "burn", "agents_map", "sessions", "processes", "git"}
	if !reflect.DeepEqual(IDs, want) {
		t.Fatalf("IDs = %v, want %v", IDs, want)
	}
}

func TestPresets(t *testing.T) {
	dev := Developer()
	for _, id := range IDs {
		if !dev.On(id) {
			t.Errorf("developer: %s is off", id)
		}
	}
	if dev.Plain {
		t.Error("developer: plain mode is on")
	}

	simple := Simple()
	if !simple.Plain {
		t.Error("simple: plain mode is off")
	}
	for _, id := range []string{"waits", "outcomes", "burn", "agents_map", "sessions"} {
		if !simple.On(id) {
			t.Errorf("simple: %s is off", id)
		}
	}
	for _, id := range []string{"processes", "git"} {
		if simple.On(id) {
			t.Errorf("simple: %s is on", id)
		}
	}
}

func TestPresetNameIsDerived(t *testing.T) {
	if got := Simple().PresetName(); got != "simple" {
		t.Errorf("simple preset named %q", got)
	}
	if got := Developer().PresetName(); got != "developer" {
		t.Errorf("developer preset named %q", got)
	}
	s := Developer().With("git", false)
	if got := s.PresetName(); got != "custom" {
		t.Errorf("developer with git off named %q, want custom", got)
	}
	// Flipping one switch back to a preset's exact shape names it again.
	if got := s.With("git", true).PresetName(); got != "developer" {
		t.Errorf("restored developer named %q", got)
	}
	// Plain Mode is part of the shape.
	if got := Simple().WithPlain(false).PresetName(); got != "custom" {
		t.Errorf("simple with plain off named %q, want custom", got)
	}
}

func TestWithIgnoresUnknownSection(t *testing.T) {
	s := Developer().With("bogus", false)
	if !reflect.DeepEqual(s, Developer()) {
		t.Errorf("unknown id changed the state: %+v", s)
	}
}

func TestNormalizeFillsAndDrops(t *testing.T) {
	got := State{Sections: map[string]bool{"git": false, "nonsense": true}}.Normalize()
	if got.On("git") {
		t.Error("git switch lost")
	}
	if !got.On("burn") {
		t.Error("a section missing from a saved file must default to on")
	}
	if _, ok := got.Sections["nonsense"]; ok {
		t.Error("unknown section kept")
	}
	if len(got.Sections) != len(IDs) {
		t.Errorf("%d sections, want %d", len(got.Sections), len(IDs))
	}
}

func TestStoreRoundTripAndDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "view.json")
	if got := Load(path); !reflect.DeepEqual(got, Developer()) {
		t.Errorf("no file: %+v, want developer", got)
	}
	for _, want := range []State{Simple(), Developer().With("processes", false), Developer()} {
		if err := Save(path, want); err != nil {
			t.Fatal(err)
		}
		if got := Load(path); !reflect.DeepEqual(got, want) {
			t.Errorf("round trip: got %+v, want %+v", got, want)
		}
	}
}

// The view file also holds ui_theme. Saving sections must not drop it, and
// saving a theme must not drop the sections.
func TestSaveKeepsOtherKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view.json")
	if err := writeRaw(path, `{"ui_theme": "light"}`); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, Simple()); err != nil {
		t.Fatal(err)
	}
	if got := readRaw(t, path)["ui_theme"]; got != "light" {
		t.Errorf("ui_theme after Save = %v, want light", got)
	}
}

func TestLoadBadFileIsDeveloper(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view.json")
	for _, body := range []string{`{"sections": `, `{"sections": 3}`, `[]`, ``} {
		if err := writeRaw(path, body); err != nil {
			t.Fatal(err)
		}
		if got := Load(path); !reflect.DeepEqual(got, Developer()) {
			t.Errorf("body %q: %+v, want developer", body, got)
		}
	}
}

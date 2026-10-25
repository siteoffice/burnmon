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

// The first run is Standard with every section on. Plain starts with the
// placeholder set (the old Simple set): Processes and Git off.
func TestDefaults(t *testing.T) {
	d := Default()
	if d.Plain || d.Mode() != "standard" {
		t.Fatalf("default mode %q, want standard", d.Mode())
	}
	for _, id := range IDs {
		if !d.OnIn("standard", id) {
			t.Errorf("standard: %s is off", id)
		}
	}
	for _, id := range []string{"waits", "outcomes", "burn", "agents_map", "sessions"} {
		if !d.OnIn("plain", id) {
			t.Errorf("plain: %s is off", id)
		}
	}
	for _, id := range []string{"processes", "git"} {
		if d.OnIn("plain", id) {
			t.Errorf("plain: %s is on", id)
		}
	}
}

func TestOnFollowsTheMode(t *testing.T) {
	d := Default()
	if !d.On("git") {
		t.Error("standard: git should be on")
	}
	p := d.WithPlain(true)
	if p.Mode() != "plain" || p.On("git") {
		t.Errorf("plain: mode %q, git on %v", p.Mode(), p.On("git"))
	}
	if !p.On("burn") {
		t.Error("plain: burn should be on")
	}
}

// One list per mode: a switch in one mode never moves the other.
func TestWithChangesOnlyThatMode(t *testing.T) {
	d := Default().With("plain", "git", true).With("standard", "burn", false)
	if !d.OnIn("plain", "git") || d.OnIn("standard", "burn") {
		t.Fatalf("switches not set: %+v", d)
	}
	if !d.OnIn("standard", "git") || !d.OnIn("plain", "burn") {
		t.Fatalf("a switch leaked into the other mode: %+v", d)
	}
}

func TestWithIgnoresUnknownInput(t *testing.T) {
	d := Default()
	for _, c := range [][2]string{{"standard", "bogus"}, {"wild", "git"}, {"", "git"}} {
		if got := d.With(c[0], c[1], false); !reflect.DeepEqual(got, d) {
			t.Errorf("With(%q, %q) changed the state: %+v", c[0], c[1], got)
		}
	}
}

func TestNormalizeFillsAndDrops(t *testing.T) {
	got := State{Standard: map[string]bool{"git": false, "nonsense": true}}.Normalize()
	if got.OnIn("standard", "git") {
		t.Error("git switch lost")
	}
	if !got.OnIn("standard", "burn") {
		t.Error("a standard section missing from a saved file must default to on")
	}
	if got.OnIn("plain", "git") || !got.OnIn("plain", "burn") {
		t.Error("plain list missing from a saved file must default to the placeholder set")
	}
	if _, ok := got.Standard["nonsense"]; ok {
		t.Error("unknown section kept")
	}
	if len(got.Standard) != len(IDs) || len(got.PlainSet) != len(IDs) {
		t.Errorf("%d and %d sections, want %d each", len(got.Standard), len(got.PlainSet), len(IDs))
	}
}

// Saving then loading from a fresh Load call is what a restart does.
func TestStoreRoundTripAndDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view.json")
	if got := Load(path); !reflect.DeepEqual(got, Default()) {
		t.Errorf("no file: %+v, want default", got)
	}
	for _, want := range []State{
		Default().WithPlain(true),
		Default().With("standard", "processes", false).With("plain", "git", true),
		Default().With("plain", "burn", false).WithPlain(true),
		Default(),
	} {
		if err := Save(path, want); err != nil {
			t.Fatal(err)
		}
		if got := Load(path); !reflect.DeepEqual(got, want) {
			t.Errorf("round trip: got %+v, want %+v", got, want)
		}
	}
}

// The view file also holds ui_theme. Saving sections must not drop it.
func TestSaveKeepsOtherKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view.json")
	if err := writeRaw(path, `{"ui_theme": "light"}`); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, Default().WithPlain(true)); err != nil {
		t.Fatal(err)
	}
	if got := readRaw(t, path)["ui_theme"]; got != "light" {
		t.Errorf("ui_theme after Save = %v, want light", got)
	}
}

// The R8 first cut saved "sections" and "plain_mode". The mode carries over;
// the old single list is dropped from the file and never read as either list.
func TestOldR8FileKeepsModeOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view.json")
	old := `{"ui_theme": "dark", "plain_mode": true, "sections": {"burn": false, "git": true}}`
	if err := writeRaw(path, old); err != nil {
		t.Fatal(err)
	}
	got := Load(path)
	if !got.Plain {
		t.Error("plain_mode from the old file was lost")
	}
	if !got.OnIn("standard", "burn") || got.OnIn("plain", "git") {
		t.Errorf("old single list leaked into the new lists: %+v", got)
	}
	if err := Save(path, got); err != nil {
		t.Fatal(err)
	}
	raw := readRaw(t, path)
	if _, ok := raw["sections"]; ok {
		t.Error("old sections key still in the file after Save")
	}
	if raw["ui_theme"] != "dark" {
		t.Errorf("ui_theme = %v", raw["ui_theme"])
	}
}

func TestLoadBadFileIsDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view.json")
	for _, body := range []string{`{"sections_standard": `, `{"sections_standard": 3}`, `[]`, ``} {
		if err := writeRaw(path, body); err != nil {
			t.Fatal(err)
		}
		if got := Load(path); !reflect.DeepEqual(got, Default()) {
			t.Errorf("body %q: %+v, want default", body, got)
		}
	}
}

package sections

import (
	"path/filepath"
	"reflect"
	"testing"
)

var wantIDs = []string{
	"waits", "outcomes", "burn", "git", "agents_map", "sessions",
	"burn_30min", "vendor_strip", "activity", "heatmap", "turn_ticker",
	"system", "process_groups", "processes",
	"todo",
}

func onSet(ids ...string) map[string]bool {
	m := map[string]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func onlyOn(t *testing.T, name string, got map[string]bool, want ...string) {
	t.Helper()
	w := onSet(want...)
	if len(got) != len(IDs) {
		t.Errorf("%s: %d entries, want %d", name, len(got), len(IDs))
	}
	for _, id := range IDs {
		if got[id] != w[id] {
			t.Errorf("%s: %s is %v, want %v", name, id, got[id], w[id])
		}
	}
}

func TestFourGroupsInOrder(t *testing.T) {
	want := []Group{
		{ID: "status", Label: "Status", Parts: []Part{
			{"waits", "Waits on you"}, {"outcomes", "Outcomes"}, {"burn", "Burn"},
			{"git", "Git"}, {"agents_map", "Agents map"}, {"sessions", "Sessions"},
		}},
		{ID: "sessions_30min", Label: "Sessions last 30 min", Parts: []Part{
			{"burn_30min", "Burn last 30 min"}, {"vendor_strip", "Vendor strip"},
			{"activity", "Activity"}, {"heatmap", "Heatmap harness x minute"},
			{"turn_ticker", "Turn ticker"},
		}},
		{ID: "processes_30min", Label: "Processes last 30 min", Parts: []Part{
			{"system", "System"},
			{"process_groups", "Process groups (with memory, disks and network)"},
			{"processes", "Processes"},
		}},
		{ID: "todo", Label: "Microsoft To Do", Parts: []Part{{"todo", "To Do"}}},
	}
	if !reflect.DeepEqual(Groups, want) {
		t.Fatalf("Groups = %+v, want %+v", Groups, want)
	}
	if !reflect.DeepEqual(IDs, wantIDs) {
		t.Fatalf("IDs = %v, want %v", IDs, wantIDs)
	}
}

func TestDefaults(t *testing.T) {
	d := Default()
	if d.Plain || d.Mode() != "standard" {
		t.Fatalf("default mode %q, want standard", d.Mode())
	}
	if !d.Scrollbar {
		t.Error("default scrollbar is off")
	}
	onlyOn(t, "standard", d.Standard, wantIDs...)
	onlyOn(t, "plain", d.PlainSet, "waits", "outcomes", "burn", "agents_map", "sessions")
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

func TestWithChangesOnlyThatMode(t *testing.T) {
	d := Default().With("plain", "turn_ticker", true).With("standard", "burn", false)
	if !d.OnIn("plain", "turn_ticker") || d.OnIn("standard", "burn") {
		t.Fatalf("switches not set: %+v", d)
	}
	if !d.OnIn("standard", "turn_ticker") || !d.OnIn("plain", "burn") {
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

func TestWithScrollbar(t *testing.T) {
	d := Default()
	off := d.WithScrollbar(false)
	if off.Scrollbar {
		t.Fatal("scrollbar still on")
	}
	if !reflect.DeepEqual(off.Standard, d.Standard) || !reflect.DeepEqual(off.PlainSet, d.PlainSet) || off.Plain != d.Plain {
		t.Errorf("WithScrollbar changed more than the flag: %+v", off)
	}
	if !off.WithScrollbar(true).Scrollbar {
		t.Error("scrollbar did not come back on")
	}
}

func TestNormalizeFillsAndDrops(t *testing.T) {
	got := State{Scrollbar: true, Standard: map[string]bool{"git": false, "nonsense": true}}.Normalize()
	if got.OnIn("standard", "git") {
		t.Error("git switch lost")
	}
	if !got.OnIn("standard", "burn_30min") {
		t.Error("a standard part missing from a saved file must default to on")
	}
	if got.OnIn("plain", "git") || !got.OnIn("plain", "burn") {
		t.Error("plain list missing from a saved file must default to the Plain set")
	}
	if _, ok := got.Standard["nonsense"]; ok {
		t.Error("unknown part kept")
	}
	if len(got.Standard) != 15 || len(got.PlainSet) != 15 {
		t.Errorf("%d and %d parts, want 15 each", len(got.Standard), len(got.PlainSet))
	}
}

func TestStoreRoundTripAndDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view.json")
	if got := Load(path); !reflect.DeepEqual(got, Default()) {
		t.Errorf("no file: %+v, want default", got)
	}
	for _, want := range []State{
		Default().WithPlain(true),
		Default().With("standard", "processes", false).With("plain", "git", true),
		Default().With("plain", "burn", false).WithPlain(true),
		Default().WithScrollbar(false).WithPlain(true),
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

func TestScrollbarAbsentMeansOn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view.json")
	if err := writeRaw(path, `{"parts_standard": {}}`); err != nil {
		t.Fatal(err)
	}
	if !Load(path).Scrollbar {
		t.Error("absent overall_scrollbar loaded as off")
	}
	if err := writeRaw(path, `{"overall_scrollbar": false}`); err != nil {
		t.Fatal(err)
	}
	if Load(path).Scrollbar {
		t.Error("overall_scrollbar false loaded as on")
	}
}

func TestR8FileMigratesEverySwitch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view.json")
	old := `{"ui_theme":"dark","plain_mode":true,"sections_standard":{"waits":true,"outcomes":false,"burn":false,"agents_map":true,"sessions":true,"processes":false,"git":true},"sections_plain":{"waits":false,"outcomes":true,"burn":true,"agents_map":false,"sessions":true,"processes":true,"git":true}}`
	if err := writeRaw(path, old); err != nil {
		t.Fatal(err)
	}
	got := Load(path)
	if !got.Plain || !got.Scrollbar {
		t.Errorf("Plain %v Scrollbar %v, want true true", got.Plain, got.Scrollbar)
	}
	onlyOn(t, "standard", got.Standard, "waits", "agents_map", "sessions", "git", "todo")
	onlyOn(t, "plain", got.PlainSet, "outcomes", "burn", "sessions", "processes", "git")
	if err := Save(path, got); err != nil {
		t.Fatal(err)
	}
	raw := readRaw(t, path)
	for _, k := range []string{"parts_standard", "parts_plain"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("%s missing after Save", k)
		}
	}
	for _, k := range []string{"sections", "sections_standard", "sections_plain"} {
		if _, ok := raw[k]; ok {
			t.Errorf("%s still in the file after Save", k)
		}
	}
	if raw["overall_scrollbar"] != true || raw["plain_mode"] != true || raw["ui_theme"] != "dark" {
		t.Errorf("raw keys wrong: %v", raw)
	}
	if again := Load(path); !reflect.DeepEqual(again, got) {
		t.Errorf("reload %+v, want %+v", again, got)
	}
}

func TestR8DefaultsMigrateToNewDefaults(t *testing.T) {
	std := map[string]bool{"waits": true, "outcomes": true, "burn": true, "agents_map": true, "sessions": true, "processes": true, "git": true}
	pl := map[string]bool{"waits": true, "outcomes": true, "burn": true, "agents_map": true, "sessions": true, "processes": false, "git": false}
	if got := migrateR8(false, std, pl); !reflect.DeepEqual(got, Default()) {
		t.Errorf("migrateR8 of the R8 defaults = %+v, want %+v", got, Default())
	}
}

func TestR8FileWithOneListMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view.json")
	if err := writeRaw(path, `{"sections_standard": {"burn": false}}`); err != nil {
		t.Fatal(err)
	}
	got := Load(path)
	if !reflect.DeepEqual(got.PlainSet, Default().PlainSet) {
		t.Errorf("plain = %v, want the new Plain default", got.PlainSet)
	}
	off := map[string]bool{"burn": true, "burn_30min": true, "vendor_strip": true, "activity": true, "heatmap": true, "turn_ticker": true}
	for _, id := range IDs {
		if got.Standard[id] == off[id] {
			t.Errorf("standard %s = %v", id, got.Standard[id])
		}
	}
}

func TestOldR8FirstCutKeepsModeOnly(t *testing.T) {
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

func TestNewKeysWinOverR8Keys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view.json")
	body := `{"parts_standard": {"git": false}, "sections_standard": {"git": true, "burn": false}}`
	if err := writeRaw(path, body); err != nil {
		t.Fatal(err)
	}
	got := Load(path)
	if got.OnIn("standard", "git") {
		t.Error("new parts_standard git=false lost to the R8 key")
	}
	if !got.OnIn("standard", "burn") {
		t.Error("R8 burn=false was applied although parts_standard exists")
	}
	if err := Save(path, got); err != nil {
		t.Fatal(err)
	}
	if _, ok := readRaw(t, path)["sections_standard"]; ok {
		t.Error("sections_standard still in the file after Save")
	}
}

func TestLoadBadFileIsDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view.json")
	for _, body := range []string{`{"sections_standard": `, `{"sections_standard": 3}`, `{"parts_standard": 3}`, `[]`, ``} {
		if err := writeRaw(path, body); err != nil {
			t.Fatal(err)
		}
		if got := Load(path); !reflect.DeepEqual(got, Default()) {
			t.Errorf("body %q: %+v, want default", body, got)
		}
	}
}

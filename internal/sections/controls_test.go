package sections

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"burnmon/internal/controls"
)

func TestControlsDefaultOff(t *testing.T) {
	want := map[string]bool{"stop": false, "attach": false, "approve": false, "send_back": false}
	if got := Default().Controls; !reflect.DeepEqual(got, want) {
		t.Errorf("Default().Controls = %v, want every control off", got)
	}
	p := filepath.Join(t.TempDir(), "view.json")
	// A file without the key, and a file that holds only some keys: absent is off.
	if err := os.WriteFile(p, []byte(`{"plain_mode": true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(p).Controls; !reflect.DeepEqual(got, want) {
		t.Errorf("Load without controls = %v, want every control off", got)
	}
	if err := os.WriteFile(p, []byte(`{"controls": {"stop": true, "reply": true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load(p).Controls
	if !got["stop"] || got["attach"] || got["approve"] || got["send_back"] {
		t.Errorf("Load with only stop = %v", got)
	}
	if _, ok := got["reply"]; ok || len(got) != 4 {
		t.Errorf("unknown control id kept: %v", got)
	}
}

func TestControlsSaveLoadKeepsThemeKey(t *testing.T) {
	p := filepath.Join(t.TempDir(), "view.json")
	if err := os.WriteFile(p, []byte(`{"theme": "light"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(p, Default().WithControl("stop", true)); err != nil {
		t.Fatal(err)
	}
	st := Load(p)
	if !st.Controls["stop"] || st.Controls["attach"] {
		t.Errorf("after save and load: %v", st.Controls)
	}
	b, _ := os.ReadFile(p)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if string(m["theme"]) != `"light"` {
		t.Errorf("theme key lost: %s", b)
	}
	if st.WithControl("stop", false).Controls["stop"] {
		t.Error("WithControl off did not switch off")
	}
}

func TestWithControlUnknownChangesNothing(t *testing.T) {
	base := Default().WithControl("stop", true)
	for _, id := range []string{"reply", "", "Stop", "stop "} {
		if got := base.WithControl(id, true); !reflect.DeepEqual(got.Controls, base.Controls) {
			t.Errorf("WithControl(%q) changed %v to %v", id, base.Controls, got.Controls)
		}
	}
}

func TestControlIDsMatchControlsAll(t *testing.T) {
	if !reflect.DeepEqual(ControlIDs, controls.All) {
		t.Errorf("ControlIDs %v, controls.All %v", ControlIDs, controls.All)
	}
}

package lamp

import "testing"

func TestSettingsDefaults(t *testing.T) {
	want := Settings{Enabled: false, Position: "top-center", Language: "en", Lines: true, ForemanItems: true, BurnFloorTPM: 20000}
	if got := DefaultSettings(); got != want {
		t.Fatalf("DefaultSettings() = %+v, want %+v", got, want)
	}
	for _, in := range []string{"", "not json", "[1]", "{}", "null"} {
		if got := ParseSettings([]byte(in)); got != want {
			t.Errorf("ParseSettings(%q) = %+v, want defaults", in, got)
		}
	}
}

func TestSettingsValues(t *testing.T) {
	in := `{"retention_days":7,"lamp_enabled":true,"lamp_position":"top-left","lamp_language":"nl",
	"lamp_lines":false,"lamp_foreman_items":false,"lamp_burn_floor_tpm":5000}`
	want := Settings{Enabled: true, Position: "top-left", Language: "nl", Lines: false, ForemanItems: false, BurnFloorTPM: 5000}
	if got := ParseSettings([]byte(in)); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestSettingsBadValueBecomesDefault(t *testing.T) {
	in := `{"lamp_enabled":"yes","lamp_position":"middle","lamp_language":"de",
	"lamp_lines":1,"lamp_foreman_items":"no","lamp_burn_floor_tpm":-3}`
	if got := ParseSettings([]byte(in)); got != DefaultSettings() {
		t.Fatalf("got %+v, want defaults", got)
	}
	// One bad key does not spoil the good ones.
	got := ParseSettings([]byte(`{"lamp_enabled":true,"lamp_language":"fr","lamp_burn_floor_tpm":1.5}`))
	want := DefaultSettings()
	want.Enabled = true
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestSettingsNullKeyIsDefault(t *testing.T) {
	in := `{"lamp_enabled":null,"lamp_position":null,"lamp_language":null,
	"lamp_lines":null,"lamp_foreman_items":null,"lamp_burn_floor_tpm":null}`
	if got := ParseSettings([]byte(in)); got != DefaultSettings() {
		t.Fatalf("got %+v, want defaults", got)
	}
}

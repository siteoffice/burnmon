package lamp

import "testing"

func TestSettingsDefaults(t *testing.T) {
	want := Settings{Enabled: false, Position: "top-center", Language: "en", Lines: true, ForemanItems: true, BurnFloorTPM: 20000,
		TickerRows: 1, SizePct: 150, Theme: "system", Motion: "lively", Quiet: "hide", Backdrop: "clear"}
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
	want := DefaultSettings()
	want.Enabled, want.Position, want.Language, want.Lines, want.ForemanItems, want.BurnFloorTPM = true, "top-left", "nl", false, false, 5000
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

func TestNewLampKeys(t *testing.T) {
	in := `{"lamp_ticker_rows":4,"lamp_size":"large","lamp_theme":"dark","lamp_motion":"calm","lamp_quiet":"stay","lamp_backdrop":"card"}`
	got := ParseSettings([]byte(in))
	want := DefaultSettings()
	want.TickerRows, want.SizePct, want.Theme, want.Motion, want.Quiet, want.Backdrop = 4, 190, "dark", "calm", "stay", "card"
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for _, c := range []struct {
		in   string
		pick func(Settings) any
		want any
	}{
		{`{"lamp_ticker_rows":2}`, func(s Settings) any { return s.TickerRows }, 2},
		{`{"lamp_ticker_rows":3}`, func(s Settings) any { return s.TickerRows }, 3},
		{`{"lamp_size":"small"}`, func(s Settings) any { return s.SizePct }, 120},
		{`{"lamp_theme":"light"}`, func(s Settings) any { return s.Theme }, "light"},
		{`{"lamp_motion":"off"}`, func(s Settings) any { return s.Motion }, "off"},
	} {
		if got := c.pick(ParseSettings([]byte(c.in))); got != c.want {
			t.Errorf("%s gave %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNewLampKeyBadValueBecomesDefault(t *testing.T) {
	for _, in := range []string{
		`{"lamp_ticker_rows":0}`, `{"lamp_ticker_rows":5}`, `{"lamp_ticker_rows":2.5}`, `{"lamp_ticker_rows":"2"}`, `{"lamp_ticker_rows":-1}`,
		`{"lamp_size":"huge"}`, `{"lamp_size":2}`, `{"lamp_size":"Large"}`,
		`{"lamp_theme":"auto"}`, `{"lamp_theme":true}`,
		`{"lamp_motion":"fast"}`, `{"lamp_motion":0}`,
		`{"lamp_quiet":"show"}`, `{"lamp_quiet":false}`,
		`{"lamp_backdrop":"glass"}`, `{"lamp_backdrop":1}`,
		`{"lamp_ticker_rows":null,"lamp_size":null,"lamp_theme":null,"lamp_motion":null,"lamp_quiet":null,"lamp_backdrop":null}`,
	} {
		if got := ParseSettings([]byte(in)); got != DefaultSettings() {
			t.Errorf("%s gave %+v, want defaults", in, got)
		}
	}
	// One bad key does not spoil a good one.
	got := ParseSettings([]byte(`{"lamp_theme":"light","lamp_size":"huge"}`))
	want := DefaultSettings()
	want.Theme = "light"
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSizeScale(t *testing.T) {
	for size, want := range map[string]float64{"small": 1.2, "medium": 1.5, "large": 1.9, "": 1.5, "bogus": 1.5} {
		if got := SizeScale(size); got != want {
			t.Errorf("SizeScale(%q) = %v, want %v", size, got, want)
		}
	}
}

// lamp_monitor is the device name of the monitor the lamp hangs on. Any string
// is kept; empty (or a missing, null or wrong typed key) is the primary monitor.
func TestLampMonitorKey(t *testing.T) {
	if DefaultSettings().Monitor != "" {
		t.Errorf("default monitor = %q, want empty", DefaultSettings().Monitor)
	}
	for in, want := range map[string]string{
		`{"lamp_monitor":"DISPLAY3"}`:        "DISPLAY3",
		`{"lamp_monitor":""}`:                "",
		`{}`:                                 "",
		`{"lamp_monitor":null}`:              "",
		`{"lamp_monitor":3}`:                 "",
		`{"lamp_monitor":true}`:              "",
		`{"lamp_monitor":"\\\\.\\DISPLAY3"}`: `\\.\DISPLAY3`,
	} {
		if got := ParseSettings([]byte(in)).Monitor; got != want {
			t.Errorf("%s gave monitor %q, want %q", in, got, want)
		}
	}
}

// lamp_size is a percentage from 100 to 200 (default 150). The three old words
// still load as 120, 150 and 190. Anything else is 150.
func TestLampSizePct(t *testing.T) {
	if DefaultSettings().SizePct != 150 {
		t.Errorf("default size = %d, want 150", DefaultSettings().SizePct)
	}
	for in, want := range map[string]int{
		`{"lamp_size":100}`:      100,
		`{"lamp_size":200}`:      200,
		`{"lamp_size":150}`:      150,
		`{"lamp_size":137}`:      137, // a whole number in range is kept as saved
		`{"lamp_size":99}`:       150,
		`{"lamp_size":201}`:      150,
		`{"lamp_size":137.5}`:    150, // a fraction is not a whole number
		`{"lamp_size":"small"}`:  120,
		`{"lamp_size":"medium"}`: 150,
		`{"lamp_size":"large"}`:  190,
		`{"lamp_size":"huge"}`:   150,
		`{"lamp_size":true}`:     150,
		`{"lamp_size":null}`:     150,
		`{}`:                     150,
	} {
		if got := ParseSettings([]byte(in)).SizePct; got != want {
			t.Errorf("%s gave size %d, want %d", in, got, want)
		}
	}
}

func TestPctScale(t *testing.T) {
	for pct, want := range map[int]float64{100: 1, 150: 1.5, 200: 2, 120: 1.2, 0: 1.5, 99: 1.5, 201: 1.5} {
		if got := PctScale(pct); got != want {
			t.Errorf("PctScale(%d) = %v, want %v", pct, got, want)
		}
	}
}

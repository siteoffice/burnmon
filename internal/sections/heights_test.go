package sections

import (
	"math"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOldViewFileLoadsWithoutHeights(t *testing.T) {
	p := filepath.Join(t.TempDir(), "view.json")
	body := `{"plain_mode": true, "overall_scrollbar": false, "parts_standard": {"waits": false}, "parts_plain": {"burn": false}, "ui_theme": "example-dark"}`
	if err := writeRaw(p, body); err != nil {
		t.Fatal(err)
	}
	st := Load(p)
	if st.HeightsStandard != nil || st.HeightsPlain != nil {
		t.Fatalf("heights = %v / %v, want nil", st.HeightsStandard, st.HeightsPlain)
	}
	want := State{Plain: true, Scrollbar: false, Standard: map[string]bool{"waits": false}, PlainSet: map[string]bool{"burn": false}}.Normalize()
	want.Controls = st.Controls
	if !reflect.DeepEqual(st, want) {
		t.Fatalf("loaded %+v, want %+v", st, want)
	}
	if err := Save(p, st); err != nil {
		t.Fatal(err)
	}
	m := readRaw(t, p)
	if _, ok := m["heights_standard"]; ok {
		t.Fatal("heights_standard written")
	}
	if _, ok := m["heights_plain"]; ok {
		t.Fatal("heights_plain written")
	}
	if m["ui_theme"] != "example-dark" {
		t.Fatalf("ui_theme lost: %v", m["ui_theme"])
	}
}

func TestHeightsRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "view.json")
	h := map[string]float64{"burn_30min": 0.4, "turn_ticker": 0.6}
	if err := Save(p, Default().WithHeights("standard", h)); err != nil {
		t.Fatal(err)
	}
	st := Load(p)
	if !reflect.DeepEqual(st.HeightsStandard, h) {
		t.Fatalf("standard = %v, want %v", st.HeightsStandard, h)
	}
	if st.HeightsPlain != nil {
		t.Fatalf("plain = %v, want nil", st.HeightsPlain)
	}
	m := readRaw(t, p)
	if _, ok := m["heights_standard"]; !ok {
		t.Fatal("heights_standard missing")
	}
	if _, ok := m["heights_plain"]; ok {
		t.Fatal("heights_plain written")
	}
}

func TestHeightsNormalizeDropsBadInput(t *testing.T) {
	bad := map[string]float64{"example-part": 0.5, "burn_30min": 0, "vendor_strip": -0.2, "turn_ticker": 1.5, "system_chart": math.NaN(), "process_groups": math.Inf(1)}
	if got := Default().WithHeights("standard", bad).HeightsStandard; got != nil {
		t.Fatalf("got %v, want nil", got)
	}
	got := Default().WithHeights("plain", map[string]float64{"burn_30min": 1.0, "example-part": 0.5}).HeightsPlain
	if !reflect.DeepEqual(got, map[string]float64{"burn_30min": 1.0}) {
		t.Fatalf("got %v", got)
	}
}

func TestWithHeightsUnknownModeChangesNothing(t *testing.T) {
	s := Default().WithHeights("standard", map[string]float64{"burn_30min": 0.5})
	out := s.WithHeights("example-mode", map[string]float64{"turn_ticker": 0.5})
	if !reflect.DeepEqual(out, s.Normalize()) {
		t.Fatalf("changed: %+v", out)
	}
}

func TestResetHeightsClearsBothModes(t *testing.T) {
	s := Default().WithHeights("standard", map[string]float64{"burn_30min": 0.5}).WithHeights("plain", map[string]float64{"turn_ticker": 0.5})
	out := s.ResetHeights()
	if out.HeightsStandard != nil || out.HeightsPlain != nil {
		t.Fatalf("not cleared: %+v", out)
	}
}

func TestHeightsDoNotDisturbOtherSwitches(t *testing.T) {
	h := map[string]float64{"burn_30min": 0.4, "turn_ticker": 0.6}
	s := Default().WithHeights("standard", h)
	if got := s.With("plain", "todo", true).HeightsStandard; !reflect.DeepEqual(got, h) {
		t.Fatalf("With lost heights: %v", got)
	}
	if got := s.WithPlain(true).HeightsStandard; !reflect.DeepEqual(got, h) {
		t.Fatalf("WithPlain lost heights: %v", got)
	}
}

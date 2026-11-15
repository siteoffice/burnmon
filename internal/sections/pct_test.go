package sections

import (
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func near(t *testing.T, name string, got map[string]float64, want map[string]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %v, want %v", name, got, want)
	}
	for k, w := range want {
		if math.Abs(got[k]-w) > 0.001 {
			t.Fatalf("%s: got %v, want %v", name, got, want)
		}
	}
}

// Rule 2a: under 100, the rest key (Status) gets the rest.
func TestPlanUnderHundredRestGoesToRestKey(t *testing.T) {
	sp := Plan([]string{"status", "sessions_30min", "processes_30min"}, map[string]int{"status": 20, "sessions_30min": 50, "processes_30min": 20}, "status")
	near(t, "split", sp.Pct, map[string]float64{"status": 30, "sessions_30min": 50, "processes_30min": 20})
	if sp.Sum != 90 || sp.Over || sp.Starved != "" {
		t.Fatalf("got %+v", sp)
	}
}

// Rule 2a and 2e: the rest key hidden, the first shown key gets the rest.
func TestPlanRestKeyHiddenFirstShownGetsRest(t *testing.T) {
	sp := Plan([]string{"sessions_30min", "processes_30min"}, map[string]int{"status": 40, "sessions_30min": 30, "processes_30min": 20}, "status")
	near(t, "split", sp.Pct, map[string]float64{"sessions_30min": 80, "processes_30min": 20})
}

// Rule 2b: over 100 is reported; the caller decides if it is allowed.
func TestPlanOverHundred(t *testing.T) {
	sp := Plan([]string{"status", "sessions_30min"}, map[string]int{"status": 60, "sessions_30min": 52}, "status")
	if !sp.Over || sp.Sum != 112 {
		t.Fatalf("got %+v", sp)
	}
	near(t, "split", sp.Pct, map[string]float64{"status": 60, "sessions_30min": 52})
}

// Rule 2e: hidden keys do not count, their number is ignored.
func TestPlanHiddenKeysDoNotCount(t *testing.T) {
	sp := Plan([]string{"status", "todo"}, map[string]int{"status": 70, "sessions_30min": 90, "todo": 30}, "status")
	if sp.Sum != 100 || sp.Over {
		t.Fatalf("got %+v", sp)
	}
	near(t, "split", sp.Pct, map[string]float64{"status": 70, "todo": 30})
}

// Rule 2f: empty boxes share the rest equally; no numbers at all is equal shares.
func TestPlanEmptyBoxesShareTheRest(t *testing.T) {
	sp := Plan([]string{"status", "sessions_30min", "processes_30min", "todo"}, map[string]int{"status": 40}, "status")
	near(t, "one set", sp.Pct, map[string]float64{"status": 40, "sessions_30min": 20, "processes_30min": 20, "todo": 20})
	sp = Plan([]string{"status", "sessions_30min", "processes_30min", "todo"}, nil, "status")
	near(t, "fresh", sp.Pct, map[string]float64{"status": 25, "sessions_30min": 25, "processes_30min": 25, "todo": 25})
}

// Rule 2f: an empty box with nothing left is starved, named by its key.
func TestPlanEmptyBoxWithNothingLeftIsStarved(t *testing.T) {
	sp := Plan([]string{"status", "sessions_30min", "todo"}, map[string]int{"status": 60, "todo": 40}, "status")
	if sp.Starved != "sessions_30min" {
		t.Fatalf("got %+v", sp)
	}
}

// Rule 2c: panes use the same plan with the first shown pane as rest key.
func TestPlanPanesFirstShownGetsRest(t *testing.T) {
	s := Default()
	shown := s.ShownPanes(ModeStandard, "sessions_30min")
	if !reflect.DeepEqual(shown, []string{"burn_30min", "vendor_strip", "activity_heatmap", "turn_ticker"}) {
		t.Fatalf("shown = %v", shown)
	}
	sp := Plan(shown, map[string]int{"burn_30min": 30, "vendor_strip": 20, "activity_heatmap": 20, "turn_ticker": 20}, shown[0])
	near(t, "panes", sp.Pct, map[string]float64{"burn_30min": 40, "vendor_strip": 20, "activity_heatmap": 20, "turn_ticker": 20})
}

// Rule 2d: Activity and Heatmap are one pane; it is shown while either is on.
func TestActivityAndHeatmapShareOnePane(t *testing.T) {
	var n int
	for _, p := range Panes {
		for _, part := range p.Parts {
			if part == Activity || part == Heatmap {
				n++
				if p.ID != "activity_heatmap" {
					t.Fatalf("%s sits in pane %s", part, p.ID)
				}
			}
		}
	}
	if n != 2 {
		t.Fatalf("found %d of the two parts", n)
	}
	s := Default().With(ModeStandard, Activity, false)
	if !contains(s.ShownPanes(ModeStandard, "sessions_30min"), "activity_heatmap") {
		t.Fatal("pane hidden with heatmap still on")
	}
	s = s.With(ModeStandard, Heatmap, false)
	if contains(s.ShownPanes(ModeStandard, "sessions_30min"), "activity_heatmap") {
		t.Fatal("pane shown with both off")
	}
}

func contains(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

// Every part sits in exactly one pane, and every pane in a known section.
func TestPanesCoverEveryPartOnce(t *testing.T) {
	seen := map[string]int{}
	secs := map[string]bool{}
	for _, g := range Groups {
		secs[g.ID] = true
	}
	for _, p := range Panes {
		if !secs[p.Section] {
			t.Fatalf("pane %s in unknown section %s", p.ID, p.Section)
		}
		for _, part := range p.Parts {
			seen[part]++
		}
	}
	for _, id := range IDs {
		if seen[id] != 1 {
			t.Fatalf("part %s sits in %d panes", id, seen[id])
		}
	}
}

// Rule 2e: a section is shown while one of its parts is on.
func TestShownSectionsFollowParts(t *testing.T) {
	s := Default()
	if got := s.ShownSections(ModeStandard); !reflect.DeepEqual(got, []string{"status", "sessions_30min", "processes_30min", "todo"}) {
		t.Fatalf("standard = %v", got)
	}
	if got := s.ShownSections(ModePlain); !reflect.DeepEqual(got, []string{"status"}) {
		t.Fatalf("plain = %v", got)
	}
	for _, id := range []string{System, ProcessGroups} {
		s = s.With(ModeStandard, id, false)
	}
	if !contains(s.ShownSections(ModeStandard), "processes_30min") {
		t.Fatal("processes section hidden while Processes is on")
	}
	s = s.With(ModeStandard, Processes, false)
	if contains(s.ShownSections(ModeStandard), "processes_30min") {
		t.Fatal("processes section shown with all three off")
	}
}

func TestCheckPctSectionsOverHundredNeedsScrollbar(t *testing.T) {
	secs := map[string]int{"status": 40, "sessions_30min": 40, "processes_30min": 20, "todo": 12}
	on := Default().WithScrollbar(true)
	if err := on.CheckPct(ModeStandard, secs, nil); err != nil {
		t.Fatalf("scrollbar on: %v", err)
	}
	err := on.WithScrollbar(false).CheckPct(ModeStandard, secs, nil)
	if err == nil || err.Error() != "Sections add up to 112 percent; switch on Overall scrollbar or lower a number" {
		t.Fatalf("scrollbar off: %v", err)
	}
}

func TestCheckPctPanesOverHundredRefused(t *testing.T) {
	panes := map[string]int{"burn_30min": 60, "vendor_strip": 50}
	err := Default().CheckPct(ModeStandard, nil, panes)
	if err == nil || err.Error() != "Panes in Sessions last 30 min add up to 110 percent; lower a number" {
		t.Fatalf("got %v", err)
	}
	// The scrollbar never allows panes over 100.
	if Default().WithScrollbar(true).CheckPct(ModeStandard, nil, panes) == nil {
		t.Fatal("scrollbar allowed panes over 100")
	}
}

func TestCheckPctHiddenNumbersIgnored(t *testing.T) {
	// Plain shows only Status: Sessions 90 is hidden there and does not count.
	if err := Default().WithScrollbar(false).CheckPct(ModePlain, map[string]int{"status": 50, "sessions_30min": 90}, nil); err != nil {
		t.Fatalf("got %v", err)
	}
	// A hidden pane's number does not count either.
	s := Default().With(ModeStandard, VendorStrip, false)
	if err := s.CheckPct(ModeStandard, nil, map[string]int{"burn_30min": 60, "vendor_strip": 60}); err != nil {
		t.Fatalf("got %v", err)
	}
}

func TestCheckPctStarvedEmptyBox(t *testing.T) {
	err := Default().CheckPct(ModeStandard, map[string]int{"status": 50, "sessions_30min": 30, "processes_30min": 20}, nil)
	if err == nil || err.Error() != "Sections add up to 100 percent and Microsoft To Do has no number; give it a number or lower another" {
		t.Fatalf("got %v", err)
	}
	err = Default().CheckPct(ModeStandard, nil, map[string]int{"system": 70, "process_groups": 30})
	if err == nil || err.Error() != "Panes in Processes last 30 min add up to 100 percent and Processes has no number; give it a number or lower another" {
		t.Fatalf("got %v", err)
	}
}

func TestCheckPctRefusesUnknownKeysAndRange(t *testing.T) {
	for name, c := range map[string][2]map[string]int{
		"section key": {{"example-section": 10}, nil},
		"pane key":    {nil, {"example-pane": 10}},
		"zero":        {{"status": 0}, nil},
		"over":        {{"status": 101}, nil},
		"negative":    {nil, {"burn_30min": -5}},
	} {
		if err := Default().CheckPct(ModeStandard, c[0], c[1]); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
	if err := Default().CheckPct("example-mode", nil, nil); err == nil || !strings.Contains(err.Error(), "mode") {
		t.Fatalf("mode: %v", err)
	}
}

func TestPctRoundTripPerMode(t *testing.T) {
	p := filepath.Join(t.TempDir(), "view.json")
	secs := map[string]int{"status": 30, "sessions_30min": 50, "processes_30min": 20}
	panes := map[string]int{"burn_30min": 40}
	s := Default().WithPct(ModeStandard, secs, panes)
	if err := Save(p, s); err != nil {
		t.Fatal(err)
	}
	st := Load(p)
	if !reflect.DeepEqual(st.SectionsPctOf(ModeStandard), secs) || !reflect.DeepEqual(st.PanesPctOf(ModeStandard), panes) {
		t.Fatalf("loaded %+v / %+v", st.SectionsPct, st.PanesPct)
	}
	// Plain was not touched: it keeps the defaults.
	if !reflect.DeepEqual(st.SectionsPctOf(ModePlain), DefaultSectionsPct) || !reflect.DeepEqual(st.PanesPctOf(ModePlain), DefaultPanesPct) {
		t.Fatalf("plain lost its defaults: %+v / %+v", st.SectionsPct, st.PanesPct)
	}
	m := readRaw(t, p)
	if _, ok := m["sections_pct"]; !ok {
		t.Fatal("sections_pct missing")
	}
	if _, ok := m["panes_pct"]; !ok {
		t.Fatal("panes_pct missing")
	}
	// Other switches keep the numbers.
	if got := st.With(ModePlain, Todo, true).WithPlain(true).WithScrollbar(false).SectionsPctOf(ModeStandard); !reflect.DeepEqual(got, secs) {
		t.Fatalf("lost: %v", got)
	}
	// An empty map clears one mode, and a cleared mode stays empty after a
	// save and a load (it does not fall back to the defaults).
	cleared := st.WithPct(ModeStandard, nil, nil)
	if len(cleared.SectionsPctOf(ModeStandard)) != 0 || len(cleared.PanesPctOf(ModeStandard)) != 0 {
		t.Fatalf("not cleared: %+v / %+v", cleared.SectionsPct, cleared.PanesPct)
	}
	if err := Save(p, cleared); err != nil {
		t.Fatal(err)
	}
	if got := Load(p); len(got.SectionsPctOf(ModeStandard)) != 0 || len(got.PanesPctOf(ModeStandard)) != 0 {
		t.Fatalf("cleared mode came back as %+v / %+v", got.SectionsPct, got.PanesPct)
	}
}

func TestPctNormalizeDropsBadInput(t *testing.T) {
	s := State{SectionsPct: map[string]map[string]int{"standard": {"status": 30, "example-section": 10, "todo": 0, "sessions_30min": 140}, "example-mode": {"status": 20}}}.Normalize()
	if !reflect.DeepEqual(s.SectionsPct, map[string]map[string]int{"standard": {"status": 30}, "plain": {}}) {
		t.Fatalf("got %v", s.SectionsPct)
	}
}

// Prefilled numbers (Wilco, 2026-11-15): a fresh install and a file without
// the keys start with the defaults; they pass every rule in every mode with
// the default scrollbar on, and add up to more than 100 so the page scrolls.
func TestDefaultsPrefilledAndValid(t *testing.T) {
	if got := Default().SectionsPctOf(ModeStandard); !reflect.DeepEqual(got, DefaultSectionsPct) {
		t.Fatalf("Default standard = %v", got)
	}
	if got := Load(filepath.Join(t.TempDir(), "missing.json")).PanesPctOf(ModePlain); !reflect.DeepEqual(got, DefaultPanesPct) {
		t.Fatalf("missing file plain panes = %v", got)
	}
	p := filepath.Join(t.TempDir(), "view.json")
	if err := writeRaw(p, `{"plain_mode": false}`); err != nil {
		t.Fatal(err)
	}
	if got := Load(p).SectionsPctOf(ModePlain); !reflect.DeepEqual(got, DefaultSectionsPct) {
		t.Fatalf("file without keys = %v", got)
	}
	sum := 0
	for _, v := range DefaultSectionsPct {
		sum += v
	}
	if sum != 160 {
		t.Fatalf("default sections add up to %d, want 160", sum)
	}
	if !Default().Scrollbar {
		t.Fatal("the default has the scrollbar off; the defaults need it on")
	}
	for _, mode := range []string{ModeStandard, ModePlain} {
		if err := Default().CheckPct(mode, DefaultSectionsPct, DefaultPanesPct); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
	}
	// Standard shows every section, so with the scrollbar off the sum is refused.
	if err := Default().WithScrollbar(false).CheckPct(ModeStandard, DefaultSectionsPct, DefaultPanesPct); err == nil {
		t.Fatal("160 accepted with the scrollbar off")
	}
}

// Step 1: an old saved "heights" key is ignored on load, no error, and the
// next save drops it.
func TestOldHeightsKeysIgnoredAndDropped(t *testing.T) {
	p := filepath.Join(t.TempDir(), "view.json")
	body := `{"plain_mode": true, "overall_scrollbar": false, "parts_standard": {"waits": false}, "heights_standard": {"burn_30min": 0.4, "host_lists": 0.3}, "heights_plain": {"turn_ticker": 0.5}, "heights": {"burn_30min": 0.4}, "ui_theme": "example-dark"}`
	if err := writeRaw(p, body); err != nil {
		t.Fatal(err)
	}
	st := Load(p)
	want := State{Plain: true, Scrollbar: false, Standard: map[string]bool{"waits": false},
		SectionsPct: defaultPct(DefaultSectionsPct), PanesPct: defaultPct(DefaultPanesPct)}.Normalize()
	if !reflect.DeepEqual(st, want) {
		t.Fatalf("loaded %+v, want %+v", st, want)
	}
	if err := Save(p, st); err != nil {
		t.Fatal(err)
	}
	m := readRaw(t, p)
	for _, k := range []string{"heights_standard", "heights_plain", "heights"} {
		if _, ok := m[k]; ok {
			t.Fatalf("%s kept", k)
		}
	}
	if m["ui_theme"] != "example-dark" {
		t.Fatalf("ui_theme lost: %v", m["ui_theme"])
	}
}

package sections

import (
	"path/filepath"
	"reflect"
	"testing"
)

// The layout a fresh install starts with (Wilco, 2026-11-16), one set per mode.
// A hidden part keeps its number so it has a size when it is ticked.
var wantInstallSections = map[string]map[string]int{
	ModeStandard: {"status": 15, "sessions_30min": 40, "processes_30min": 45, "todo": 10},
	ModePlain:    {"status": 15, "sessions_30min": 40, "processes_30min": 45, "todo": 10},
}

var wantInstallPanes = map[string]map[string]int{
	ModeStandard: {
		"status_row1": 60, "sessions": 40,
		"burn_30min": 40, "vendor_strip": 20, "activity_heatmap": 20, "turn_ticker": 20,
		"system": 50, "process_groups": 50, "processes": 20,
	},
	ModePlain: {
		"status_row1": 60, "sessions": 40,
		"burn_30min": 50, "vendor_strip": 50, "activity_heatmap": 20, "turn_ticker": 20,
		"system": 100, "process_groups": 50, "processes": 20,
	},
}

var wantInstallOn = map[string][]string{
	ModeStandard: {Waits, AgentsMap, Sessions, Burn30, VendorStrip, Activity, Heatmap, TurnTicker, System, ProcessGroups},
	ModePlain:    {Waits, AgentsMap, Sessions, Burn30, VendorStrip, System},
}

func TestInstallDefaultsAreTheTable(t *testing.T) {
	d := Default()
	if d.Scrollbar {
		t.Error("the Overall scrollbar is on after install, want off")
	}
	if d.Plain {
		t.Error("a fresh install starts in Standard")
	}
	for mode, ids := range wantInstallOn {
		on := map[string]bool{}
		for _, id := range ids {
			on[id] = true
		}
		for _, id := range IDs {
			if d.OnIn(mode, id) != on[id] {
				t.Errorf("%s %s: on = %v, want %v", mode, id, d.OnIn(mode, id), on[id])
			}
		}
	}
	for _, mode := range []string{ModeStandard, ModePlain} {
		if got := d.SectionsPctOf(mode); !reflect.DeepEqual(got, wantInstallSections[mode]) {
			t.Errorf("%s sections = %v, want %v", mode, got, wantInstallSections[mode])
		}
		if got := d.PanesPctOf(mode); !reflect.DeepEqual(got, wantInstallPanes[mode]) {
			t.Errorf("%s panes = %v, want %v", mode, got, wantInstallPanes[mode])
		}
	}
	for _, id := range ControlIDs {
		if d.Controls[id] {
			t.Errorf("control %s is on after install", id)
		}
	}
}

// The defaults pass every rule in both modes with the scrollbar off, and the
// shown sections and the shown panes of each shown section add up to 100.
func TestInstallDefaultsPassCheckPctAndAddUpTo100(t *testing.T) {
	d := Default()
	for _, mode := range []string{ModeStandard, ModePlain} {
		secs, panes := d.SectionsPctOf(mode), d.PanesPctOf(mode)
		if err := d.CheckPct(mode, secs, panes); err != nil {
			t.Errorf("%s: %v", mode, err)
		}
		sum := 0
		for _, s := range d.ShownSections(mode) {
			sum += secs[s]
			pane := 0
			for _, p := range d.ShownPanes(mode, s) {
				pane += panes[p]
			}
			if pane != 100 {
				t.Errorf("%s: the shown panes of %s add up to %d, want 100", mode, s, pane)
			}
		}
		if sum != 100 {
			t.Errorf("%s: the shown sections add up to %d, want 100", mode, sum)
		}
	}
	if got := d.ShownSections(ModeStandard); !reflect.DeepEqual(got, []string{"status", "sessions_30min", "processes_30min"}) {
		t.Errorf("standard shows %v", got)
	}
}

// A part that is hidden at install keeps its number, so ticking it later
// still has a size; the number is checked again when it is shown.
func TestInstallDefaultsHiddenPartsKeepTheirNumber(t *testing.T) {
	d := Default()
	if d.SectionsPctOf(ModeStandard)["todo"] != 10 || d.PanesPctOf(ModePlain)["turn_ticker"] != 20 || d.PanesPctOf(ModePlain)["activity_heatmap"] != 20 {
		t.Errorf("a hidden part lost its number: %v %v", d.SectionsPct, d.PanesPct)
	}
	if d.PanesPctOf(ModeStandard)["processes"] != 20 || d.PanesPctOf(ModePlain)["process_groups"] != 50 {
		t.Errorf("a hidden pane lost its number: %v", d.PanesPct)
	}
}

// A missing file gives the table; a saved file wins and nothing is migrated.
func TestInstallDefaultsMissingFileAndSavedFileWins(t *testing.T) {
	got := Load(filepath.Join(t.TempDir(), "missing.json"))
	if !reflect.DeepEqual(got, Default()) {
		t.Errorf("missing file gives %+v, want Default", got)
	}
	p := filepath.Join(t.TempDir(), "view.json")
	body := `{"overall_scrollbar": true, "parts_standard": {"git": true, "waits": false}, "sections_pct": {"standard": {"status": 30, "sessions_30min": 60}}, "panes_pct": {"standard": {"burn_30min": 40}}}`
	if err := writeRaw(p, body); err != nil {
		t.Fatal(err)
	}
	st := Load(p)
	if !st.Scrollbar || !st.OnIn(ModeStandard, Git) || st.OnIn(ModeStandard, Waits) {
		t.Errorf("a saved choice lost: %+v", st)
	}
	if !reflect.DeepEqual(st.SectionsPctOf(ModeStandard), map[string]int{"status": 30, "sessions_30min": 60}) ||
		!reflect.DeepEqual(st.PanesPctOf(ModeStandard), map[string]int{"burn_30min": 40}) {
		t.Errorf("saved numbers lost: %v %v", st.SectionsPct, st.PanesPct)
	}
}

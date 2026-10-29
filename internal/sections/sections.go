// Package sections is the part model of BurnMon Dev: four groups of fifteen
// parts, which of them each mode shows (one checkbox per part per mode), and
// one Overall scrollbar flag. There are two modes, Standard and Plain, one
// switch for the mode and one list of part switches per mode. It holds no page
// code and no snapshot logic. The choice is stored in burnmon-dev-view.json,
// next to the light or dark theme. A file from R8 (seven sections) is migrated
// in memory and written out in the new shape on the first save.
package sections

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
)

// Part ids.
const (
	Waits         = "waits"
	Outcomes      = "outcomes"
	Burn          = "burn"
	Git           = "git"
	AgentsMap     = "agents_map"
	Sessions      = "sessions"
	Burn30        = "burn_30min"
	VendorStrip   = "vendor_strip"
	Activity      = "activity"
	Heatmap       = "heatmap"
	TurnTicker    = "turn_ticker"
	System        = "system"
	ProcessGroups = "process_groups"
	Processes     = "processes"
	Todo          = "todo"
)

// Part is one switchable part of the page.
type Part struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Group is a heading in the gear list with its parts.
type Group struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Parts []Part `json:"parts"`
}

// Groups is the gear list: four groups, in page order.
var Groups = []Group{
	{ID: "status", Label: "Status", Parts: []Part{
		{Waits, "Waits on you"}, {Outcomes, "Outcomes"}, {Burn, "Burn"},
		{Git, "Git"}, {AgentsMap, "Agents map"}, {Sessions, "Sessions"},
	}},
	{ID: "sessions_30min", Label: "Sessions last 30 min", Parts: []Part{
		{Burn30, "Burn last 30 min"}, {VendorStrip, "Vendor strip"},
		{Activity, "Activity"}, {Heatmap, "Heatmap harness x minute"},
		{TurnTicker, "Turn ticker"},
	}},
	{ID: "processes_30min", Label: "Processes last 30 min", Parts: []Part{
		{System, "System"},
		{ProcessGroups, "Process groups (with memory, disks and network)"},
		{Processes, "Processes"},
	}},
	{ID: "todo", Label: "Microsoft To Do", Parts: []Part{{Todo, "To Do"}}},
}

// IDs lists every part id in group order.
var IDs = func() []string {
	var ids []string
	for _, g := range Groups {
		for _, p := range g.Parts {
			ids = append(ids, p.ID)
		}
	}
	return ids
}()

// Mode names.
const (
	ModeStandard = "standard"
	ModePlain    = "plain"
)

// State is the mode, the Overall scrollbar flag and one list of switches per
// mode. Plain Mode means plain words and the Plain list. It makes no model
// call. The key "plain_mode" keeps the name the first R8 cut gave it.
type State struct {
	Plain     bool            `json:"plain_mode"`
	Scrollbar bool            `json:"overall_scrollbar"`
	Standard  map[string]bool `json:"parts_standard"`
	PlainSet  map[string]bool `json:"parts_plain"`
	// Controls holds one switch per control (ControlIDs). Every control is off
	// until the person turns it on; a missing key is off.
	Controls map[string]bool `json:"controls"`
	// HeightsStandard and HeightsPlain hold the saved split of a column per mode:
	// layout key (HeightKeys) to the part's share of the column's resizable
	// height. Nil when nothing is saved; the view file then has no such key.
	HeightsStandard map[string]float64 `json:"heights_standard,omitempty"`
	HeightsPlain    map[string]float64 `json:"heights_plain,omitempty"`
}

// HeightKeys are layout keys, not part ids: the Activity and Heatmap halves
// share one row, and the System part draws a CPU box and a chart.
// host_lists is not a part: it is the fraction of the host lists' shrink range
// (natural height down to three rows) that a drag took from the Waits,
// Outcomes and Sessions lists. It lives in the same map so an old file and an
// older reader see nothing new.
var HeightKeys = []string{"burn_30min", "vendor_strip", "activity_heatmap", "turn_ticker", "system_chart", "process_groups", "host_lists"}

// cleanHeights keeps known keys with a finite value in (0, 1]; empty gives nil.
func cleanHeights(h map[string]float64) map[string]float64 {
	var out map[string]float64
	for _, k := range HeightKeys {
		v, ok := h[k]
		if !ok || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > 1 {
			continue
		}
		if out == nil {
			out = map[string]float64{}
		}
		out[k] = v
	}
	return out
}

// WithHeights returns a copy with one mode's saved heights replaced. An
// unknown mode changes nothing.
func (s State) WithHeights(mode string, h map[string]float64) State {
	out := s.Normalize()
	switch mode {
	case ModeStandard:
		out.HeightsStandard = cleanHeights(h)
	case ModePlain:
		out.HeightsPlain = cleanHeights(h)
	}
	return out
}

// ResetHeights returns a copy with the saved heights of both modes cleared.
func (s State) ResetHeights() State {
	out := s.Normalize()
	out.HeightsStandard, out.HeightsPlain = nil, nil
	return out
}

// ControlIDs are the control switch ids, the same list as controls.All (a test
// keeps them equal; this package does not import controls).
var ControlIDs = []string{"stop", "attach", "approve", "send_back", "close_sprint"}

func controlsOf(on func(id string) bool) map[string]bool {
	m := make(map[string]bool, len(ControlIDs))
	for _, id := range ControlIDs {
		m[id] = on(id)
	}
	return m
}

func listOf(on func(id string) bool) map[string]bool {
	m := make(map[string]bool, len(IDs))
	for _, id := range IDs {
		m[id] = on(id)
	}
	return m
}

// Default is Standard with every part on and the scrollbar on. Plain shows the
// five R8 panels that Plain showed (Waits, Outcomes, Burn, Agents map, Sessions).
func Default() State {
	plain := map[string]bool{Waits: true, Outcomes: true, Burn: true, AgentsMap: true, Sessions: true}
	return State{
		Scrollbar: true,
		Standard:  listOf(func(string) bool { return true }),
		PlainSet:  listOf(func(id string) bool { return plain[id] }),
		Controls:  controlsOf(func(string) bool { return false }),
	}
}

// Mode is "plain" or "standard".
func (s State) Mode() string {
	if s.Plain {
		return ModePlain
	}
	return ModeStandard
}

func (s State) list(mode string) map[string]bool {
	switch mode {
	case ModeStandard:
		return s.Standard
	case ModePlain:
		return s.PlainSet
	}
	return nil
}

// OnIn reports whether a part is on in the given mode. Unknown is off.
func (s State) OnIn(mode, id string) bool { return s.list(mode)[id] }

// On reports whether a part is on in the current mode.
func (s State) On(id string) bool { return s.OnIn(s.Mode(), id) }

// Current is the switch list of the current mode.
func (s State) Current() map[string]bool { return s.list(s.Mode()) }

// With returns a copy with one part switched in one mode. An unknown mode
// or part changes nothing.
func (s State) With(mode, id string, on bool) State {
	out := s.Normalize()
	if l := out.list(mode); l != nil {
		if _, ok := l[id]; ok {
			l[id] = on
		}
	}
	return out
}

// WithPlain returns a copy with the mode switched.
func (s State) WithPlain(plain bool) State {
	out := s.Normalize()
	out.Plain = plain
	return out
}

// WithControl returns a copy with one control switched. An unknown id changes
// nothing.
func (s State) WithControl(id string, on bool) State {
	out := s.Normalize()
	if _, ok := out.Controls[id]; ok {
		out.Controls[id] = on
	}
	return out
}

// WithScrollbar returns a copy with the Overall scrollbar switched.
func (s State) WithScrollbar(on bool) State {
	out := s.Normalize()
	out.Scrollbar = on
	return out
}

// Normalize returns a copy with exactly the known parts in each list: unknown
// ids are dropped, and one missing from a saved file takes its default.
func (s State) Normalize() State {
	d := Default()
	pick := func(saved, def map[string]bool) map[string]bool {
		return listOf(func(id string) bool {
			if v, ok := saved[id]; ok {
				return v
			}
			return def[id]
		})
	}
	ctl := controlsOf(func(id string) bool { return s.Controls[id] })
	return State{Plain: s.Plain, Scrollbar: s.Scrollbar, Standard: pick(s.Standard, d.Standard), PlainSet: pick(s.PlainSet, d.PlainSet), Controls: ctl, HeightsStandard: cleanHeights(s.HeightsStandard), HeightsPlain: cleanHeights(s.HeightsPlain)}
}

// r8Fill completes an R8 list (seven sections) with its R8 defaults.
func r8Fill(saved map[string]bool, plain bool) map[string]bool {
	ids := []string{"waits", "outcomes", "burn", "agents_map", "sessions", "processes", "git"}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		def := true
		if plain && (id == "processes" || id == "git") {
			def = false
		}
		if v, ok := saved[id]; ok {
			def = v
		}
		out[id] = def
	}
	return out
}

// migrateR8 maps the two R8 lists (already filled with R8 defaults) onto the
// parts. Standard: the old burn switch carries the whole burn zone, the old
// processes switch the whole system zone, To Do is on. Plain: R8 Plain never
// showed those zones or To Do, so they stay off.
func migrateR8(plain bool, standard, plainSet map[string]bool) State {
	std := listOf(func(string) bool { return false })
	pl := listOf(func(string) bool { return false })
	for _, id := range []string{Waits, Outcomes, Git, AgentsMap, Sessions} {
		std[id] = standard[id]
		pl[id] = plainSet[id]
	}
	for _, id := range []string{Burn, Burn30, VendorStrip, Activity, Heatmap, TurnTicker} {
		std[id] = standard[Burn]
	}
	for _, id := range []string{Processes, System, ProcessGroups} {
		std[id] = standard[Processes]
	}
	std[Todo] = true
	pl[Burn] = plainSet[Burn]
	pl[Processes] = plainSet[Processes]
	return State{Plain: plain, Scrollbar: true, Standard: std, PlainSet: pl, Controls: controlsOf(func(string) bool { return false })}
}

// Load reads the state from the view file at path. A missing or unparsable
// file gives Default. New keys win over R8 keys; an R8 list is migrated. A file
// from the first R8 cut keeps its mode and takes the defaults for both lists,
// since its single list belonged to no mode.
func Load(path string) State {
	b, err := os.ReadFile(path)
	if err != nil {
		return Default()
	}
	var f struct {
		Plain           bool               `json:"plain_mode"`
		Scrollbar       *bool              `json:"overall_scrollbar"`
		Standard        map[string]bool    `json:"parts_standard"`
		PlainSet        map[string]bool    `json:"parts_plain"`
		R8Std           map[string]bool    `json:"sections_standard"`
		R8Plain         map[string]bool    `json:"sections_plain"`
		Controls        map[string]bool    `json:"controls"`
		HeightsStandard map[string]float64 `json:"heights_standard"`
		HeightsPlain    map[string]float64 `json:"heights_plain"`
	}
	if json.Unmarshal(b, &f) != nil {
		return Default()
	}
	st := migrateR8(f.Plain, r8Fill(f.R8Std, false), r8Fill(f.R8Plain, true))
	if f.Standard != nil {
		st.Standard = f.Standard
	}
	if f.PlainSet != nil {
		st.PlainSet = f.PlainSet
	}
	if f.Scrollbar != nil {
		st.Scrollbar = *f.Scrollbar
	}
	st.Controls = f.Controls
	st.HeightsStandard = f.HeightsStandard
	st.HeightsPlain = f.HeightsPlain
	return st.Normalize()
}

// Save writes the state into the view file at path and keeps every other key
// in it (the theme). The R8 keys ("sections", "sections_standard",
// "sections_plain") are removed. A file that cannot be parsed is replaced.
func Save(path string, s State) error {
	s = s.Normalize()
	rest := map[string]json.RawMessage{}
	if b, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(b, &rest) != nil || rest == nil {
			rest = map[string]json.RawMessage{}
		}
	}
	delete(rest, "sections")
	delete(rest, "sections_standard")
	delete(rest, "sections_plain")
	var err error
	if rest["plain_mode"], err = json.Marshal(s.Plain); err != nil {
		return err
	}
	if rest["overall_scrollbar"], err = json.Marshal(s.Scrollbar); err != nil {
		return err
	}
	if rest["parts_standard"], err = json.Marshal(s.Standard); err != nil {
		return err
	}
	if rest["parts_plain"], err = json.Marshal(s.PlainSet); err != nil {
		return err
	}
	if rest["controls"], err = json.Marshal(s.Controls); err != nil {
		return err
	}
	for key, h := range map[string]map[string]float64{"heights_standard": s.HeightsStandard, "heights_plain": s.HeightsPlain} {
		if len(h) == 0 {
			delete(rest, key)
			continue
		}
		if rest[key], err = json.Marshal(h); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(rest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

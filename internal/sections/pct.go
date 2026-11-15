package sections

import "fmt"

// Section and pane sizes (2026-11-15, replaces the B7.1 drag heights). Each
// mode keeps one number per section (percent of the window height) and one
// per pane (percent of its section). The page draws them (page.html
// pctPlan, the same rules); this file validates what the page saves.

// Pane is one box inside a section: the parts on one row share it (the
// Activity and Heatmap halves, the first five Status panels).
type Pane struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Section string   `json:"section"`
	Parts   []string `json:"parts"`
}

// Panes in page order. Section is a Group id.
var Panes = []Pane{
	{"status_row1", "Waits on you, Outcomes, Burn, Git, Agents map", "status", []string{Waits, Outcomes, Burn, Git, AgentsMap}},
	{"sessions", "Sessions", "status", []string{Sessions}},
	{"burn_30min", "Burn last 30 min", "sessions_30min", []string{Burn30}},
	{"vendor_strip", "Vendor strip", "sessions_30min", []string{VendorStrip}},
	{"activity_heatmap", "Activity and Heatmap", "sessions_30min", []string{Activity, Heatmap}},
	{"turn_ticker", "Turn ticker", "sessions_30min", []string{TurnTicker}},
	{"system", "System", "processes_30min", []string{System}},
	{"process_groups", "Process groups", "processes_30min", []string{ProcessGroups}},
	{"processes", "Processes", "processes_30min", []string{Processes}},
	{"todo", "To Do", "todo", []string{Todo}},
}

// RestSection takes what the section numbers leave under 100 when no box is
// empty; hidden, the first shown section takes it.
const RestSection = "status"

func sectionKnown(id string) bool {
	for _, g := range Groups {
		if g.ID == id {
			return true
		}
	}
	return false
}

func paneKnown(id string) bool {
	for _, p := range Panes {
		if p.ID == id {
			return true
		}
	}
	return false
}

func sectionLabel(id string) string {
	for _, g := range Groups {
		if g.ID == id {
			return g.Label
		}
	}
	return id
}

func paneLabel(id string) string {
	for _, p := range Panes {
		if p.ID == id {
			return p.Label
		}
	}
	return id
}

// DefaultSectionsPct and DefaultPanesPct are the numbers a fresh install
// starts with in both modes, close to the B7 look (Wilco, 2026-11-15): the
// sections add up to 160, so with the default Overall scrollbar on the page
// scrolls as it did, and every chart has room at 1280x860. With the scrollbar
// off they are refused until lowered, and the page draws them scaled to fit.
// They show in the gear's boxes; a box the person clears shares what is left.
// The page carries the same numbers (page.html PCT_DEFAULT_*), a test keeps them equal.
var DefaultSectionsPct = map[string]int{"status": 30, "sessions_30min": 60, "processes_30min": 60, "todo": 10}

var DefaultPanesPct = map[string]int{
	"status_row1": 50, "sessions": 50,
	"burn_30min": 40, "vendor_strip": 20, "activity_heatmap": 20, "turn_ticker": 20,
	"system": 50, "process_groups": 30, "processes": 20,
}

func defaultPct(d map[string]int) map[string]map[string]int {
	return map[string]map[string]int{ModeStandard: copyInts(d), ModePlain: copyInts(d)}
}

// cleanPct keeps the known keys with a value from 1 to 100 in both modes; the
// result always holds both modes, an empty mode meaning every box is empty.
func cleanPct(m map[string]map[string]int, known func(string) bool) map[string]map[string]int {
	out := map[string]map[string]int{ModeStandard: {}, ModePlain: {}}
	for _, mode := range []string{ModeStandard, ModePlain} {
		for k, v := range m[mode] {
			if known(k) && v >= 1 && v <= 100 {
				out[mode][k] = v
			}
		}
	}
	return out
}

func copyInts(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// SectionsPctOf is one mode's section numbers (a copy, never nil).
func (s State) SectionsPctOf(mode string) map[string]int { return copyInts(s.SectionsPct[mode]) }

// PanesPctOf is one mode's pane numbers (a copy, never nil).
func (s State) PanesPctOf(mode string) map[string]int { return copyInts(s.PanesPct[mode]) }

// WithPct returns a copy with one mode's numbers replaced; bad keys and
// values are dropped. An unknown mode changes nothing.
func (s State) WithPct(mode string, secs, panes map[string]int) State {
	out := s.Normalize()
	if mode != ModeStandard && mode != ModePlain {
		return out
	}
	set := func(m map[string]map[string]int, v map[string]int, known func(string) bool) map[string]map[string]int {
		n := map[string]map[string]int{}
		for k, x := range m {
			n[k] = x
		}
		n[mode] = v
		return cleanPct(n, known)
	}
	out.SectionsPct = set(out.SectionsPct, secs, sectionKnown)
	out.PanesPct = set(out.PanesPct, panes, paneKnown)
	return out
}

// ShownPanes lists the panes of one section with a part on in mode, in order.
func (s State) ShownPanes(mode, section string) []string {
	var out []string
	for _, p := range Panes {
		if p.Section != section {
			continue
		}
		for _, part := range p.Parts {
			if s.OnIn(mode, part) {
				out = append(out, p.ID)
				break
			}
		}
	}
	return out
}

// ShownSections lists the sections with a part on in mode, in order.
func (s State) ShownSections(mode string) []string {
	var out []string
	for _, g := range Groups {
		if len(s.ShownPanes(mode, g.ID)) > 0 {
			out = append(out, g.ID)
		}
	}
	return out
}

// Split is a Plan result: the percent per shown key, the sum of the typed
// numbers of shown keys, whether that is over 100, and the first empty box
// left with nothing (Starved, "" when none).
type Split struct {
	Pct     map[string]float64
	Sum     int
	Over    bool
	Starved string
}

// Plan applies the rules to the shown keys only (rule 2e): empty boxes share
// what the numbers leave equally (2f); with no empty box, the rest under 100
// goes to restKey, or to the first shown key when restKey is not shown (2a,
// 2c). Over 100 is reported, not cut (2b decides).
func Plan(shown []string, vals map[string]int, restKey string) Split {
	sp := Split{Pct: map[string]float64{}}
	var empty []string
	for _, k := range shown {
		if v, ok := vals[k]; ok {
			sp.Pct[k] = float64(v)
			sp.Sum += v
		} else {
			empty = append(empty, k)
		}
	}
	sp.Over = sp.Sum > 100
	rest := 100 - sp.Sum
	if len(empty) > 0 {
		if rest <= 0 {
			sp.Starved = empty[0]
			rest = 0
		}
		for _, k := range empty {
			sp.Pct[k] = float64(rest) / float64(len(empty))
		}
		return sp
	}
	if rest > 0 && len(shown) > 0 {
		key := shown[0]
		for _, k := range shown {
			if k == restKey {
				key = k
			}
		}
		sp.Pct[key] += float64(rest)
	}
	return sp
}

// CheckPct validates one mode's numbers against that mode's shown parts and
// the Overall scrollbar, with the page's own sentences.
func (s State) CheckPct(mode string, secs, panes map[string]int) error {
	if mode != ModeStandard && mode != ModePlain {
		return fmt.Errorf("mode %q is not standard or plain", mode)
	}
	for k, v := range secs {
		if !sectionKnown(k) {
			return fmt.Errorf("section %q is not known", k)
		}
		if v < 1 || v > 100 {
			return fmt.Errorf("section %q must be 1 to 100 percent", k)
		}
	}
	for k, v := range panes {
		if !paneKnown(k) {
			return fmt.Errorf("pane %q is not known", k)
		}
		if v < 1 || v > 100 {
			return fmt.Errorf("pane %q must be 1 to 100 percent", k)
		}
	}
	sp := Plan(s.ShownSections(mode), secs, RestSection)
	if sp.Over && !s.Scrollbar {
		return fmt.Errorf("Sections add up to %d percent; switch on Overall scrollbar or lower a number", sp.Sum)
	}
	if sp.Starved != "" {
		return fmt.Errorf("Sections add up to %d percent and %s has no number; give it a number or lower another", sp.Sum, sectionLabel(sp.Starved))
	}
	for _, sec := range s.ShownSections(mode) {
		shown := s.ShownPanes(mode, sec)
		pp := Plan(shown, panes, shown[0])
		if pp.Over {
			return fmt.Errorf("Panes in %s add up to %d percent; lower a number", sectionLabel(sec), pp.Sum)
		}
		if pp.Starved != "" {
			return fmt.Errorf("Panes in %s add up to %d percent and %s has no number; give it a number or lower another", sectionLabel(sec), pp.Sum, paneLabel(pp.Starved))
		}
	}
	return nil
}

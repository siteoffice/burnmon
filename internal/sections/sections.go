// Package sections is R8 of the agent control plan: the seven sections
// BurnMon Dev is built from, and which of them each mode shows. There are two
// modes, Standard and Plain, one switch for the mode and one list of section
// switches per mode. It holds no page code and no snapshot logic. The choice
// is stored in burnmon-dev-view.json, next to the light or dark theme.
package sections

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Section ids, in the order the page shows them.
const (
	Waits     = "waits"
	Outcomes  = "outcomes"
	Burn      = "burn"
	AgentsMap = "agents_map"
	Sessions  = "sessions"
	Processes = "processes"
	Git       = "git"
)

// IDs lists every section in display order.
var IDs = []string{Waits, Outcomes, Burn, AgentsMap, Sessions, Processes, Git}

// Mode names.
const (
	ModeStandard = "standard"
	ModePlain    = "plain"
)

// State is the mode and one list of switches per mode. Plain Mode means plain
// words, a page that fits the window, and the Plain list. It makes no model
// call. The key "plain_mode" keeps the name the first R8 cut gave it.
type State struct {
	Plain    bool            `json:"plain_mode"`
	Standard map[string]bool `json:"sections_standard"`
	PlainSet map[string]bool `json:"sections_plain"`
}

func listOf(on func(id string) bool) map[string]bool {
	m := make(map[string]bool, len(IDs))
	for _, id := range IDs {
		m[id] = on(id)
	}
	return m
}

// Default is Standard with every section on. Plain starts with Processes and
// Git off. That Plain set is a placeholder until Wilco decides what Plain shows.
func Default() State {
	return State{
		Standard: listOf(func(string) bool { return true }),
		PlainSet: listOf(func(id string) bool { return id != Processes && id != Git }),
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

// OnIn reports whether a section is on in the given mode. Unknown is off.
func (s State) OnIn(mode, id string) bool { return s.list(mode)[id] }

// On reports whether a section is on in the current mode.
func (s State) On(id string) bool { return s.OnIn(s.Mode(), id) }

// Current is the switch list of the current mode.
func (s State) Current() map[string]bool { return s.list(s.Mode()) }

// With returns a copy with one section switched in one mode. An unknown mode
// or section changes nothing.
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

// Normalize returns a copy with exactly the known sections in each list:
// unknown ids are dropped, and one missing from a saved file takes its
// default (on in Standard, the placeholder set in Plain).
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
	return State{Plain: s.Plain, Standard: pick(s.Standard, d.Standard), PlainSet: pick(s.PlainSet, d.PlainSet)}
}

// Load reads the state from the view file at path. A missing or unparsable
// file gives Default. A file from the first R8 cut keeps its mode and takes
// the defaults for both lists, since its single list belonged to no mode.
func Load(path string) State {
	b, err := os.ReadFile(path)
	if err != nil {
		return Default()
	}
	var st State
	if json.Unmarshal(b, &st) != nil {
		return Default()
	}
	return st.Normalize()
}

// Save writes the state into the view file at path and keeps every other key
// in it (the theme). The first cut's "sections" key is removed. A file that
// cannot be parsed is replaced.
func Save(path string, s State) error {
	s = s.Normalize()
	rest := map[string]json.RawMessage{}
	if b, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(b, &rest) != nil || rest == nil {
			rest = map[string]json.RawMessage{}
		}
	}
	delete(rest, "sections")
	var err error
	if rest["plain_mode"], err = json.Marshal(s.Plain); err != nil {
		return err
	}
	if rest["sections_standard"], err = json.Marshal(s.Standard); err != nil {
		return err
	}
	if rest["sections_plain"], err = json.Marshal(s.PlainSet); err != nil {
		return err
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

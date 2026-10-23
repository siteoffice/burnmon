// Package sections is R8 of the agent control plan: the seven sections
// BurnMon Dev is built from, one switch per section, and the two presets
// (Simple and Developer). It holds no page code and no snapshot logic. The
// choice is stored under "sections" and "plain_mode" in burnmon-dev-view.json,
// next to the light or dark theme.
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

// State is the switches. Plain Mode here means plain-word labels and short
// explanations written into the page. It makes no model call.
type State struct {
	Sections map[string]bool `json:"sections"`
	Plain    bool            `json:"plain_mode"`
}

func build(on map[string]bool, plain bool) State {
	m := make(map[string]bool, len(IDs))
	for _, id := range IDs {
		m[id] = on[id]
	}
	return State{Sections: m, Plain: plain}
}

// Developer is every section on and Plain Mode off. It is also what a missing
// or unreadable view file means, so BurnMon Dev looks as it did before R8.
func Developer() State {
	on := map[string]bool{}
	for _, id := range IDs {
		on[id] = true
	}
	return build(on, false)
}

// Simple drops Processes and Git and turns Plain Mode on.
func Simple() State {
	on := map[string]bool{}
	for _, id := range IDs {
		on[id] = id != Processes && id != Git
	}
	return build(on, true)
}

// On reports whether a section is switched on. An unknown id is off.
func (s State) On(id string) bool { return s.Sections[id] }

// With returns a copy with one section switched. An unknown id changes nothing.
func (s State) With(id string, on bool) State {
	out := s.Normalize()
	if _, ok := out.Sections[id]; ok {
		out.Sections[id] = on
	}
	return out
}

// WithPlain returns a copy with Plain Mode switched.
func (s State) WithPlain(plain bool) State {
	out := s.Normalize()
	out.Plain = plain
	return out
}

// Normalize returns a copy with exactly the known sections: unknown ids are
// dropped, a section missing from a saved file is on.
func (s State) Normalize() State {
	m := make(map[string]bool, len(IDs))
	for _, id := range IDs {
		v, ok := s.Sections[id]
		m[id] = v || !ok
	}
	return State{Sections: m, Plain: s.Plain}
}

func (s State) equal(o State) bool {
	s, o = s.Normalize(), o.Normalize()
	if s.Plain != o.Plain {
		return false
	}
	for _, id := range IDs {
		if s.Sections[id] != o.Sections[id] {
			return false
		}
	}
	return true
}

// PresetName is "simple" or "developer" when the switches match a preset
// exactly, otherwise "custom". It is derived, never stored, so it cannot
// disagree with the switches.
func (s State) PresetName() string {
	switch {
	case s.equal(Simple()):
		return "simple"
	case s.equal(Developer()):
		return "developer"
	}
	return "custom"
}

type file struct {
	Sections map[string]bool `json:"sections"`
	Plain    *bool           `json:"plain_mode"`
}

// Load reads the sections from the view file at path. A missing, unparsable
// or incomplete file gives Developer, or Developer's values for what is missing.
func Load(path string) State {
	b, err := os.ReadFile(path)
	if err != nil {
		return Developer()
	}
	var f file
	if json.Unmarshal(b, &f) != nil || f.Sections == nil {
		return Developer()
	}
	st := State{Sections: f.Sections}
	if f.Plain != nil {
		st.Plain = *f.Plain
	}
	return st.Normalize()
}

// Save writes the sections into the view file at path and keeps every other
// key in it (the theme). A file that cannot be parsed is replaced.
func Save(path string, s State) error {
	s = s.Normalize()
	rest := map[string]json.RawMessage{}
	if b, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(b, &rest) != nil || rest == nil {
			rest = map[string]json.RawMessage{}
		}
	}
	var err error
	if rest["sections"], err = json.Marshal(s.Sections); err != nil {
		return err
	}
	if rest["plain_mode"], err = json.Marshal(s.Plain); err != nil {
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

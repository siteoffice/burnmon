//go:build windows

// sections_view.go is the Go side of R8: what the page asks for (the section
// switches, the presets, the newest snapshot) and what it sends back. The
// switches live in burnmon-dev-view.json via internal/sections. The snapshot
// is read from disk only, never over a port.
package main

import (
	"fmt"
	"time"

	"burnmon/internal/sections"
	"burnmon/internal/snapshot"
)

// sectionsPayload is bdevSections' return value.
type sectionsPayload struct {
	sections.State
	Preset  string           `json:"preset"`
	Order   []string         `json:"order"`
	Reading snapshot.Reading `json:"reading"`
}

// sectionsChange is what the page sends: a preset name, one switch, or Plain
// Mode. Exactly one of the three is set.
type sectionsChange struct {
	Preset  string `json:"preset,omitempty"`
	Section string `json:"section,omitempty"`
	On      *bool  `json:"on,omitempty"`
	Plain   *bool  `json:"plain,omitempty"`
}

func loadSections(dataDir string) sections.State {
	return sections.Load(viewStatePath(dataDir))
}

func saveSections(dataDir string, s sections.State) error {
	return sections.Save(viewStatePath(dataDir), s)
}

func sectionsPayloadFor(dataDir, snapPath string, now time.Time) sectionsPayload {
	st := loadSections(dataDir)
	return sectionsPayload{
		State:   st,
		Preset:  st.PresetName(),
		Order:   sections.IDs,
		Reading: snapshot.Load(snapPath, now),
	}
}

// applySectionsChange validates a change from the page and saves it. A change
// it does not understand is refused and the file is left as it was.
func applySectionsChange(dataDir string, c sectionsChange) error {
	cur := loadSections(dataDir)
	switch {
	case c.Preset == "simple":
		return saveSections(dataDir, sections.Simple())
	case c.Preset == "developer":
		return saveSections(dataDir, sections.Developer())
	case c.Preset != "":
		return fmt.Errorf("preset %q is not simple or developer", c.Preset)
	case c.Plain != nil:
		return saveSections(dataDir, cur.WithPlain(*c.Plain))
	case c.Section != "" && c.On != nil:
		for _, id := range sections.IDs {
			if id == c.Section {
				return saveSections(dataDir, cur.With(id, *c.On))
			}
		}
		return fmt.Errorf("section %q is not one of the seven", c.Section)
	}
	return fmt.Errorf("empty sections change")
}

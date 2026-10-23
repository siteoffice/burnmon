//go:build windows

// sections_view.go is the Go side of R8: what the page asks for (the section
// switches, the presets, the newest snapshot) and what it sends back. The
// switches live in burnmon-dev-view.json via internal/sections. The snapshot
// is read from disk only, never over a port.
package main

import (
	"fmt"
	"time"

	"burnmon/internal/agentmap"
	"burnmon/internal/sections"
	"burnmon/internal/snapshot"
)

// sectionsPayload is bdevSections' return value. The embedded state carries
// plain_mode and both lists; Sections is the list of the current mode, so the
// page can draw without choosing.
type sectionsPayload struct {
	sections.State
	Mode     string           `json:"mode"`
	Sections map[string]bool  `json:"sections"`
	Order    []string         `json:"order"`
	Reading  snapshot.Reading `json:"reading"`
	// AgentMap is the live map for the Agents map section, nil unless the
	// snapshot is current. History is a separate call (agentmap_view.go).
	AgentMap *agentmap.View `json:"agent_map"`
}

// sectionsChange is what the page sends: the mode toggle (Plain), or one
// switch (Mode, Section and On together). Anything else is refused.
type sectionsChange struct {
	Plain   *bool  `json:"plain,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Section string `json:"section,omitempty"`
	On      *bool  `json:"on,omitempty"`
}

func loadSections(dataDir string) sections.State {
	return sections.Load(viewStatePath(dataDir))
}

func saveSections(dataDir string, s sections.State) error {
	return sections.Save(viewStatePath(dataDir), s)
}

func sectionsPayloadFor(dataDir, snapPath string, now time.Time) sectionsPayload {
	st := loadSections(dataDir)
	p := sectionsPayload{
		State:    st,
		Mode:     st.Mode(),
		Sections: st.Current(),
		Order:    sections.IDs,
		Reading:  snapshot.Load(snapPath, now),
	}
	if p.Reading.Status == snapshot.StatusOK && p.Reading.Snapshot != nil {
		v := agentmap.Build(p.Reading.Snapshot.Nodes, p.Reading.Snapshot.Edges)
		p.AgentMap = &v
	}
	return p
}

// applySectionsChange validates a change from the page and saves it. A change
// it does not understand is refused and the file is left as it was.
func applySectionsChange(dataDir string, c sectionsChange) error {
	cur := loadSections(dataDir)
	switch {
	case c.Plain != nil && c.Mode == "" && c.Section == "":
		return saveSections(dataDir, cur.WithPlain(*c.Plain))
	case c.Mode != "" && c.Section != "" && c.On != nil:
		if c.Mode != sections.ModeStandard && c.Mode != sections.ModePlain {
			return fmt.Errorf("mode %q is not standard or plain", c.Mode)
		}
		for _, id := range sections.IDs {
			if id == c.Section {
				return saveSections(dataDir, cur.With(c.Mode, id, *c.On))
			}
		}
		return fmt.Errorf("section %q is not one of the seven", c.Section)
	}
	return fmt.Errorf("sections change needs plain, or mode with section and on")
}

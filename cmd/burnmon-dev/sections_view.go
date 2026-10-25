//go:build windows

// sections_view.go is the Go side of the gear list: what the page asks for
// (the part switches per mode, the overall scrollbar flag, the newest
// snapshot) and what it sends back. The switches live in
// burnmon-dev-view.json via internal/sections. The snapshot is read from disk
// only, never over a port.
package main

import (
	"fmt"
	"time"

	"burnmon/internal/agentmap"
	"burnmon/internal/sections"
	"burnmon/internal/snapshot"
)

// sectionsPayload is bdevSections' return value. The embedded state carries
// plain_mode, overall_scrollbar and both lists; Parts is the list of the
// current mode, so the page can draw without choosing. Groups gives the page
// the labels and order of the gear list.
type sectionsPayload struct {
	sections.State
	Mode    string           `json:"mode"`
	Parts   map[string]bool  `json:"parts"`
	Groups  []sections.Group `json:"groups"`
	Reading snapshot.Reading `json:"reading"`
	// AgentMap is the live map for the Agents map section, nil unless the
	// snapshot is current. History is a separate call (agentmap_view.go).
	AgentMap *agentmap.View `json:"agent_map"`
}

// sectionsChange is what the page sends: the mode toggle (Plain), the overall
// scrollbar flag (Scrollbar), or one part switch (Mode, Section and On
// together, Section being a part id). Exactly one kind per call; anything else
// is refused.
type sectionsChange struct {
	Plain     *bool  `json:"plain,omitempty"`
	Scrollbar *bool  `json:"scrollbar,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Section   string `json:"section,omitempty"`
	On        *bool  `json:"on,omitempty"`
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
		State:   st,
		Mode:    st.Mode(),
		Parts:   st.Current(),
		Groups:  sections.Groups,
		Reading: snapshot.Load(snapPath, now),
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
	hasPart := c.Mode != "" || c.Section != "" || c.On != nil
	switch {
	case c.Plain != nil && c.Scrollbar == nil && !hasPart:
		return saveSections(dataDir, cur.WithPlain(*c.Plain))
	case c.Scrollbar != nil && c.Plain == nil && !hasPart:
		return saveSections(dataDir, cur.WithScrollbar(*c.Scrollbar))
	case c.Plain == nil && c.Scrollbar == nil && c.Mode != "" && c.Section != "" && c.On != nil:
		if c.Mode != sections.ModeStandard && c.Mode != sections.ModePlain {
			return fmt.Errorf("mode %q is not standard or plain", c.Mode)
		}
		for _, id := range sections.IDs {
			if id == c.Section {
				return saveSections(dataDir, cur.With(c.Mode, id, *c.On))
			}
		}
		return fmt.Errorf("part %q is not known", c.Section)
	}
	return fmt.Errorf("sections change needs plain, scrollbar, or mode with section and on")
}

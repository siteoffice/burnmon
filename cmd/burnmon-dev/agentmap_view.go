//go:build windows

// agentmap_view.go is the Go side of R7: the agent map for a past window,
// built from BurnMon's store the same way `burnmon-cli graph` does, and
// shaped by internal/agentmap. The live map needs no call of its own, it
// rides in the sections payload (sections_view.go). Nothing here opens a port.
package main

import (
	"os"
	"path/filepath"
	"time"

	"burnmon/internal/agentmap"
	"burnmon/internal/graph"
	"burnmon/internal/store"
)

const (
	mapHoursDefault = 24
	mapHoursMax     = 720
)

// agentMapPayload is bdevAgentMapHistory's return value. Note says, in plain
// words, anything that left the map thinner than it could be.
type agentMapPayload struct {
	Source string        `json:"source"`
	Hours  int           `json:"hours"`
	View   agentmap.View `json:"view"`
	Note   string        `json:"note,omitempty"`
}

// clampMapHours keeps the window the page asks for between one hour and 30
// days; anything else is the 24 hour default.
func clampMapHours(h int) int {
	switch {
	case h <= 0:
		return mapHoursDefault
	case h > mapHoursMax:
		return mapHoursMax
	}
	return h
}

// agentMapHistory is the map for the last hours before now. foremanRoot is
// the configured Foreman folder ("" when none): without it sessions are not
// grouped under sprints, and a folder that cannot be read costs the sprints
// only, never the map.
func agentMapHistory(st *store.Store, foremanRoot string, hours int, now time.Time) (agentMapPayload, error) {
	hours = clampMapHours(hours)
	hi := graph.HistoryInput{From: now.Add(-time.Duration(hours) * time.Hour), To: now, Now: now, RepoFor: graph.NewRepoLocator().For}
	if home, err := os.UserHomeDir(); err == nil {
		hi.SubagentFor = graph.NewSubagentMeta(filepath.Join(home, ".claude", "projects")).Lookup
	}
	p := agentMapPayload{Source: "history", Hours: hours}
	if foremanRoot != "" {
		sprints, _, err := graph.ReadForeman(foremanRoot)
		if err != nil {
			p.Note = "Foreman state could not be read, so sessions are not grouped under sprints."
		} else {
			hi.Sprints = sprints
		}
	}
	g, err := graph.History(st, hi)
	if err != nil {
		return p, err
	}
	p.View = agentmap.Build(g.Nodes, g.Edges)
	return p, nil
}

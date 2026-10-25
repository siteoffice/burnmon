//go:build windows

// agentmap_view.go is the Go side of R7's full agent map: the boxes and lines
// for the live snapshot or for a past window of BurnMon's store, laid out by
// internal/agentmap. The small overview rides in the sections payload
// (sections_view.go). Nothing here opens a port; the page asks through one
// binding and the answer is built from files and the store.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"burnmon/internal/agentmap"
	"burnmon/internal/graph"
	"burnmon/internal/snapshot"
	"burnmon/internal/store"
)

const (
	mapHoursDefault = 24
	mapHoursMax     = 720
)

// agentMapEnv is what the map reads from: the view file folder (for Plain
// Mode), the snapshot file, the store and the Foreman folder.
type agentMapEnv struct {
	DataDir     string
	SnapPath    string
	St          *store.Store
	ForemanRoot string
}

// agentMapRequest is what the page sends: live or history, how many hours of
// history, and the frames the person opened.
type agentMapRequest struct {
	Source string   `json:"source"`
	Hours  int      `json:"hours"`
	Open   []string `json:"open"`
}

// agentMapPayload is bdevAgentMap's return value. Note says, in plain words,
// anything that left the map thinner than it could be.
type agentMapPayload struct {
	Source     string          `json:"source"`
	Hours      int             `json:"hours"`
	Status     string          `json:"status"`
	AgeSeconds int64           `json:"age_seconds"`
	Note       string          `json:"note,omitempty"`
	Layout     agentmap.Layout `json:"layout"`
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

func joinNotes(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + " " + b
}

// agentMapFor builds the map the page asked for. A snapshot that is not
// current draws no boxes: its old numbers are never shown as current.
func agentMapFor(env agentMapEnv, req agentMapRequest, now time.Time) (agentMapPayload, error) {
	open := map[string]bool{}
	for _, id := range req.Open {
		open[id] = true
	}
	in := agentmap.LayoutInput{Open: open, Plain: loadSections(env.DataDir).Plain}
	p := agentMapPayload{Source: req.Source, Status: "ok"}
	switch req.Source {
	case "live":
		r := snapshot.Load(env.SnapPath, now)
		p.Status, p.AgeSeconds = r.Status, r.AgeSeconds
		if r.Status != snapshot.StatusOK || r.Snapshot == nil {
			switch r.Status {
			case snapshot.StatusUnsupported:
				p.Note = "The snapshot is a newer version than this window reads."
			case snapshot.StatusStale, snapshot.StatusMissing:
				p.Note = fmt.Sprintf("BurnMon is not running (%s). No current boxes are shown.", r.Status)
			default:
				p.Note = "The snapshot file cannot be read: " + r.Error
			}
			p.Layout = agentmap.BuildLayout(agentmap.LayoutInput{Plain: in.Plain})
			return p, nil
		}
		in.Nodes, in.Edges = r.Snapshot.Nodes, r.Snapshot.Edges
		in.Evidence = map[string]agentmap.Evidence{}
		for _, s := range r.Snapshot.Sessions {
			in.Evidence[graph.SessionID(s.Vendor, s.ID)] = agentmap.Evidence{Model: s.Model, Tokens: s.Tokens, CostEUR: s.Cost.EUR,
				CostLabel: s.Cost.Label, Source: s.Source, AgeSeconds: s.AgeSeconds, Known: true}
		}
	case "history":
		p.Hours = clampMapHours(req.Hours)
		hi := graph.HistoryInput{From: now.Add(-time.Duration(p.Hours) * time.Hour), To: now, Now: now, RepoFor: graph.NewRepoLocator().For}
		if home, err := os.UserHomeDir(); err == nil {
			hi.SubagentFor = graph.NewSubagentMeta(filepath.Join(home, ".claude", "projects")).Lookup
		}
		p.Note = "Tokens and prices are not kept in the history view, so they read unknown."
		if env.ForemanRoot != "" {
			sprints, _, err := graph.ReadForeman(env.ForemanRoot)
			if err != nil {
				p.Note = joinNotes(p.Note, "Foreman state could not be read, so sessions are not grouped under sprints.")
			} else {
				hi.Sprints = sprints
			}
		}
		if env.St == nil {
			return p, fmt.Errorf("the store is not open")
		}
		g, err := graph.History(env.St, hi)
		if err != nil {
			return p, err
		}
		in.Nodes, in.Edges = g.Nodes, g.Edges
	default:
		return p, fmt.Errorf("source %q is not live or history", req.Source)
	}
	p.Layout = agentmap.BuildLayout(in)
	return p, nil
}

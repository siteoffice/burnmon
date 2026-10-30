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
	"strings"
	"time"

	"burnmon/internal/agentmap"
	"burnmon/internal/gitrefs"
	"burnmon/internal/graph"
	"burnmon/internal/snapshot"
	"burnmon/internal/snapshotwriter"
	"burnmon/internal/store"
	"burnmon/internal/waits"
)

// historyChecker is the same merge check the live writer uses, kept so its
// answers are cached between map refreshes.
var historyChecker = gitrefs.NewChecker()

const (
	mapHoursDefault = 24
	mapHoursMax     = 720
)

// agentMapEnv is what the map reads from: the view file folder (settingsDir, for Plain
// Mode), the snapshot file, the store and the Foreman folder.
type agentMapEnv struct {
	DataDir     string
	SnapPath    string
	St          *store.Store
	ForemanRoot string
	// MergedFor answers "is this branch in main" for the history map; nil
	// means a gitrefs checker.
	MergedFor func(repo, branch string) (merged, known bool)
	// BranchGone and MergedByMessage are the deleted-branch check; nil
	// means a gitrefs checker.
	BranchGone      func(repo, branch string) bool
	MergedByMessage func(repo, branch string) (found, known bool)
	// Writer is the snapshot writer's state for the live note; nil means
	// unknown.
	Writer func() snapshotwriter.StatusView
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

// asSentence capitalises a header line and ends it with a period, so a note
// can carry more text after it.
func asSentence(s string) string {
	if s == "" {
		return s
	}
	s = strings.ToUpper(s[:1]) + s[1:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
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
			// The same sentence as the header line (snapshot_line.go).
			var ws snapshotwriter.StatusView
			if env.Writer != nil {
				ws = env.Writer()
			}
			p.Note = asSentence(lineFor(ws, r).Text) + " No current boxes are shown."
			p.Layout = agentmap.BuildLayout(agentmap.LayoutInput{Plain: in.Plain})
			return p, nil
		}
		in.Nodes, in.Edges = r.Snapshot.Nodes, r.Snapshot.Edges
		in.Counts = &r.Snapshot.WaitCounts
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
			sprints, flows, err := graph.ReadForeman(env.ForemanRoot)
			if err != nil {
				p.Note = joinNotes(p.Note, "Foreman state could not be read, so sessions are not grouped under sprints.")
			} else {
				hi.Sprints, hi.Flows = sprints, flows
			}
		}
		hi.MergedFor = env.MergedFor
		if hi.MergedFor == nil {
			hi.MergedFor = historyChecker.Merged
		}
		hi.BranchGone, hi.MergedByMessage = env.BranchGone, env.MergedByMessage
		if hi.BranchGone == nil {
			hi.BranchGone = historyChecker.BranchGone
		}
		if hi.MergedByMessage == nil {
			hi.MergedByMessage = historyChecker.MergedByMessage
		}
		if env.St == nil {
			return p, fmt.Errorf("the store is not open")
		}
		g, err := graph.History(env.St, hi)
		if err != nil {
			return p, err
		}
		in.Nodes, in.Edges = g.Nodes, g.Edges
		// One count: the same function the snapshot uses, over the history graph.
		waiting, _ := waits.Build(waits.Input{Now: now, Graph: g, Sprints: hi.Sprints, Flows: hi.Flows})
		c := waits.CountOf(waiting)
		in.Counts = &c
	default:
		return p, fmt.Errorf("source %q is not live or history", req.Source)
	}
	p.Layout = agentmap.BuildLayout(in)
	return p, nil
}

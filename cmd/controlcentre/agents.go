//go:build windows || darwin

package main

import (
	"strconv"

	"burnmon/internal/centre"
	"burnmon/internal/snapshot"
)

// AgentRow is one agent session on the Agents page.
type AgentRow struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Stoppable  bool   `json:"stoppable"`
	State      string `json:"state"`
	Status     string `json:"status"`
	WaitingFor string `json:"waiting_for"`
	Project    string `json:"project"`
	PID        int    `json:"pid"`
	StartedAt  int64  `json:"started_at"`
}

// AgentsView is what ccAgents returns.
type AgentsView struct {
	Available bool          `json:"available"`
	Line      string        `json:"line"`
	Agents    []AgentRow    `json:"agents"`
	Pages     []centre.Span `json:"pages"`
}

// agentsView is the ccAgents handler: every session row of the BurnMon
// snapshot (an entry with an ID, else a SessionID), with its kind, in the
// snapshot's order (first-look fixes item 2). Only a background row is
// stoppable, the kind Stop accepts. A snapshot that is missing, stale or
// unreadable gives no list at all (spec item 7).
func agentsView(e bindEnv, rows int) AgentsView {
	rd := snapshot.Load(e.snapPath, e.clock())
	if rd.Status != snapshot.StatusOK || rd.Snapshot == nil {
		return AgentsView{
			Available: false,
			Line:      "BurnMon Dev is not running, so there is no agent list",
			Agents:    []AgentRow{},
			Pages:     []centre.Span{},
		}
	}
	agents := []AgentRow{}
	stoppable := 0
	for _, a := range rd.Snapshot.AgentSessions {
		id := a.ID
		if id == "" {
			id = a.SessionID
		}
		if id == "" {
			continue
		}
		stop := a.Kind == "background"
		if stop {
			stoppable++
		}
		agents = append(agents, AgentRow{ID: id, Kind: a.Kind, Stoppable: stop, State: a.State, Status: a.Status,
			WaitingFor: a.WaitingFor, Project: a.Project, PID: a.PID, StartedAt: a.StartedAt})
	}
	line := strconv.Itoa(len(agents)) + " sessions, "
	if len(agents) == 1 {
		line = "1 session, "
	}
	line += strconv.Itoa(stoppable) + " can be stopped here."
	return AgentsView{Available: true, Line: line, Agents: agents, Pages: centre.Pages(len(agents), rows)}
}

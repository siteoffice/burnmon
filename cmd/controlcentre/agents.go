//go:build windows || darwin

package main

import (
	"strconv"

	"burnmon/internal/centre"
	"burnmon/internal/snapshot"
)

// AgentRow is one background agent session on the Agents page.
type AgentRow struct {
	ID         string `json:"id"`
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

// agentsView is the ccAgents handler: the background agent sessions of the
// BurnMon snapshot (choices item 26), the ones Stop accepts, in the
// snapshot's order. A snapshot that is missing, stale or unreadable gives no
// list at all (spec item 7).
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
	for _, a := range rd.Snapshot.AgentSessions {
		if a.Kind != "background" || a.ID == "" {
			continue
		}
		agents = append(agents, AgentRow{ID: a.ID, State: a.State, Status: a.Status, WaitingFor: a.WaitingFor,
			Project: a.Project, PID: a.PID, StartedAt: a.StartedAt})
	}
	line := strconv.Itoa(len(agents)) + " agents run."
	if len(agents) == 1 {
		line = "1 agent runs."
	}
	return AgentsView{Available: true, Line: line, Agents: agents, Pages: centre.Pages(len(agents), rows)}
}

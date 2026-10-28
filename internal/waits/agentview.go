package waits

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"
)

// AgentEntry is one session from `claude agents --json`, the documented way
// to read session state from outside Claude Code. Only the fields below are
// read; the session name is not (a renamed session can carry plan text).
type AgentEntry struct {
	Kind, Cwd, ID, State, Status, WaitingFor, SessionID string
	PID                                                 int
	StartedAt                                           int64     // Unix ms
	Since                                               time.Time // set by the poller: first poll that saw it waiting; zero means Now
}

// Key is the item id: the graph's session node id when the entry names its
// session, else a key built from the background id or the pid.
func (e AgentEntry) Key() string {
	switch {
	case e.SessionID != "":
		return "session:anthropic:" + e.SessionID
	case e.ID != "":
		return "agentview:" + e.ID
	}
	return "agentview:pid:" + itoa(e.PID)
}

// Waiting reports whether the entry waits on the person: it says what it
// waits for, or it is a blocked background session.
func (e AgentEntry) Waiting() bool {
	return (e.Status == "waiting" && e.WaitingFor != "") || e.State == "blocked"
}

// YourTurn reports whether the entry is idle: its turn ended and it does
// not wait on a prompt.
func (e AgentEntry) YourTurn() bool { return e.Status == "idle" && !e.Waiting() }

// ParseAgentView parses the output of `claude agents --json`. Anything that
// is not a JSON array (a warning line before it, nothing, an object) is an
// error.
func ParseAgentView(b []byte) ([]AgentEntry, error) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return nil, errors.New("agent view: empty output")
	}
	if b[0] != '[' {
		return nil, errors.New("agent view: output is not a JSON array")
	}
	var raw []struct {
		Kind       string `json:"kind"`
		Cwd        string `json:"cwd"`
		ID         string `json:"id"`
		State      string `json:"state"`
		Status     string `json:"status"`
		WaitingFor string `json:"waitingFor"`
		SessionID  string `json:"sessionId"`
		PID        int    `json:"pid"`
		StartedAt  int64  `json:"startedAt"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, errors.New("agent view: bad JSON: " + err.Error())
	}
	out := make([]AgentEntry, 0, len(raw))
	for _, r := range raw {
		out = append(out, AgentEntry{Kind: r.Kind, Cwd: r.Cwd, ID: r.ID, State: r.State, Status: r.Status,
			WaitingFor: r.WaitingFor, SessionID: r.SessionID, PID: r.PID, StartedAt: r.StartedAt})
	}
	return out, nil
}

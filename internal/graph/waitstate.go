package graph

import (
	"sort"
	"strings"
	"time"
)

// SourceAgentView names the agent view as the source of an edge, the same
// name the snapshot lists in sources[].
const SourceAgentView = "agent_view"

// defaultTick is the snapshot tick when Input.Tick is 0.
const defaultTick = 5 * time.Second

// asksInTranscript lists the agents whose open AskUserQuestion or
// ExitPlanMode call counts as a wait on the person. Cowork cloud is not in it:
// its questions are not visible, a known gap, so it never reads Blocked.
var asksInTranscript = map[string]bool{"claude-code": true, "cowork": true}

// runningChild reports whether a subagent is running now: its last turn is
// within two ticks of Now and its stage is not an end ("waiting", "dormant").
func (b *builder) runningChild(s Session) bool {
	if s.ParentID == "" || s.State == "waiting" || s.State == "dormant" || s.LastActivity.IsZero() {
		return false
	}
	tick := b.in.Tick
	if tick <= 0 {
		tick = defaultTick
	}
	return b.in.Now.Sub(s.LastActivity) <= 2*tick
}

// waitStates sets attr "wait" on every top-level session, marks running
// subagents with attr "running", and draws the waits_on edge for Blocked
// sessions only. idle holds the sessions whose pid file says "idle".
func (b *builder) waitStates(byID map[string]Session, idle map[string]PidFile) {
	agents := map[string]AgentState{}
	for _, a := range b.in.Agents {
		if a.SessionID != "" {
			agents[SessionID("anthropic", a.SessionID)] = a
		}
	}
	parentRuns := map[string]bool{}
	for _, s := range b.in.Sessions {
		if !b.runningChild(s) {
			continue
		}
		id := SessionID(s.Vendor, s.ID)
		if i, ok := b.nodes[id]; ok {
			b.g.Nodes[i].Attrs["running"] = "true"
		}
		parentRuns[SessionID(s.Vendor, s.ParentID)] = true
	}
	ids := make([]string, 0, len(byID))
	for id, s := range byID {
		if s.ParentID == "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		s := byID[id]
		i, ok := b.nodes[id]
		if !ok {
			continue
		}
		pf, hasPid := idle[id]
		a, hasAgent := agents[id]
		age := b.age(s.LastActivity)
		if age == 0 && hasPid {
			age = b.age(pf.UpdatedAt)
		}
		state := WaitWorking
		switch {
		case hasAgent && (a.State == "blocked" || (a.Status == "waiting" && a.WaitingFor != "")):
			state = WaitBlocked
			b.person()
			b.edge(Edge{From: id, To: personID, Kind: KindWaitsOn, Basis: BasisVendor, Source: SourceAgentView, AgeSeconds: age})
		case s.AskOpen && asksInTranscript[strings.ToLower(s.Agent)]:
			state = WaitBlocked
			b.person()
			b.edge(Edge{From: id, To: personID, Kind: KindWaitsOn, Basis: BasisInferred, Source: SourceStore, AgeSeconds: age})
		case parentRuns[id]:
			state = WaitWorking
		case hasAgent && a.Status == "busy":
			// A background task: the agent view says the session is busy.
			state = WaitWorking
		case hasPid || (hasAgent && a.Status == "idle") || s.State == "waiting":
			state = WaitYourTurn
		}
		b.g.Nodes[i].Attrs["wait"] = state
	}
}

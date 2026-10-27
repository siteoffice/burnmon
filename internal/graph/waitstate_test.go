package graph

import (
	"testing"
	"time"
)

func ws(vendor, agent, id, parent, state string, last time.Time) Session {
	return Session{Vendor: vendor, Agent: agent, ID: id, ParentID: parent, State: state, LastActivity: last}
}

func waitOf(t *testing.T, g Graph, id string) string {
	t.Helper()
	n, ok := node(g, id)
	if !ok {
		t.Fatalf("no node %s", id)
	}
	return n.Attrs["wait"]
}

func TestAgentViewBlockedIsBlocked(t *testing.T) {
	g := Build(Input{Now: t0,
		Sessions: []Session{ws("anthropic", "claude-code", "s1", "", "waiting", t0.Add(-20*time.Second))},
		Agents:   []AgentState{{SessionID: "s1", State: "blocked", Status: "waiting", WaitingFor: "permission prompt"}}})
	id := "session:anthropic:s1"
	if got := waitOf(t, g, id); got != WaitBlocked {
		t.Fatalf("wait = %q, want blocked", got)
	}
	e, ok := edge(g, id, personID, KindWaitsOn)
	if !ok || e.Basis != BasisVendor {
		t.Fatalf("waits_on = %+v, %v; want vendor edge", e, ok)
	}
}

func TestPendingAskUserQuestionIsBlocked(t *testing.T) {
	for _, agent := range []string{"claude-code", "cowork", "cowork-cloud"} {
		s := ws("anthropic", agent, "q1", "", "waiting", t0.Add(-20*time.Second))
		s.AskOpen = true
		g := Build(Input{Now: t0, Sessions: []Session{s}})
		id := "session:anthropic:q1"
		if got := waitOf(t, g, id); got != WaitBlocked {
			t.Errorf("%s: wait = %q, want blocked", agent, got)
		}
		e, ok := edge(g, id, personID, KindWaitsOn)
		if !ok || e.Basis != BasisInferred {
			t.Errorf("%s: waits_on = %+v, %v; want inferred edge", agent, e, ok)
		}
	}
	s := ws("openai", "codex", "c1", "", "waiting", t0.Add(-20*time.Second))
	s.AskOpen = true
	g := Build(Input{Now: t0, Sessions: []Session{s}})
	if got := waitOf(t, g, "session:openai:c1"); got != WaitYourTurn {
		t.Errorf("codex: wait = %q, want your_turn", got)
	}
	if _, ok := edge(g, "session:openai:c1", personID, KindWaitsOn); ok {
		t.Error("codex session is Blocked")
	}
}

func TestIdlePidIsYourTurn(t *testing.T) {
	g := Build(Input{Now: t0,
		Sessions: []Session{ws("anthropic", "claude-code", "s1", "", "reading", t0.Add(-20*time.Second))},
		Procs:    []Proc{{PID: 2, PPID: 0, Name: "claude.exe", CreateTime: 100, Harness: "claude"}},
		Pids:     []PidFile{{PID: 2, ProcStart: 100, SessionID: "s1", Cwd: `C:\example\a`, Status: "idle", UpdatedAt: t0}}})
	id := "session:anthropic:s1"
	if got := waitOf(t, g, id); got != WaitYourTurn {
		t.Fatalf("wait = %q, want your_turn", got)
	}
	if _, ok := edge(g, id, personID, KindWaitsOn); ok {
		t.Error("an idle session draws a line to You")
	}
}

func TestStageWaitingIsYourTurn(t *testing.T) {
	g := Build(Input{Now: t0, Sessions: []Session{ws("anthropic", "claude-code", "s1", "", "waiting", t0)}})
	id := "session:anthropic:s1"
	if got := waitOf(t, g, id); got != WaitYourTurn {
		t.Fatalf("wait = %q, want your_turn", got)
	}
	if _, ok := edge(g, id, personID, KindWaitsOn); ok {
		t.Error("a Your turn session draws a line to You")
	}
}

func TestStageBusyIsWorking(t *testing.T) {
	g := Build(Input{Now: t0, Sessions: []Session{ws("anthropic", "claude-code", "s1", "", "reading", t0)}})
	if got := waitOf(t, g, "session:anthropic:s1"); got != WaitWorking {
		t.Fatalf("wait = %q, want working", got)
	}
}

func TestParentWithRunningSubagentIsWorking(t *testing.T) {
	in := Input{Now: t0, Tick: 5 * time.Second,
		Sessions: []Session{
			ws("anthropic", "claude-code", "p", "", "waiting", t0.Add(-60*time.Second)),
			ws("anthropic", "claude-code", "agent-k", "p", "running", t0.Add(-4*time.Second)),
		},
		Procs: []Proc{{PID: 2, PPID: 0, Name: "claude.exe", CreateTime: 100, Harness: "claude"}},
		Pids:  []PidFile{{PID: 2, ProcStart: 100, SessionID: "p", Cwd: `C:\example\a`, Status: "idle", UpdatedAt: t0}}}
	g := Build(in)
	p, k := "session:anthropic:p", "session:anthropic:agent-k"
	if got := waitOf(t, g, p); got != WaitWorking {
		t.Fatalf("parent wait = %q, want working", got)
	}
	if _, ok := edge(g, p, personID, KindWaitsOn); ok {
		t.Error("a parent with a running subagent draws a line to You")
	}
	kn, _ := node(g, k)
	if kn.Attrs["running"] != "true" {
		t.Errorf("child attrs = %v, want running=true", kn.Attrs)
	}
}

func TestFinishedSubagentLeavesParentYourTurn(t *testing.T) {
	for name, child := range map[string]Session{
		"stage waiting": ws("anthropic", "claude-code", "agent-k", "p", "waiting", t0.Add(-4*time.Second)),
		"old activity":  ws("anthropic", "claude-code", "agent-k", "p", "running", t0.Add(-30*time.Second)),
	} {
		g := Build(Input{Now: t0, Tick: 5 * time.Second, Sessions: []Session{
			ws("anthropic", "claude-code", "p", "", "waiting", t0.Add(-60*time.Second)), child}})
		if got := waitOf(t, g, "session:anthropic:p"); got != WaitYourTurn {
			t.Errorf("%s: parent wait = %q, want your_turn", name, got)
		}
		kn, _ := node(g, "session:anthropic:agent-k")
		if _, ok := kn.Attrs["running"]; ok {
			t.Errorf("%s: finished child has running attr", name)
		}
		if _, ok := kn.Attrs["wait"]; ok {
			t.Errorf("%s: subagent has a wait attr", name)
		}
	}
}

func TestSubagentNeverWaits(t *testing.T) {
	k := ws("anthropic", "claude-code", "agent-k", "p", "waiting", t0)
	k.AskOpen = true
	g := Build(Input{Now: t0,
		Sessions: []Session{ws("anthropic", "claude-code", "p", "", "reading", t0), k},
		Agents:   []AgentState{{SessionID: "agent-k", State: "blocked"}}})
	kid := "session:anthropic:agent-k"
	kn, _ := node(g, kid)
	if _, ok := kn.Attrs["wait"]; ok {
		t.Error("subagent has a wait attr")
	}
	if _, ok := edge(g, kid, personID, KindWaitsOn); ok {
		t.Error("subagent waits on you")
	}
}

func TestBlockedBeatsWorking(t *testing.T) {
	p := ws("anthropic", "claude-code", "p", "", "waiting", t0.Add(-60*time.Second))
	p.AskOpen = true
	g := Build(Input{Now: t0, Tick: 5 * time.Second, Sessions: []Session{p,
		ws("anthropic", "claude-code", "agent-k", "p", "running", t0.Add(-2*time.Second))}})
	if got := waitOf(t, g, "session:anthropic:p"); got != WaitBlocked {
		t.Fatalf("wait = %q, want blocked", got)
	}
	if _, ok := edge(g, "session:anthropic:p", personID, KindWaitsOn); !ok {
		t.Error("blocked session has no waits_on edge")
	}
}

func TestSprintMainRepoExported(t *testing.T) {
	if got := SprintMainRepo(`C:\example\repo\.worktrees\sprint-x\`); got != `C:\example\repo` {
		t.Errorf("SprintMainRepo = %q", got)
	}
}

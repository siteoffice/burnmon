package controls

import (
	"testing"

	"burnmon/internal/graph"
	"burnmon/internal/waits"
)

func agentTarget(control string) Target {
	return Target{Control: control, AgentID: "ab12", PID: 42, StartedAt: 1}
}

func agentEntry() waits.AgentEntry {
	return waits.AgentEntry{Kind: "background", ID: "ab12", State: "blocked", PID: 42, StartedAt: 1, Cwd: `C:\example\project`}
}

func TestCheckAgent(t *testing.T) {
	self := Self{PID: 10, Ancestors: []int{50}, ExeDir: `C:\example\burnmon`}
	mod := func(f func(*waits.AgentEntry)) []waits.AgentEntry {
		e := agentEntry()
		f(&e)
		return []waits.AgentEntry{e}
	}
	got, err := CheckAgent(agentTarget(Stop), []waits.AgentEntry{agentEntry()}, self)
	if err != nil || got.ID != "ab12" || got.PID != 42 {
		t.Fatalf("matching entry: %+v %v", got, err)
	}
	cases := []struct {
		name  string
		fresh []waits.AgentEntry
		want  string
	}{
		{"missing", []waits.AgentEntry{{Kind: "background", ID: "zz99", PID: 42, StartedAt: 1}}, "the session is no longer listed"},
		{"empty", nil, "the session is no longer listed"},
		{"interactive", mod(func(e *waits.AgentEntry) { e.Kind = "interactive" }), "only background sessions have controls"},
		{"pid", mod(func(e *waits.AgentEntry) { e.PID = 43 }), "the session changed since the preview, press again"},
		{"started", mod(func(e *waits.AgentEntry) { e.StartedAt = 2 }), "the session changed since the preview, press again"},
		{"stopped", mod(func(e *waits.AgentEntry) { e.State = "stopped" }), "the session has already ended"},
		{"done", mod(func(e *waits.AgentEntry) { e.State = "done" }), "the session has already ended"},
		{"failed", mod(func(e *waits.AgentEntry) { e.State = "failed" }), "the session has already ended"},
		{"ancestor", mod(func(e *waits.AgentEntry) { e.PID = 50 }), "this session started BurnMon Dev"},
		{"own folder", mod(func(e *waits.AgentEntry) { e.Cwd = `C:\example\burnmon\x` }), "this session works in BurnMon Dev's own folder"},
	}
	for _, c := range cases {
		tgt := agentTarget(Stop)
		if c.name == "ancestor" {
			tgt.PID = 50
		}
		_, err := CheckAgent(tgt, c.fresh, self)
		if err == nil || err.Error() != c.want {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
	// A stopped session may still be attached.
	if _, err := CheckAgent(agentTarget(Attach), mod(func(e *waits.AgentEntry) { e.State = "stopped" }), self); err != nil {
		t.Errorf("attach on stopped: %v", err)
	}
	// Never-self applies to attach too.
	if _, err := CheckAgent(agentTarget(Attach), mod(func(e *waits.AgentEntry) { e.Cwd = `C:\example\burnmon` }), self); err == nil {
		t.Errorf("attach in own folder must be refused")
	}
}

func gateTarget() Target {
	return Target{Control: Approve, FlowID: "f1", Gate: "G2", GateToken: "t1", FlowDir: `C:\example\f1`}
}

func gateFlow() graph.Flow {
	return graph.Flow{ID: "f1", Gate: "G2", GatePending: true, GateToken: "t1", Dir: `C:\example\f1`}
}

func TestCheckGate(t *testing.T) {
	if err := CheckGate(gateTarget(), []graph.Flow{{ID: "other"}, gateFlow()}); err != nil {
		t.Fatalf("matching flow: %v", err)
	}
	mod := func(f func(*graph.Flow)) []graph.Flow {
		g := gateFlow()
		f(&g)
		return []graph.Flow{g}
	}
	changed := "the gate changed since the preview, press again"
	cases := []struct {
		name  string
		flows []graph.Flow
		want  string
	}{
		{"missing", []graph.Flow{{ID: "other"}}, "the flow is no longer listed"},
		{"decided", mod(func(g *graph.Flow) { g.GatePending = false }), "the gate is already decided"},
		{"gate", mod(func(g *graph.Flow) { g.Gate = "G3" }), changed},
		{"token", mod(func(g *graph.Flow) { g.GateToken = "t2" }), changed},
		{"dir", mod(func(g *graph.Flow) { g.Dir = `C:\example\f2` }), changed},
	}
	for _, c := range cases {
		err := CheckGate(gateTarget(), c.flows)
		if err == nil || err.Error() != c.want {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
	// An empty token never passes, even when both sides are empty.
	tg := gateTarget()
	tg.GateToken = ""
	g := gateFlow()
	g.GateToken = ""
	if err := CheckGate(tg, []graph.Flow{g}); err == nil {
		t.Errorf("empty token must never pass")
	}
}

func TestCheckAgentRefusesMissingPid(t *testing.T) {
	self := Self{PID: 10, ExeDir: `C:\example\burnmon`}
	noPID := "the session has no process id"
	e := agentEntry()
	e.PID = 0
	tgt := agentTarget(Stop)
	tgt.PID = 0
	if _, err := CheckAgent(tgt, []waits.AgentEntry{e}, self); err == nil || err.Error() != noPID {
		t.Errorf("both pid 0: %v", err)
	}
	tgt.PID = 42
	if _, err := CheckAgent(tgt, []waits.AgentEntry{e}, self); err == nil || err.Error() != noPID {
		t.Errorf("fresh pid 0: %v", err)
	}
	e.PID = 42
	tgt.PID = -1
	if _, err := CheckAgent(tgt, []waits.AgentEntry{e}, self); err == nil || err.Error() != noPID {
		t.Errorf("preview pid -1: %v", err)
	}
}

func TestCheckAgentRefusesDuplicateIDs(t *testing.T) {
	self := Self{PID: 10, ExeDir: `C:\example\burnmon`}
	a, b := agentEntry(), agentEntry()
	b.PID = 99
	_, err := CheckAgent(agentTarget(Stop), []waits.AgentEntry{a, b}, self)
	if err == nil || err.Error() != "the session id is listed more than once" {
		t.Fatalf("got %v", err)
	}
}

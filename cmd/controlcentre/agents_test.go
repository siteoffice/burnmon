//go:build windows || darwin

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"burnmon/internal/controls"
)

const noAgentList = "BurnMon Dev is not running, so there is no agent list"

func agentsEnv(snap string, at time.Time) bindEnv {
	return bindEnv{snapPath: snap, now: func() time.Time { return at }}
}

func TestAgentsOnRecordedSnapshot(t *testing.T) {
	v := agentsView(agentsEnv(exampleSnap, rigT0), 10)
	if !v.Available {
		t.Fatalf("view = %+v, want available", v)
	}
	var ids []string
	for _, a := range v.Agents {
		ids = append(ids, a.ID)
	}
	if strings.Join(ids, ",") != "ab12,cd34,example-parent" {
		t.Errorf("ids = %v, want [ab12 cd34 example-parent] (every session row)", ids)
	}
	a := v.Agents[0]
	if a.State != "blocked" || a.WaitingFor != "input needed" || a.Project != "other" || a.PID != 4242 {
		t.Errorf("first row = %+v", a)
	}
	if len(v.Pages) != 1 || v.Pages[0].From != 0 || v.Pages[0].To != 3 {
		t.Errorf("pages = %+v", v.Pages)
	}
	if v.Line != "3 sessions, 2 can be stopped here." {
		t.Errorf("line = %q", v.Line)
	}
	if p := agentsView(agentsEnv(exampleSnap, rigT0), 1).Pages; len(p) != 3 {
		t.Errorf("one row per page: pages = %+v", p)
	}
}

// agentsSnap writes the recorded snapshot with its agent_sessions replaced
// by sessions to a temp file and returns its path.
func agentsSnap(t *testing.T, sessions []map[string]any) string {
	t.Helper()
	raw, err := os.ReadFile(exampleSnap)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m["agent_sessions"] = sessions
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAgentsAllSessions(t *testing.T) {
	path := agentsSnap(t, []map[string]any{
		{"kind": "background", "id": "ab12", "state": "blocked", "project": "other"},
		{"kind": "interactive", "session_id": "ia01", "status": "busy", "project": "proj"},
		{"kind": "interactive", "session_id": "ia02", "project": "proj"},
		{"kind": "interactive", "project": "proj"},
	})
	v := agentsView(agentsEnv(path, rigT0), 10)
	if !v.Available {
		t.Fatalf("view = %+v, want available", v)
	}
	var ids, kinds, stop []string
	for _, a := range v.Agents {
		ids = append(ids, a.ID)
		kinds = append(kinds, a.Kind)
		if a.Stoppable {
			stop = append(stop, a.ID)
		}
	}
	if strings.Join(ids, ",") != "ab12,ia01,ia02" {
		t.Errorf("ids = %v, want [ab12 ia01 ia02]", ids)
	}
	if strings.Join(kinds, ",") != "background,interactive,interactive" {
		t.Errorf("kinds = %v", kinds)
	}
	if strings.Join(stop, ",") != "ab12" {
		t.Errorf("stoppable = %v, want [ab12]", stop)
	}
	if v.Line != "3 sessions, 1 can be stopped here." {
		t.Errorf("line = %q", v.Line)
	}
	if len(v.Pages) != 1 || v.Pages[0].From != 0 || v.Pages[0].To != 3 {
		t.Errorf("pages = %+v", v.Pages)
	}
}

func TestPageAgentsKindAndStopRule(t *testing.T) {
	pa := pageFunc(t, "pageAgents")
	for _, want := range []string{"'Kind'", "r.kind"} {
		if !strings.Contains(pa, want) {
			t.Errorf("pageAgents lacks %s", want)
		}
	}
	ad := pageFunc(t, "agentDetail")
	for _, want := range []string{"r.stoppable", "'Stop works on background agents only.'", "'Stop'", "decide('stop'"} {
		if !strings.Contains(ad, want) {
			t.Errorf("agentDetail lacks %s", want)
		}
	}
}

func checkNoAgentList(t *testing.T, v AgentsView) {
	t.Helper()
	if v.Available || v.Line != noAgentList {
		t.Errorf("view = %+v, want unavailable with %q", v, noAgentList)
	}
	if v.Agents == nil || len(v.Agents) != 0 || v.Pages == nil || len(v.Pages) != 0 {
		t.Errorf("agents %v pages %v, want both empty and not nil", v.Agents, v.Pages)
	}
}

func TestAgentsMissingSnapshot(t *testing.T) {
	checkNoAgentList(t, agentsView(agentsEnv(filepath.Join(t.TempDir(), "snapshot.json"), rigT0), 10))
}

func TestAgentsStaleSnapshot(t *testing.T) {
	checkNoAgentList(t, agentsView(agentsEnv(exampleSnap, time.Date(2026, 10, 23, 11, 0, 0, 0, time.UTC)), 10))
}

func TestStopThroughFakeRunner(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the fixture cwd is a Windows path; the folder checks parse it only on Windows")
	}
	r := newDecisionRig(t)
	view, _ := json.Marshal([]map[string]any{{"kind": "background", "id": "ab12", "cwd": `C:\example\other`,
		"state": "blocked", "status": "waiting", "pid": 4242, "startedAt": 1761213600000}})
	r.svc.Env.AgentView = func(ctx context.Context) ([]byte, error) { return view, nil }
	r.runner.line = "stopped ab12"

	if _, err := previewDecision(r.be, DecisionRequest{Control: controls.Stop}); err == nil || err.Error() != "stop needs an agent" {
		t.Errorf("stop without an agent: err = %v", err)
	}
	pr, err := previewDecision(r.be, DecisionRequest{Control: controls.Stop, Agent: "ab12"})
	if err != nil {
		t.Fatal(err)
	}
	if pr.Command != controls.CommandLine([]string{`C:\example\claude.exe`, "stop", "ab12"}) {
		t.Errorf("command = %q", pr.Command)
	}
	if n := r.runner.calls(); n != 0 {
		t.Fatalf("preview ran the runner %d times", n)
	}
	r.clock.add(time.Second)
	rv, err := runDecision(r.be, pr.Token)
	if err != nil {
		t.Fatal(err)
	}
	if rv.ExitCode != 0 || rv.Refused != "" || rv.AuditPath != r.audit {
		t.Fatalf("run = %+v", rv)
	}
	if len(r.runner.runs) != 1 || strings.Join(r.runner.runs[0], " ") != `C:\example\claude.exe stop ab12` {
		t.Fatalf("runs = %v", r.runner.runs)
	}
	lines := r.auditLines()
	if len(lines) != 2 || lines[0]["phase"] != "start" {
		t.Fatalf("audit lines = %v, want a start line and a result line", lines)
	}
	if _, ok := lines[1]["phase"]; ok {
		t.Errorf("result line has a phase: %v", lines[1])
	}
	for _, l := range lines {
		if l["control"] != "stop" || l["target"] != "ab12" {
			t.Errorf("audit line %v", l)
		}
	}
}

func TestPageAgents(t *testing.T) {
	pa := pageFunc(t, "pageAgents")
	for _, want := range []string{"api.agents(", "rowsFit()", "pager(", "agentDetail(", "'ID'", "'Project'", "'State'", "'Waiting for'"} {
		if !strings.Contains(pa, want) {
			t.Errorf("pageAgents lacks %s", want)
		}
	}
	for _, bad := range []string{".sort(", ".reverse(", "api.preview(", "api.run("} {
		if strings.Contains(pa, bad) {
			t.Errorf("pageAgents has %s", bad)
		}
	}
	ad := pageFunc(t, "agentDetail")
	if !strings.Contains(ad, "'Stop'") || !strings.Contains(ad, "decide('stop'") {
		t.Error("agentDetail must offer Stop through decide('stop', ...)")
	}
	if strings.Contains(ad, "api.preview(") || strings.Contains(ad, "api.run(") {
		t.Error("agentDetail must reuse decide, not call the bindings itself")
	}
	// Stop sends only the control and the agent id.
	d := pageFunc(t, "decide")
	if !regexp.MustCompile(`\{control: control, agent: r\.id\}`).MatchString(d) {
		t.Error("decide must send {control: control, agent: r.id} for stop")
	}
	if strings.Contains(pageHTML, "The agent list is not built in this version.") {
		t.Error("page still carries the stub line")
	}
}

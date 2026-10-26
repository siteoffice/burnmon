package controls

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"burnmon/internal/graph"
	"burnmon/internal/snapshot"
)

const (
	svcClaude = `C:\example\claude.exe`
	svcPython = `C:\example\python.exe`
)

type fakeRunner struct {
	mu      sync.Mutex
	runs    [][]string
	starts  [][]string
	dirs    []string
	code    int
	line    string
	err     error
	startEr error
	entered chan struct{} // closed-over signal: sent once per Run when set
	release chan struct{} // Run blocks on it when set
}

func (f *fakeRunner) Run(ctx context.Context, argv []string, dir string) (int, string, error) {
	f.mu.Lock()
	f.runs = append(f.runs, append([]string(nil), argv...))
	f.dirs = append(f.dirs, dir)
	f.mu.Unlock()
	if f.entered != nil {
		f.entered <- struct{}{}
	}
	if f.release != nil {
		<-f.release
	}
	return f.code, f.line, f.err
}

func (f *fakeRunner) Start(argv []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts = append(f.starts, append([]string(nil), argv...))
	return f.startEr
}

func (f *fakeRunner) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.runs) + len(f.starts)
}

type rig struct {
	t      *testing.T
	clock  *fakeClock
	runner *fakeRunner
	svc    *Service
	env    *Env
	audit  string
	root   string

	mu       sync.Mutex
	agent    string
	agentErr error
	flows    []graph.Flow
	foreErr  error
	switches map[string]bool
	self     Self
	selfErr  error
	elevated bool
	snap     func(*snapshot.Snapshot)
}

func agentJSON(id string, pid int, state, cwd string) string {
	b, _ := json.Marshal([]map[string]any{{"kind": "background", "id": id, "cwd": cwd, "state": state,
		"status": "waiting", "pid": pid, "startedAt": 1761213600000}})
	return string(b)
}

func newRig(t *testing.T) *rig {
	t.Helper()
	root := t.TempDir()
	lib := filepath.Join(root, "flows", "_lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "flow_transition.py"), []byte("# fake\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &rig{t: t, clock: &fakeClock{t: t0}, runner: &fakeRunner{line: "stopped ab12"},
		audit: filepath.Join(t.TempDir(), "controls-audit.ndjson"), root: root,
		agent:    agentJSON("ab12", 4242, "blocked", `C:\example\other`),
		switches: map[string]bool{Stop: true, Attach: true, Approve: true, SendBack: true},
		self:     Self{PID: 1, ExeDir: `C:\example\burnmon`},
		flows: []graph.Flow{{ID: "example__flow__2", Gate: "G2", GatePending: true, GateToken: "t1",
			Dir: filepath.Join(root, "flows", "example", "flow-review")}},
	}
	r.env = &Env{
		Now:         r.clock.now,
		ClaudePath:  svcClaude,
		PythonPath:  svcPython,
		AgentViewOn: true,
		ForemanRoot: root,
		AgentView: func(ctx context.Context) ([]byte, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			return []byte(r.agent), r.agentErr
		},
		ReadForeman: func(string) ([]graph.Sprint, []graph.Flow, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			return nil, append([]graph.Flow(nil), r.flows...), r.foreErr
		},
		Snapshot: func() snapshot.Reading {
			rd := snapshot.Load("../../testdata/snapshot_example.json", time.Date(2026, 10, 23, 10, 0, 10, 0, time.UTC))
			if rd.Snapshot != nil && r.snap != nil {
				r.snap(rd.Snapshot)
			}
			return rd
		},
		Switches: func() map[string]bool {
			r.mu.Lock()
			defer r.mu.Unlock()
			m := map[string]bool{}
			for k, v := range r.switches {
				m[k] = v
			}
			return m
		},
		Self: func() (Self, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			return r.self, r.selfErr
		},
		Elevated:  func() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.elevated },
		Runner:    r.runner,
		AuditPath: r.audit,
	}
	r.svc = NewService(*r.env)
	return r
}

func (r *rig) rebuild() { r.svc = NewService(*r.env) }

func (r *rig) preview(control, target string) (PreviewResult, error) {
	return r.svc.Preview(PreviewRequest{Control: control, Target: target})
}

// confirm advances the clock past the confirm gap and runs the token.
func (r *rig) confirm(token string) (RunResult, error) {
	r.clock.t = r.clock.t.Add(time.Second)
	return r.svc.Run(token)
}

func (r *rig) auditLines() []map[string]any {
	r.t.Helper()
	b, err := os.ReadFile(r.audit)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		r.t.Fatal(err)
	}
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			r.t.Fatalf("audit line is not JSON: %q", l)
		}
		out = append(out, m)
	}
	return out
}

// last is the final audit line (the result of the run).
func (r *rig) last() map[string]any {
	r.t.Helper()
	ls := r.auditLines()
	if len(ls) == 0 {
		r.t.Fatal("no audit line")
	}
	return ls[len(ls)-1]
}

func (r *rig) gateTarget() string { return "gate:example__flow__2:G2" }

func TestPreviewShowsExactStopCommand(t *testing.T) {
	r := newRig(t)
	p, err := r.preview(Stop, "ab12")
	if err != nil {
		t.Fatal(err)
	}
	if p.Command != CommandLine([]string{svcClaude, "stop", "ab12"}) || p.Label != "other (ab12)" || p.ExpiresInMs != 5000 {
		t.Fatalf("%+v", p)
	}
	if len(p.Token) != 32 {
		t.Fatalf("token %q", p.Token)
	}
	if r.runner.calls() != 0 {
		t.Fatal("preview ran something")
	}
	if _, err := os.Stat(r.audit); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("preview wrote the audit file")
	}
}

func TestRunStopsAfterSecondPress(t *testing.T) {
	r := newRig(t)
	p, _ := r.preview(Stop, "ab12")
	res, err := r.confirm(p.Token)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.runner.runs) != 1 || strings.Join(r.runner.runs[0], " ") != svcClaude+" stop ab12" {
		t.Fatalf("runs %v", r.runner.runs)
	}
	if res.ExitCode != 0 || res.Line != "stopped ab12" || res.Failed || res.Refused != "" {
		t.Fatalf("%+v", res)
	}
	l := r.last()
	if l["control"] != "stop" || l["target"] != "ab12" || l["command"] != p.Command || l["exit_code"] != float64(0) {
		t.Fatalf("%v", l)
	}
	if _, has := l["refused"]; has {
		t.Fatalf("%v", l)
	}
}

func TestRunWithoutPreviewRunsNothing(t *testing.T) {
	r := newRig(t)
	_, err := r.svc.Run("deadbeef")
	if !errors.Is(err, ErrNoPreview) {
		t.Fatalf("%v", err)
	}
	if r.runner.calls() != 0 || len(r.auditLines()) != 0 {
		t.Fatal("something ran or was audited")
	}
}

func TestRunAfterTimeoutRunsNothing(t *testing.T) {
	r := newRig(t)
	p, _ := r.preview(Stop, "ab12")
	r.clock.t = r.clock.t.Add(6 * time.Second)
	res, err := r.svc.Run(p.Token)
	if !errors.Is(err, ErrExpired) || res.Refused == "" {
		t.Fatalf("%v %+v", err, res)
	}
	if r.runner.calls() != 0 {
		t.Fatal("ran")
	}
	ls := r.auditLines()
	if len(ls) != 1 || ls[0]["refused"] == nil || ls[0]["exit_code"] != float64(-1) {
		t.Fatalf("%v", ls)
	}
}

func TestSecondPressTooSoonIsAuditedAndKeepsToken(t *testing.T) {
	r := newRig(t)
	p, _ := r.preview(Stop, "ab12")
	r.clock.t = r.clock.t.Add(50 * time.Millisecond)
	if _, err := r.svc.Run(p.Token); !errors.Is(err, ErrTooSoon) {
		t.Fatalf("%v", err)
	}
	if r.runner.calls() != 0 || len(r.auditLines()) != 1 {
		t.Fatal("too-soon press ran or was not audited")
	}
	r.clock.t = r.clock.t.Add(time.Second)
	if _, err := r.svc.Run(p.Token); err != nil {
		t.Fatalf("deliberate press: %v", err)
	}
	if len(r.runner.runs) != 1 {
		t.Fatal("deliberate press did not run")
	}
}

func TestRunRefusesChangedSession(t *testing.T) {
	cases := []struct{ name, agent, want string }{
		{"new pid", agentJSON("ab12", 99, "blocked", `C:\example\other`), "the session changed since the preview, press again"},
		{"stopped", agentJSON("ab12", 4242, "stopped", `C:\example\other`), "the session has already ended"},
		{"gone", agentJSON("zz99", 4242, "blocked", `C:\example\other`), "the session is no longer listed"},
		{"agent view fails", "boom", "agent view"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			p, err := r.preview(Stop, "ab12")
			if err != nil {
				t.Fatal(err)
			}
			r.agent = c.agent
			res, err := r.confirm(p.Token)
			if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(res.Refused, c.want) {
				t.Fatalf("%v %+v", err, res)
			}
			if r.runner.calls() != 0 {
				t.Fatal("ran")
			}
			l := r.last()
			if l["exit_code"] != float64(-1) || !strings.Contains(l["refused"].(string), c.want) {
				t.Fatalf("%v", l)
			}
		})
	}
}

func TestNeverSelfRefusesAtPreviewAndRun(t *testing.T) {
	r := newRig(t)
	r.self.Ancestors = []int{4242}
	if _, err := r.preview(Stop, "ab12"); err == nil || !strings.Contains(err.Error(), "this session started BurnMon Dev") {
		t.Fatalf("preview: %v", err)
	}
	r.self.Ancestors = nil
	p, err := r.preview(Stop, "ab12")
	if err != nil {
		t.Fatal(err)
	}
	r.agent = agentJSON("ab12", 4242, "blocked", `C:\example\burnmon\.worktrees\x`)
	if _, err := r.confirm(p.Token); err == nil || !strings.Contains(err.Error(), "this session works in BurnMon Dev's own folder") {
		t.Fatalf("run: %v", err)
	}
	if r.runner.calls() != 0 || r.last()["refused"] == nil {
		t.Fatal("ran or not audited")
	}
}

func TestPreviewRefusesBadID(t *testing.T) {
	r := newRig(t)
	for _, id := range []string{"-x", "a b", "%PATH%", "cd34 ", "example-parent", ""} {
		if _, err := r.preview(Stop, id); err == nil {
			t.Fatalf("id %q accepted", id)
		}
	}
	r.snap = func(s *snapshot.Snapshot) {
		s.AgentSessions = append(s.AgentSessions, snapshot.AgentSession{Kind: "background", ID: "-all", PID: 5, StartedAt: 1})
	}
	if _, err := r.preview(Stop, "-all"); err == nil {
		t.Fatal("-all accepted")
	}
	if r.runner.calls() != 0 {
		t.Fatal("ran")
	}
}

func TestPreviewRefusesInteractiveAndUnknown(t *testing.T) {
	r := newRig(t)
	r.snap = func(s *snapshot.Snapshot) {
		s.AgentSessions = append(s.AgentSessions, snapshot.AgentSession{Kind: "interactive", ID: "ef56", PID: 6, StartedAt: 1})
	}
	for _, id := range []string{"ef56", "nope"} {
		if _, err := r.preview(Stop, id); err == nil {
			t.Fatalf("%q accepted", id)
		}
	}
	if _, err := r.preview("reply", "ab12"); err == nil {
		t.Fatal("unknown control accepted")
	}
}

func TestPreviewNeedsOkSnapshot(t *testing.T) {
	r := newRig(t)
	r.env.Snapshot = func() snapshot.Reading { return snapshot.Reading{Status: "stale"} }
	r.rebuild()
	if _, err := r.preview(Stop, "ab12"); err == nil {
		t.Fatal("stale snapshot accepted")
	}
}

func TestRunFailsClosedWithoutAudit(t *testing.T) {
	r := newRig(t)
	blocker := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.env.AuditPath = filepath.Join(blocker, "controls-audit.ndjson")
	r.rebuild()
	p, err := r.preview(Stop, "ab12")
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.confirm(p.Token)
	if err == nil || !strings.Contains(err.Error(), "audit file cannot be written") {
		t.Fatalf("%v", err)
	}
	if r.runner.calls() != 0 {
		t.Fatal("ran without audit")
	}
}

// svcFailWriter fails on the nth write, optionally after writing half the bytes.
type svcFailWriter struct {
	mu     sync.Mutex
	buf    []byte
	writes int
	failAt int // 1-based; 0 never
	half   bool
}

func (f *svcFailWriter) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	if f.failAt != 0 && f.writes == f.failAt {
		if f.half {
			f.buf = append(f.buf, p[:len(p)/2]...)
			return len(p) / 2, errors.New("disk full")
		}
		return 0, errors.New("disk full")
	}
	f.buf = append(f.buf, p...)
	return len(p), nil
}
func (f *svcFailWriter) Close() error { return nil }

func TestRunDoesNotRunWhenAuditLineCannotBeWritten(t *testing.T) {
	for _, half := range []bool{false, true} {
		r := newRig(t)
		fw := &svcFailWriter{failAt: 1, half: half}
		r.svc.openAudit = func(string) (io.WriteCloser, error) { return fw, nil }
		p, _ := r.preview(Stop, "ab12")
		_, err := r.confirm(p.Token)
		if err == nil || !strings.Contains(err.Error(), "audit file cannot be written") {
			t.Fatalf("half=%v: %v", half, err)
		}
		if r.runner.calls() != 0 {
			t.Fatalf("half=%v: ran after the audit line failed", half)
		}
		// a partial line is closed with a newline so the next line is not glued to it
		if half && !strings.HasSuffix(string(fw.buf), "\n") {
			t.Fatalf("partial line left open: %q", fw.buf)
		}
	}
}

func TestAuditNewlineRepairsPartialLineFromEarlierRun(t *testing.T) {
	r := newRig(t)
	if err := os.WriteFile(r.audit, []byte(`{"time":"2026-10-26T09`), 0o600); err != nil {
		t.Fatal(err)
	}
	p, _ := r.preview(Stop, "ab12")
	if _, err := r.confirm(p.Token); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(r.audit)
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(lines) < 2 || lines[0] != `{"time":"2026-10-26T09` {
		t.Fatalf("%q", lines)
	}
	for _, l := range lines[1:] {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("glued or broken line %q", l)
		}
	}
}

func TestRunnerNonzeroExitIsFailure(t *testing.T) {
	r := newRig(t)
	r.runner.code, r.runner.line = 2, "no such session"
	p, _ := r.preview(Stop, "ab12")
	res, err := r.confirm(p.Token)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 2 || !res.Failed || res.Refused != "" || res.Line != "no such session" {
		t.Fatalf("%+v", res)
	}
	if r.last()["exit_code"] != float64(2) {
		t.Fatalf("%v", r.last())
	}
}

func TestRunnerErrorIsFailure(t *testing.T) {
	r := newRig(t)
	r.runner.code, r.runner.err = 0, errors.New("deadline exceeded")
	p, _ := r.preview(Stop, "ab12")
	res, err := r.confirm(p.Token)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != -1 || !res.Failed || !strings.Contains(res.Line, "deadline exceeded") {
		t.Fatalf("%+v", res)
	}
	if r.last()["exit_code"] != float64(-1) {
		t.Fatalf("%v", r.last())
	}
}

func TestAttachStartsConsole(t *testing.T) {
	r := newRig(t)
	p, err := r.preview(Attach, "ab12")
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.confirm(p.Token)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.runner.starts) != 1 || len(r.runner.runs) != 0 || strings.Join(r.runner.starts[0], " ") != svcClaude+" attach ab12" {
		t.Fatalf("%v %v", r.runner.starts, r.runner.runs)
	}
	if res.Failed || r.last()["exit_code"] != float64(0) {
		t.Fatalf("%+v %v", res, r.last())
	}

	r2 := newRig(t)
	r2.runner.startEr = errors.New("cannot start")
	p, _ = r2.preview(Attach, "ab12")
	res, err = r2.confirm(p.Token)
	if err != nil || !res.Failed || res.ExitCode != -1 || !strings.Contains(res.Line, "cannot start") || r2.last()["exit_code"] != float64(-1) {
		t.Fatalf("%v %+v %v", err, res, r2.last())
	}
}

func TestGateApproveRunsCmdGate(t *testing.T) {
	r := newRig(t)
	lib := filepath.Join(r.root, "flows", "_lib")
	dir := filepath.Join(r.root, "flows", "example", "flow-review")
	p, err := r.preview(Approve, r.gateTarget())
	if err != nil {
		t.Fatal(err)
	}
	want := GateArgv(svcPython, lib, dir, "approve", "G2", "t1")
	if p.Command != CommandLine(want) || p.Label != "flow-review gate G2 (example__flow__2)" {
		t.Fatalf("%+v", p)
	}
	res, err := r.confirm(p.Token)
	if err != nil || res.Failed {
		t.Fatalf("%v %+v", err, res)
	}
	if len(r.runner.runs) != 1 || strings.Join(r.runner.runs[0], "\x00") != strings.Join(want, "\x00") || r.runner.dirs[0] != lib {
		t.Fatalf("%v dir %v", r.runner.runs, r.runner.dirs)
	}
	if r.last()["target"] != r.gateTarget() || r.last()["control"] != "approve" {
		t.Fatalf("%v", r.last())
	}
}

func TestGateRefusesChangedToken(t *testing.T) {
	r := newRig(t)
	p, _ := r.preview(Approve, r.gateTarget())
	r.flows[0].GateToken = "t2"
	_, err := r.confirm(p.Token)
	if err == nil || !strings.Contains(err.Error(), "the gate changed since the preview, press again") {
		t.Fatalf("%v", err)
	}
	if r.runner.calls() != 0 || r.last()["refused"] == nil {
		t.Fatal("ran or not audited")
	}
}

func TestGateTokenForAIsNeverRunForB(t *testing.T) {
	r := newRig(t)
	r.flows = append(r.flows, graph.Flow{ID: "example__flow__3", Gate: "G2", GatePending: true, GateToken: "t3",
		Dir: filepath.Join(r.root, "flows", "example", "other")})
	r.snap = func(s *snapshot.Snapshot) {
		s.WaitItems = append(s.WaitItems, s.WaitItems[len(s.WaitItems)-1])
		s.WaitItems[len(s.WaitItems)-1].ID = "gate:example__flow__3:G2"
	}
	pa, err := r.preview(Approve, r.gateTarget())
	if err != nil {
		t.Fatal(err)
	}
	pb, err := r.preview(Approve, "gate:example__flow__3:G2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.confirm(pa.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Run(pb.Token); err != nil {
		t.Fatal(err)
	}
	got := []string{r.runner.runs[0][len(r.runner.runs[0])-1], r.runner.runs[1][len(r.runner.runs[1])-1]}
	if got[0] != "t1" || got[1] != "t3" || r.runner.runs[0][5] == r.runner.runs[1][5] {
		t.Fatalf("each token must run only its own argv: %v", r.runner.runs)
	}
}

func TestGateControlsNotOffered(t *testing.T) {
	r := newRig(t)
	r.snap = func(s *snapshot.Snapshot) {
		w := s.WaitItems[len(s.WaitItems)-1]
		w.ID = "gate:example__flow__2:G3"
		s.WaitItems = append(s.WaitItems, w)
	}
	r.flows[0].Gate = "G3"
	if _, err := r.preview(SendBack, "gate:example__flow__2:G3"); err == nil || !strings.Contains(err.Error(), "no send_back for gate G3") {
		t.Fatalf("%v", err)
	}
	if _, err := r.preview(Approve, "gate:example__flow__1:G4"); err == nil || !strings.Contains(err.Error(), "no approve for gate G4") {
		t.Fatalf("%v", err)
	}
	r2 := newRig(t)
	r2.flows[0].Dir = `C:\elsewhere\flows\x`
	if _, err := r2.preview(Approve, r2.gateTarget()); err == nil {
		t.Fatal("flow dir outside <root>\\flows accepted")
	}
	r3 := newRig(t)
	r3.flows[0].Dir = filepath.Join(r3.root, "flows", "..", "..", "x")
	if _, err := r3.preview(Approve, r3.gateTarget()); err == nil {
		t.Fatal("dotdot flow dir accepted")
	}
	r4 := newRig(t)
	r4.flows[0].GateToken = ""
	if _, err := r4.preview(Approve, r4.gateTarget()); err == nil {
		t.Fatal("empty gate token accepted")
	}
}

func TestGateExitCodesMapToClearResults(t *testing.T) {
	cases := map[int]string{GateExitChanged: "the gate changed or was already decided", GateExitVerdict: "the verdict is not valid for this gate"}
	for code, want := range cases {
		r := newRig(t)
		r.runner.code, r.runner.line = code, ""
		p, _ := r.preview(Approve, r.gateTarget())
		res, err := r.confirm(p.Token)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Failed || res.ExitCode != code || res.Refused != want {
			t.Fatalf("code %d: %+v", code, res)
		}
		if r.last()["exit_code"] != float64(code) || r.last()["refused"] != want {
			t.Fatalf("code %d audit: %v", code, r.last())
		}
	}
	r := newRig(t)
	p, _ := r.preview(Approve, r.gateTarget())
	res, _ := r.confirm(p.Token)
	if res.Failed || res.Refused != "" || res.ExitCode != 0 {
		t.Fatalf("exit 0 is decided: %+v", res)
	}
}

func TestAuditHoldsNoTokenOrOutput(t *testing.T) {
	r := newRig(t)
	r.runner.line = "SESSION-TEXT-SECRET-LINE"
	pg, _ := r.preview(Approve, r.gateTarget())
	r.confirm(pg.Token)
	ps, _ := r.preview(Stop, "ab12")
	r.confirm(ps.Token)
	b, _ := os.ReadFile(r.audit)
	s := string(b)
	for _, bad := range []string{pg.Token, ps.Token, "SESSION-TEXT-SECRET-LINE", `"t1"`, " t1"} {
		if strings.Contains(s, bad) {
			t.Fatalf("audit holds %q:\n%s", bad, s)
		}
	}
	for _, l := range r.auditLines() {
		for k := range l {
			switch k {
			case "time", "control", "target", "command", "exit_code", "refused", "phase":
			default:
				t.Fatalf("unexpected audit key %q", k)
			}
		}
	}
}

func TestSwitchOffMeansNotReadyAndRefused(t *testing.T) {
	r := newRig(t)
	p, _ := r.preview(Stop, "ab12")
	r.switches[Stop] = false
	a := r.svc.Availability()
	if a.Ready[Stop] || a.Why[Stop] != "switched off in the gear" || !a.Ready[Attach] {
		t.Fatalf("%+v", a)
	}
	if _, err := r.confirm(p.Token); err == nil || !strings.Contains(err.Error(), "switched off in the gear") {
		t.Fatalf("%v", err)
	}
	if r.runner.calls() != 0 || r.last()["refused"] == nil {
		t.Fatal("ran or not audited")
	}
	if _, err := r.preview(Stop, "ab12"); err == nil {
		t.Fatal("preview while off")
	}
}

func TestElevatedTurnsAllOff(t *testing.T) {
	r := newRig(t)
	p, _ := r.preview(Stop, "ab12")
	r.elevated = true
	a := r.svc.Availability()
	for _, c := range All {
		if a.Ready[c] || a.Why[c] != "BurnMon Dev runs as administrator: controls are off" {
			t.Fatalf("%s: %+v", c, a)
		}
		if _, err := r.preview(c, "ab12"); err == nil {
			t.Fatalf("%s preview while elevated", c)
		}
	}
	if _, err := r.confirm(p.Token); err == nil {
		t.Fatal("run while elevated")
	}
	if r.runner.calls() != 0 {
		t.Fatal("ran")
	}
}

func TestSelfErrorTurnsAllOff(t *testing.T) {
	r := newRig(t)
	p, _ := r.preview(Stop, "ab12")
	// a Self that looks harmless but came with an error must not be used
	r.self, r.selfErr = Self{PID: 1, ExeDir: `C:\example\burnmon`}, errors.New("cannot walk the process tree")
	a := r.svc.Availability()
	for _, c := range All {
		if a.Ready[c] || !strings.Contains(a.Why[c], "cannot walk the process tree") {
			t.Fatalf("%s: %+v", c, a)
		}
	}
	if _, err := r.preview(Stop, "ab12"); err == nil {
		t.Fatal("preview with a Self error")
	}
	if _, err := r.confirm(p.Token); err == nil || r.runner.calls() != 0 {
		t.Fatalf("run with a Self error: %v", err)
	}
}

func TestMissingDependenciesGiveReasons(t *testing.T) {
	r := newRig(t)
	r.env.ClaudePath, r.env.ClaudeWhy = "", ""
	r.env.PythonPath, r.env.PythonWhy = "", ""
	r.env.ForemanRoot = ""
	r.rebuild()
	a := r.svc.Availability()
	if a.Why[Stop] != "needs claude.exe: set claude_path in burnmon.json" || a.Why[Approve] != "needs python.exe on PATH" {
		t.Fatalf("%+v", a)
	}
	r.env.ClaudePath = svcClaude
	r.env.AgentViewOn = false
	r.env.PythonPath = svcPython
	r.rebuild()
	a = r.svc.Availability()
	if a.Why[Stop] != "needs agent_view on in burnmon.json" || a.Why[Approve] != "needs foreman_root in burnmon.json" {
		t.Fatalf("%+v", a)
	}
	r.env.ForemanRoot = t.TempDir() // no flows\_lib\flow_transition.py
	r.rebuild()
	if a = r.svc.Availability(); a.Ready[Approve] || a.Why[Approve] == "" {
		t.Fatalf("%+v", a)
	}
}

func TestNilEnvFunctionsFailClosed(t *testing.T) {
	r := newRig(t)
	for _, mut := range []func(*Env){
		func(e *Env) { e.Self = nil },
		func(e *Env) { e.Elevated = nil },
		func(e *Env) { e.Switches = nil },
		func(e *Env) { e.Runner = nil },
	} {
		e := *r.env
		mut(&e)
		a := NewService(e).Availability()
		for _, c := range All {
			if a.Ready[c] {
				t.Fatalf("%s ready with a nil dependency", c)
			}
		}
	}
}

func TestOneRunAtATime(t *testing.T) {
	r := newRig(t)
	r.runner.entered = make(chan struct{}, 4)
	r.runner.release = make(chan struct{})
	p1, _ := r.preview(Stop, "ab12")
	p2, _ := r.preview(Attach, "ab12")
	r.clock.t = r.clock.t.Add(time.Second)
	done := make(chan error, 1)
	go func() { _, err := r.svc.Run(p1.Token); done <- err }()
	<-r.runner.entered
	if _, err := r.svc.Run(p2.Token); err == nil || !strings.Contains(err.Error(), "another control is running") {
		t.Fatalf("%v", err)
	}
	close(r.runner.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	// the refused token never ran and stays usable
	if _, err := r.svc.Run(p2.Token); err != nil {
		t.Fatalf("second token after release: %v", err)
	}
	if len(r.runner.runs) != 1 || len(r.runner.starts) != 1 {
		t.Fatalf("%v %v", r.runner.runs, r.runner.starts)
	}
}

func TestConcurrentConfirmsOfOneTokenRunOnce(t *testing.T) {
	r := newRig(t)
	p, _ := r.preview(Stop, "ab12")
	r.clock.t = r.clock.t.Add(time.Second)
	var ok int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := r.svc.Run(p.Token); err == nil {
				atomic.AddInt32(&ok, 1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if ok != 1 || len(r.runner.runs) != 1 {
		t.Fatalf("ok=%d runs=%d", ok, len(r.runner.runs))
	}
}

func TestSecondPressCarriesOnlyTheToken(t *testing.T) {
	// Run takes a string only: a page cannot hand over an argv, id or path.
	var f func(*Service, string) (RunResult, error) = (*Service).Run
	_ = f
	r := newRig(t)
	for _, tok := range []string{"ab12", "stop ab12", `C:\example\claude.exe stop ab12`, ""} {
		if _, err := r.svc.Run(tok); !errors.Is(err, ErrNoPreview) {
			t.Fatalf("%q: %v", tok, err)
		}
	}
	if r.runner.calls() != 0 {
		t.Fatal("ran")
	}
}

func TestTokenForAIsNeverRunForB(t *testing.T) {
	r := newRig(t)
	r.snap = func(s *snapshot.Snapshot) {
		for i := range s.AgentSessions {
			if s.AgentSessions[i].ID == "cd34" {
				s.AgentSessions[i].PID = 4343
			}
		}
	}
	r.agent = `[{"kind":"background","id":"ab12","cwd":"C:\\example\\a","state":"blocked","pid":4242,"startedAt":1761213600000},` +
		`{"kind":"background","id":"cd34","cwd":"C:\\example\\b","state":"blocked","pid":4343,"startedAt":1761213600000}]`
	pa, err := r.preview(Stop, "ab12")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.preview(Stop, "cd34"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.confirm(pa.Token); err != nil {
		t.Fatal(err)
	}
	if len(r.runner.runs) != 1 || r.runner.runs[0][2] != "ab12" {
		t.Fatalf("%v", r.runner.runs)
	}
}

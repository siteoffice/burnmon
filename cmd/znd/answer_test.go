package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"burnmon/internal/controls"
	"burnmon/internal/controlsenv"
	"burnmon/internal/graph"
	"burnmon/internal/snapshot"
)

type fakeRun struct {
	mu   sync.Mutex
	runs [][]string
	code int
	line string
}

func (f *fakeRun) Run(ctx context.Context, argv []string, dir string) (int, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs = append(f.runs, append([]string(nil), argv...))
	return f.code, f.line, nil
}

func (f *fakeRun) Start(argv []string) error { return errors.New("znd never starts a console") }

type answerRig struct {
	t      *testing.T
	runner *fakeRun
	audit  string
	root   string
	d      deps
	zerr   error
	now    time.Time
}

func newAnswerRig(t *testing.T) *answerRig {
	t.Helper()
	root := t.TempDir()
	lib := filepath.Join(root, "flows", "_lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "flow_transition.py"), []byte("# fake\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &answerRig{now: time.Date(2026, 10, 23, 10, 0, 10, 0, time.UTC), t: t, runner: &fakeRun{line: "ok"}, root: root, audit: filepath.Join(t.TempDir(), "controls-audit.ndjson")}
	flows := []graph.Flow{{ID: "example__flow__2", Slug: "flow-review", Gate: "G2", GatePending: true, GateToken: "tok-g2-one",
		Dir: filepath.Join(root, "flows", "example", "flow-review")}}
	py := "/example/python3"
	if filepath.Separator == '\\' {
		py = `C:\example\python.exe`
	}
	r.d = deps{
		wait: func(d time.Duration) { r.now = r.now.Add(d) },
		zndConfig: func() (controlsenv.ZndConfig, error) { return controlsenv.ZndConfig{GateWithoutSnapshot: controls.GateRefuse}, r.zerr },
		env: func(z controlsenv.ZndConfig) controls.Env {
			return controls.Env{
				Now:         func() time.Time { return r.now },
				PythonPath:  py,
				ForemanRoot: root,
				ReadForeman: func(string) ([]graph.Sprint, []graph.Flow, error) { return nil, flows, nil },
				Snapshot: func() snapshot.Reading {
					rd := snapshot.Load("../../testdata/snapshot_example.json", time.Date(2026, 10, 23, 10, 0, 10, 0, time.UTC))
					if rd.Snapshot != nil {
						for _, it := range rd.Snapshot.WaitItems {
							if it.ID == "gate:example__flow__2:G2" {
								it.ID = "gate:example__flow__2:G4"
								rd.Snapshot.WaitItems = append(rd.Snapshot.WaitItems, it)
								break
							}
						}
					}
					return rd
				},
				Switches:            func() map[string]bool { return map[string]bool{controls.Approve: true, controls.SendBack: true} },
				Self:                func() (controls.Self, error) { return controls.Self{PID: 1}, nil },
				Elevated:            func() bool { return false },
				Runner:              r.runner,
				AuditPath:           r.audit,
				GateWithoutSnapshot: z.GateWithoutSnapshot,
			}
		},
	}
	return r
}

func (r *answerRig) run(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb, r.d)
	return code, out.String(), errb.String()
}

const gateID = "gate:example__flow__2:G2"

// answer_requires_flag
func TestAnswerRequiresFlag(t *testing.T) {
	r := newAnswerRig(t)
	code, _, errs := r.run("answer", gateID, "approve")
	if code != 2 || len(r.runner.runs) != 0 || errs == "" {
		t.Errorf("code %d runs %d err %q", code, len(r.runner.runs), errs)
	}
	if _, err := os.Stat(r.audit); err == nil {
		t.Error("audit file written")
	}
	if code, _, _ := r.run("answer", gateID, "approve", "--dry-run", "--yes"); code != 2 {
		t.Errorf("both flags: code %d", code)
	}
	if code, _, _ := r.run("answer", gateID, "maybe", "--dry-run"); code != 2 {
		t.Errorf("bad verb: code %d", code)
	}
	if code, _, _ := r.run(); code != 2 {
		t.Errorf("no command: code %d", code)
	}
}

// answer_dry_run_writes_nothing
func TestAnswerDryRunWritesNothing(t *testing.T) {
	r := newAnswerRig(t)
	code, out, errs := r.run("answer", gateID, "approve", "--dry-run")
	if code != 0 || !strings.Contains(out, "flow_transition") && !strings.Contains(out, "approve") {
		t.Fatalf("code %d out %q err %q", code, out, errs)
	}
	if strings.Contains(out, "tok-g2-one") {
		t.Error("dry run prints the gate token")
	}
	if len(r.runner.runs) != 0 {
		t.Error("dry run ran something")
	}
	if _, err := os.Stat(r.audit); err == nil {
		t.Error("dry run wrote the audit file")
	}
}

// answer_yes_one_process
func TestAnswerYesOneProcess(t *testing.T) {
	r := newAnswerRig(t)
	code, out, errs := r.run("answer", gateID, "approve", "--yes")
	if code != 0 {
		t.Fatalf("code %d out %q err %q", code, out, errs)
	}
	if len(r.runner.runs) != 1 {
		t.Fatalf("runs = %v", r.runner.runs)
	}
	if strings.Contains(out, "tok-g2-one") || strings.Contains(errs, "tok-g2-one") {
		t.Error("output holds the gate token")
	}
	b, _ := os.ReadFile(r.audit)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"phase":"start"`) || strings.Contains(string(b), "tok-g2-one") {
		t.Errorf("audit = %s", b)
	}
}

// answer_spec_only
func TestAnswerSpecOnly(t *testing.T) {
	r := newAnswerRig(t)
	code, _, errs := r.run("answer", "gate:example__flow__2:G4", "approve", "--yes")
	if code != 5 || !strings.Contains(errs, "no approve for gate G4") || len(r.runner.runs) != 0 {
		t.Errorf("code %d err %q runs %d", code, errs, len(r.runner.runs))
	}
}

// answer_exit_codes
func TestAnswerExitCodes(t *testing.T) {
	for code, want := range map[int]string{controls.GateExitChanged: "the gate changed or was already decided", controls.GateExitVerdict: "the verdict is not valid for this gate"} {
		r := newAnswerRig(t)
		r.runner.code = code
		got, _, errs := r.run("answer", gateID, "approve", "--yes")
		if got != code || !strings.Contains(errs, want) {
			t.Errorf("runner %d: exit %d err %q", code, got, errs)
		}
	}
}

// zndconfig_bad_value, as seen by answer: exit 2 and no audit line.
func TestAnswerBadZndConfig(t *testing.T) {
	r := newAnswerRig(t)
	r.zerr = errors.New(`znd.json: gate_without_snapshot "maybe" is not refuse or foreman_read`)
	code, _, errs := r.run("answer", gateID, "approve", "--yes")
	if code != 2 || !strings.Contains(errs, "maybe") || len(r.runner.runs) != 0 {
		t.Errorf("code %d err %q", code, errs)
	}
	if _, err := os.Stat(r.audit); err == nil {
		t.Error("audit written")
	}
}

// A refusal (stale snapshot, setting refuse) exits 5 with the reason.
func TestAnswerRefusalExits5(t *testing.T) {
	r := newAnswerRig(t)
	orig := r.d.env
	r.d.env = func(z controlsenv.ZndConfig) controls.Env {
		e := orig(z)
		e.Snapshot = func() snapshot.Reading { return snapshot.Reading{Status: snapshot.StatusMissing} }
		return e
	}
	code, _, errs := r.run("answer", gateID, "approve", "--dry-run")
	if code != 5 || !strings.Contains(errs, "the snapshot is not current") {
		t.Errorf("code %d err %q", code, errs)
	}
}

//go:build windows || darwin

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"burnmon/internal/controls"
	"burnmon/internal/graph"
	"burnmon/internal/snapshot"
)

// exampleSnap is the shared example snapshot; rigT0 is a moment it is current.
const exampleSnap = "../../testdata/snapshot_example.json"

var rigT0 = time.Date(2026, 10, 23, 10, 0, 10, 0, time.UTC)

// fakeRunner is a controls.Runner that records what it was asked to run and
// runs nothing.
type fakeRunner struct {
	mu     sync.Mutex
	runs   [][]string
	starts [][]string
	code   int
	line   string
}

func (f *fakeRunner) Run(ctx context.Context, argv []string, dir string) (int, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs = append(f.runs, append([]string(nil), argv...))
	return f.code, f.line, nil
}

func (f *fakeRunner) Start(argv []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts = append(f.starts, append([]string(nil), argv...))
	return nil
}

func (f *fakeRunner) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.runs) + len(f.starts)
}

// fakeClock is a clock a test moves by hand.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// decisionRig is a controls service over a fake runner, a fake clock, a
// temp Foreman root and the example snapshot, plus the bindEnv the
// bindings get.
type decisionRig struct {
	t      *testing.T
	clock  *fakeClock
	runner *fakeRunner
	env    controls.Env
	svc    *controls.Service
	be     bindEnv
	audit  string
	root   string
	flows  []graph.Flow
}

func newDecisionRig(t *testing.T) *decisionRig {
	t.Helper()
	root := t.TempDir()
	lib := filepath.Join(root, "flows", "_lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "flow_transition.py"), []byte("# fake\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &decisionRig{t: t, clock: &fakeClock{t: rigT0}, runner: &fakeRunner{line: "gate decided"}, root: root,
		audit: filepath.Join(t.TempDir(), "controls-audit.ndjson"),
		flows: []graph.Flow{{ID: "example__flow__2", Slug: "flow-review", Gate: "G2", GatePending: true, GateToken: "t1",
			Dir: filepath.Join(root, "flows", "example", "flow-review")}},
	}
	r.env = controls.Env{
		Now:         r.clock.now,
		ClaudePath:  `C:\example\claude.exe`,
		PythonPath:  `C:\example\python.exe`,
		AgentViewOn: true,
		ForemanRoot: root,
		ReadForeman: func(string) ([]graph.Sprint, []graph.Flow, error) {
			return nil, append([]graph.Flow(nil), r.flows...), nil
		},
		Snapshot: func() snapshot.Reading { return snapshot.Load(exampleSnap, r.clock.now()) },
		Switches: func() map[string]bool {
			m := map[string]bool{}
			for _, c := range controls.All {
				m[c] = true
			}
			return m
		},
		Self:      func() (controls.Self, error) { return controls.Self{PID: 1, ExeDir: `C:\example\cc`}, nil },
		Elevated:  func() bool { return false },
		Runner:    r.runner,
		AuditPath: r.audit,
	}
	r.svc = controls.NewService(r.env)
	r.be = bindEnv{root: t.TempDir(), dataDir: t.TempDir(), snapPath: exampleSnap, auditPath: r.audit,
		now: r.clock.now, svc: r.svc}
	return r
}

// auditLines reads the audit file, one map per line; nil when it does not exist.
func (r *decisionRig) auditLines() []map[string]any {
	r.t.Helper()
	b, err := os.ReadFile(r.audit)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		r.t.Fatal(err)
	}
	var out []map[string]any
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			r.t.Fatalf("audit line %q: %v", sc.Text(), err)
		}
		out = append(out, m)
	}
	return out
}

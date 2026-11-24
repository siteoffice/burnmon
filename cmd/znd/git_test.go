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
)

// seqRun records every argv and fails the call numbers in codes.
type seqRun struct {
	mu    sync.Mutex
	runs  [][]string
	dirs  []string
	codes map[int]int
}

func (s *seqRun) Run(ctx context.Context, argv []string, dir string) (int, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs = append(s.runs, append([]string(nil), argv...))
	s.dirs = append(s.dirs, dir)
	return s.codes[len(s.runs)], "line", nil
}

func (s *seqRun) Start(argv []string) error { return errors.New("znd never starts a console") }

type gitRig struct {
	t      *testing.T
	runner *seqRun
	audit  string
	cwd    string
	git    string
	engine string
	d      deps
}

func newGitRig(t *testing.T) *gitRig {
	t.Helper()
	r := &gitRig{t: t, runner: &seqRun{codes: map[int]int{}}, cwd: t.TempDir()}
	bin := t.TempDir()
	r.git = filepath.Join(bin, "git")
	r.audit = filepath.Join(t.TempDir(), "controls-audit.ndjson")
	r.engine = filepath.Join(bin, "git_flow.py")
	r.d = deps{git: func() gitEnv {
		return gitEnv{Git: r.git, Python: filepath.Join(bin, "python3"), Engine: r.engine, AuditPath: r.audit,
			Runner: r.runner, Now: func() time.Time { return time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC) }}
	}}
	return r
}

func (r *gitRig) run(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(append([]string{"git"}, args...), &out, &errb, r.d)
	return code, out.String(), errb.String()
}

func (r *gitRig) auditLines() []string {
	b, err := os.ReadFile(r.audit)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

func TestGitRequiresFlag(t *testing.T) {
	r := newGitRig(t)
	for _, args := range [][]string{{"sync", "--cwd", r.cwd}, {"sync", "--cwd", r.cwd, "--dry-run", "--yes"}} {
		code, _, _ := r.run(args...)
		if code != exitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, exitUsage)
		}
	}
	if len(r.runner.runs) != 0 || r.auditLines() != nil {
		t.Error("something ran or was audited without exactly one mode flag")
	}
}

func TestGitDryRunWritesNothing(t *testing.T) {
	r := newGitRig(t)
	code, out, errs := r.run("sync", "--cwd", r.cwd, "--dry-run")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errs)
	}
	if !strings.Contains(out, "# recipe: git sync") || !strings.Contains(out, "git fetch --prune origin") {
		t.Errorf("block not printed: %q", out)
	}
	if len(r.runner.runs) != 0 || r.auditLines() != nil {
		t.Error("dry run ran or wrote an audit line")
	}
}

func TestGitYesRunsAndAudits(t *testing.T) {
	r := newGitRig(t)
	code, out, errs := r.run("sync", "--cwd", r.cwd, "--yes")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errs)
	}
	if !strings.Contains(out, "# recipe: git sync") {
		t.Errorf("block not printed: %q", out)
	}
	if len(r.runner.runs) != 3 {
		t.Fatalf("ran %d lines, want 3", len(r.runner.runs))
	}
	for _, d := range r.runner.dirs {
		if d != r.cwd {
			t.Errorf("ran in %q, want %q", d, r.cwd)
		}
	}
	lines := r.auditLines()
	if len(lines) != 2 || !strings.Contains(lines[0], `"phase":"start"`) || !strings.Contains(lines[0], `"control":"git_sync"`) ||
		!strings.Contains(lines[1], `"control":"git_sync"`) || strings.Contains(lines[1], `"phase"`) {
		t.Errorf("audit lines: %v", lines)
	}
}

func TestGitYesStopsOnFailure(t *testing.T) {
	r := newGitRig(t)
	r.runner.codes[2] = 1
	code, _, errs := r.run("sync", "--cwd", r.cwd, "--yes")
	if code != exitFailed {
		t.Errorf("exit %d, want %d (%s)", code, exitFailed, errs)
	}
	if len(r.runner.runs) != 2 {
		t.Errorf("ran %d lines, want 2: line 3 must not run", len(r.runner.runs))
	}
	lines := r.auditLines()
	if len(lines) != 2 || !strings.Contains(lines[1], `"exit_code":1`) || !strings.Contains(lines[1], `"refused":"`) {
		t.Errorf("result line lacks the exit code and the reason: %v", lines)
	}
}

func TestGitNoShell(t *testing.T) {
	r := newGitRig(t)
	if code, _, errs := r.run("save", "--cwd", r.cwd, "--files", "a.txt", "--message", "msg", "--yes"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errs)
	}
	if code, _, errs := r.run("land", "--cwd", r.cwd, "--yes"); code != exitOK {
		t.Fatalf("land exit %d: %s", code, errs)
	}
	for _, argv := range r.runner.runs {
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(argv[0]), ".exe"))
		if argv[0] != r.git && base != "python3" {
			t.Errorf("argv starts with %q", argv[0])
		}
		for _, bad := range []string{"cmd", "pwsh", "powershell", "sh", "bash", "zsh"} {
			if base == bad {
				t.Errorf("argv runs a shell: %v", argv)
			}
		}
	}
}

func TestGitAuditUnwritableRunsNothing(t *testing.T) {
	r := newGitRig(t)
	r.audit = filepath.Join(t.TempDir(), "no", "such", "dir", "controls-audit.ndjson")
	code, _, _ := r.run("sync", "--cwd", r.cwd, "--yes")
	if code != exitRefused {
		t.Errorf("exit %d, want %d", code, exitRefused)
	}
	if len(r.runner.runs) != 0 {
		t.Error("a line ran although the audit line could not be written")
	}
}

func TestGitRefusals(t *testing.T) {
	r := newGitRig(t)
	cases := [][]string{
		{"save", "--cwd", r.cwd, "--files", ".", "--message", "m", "--yes"},
		{"save", "--cwd", r.cwd, "--files", "a", "--message", "done.", "--yes"},
		{"nonsense", "--cwd", r.cwd, "--yes"},
		{"sync", "--yes"},
		{"sync", "--cwd", r.cwd, "--bogus", "--yes"},
	}
	for _, args := range cases {
		if code, _, _ := r.run(args...); code != exitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, exitUsage)
		}
	}
	if len(r.runner.runs) != 0 || r.auditLines() != nil {
		t.Error("a refused call ran or was audited")
	}
	r.engine = ""
	if code, _, errs := r.run("land", "--cwd", r.cwd, "--yes"); code != exitUsage || !strings.Contains(errs, "groundwork_engine") {
		t.Errorf("land without engine: exit %d, %q", code, errs)
	}
}

func TestGitFlagValues(t *testing.T) {
	r := newGitRig(t)
	code, _, errs := r.run("save", "--cwd", r.cwd, "--files", "a b.txt", "c.go", "--message", "fix it",
		"--body", "why", "--co-author", "A B <a@b.c>", "--yes")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errs)
	}
	if got := strings.Join(r.runner.runs[0][1:], "|"); got != "add|--|a b.txt|c.go" {
		t.Errorf("add argv %s", got)
	}
	if got := strings.Join(r.runner.runs[1][1:], "|"); got != "commit|-m|fix it|-m|why|-m|Co-Authored-By: A B <a@b.c>" {
		t.Errorf("commit argv %s", got)
	}
}

// TestGitDryRunShowsRunLines: the block copies snip.py, so the exact argv that
// --yes runs is printed after it, one line per step.
func TestGitDryRunShowsRunLines(t *testing.T) {
	r := newGitRig(t)
	code, out, errs := r.run("sync", "--cwd", r.cwd, "--dry-run")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errs)
	}
	i := strings.LastIndex(out, "```")
	if i < 0 {
		t.Fatalf("no block end: %q", out)
	}
	tail := out[i:]
	for n, want := range []string{"# runs 1 of 3: ", "# runs 2 of 3: ", "# runs 3 of 3: "} {
		if !strings.Contains(tail, want) {
			t.Errorf("line %d missing after the block: %q", n+1, tail)
		}
	}
	if !strings.Contains(tail, "merge --ff-only @{u}") {
		t.Errorf("argv text missing: %q", tail)
	}
}

package controls

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"burnmon/internal/graph"
	"burnmon/internal/snapshot"
	"burnmon/internal/waits"
)

const (
	closeSprintID = "example__sprint__9"
	closeItemID   = "closeout:example__sprint__9"
	closeBranch   = "sprint/example-merged"
	closeWT       = `C:\example\repo\.worktrees\wt`
)

// closeRig is a rig whose Foreman read and merge check the test can change.
type closeRig struct {
	*rig
	cmu     sync.Mutex
	sprints []graph.Sprint
	merged  bool
	known   bool
}

func (c *closeRig) goodSprint() graph.Sprint {
	return graph.Sprint{ID: closeSprintID, Slug: "example-merged", Status: "waiting-on-you", Phase: "report",
		Dir:          filepath.Join(c.root, "sprints", "example", "example-merged"),
		Branch:       closeBranch,
		WorktreePath: closeWT}
}

func newCloseRig(t *testing.T) *closeRig {
	t.Helper()
	r := newRig(t)
	lib := filepath.Join(r.root, "sprints", "_lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "sprint_step.py"), []byte("# fake\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &closeRig{rig: r, merged: true, known: true}
	c.sprints = []graph.Sprint{c.goodSprint()}
	r.switches[CloseSprint] = true
	r.snap = func(s *snapshot.Snapshot) {
		s.WaitItems = append(s.WaitItems, waits.Item{Kind: waits.KindCloseOut, ID: closeItemID,
			Title: "example-merged", Group: "done", SprintID: closeSprintID})
	}
	r.env.ReadForeman = func(string) ([]graph.Sprint, []graph.Flow, error) {
		c.cmu.Lock()
		defer c.cmu.Unlock()
		return append([]graph.Sprint(nil), c.sprints...), nil, nil
	}
	r.env.Merged = func(repo, branch string) (bool, bool) {
		c.cmu.Lock()
		defer c.cmu.Unlock()
		return c.merged, c.known
	}
	r.rebuild()
	return c
}

func (c *closeRig) set(f func(s *graph.Sprint)) {
	c.cmu.Lock()
	defer c.cmu.Unlock()
	f(&c.sprints[0])
}

func (c *closeRig) setMerged(merged, known bool) {
	c.cmu.Lock()
	defer c.cmu.Unlock()
	c.merged, c.known = merged, known
}

func TestCloseArgv(t *testing.T) {
	got := CloseArgv(`C:\py\python.exe`, `C:\example\root`, `C:\example\root\sprints\example\example-merged`)
	want := []string{`C:\py\python.exe`, "-E", "-s", `C:\example\root\sprints\_lib\sprint_step.py`,
		`C:\example\root\sprints\example\example-merged`, "--phase", "done", "--action", "sprint_done",
		"--actor", "burnmon", "--detail", "branch in main, closed from BurnMon Dev", "--status", "done"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CloseArgv = %q", got)
	}
	if SprintScript(`C:\example\root`) != `C:\example\root\sprints\_lib\sprint_step.py` {
		t.Errorf("SprintScript = %q", SprintScript(`C:\example\root`))
	}
}

func TestCloseUnavailable(t *testing.T) {
	why := func(mut func(c *closeRig)) string {
		c := newCloseRig(t)
		mut(c)
		c.rebuild()
		a := c.svc.Availability()
		if a.Ready[CloseSprint] {
			t.Fatal("close_sprint ready")
		}
		return a.Why[CloseSprint]
	}
	if got := why(func(c *closeRig) { c.env.ForemanRoot = "" }); got != "needs foreman_root in burnmon.json" {
		t.Errorf("no root: %q", got)
	}
	if got := why(func(c *closeRig) { c.env.PythonPath = "" }); got != "needs python.exe on PATH" {
		t.Errorf("no python: %q", got)
	}
	script := func(c *closeRig) string { return filepath.Join(c.root, "sprints", "_lib", "sprint_step.py") }
	if got := why(func(c *closeRig) { _ = os.Rename(script(c), script(c)+".gone") }); got != `needs sprints\_lib\sprint_step.py under foreman_root` {
		t.Errorf("no script: %q", got)
	}
	if got := why(func(c *closeRig) {
		_ = os.Rename(script(c), script(c)+".gone")
		_ = os.MkdirAll(script(c), 0o755)
	}); got != `needs sprints\_lib\sprint_step.py under foreman_root` {
		t.Errorf("script is a folder: %q", got)
	}
	if got := why(func(c *closeRig) { c.switches[CloseSprint] = false }); got != "switched off in the gear" {
		t.Errorf("switch off: %q", got)
	}
}

func TestClosePreviewShowsExactCommand(t *testing.T) {
	c := newCloseRig(t)
	pv, err := c.preview(CloseSprint, closeItemID)
	if err != nil {
		t.Fatal(err)
	}
	want := CommandLine(CloseArgv(svcPython, c.root, c.goodSprint().Dir))
	if pv.Command != want {
		t.Errorf("Command = %q, want %q", pv.Command, want)
	}
	if pv.Label != "example-merged" || pv.ExpiresInMs != 5000 || pv.Token == "" {
		t.Errorf("preview = %+v", pv)
	}
	if c.runner.calls() != 0 {
		t.Error("preview ran something")
	}
}

func TestCloseRefusesBadSprint(t *testing.T) {
	cases := map[string]func(c *closeRig){
		"dotdot dir":   func(c *closeRig) { c.set(func(s *graph.Sprint) { s.Dir = `..\..\x` }) },
		"relative dir": func(c *closeRig) { c.set(func(s *graph.Sprint) { s.Dir = `sprints\example\x` }) },
		"outside dir":  func(c *closeRig) { c.set(func(s *graph.Sprint) { s.Dir = `C:\example\elsewhere\x` }) },
		"unclean dir": func(c *closeRig) {
			c.set(func(s *graph.Sprint) { s.Dir = filepath.Join(c.root, "sprints", "a") + `\..\b` })
		},
		"sprints itself":   func(c *closeRig) { c.set(func(s *graph.Sprint) { s.Dir = filepath.Join(c.root, "sprints") }) },
		"branch dash":      func(c *closeRig) { c.set(func(s *graph.Sprint) { s.Branch = "-x" }) },
		"branch option":    func(c *closeRig) { c.set(func(s *graph.Sprint) { s.Branch = "--status=running" }) },
		"branch space":     func(c *closeRig) { c.set(func(s *graph.Sprint) { s.Branch = "a b" }) },
		"branch ampersand": func(c *closeRig) { c.set(func(s *graph.Sprint) { s.Branch = "a&b" }) },
		"branch quote":     func(c *closeRig) { c.set(func(s *graph.Sprint) { s.Branch = `"` }) },
		"branch empty":     func(c *closeRig) { c.set(func(s *graph.Sprint) { s.Branch = "" }) },
		"no worktree":      func(c *closeRig) { c.set(func(s *graph.Sprint) { s.WorktreePath = "" }) },
		"listed twice": func(c *closeRig) {
			c.cmu.Lock()
			c.sprints = append(c.sprints, c.sprints[0])
			c.cmu.Unlock()
		},
		"not listed": func(c *closeRig) {
			c.cmu.Lock()
			c.sprints = nil
			c.cmu.Unlock()
		},
		"status running":          func(c *closeRig) { c.set(func(s *graph.Sprint) { s.Status = "running" }) },
		"phase done":              func(c *closeRig) { c.set(func(s *graph.Sprint) { s.Phase = "done" }) },
		"not merged":              func(c *closeRig) { c.setMerged(false, true) },
		"merge unknown":           func(c *closeRig) { c.setMerged(false, false) },
		"merged but unknown flag": func(c *closeRig) { c.setMerged(true, false) },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			c := newCloseRig(t)
			mut(c)
			pv, err := c.preview(CloseSprint, closeItemID)
			if err == nil || err.Error() == "" {
				t.Fatalf("preview accepted: %+v", pv)
			}
			if pv.Token != "" {
				t.Error("a token was kept")
			}
			if c.runner.calls() != 0 {
				t.Error("the runner was called")
			}
			if _, err := c.confirm("anything"); err == nil {
				t.Error("run accepted")
			}
			if c.runner.calls() != 0 {
				t.Error("the runner was called")
			}
		})
	}
}

func TestCloseRefusesUnlistedItem(t *testing.T) {
	c := newCloseRig(t)
	if _, err := c.preview(CloseSprint, "closeout:other"); err == nil {
		t.Error("an item that is not in the snapshot was accepted")
	}
	if _, err := c.preview(CloseSprint, "gate:example__flow__2:G2"); err == nil {
		t.Error("a gate id was accepted")
	}
}

func TestCloseRunRefusesChangedSprint(t *testing.T) {
	cases := map[string]func(c *closeRig){
		"status done": func(c *closeRig) { c.set(func(s *graph.Sprint) { s.Status = "done" }) },
		"phase done":  func(c *closeRig) { c.set(func(s *graph.Sprint) { s.Phase = "done" }) },
		"dir moved": func(c *closeRig) {
			c.set(func(s *graph.Sprint) { s.Dir = filepath.Join(c.root, "sprints", "example", "other") })
		},
		"branch changed": func(c *closeRig) { c.set(func(s *graph.Sprint) { s.Branch = "sprint/example-other" }) },
		"merge turned":   func(c *closeRig) { c.setMerged(false, true) },
		"merge unknown":  func(c *closeRig) { c.setMerged(false, false) },
		"sprint gone": func(c *closeRig) {
			c.cmu.Lock()
			c.sprints = nil
			c.cmu.Unlock()
		},
		"switch off": func(c *closeRig) { c.mu.Lock(); c.switches[CloseSprint] = false; c.mu.Unlock() },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			c := newCloseRig(t)
			pv, err := c.preview(CloseSprint, closeItemID)
			if err != nil {
				t.Fatal(err)
			}
			mut(c)
			res, err := c.confirm(pv.Token)
			if err == nil || res.Refused == "" || res.ExitCode != -1 {
				t.Fatalf("run = %+v, %v", res, err)
			}
			if c.runner.calls() != 0 {
				t.Fatal("sprint_step.py would have started")
			}
			lines := c.auditLines()
			if len(lines) != 1 || lines[0]["refused"] == nil || lines[0]["exit_code"].(float64) != -1 {
				t.Fatalf("audit = %v", lines)
			}
			if lines[0]["control"] != CloseSprint || lines[0]["target"] != "sprint:"+closeSprintID {
				t.Errorf("audit line = %v", lines[0])
			}
		})
	}
}

func TestCloseRunWritesTwoAuditLines(t *testing.T) {
	c := newCloseRig(t)
	c.runner.code, c.runner.line = 0, ""
	pv, err := c.preview(CloseSprint, closeItemID)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.confirm(pv.Token)
	if err != nil || res.Failed || res.Refused != "" {
		t.Fatalf("run = %+v, %v", res, err)
	}
	if len(c.runner.runs) != 1 || !reflect.DeepEqual(c.runner.runs[0], CloseArgv(svcPython, c.root, c.goodSprint().Dir)) {
		t.Fatalf("runs = %q", c.runner.runs)
	}
	if c.runner.dirs[0] != filepath.Join(c.root, "sprints", "_lib") {
		t.Errorf("dir = %q", c.runner.dirs[0])
	}
	lines := c.auditLines()
	if len(lines) != 2 {
		t.Fatalf("audit = %v", lines)
	}
	if lines[0]["phase"] != "start" || lines[1]["phase"] != nil {
		t.Errorf("phases = %v, %v", lines[0]["phase"], lines[1]["phase"])
	}
	for _, l := range lines {
		if l["control"] != CloseSprint || l["target"] != "sprint:"+closeSprintID || l["command"] != pv.Command {
			t.Errorf("audit line = %v", l)
		}
	}
}

// deadlineRunner records whether the run context carried a 30 s deadline.
type deadlineRunner struct {
	fakeRunner
	left time.Duration
	ok   bool
}

func (d *deadlineRunner) Run(ctx context.Context, argv []string, dir string) (int, string, error) {
	dl, ok := ctx.Deadline()
	d.ok, d.left = ok, time.Until(dl)
	return d.fakeRunner.Run(ctx, argv, dir)
}

func TestCloseRunHasThirtySecondDeadline(t *testing.T) {
	c := newCloseRig(t)
	dr := &deadlineRunner{}
	c.env.Runner = dr
	c.rebuild()
	pv, err := c.preview(CloseSprint, closeItemID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.confirm(pv.Token); err != nil {
		t.Fatal(err)
	}
	if !dr.ok || dr.left > 30*time.Second || dr.left < 25*time.Second {
		t.Errorf("deadline ok=%v left=%v", dr.ok, dr.left)
	}
}

func TestCloseExitOneIsARefusal(t *testing.T) {
	c := newCloseRig(t)
	c.runner.code, c.runner.line = 1, ""
	pv, err := c.preview(CloseSprint, closeItemID)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.confirm(pv.Token)
	if err != nil {
		t.Fatal(err)
	}
	const want = "Foreman stopped with exit 1: the sprint may or may not be closed, check the sprint's dashboard or report"
	if strings.Contains(want, "refused") || strings.Contains(want, "close-out block") {
		t.Fatal("the exit 1 text claims a reason Foreman did not give")
	}
	if res.Refused != want || !res.Failed || res.ExitCode != 1 {
		t.Fatalf("result = %+v", res)
	}
	lines := c.auditLines()
	if len(lines) != 2 || lines[1]["exit_code"].(float64) != 1 || lines[1]["refused"] != want {
		t.Fatalf("audit = %v", lines)
	}
}

func TestCloseFailsClosedWithoutAudit(t *testing.T) {
	c := newCloseRig(t)
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.env.AuditPath = filepath.Join(blocker, "controls-audit.ndjson")
	c.rebuild()
	pv, err := c.preview(CloseSprint, closeItemID)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := c.confirm(pv.Token); err == nil || res.Refused == "" {
		t.Fatalf("run = %+v, %v", res, err)
	}
	if c.runner.calls() != 0 {
		t.Fatal("ran without an audit file")
	}
}

func TestCloseNeverWritesFiles(t *testing.T) {
	b, err := os.ReadFile("close.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"os.WriteFile", "os.OpenFile", "os.Create", "os.Rename", "os.Remove"} {
		if strings.Contains(string(b), bad) {
			t.Errorf("close.go uses %s", bad)
		}
	}
}

func TestCloseWithFakeForemanLibrary(t *testing.T) {
	py, err := ResolvePython(exec.LookPath, runtime.GOOS)
	if err != nil {
		t.Skip("no usable python: " + err.Error())
	}
	root := t.TempDir()
	lib := filepath.Join(root, "sprints", "_lib")
	sprintDir := filepath.Join(root, "sprints", "example", "example-merged")
	for _, d := range []string{lib, sprintDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	step := "import sys, json, close_out\n" +
		"open(sys.argv[1] + '/called.json', 'w').write(json.dumps(sys.argv[1:]))\n" +
		"sys.exit(0)\n"
	if err := os.WriteFile(filepath.Join(lib, "sprint_step.py"), []byte(step), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "close_out.py"), []byte("# sibling\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	evil := t.TempDir()
	if err := os.WriteFile(filepath.Join(evil, "close_out.py"),
		[]byte("open(r'"+filepath.Join(evil, "pwned.txt")+"', 'w').write('x')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PYTHONPATH", evil)

	argv := CloseArgv(py, root, sprintDir)
	code, line, err := ExecRunner{}.Run(context.Background(), argv, lib)
	if err != nil || code != 0 {
		t.Fatalf("run = %d, %q, %v", code, line, err)
	}
	b, err := os.ReadFile(filepath.Join(sprintDir, "called.json"))
	if err != nil {
		t.Fatalf("script did not run: %v", err)
	}
	var got []string
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, argv[4:]) {
		t.Errorf("script got %q, want %q", got, argv[4:])
	}
	if _, err := os.Stat(filepath.Join(evil, "pwned.txt")); err == nil {
		t.Error("PYTHONPATH was honoured: pwned.txt written")
	}
}

func closeExitOne(t *testing.T, line string) (RunResult, []map[string]any) {
	t.Helper()
	c := newCloseRig(t)
	c.runner.code, c.runner.line = 1, line
	pv, err := c.preview(CloseSprint, closeItemID)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.confirm(pv.Token)
	if err != nil {
		t.Fatal(err)
	}
	return res, c.auditLines()
}

func TestCloseExitOneShowsLastStderrLine(t *testing.T) {
	const last = "ValueError: an Unattended sprint with parked lines ends waiting-on-you, not done"
	trace := "Traceback (most recent call last):\n  File \"sprint_step.py\", line 9, in <module>\n    run()\n" + last + "\n\n"
	res, lines := closeExitOne(t, trace)
	want := closeRefused + ": " + last
	if res.Refused != want || !res.Failed || res.ExitCode != 1 {
		t.Fatalf("result = %+v", res)
	}
	if len(lines) != 2 || lines[1]["exit_code"].(float64) != 1 || lines[1]["refused"] != want {
		t.Fatalf("audit = %v", lines)
	}
}

func TestCloseExitOneEmptyStderrKeepsSentenceOnly(t *testing.T) {
	res, lines := closeExitOne(t, "  \n")
	if res.Refused != closeRefused || lines[1]["refused"] != closeRefused {
		t.Fatalf("result = %+v, audit = %v", res, lines)
	}
}

func TestCloseExitOneCapsLongLastLine(t *testing.T) {
	res, lines := closeExitOne(t, "Traceback (most recent call last):\nValueError: "+strings.Repeat("x\u00e9", 400))
	suffix := strings.TrimPrefix(res.Refused, closeRefused+": ")
	if n := len([]rune(suffix)); n != 300 || !strings.HasPrefix(suffix, "ValueError: ") {
		t.Fatalf("runes = %d, refused = %q", n, res.Refused)
	}
	if lines[1]["refused"] != res.Refused {
		t.Fatalf("audit = %v", lines[1])
	}
}

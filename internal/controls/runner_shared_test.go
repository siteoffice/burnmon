package controls

import (
	"context"
	"errors"
	"testing"
)

// runner_shared_argv: no build tag, so it runs on every system.
func TestRunnerSharedArgv(t *testing.T) {
	for _, bad := range [][]string{nil, {""}, {"rel/prog"}, {"prog"}} {
		if err := validateArgv(bad); err == nil {
			t.Errorf("validateArgv(%q) = nil, want refusal", bad)
		}
	}
}

func TestClassifyRunSharedKillAndTimeout(t *testing.T) {
	if code, e := classifyRun(errors.New("x"), true, context.DeadlineExceeded); code != -1 || !errors.Is(e, context.DeadlineExceeded) {
		t.Errorf("kill + timeout: %d, %v", code, e)
	}
	if code, e := classifyRun(nil, true, nil); code != -1 || !errors.Is(e, context.Canceled) {
		t.Errorf("kill, no ctx error: %d, %v", code, e)
	}
	if code, e := classifyRun(nil, false, context.DeadlineExceeded); code != 0 || e != nil {
		t.Errorf("exited by itself: %d, %v", code, e)
	}
}

type fakeRunnerShared struct{}

func (fakeRunnerShared) Run(ctx context.Context, argv []string, dir string) (int, string, error) {
	return 0, "", nil
}
func (fakeRunnerShared) Start(argv []string) error { return nil }

// agent_controls_off_windows: goos is a package variable, never os.name.
func TestAgentControlsOffWindows(t *testing.T) {
	old := controlsGOOS
	controlsGOOS = "darwin"
	defer func() { controlsGOOS = old }()
	s := NewService(Env{
		Elevated:    func() bool { return false },
		Self:        func() (Self, error) { return Self{}, nil },
		Runner:      fakeRunnerShared{},
		Switches:    func() map[string]bool { return map[string]bool{Stop: true, Attach: true, Approve: true, SendBack: true} },
		ClaudePath:  "/usr/local/bin/claude",
		AgentViewOn: true,
		PythonPath:  "/usr/bin/python3",
	})
	a := s.Availability()
	for _, c := range []string{Stop, Attach} {
		if a.Ready[c] || a.Why[c] != WhyAgentOffWindows {
			t.Errorf("%s: ready %v why %q", c, a.Ready[c], a.Why[c])
		}
	}
	if WhyAgentOffWindows != "needs the process tree check, Windows only" {
		t.Errorf("reason text = %q", WhyAgentOffWindows)
	}
	if a.Why[Approve] == WhyAgentOffWindows || a.Why[CloseSprint] == WhyAgentOffWindows {
		t.Error("gate or close carries the agent reason")
	}
}

// elevated_root_off_windows
func TestElevatedRootOffWindows(t *testing.T) {
	if !rootElevated(0) || rootElevated(501) {
		t.Error("uid 0 must be elevated, 501 must not")
	}
}

// python3_first_off_windows
func TestPython3FirstOffWindows(t *testing.T) {
	both := func(n string) (string, error) {
		if n == "python3" || n == "python" {
			return "/usr/bin/" + n, nil
		}
		return "", errors.New("no")
	}
	if p, err := ResolvePython(both, "darwin"); err != nil || p != "/usr/bin/python3" {
		t.Errorf("both: %q, %v", p, err)
	}
	only := func(n string) (string, error) {
		if n == "python" {
			return "/usr/bin/python", nil
		}
		return "", errors.New("no")
	}
	if p, err := ResolvePython(only, "darwin"); err != nil || p != "/usr/bin/python" {
		t.Errorf("only python: %q, %v", p, err)
	}
}

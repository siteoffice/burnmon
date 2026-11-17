package cloudpull

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burnmon/internal/snapshotwriter"
)

// A pull whose CLI says the session is gone marks it gone: no feature error,
// never pulled again.
func TestPullOneSessionGone(t *testing.T) {
	r, _ := newTestRunner(t)
	r.Run = func(context.Context, string, string, ...string) ([]byte, error) {
		return nil, errors.New("exit status 1: Session not found: session_X")
	}
	if err := r.PullOne(context.Background(), "session_X"); err != nil {
		t.Fatalf("PullOne = %v, want nil for a gone session", err)
	}
	st, _ := LoadState(r.StatePath)
	if e := st.Sessions["session_X"]; e == nil || !e.Gone || st.LastError != "" || st.Disabled != "" {
		t.Fatalf("state = %+v, entry %+v", st, e)
	}
	if got := Due(t0.Add(48*time.Hour), []string{"session_X"}, st); len(got) != 0 {
		t.Fatalf("Due = %v, want a gone session skipped", got)
	}
}

func TestReadStatus(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	if s := ReadStatus(p); s.OK || s.Reason == "" {
		t.Errorf("no state: %+v, want not OK with a reason", s)
	}
	st := &State{Sessions: map[string]*Entry{"session_A": {SessionID: "a", Path: "p", PulledAt: t0}}}
	st.Save(p)
	if s := ReadStatus(p); !s.OK {
		t.Errorf("one good pull: %+v, want OK", s)
	}
	st.LastError = "cloudpull: Claude Code is not signed in"
	st.Save(p)
	if s := ReadStatus(p); s.OK || !strings.Contains(s.Reason, "not signed in") {
		t.Errorf("error: %+v", s)
	}
	st.LastError, st.Disabled = "", "session_A at x: a pull cost tokens"
	st.Save(p)
	if s := ReadStatus(p); s.OK || !strings.Contains(s.Reason, "cost tokens") {
		t.Errorf("disabled: %+v", s)
	}
}

func newTestSupervisor(t *testing.T, lockPath string, on *bool) (*Supervisor, *fakeCLI, *Runner) {
	t.Helper()
	r, f := newTestRunner(t)
	root := t.TempDir()
	writeList(t, root, "acc1", orgA, `{"entries":[{"sessionId":"session_A"}]}`)
	r.Roots = func() []string { return []string{root} }
	return &Supervisor{
		On:        func() bool { return *on },
		Lock:      snapshotwriter.NewLock(lockPath),
		Exe:       "test",
		StatePath: r.StatePath,
		NewRunner: func() (*Runner, error) { return r, nil },
	}, f, r
}

func TestSupervisorOnOff(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "cloud-pull.lock")
	on := false
	s, f, r := newTestSupervisor(t, lock, &on)
	ctx := context.Background()
	if err := s.Tick(ctx); err != nil || len(f.calls) != 0 || s.Lock.(*snapshotwriter.Lock).Held() {
		t.Fatalf("off: err %v, calls %v, held %v", err, f.calls, s.Lock.(*snapshotwriter.Lock).Held())
	}
	on = true
	if err := s.Tick(ctx); err != nil || len(f.calls) != 1 || !s.Lock.(*snapshotwriter.Lock).Held() {
		t.Fatalf("on: err %v, calls %v, held %v", err, f.calls, s.Lock.(*snapshotwriter.Lock).Held())
	}
	// A cost-guard stop, then switching off clears it and frees the lock.
	st, _ := LoadState(r.StatePath)
	st.Disabled = "cost"
	st.Save(r.StatePath)
	on = false
	if err := s.Tick(ctx); err != nil || s.Lock.(*snapshotwriter.Lock).Held() {
		t.Fatalf("off again: err %v, held %v", err, s.Lock.(*snapshotwriter.Lock).Held())
	}
	if st, _ := LoadState(r.StatePath); st.Disabled != "" {
		t.Fatal("switching off did not clear the cost-guard stop")
	}
}

// Two apps, one lock: only the holder pulls; the other takes over when the
// holder lets go.
func TestSupervisorOneAppPulls(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "cloud-pull.lock")
	on := true
	a, fa, _ := newTestSupervisor(t, lock, &on)
	b, fb, _ := newTestSupervisor(t, lock, &on)
	ctx := context.Background()
	if err := a.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fa.calls) != 1 || len(fb.calls) != 0 {
		t.Fatalf("calls a %v, b %v; want only a", fa.calls, fb.calls)
	}
	a.Lock.Release()
	if err := b.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fb.calls) != 1 {
		t.Fatalf("b did not take over: %v", fb.calls)
	}
	b.Lock.Release()
}

// No CLI: the reason lands in the state file for the screen.
func TestSupervisorNoCLI(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "cloud-pull.lock")
	on := true
	s, _, _ := newTestSupervisor(t, lock, &on)
	s.NewRunner = func() (*Runner, error) { return nil, errors.New("cloudpull: Claude Code is not installed") }
	if err := s.Tick(context.Background()); err == nil {
		t.Fatal("no error without a CLI")
	}
	if st := ReadStatus(s.StatePath); st.OK || !strings.Contains(st.Reason, "not installed") {
		t.Fatalf("status = %+v", st)
	}
	s.Lock.Release()
}

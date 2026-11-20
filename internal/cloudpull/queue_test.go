package cloudpull

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const cliReason = "Couldn't load the history of cloud session session_X. Check it at https://claude.ai/code/session_X?from=cli"

// The CLI exits 1 with an empty output and its reason on the error stream
// (measured 2026-11-20). The reason and the exit code reach the entry.
func TestPullOneKeepsTheCLIReason(t *testing.T) {
	r, _ := newTestRunner(t)
	r.Run = func(context.Context, string, string, ...string) ([]byte, error) {
		return nil, errors.New("exit status 1: " + cliReason)
	}
	err := r.PullOne(context.Background(), "session_X")
	if err == nil || !strings.Contains(err.Error(), cliReason) || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("err = %v, want the CLI reason and its exit code", err)
	}
	st, _ := LoadState(r.StatePath)
	e := st.Sessions["session_X"]
	if e == nil || !strings.Contains(e.LastError, cliReason) || e.Failures != 1 {
		t.Fatalf("entry = %+v", e)
	}
	if strings.HasSuffix(strings.TrimSpace(e.LastError), ":") {
		t.Fatalf("the reason is empty: %q", e.LastError)
	}
	if st.LastError != "" {
		t.Fatalf("one failing session set the runner-wide error: %q", st.LastError)
	}
}

// A failing session is not a runner-wide error: Step does not record it either.
func TestStepKeepsSessionFailureOffTheRunner(t *testing.T) {
	r, f := newTestRunner(t)
	f.fail = true
	root := t.TempDir()
	writeList(t, root, "acc1", orgA, `{"entries":[{"sessionId":"session_A"}]}`)
	r.Roots = func() []string { return []string{root} }
	if err := r.Step(context.Background()); err == nil {
		t.Fatal("Step = nil, want the session's error returned")
	}
	st, _ := LoadState(r.StatePath)
	if st.LastError != "" || st.Sessions["session_A"] == nil || st.Sessions["session_A"].LastError == "" {
		t.Fatalf("state = %+v", st)
	}
}

func TestRetryDelayDoublesToOneDay(t *testing.T) {
	want := map[int]time.Duration{0: 15 * time.Minute, 1: 15 * time.Minute, 2: 30 * time.Minute, 3: time.Hour,
		4: 2 * time.Hour, 7: 16 * time.Hour, 8: 24 * time.Hour, 30: 24 * time.Hour}
	for n, d := range want {
		if got := retryDelay(n); got != d {
			t.Errorf("retryDelay(%d) = %v, want %v", n, got, d)
		}
	}
	st := &State{Sessions: map[string]*Entry{"session_F": {FailedAt: t0.Add(-59 * time.Minute), Failures: 3, LastError: "x"}}}
	if got := Due(t0, []string{"session_F"}, st); len(got) != 0 {
		t.Errorf("third failure, 59 minutes: Due = %v, want none", got)
	}
	st.Sessions["session_F"].FailedAt = t0.Add(-time.Hour)
	if got := Due(t0, []string{"session_F"}, st); len(got) != 1 {
		t.Errorf("third failure, 60 minutes: Due = %v, want one", got)
	}
}

func TestFailuresCountInARowAndReset(t *testing.T) {
	r, f := newTestRunner(t)
	f.fail = true
	for i := 0; i < 2; i++ {
		r.PullOne(context.Background(), "session_A")
	}
	st, _ := LoadState(r.StatePath)
	if n := st.Sessions["session_A"].Failures; n != 2 {
		t.Fatalf("Failures = %d, want 2", n)
	}
	f.fail = false
	if err := r.PullOne(context.Background(), "session_A"); err != nil {
		t.Fatal(err)
	}
	st, _ = LoadState(r.StatePath)
	if e := st.Sessions["session_A"]; e.Failures != 0 || !e.FailedAt.IsZero() {
		t.Fatalf("after a good pull: %+v", e)
	}
}

// 31 failing ids (11 never copied, 20 copied before) wait behind every due
// re-pull and every new id.
func TestDueRetriesOfFailedIDsGoLast(t *testing.T) {
	day := 24 * time.Hour
	st := &State{Sessions: map[string]*Entry{}}
	var list []string
	for i := 0; i < 11; i++ {
		id := fmt.Sprintf("session_bad_new_%d", i)
		list = append(list, id)
		st.Sessions[id] = &Entry{FailedAt: t0.Add(-16 * time.Minute), Failures: 1, LastError: "x"}
	}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("session_bad_old_%d", i)
		list = append(list, id)
		e := entry(2*day, 3*day)
		e.FailedAt, e.Failures, e.LastError = t0.Add(-16*time.Minute), 1, "x"
		st.Sessions[id] = e
	}
	list = append(list, "session_warm", "session_cold", "session_new")
	st.Sessions["session_warm"] = entry(40*time.Minute, 2*time.Hour)
	st.Sessions["session_cold"] = entry(25*time.Hour, 3*day)
	got := Due(t0, list, st)
	if len(got) != 34 {
		t.Fatalf("Due holds %d ids, want 34", len(got))
	}
	front := map[string]bool{got[0]: true, got[1]: true, got[2]: true}
	for _, id := range []string{"session_warm", "session_cold", "session_new"} {
		if !front[id] {
			t.Fatalf("the first three are %v, want %s among them", got[:3], id)
		}
	}
}

// An hour of ticks with 31 failing ids: every due re-pull is pulled in that hour.
func TestRepullsGetTheirTurnWithManyFailingIDs(t *testing.T) {
	r, _ := newTestRunner(t)
	now := t0
	r.Now = func() time.Time { return now }
	st := &State{Sessions: map[string]*Entry{}}
	var list []string
	for i := 0; i < 31; i++ {
		id := fmt.Sprintf("session_bad_%d", i)
		list = append(list, id)
		st.Sessions[id] = &Entry{FailedAt: t0.Add(-20 * time.Minute), Failures: 1, LastError: "x"}
	}
	warm := []string{"session_w1", "session_w2", "session_w3", "session_w4", "session_w5"}
	for _, id := range warm {
		list = append(list, id)
		st.Sessions[id] = entry(40*time.Minute, 2*time.Hour)
	}
	if err := st.Save(r.StatePath); err != nil {
		t.Fatal(err)
	}
	pulled := map[string]bool{}
	r.Run = func(_ context.Context, _, _ string, args ...string) ([]byte, error) {
		id := args[1]
		pulled[id] = true
		if strings.HasPrefix(id, "session_bad") {
			return nil, errors.New("exit status 1: " + cliReason)
		}
		return []byte(`{"is_error":false,"num_turns":0,"total_cost_usd":0,"session_id":"` + id + `"}`), nil
	}
	for i := 0; i < 120; i++ {
		r.Tick(context.Background(), list)
		now = now.Add(30 * time.Second)
	}
	for _, id := range warm {
		if !pulled[id] {
			t.Errorf("%s was not re-pulled within the hour", id)
		}
	}
}

func TestReadStatusPartialAndAllFailing(t *testing.T) {
	p := t.TempDir() + `\state.json`
	st := &State{Sessions: map[string]*Entry{
		"session_A": {SessionID: "a", Path: "p", PulledAt: t0},
		"session_B": {SessionID: "b", Path: "p", PulledAt: t0, FailedAt: t0.Add(-time.Hour), LastError: "cloudpull: old"},
		"session_C": {FailedAt: t0, LastError: "cloudpull: " + cliReason},
		"session_D": {Gone: true, LastError: "session not found", FailedAt: t0},
	}}
	st.Save(p)
	s := ReadStatus(p)
	if !s.OK || s.Unread != 2 || s.UnreadReason != cliReason {
		t.Fatalf("some read, some failing: %+v", s)
	}
	st.Sessions = map[string]*Entry{"session_C": st.Sessions["session_C"]}
	st.Save(p)
	if s := ReadStatus(p); s.OK || s.Reason != cliReason {
		t.Fatalf("none read: %+v, want not OK with the CLI reason", s)
	}
}

// Through the real path: the failure of PullOne reaches ReadStatus, and the
// reason the strip prints holds the CLI's words and its exit code.
func TestPullOneFailureReachesReadStatus(t *testing.T) {
	r, _ := newTestRunner(t)
	st := &State{Sessions: map[string]*Entry{"session_A": {SessionID: "a", Path: "p", PulledAt: t0}}}
	if err := st.Save(r.StatePath); err != nil {
		t.Fatal(err)
	}
	r.Run = func(context.Context, string, string, ...string) ([]byte, error) {
		return []byte("Error: nope\nmore"), errors.New("exit status 1: " + cliReason)
	}
	r.PullOne(context.Background(), "session_X")
	s := ReadStatus(r.StatePath)
	if !s.OK || s.Unread != 1 || !strings.Contains(s.UnreadReason, cliReason) ||
		!strings.Contains(s.UnreadReason, "exit status 1") || !strings.Contains(s.UnreadReason, "Error: nope") {
		t.Fatalf("status = %+v", s)
	}
}

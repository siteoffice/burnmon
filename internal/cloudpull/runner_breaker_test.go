package cloudpull

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// The CLI's real result for a session whose history loads but is too long for
// the rename turn (measured 2026-11-23): is_error, one turn, no cost, exit 1.
const promptTooLong = `{"type":"result","subtype":"success","is_error":true,"api_error_status":400,"result":"Prompt is too long","num_turns":1,"total_cost_usd":0,"session_id":"81ac3b9c-5783-4435-a76e-2a9d17b950d7"}`

func TestParseResultErrorWithoutCostIsAPlainFailure(t *testing.T) {
	_, err := ParseResult([]byte(promptTooLong))
	if err == nil || errors.Is(err, ErrCost) {
		t.Fatalf("err = %v, want a plain failure, not ErrCost", err)
	}
	if !strings.Contains(err.Error(), "Prompt is too long") {
		t.Errorf("err = %q, want the CLI's own words", err)
	}
	// A turn that is not an error, and any cost, still trip the guard.
	for name, in := range map[string]string{
		"turn":       `{"is_error":false,"num_turns":1,"total_cost_usd":0,"session_id":"x"}`,
		"error+cost": `{"is_error":true,"num_turns":1,"total_cost_usd":0.02,"result":"x","session_id":"x"}`,
	} {
		if _, err := ParseResult([]byte(in)); !errors.Is(err, ErrCost) {
			t.Errorf("%s: err = %v, want ErrCost", name, err)
		}
	}
}

func TestPullOnePromptTooLongKeepsPullingOn(t *testing.T) {
	r, _ := newTestRunner(t)
	r.Run = func(context.Context, string, string, ...string) ([]byte, error) {
		return []byte(promptTooLong), errors.New("exit status 1")
	}
	err := r.PullOne(context.Background(), "session_A")
	if err == nil || errors.Is(err, ErrCost) {
		t.Fatalf("err = %v, want a plain failure", err)
	}
	st, _ := LoadState(r.StatePath)
	e := st.Sessions["session_A"]
	if st.Disabled != "" || e == nil || !strings.Contains(e.LastError, "Prompt is too long") ||
		strings.Contains(e.LastError, "exited with") {
		t.Fatalf("state = %+v, entry %+v", st, e)
	}
}

func couldNotLoad(id string) error {
	return fmt.Errorf("exit status 1: Couldn't load the history of cloud session %s. Check it at https://claude.ai/code/%s?from=cli", id, id)
}

func TestBreakerPausesAfterFiveDifferentSessionsFailTheSameWay(t *testing.T) {
	r, f := newTestRunner(t)
	r.Run = func(_ context.Context, _, _ string, args ...string) ([]byte, error) {
		f.calls = append(f.calls, args[1])
		return nil, couldNotLoad(args[1])
	}
	ctx := context.Background()
	for i := 1; i <= 4; i++ {
		r.PullOne(ctx, fmt.Sprintf("session_%d", i))
	}
	// The same session failing again is not a different session.
	r.PullOne(ctx, "session_4")
	if st, _ := LoadState(r.StatePath); !st.PausedUntil.IsZero() {
		t.Fatalf("paused after 4 different sessions: %+v", st)
	}
	r.PullOne(ctx, "session_5")
	st, _ := LoadState(r.StatePath)
	if want := t0.Add(6 * time.Hour); !st.PausedUntil.Equal(want) {
		t.Fatalf("PausedUntil = %v, want %v", st.PausedUntil, want)
	}
	if !strings.Contains(st.PauseReason, "Couldn't load the history") || strings.Contains(st.PauseReason, "session_5") {
		t.Errorf("PauseReason = %q, want the error without the session id", st.PauseReason)
	}
	n := len(f.calls)
	if got, err := r.Tick(ctx, []string{"session_new"}); err != nil || got != "" || len(f.calls) != n {
		t.Fatalf("Tick while paused = %q, %v, calls %d -> %d", got, err, n, len(f.calls))
	}
	now := t0.Add(6*time.Hour + time.Minute)
	r.Now = func() time.Time { return now }
	if got, err := r.Tick(ctx, []string{"session_new"}); got != "session_new" || err == nil || len(f.calls) != n+1 {
		t.Fatalf("Tick after the pause = %q, %v, calls %d", got, err, len(f.calls))
	}
}

func TestBreakerStreakEndsOnAGoodPull(t *testing.T) {
	r, f := newTestRunner(t)
	ctx := context.Background()
	fail := func(id string) {
		inner := r.Run
		r.Run = func(context.Context, string, string, ...string) ([]byte, error) { return nil, couldNotLoad(id) }
		r.PullOne(ctx, id)
		r.Run = inner
	}
	for i := 1; i <= 4; i++ {
		fail(fmt.Sprintf("session_%d", i))
	}
	if err := r.PullOne(ctx, "session_ok"); err != nil || f.n != 1 {
		t.Fatalf("good pull: %v, n %d", err, f.n)
	}
	fail("session_5")
	if st, _ := LoadState(r.StatePath); !st.PausedUntil.IsZero() {
		t.Fatalf("paused although a good pull came between: %+v", st)
	}
}

func TestCheckoutFailureHasItsOwnReasonAndRetriesDaily(t *testing.T) {
	r, _ := newTestRunner(t)
	r.Run = func(_ context.Context, _, _ string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("exit status 1: Run claude --teleport %s from a checkout of zero-nonsense-dev/burnrate-private.", args[1])
	}
	ctx := context.Background()
	for i := 1; i <= 6; i++ {
		r.PullOne(ctx, fmt.Sprintf("session_%d", i))
	}
	st, _ := LoadState(r.StatePath)
	e := st.Sessions["session_1"]
	if e == nil || !strings.Contains(e.LastError, "needs a checkout of zero-nonsense-dev/burnrate-private") {
		t.Fatalf("entry = %+v", e)
	}
	if !st.PausedUntil.IsZero() {
		t.Errorf("checkout failures fed the breaker: %+v", st)
	}
	list := []string{"session_1"}
	if got := Due(t0.Add(23*time.Hour), list, st); len(got) != 0 {
		t.Errorf("due after 23 h: %v", got)
	}
	if got := Due(t0.Add(25*time.Hour), list, st); len(got) != 1 {
		t.Errorf("not due after 25 h: %v", got)
	}
}

func TestStatusCarriesLastPulledAtAndPause(t *testing.T) {
	r, _ := newTestRunner(t)
	old := statusNow
	statusNow = func() time.Time { return t0 }
	defer func() { statusNow = old }()
	pulled := t0.Add(-72 * time.Hour)
	st := &State{
		PausedUntil: t0.Add(time.Hour),
		PauseReason: "Couldn't load the history",
		Sessions: map[string]*Entry{
			"session_A": {SessionID: "a", Path: "p", PulledAt: pulled, FailedAt: t0, LastError: "x"},
			"session_B": {SessionID: "b", Path: "q", PulledAt: pulled.Add(-time.Hour)},
		},
	}
	if err := st.Save(r.StatePath); err != nil {
		t.Fatal(err)
	}
	s := ReadStatus(r.StatePath)
	if !s.OK || !s.LastPulledAt.Equal(pulled) || s.Unread != 1 {
		t.Fatalf("status = %+v", s)
	}
	if !strings.Contains(s.UnreadReason, "paused") || !strings.Contains(s.UnreadReason, "Couldn't load the history") {
		t.Errorf("UnreadReason = %q, want the pause named", s.UnreadReason)
	}
}

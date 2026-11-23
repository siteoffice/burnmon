package cloudpull

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A session whose history loads but is too long for the CLI's rename turn:
// the CLI writes the whole copy first, then returns is_error, 1 turn, cost 0,
// "Prompt is too long" and exit 1 (measured 2026-11-23 on a live session: 258
// rows, 86 with usage, written before the failure). The copy is good; only the
// rename failed, and nothing was spent.
func TestPullOneTooLongHistoryIsStillACopy(t *testing.T) {
	r, _ := newTestRunner(t)
	body := `{"type":"assistant","timestamp":"2026-11-17T11:59:00Z","requestId":"r1","message":{"usage":{"input_tokens":1}}}` + "\n"
	r.Run = func(context.Context, string, string, ...string) ([]byte, error) {
		if err := os.WriteFile(filepath.Join(r.ProjectDir, "local-big.jsonl"), []byte(body), 0o644); err != nil {
			return nil, err
		}
		out := `{"is_error":true,"num_turns":1,"total_cost_usd":0,"result":"Prompt is too long","session_id":"local-big"}`
		return []byte(out), errors.New("exit status 1")
	}
	if err := r.PullOne(context.Background(), "session_BIG"); err != nil {
		t.Fatalf("err = %v, want the copy to count", err)
	}
	st, _ := LoadState(r.StatePath)
	e := st.Sessions["session_BIG"]
	if st.Disabled != "" || e == nil || e.SessionID != "local-big" || e.Path == "" || !e.PulledAt.Equal(t0) ||
		e.LastError != "" || e.Failures != 0 || !e.FailedAt.IsZero() {
		t.Fatalf("state = %+v, entry %+v", st, e)
	}
	if !e.LastUsage.Equal(time.Date(2026, 11, 17, 11, 59, 0, 0, time.UTC)) {
		t.Errorf("LastUsage = %v", e.LastUsage)
	}
}

// The same result with no copy on disk is still a plain failure.
func TestPullOneTooLongWithoutACopyFails(t *testing.T) {
	r, _ := newTestRunner(t)
	r.Run = func(context.Context, string, string, ...string) ([]byte, error) {
		return []byte(promptTooLong), errors.New("exit status 1")
	}
	if err := r.PullOne(context.Background(), "session_BIG"); err == nil {
		t.Fatal("a result with no copy counted as a copy")
	}
	st, _ := LoadState(r.StatePath)
	if e := st.Sessions["session_BIG"]; e == nil || e.Path != "" || !strings.Contains(e.LastError, "Prompt is too long") {
		t.Fatalf("entry %+v", e)
	}
}

// The CLI's failed rename turn adds synthetic rows with zero tokens and the pull
// time (measured 2026-11-23). They are not the session's last activity: an old
// session must not read as active just because it was pulled now, or it would be
// pulled every 2 minutes as a hot one.
func TestLastUsageIgnoresSyntheticRows(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	body := `{"type":"assistant","timestamp":"2026-08-05T10:00:00Z","requestId":"r1","message":{"model":"claude-opus-5-5","usage":{"input_tokens":5}}}` + "\n" +
		`{"type":"assistant","timestamp":"2026-11-23T16:10:59.661Z","message":{"model":"<synthetic>","usage":{"input_tokens":0,"output_tokens":0}}}` + "\n" +
		`{"type":"assistant","timestamp":"2026-11-23T16:11:06.025Z","requestId":"req_x","message":{"model":"<synthetic>","usage":{"input_tokens":0,"output_tokens":0}}}` + "\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LastUsage(p)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("LastUsage = %v, want %v (the synthetic rows must not count)", got, want)
	}
}

// Entries written by an older runner carry only the words, not the flags (2026-11-23: three
// "from a checkout of" entries sat in "other" and the cut at 90 characters ended inside the repo
// name, "checkout of w"). The reason follows the words, and no repo name reaches the screen.
func TestReasonOfClassifiesCheckoutByTheWords(t *testing.T) {
	e := &Entry{LastError: "cloudpull: the CLI exited with exit status 1: Run claude --teleport session_01G9gfXpysDJkVjFMxWRDJAA from a checkout of wilcodetree/znd-skills.", FailedAt: t0, Failures: 1}
	kind, _ := reasonOf(e)
	if kind != ReasonCheckout {
		t.Fatalf("kind = %q, want %q", kind, ReasonCheckout)
	}
	plain := Reason{Kind: kind, N: 3}.Plain()
	if plain != "3 need a checkout of their repo" || strings.Contains(plain, "wilcodetree") || strings.Contains(plain, "/") {
		t.Errorf("plain = %q, want no repo or organisation name", plain)
	}
}

// An "other" reason keeps the CLI's words but no repo, organisation or session id.
func TestOtherReasonCarriesNoRepoOrSessionID(t *testing.T) {
	e := &Entry{LastError: "cloudpull: the CLI exited with exit status 1: session_0131Y29YQutk7UuzNFk8CJJS stopped while reading from a checkout of org/repo, then the network went away", FailedAt: t0, Failures: 1}
	kind, text := reasonOf(e)
	if kind != ReasonOther {
		t.Fatalf("kind = %q, want other", kind)
	}
	for _, bad := range []string{"session_", "org/repo", "0131Y29"} {
		if strings.Contains(text, bad) {
			t.Errorf("text %q still holds %q", text, bad)
		}
	}
	if !strings.Contains(text, "network went away") || !strings.Contains(text, "exit status 1") {
		t.Errorf("text %q lost the CLI's own words", text)
	}
}

// LastMeasured is the newest session activity inside the copies, not the time a copy was
// written. A fresh re-copy of an August session writes rows stamped today, but its usage is
// August's (2026-11-23: LastPulledAt 17:40 UTC, newest real usage 17 Nov).
func TestStatusLastMeasuredIsTheNewestRealUsage(t *testing.T) {
	r, _ := newTestRunner(t)
	aug := time.Date(2026, 8, 5, 8, 40, 24, 0, time.UTC)
	nov := time.Date(2026, 11, 17, 16, 1, 0, 0, time.UTC)
	st := &State{Sessions: map[string]*Entry{
		"session_aug":    {SessionID: "a", Path: "p", PulledAt: t0, LastUsage: aug},
		"session_nov":    {SessionID: "b", Path: "q", PulledAt: t0.Add(-72 * time.Hour), LastUsage: nov},
		"session_nousag": {SessionID: "c", Path: "r", PulledAt: t0},
		"session_fail":   {LastError: "cloudpull: the CLI exited with exit status 1: " + notFoundMsg + " x", FailedAt: t0, Failures: 1, LastUsage: t0},
	}}
	if err := st.Save(r.StatePath); err != nil {
		t.Fatal(err)
	}
	s := ReadStatus(r.StatePath)
	if !s.LastMeasured.Equal(nov) {
		t.Errorf("LastMeasured = %v, want %v", s.LastMeasured, nov)
	}
	if !s.LastPulledAt.Equal(t0) {
		t.Errorf("LastPulledAt = %v, want %v (the runner still needs it)", s.LastPulledAt, t0)
	}
}

// The state of the screenshot of 2026-11-23: 88 not found, 3 old checkout entries. The line says
// "3 need a checkout of their repo", not "checkout of w".
func TestStatusOfTheScreenshotState(t *testing.T) {
	r, _ := newTestRunner(t)
	st := &State{Sessions: map[string]*Entry{"session_ok": {SessionID: "a", Path: "p", PulledAt: t0, LastUsage: t0}}}
	for i := 0; i < 88; i++ {
		id := fmt.Sprintf("session_nf%02d", i)
		st.Sessions[id] = &Entry{LastError: "cloudpull: the CLI exited with exit status 1: Couldn't load the history of cloud session " + id + ". Check it at https://claude.ai/code/" + id + "?from=cli", FailedAt: t0, Failures: 1}
	}
	for _, repo := range []string{"zero-nonsense-dev/burnrate-private", "wilcodetree/znd-skills", "someorg/their-repo"} {
		id := "session_co_" + strings.ReplaceAll(repo, "/", "_")
		st.Sessions[id] = &Entry{LastError: "cloudpull: the CLI exited with exit status 1: Run claude --teleport " + id + " from a checkout of " + repo + ".", FailedAt: t0, Failures: 1}
	}
	if err := st.Save(r.StatePath); err != nil {
		t.Fatal(err)
	}
	s := ReadStatus(r.StatePath)
	var plain []string
	for _, rs := range s.Reasons {
		plain = append(plain, rs.Plain())
	}
	got := strings.Join(plain, "; ")
	if got != "88 are not found by the Claude Code service for your login; 3 need a checkout of their repo" {
		t.Fatalf("reasons = %q", got)
	}
}

func boom(id string) error { return fmt.Errorf("exit status 1: boom for %s", id) }

// "Couldn't load the history" is the API saying not found for this session
// under this login. It is that session's own state: retried once a day, and it
// does not feed the breaker, which would otherwise pause the sessions that do
// load (2 of 6 sampled loaded on 2026-11-23, the other 4 did not).
func TestUnloadableSessionRetriesDailyAndStaysOutOfTheBreaker(t *testing.T) {
	r, _ := newTestRunner(t)
	r.Run = func(_ context.Context, _, _ string, args ...string) ([]byte, error) {
		return nil, couldNotLoad(args[1])
	}
	ctx := context.Background()
	for i := 1; i <= 8; i++ {
		r.PullOne(ctx, fmt.Sprintf("session_%d", i))
	}
	st, _ := LoadState(r.StatePath)
	if !st.PausedUntil.IsZero() || st.Streak != nil {
		t.Fatalf("not-found sessions fed the breaker: paused %v streak %+v", st.PausedUntil, st.Streak)
	}
	e := st.Sessions["session_1"]
	if e == nil || !e.Unloadable || !strings.Contains(e.LastError, "Couldn't load the history") {
		t.Fatalf("entry %+v", e)
	}
	list := []string{"session_1"}
	if got := Due(t0.Add(23*time.Hour), list, st); len(got) != 0 {
		t.Errorf("due after 23 h: %v", got)
	}
	if got := Due(t0.Add(25*time.Hour), list, st); len(got) != 1 {
		t.Errorf("not due after 25 h: %v", got)
	}
}

// A failure of another kind still pauses the pull after 5 different sessions.
func TestBreakerStillCatchesOtherErrors(t *testing.T) {
	r, _ := newTestRunner(t)
	r.Run = func(_ context.Context, _, _ string, args ...string) ([]byte, error) { return nil, boom(args[1]) }
	for i := 1; i <= 5; i++ {
		r.PullOne(context.Background(), fmt.Sprintf("session_%d", i))
	}
	if st, _ := LoadState(r.StatePath); st.PausedUntil.IsZero() {
		t.Fatal("5 different sessions failing the same way did not pause the pull")
	}
}

func TestStatusCountsEachReason(t *testing.T) {
	r, _ := newTestRunner(t)
	st := &State{Sessions: map[string]*Entry{
		"session_ok": {SessionID: "a", Path: "p", PulledAt: t0},
	}}
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("session_nf%d", i)
		st.Sessions[id] = &Entry{LastError: "cloudpull: the CLI exited with exit status 1: Couldn't load the history of cloud session " + id + ".", FailedAt: t0.Add(time.Duration(i) * time.Minute), Failures: 1, Unloadable: true}
	}
	for i := 0; i < 2; i++ {
		id := fmt.Sprintf("session_co%d", i)
		st.Sessions[id] = &Entry{LastError: "cloudpull: needs a checkout of a/b", FailedAt: t0.Add(time.Hour), Failures: 1, NeedsCheckout: true}
	}
	st.Sessions["session_x"] = &Entry{LastError: "cloudpull: the CLI exited with exit status 1: network down", FailedAt: t0, Failures: 1}
	st.Sessions["session_gone"] = &Entry{LastError: "session not found", FailedAt: t0, Gone: true}
	if err := st.Save(r.StatePath); err != nil {
		t.Fatal(err)
	}
	s := ReadStatus(r.StatePath)
	if !s.OK || s.Unread != 8 {
		t.Fatalf("status = %+v, want 8 unread (the gone one is not counted)", s)
	}
	if len(s.Reasons) != 3 {
		t.Fatalf("reasons = %+v, want 3 kinds", s.Reasons)
	}
	if s.Reasons[0].Kind != ReasonNotFound || s.Reasons[0].N != 5 || s.Reasons[1].Kind != ReasonCheckout || s.Reasons[1].N != 2 ||
		s.Reasons[2].Kind != ReasonOther || s.Reasons[2].N != 1 {
		t.Errorf("reasons = %+v, want 5 not found, 2 checkout, 1 other, largest first", s.Reasons)
	}
	if s.Reasons[0].Example != "session_nf4" {
		t.Errorf("example = %q, want the newest failure's id", s.Reasons[0].Example)
	}
	if got := s.Reasons[0].Plain(); got != "5 are not found by the Claude Code service for your login" {
		t.Errorf("plain = %q", got)
	}
	if got := s.Reasons[1].Plain(); got != "2 need a checkout of their repo" {
		t.Errorf("plain = %q", got)
	}
	if got := (Reason{Kind: ReasonNotFound, N: 1}).Plain(); got != "1 is not found by the Claude Code service for your login" {
		t.Errorf("plain one = %q", got)
	}
}

// A real pull of one session the CLI can load, through the real runner and CLI,
// with a temp state file. Set BURNMON_LIVE_PULL_ID to a session id that loads
// (2026-11-23: session_01AbHCJ1k9V9uEkMguVJ2HPH, whose history is too long for the
// rename turn). The copy it writes is removed afterwards.
func TestLivePullOfALoadableSession(t *testing.T) {
	id := os.Getenv("BURNMON_LIVE_PULL_ID")
	if id == "" {
		t.Skip("set BURNMON_LIVE_PULL_ID to a cloud session id that loads to run a real pull")
	}
	r, err := NewRunner()
	if err != nil {
		t.Fatal(err)
	}
	r.StatePath = filepath.Join(t.TempDir(), "state.json")
	if err := r.PullOne(context.Background(), id); err != nil {
		t.Fatalf("the pull failed: %v", err)
	}
	st, _ := LoadState(r.StatePath)
	e := st.Sessions[id]
	if e == nil || e.Path == "" || !e.FailedAt.IsZero() || e.LastError != "" {
		t.Fatalf("state entry = %+v, want a copy with no failure", e)
	}
	t.Cleanup(func() { r.removeOld(e.SessionID) })
	if !exists(e.Path) || e.LastUsage.IsZero() {
		t.Fatalf("copy %s exists %v, last usage %v", e.Path, exists(e.Path), e.LastUsage)
	}
	fi, _ := os.Stat(e.Path)
	t.Logf("live copy ok: %d bytes, last usage %s, pulled %s", fi.Size(), e.LastUsage.Format(time.RFC3339), e.PulledAt.Format(time.RFC3339))
}

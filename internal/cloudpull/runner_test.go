package cloudpull

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

func TestProjectDirName(t *testing.T) {
	got := ProjectDirName(`C:\Users\WilcoDeTree\.burnmon\cloud-pull`)
	if want := "C--Users-WilcoDeTree--burnmon-cloud-pull"; got != want {
		t.Fatalf("ProjectDirName = %q, want %q", got, want)
	}
}

func TestResolveCLI(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(dir, "claude.cmd")
	real := filepath.Join(dir, "node_modules", "@anthropic-ai", "claude-code", "bin", "claude.exe")
	for _, p := range []string{shim, real} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	look := func(p string) func(string) (string, error) {
		return func(string) (string, error) { return p, nil }
	}
	if got, err := ResolveCLI(look(shim)); err != nil || got != real {
		t.Errorf("shim: ResolveCLI = %q, %v; want %q", got, err, real)
	}
	native := filepath.Join(dir, "claude.exe")
	if got, err := ResolveCLI(look(native)); err != nil || got != native {
		t.Errorf("native: ResolveCLI = %q, %v; want %q", got, err, native)
	}
	// A shim without the package next to it cannot be used.
	other := t.TempDir()
	lone := filepath.Join(other, "claude.ps1")
	if _, err := ResolveCLI(look(lone)); err == nil {
		t.Error("lone shim: no error")
	}
	missing := func(string) (string, error) { return "", errors.New("not found") }
	if _, err := ResolveCLI(missing); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("missing: err = %v, want 'not installed'", err)
	}
}

func TestParseResult(t *testing.T) {
	ok := `{"type":"result","is_error":false,"num_turns":0,"total_cost_usd":0,"session_id":"aaaa-1","local_command":"rename"}`
	r, err := ParseResult([]byte(ok))
	if err != nil || r.SessionID != "aaaa-1" {
		t.Fatalf("ok: %+v, %v", r, err)
	}
	for name, in := range map[string]string{
		"turns": `{"is_error":false,"num_turns":1,"total_cost_usd":0,"session_id":"x"}`,
		"cost":  `{"is_error":false,"num_turns":0,"total_cost_usd":0.01,"session_id":"x"}`,
	} {
		if _, err := ParseResult([]byte(in)); !errors.Is(err, ErrCost) {
			t.Errorf("%s: err = %v, want ErrCost", name, err)
		}
	}
	for name, in := range map[string]string{
		"is_error":   `{"is_error":true,"num_turns":0,"total_cost_usd":0,"session_id":"x"}`,
		"no session": `{"is_error":false,"num_turns":0,"total_cost_usd":0}`,
		"not json":   `Error: not logged in`,
	} {
		_, err := ParseResult([]byte(in))
		if err == nil || errors.Is(err, ErrCost) {
			t.Errorf("%s: err = %v, want a plain failure", name, err)
		}
	}
}

func TestLastUsage(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	body := `{"type":"user","timestamp":"2026-11-17T09:00:00Z"}
{"type":"assistant","timestamp":"2026-11-17T10:01:42.657Z","requestId":"r1","message":{"usage":{"input_tokens":5}}}
not json
{"type":"assistant","timestamp":"2026-11-17T09:30:00Z","requestId":"r0","message":{"usage":{"input_tokens":3}}}
{"type":"system","timestamp":"2026-11-17T10:34:49Z","content":"Session renamed"}
`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LastUsage(p)
	want := time.Date(2026, 11, 17, 10, 1, 42, 657000000, time.UTC)
	if err != nil || !got.Equal(want) {
		t.Fatalf("LastUsage = %v, %v; want %v", got, err, want)
	}
}

func TestStateRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	st, err := LoadState(p)
	if err != nil || st.Disabled != "" || len(st.Sessions) != 0 {
		t.Fatalf("missing file: %+v, %v", st, err)
	}
	st.Disabled = "cost"
	st.Sessions["session_A"] = &Entry{SessionID: "aaaa", Path: `C:\x\aaaa.jsonl`, PulledAt: t0, LastUsage: t0.Add(-time.Hour)}
	if err := st.Save(p); err != nil {
		t.Fatal(err)
	}
	back, err := LoadState(p)
	if err != nil || !reflect.DeepEqual(back, st) {
		t.Fatalf("round trip: %+v, %v; want %+v", back, err, st)
	}
}

func entry(pulledAgo, usageAgo time.Duration) *Entry {
	e := &Entry{SessionID: "x", Path: "p", PulledAt: t0.Add(-pulledAgo)}
	if usageAgo >= 0 {
		e.LastUsage = t0.Add(-usageAgo)
	}
	return e
}

func TestDueTiers(t *testing.T) {
	day := 24 * time.Hour
	cases := []struct {
		name string
		e    *Entry
		due  bool
	}{
		{"hot, pulled 2m ago", entry(2*time.Minute, 29*time.Minute), true},
		{"hot, pulled 1m ago", entry(time.Minute, 29*time.Minute), false},
		{"warm (31m), pulled 2m ago", entry(2*time.Minute, 31*time.Minute), false},
		{"warm, pulled 30m ago", entry(30*time.Minute, 2*time.Hour), true},
		{"warm, pulled 29m ago", entry(29*time.Minute, 2*time.Hour), false},
		{"cold, pulled 24h ago", entry(day, 2*day), true},
		{"cold, pulled 23h ago", entry(23*time.Hour, 2*day), false},
		{"archive, pulled long ago", entry(40*day, 32*day), false},
		{"no usage rows: cold", entry(day, -1), true},
		{"no usage rows, pulled 1h ago", entry(time.Hour, -1), false},
	}
	for _, c := range cases {
		st := &State{Sessions: map[string]*Entry{"session_A": c.e}}
		got := Due(t0, []string{"session_A"}, st)
		if (len(got) == 1) != c.due {
			t.Errorf("%s: Due = %v, want due=%v", c.name, got, c.due)
		}
	}
}

func TestDueFailedBackoff(t *testing.T) {
	st := &State{Sessions: map[string]*Entry{
		"session_N": {FailedAt: t0.Add(-14 * time.Minute), LastError: "x"},
		"session_M": {FailedAt: t0.Add(-15 * time.Minute), LastError: "x"},
		"session_H": {SessionID: "h", Path: "p", PulledAt: t0.Add(-5 * time.Minute), LastUsage: t0.Add(-time.Minute), FailedAt: t0.Add(-time.Minute)},
	}}
	got := Due(t0, []string{"session_N", "session_M", "session_H"}, st)
	if want := []string{"session_M"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Due = %v, want %v", got, want)
	}
}

func TestDueOrder(t *testing.T) {
	st := &State{Sessions: map[string]*Entry{
		"session_W1": entry(40*time.Minute, 2*time.Hour),
		"session_W2": entry(50*time.Minute, 2*time.Hour),
		"session_H":  entry(3*time.Minute, time.Minute),
		"session_C":  entry(25*time.Hour, 3*24*time.Hour),
	}}
	list := []string{"session_C", "session_N1", "session_W1", "session_W2", "session_H", "session_N2"}
	got := Due(t0, list, st)
	want := []string{"session_N2", "session_N1", "session_H", "session_W2", "session_W1", "session_C"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Due = %v, want %v", got, want)
	}
}

// fakeCLI writes a transcript into the project folder like the real pull does
// and returns the JSON result.
type fakeCLI struct {
	projectDir string
	n          int
	turns      int
	fail       bool
	calls      []string
}

func (f *fakeCLI) run(_ context.Context, _, _ string, args ...string) ([]byte, error) {
	if len(args) == 2 && args[0] == "auth" {
		return []byte(`{"loggedIn":true,"orgId":"` + orgA + `"}`), nil
	}
	id := args[1]
	f.calls = append(f.calls, id)
	if f.fail {
		return []byte("Error: not logged in"), errors.New("exit status 1")
	}
	f.n++
	sid := fmt.Sprintf("local-%d", f.n)
	body := `{"type":"assistant","timestamp":"2026-11-17T11:59:00Z","requestId":"r1","message":{"usage":{"input_tokens":1}}}` + "\n"
	if err := os.WriteFile(filepath.Join(f.projectDir, sid+".jsonl"), []byte(body), 0o644); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(f.projectDir, sid), 0o755); err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf(`{"is_error":false,"num_turns":%d,"total_cost_usd":0,"session_id":%q}`, f.turns, sid)), nil
}

func newTestRunner(t *testing.T) (*Runner, *fakeCLI) {
	t.Helper()
	base := t.TempDir()
	r := &Runner{
		Exe:        "claude.exe",
		PullDir:    filepath.Join(base, "cloud-pull"),
		ProjectDir: filepath.Join(base, "projects", "pull"),
		StatePath:  filepath.Join(base, "state.json"),
		Now:        func() time.Time { return t0 },
	}
	if err := os.MkdirAll(r.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	f := &fakeCLI{projectDir: r.ProjectDir}
	r.Run = f.run
	return r, f
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestPullOneAndCleanup(t *testing.T) {
	r, _ := newTestRunner(t)
	ctx := context.Background()
	if err := r.PullOne(ctx, "session_A"); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(r.ProjectDir, "local-1.jsonl")
	if !exists(first) || !exists(r.PullDir) {
		t.Fatal("first pull file or pull folder missing")
	}
	if err := r.PullOne(ctx, "session_A"); err != nil {
		t.Fatal(err)
	}
	if exists(first) || exists(filepath.Join(r.ProjectDir, "local-1")) {
		t.Error("old pull file or folder not deleted")
	}
	st, _ := LoadState(r.StatePath)
	e := st.Sessions["session_A"]
	if e == nil || e.SessionID != "local-2" || !e.PulledAt.Equal(t0) ||
		!e.LastUsage.Equal(time.Date(2026, 11, 17, 11, 59, 0, 0, time.UTC)) {
		t.Fatalf("state entry = %+v", e)
	}
}

func TestCleanupRefusesOutsidePath(t *testing.T) {
	r, _ := newTestRunner(t)
	outside := filepath.Join(filepath.Dir(r.ProjectDir), "keep.jsonl")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := &State{Sessions: map[string]*Entry{"session_A": {SessionID: `..\keep`, Path: outside, PulledAt: t0.Add(-time.Hour)}}}
	if err := st.Save(r.StatePath); err != nil {
		t.Fatal(err)
	}
	if err := r.PullOne(context.Background(), "session_A"); err != nil {
		t.Fatal(err)
	}
	if !exists(outside) {
		t.Fatal("clean-up deleted a file outside the project folder")
	}
}

func TestPullOneCostGuard(t *testing.T) {
	r, f := newTestRunner(t)
	f.turns = 1
	if err := r.PullOne(context.Background(), "session_A"); !errors.Is(err, ErrCost) {
		t.Fatalf("err = %v, want ErrCost", err)
	}
	st, _ := LoadState(r.StatePath)
	if st.Disabled == "" {
		t.Fatal("cost guard did not persist disabled")
	}
	f.turns = 0
	if _, err := r.Tick(context.Background(), []string{"session_B"}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("Tick after disable: err = %v, want ErrDisabled", err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("CLI calls = %v, want only the first", f.calls)
	}
}

func TestPullOneFailureIsNotDisable(t *testing.T) {
	r, f := newTestRunner(t)
	f.fail = true
	err := r.PullOne(context.Background(), "session_A")
	if err == nil || errors.Is(err, ErrCost) {
		t.Fatalf("err = %v, want a plain failure", err)
	}
	st, _ := LoadState(r.StatePath)
	e := st.Sessions["session_A"]
	if st.Disabled != "" || e == nil || e.LastError == "" || !e.FailedAt.Equal(t0) || st.LastError == "" {
		t.Fatalf("state = %+v, entry %+v", st, e)
	}
}

func TestTickOnePullPerTick(t *testing.T) {
	r, f := newTestRunner(t)
	got, err := r.Tick(context.Background(), []string{"session_A", "session_B"})
	if err != nil || got != "session_B" {
		t.Fatalf("Tick = %q, %v; want session_B (newest first)", got, err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("CLI calls = %v, want one", f.calls)
	}
	// Nothing due: no pull.
	r2, f2 := newTestRunner(t)
	if got, err := r2.Tick(context.Background(), nil); err != nil || got != "" || len(f2.calls) != 0 {
		t.Fatalf("empty list: %q, %v, calls %v", got, err, f2.calls)
	}
}

// Loop: reads the org, then the list for that org, and pulls the newest id.
func TestLoopFirstTick(t *testing.T) {
	r, f := newTestRunner(t)
	root := t.TempDir()
	writeList(t, root, "acc1", orgA, `{"entries":[{"sessionId":"session_A"},{"sessionId":"session_B"}]}`)
	r.Roots = func() []string { return []string{root} }
	ctx, cancel := context.WithCancel(context.Background())
	inner := r.Run
	r.Run = func(c context.Context, exe, dir string, args ...string) ([]byte, error) {
		out, err := inner(c, exe, dir, args...)
		if args[0] == "--teleport" {
			cancel()
		}
		return out, err
	}
	if err := r.Loop(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Loop = %v, want context.Canceled", err)
	}
	if want := []string{"session_B"}; !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("CLI calls = %v, want %v", f.calls, want)
	}
}

// One real pull of the hub session through the runner, twice. Runs only with
// BURNMON_LIVE_PULL=1 because it starts the real Claude Code CLI.
func TestLivePullHubSession(t *testing.T) {
	if os.Getenv("BURNMON_LIVE_PULL") != "1" {
		t.Skip("set BURNMON_LIVE_PULL=1 to run a real pull")
	}
	r, err := NewRunner()
	if err != nil {
		t.Fatal(err)
	}
	r.StatePath = filepath.Join(t.TempDir(), "state.json")
	ctx := context.Background()
	const hub = "session_0131Y29YSSRdAPR2VmEG1ET3"
	if err := r.PullOne(ctx, hub); err != nil {
		t.Fatal(err)
	}
	st, _ := LoadState(r.StatePath)
	first := *st.Sessions[hub]
	if first.LastUsage.IsZero() || !exists(first.Path) {
		t.Fatalf("first pull: %+v", first)
	}
	if err := r.PullOne(ctx, hub); err != nil {
		t.Fatal(err)
	}
	st, _ = LoadState(r.StatePath)
	if exists(first.Path) || st.Sessions[hub].Path == first.Path || !exists(st.Sessions[hub].Path) {
		t.Fatalf("second pull did not replace the first: first %s, now %+v", first.Path, st.Sessions[hub])
	}
	t.Logf("live pull ok: %s, last usage %s", st.Sessions[hub].Path, st.Sessions[hub].LastUsage)
}

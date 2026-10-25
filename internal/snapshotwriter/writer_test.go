package snapshotwriter_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"burnmon/internal/graph"
	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/snapshot"
	"burnmon/internal/snapshotwriter"
	"burnmon/internal/snapshotwriter/writertest"
	"burnmon/internal/store"
	"burnmon/internal/sysmon"
)

func snapshotTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	now := time.Now().UTC()
	if err := st.UpsertEvents([]schema.Event{{
		Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
		SessionID: "s1", RequestID: "r1", At: now.Add(-time.Minute),
		Model: "claude-sonnet-5", Input: 1000, Output: 200,
	}}); err != nil {
		t.Fatal(err)
	}
	return st
}

func newWriter(st *store.Store, path string, procs func() ([]sysmon.ProcessGroupSample, error), gr *snapshotwriter.GraphReaders) *snapshotwriter.Writer {
	cfg, err := pricing.Load("")
	if err != nil {
		panic(err)
	}
	return &snapshotwriter.Writer{Store: st, Path: path, Tick: 5 * time.Second, Config: func() pricing.Config { return cfg }, Procs: procs, Graph: gr}
}

func noProcesses() ([]sysmon.ProcessGroupSample, error) { return nil, nil }

func TestSnapshotOnceWritesSessionFromStore(t *testing.T) {
	st := snapshotTestStore(t)
	path := filepath.Join(t.TempDir(), "snapshot.json")
	procs := func() ([]sysmon.ProcessGroupSample, error) {
		return []sysmon.ProcessGroupSample{{Ts: time.Now(), Harness: sysmon.HarnessClaude, CPUPct: 3}}, nil
	}
	if err := newWriter(st, path, procs, nil).Once(); err != nil {
		t.Fatal(err)
	}
	got, err := snapshot.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sessions) != 1 || got.Sessions[0].ID != "s1" || got.Sessions[0].Tokens == 0 {
		t.Fatalf("session missing: %+v", got.Sessions)
	}
	if got.Sessions[0].State == "" {
		t.Fatalf("state should be filled from the stage rules: %+v", got.Sessions[0])
	}
	if got.TickSeconds != 5 || got.IsStale(time.Now()) {
		t.Fatalf("header wrong or already stale: %+v", got)
	}
	if len(got.Processes) != 1 || len(got.Sources) != 2 || !got.Sources[0].OK || !got.Sources[1].OK {
		t.Fatalf("processes or sources wrong: %+v %+v", got.Processes, got.Sources)
	}
}

func TestSnapshotOnceReportsProcessErrorAndStillWrites(t *testing.T) {
	st := snapshotTestStore(t)
	path := filepath.Join(t.TempDir(), "snapshot.json")
	failing := func() ([]sysmon.ProcessGroupSample, error) { return nil, errors.New("no process list") }
	if err := newWriter(st, path, failing, nil).Once(); err != nil {
		t.Fatal(err)
	}
	got, err := snapshot.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	var sys *snapshot.Source
	for i := range got.Sources {
		if got.Sources[i].Name == "sysmon" {
			sys = &got.Sources[i]
		}
	}
	if sys == nil || sys.OK || sys.Error != "no process list" {
		t.Fatalf("sysmon source should carry the error: %+v", got.Sources)
	}
	if len(got.Sessions) != 1 {
		t.Fatalf("sessions should still be written: %+v", got.Sessions)
	}
}

func TestSnapshotOnceReportsStoreErrorAndStillWrites(t *testing.T) {
	st := snapshotTestStore(t)
	path := filepath.Join(t.TempDir(), "snapshot.json")
	st.Close()
	if err := newWriter(st, path, noProcesses, nil).Once(); err != nil {
		t.Fatal(err)
	}
	got, err := snapshot.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sources[0].Name != "store" || got.Sources[0].OK || got.Sources[0].Error == "" {
		t.Fatalf("store source should carry the error: %+v", got.Sources)
	}
}

// fakeReaders is a graph source set in temp folders: one running Claude Code
// session (pid 4100) in a sprint worktree, the process walk that shows it,
// and a Foreman root with that sprint.
func fakeReaders(t *testing.T, pidFile string) (*snapshotwriter.GraphReaders, string) {
	t.Helper()
	root := t.TempDir()
	wt := filepath.Join(root, "repo", ".worktrees", "sprint-x")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	pidDir := filepath.Join(root, "sessions")
	foreman := filepath.Join(root, "foreman")
	for path, body := range map[string]string{
		filepath.Join(pidDir, "4100.json"): pidFile,
		filepath.Join(foreman, "sprints", "p", "x", "sprint.json"): `{"sprint_id":"p__x__1","slug":"x","status":"running","worktree_path":` +
			strconv.Quote(wt) + `,"updated_at":"2026-10-23T10:00:00Z"}`,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gr := snapshotwriter.NewGraphReaders(root, foreman)
	gr.PidDir = pidDir
	gr.Rows = func() ([]sysmon.ProcRow, time.Time) {
		return []sysmon.ProcRow{
			{PID: 4100, PPID: 1, Name: "claude.exe", CreateTime: 777, Harness: sysmon.HarnessClaude},
			{PID: 4200, PPID: 4100, Name: "bash.exe", CreateTime: 778, Harness: sysmon.HarnessClaude},
		}, time.Now()
	}
	gr.Listen = func() ([]graph.Listen, error) { return nil, nil }
	return gr, wt
}

func findEdge(s snapshot.Snapshot, from, to, kind string) bool {
	for _, e := range s.Edges {
		if e.From == from && e.To == to && e.Kind == kind {
			return true
		}
	}
	return false
}

func sourceNamed(s snapshot.Snapshot, name string) *snapshot.Source {
	for i := range s.Sources {
		if s.Sources[i].Name == name {
			return &s.Sources[i]
		}
	}
	return nil
}

func TestSnapshotOnceWritesGraphAndRecordsLiveEdges(t *testing.T) {
	st := snapshotTestStore(t)
	gr, wt := fakeReaders(t, `{"pid":4100,"sessionId":"s2","cwd":"x","procStart":"777","status":"busy"}`)
	if err := st.UpsertEvents([]schema.Event{{
		Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "s2", RequestID: "r1",
		At: time.Now().UTC().Add(-time.Minute), Model: "claude-sonnet-5", Project: wt, Input: 10, Output: 2,
	}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := newWriter(st, path, noProcesses, gr).Once(); err != nil {
		t.Fatal(err)
	}
	got, err := snapshot.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"store", "sysmon", "claude_pids", "mcp_config", "net_tables", "git", "foreman"} {
		if s := sourceNamed(got, name); s == nil || !s.OK {
			t.Errorf("source %s missing or not ok: %+v", name, s)
		}
	}
	s2, proc := "session:anthropic:s2", "process:4100:777"
	if !findEdge(got, s2, proc, "runs_in") || !findEdge(got, proc, "process:4200:778", "started") || !findEdge(got, s2, "sprint:p__x__1", "works_for") {
		t.Fatalf("graph edges missing: %+v", got.Edges)
	}
	_, recorded, err := st.GraphBetween(time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) == 0 {
		t.Fatal("live-only edges were not recorded in the store")
	}
}

// Condition from the G4 review: a pid file in a format BurnMon does not know
// marks claude_pids not ok and drops its edges; the tick is still written.
func TestSnapshotOncePidFormatChangeDropsEdges(t *testing.T) {
	st := snapshotTestStore(t)
	gr, _ := fakeReaders(t, `{"pid":4100,"session":"renamed","procStart":777}`)
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := newWriter(st, path, noProcesses, gr).Once(); err != nil {
		t.Fatal(err)
	}
	got, err := snapshot.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	src := sourceNamed(got, "claude_pids")
	if src == nil || src.OK || src.Error == "" {
		t.Fatalf("claude_pids should be not ok with an error: %+v", src)
	}
	for _, e := range got.Edges {
		if e.Source == "claude_pids" {
			t.Fatalf("edge from a pid file in an unknown format: %+v", e)
		}
	}
	if len(got.Sessions) != 1 {
		t.Fatalf("sessions should still be written: %+v", got.Sessions)
	}
}

// No Foreman root configured: no foreman source, no error.
func TestSnapshotOnceWithoutForeman(t *testing.T) {
	st := snapshotTestStore(t)
	gr, _ := fakeReaders(t, `{"pid":4100,"sessionId":"s2","procStart":"777"}`)
	gr.ForemanRoot = ""
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := newWriter(st, path, noProcesses, gr).Once(); err != nil {
		t.Fatal(err)
	}
	got, _ := snapshot.Read(path)
	if sourceNamed(got, "foreman") != nil {
		t.Fatalf("foreman source listed without a configured root: %+v", got.Sources)
	}
}

// The same inputs and the same clock give the same snapshot bytes, so a
// byte comparison between two writers is a fair test.
func TestOnceIsDeterministicForFixedInputs(t *testing.T) {
	a, b := writertest.Golden(t), writertest.Golden(t)
	if len(a) == 0 || !bytes.Equal(a, b) {
		t.Fatalf("snapshot bytes differ between two runs:\n%s\n%s", a, b)
	}
}

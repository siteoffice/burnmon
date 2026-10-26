package snapshotwriter_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/snapshot"
	"burnmon/internal/snapshotwriter"
	"burnmon/internal/waits"
)

// Fake runners only: these tests never start the real claude binary.

const twoEntries = `[{"cwd":"C:\\example\\a","kind":"background","startedAt":1761213600000,"id":"ab12","state":"blocked","pid":42,"status":"waiting","waitingFor":"permission prompt","sessionId":"s1","name":"ignored"},{"cwd":"C:\\example\\b","kind":"interactive","startedAt":1761213600000,"pid":7,"status":"busy"}]`

func runnerOf(out string, err error) func(ctx context.Context) ([]byte, error) {
	return func(ctx context.Context) ([]byte, error) { return []byte(out), err }
}

func onceRead(t *testing.T, w *snapshotwriter.Writer) snapshot.Snapshot {
	t.Helper()
	if err := w.Once(); err != nil {
		t.Fatal(err)
	}
	got, err := snapshot.Read(w.Path)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func findItem(items []waits.Item, kind string) *waits.Item {
	for i := range items {
		if items[i].Kind == kind {
			return &items[i]
		}
	}
	return nil
}

// writeForemanGate writes an open-gate flow under a Foreman root.
func writeForemanGate(t *testing.T, root string) {
	t.Helper()
	p := filepath.Join(root, "flows", "p", "example-flow", "flow.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"flow_id":"p__f__1","slug":"example-flow","project":"example","status":"active","updated_at":"2026-10-25T10:00:00Z",` +
		`"spec_path":"C:\\example\\specs\\a.md","gate":{"id":"G4","status":"awaiting-owner","opened_at":"2026-10-25T11:00:00Z"}}`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAgentViewItemsInSnapshot(t *testing.T) {
	st := snapshotTestStore(t)
	gr, _ := fakeReaders(t, `{"pid":4100,"sessionId":"s2","procStart":"777"}`)
	gr.AgentView = runnerOf(twoEntries, nil)
	gr.PollAgentViewNow(context.Background())
	w := newWriter(st, filepath.Join(t.TempDir(), "snapshot.json"), noProcesses, gr)
	got := onceRead(t, w)
	p := findItem(got.WaitItems, waits.KindPrompt)
	if p == nil || p.Source != "agent_view" || p.Detail != "permission prompt" {
		t.Fatalf("prompt item missing: %+v", got.WaitItems)
	}
	if s := sourceNamed(got, "agent_view"); s == nil || !s.OK {
		t.Fatalf("agent_view source: %+v", s)
	}
}

func TestAgentViewErrorKeepsForemanItems(t *testing.T) {
	st := snapshotTestStore(t)
	gr, _ := fakeReaders(t, `{"pid":4100,"sessionId":"s2","procStart":"777"}`)
	writeForemanGate(t, gr.ForemanRoot)
	gr.AgentView = runnerOf("boom", nil)
	gr.PollAgentViewNow(context.Background())
	got := onceRead(t, newWriter(st, filepath.Join(t.TempDir(), "snapshot.json"), noProcesses, gr))
	s := sourceNamed(got, "agent_view")
	if s == nil || s.OK || s.Error == "" {
		t.Fatalf("agent_view should be not ok with an error: %+v", s)
	}
	if findItem(got.WaitItems, waits.KindGate) == nil {
		t.Fatalf("foreman gate lost: %+v", got.WaitItems)
	}
}

func TestAgentViewRunErrorIsReported(t *testing.T) {
	st := snapshotTestStore(t)
	gr, _ := fakeReaders(t, `{"pid":4100,"sessionId":"s2","procStart":"777"}`)
	gr.AgentView = runnerOf("", errors.New("exit status 1: nope"))
	gr.PollAgentViewNow(context.Background())
	got := onceRead(t, newWriter(st, filepath.Join(t.TempDir(), "snapshot.json"), noProcesses, gr))
	if s := sourceNamed(got, "agent_view"); s == nil || s.OK || s.Error != "exit status 1: nope" {
		t.Fatalf("agent_view: %+v", s)
	}
}

func TestAgentViewHangDoesNotBlockTick(t *testing.T) {
	st := snapshotTestStore(t)
	gr, _ := fakeReaders(t, `{"pid":4100,"sessionId":"s2","procStart":"777"}`)
	starts := make(chan struct{}, 10)
	gr.AgentView = func(ctx context.Context) ([]byte, error) {
		starts <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	w := newWriter(st, filepath.Join(t.TempDir(), "snapshot.json"), noProcesses, gr)
	for i := 0; i < 2; i++ {
		done := make(chan error, 1)
		go func() { done <- w.Once() }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatalf("tick %d blocked on the agent view runner", i)
		}
	}
	time.Sleep(50 * time.Millisecond)
	if n := len(starts); n != 1 {
		t.Fatalf("runner started %d times, want 1 in flight", n)
	}
}

func TestAgentViewSinceIsFirstSeen(t *testing.T) {
	st := snapshotTestStore(t)
	gr, _ := fakeReaders(t, `{"pid":4100,"sessionId":"s2","procStart":"777"}`)
	t0 := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	clock := t0
	gr.Now = func() time.Time { return clock }
	out := twoEntries
	gr.AgentView = func(ctx context.Context) ([]byte, error) { return []byte(out), nil }
	w := newWriter(st, filepath.Join(t.TempDir(), "snapshot.json"), noProcesses, gr)
	w.Now = func() time.Time { return clock }
	since := func() string {
		gr.PollAgentViewNow(context.Background())
		it := findItem(onceRead(t, w).WaitItems, waits.KindPrompt)
		if it == nil {
			t.Fatal("no prompt item")
		}
		return it.Since
	}
	if got := since(); got != t0.Format(time.RFC3339) {
		t.Fatalf("first since %s", got)
	}
	clock = t0.Add(30 * time.Second)
	if got := since(); got != t0.Format(time.RFC3339) {
		t.Fatalf("second since %s, want first seen", got)
	}
	clock = t0.Add(60 * time.Second)
	out = `[{"cwd":"C:\\example\\a","kind":"background","id":"ab12","state":"working","pid":42,"status":"busy","sessionId":"s1"}]`
	gr.PollAgentViewNow(context.Background())
	if findItem(onceRead(t, w).WaitItems, waits.KindPrompt) != nil {
		t.Fatal("busy entry should not wait")
	}
	clock = t0.Add(90 * time.Second)
	out = twoEntries
	if got := since(); got != clock.Format(time.RFC3339) {
		t.Fatalf("since after a gap %s, want %s", got, clock.Format(time.RFC3339))
	}
}

func TestAgentViewOffWithoutRunner(t *testing.T) {
	st := snapshotTestStore(t)
	gr, _ := fakeReaders(t, `{"pid":4100,"sessionId":"s2","procStart":"777"}`)
	got := onceRead(t, newWriter(st, filepath.Join(t.TempDir(), "snapshot.json"), noProcesses, gr))
	if sourceNamed(got, "agent_view") != nil {
		t.Fatalf("agent_view listed without a runner: %+v", got.Sources)
	}
}

func TestAgentSessionsInSnapshot(t *testing.T) {
	st := snapshotTestStore(t)
	gr, _ := fakeReaders(t, `{"pid":4100,"sessionId":"s2","procStart":"777"}`)
	gr.AgentView = runnerOf(twoEntries, nil)
	gr.PollAgentViewNow(context.Background())
	w := newWriter(st, filepath.Join(t.TempDir(), "snapshot.json"), noProcesses, gr)
	got := onceRead(t, w)
	if len(got.AgentSessions) != 2 {
		t.Fatalf("agent sessions = %+v", got.AgentSessions)
	}
	want := snapshot.AgentSession{Kind: "background", ID: "ab12", State: "blocked", Status: "waiting", WaitingFor: "permission prompt", SessionID: "s1", PID: 42, StartedAt: 1761213600000, Project: "a"}
	if got.AgentSessions[0] != want {
		t.Fatalf("first = %+v, want %+v", got.AgentSessions[0], want)
	}
	if got.AgentSessions[1].Kind != "interactive" || got.AgentSessions[1].ID != "" || got.AgentSessions[1].Project != "b" {
		t.Fatalf("second = %+v", got.AgentSessions[1])
	}

	gr2, _ := fakeReaders(t, `{"pid":4100,"sessionId":"s2","procStart":"777"}`)
	gr2.AgentView = runnerOf("boom", nil)
	gr2.PollAgentViewNow(context.Background())
	w2 := newWriter(st, filepath.Join(t.TempDir(), "snapshot.json"), noProcesses, gr2)
	got2 := onceRead(t, w2)
	if got2.AgentSessions == nil || len(got2.AgentSessions) != 0 {
		t.Fatalf("a failed poll should give an empty array: %+v", got2.AgentSessions)
	}
}

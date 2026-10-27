package snapshotwriter_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/graph"
	"burnmon/internal/schema"
	"burnmon/internal/snapshotwriter"
	"burnmon/internal/store"
	"burnmon/internal/waits"
)

const blockedAndIdle = `[{"kind":"interactive","status":"waiting","waitingFor":"permission prompt","sessionId":"s1","cwd":"C:/example/a"},` +
	`{"kind":"interactive","status":"idle","sessionId":"s2","cwd":"C:/example/b"}]`

// twoLiveSessions adds s2 to the store next to the s1 of snapshotTestStore.
func twoLiveSessions(t *testing.T, st *store.Store, project string) {
	t.Helper()
	if err := st.UpsertEvents([]schema.Event{{
		Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "s2", RequestID: "r2",
		At: time.Now().UTC().Add(-time.Minute), Model: "claude-sonnet-5", Project: project, Input: 10, Output: 2,
	}}); err != nil {
		t.Fatal(err)
	}
}

func TestOnceWritesWaitCounts(t *testing.T) {
	st := snapshotTestStore(t)
	gr, wt := fakeReaders(t, `{"pid":4100,"sessionId":"s2","procStart":"777","status":"busy"}`)
	twoLiveSessions(t, st, wt)
	gr.AgentView = runnerOf(blockedAndIdle, nil)
	gr.PollAgentViewNow(context.Background())
	got := onceRead(t, newWriter(st, filepath.Join(t.TempDir(), "snapshot.json"), noProcesses, gr))
	if got.WaitCounts != (waits.Counts{Blocked: 1, YourTurn: 1}) {
		t.Fatalf("wait_counts = %+v, items %+v", got.WaitCounts, got.WaitItems)
	}
	blocked := 0
	for _, it := range got.WaitItems {
		if it.Group == waits.GroupBlocked {
			blocked++
		}
	}
	if got.WaitCounts.Blocked != blocked || got.WaitCounts != waits.CountOf(got.WaitItems) {
		t.Fatalf("wait_counts %+v does not match the list (%d blocked)", got.WaitCounts, blocked)
	}
}

// Before B6 the agent view reached only the list; now it reaches the graph.
func TestAgentViewReachesGraph(t *testing.T) {
	st := snapshotTestStore(t)
	gr, wt := fakeReaders(t, `{"pid":4100,"sessionId":"s2","procStart":"777","status":"busy"}`)
	twoLiveSessions(t, st, wt)
	gr.AgentView = runnerOf(blockedAndIdle, nil)
	gr.PollAgentViewNow(context.Background())
	got := onceRead(t, newWriter(st, filepath.Join(t.TempDir(), "snapshot.json"), noProcesses, gr))
	if !findEdge(got, "session:anthropic:s1", "person:you", "waits_on") {
		t.Fatalf("blocked session has no waits_on edge: %+v", got.Edges)
	}
	if findEdge(got, "session:anthropic:s2", "person:you", "waits_on") {
		t.Fatalf("idle session has a waits_on edge: %+v", got.Edges)
	}
	for _, n := range got.Nodes {
		if n.ID == "session:anthropic:s1" && n.Attrs["wait"] != "blocked" {
			t.Fatalf("s1 wait attr = %q", n.Attrs["wait"])
		}
		if n.ID == "session:anthropic:s2" && n.Attrs["wait"] != "your_turn" {
			t.Fatalf("s2 wait attr = %q", n.Attrs["wait"])
		}
	}
}

// With no foreman_root: no foreman source, wait_counts present, no Foreman
// item kinds and no error text in any source.
func TestWriterWithoutForemanRoot(t *testing.T) {
	st := snapshotTestStore(t)
	home := t.TempDir()
	gr := snapshotwriter.NewGraphReaders(home, "")
	gr.Listen = func() ([]graph.Listen, error) { return nil, nil }
	path := filepath.Join(t.TempDir(), "snapshot.json")
	got := onceRead(t, newWriter(st, path, noProcesses, gr))
	if sourceNamed(got, "foreman") != nil {
		t.Fatalf("foreman source listed without a root: %+v", got.Sources)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["wait_counts"]) == "" {
		t.Fatal("wait_counts missing from the file")
	}
	for _, it := range got.WaitItems {
		switch it.Kind {
		case waits.KindGate, waits.KindSprint, waits.KindFailedCheck, waits.KindCloseOut:
			t.Fatalf("Foreman item without a foreman_root: %+v", it)
		}
	}
	for _, s := range got.Sources {
		if s.Error != "" {
			t.Fatalf("source %s carries error text: %q", s.Name, s.Error)
		}
	}
}

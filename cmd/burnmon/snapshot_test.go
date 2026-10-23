package main

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/snapshot"
	"burnmon/internal/store"
	"burnmon/internal/sysmon"
)

func snapshotTestApp(t *testing.T) (*app, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg, err := pricing.Load("")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := st.UpsertEvents([]schema.Event{{
		Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
		SessionID: "s1", RequestID: "r1", At: now.Add(-time.Minute),
		Model: "claude-sonnet-5", Input: 1000, Output: 200,
	}}); err != nil {
		t.Fatal(err)
	}
	a := &app{cfg: cfg}
	a.cache.Store = st
	return a, st
}

func noProcesses() ([]sysmon.ProcessGroupSample, error) { return nil, nil }

func TestSnapshotOnceWritesSessionFromStore(t *testing.T) {
	a, st := snapshotTestApp(t)
	path := filepath.Join(t.TempDir(), "snapshot.json")
	procs := func() ([]sysmon.ProcessGroupSample, error) {
		return []sysmon.ProcessGroupSample{{Ts: time.Now(), Harness: sysmon.HarnessClaude, CPUPct: 3}}, nil
	}
	if err := a.snapshotOnce(st, path, 5*time.Second, procs); err != nil {
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
	a, st := snapshotTestApp(t)
	path := filepath.Join(t.TempDir(), "snapshot.json")
	failing := func() ([]sysmon.ProcessGroupSample, error) { return nil, errors.New("no process list") }
	if err := a.snapshotOnce(st, path, 5*time.Second, failing); err != nil {
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
	a, st := snapshotTestApp(t)
	path := filepath.Join(t.TempDir(), "snapshot.json")
	st.Close()
	if err := a.snapshotOnce(st, path, 5*time.Second, noProcesses); err != nil {
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

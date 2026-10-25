// Package writertest holds the fixture and golden bytes that the snapshot
// writer's own tests and the BurnMon app's tests share, so both prove the
// same snapshot.
package writertest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/snapshotwriter"
	"burnmon/internal/store"
	"burnmon/internal/sysmon"
)

// FixedNow is the clock every fixture runs on.
var FixedNow = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

// Store opens a temp store with one claude-code event "s1" a minute before
// FixedNow.
func Store(t testing.TB) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.UpsertEvents([]schema.Event{{
		Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
		SessionID: "s1", RequestID: "r1", At: FixedNow.Add(-time.Minute),
		Model: "claude-sonnet-5", Input: 1000, Output: 200,
	}}); err != nil {
		t.Fatal(err)
	}
	return st
}

// Fake pins w to FixedNow, one Claude process group and no graph sources.
func Fake(w *snapshotwriter.Writer) {
	w.Now = func() time.Time { return FixedNow }
	w.Procs = func() ([]sysmon.ProcessGroupSample, error) {
		return []sysmon.ProcessGroupSample{{Ts: FixedNow, Harness: sysmon.HarnessClaude, CPUPct: 3}}, nil
	}
	w.Graph = nil
}

// Golden is the snapshot file a default writer produces over a fresh Store.
func Golden(t testing.TB) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snapshot.json")
	w := &snapshotwriter.Writer{Store: Store(t), Path: path, Tick: snapshotwriter.Tick, Config: pricing.Defaults}
	Fake(w)
	if err := w.Once(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

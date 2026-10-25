// Package exporttest holds the fixture and golden bytes that the export
// tests of the CLI and both apps share. It is a non-test file because tests
// in other packages import it.
package exporttest

import (
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/export"
	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/store"
)

// FixedNow is the exported_at every golden uses, so the bytes are stable.
var FixedNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

// Store opens a temp store holding four events over three local days, two
// owners (wilco, other) and two vendors.
func Store(t testing.TB) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	at := func(day int) time.Time { return time.Date(2026, 10, day, 12, 0, 0, 0, time.Local).UTC() }
	events := []schema.Event{
		{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "s1", RequestID: "r1",
			At: at(18), Model: "claude-sonnet-5", Owner: "wilco", Client: "burnmon", Input: 100, Output: 10},
		{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "s1", RequestID: "r2",
			At: at(19), Model: "claude-sonnet-5", Owner: "other", Client: "unassigned", Input: 200, Output: 20},
		{Vendor: "openai", Agent: "codex", Surface: "cli", SessionID: "s2", RequestID: "r3",
			At: at(19), Model: "gpt-5.6-terra", Owner: "wilco", Client: "burnmon", Input: 300, Output: 30},
		{Vendor: "openai", Agent: "codex", Surface: "cli", SessionID: "s2", RequestID: "r4",
			At: at(20), Model: "gpt-5.6-terra", Owner: "other", Client: "unassigned", Input: 400, Output: 40},
	}
	if err := st.UpsertEvents(events); err != nil {
		t.Fatal(err)
	}
	return st
}

// Golden returns the bytes an export of st under f must produce:
// Encode(Build(Select(AllEvents, f))) at FixedNow.
func Golden(t testing.TB, st *store.Store, cfg *pricing.Config, f export.Filter, label string) []byte {
	t.Helper()
	events, err := st.AllEvents()
	if err != nil {
		t.Fatal(err)
	}
	b, err := export.Encode(export.Build(export.Select(events, f), cfg, label, FixedNow))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"burnmon/internal/export"
	"burnmon/internal/export/exporttest"
	"burnmon/internal/live"
	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/store"
)

// TestParseSinceDuration guards `tools -since`'s day-unit parsing, the one
// bit time.ParseDuration cannot do on its own.
func TestParseSinceDuration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"30d", 30 * 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"24h", 24 * time.Hour},
		{"90m", 90 * time.Minute},
	}
	for _, c := range cases {
		got, err := parseSinceDuration(c.in)
		if err != nil {
			t.Fatalf("parseSinceDuration(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("parseSinceDuration(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	if _, err := parseSinceDuration("bad"); err == nil {
		t.Fatal("parseSinceDuration(\"bad\") = nil error, want one")
	}
}

// TestLiveQueryPathStaysBoundedOnLargeStore guards S3: `burnmon-cli live` must
// use the same windowed store.EventsSince + live.ApplySessionTotals path the
// app's own live poll uses (internal/live's F1 fix), not store.AllEvents,
// which re-groups the whole table in memory on every call (F1 measured
// burnmon.exe at 1,886 MB and thrashing before that fix, SESSION_LOG.md).
// This exercises exactly the query path runLive runs after its Collect call,
// against a store holding many old events plus a couple of recent ones,
// and asserts both that EventsSince returns only the recent window (not
// proportional to the store's total size) and that the whole pass's own
// heap growth stays well under 50 MB.
func TestLiveQueryPathStaysBoundedOnLargeStore(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	now := time.Now().UTC()
	const oldEventCount = 50_000
	old := make([]schema.Event, 0, oldEventCount)
	base := now.Add(-365 * 24 * time.Hour)
	for i := 0; i < oldEventCount; i++ {
		old = append(old, schema.Event{
			Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
			SessionID: "old-session", RequestID: "old-" + strconv.Itoa(i), Model: "claude-x",
			At: base.Add(time.Duration(i) * time.Second), Input: 100, Output: 10,
		})
	}
	// Batch the insert the same way dataset.ingest does, so this is not
	// itself an unrealistic single giant transaction.
	const batch = 1000
	for start := 0; start < len(old); start += batch {
		end := start + batch
		if end > len(old) {
			end = len(old)
		}
		if err := st.UpsertEvents(old[start:end]); err != nil {
			t.Fatal(err)
		}
	}

	recent := []schema.Event{
		{Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
			SessionID: "running-1", RequestID: "r1", Model: "claude-x",
			At: now.Add(-90 * time.Second), Input: 500, Output: 50},
		{Vendor: "openai", Agent: "codex", Surface: "cli",
			SessionID: "running-2", RequestID: "r2", Model: "gpt-5.6-terra",
			At: now.Add(-10 * time.Second), Input: 800, Output: 80},
	}
	if err := st.UpsertEvents(recent); err != nil {
		t.Fatal(err)
	}

	cfg, err := pricing.Load("")
	if err != nil {
		t.Fatal(err)
	}

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	events, err := st.EventsSince(now.Add(-live.ChartWindow))
	if err != nil {
		t.Fatal(err)
	}
	snap := live.BuildSnapshot(events, &cfg, now)
	if err := live.ApplySessionTotals(snap.Sessions, st, &cfg); err != nil {
		t.Fatal(err)
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	if len(events) >= oldEventCount {
		t.Fatalf("EventsSince returned %d events, want far fewer than the %d old ones "+
			"(the query is not windowed)", len(events), oldEventCount)
	}
	if len(snap.Sessions) != 2 {
		t.Fatalf("got %d running sessions, want 2 (both recent events are within the running window)", len(snap.Sessions))
	}

	const memLimit = 50 * 1024 * 1024
	grew := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	if grew > memLimit {
		t.Fatalf("live query path grew heap by %d bytes, want under %d (50 MB) on a %d-event store",
			grew, memLimit, oldEventCount+len(recent))
	}
}

// TestWriteExportMatchesGolden pins the CLI's export bytes to the shared
// export path: the file writeExport leaves is exactly Encode(Build(Select())).
func TestWriteExportMatchesGolden(t *testing.T) {
	st := exporttest.Store(t)
	cfg := pricing.Defaults()
	f := export.Filter{Owners: []string{"wilco"}}
	out := filepath.Join(t.TempDir(), "export.json")
	if _, err := writeExport(st, &cfg, f, "dev-1", out, exporttest.FixedNow); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if want := exporttest.Golden(t, st, &cfg, f, "dev-1"); !bytes.Equal(got, want) {
		t.Fatalf("export file differs from the golden bytes")
	}
}

// TestExportValidateText pins the CLI's missing-owner words, which scripts and
// docs quote: the shared export.ErrOwnerRequired text is for the apps.
func TestExportValidateText(t *testing.T) {
	want := "export: owner rules are configured; --owner is required " +
		"(repeat --owner for more than one), so a ClientA row never leaves by accident"
	if got := exportValidateText(export.ErrOwnerRequired); got != want {
		t.Fatalf("missing-owner text = %q, want %q", got, want)
	}
}

// TestExportFailureText pins the prefixes the CLI printed before the export
// refactor: "could not write" only for the file write, "internal error:" for
// reading events and encoding.
func TestExportFailureText(t *testing.T) {
	boom := errors.New("boom")
	if got, want := exportFailureText(&exportWriteError{boom}, "x.json"), "could not write x.json : boom"; got != want {
		t.Fatalf("write failure = %q, want %q", got, want)
	}
	if got, want := exportFailureText(boom, "x.json"), "internal error: boom"; got != want {
		t.Fatalf("other failure = %q, want %q", got, want)
	}
}

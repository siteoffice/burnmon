package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"burnmon/internal/export"
	"burnmon/internal/export/exporttest"
	"burnmon/internal/graph"
	"burnmon/internal/live"
	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/store"
	"burnmon/internal/waits"
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

// TestWaitsReportCounts: the report lists each item on one line and the last
// line counts gates, waiting sprints, failed checks (once per id) and
// finished sprints.
func TestWaitsReportCounts(t *testing.T) {
	fc := waits.Item{Kind: waits.KindFailedCheck, ID: "check:s:verify", Title: "capped", Detail: "verify fail: x", AgeSeconds: 3600}
	wait := []waits.Item{
		{Kind: waits.KindGate, ID: "gate:a:G4", Title: "one", Detail: "G4", AgeSeconds: 60, SpecPath: `C:\example\a.md`},
		{Kind: waits.KindGate, ID: "gate:b:G4", Title: "two", Detail: "G4", AgeSeconds: 120},
		{Kind: waits.KindSprint, ID: "sprint:s", Title: "sp", Detail: "1 task parked", AgeSeconds: 600},
		fc,
	}
	outs := []waits.Item{fc, {Kind: waits.KindFinished, ID: "sprint:d", Title: "done", Detail: "test pass, verify -", AgeSeconds: 5000}}
	got := waitsReport(wait, outs)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if last := lines[len(lines)-1]; last != "open gates: 2, waiting sprints: 1, failed checks: 1, finished last 24 h: 1" {
		t.Fatalf("counts line = %q", last)
	}
	for _, id := range []string{"one", "two", "sp", "capped", "done"} {
		n := 0
		for _, l := range lines {
			if strings.Contains(l, "  "+id+"  ") {
				n++
			}
		}
		if n == 0 {
			t.Errorf("no line for %q in:\n%s", id, got)
		}
	}
	if !strings.Contains(got, `spec=C:\example\a.md`) || !strings.Contains(got, "spec=-") || !strings.Contains(got, "result=-") {
		t.Errorf("spec/result columns wrong:\n%s", got)
	}
	if !strings.Contains(got, "Waits on you") || !strings.Contains(got, "Outcomes, last 24 h") {
		t.Errorf("headings missing:\n%s", got)
	}
}

func waitsForemanRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "flows", "p", "f")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"flow_id":"f-1","slug":"example-flow","project":"example","status":"awaiting-owner","gate":{"id":"G4","status":"awaiting-owner","opened_at":"2026-10-23T08:00:00Z"},"updated_at":"2026-10-23T09:00:00Z"}`
	if err := os.WriteFile(filepath.Join(dir, "flow.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestRunWaitsReadsForemanRoot(t *testing.T) {
	var buf bytes.Buffer
	if code := runWaitsTo(&buf, []string{"-foreman", waitsForemanRoot(t)}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(buf.String(), "open gates: 1") {
		t.Fatalf("output:\n%s", buf.String())
	}
}

// TestRunWaitsWithRootGolden pins the text output of a run with a Foreman
// root, at a fixed clock, so a change for the no-root path cannot alter it.
func TestRunWaitsWithRootGolden(t *testing.T) {
	old := waitsNow
	waitsNow = func() time.Time { return time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { waitsNow = old })
	var buf bytes.Buffer
	if code := runWaitsTo(&buf, []string{"-foreman", waitsForemanRoot(t)}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "waits_foreman.golden"))
	if err != nil {
		t.Fatalf("no golden; output was:\n%s", buf.String())
	}
	if got := strings.ReplaceAll(buf.String(), "\r\n", "\n"); got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Fatalf("output differs from golden:\n%s", got)
	}
}

// noRootConfig writes an empty config, so no foreman_root is set.
func noRootConfig(t *testing.T) string {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "burnmon.json")
	if err := os.WriteFile(cfg, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// noRootSnapshot writes a snapshot file with one your_turn wait item.
func noRootSnapshot(t *testing.T) string {
	t.Helper()
	body := `{"schema_version":1,"wait_items":[{"kind":"session","id":"s-1","title":"example-session","detail":"asks a question","since":"2026-10-23T09:00:00Z","age_seconds":3600,"source":"session","group":"your_turn","holds":1}]}`
	p := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunWaitsNoRootListsSessions(t *testing.T) {
	var buf bytes.Buffer
	code := runWaitsTo(&buf, []string{"-config", noRootConfig(t), "-snapshot", noRootSnapshot(t)})
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	got := buf.String()
	want := "Waits on you\nsession  example-session  asks a question  1h  spec=-  result=-\n"
	if got != want {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(got, "open gates:") || strings.Contains(got, "Outcomes") {
		t.Fatalf("Foreman wording without a root:\n%s", got)
	}
}

// staleSnapshot writes a snapshot with a written_at and a tick, for the
// stale-file warning tests.
func staleSnapshot(t *testing.T, writtenAt string) string {
	t.Helper()
	body := `{"schema_version":1,"written_at":"` + writtenAt + `","tick_seconds":5,"wait_items":[]}`
	p := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunWaitsNoRootStaleSnapshotWarns(t *testing.T) {
	old, oldErr := waitsNow, waitsStderr
	waitsNow = func() time.Time { return time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC) }
	var errBuf bytes.Buffer
	waitsStderr = &errBuf
	t.Cleanup(func() { waitsNow, waitsStderr = old, oldErr })
	var buf bytes.Buffer
	code := runWaitsTo(&buf, []string{"-config", noRootConfig(t), "-snapshot", staleSnapshot(t, "2026-10-23T09:00:00Z")})
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if !strings.Contains(errBuf.String(), "stale") {
		t.Fatalf("no stale warning on stderr: %q", errBuf.String())
	}
	if buf.String() != "Waits on you\n" {
		t.Fatalf("stdout changed: %q", buf.String())
	}
}

func TestRunWaitsNoRootFreshSnapshotQuiet(t *testing.T) {
	old, oldErr := waitsNow, waitsStderr
	waitsNow = func() time.Time { return time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC) }
	var errBuf bytes.Buffer
	waitsStderr = &errBuf
	t.Cleanup(func() { waitsNow, waitsStderr = old, oldErr })
	var buf bytes.Buffer
	if code := runWaitsTo(&buf, []string{"-config", noRootConfig(t), "-snapshot", staleSnapshot(t, "2026-10-23T09:59:55Z")}); code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if errBuf.Len() != 0 {
		t.Fatalf("warning on a fresh snapshot: %q", errBuf.String())
	}
}

func TestRunWaitsNoRootNoSnapshot(t *testing.T) {
	var buf bytes.Buffer
	missing := filepath.Join(t.TempDir(), "snapshot.json")
	code := runWaitsTo(&buf, []string{"-config", noRootConfig(t), "-snapshot", missing})
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if buf.String() != "Waits on you\n" {
		t.Fatalf("output:\n%s", buf.String())
	}
}

func TestRunWaitsNoRootJSON(t *testing.T) {
	var buf bytes.Buffer
	code := runWaitsTo(&buf, []string{"-json", "-config", noRootConfig(t), "-snapshot", noRootSnapshot(t)})
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out["outcome_items"]; ok || len(out) != 1 {
		t.Fatalf("want wait_items only, got %s", buf.String())
	}
	var items []waits.Item
	if err := json.Unmarshal(out["wait_items"], &items); err != nil || len(items) != 1 || items[0].ID != "s-1" {
		t.Fatalf("items %+v, err %v", items, err)
	}
}

func TestRunWaitsBadRootStillFails(t *testing.T) {
	var buf bytes.Buffer
	missing := filepath.Join(t.TempDir(), "no-such-foreman")
	if code := runWaitsTo(&buf, []string{"-config", noRootConfig(t), "-foreman", missing}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
}

func TestRunWaitsJSON(t *testing.T) {
	var buf bytes.Buffer
	if code := runWaitsTo(&buf, []string{"-json", "-foreman", waitsForemanRoot(t)}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	var out struct {
		WaitItems    []waits.Item `json:"wait_items"`
		OutcomeItems []waits.Item `json:"outcome_items"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.WaitItems) != 1 || out.OutcomeItems == nil {
		t.Fatalf("got %+v", out)
	}
}

// TestWaitsReportClosedSprintCounts: a capped sprint at phase done with a
// failed test inside 24 h gives no wait item. Its one outcome row stays
// inside 24 h, so the report still counts that failed check once.
func TestWaitsReportClosedSprintCounts(t *testing.T) {
	at := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	sp := graph.Sprint{ID: "s-closed", Slug: "closed", Status: "capped", Phase: "done", UpdatedAt: at.Add(-time.Hour),
		Test: &graph.Check{Failed: true, Label: "test exit 1", At: at.Add(-time.Hour)}}
	wait, outcomes := waits.Build(waits.Input{Now: at, Sprints: []graph.Sprint{sp}})
	for _, it := range wait {
		if it.Kind == waits.KindFailedCheck {
			t.Errorf("wait item %s for a closed sprint", it.ID)
		}
	}
	if len(outcomes) != 1 {
		t.Errorf("outcomes = %d rows, want 1", len(outcomes))
	}
	got := waitsReport(wait, outcomes)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if last := lines[len(lines)-1]; last != "open gates: 0, waiting sprints: 0, failed checks: 1, finished last 24 h: 0" {
		t.Fatalf("counts line = %q in:\n%s", last, got)
	}
}

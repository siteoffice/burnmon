package store

import (
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burnmon/internal/schema"
)

func ptr[T any](v T) *T { return &v }

func TestUpsertEventsLargestOutputWins(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	base := schema.Event{
		Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
		SessionID: "s1", RequestID: "r1", Model: "claude-x",
		At: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		Input: 100, Output: 5,
	}
	if err := st.UpsertEvents([]schema.Event{base}); err != nil {
		t.Fatal(err)
	}
	bigger := base
	bigger.Output = 50
	if err := st.UpsertEvents([]schema.Event{bigger}); err != nil {
		t.Fatal(err)
	}
	smaller := base
	smaller.Output = 10
	if err := st.UpsertEvents([]schema.Event{smaller}); err != nil {
		t.Fatal(err)
	}

	got, err := st.AllEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1 (same vendor+request_id must dedup)", len(got))
	}
	if got[0].Output != 50 {
		t.Fatalf("Output = %d, want 50 (largest output wins)", got[0].Output)
	}
}

// TestUpsertEventsScopedPerSession guards against the Cowork mirror-session
// bug: two different sessions can share the same request_id (e.g. a
// transcript mirrored across session-id files), and each session's event
// must survive independently rather than being deduped against the other.
func TestUpsertEventsScopedPerSession(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a := schema.Event{
		Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
		SessionID: "session-a", RequestID: "req_shared", Model: "claude-x",
		At: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		Input: 100, Output: 100,
	}
	b := schema.Event{
		Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
		SessionID: "session-b", RequestID: "req_shared", Model: "claude-x",
		At: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		Input: 100, Output: 50,
	}
	if err := st.UpsertEvents([]schema.Event{a, b}); err != nil {
		t.Fatal(err)
	}

	got, err := st.AllEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2 (same request_id, different session_id must not dedup)", len(got))
	}
	bySession := map[string]schema.Event{}
	for _, e := range got {
		bySession[e.SessionID] = e
	}
	ga, ok := bySession["session-a"]
	if !ok {
		t.Fatal("missing event for session-a")
	}
	if ga.Output != 100 {
		t.Fatalf("session-a Output = %d, want 100 (not overwritten by session-b)", ga.Output)
	}
	gb, ok := bySession["session-b"]
	if !ok {
		t.Fatal("missing event for session-b")
	}
	if gb.Output != 50 {
		t.Fatalf("session-b Output = %d, want 50 (not overwritten by session-a)", gb.Output)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, _, _, ok, err := st.Cursor(`C:\trail.jsonl`); err != nil || ok {
		t.Fatalf("Cursor on an unknown path: ok=%v err=%v, want ok=false", ok, err)
	}
	mtime := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if err := st.SetCursor(`C:\trail.jsonl`, 1234, mtime, 5000); err != nil {
		t.Fatal(err)
	}
	offset, gotMtime, size, ok, err := st.Cursor(`C:\trail.jsonl`)
	if err != nil || !ok {
		t.Fatalf("Cursor after SetCursor: ok=%v err=%v", ok, err)
	}
	if offset != 1234 || size != 5000 || !gotMtime.Equal(mtime) {
		t.Fatalf("Cursor = (%d, %v, %d), want (1234, %v, 5000)", offset, gotMtime, size, mtime)
	}
}

func TestNullableFieldsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	e := schema.Event{
		Vendor: "openai", Agent: "codex", Surface: "cli",
		SessionID: "s2", RequestID: "r2", Model: "gpt-x",
		At: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Input: 10, Output: 20,
		CacheRead: ptr(int64(5)), Reasoning: ptr(int64(3)),
		Tools: map[string]int64{"Read": 2},
	}
	if err := st.UpsertEvents([]schema.Event{e}); err != nil {
		t.Fatal(err)
	}
	got, err := st.AllEvents()
	if err != nil || len(got) != 1 {
		t.Fatalf("AllEvents: %v, len=%d", err, len(got))
	}
	g := got[0]
	if g.CacheWrite != nil {
		t.Fatal("CacheWrite should stay nil")
	}
	if g.CacheRead == nil || *g.CacheRead != 5 {
		t.Fatalf("CacheRead = %v, want 5", g.CacheRead)
	}
	if g.Tools["Read"] != 2 {
		t.Fatalf("Tools[Read] = %d, want 2", g.Tools["Read"])
	}
}

// TestEventsSince guards F1's replacement for AllEvents on the bmLive poll
// path: only events at or after the cutoff come back.
func TestEventsSince(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	old := schema.Event{
		Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
		SessionID: "s1", RequestID: "r1", Model: "claude-x",
		At: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC), Input: 10, Output: 1,
	}
	recent := old
	recent.RequestID = "r2"
	recent.At = time.Date(2026, 9, 1, 10, 20, 0, 0, time.UTC)
	if err := st.UpsertEvents([]schema.Event{old, recent}); err != nil {
		t.Fatal(err)
	}

	got, err := st.EventsSince(time.Date(2026, 9, 1, 10, 10, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].RequestID != "r2" {
		t.Fatalf("EventsSince = %+v, want only r2", got)
	}
}

// TestEventsForSession guards the query `burnmon-cli insight <session-id>`
// (v0.2 I1) reads through: every event for sessionID, ordered oldest first,
// and none of another session's events.
func TestEventsForSession(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	s1a := schema.Event{
		Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
		SessionID: "s1", RequestID: "r1", Model: "claude-x",
		At: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC), Input: 10, Output: 1,
	}
	s1b := s1a
	s1b.RequestID = "r2"
	s1b.At = time.Date(2026, 9, 1, 10, 5, 0, 0, time.UTC)
	other := s1a
	other.SessionID = "s2"
	other.RequestID = "r3"
	if err := st.UpsertEvents([]schema.Event{s1b, s1a, other}); err != nil {
		t.Fatal(err)
	}

	got, err := st.EventsForSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].RequestID != "r1" || got[1].RequestID != "r2" {
		t.Fatalf("EventsForSession = %+v, want r1 then r2", got)
	}
}

// TestSessionTotals guards F1's SQL-aggregated whole-session totals used by
// live.ApplySessionTotals: sums must be exact and grouped per model, and the
// claude adapter's synthetic tool-only events (model="" and input=output=0)
// must be excluded, matching internal/live's own isTurn filter.
func TestSessionTotals(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	events := []schema.Event{
		{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "s1",
			RequestID: "r1", Model: "claude-sonnet-5", At: base, Input: 100, Output: 10},
		{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "s1",
			RequestID: "r2", Model: "claude-sonnet-5", At: base.Add(time.Minute), Input: 200, Output: 20},
		{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "s1",
			RequestID: "r3", Model: "", At: base.Add(2 * time.Minute), Input: 0, Output: 0}, // synthetic tool-only event
	}
	if err := st.UpsertEvents(events); err != nil {
		t.Fatal(err)
	}

	rows, err := st.SessionTotals([]SessionKey{{Vendor: "anthropic", SessionID: "s1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 (vendor,session,model) group, got %d: %+v", len(rows), rows)
	}
	r := rows[0]
	if r.Input != 300 || r.Output != 30 || r.Count != 2 {
		t.Fatalf("want input=300 output=30 count=2, got input=%d output=%d count=%d", r.Input, r.Output, r.Count)
	}
	if !r.MinAt.Equal(base) {
		t.Fatalf("MinAt = %v, want %v", r.MinAt, base)
	}
}

// TestFreshStoreAtHeadVersion guards S1: a brand-new store must run every
// migration and land at the current head version.
func TestFreshStoreAtHeadVersion(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var version int
	if err := st.db.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&version); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	if version != 7 {
		t.Fatalf("schema_version = %d, want 7", version)
	}
}

// TestUpsertToolCallsAndTotals guards S2's tool_calls store path: a call and
// its result upsert onto the same row (matched by call_id), a later upsert
// that carries no result never wipes one already recorded, and
// ToolCallTotals aggregates per tool name within the given cutoff.
func TestUpsertToolCallsAndTotals(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	call := schema.ToolCall{
		Vendor: "anthropic", Agent: "claude-code", SessionID: "s1",
		CallID: "toolu_1", Turn: "req-1", Tool: "Read", At: base, InputBytes: 20,
	}
	if err := st.UpsertToolCalls([]schema.ToolCall{call}); err != nil {
		t.Fatal(err)
	}
	// A later row for the same call_id with no result yet (input_bytes only)
	// must not erase a result already recorded... except none has been
	// recorded yet here, so this checks the opposite: recording one now.
	withResult := call
	withResult.ResultBytes = ptr(int64(100))
	if err := st.UpsertToolCalls([]schema.ToolCall{withResult}); err != nil {
		t.Fatal(err)
	}
	// Simulate a stray re-upsert of the bare call (e.g. a re-read of the same
	// region) with no result: the already-recorded result_bytes must survive.
	if err := st.UpsertToolCalls([]schema.ToolCall{call}); err != nil {
		t.Fatal(err)
	}

	other := schema.ToolCall{
		Vendor: "openai", Agent: "codex", SessionID: "s2",
		CallID: "call_1", Turn: "turn-1", Tool: "exec",
		At: base.Add(time.Minute), InputBytes: 50, ResultBytes: ptr(int64(30)),
	}
	old := schema.ToolCall{
		Vendor: "anthropic", Agent: "claude-code", SessionID: "s1",
		CallID: "toolu_old", Turn: "req-0", Tool: "Read",
		At: base.Add(-48 * time.Hour), InputBytes: 5, ResultBytes: ptr(int64(5)),
	}
	if err := st.UpsertToolCalls([]schema.ToolCall{other, old}); err != nil {
		t.Fatal(err)
	}

	rows, err := st.ToolCallTotals(base.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	byTool := map[string]ToolCallTotal{}
	for _, r := range rows {
		byTool[r.Tool] = r
	}
	read, ok := byTool["Read"]
	if !ok {
		t.Fatal("missing Read row")
	}
	if read.Calls != 1 || read.Sessions != 1 || read.InputBytes != 20 || read.ResultBytes != 100 {
		t.Fatalf("Read totals = %+v, want 1 call, 1 session, 20 input bytes, 100 result bytes (old call outside the window excluded)", read)
	}
	exec, ok := byTool["exec"]
	if !ok {
		t.Fatal("missing exec row")
	}
	if exec.Calls != 1 || exec.InputBytes != 50 || exec.ResultBytes != 30 {
		t.Fatalf("exec totals = %+v, want 1 call, 50 input bytes, 30 result bytes", exec)
	}
}

// TestToolCallsForTurn checks the I3 drawer's join: looked up by (vendor,
// session_id, turn), oldest first, a path carried through and preserved
// across a later re-upsert that has no path of its own (migration4).
func TestToolCallsForTurn(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	read := schema.ToolCall{
		Vendor: "anthropic", Agent: "claude-code", SessionID: "s1",
		CallID: "toolu_1", Turn: "req-1", Tool: "Read", At: base,
		InputBytes: 20, Path: `C:\ZND\foo.go`,
	}
	edit := schema.ToolCall{
		Vendor: "anthropic", Agent: "claude-code", SessionID: "s1",
		CallID: "toolu_2", Turn: "req-1", Tool: "Edit", At: base.Add(time.Second),
		InputBytes: 40, Path: `C:\ZND\bar.go`,
	}
	otherTurn := schema.ToolCall{
		Vendor: "anthropic", Agent: "claude-code", SessionID: "s1",
		CallID: "toolu_3", Turn: "req-2", Tool: "Read", At: base.Add(time.Minute),
		InputBytes: 5, Path: `C:\ZND\baz.go`,
	}
	if err := st.UpsertToolCalls([]schema.ToolCall{read, edit, otherTurn}); err != nil {
		t.Fatal(err)
	}
	// A stray re-upsert of the same call_id with an empty path (e.g. a
	// second Parse pass that could not re-derive it) must not blank out the
	// path already recorded.
	readNoPath := read
	readNoPath.Path = ""
	readNoPath.ResultBytes = ptr(int64(99))
	if err := st.UpsertToolCalls([]schema.ToolCall{readNoPath}); err != nil {
		t.Fatal(err)
	}

	calls, err := st.ToolCallsForTurn("anthropic", "s1", "req-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("want 2 calls for req-1, got %d: %+v", len(calls), calls)
	}
	if calls[0].Tool != "Read" || calls[0].Path != `C:\ZND\foo.go` {
		t.Fatalf("calls[0] = %+v, want Read with its path preserved through the re-upsert", calls[0])
	}
	if calls[0].ResultBytes == nil || *calls[0].ResultBytes != 99 {
		t.Fatalf("calls[0].ResultBytes = %v, want 99", calls[0].ResultBytes)
	}
	if calls[1].Tool != "Edit" || calls[1].Path != `C:\ZND\bar.go` {
		t.Fatalf("calls[1] = %+v, want Edit with its own path", calls[1])
	}

	none, err := st.ToolCallsForTurn("anthropic", "s1", "req-nope")
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("want no calls for an unknown turn, got %d", len(none))
	}
}

// copyFile is a plain byte copy, used to snapshot a real store file into a
// temp dir without touching the original.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// rawEventKeys opens path with a bare sqlite connection (not Open, which
// would migrate it) and returns the (vendor, session_id, request_id) key of
// every row in events, plus the row count.
func rawEventKeys(t *testing.T, path string) (map[string]bool, int) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw %s: %v", path, err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT vendor, session_id, request_id FROM events`)
	if err != nil {
		t.Fatalf("query raw events in %s: %v", path, err)
	}
	defer rows.Close()
	keys := map[string]bool{}
	for rows.Next() {
		var v, s, r string
		if err := rows.Scan(&v, &s, &r); err != nil {
			t.Fatalf("scan raw event in %s: %v", path, err)
		}
		keys[v+"|"+s+"|"+r] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read raw events in %s: %v", path, err)
	}
	return keys, len(keys)
}

// TestMigrateRealV01Store is S1's own acceptance test: copy a real v0.1
// store from this laptop (DefaultPath, a database created before this
// session's migrations existed, so it has no schema_version table and no
// owner column) into a temp dir, run it to head through Open, and check
// every one of its events survived with identical (vendor, session_id,
// request_id) ids and that schema_version now reads 2. Skips (does not
// fail) when this laptop has no such store, or when it is locked by a
// running burnmon/burnmon-cli process, since neither says anything about
// whether the migration code is correct.
func TestMigrateRealV01Store(t *testing.T) {
	real, err := DefaultPath()
	if err != nil {
		t.Skip("no default store path resolvable on this platform")
	}
	if _, err := os.Stat(real); err != nil {
		t.Skipf("no real store at %s to migrate: %v", real, err)
	}

	dir := t.TempDir()
	dst := filepath.Join(dir, "burnmon.db")
	if err := copyFile(real, dst); err != nil {
		t.Skipf("could not copy the real store at %s (likely locked by a running app): %v", real, err)
	}

	wantKeys, wantCount := rawEventKeys(t, dst)
	if wantCount == 0 {
		t.Skip("real store has no events to check")
	}

	st, err := Open(dst)
	if err != nil {
		t.Fatalf("Open (migrate) the copied v0.1 store: %v", err)
	}
	defer st.Close()

	var version int
	if err := st.db.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&version); err != nil {
		t.Fatalf("read schema_version after migrate: %v", err)
	}
	if version != 7 {
		t.Fatalf("schema_version after migrate = %d, want 7", version)
	}

	got, err := st.AllEvents()
	if err != nil {
		t.Fatalf("AllEvents after migrate: %v", err)
	}
	if len(got) != wantCount {
		t.Fatalf("event count after migrate = %d, want %d (every v0.1 event must survive)", len(got), wantCount)
	}
	for _, e := range got {
		key := e.Vendor + "|" + e.SessionID + "|" + e.RequestID
		if !wantKeys[key] {
			t.Fatalf("event %s not present in the pre-migration v0.1 store", key)
		}
		if e.Owner != "" {
			t.Fatalf("event %s Owner = %q, want empty (migration 2's default before any reown)", key, e.Owner)
		}
	}
}

// TestReownEvents guards `burnmon-cli reown`: every event's owner is
// recomputed from its stored Project, only rows whose owner actually
// changes are written, and a repeat call with the same rules is a no-op.
func TestReownEvents(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	events := []schema.Event{
		{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "s1", RequestID: "r1",
			Model: "claude-x", At: base, Input: 1, Output: 1, Project: `C:\ZND\projects\burnmon`},
		{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "s2", RequestID: "r2",
			Model: "claude-x", At: base, Input: 1, Output: 1, Project: `C:\dev\Work\other`, Owner: "personal"},
	}
	if err := st.UpsertEvents(events); err != nil {
		t.Fatal(err)
	}

	ownerFor := func(project string) string {
		if strings.HasPrefix(strings.ToLower(project), `c:\znd\`) {
			return "ZND"
		}
		return "ClientA"
	}
	clientFor := func(string) string { return "" }
	n, err := st.ReownEvents(ownerFor, clientFor)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("ReownEvents updated %d row(s), want 2 (both differ from their stored owner)", n)
	}

	got, err := st.AllEvents()
	if err != nil {
		t.Fatal(err)
	}
	bySession := map[string]schema.Event{}
	for _, e := range got {
		bySession[e.SessionID] = e
	}
	if bySession["s1"].Owner != "ZND" {
		t.Fatalf("s1 Owner = %q, want ZND", bySession["s1"].Owner)
	}
	if bySession["s2"].Owner != "ClientA" {
		t.Fatalf("s2 Owner = %q, want ClientA", bySession["s2"].Owner)
	}

	n2, err := st.ReownEvents(ownerFor, clientFor)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 {
		t.Fatalf("second ReownEvents updated %d row(s), want 0 (already reowned)", n2)
	}
}

// TestReownEventsReappliesClient guards K1's reown extension: ReownEvents
// recomputes client the same way it recomputes owner, only rows whose
// client actually changes are written, and a repeat call is a no-op.
func TestReownEventsReappliesClient(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	events := []schema.Event{
		{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "s1", RequestID: "r1",
			Model: "claude-x", At: base, Input: 1, Output: 1, Project: `C:\ZND\projects\dsi`, Client: ""},
	}
	if err := st.UpsertEvents(events); err != nil {
		t.Fatal(err)
	}

	ownerFor := func(string) string { return "ZND" }
	clientFor := func(project string) string {
		if strings.HasPrefix(strings.ToLower(project), `c:\znd\projects\dsi`) {
			return "ClientB"
		}
		return "unassigned"
	}
	n, err := st.ReownEvents(ownerFor, clientFor)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("ReownEvents updated %d row(s), want 1", n)
	}

	got, err := st.AllEvents()
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Client != "ClientB" {
		t.Fatalf("Client = %q, want ClientB", got[0].Client)
	}

	n2, err := st.ReownEvents(ownerFor, clientFor)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 {
		t.Fatalf("second ReownEvents updated %d row(s), want 0 (already reowned)", n2)
	}
}

// TestMigration7AddsClientColumn guards the client column itself: a fresh
// store accepts and round-trips Client through UpsertEvents/AllEvents.
func TestMigration7AddsClientColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "burnmon.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	events := []schema.Event{
		{Vendor: "anthropic", Agent: "claude-code", SessionID: "s1", RequestID: "r1",
			At: time.Now().UTC(), Model: "claude-sonnet-5", Input: 1, Output: 1,
			Owner: "ZND", Client: "ClientB"},
	}
	if err := st.UpsertEvents(events); err != nil {
		t.Fatalf("UpsertEvents: %v", err)
	}
	got, err := st.AllEvents()
	if err != nil {
		t.Fatalf("AllEvents: %v", err)
	}
	if len(got) != 1 || got[0].Client != "ClientB" {
		t.Fatalf("got %+v, want one event with Client=ClientB", got)
	}
}

// TestSessionTotalsVendorQualified guards Wilco's decision to key
// SessionTotals by (vendor, session_id) pairs, not session_id alone: two
// different vendors can in principle mint the same session id, and a
// vendor-unqualified lookup would wrongly sum both vendors' events into one
// session's totals.
func TestSessionTotalsVendorQualified(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	events := []schema.Event{
		{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "shared",
			RequestID: "r1", Model: "claude-sonnet-5", At: base, Input: 100, Output: 10},
		{Vendor: "openai", Agent: "codex", Surface: "cli", SessionID: "shared",
			RequestID: "r1", Model: "gpt-5.6-sol", At: base, Input: 5000, Output: 500},
	}
	if err := st.UpsertEvents(events); err != nil {
		t.Fatal(err)
	}

	rows, err := st.SessionTotals([]SessionKey{{Vendor: "anthropic", SessionID: "shared"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 group for the anthropic key alone, got %d: %+v", len(rows), rows)
	}
	if rows[0].Vendor != "anthropic" || rows[0].Input != 100 || rows[0].Output != 10 {
		t.Fatalf("want anthropic's own totals only (input=100 output=10), got %+v", rows[0])
	}
}

// TestForecastScoresRoundTrip guards F1's store layer: InsertForecastPlan
// never overwrites a week already on record, RecordForecastActual only
// fills a week in once, and DailyTokenTotals buckets by local calendar day
// (Wilco's decision, 2026-09-26: local time everywhere).
func TestForecastScoresRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ws := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC) // Monday
	isoYear, isoWeek := ws.ISOWeek()

	if _, ok, err := st.ForecastScore(isoYear, isoWeek); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("ForecastScore found a row before any was inserted")
	}

	if err := st.InsertForecastPlan(isoYear, isoWeek, ws, 4900); err != nil {
		t.Fatal(err)
	}
	// A later call for the same week must not overwrite the plan already
	// recorded ("the forecast made on Monday", never revised).
	if err := st.InsertForecastPlan(isoYear, isoWeek, ws, 9999); err != nil {
		t.Fatal(err)
	}

	sc, ok, err := st.ForecastScore(isoYear, isoWeek)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ForecastScore: row not found after insert")
	}
	if sc.PlanTokens != 4900 {
		t.Fatalf("PlanTokens = %d, want 4900 (second insert must not overwrite)", sc.PlanTokens)
	}
	if sc.ActualTokens != nil {
		t.Fatalf("ActualTokens = %v, want nil before scoring", sc.ActualTokens)
	}

	scoredAt := ws.AddDate(0, 0, 8)
	if err := st.RecordForecastActual(isoYear, isoWeek, 7000, scoredAt); err != nil {
		t.Fatal(err)
	}
	// A second scoring pass over the same week must not change the actual
	// already recorded.
	if err := st.RecordForecastActual(isoYear, isoWeek, 1, scoredAt); err != nil {
		t.Fatal(err)
	}

	scores, err := st.ForecastScores()
	if err != nil {
		t.Fatal(err)
	}
	if len(scores) != 1 {
		t.Fatalf("len(ForecastScores) = %d, want 1", len(scores))
	}
	got := scores[0]
	if got.ActualTokens == nil || *got.ActualTokens != 7000 {
		t.Fatalf("ActualTokens = %v, want 7000", got.ActualTokens)
	}
	if got.ScoredAt == nil {
		t.Fatal("ScoredAt = nil, want set")
	}

	events := []schema.Event{
		{Vendor: "anthropic", Agent: "claude-code", SessionID: "s1", RequestID: "r1",
			Model: "m", At: ws.Add(9 * time.Hour), Input: 300},
		{Vendor: "anthropic", Agent: "claude-code", SessionID: "s1", RequestID: "r2",
			Model: "m", At: ws.Add(20 * time.Hour), Input: 200},
		{Vendor: "anthropic", Agent: "claude-code", SessionID: "s1", RequestID: "r3",
			Model: "m", At: ws.AddDate(0, 0, 1).Add(9 * time.Hour), Input: 50},
	}
	if err := st.UpsertEvents(events); err != nil {
		t.Fatal(err)
	}
	daily, err := st.DailyTokenTotals(ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(daily) != 2 {
		t.Fatalf("len(DailyTokenTotals) = %d, want 2 (two distinct days)", len(daily))
	}
	if daily[0].Date != ws.In(time.Local).Format("2006-01-02") || daily[0].Tokens != 500 {
		t.Fatalf("day 0 = %+v, want {%s 500}", daily[0], ws.In(time.Local).Format("2006-01-02"))
	}
	if daily[1].Tokens != 50 {
		t.Fatalf("day 1 tokens = %d, want 50", daily[1].Tokens)
	}
}

// TestDailyTokenTotals_DSTEarlyMorning is extra item 5's spec'd check
// (Wilco, 2026-09-26: local time everywhere), for DailyTokenTotals
// specifically: SQLite has no IANA timezone database, so bucketing this in
// SQL (a substr(at,1,10) on the UTC-stored column, the old design) can only
// ever apply UTC calendar days, or at best a single fixed offset that is
// wrong on the DST transition's own day. An early-morning turn just after
// each 2026 Europe/Amsterdam transition must still bucket to the correct
// local day; late-evening turns do not distinguish the bug (see
// internal/history's TestBuild_LocalDayBucketing_DST, which explains why in
// full). Skips itself if this machine's time.Local does not actually agree
// with Europe/Amsterdam at the tested instants.
func TestDailyTokenTotals_DSTEarlyMorning(t *testing.T) {
	ams, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Skip("Europe/Amsterdam tzdata not available:", err)
	}

	cases := []struct {
		name      string
		localWall time.Time
		wantDay   string
	}{
		// 2026-03-29, spring forward (23-hour day): 00:30 CET (UTC+1) is
		// 2026-03-28 23:30 UTC, the distinguishing case.
		{"spring forward", time.Date(2026, 3, 29, 0, 30, 0, 0, ams), "2026-03-29"},
		// 2026-10-25, fall back (25-hour day): 00:30 CEST (UTC+2, before
		// the fold) is 2026-10-24 22:30 UTC, the distinguishing case.
		{"fall back", time.Date(2026, 10, 25, 0, 30, 0, 0, ams), "2026-10-25"},
	}
	for _, c := range cases {
		_, offAms := c.localWall.Zone()
		_, offLocal := c.localWall.In(time.Local).Zone()
		if offAms != offLocal {
			t.Skipf("this machine's time.Local does not match Europe/Amsterdam at %s; skipping a DST test that assumes it does", c.localWall)
		}
	}

	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var events []schema.Event
	for i, c := range cases {
		events = append(events, schema.Event{
			Vendor: "anthropic", Agent: "claude-code", SessionID: "s", RequestID: "r" + c.name,
			Model: "m", At: c.localWall, Input: int64(100 * (i + 1)),
		})
	}
	if err := st.UpsertEvents(events); err != nil {
		t.Fatal(err)
	}

	daily, err := st.DailyTokenTotals(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	byDay := map[string]int64{}
	for _, d := range daily {
		byDay[d.Date] = d.Tokens
	}
	for i, c := range cases {
		want := int64(100 * (i + 1))
		if got := byDay[c.wantDay]; got != want {
			t.Errorf("%s: DailyTokenTotals[%s] = %d, want %d (got days: %+v)", c.name, c.wantDay, got, want, daily)
		}
	}
}

func TestResetCollapsedCodexSessions(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	at := time.Date(2026, 9, 29, 16, 0, 0, 0, time.UTC)
	ev := func(agent, sid, rid string) schema.Event {
		return schema.Event{Vendor: "openai", Agent: agent, SessionID: sid, RequestID: rid, At: at, Input: 10, Output: 1}
	}
	if err := st.UpsertEvents([]schema.Event{
		ev("codex", "rollout-a", "rollout-a:0"),  // collapsed, no ordinal
		ev("codex", "rollout-b", "rollout-b:0"),  // collapsed row beside real ones
		ev("codex", "rollout-b", "rollout-b:15"), // real ordinal key, must stay
		ev("codex", "rollout-c", "rollout-c:12"), // healthy session, untouched
	}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join("s", "rollout-a.jsonl"), filepath.Join("s", "rollout-b.jsonl"), filepath.Join("s", "rollout-c.jsonl")} {
		if err := st.SetCursor(p, 100, at, 100); err != nil {
			t.Fatal(err)
		}
	}

	events, cursors, err := st.ResetCollapsedCodexSessions()
	if err != nil {
		t.Fatal(err)
	}
	if events != 2 || cursors != 2 {
		t.Fatalf("removed %d events, %d cursors; want 2 and 2", events, cursors)
	}
	got, err := st.AllEvents()
	if err != nil {
		t.Fatal(err)
	}
	left := map[string]bool{}
	for _, e := range got {
		left[e.RequestID] = true
	}
	if len(left) != 2 || !left["rollout-b:15"] || !left["rollout-c:12"] {
		t.Fatalf("events left = %v, want rollout-b:15 and rollout-c:12", left)
	}
	if _, _, _, ok, _ := st.Cursor(filepath.Join("s", "rollout-a.jsonl")); ok {
		t.Fatal("cursor for rollout-a still present, file would not be re-read")
	}
	if _, _, _, ok, _ := st.Cursor(filepath.Join("s", "rollout-c.jsonl")); !ok {
		t.Fatal("cursor for healthy rollout-c was removed")
	}
	if e, c, err := st.ResetCollapsedCodexSessions(); err != nil || e != 0 || c != 0 {
		t.Fatalf("second run removed %d/%d (err %v), want a no-op", e, c, err)
	}
}

// TestLatestToolCalls guards the Station's one-query-per-tick read: the
// newest call per (vendor, session_id) by its real instant, not by the text
// of the stored `at`. RFC3339Nano trims trailing zeros, so a whole-second
// "10:00:00Z" sorts after "10:00:00.5Z" as text ('Z' > '.'), which would pick
// the older call; the 500 ms pair below fails that way.
func TestLatestToolCalls(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	base := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	calls := []schema.ToolCall{
		{Vendor: "anthropic", Agent: "claude-code", SessionID: "s1", CallID: "a", Turn: "req-1", Tool: "Read", At: base, InputBytes: 1},
		{Vendor: "anthropic", Agent: "claude-code", SessionID: "s1", CallID: "b", Turn: "req-2", Tool: "Bash", At: base.Add(500 * time.Millisecond), InputBytes: 2, ResultBytes: ptr(int64(7))},
		{Vendor: "anthropic", Agent: "claude-code", SessionID: "s1", CallID: "c", Turn: "req-0", Tool: "Edit", At: base.Add(-time.Minute), InputBytes: 3},
		// Same session id under another vendor: its own key, its own answer.
		{Vendor: "openai", Agent: "codex", SessionID: "s1", CallID: "d", Turn: "t-1", Tool: "exec", At: base.Add(time.Hour), InputBytes: 4},
		{Vendor: "openai", Agent: "codex", SessionID: "s2", CallID: "e", Turn: "t-2", Tool: "apply_patch", At: base.Add(2 * time.Second), InputBytes: 5},
		// Not asked for: must not appear.
		{Vendor: "openai", Agent: "codex", SessionID: "s3", CallID: "f", Turn: "t-3", Tool: "exec", At: base.Add(3 * time.Hour), InputBytes: 6},
	}
	if err := st.UpsertToolCalls(calls); err != nil {
		t.Fatal(err)
	}

	got, err := st.LatestToolCalls([]SessionKey{
		{Vendor: "anthropic", SessionID: "s1"},
		{Vendor: "openai", SessionID: "s1"},
		{Vendor: "openai", SessionID: "s2"},
		{Vendor: "anthropic", SessionID: "no-calls"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 sessions, got %d: %+v", len(got), got)
	}
	c := got[SessionKey{Vendor: "anthropic", SessionID: "s1"}]
	if c.CallID != "b" || c.Tool != "Bash" || c.Turn != "req-2" || !c.At.Equal(base.Add(500*time.Millisecond)) {
		t.Fatalf("anthropic/s1 = %+v, want call b (Bash, req-2, 10:00:00.5)", c)
	}
	if c.ResultBytes == nil || *c.ResultBytes != 7 {
		t.Fatalf("anthropic/s1 ResultBytes = %v, want 7", c.ResultBytes)
	}
	if c := got[SessionKey{Vendor: "openai", SessionID: "s1"}]; c.CallID != "d" || c.ResultBytes != nil {
		t.Fatalf("openai/s1 = %+v, want call d with no result", c)
	}
	if c := got[SessionKey{Vendor: "openai", SessionID: "s2"}]; c.CallID != "e" || c.Tool != "apply_patch" {
		t.Fatalf("openai/s2 = %+v, want call e", c)
	}
	if _, ok := got[SessionKey{Vendor: "anthropic", SessionID: "no-calls"}]; ok {
		t.Fatal("a session with no calls must be absent")
	}

	empty, err := st.LatestToolCalls(nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty keys: got %v, %v", empty, err)
	}
}

// TestUpsertToolCallsResultOnly: a result-only row (no Tool, the adapters'
// shape for a result read after its call) fills result_bytes on the stored
// call and changes nothing else; one for an unknown call inserts nothing.
func TestUpsertToolCallsResultOnly(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	at := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	call := schema.ToolCall{Vendor: "anthropic", Agent: "claude-code", SessionID: "s", CallID: "c1", Turn: "req-1", Tool: "Bash", At: at, InputBytes: 9, Path: "p"}
	if err := st.UpsertToolCalls([]schema.ToolCall{call}); err != nil {
		t.Fatal(err)
	}
	late := []schema.ToolCall{
		{Vendor: "anthropic", SessionID: "s", CallID: "c1", ResultBytes: ptr(int64(42))},
		{Vendor: "anthropic", SessionID: "s", CallID: "ghost", ResultBytes: ptr(int64(1))},
	}
	if err := st.UpsertToolCalls(late); err != nil {
		t.Fatal(err)
	}
	got, err := st.ToolCallsForTurn("anthropic", "s", "req-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Tool != "Bash" || got[0].Agent != "claude-code" || got[0].InputBytes != 9 || got[0].Path != "p" || !got[0].At.Equal(at) {
		t.Fatalf("got %+v, want the original call untouched apart from its result", got)
	}
	if got[0].ResultBytes == nil || *got[0].ResultBytes != 42 {
		t.Fatalf("ResultBytes = %v, want 42", got[0].ResultBytes)
	}
	m, err := st.LatestToolCalls([]SessionKey{{Vendor: "anthropic", SessionID: "s"}})
	if err != nil || m[SessionKey{Vendor: "anthropic", SessionID: "s"}].CallID != "c1" {
		t.Fatalf("the ghost result must not create a row: %+v %v", m, err)
	}
}

// TestPurgeAuditSessions removes what the 2026-10-12 live-watcher bug left
// behind: Claude events and tool calls under session key "audit" (a local
// Cowork session's audit.jsonl read as its own session) and that file's
// cursor. Real sessions and other vendors stay untouched.
func TestPurgeAuditSessions(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	at := time.Date(2026, 10, 12, 14, 45, 0, 0, time.UTC)
	ev := func(vendor, agent, sid, rid string) schema.Event {
		return schema.Event{Vendor: vendor, Agent: agent, SessionID: sid, RequestID: rid, At: at, Input: 4, Output: 20}
	}
	if err := st.UpsertEvents([]schema.Event{
		ev("anthropic", "cowork", "audit", "req_1"),
		ev("anthropic", "cowork", "audit", "req_2"),
		ev("anthropic", "cowork", "e5af89ab", "req_1"),
		ev("openai", "codex", "audit", "audit:3"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertToolCalls([]schema.ToolCall{
		{Vendor: "anthropic", Agent: "cowork", SessionID: "audit", CallID: "c1", Turn: "req_1", Tool: "Bash", At: at},
		{Vendor: "anthropic", Agent: "cowork", SessionID: "e5af89ab", CallID: "c1", Turn: "req_1", Tool: "Bash", At: at},
	}); err != nil {
		t.Fatal(err)
	}
	auditPath := `C:\x\local-agent-mode-sessions\a\o\92082970\audit.jsonl`
	realPath := `C:\x\local-agent-mode-sessions\a\o\92082970\.claude\projects\session\e5af89ab.jsonl`
	for _, p := range []string{auditPath, realPath} {
		if err := st.SetCursor(p, 100, at, 100); err != nil {
			t.Fatal(err)
		}
	}

	events, calls, cursors, err := st.PurgeAuditSessions()
	if err != nil {
		t.Fatal(err)
	}
	if events != 2 || calls != 1 || cursors != 1 {
		t.Fatalf("removed %d events, %d tool calls, %d cursors; want 2, 1, 1", events, calls, cursors)
	}
	got, err := st.AllEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("%d events left, want 2 (the real Cowork row and the Codex row)", len(got))
	}
	for _, e := range got {
		if e.Vendor == "anthropic" && e.SessionID == "audit" {
			t.Fatalf("audit event %s still present", e.RequestID)
		}
	}
	if _, _, _, ok, _ := st.Cursor(auditPath); ok {
		t.Fatal("audit.jsonl cursor still present")
	}
	if _, _, _, ok, _ := st.Cursor(realPath); !ok {
		t.Fatal("real transcript cursor was removed")
	}
	if e, c, k, err := st.PurgeAuditSessions(); err != nil || e != 0 || c != 0 || k != 0 {
		t.Fatalf("second run removed %d/%d/%d (err %v), want a no-op", e, c, k, err)
	}
}

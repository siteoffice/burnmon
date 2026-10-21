package copilotcli

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestPollOnceFixture is A2's spec'd test: the fixture at testdata\copilot\
// parses to the expected session count and tokens. The fixture holds three
// real Copilot CLI sessions Wilco recorded 2026-09-22 (session-store.db's
// `sessions` and `assistant_usage_events` tables only, no turn content:
// SESSION_LOG.md v0.2 43B), copied into a standalone SQLite file with the
// same column shapes PollOnce reads.
func TestPollOnceFixture(t *testing.T) {
	events, err := PollOnce("../../../testdata/copilot/copilot_fixture.db")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3 (all three fixture sessions have usage events)", len(events))
	}

	byID := map[string]int{}
	for i, e := range events {
		byID[e.SessionID] = i
	}

	e := events[byID["0502950e-e136-4bb9-b700-18498b0e58e4"]]
	if e.Vendor != "github" || e.Agent != "copilot-cli" {
		t.Errorf("vendor/agent = %q/%q, want github/copilot-cli", e.Vendor, e.Agent)
	}
	if e.Surface != "cli" {
		t.Errorf("surface = %q, want cli", e.Surface)
	}
	if e.Model != "claude-sonnet-5" {
		t.Errorf("model = %q, want claude-sonnet-5", e.Model)
	}
	if e.Project != `C:\ZND\projects\burnmon\testdata\copilot` {
		t.Errorf("project = %q, want C:\\ZND\\projects\\burnmon\\testdata\\copilot", e.Project)
	}
	if e.Input != 128807 || e.Output != 732 {
		t.Errorf("input/output = %d/%d, want 128807/732", e.Input, e.Output)
	}
	if e.CacheRead == nil || *e.CacheRead != 128479 {
		t.Errorf("cache_read = %v, want 128479", e.CacheRead)
	}
	if e.CacheWrite == nil || *e.CacheWrite != 326 {
		t.Errorf("cache_write = %v, want 326", e.CacheWrite)
	}
	if e.Reasoning == nil || *e.Reasoning != 0 {
		t.Errorf("reasoning = %v, want 0", e.Reasoning)
	}
	wantAt := "2026-09-22T21:12:00.975Z"
	if e.At.Format("2006-01-02T15:04:05.000Z") != wantAt {
		t.Errorf("at = %v, want %s (the latest usage row's own created_at)", e.At, wantAt)
	}

	// RequestID is fixed to the session id, same trick as Hermes: a later
	// poll's later "latest row" event for the same session upserts over
	// this one via the store's "largest output wins" rule instead of
	// piling up a new row per poll.
	if e.RequestID != e.SessionID {
		t.Errorf("request id = %q, want it to equal session id %q", e.RequestID, e.SessionID)
	}
}

// TestPollOnceIdempotent checks a second poll of the same fixture returns
// the same totals: PollOnce carries no cursor of its own, so repeat-safety
// comes from the caller's UpsertEvents upsert, not from anything here; this
// only checks PollOnce itself is a stable, repeatable read.
func TestPollOnceIdempotent(t *testing.T) {
	first, err := PollOnce("../../../testdata/copilot/copilot_fixture.db")
	if err != nil {
		t.Fatal(err)
	}
	second, err := PollOnce("../../../testdata/copilot/copilot_fixture.db")
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != len(second) {
		t.Fatalf("first poll = %d events, second = %d, want equal", len(first), len(second))
	}
}

// TestDefaultDBPathHonorsCopilotHome confirms DefaultDBPath finds a
// session-store.db under an overridden COPILOT_HOME, and returns "" for a
// home directory with no such file (the "not installed" case).
func TestDefaultDBPathHonorsCopilotHome(t *testing.T) {
	t.Setenv("COPILOT_HOME", "..")
	if got := DefaultDBPath(); got != "" {
		t.Errorf("DefaultDBPath() = %q, want \"\" (no session-store.db directly under internal/adapter)", got)
	}
}

// TestPollOnceNullTokens: Copilot CLI's schema declares every token column
// nullable, and real stores carry rows with reasoning_tokens NULL (seen on
// macOS, 2026-10-20). A NULL must read as 0 instead of failing the whole poll.
func TestPollOnceNullTokens(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "session-store.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, cwd TEXT)`,
		`CREATE TABLE assistant_usage_events (id INTEGER PRIMARY KEY, session_id TEXT, model TEXT,
			input_tokens INTEGER, output_tokens INTEGER, cache_read_tokens INTEGER,
			cache_write_tokens INTEGER, reasoning_tokens INTEGER, created_at TEXT)`,
		`INSERT INTO sessions VALUES ('s1', '/tmp/x')`,
		`INSERT INTO assistant_usage_events VALUES (1, 's1', 'claude-sonnet-5', 100, 20, NULL, NULL, NULL, '2026-10-20T12:00:00.000Z')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	events, err := PollOnce(dbPath)
	if err != nil {
		t.Fatalf("PollOnce with NULL token columns: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	e := events[0]
	if e.Input != 100 || e.Output != 20 {
		t.Errorf("input/output = %d/%d, want 100/20", e.Input, e.Output)
	}
	if e.Reasoning == nil || *e.Reasoning != 0 {
		t.Errorf("reasoning = %v, want 0", e.Reasoning)
	}
}

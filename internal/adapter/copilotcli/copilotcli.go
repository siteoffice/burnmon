// Package copilotcli reads the GitHub Copilot CLI's own SQLite state
// database (WAL mode) and turns its sessions into schema.Events.
//
// A2 confirmed the plan's assumption ("token totals in data.db, written at
// session end") false on two counts (SESSION_LOG.md, v0.2 43B), on a live
// install (COPILOT_HOME `~/.copilot`, Copilot CLI running, PID observed)
// plus three real fixture sessions recorded there the same day:
//
//  1. Wrong file. There is no `data.db`. The store is `session-store.db`
//     (+ `-wal`/`-shm`), under COPILOT_HOME (default `~/.copilot`).
//  2. Wrong timing. Usage is not written once at session close. Table
//     `assistant_usage_events` gets one row per API call, seconds apart,
//     while the turn is in progress (57 rows over ~30 minutes of one
//     conversation, observed live). Neither `sessions` nor `turns` nor
//     `session-state/<uuid>/workspace.yaml` carries an `ended_at`, `status`
//     or `closed` column: "session closed" is not a fact this store
//     records at all.
//
// Design (per Wilco's call after A2, in place of the plan's "totals at
// session end"): the same shape as internal/adapter/hermes. `input_tokens`
// and `cache_read_tokens` grow monotonically per call within a session (the
// conversation's own growing context, same pattern as Codex's cumulative
// counters), so the right per-session aggregate is the latest row, not a
// sum. PollOnce emits one Event per session carrying that latest row's
// totals, RequestID fixed to the session id so the store's own "largest
// output wins" upsert naturally keeps only the latest state; At is that
// row's own created_at (a real, recent timestamp while the CLI is active,
// not a "now" substitute), so internal/live's existing generic
// last-event-recency window decides running vs finished exactly as it does
// for every other adapter, with no adapter-side "wait for close" logic.
package copilotcli

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"burnmon/internal/schema"
)

// DefaultDBPath is COPILOT_HOME's `session-store.db` (COPILOT_HOME default
// `~/.copilot`), where Wilco's own install has been found. Empty (not an
// error) when the home directory can't be resolved or the file does not
// exist, so a caller can treat "no Copilot CLI installed" the same as "no
// roots" elsewhere in burnmon.
func DefaultDBPath() string {
	home := os.Getenv("COPILOT_HOME")
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil || h == "" {
			return ""
		}
		home = filepath.Join(h, ".copilot")
	}
	p := filepath.Join(home, "session-store.db")
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// PollOnce opens dbPath read-only (WAL mode: a concurrent Copilot CLI write
// never blocks or corrupts this read) and returns one Event per session that
// has recorded at least one usage event, current as of this call. No offset
// or cursor: call this every 5 seconds (A1's poll interval, reused here) and
// pass every row straight to store.UpsertEvents.
func PollOnce(dbPath string) ([]schema.Event, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&_pragma=busy_timeout(2000)")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
SELECT s.id, s.cwd, COALESCE(e.model, ''), COALESCE(e.input_tokens, 0), COALESCE(e.output_tokens, 0),
	COALESCE(e.cache_read_tokens, 0), COALESCE(e.cache_write_tokens, 0), COALESCE(e.reasoning_tokens, 0), e.created_at
FROM sessions s
JOIN assistant_usage_events e ON e.id = (
	SELECT id FROM assistant_usage_events
	WHERE session_id = s.id
	ORDER BY id DESC LIMIT 1
)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []schema.Event
	for rows.Next() {
		var id, model, createdAt string
		var cwd sql.NullString
		var input, output, cacheRead, cacheWrite, reasoning int64
		if err := rows.Scan(&id, &cwd, &model, &input, &output,
			&cacheRead, &cacheWrite, &reasoning, &createdAt); err != nil {
			return nil, err
		}

		at, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			at = time.Now().UTC()
		}

		events = append(events, schema.Event{
			Vendor:     "github",
			Agent:      "copilot-cli",
			Surface:    "cli",
			SessionID:  id,
			RequestID:  id, // one row per session; unique within (vendor, session_id) trivially
			At:         at.UTC(),
			Model:      model,
			Project:    cwd.String,
			Input:      input,
			CacheWrite: &cacheWrite,
			CacheRead:  &cacheRead,
			Output:     output,
			Reasoning:  &reasoning,
		})
	}
	return events, rows.Err()
}

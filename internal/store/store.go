// Package store is burnmon's local SQLite persistence: every parsed Event,
// one cursor per trail file (so a file is read from where it stopped, not
// from the start every time), and a small meta key/value table.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"burnmon/internal/schema"
	"burnmon/internal/store/migrations"
)

// Store holds two *sql.DB handles onto the same file: db is the writer,
// one connection, serialising every insert/update/delete exactly as before;
// readDB is a small pool of read-only connections, so a poll (bmLive,
// bmVendorStrip, bmForecast, bmSessionInsight, bmHistory) never queues
// behind an in-flight ingest or Collect on the same connection. WAL mode
// (set in Open) is what makes that split safe: a WAL reader sees a
// consistent snapshot without blocking on, or blocking, the one writer.
// See SESSION_LOG.md, v0.2.1 hang patch, H1.
type Store struct {
	db     *sql.DB
	readDB *sql.DB
}

const schemaVersionDDL = `CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL);`

// Open creates path's parent folder if needed, opens (creating if absent)
// the SQLite file at path, and runs every pending migration (migrations.All)
// inside one transaction, logging the from/to version when it moved. Safe to
// call every run: every migration is additive and idempotent against a
// database already at or past its version.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("store: create %s: %w", dir, err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	// One connection: modernc.org/sqlite serialises writes per connection
	// anyway, and burnmon's own callers (CLI: one-shot; app: one rebuild at
	// a time, guarded by app.building) never need concurrent writers.
	db.SetMaxOpenConns(1)
	if err := setPragmas(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: pragmas on %s: %w", path, err)
	}
	from, to, err := runMigrations(db)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("store: migrate %s: %w", path, err)
	}
	if from != to {
		log.Printf("store: migrated %s from version %d to %d", path, from, to)
	}

	// readDB is a second handle onto the same file, opened only after the
	// writer above has set WAL mode and run every migration, so a read
	// through this pool never race-reads a schema the writer has not
	// finished creating. A handful of connections (not one, not unbounded):
	// several bound functions (bmLive, bmSessionInsight per session, ...)
	// can be in flight from the UI thread inside the same second, and WAL
	// lets them all read concurrently with each other and with the writer.
	readDB, err := sql.Open("sqlite", path)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("store: open read pool for %s: %w", path, err)
	}
	readDB.SetMaxOpenConns(8)
	if err := setPragmas(readDB); err != nil {
		db.Close()
		readDB.Close()
		return nil, fmt.Errorf("store: pragmas on read pool for %s: %w", path, err)
	}
	return &Store{db: db, readDB: readDB}, nil
}

// setPragmas puts conn in WAL mode (readers and the one writer stop
// blocking each other) with a five-second busy timeout (so a reader that
// still lands mid-checkpoint retries instead of erroring SQLITE_BUSY
// straight away). Idempotent: safe to call on every connection opened
// against the same file, including the read pool's four connections.
func setPragmas(conn *sql.DB) error {
	if _, err := conn.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		return fmt.Errorf("journal_mode=WAL: %w", err)
	}
	if _, err := conn.Exec(`PRAGMA busy_timeout=5000;`); err != nil {
		return fmt.Errorf("busy_timeout: %w", err)
	}
	return nil
}

// runMigrations ensures the schema_version table exists, reads the store's
// current version (0 for a database that predates this table, including
// every real v0.1 store on disk), and runs every migration whose Version is
// greater than that, in order, inside one transaction, recording the head
// version on success.
func runMigrations(db *sql.DB) (from, to int, err error) {
	if _, err = db.Exec(schemaVersionDDL); err != nil {
		return 0, 0, fmt.Errorf("create schema_version: %w", err)
	}
	var current int
	err = db.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&current)
	if err == sql.ErrNoRows {
		current, err = 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("read schema_version: %w", err)
	}
	from = current

	var pending []migrations.Migration
	for _, m := range migrations.All {
		if m.Version > current {
			pending = append(pending, m)
		}
	}
	if len(pending) == 0 {
		return from, from, nil
	}

	tx, err := db.Begin()
	if err != nil {
		return from, from, err
	}
	defer tx.Rollback()
	for _, m := range pending {
		if err = m.Up(tx); err != nil {
			return from, from, fmt.Errorf("migration %d (%s): %w", m.Version, m.Name, err)
		}
	}
	to = pending[len(pending)-1].Version
	if _, err = tx.Exec(`DELETE FROM schema_version`); err != nil {
		return from, from, err
	}
	if _, err = tx.Exec(`INSERT INTO schema_version (version) VALUES (?)`, to); err != nil {
		return from, from, err
	}
	if err = tx.Commit(); err != nil {
		return from, from, err
	}
	return from, to, nil
}

func (s *Store) Close() error {
	err := s.readDB.Close()
	if werr := s.db.Close(); werr != nil {
		err = werr
	}
	return err
}

// UpsertEvents inserts events, keeping, per (vendor, session_id, request_id),
// the one with the largest Output ("largest output wins": a streamed API
// call is written as several lines sharing a request id with only
// output_tokens growing, all within one session's ingest).
func (s *Store) UpsertEvents(events []schema.Event) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
INSERT INTO events (
	vendor, agent, surface, session_id, request_id, parent_id, at, model,
	project, title, input, cache_write, cache_read, output, reasoning,
	vendor_cost, window_used, window_reset, tools, owner, client
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (vendor, session_id, request_id) DO UPDATE SET
	agent = excluded.agent, surface = excluded.surface,
	session_id = excluded.session_id, parent_id = excluded.parent_id,
	at = excluded.at, model = excluded.model, project = excluded.project,
	title = excluded.title, input = excluded.input,
	cache_write = excluded.cache_write, cache_read = excluded.cache_read,
	output = excluded.output, reasoning = excluded.reasoning,
	vendor_cost = excluded.vendor_cost, window_used = excluded.window_used,
	window_reset = excluded.window_reset, tools = excluded.tools,
	owner = excluded.owner, client = excluded.client
WHERE excluded.output > events.output
`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	// Before 2026-10-25 the Cowork (Cloud) parser stored a subagent's
	// request under its parent session. When that request comes in again as
	// a subagent row, the folded copy goes, so its tokens count once. Only
	// a cowork-cloud row without a parent of its own is removed: other
	// harnesses can share a request id across sessions (mirror sessions).
	unfold, err := tx.Prepare(`DELETE FROM events WHERE vendor = ? AND agent = 'cowork-cloud'
	AND session_id = ? AND request_id = ? AND parent_id = ''`)
	if err != nil {
		return err
	}
	defer unfold.Close()

	for _, e := range events {
		if e.Agent == "cowork-cloud" && e.ParentID != "" {
			if _, err := unfold.Exec(e.Vendor, e.ParentID, e.RequestID); err != nil {
				return fmt.Errorf("store: unfold %s/%s: %w", e.Vendor, e.RequestID, err)
			}
		}
		toolsJSON, err := json.Marshal(e.Tools)
		if err != nil {
			return fmt.Errorf("store: marshal tools for %s/%s: %w", e.Vendor, e.RequestID, err)
		}
		var windowReset any
		if e.WindowReset != nil {
			windowReset = e.WindowReset.UTC().Format(time.RFC3339Nano)
		}
		_, err = stmt.Exec(
			e.Vendor, e.Agent, e.Surface, e.SessionID, e.RequestID, e.ParentID,
			e.At.UTC().Format(time.RFC3339Nano), e.Model, e.Project, e.Title,
			e.Input, nullInt(e.CacheWrite), nullInt(e.CacheRead), e.Output,
			nullInt(e.Reasoning), nullFloat(e.VendorCost), nullFloat(e.WindowUsed),
			windowReset, string(toolsJSON), e.Owner, e.Client,
		)
		if err != nil {
			return fmt.Errorf("store: upsert %s/%s: %w", e.Vendor, e.RequestID, err)
		}
	}
	return tx.Commit()
}

// UpsertToolCalls inserts tool calls, keyed by (vendor, session_id, call_id).
// A repeat call for the same call_id (a later incremental read of the same
// file re-encountering the row, or the call and its result arriving in
// separate Parse calls) updates every column but only overwrites
// result_bytes when the new row actually carries one, so a result already
// recorded is never wiped back to unknown by a later row that has none.
// A result-only row (Tool "", the adapters' shape for a result read in a
// later pass than its call) only fills result_bytes on the stored call and
// never inserts one.
func (s *Store) UpsertToolCalls(calls []schema.ToolCall) error {
	if len(calls) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
INSERT INTO tool_calls (vendor, agent, session_id, call_id, turn, tool, at, input_bytes, result_bytes, path)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (vendor, session_id, call_id) DO UPDATE SET
	agent = excluded.agent, turn = excluded.turn, tool = excluded.tool, at = excluded.at,
	input_bytes = excluded.input_bytes,
	result_bytes = COALESCE(excluded.result_bytes, tool_calls.result_bytes),
	path = CASE WHEN excluded.path != '' THEN excluded.path ELSE tool_calls.path END
`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	late, err := tx.Prepare(`UPDATE tool_calls SET result_bytes = ? WHERE vendor = ? AND session_id = ? AND call_id = ? AND result_bytes IS NULL`)
	if err != nil {
		return err
	}
	defer late.Close()

	for _, c := range calls {
		if c.Tool == "" {
			if c.ResultBytes != nil {
				if _, err := late.Exec(*c.ResultBytes, c.Vendor, c.SessionID, c.CallID); err != nil {
					return fmt.Errorf("store: tool call result %s/%s: %w", c.Vendor, c.CallID, err)
				}
			}
			continue
		}
		_, err = stmt.Exec(
			c.Vendor, c.Agent, c.SessionID, c.CallID, c.Turn, c.Tool,
			c.At.UTC().Format(time.RFC3339Nano), c.InputBytes, nullInt(c.ResultBytes), c.Path,
		)
		if err != nil {
			return fmt.Errorf("store: upsert tool call %s/%s: %w", c.Vendor, c.CallID, err)
		}
	}
	return tx.Commit()
}

// ToolCallsForTurn returns every tool call at or after cutoff (S2's
// tool_calls.at) for (vendor, session_id, turn), oldest first: the I3
// drawer's "tool calls that turn" list. turn is the calling turn's own
// Event.RequestID (Claude) or rollout turn_id (Codex); a Codex turn_id whose
// id space does not actually match its Event.RequestID (unverified, no
// fixture proves the two coincide) simply returns no rows rather than
// erroring, same as a turn that truly called no tool.
func (s *Store) ToolCallsForTurn(vendor, sessionID, turn string) ([]schema.ToolCall, error) {
	rows, err := s.readDB.Query(`
SELECT vendor, agent, session_id, call_id, turn, tool, at, input_bytes, result_bytes, path
FROM tool_calls
WHERE vendor = ? AND session_id = ? AND turn = ?
ORDER BY at ASC`, vendor, sessionID, turn)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []schema.ToolCall
	for rows.Next() {
		var c schema.ToolCall
		var atStr string
		var resultBytes sql.NullInt64
		if err := rows.Scan(&c.Vendor, &c.Agent, &c.SessionID, &c.CallID, &c.Turn, &c.Tool,
			&atStr, &c.InputBytes, &resultBytes, &c.Path); err != nil {
			return nil, err
		}
		c.At, err = time.Parse(time.RFC3339Nano, atStr)
		if err != nil {
			return nil, fmt.Errorf("store: parse at %q: %w", atStr, err)
		}
		if resultBytes.Valid {
			v := resultBytes.Int64
			c.ResultBytes = &v
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// LatestToolCalls returns each key's newest tool call by its real instant,
// in one query for every key (the Station reads this once per tick, never
// once per session). Ordered by julianday(at), not the stored text:
// RFC3339Nano trims trailing zeros, so "10:00:00Z" sorts after
// "10:00:00.5Z" as text. A key with no calls is absent from the map. Ties
// on the same instant pick SQLite's first row of the group (bare columns
// next to MAX), which is fine for a room picker. Returns an empty map for
// empty keys.
func (s *Store) LatestToolCalls(keys []SessionKey) (map[SessionKey]schema.ToolCall, error) {
	out := map[SessionKey]schema.ToolCall{}
	if len(keys) == 0 {
		return out, nil
	}
	clauses := make([]string, len(keys))
	args := make([]any, 0, len(keys)*2)
	for i, k := range keys {
		clauses[i] = "(vendor = ? AND session_id = ?)"
		args = append(args, k.Vendor, k.SessionID)
	}
	rows, err := s.readDB.Query(`
SELECT vendor, agent, session_id, call_id, turn, tool, at, input_bytes, result_bytes, path, MAX(julianday(at))
FROM tool_calls
WHERE `+strings.Join(clauses, " OR ")+`
GROUP BY vendor, session_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c schema.ToolCall
		var atStr string
		var resultBytes sql.NullInt64
		var maxKey any
		if err := rows.Scan(&c.Vendor, &c.Agent, &c.SessionID, &c.CallID, &c.Turn, &c.Tool,
			&atStr, &c.InputBytes, &resultBytes, &c.Path, &maxKey); err != nil {
			return nil, err
		}
		c.At, err = time.Parse(time.RFC3339Nano, atStr)
		if err != nil {
			return nil, fmt.Errorf("store: parse at %q: %w", atStr, err)
		}
		if resultBytes.Valid {
			v := resultBytes.Int64
			c.ResultBytes = &v
		}
		out[SessionKey{Vendor: c.Vendor, SessionID: c.SessionID}] = c
	}
	return out, rows.Err()
}

// ToolCallTotal is one tool name's aggregated activity since a cutoff, as
// `burnmon-cli tools` and (later) the Tools tab both need it.
type ToolCallTotal struct {
	Tool        string `json:"tool"`
	Calls       int64  `json:"calls"`
	Sessions    int64  `json:"sessions"`
	InputBytes  int64  `json:"input_bytes"`
	ResultBytes int64  `json:"result_bytes"`
}

// ToolCallTotals returns one row per tool name for every call at or after
// since, ordered by call count descending. result_bytes sums only the calls
// that have one recorded; a tool whose calls never carried a result (an
// orphaned call_id, see schema.ToolCall) still gets a row, with
// result_bytes 0.
func (s *Store) ToolCallTotals(since time.Time) ([]ToolCallTotal, error) {
	rows, err := s.readDB.Query(`
SELECT tool, COUNT(*) AS calls, COUNT(DISTINCT session_id) AS sessions,
	SUM(input_bytes) AS input_bytes, SUM(COALESCE(result_bytes, 0)) AS result_bytes
FROM tool_calls
WHERE at >= ?
GROUP BY tool
ORDER BY calls DESC`, since.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ToolCallTotal
	for rows.Next() {
		var t ToolCallTotal
		if err := rows.Scan(&t.Tool, &t.Calls, &t.Sessions, &t.InputBytes, &t.ResultBytes); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// VendorStripTotal is one agent's token totals for the Now page's vendor
// strip (v0.2 P3): today, this week and this month, all as of the caller's
// day/week/month boundaries.
type VendorStripTotal struct {
	Agent string
	Today int64
	Week  int64
	Month int64
}

// VendorStripTotals runs one SQL query, grouped by agent, summing each
// event's token count (input + cache_write + cache_read + output) into
// today/week/month buckets via CASE, rather than loading events into Go and
// summing there (P3: refreshed once a minute, from its own timer, not
// bmLive's 2-second poll). from bounds the scan to the earliest of the three
// boundaries the caller passes (the current ISO week can start before the
// current calendar month, so it is not always monthStart).
func (s *Store) VendorStripTotals(day, week, month time.Time) ([]VendorStripTotal, error) {
	from := day
	if week.Before(from) {
		from = week
	}
	if month.Before(from) {
		from = month
	}
	rows, err := s.readDB.Query(`
SELECT agent,
	SUM(CASE WHEN at >= ? THEN input + COALESCE(cache_write,0) + COALESCE(cache_read,0) + output ELSE 0 END) AS today,
	SUM(CASE WHEN at >= ? THEN input + COALESCE(cache_write,0) + COALESCE(cache_read,0) + output ELSE 0 END) AS week,
	SUM(CASE WHEN at >= ? THEN input + COALESCE(cache_write,0) + COALESCE(cache_read,0) + output ELSE 0 END) AS month
FROM events
WHERE at >= ?
GROUP BY agent`,
		day.UTC().Format(time.RFC3339Nano), week.UTC().Format(time.RFC3339Nano),
		month.UTC().Format(time.RFC3339Nano), from.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []VendorStripTotal
	for rows.Next() {
		var t VendorStripTotal
		if err := rows.Scan(&t.Agent, &t.Today, &t.Week, &t.Month); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ForecastScore is one ISO week's F1 scoring row: the plan recorded (meant
// to be) on Monday, and the actual once that week has fully elapsed.
// ActualTokens and ScoredAt are nil until then; a week "counts as scored"
// once both are set.
type ForecastScore struct {
	ISOYear      int
	ISOWeek      int
	WeekStart    time.Time
	PlanTokens   int64
	ActualTokens *int64
	ScoredAt     *time.Time
}

const forecastScoreColumns = `iso_year, iso_week, week_start, plan_tokens, actual_tokens, scored_at`

func scanForecastScore(scan func(dest ...any) error) (ForecastScore, error) {
	var f ForecastScore
	var weekStartStr string
	var actual sql.NullInt64
	var scoredAt sql.NullString
	if err := scan(&f.ISOYear, &f.ISOWeek, &weekStartStr, &f.PlanTokens, &actual, &scoredAt); err != nil {
		return ForecastScore{}, err
	}
	var err error
	f.WeekStart, err = time.Parse(time.RFC3339Nano, weekStartStr)
	if err != nil {
		return ForecastScore{}, fmt.Errorf("store: parse week_start %q: %w", weekStartStr, err)
	}
	// .In(time.Local), not left as the UTC Location time.Parse gives a
	// string with a "Z"/"+00:00" suffix: WeekStart names a local midnight
	// instant (forecast.weekStart computes it via time.Local), and callers
	// do AddDate arithmetic on it directly (forecast.go's EnsureScored and
	// actualTokensForWeek). AddDate reconstructs a Time via its receiver's
	// own Location, so doing that arithmetic while still UTC-located adds
	// UTC calendar days (always exactly 24h, no DST) instead of local
	// calendar days (23h/25h on a transition's own day), landing one hour
	// short of the real week boundary on a fall-back week and silently
	// dropping that week's last day from the recorded actual. Converting
	// here, once, at the point every stored local-decision instant comes
	// back from the store, is safer than trusting every call site to
	// remember .In(time.Local) before its own arithmetic.
	f.WeekStart = f.WeekStart.In(time.Local)
	if actual.Valid {
		v := actual.Int64
		f.ActualTokens = &v
	}
	if scoredAt.Valid {
		t, err := time.Parse(time.RFC3339Nano, scoredAt.String)
		if err == nil {
			f.ScoredAt = &t
		}
	}
	return f, nil
}

// ForecastScore returns the row for one ISO week, ok false when that week
// has never had a plan recorded.
func (s *Store) ForecastScore(isoYear, isoWeek int) (ForecastScore, bool, error) {
	row := s.readDB.QueryRow(`SELECT `+forecastScoreColumns+` FROM forecast_scores WHERE iso_year = ? AND iso_week = ?`,
		isoYear, isoWeek)
	f, err := scanForecastScore(row.Scan)
	if err == sql.ErrNoRows {
		return ForecastScore{}, false, nil
	}
	if err != nil {
		return ForecastScore{}, false, err
	}
	return f, true, nil
}

// ForecastScores returns every scored-or-scoring week, oldest first.
func (s *Store) ForecastScores() ([]ForecastScore, error) {
	rows, err := s.readDB.Query(`SELECT ` + forecastScoreColumns + ` FROM forecast_scores ORDER BY iso_year, iso_week`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ForecastScore
	for rows.Next() {
		f, err := scanForecastScore(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// InsertForecastPlan records planTokens as the week's forecast, the first
// time that ISO week is seen only: a later call the same week (this same
// process re-checking, or a restart) is a silent no-op, so the Monday
// forecast is never overwritten once made.
func (s *Store) InsertForecastPlan(isoYear, isoWeek int, weekStart time.Time, planTokens int64) error {
	_, err := s.db.Exec(`
INSERT INTO forecast_scores (iso_year, iso_week, week_start, plan_tokens)
VALUES (?, ?, ?, ?)
ON CONFLICT (iso_year, iso_week) DO NOTHING`,
		isoYear, isoWeek, weekStart.UTC().Format(time.RFC3339Nano), planTokens)
	return err
}

// RecordForecastActual fills in a week's actual once it has fully elapsed,
// only when it has not been recorded already (the WHERE guards against a
// second scoring pass re-running the same week).
func (s *Store) RecordForecastActual(isoYear, isoWeek int, actualTokens int64, scoredAt time.Time) error {
	_, err := s.db.Exec(`
UPDATE forecast_scores SET actual_tokens = ?, scored_at = ?
WHERE iso_year = ? AND iso_week = ? AND actual_tokens IS NULL`,
		actualTokens, scoredAt.UTC().Format(time.RFC3339Nano), isoYear, isoWeek)
	return err
}

// DailyTokenTotal is one local calendar day's total tokens (Wilco's
// decision, 2026-09-26: local time everywhere) across every real turn (the
// claude adapter's synthetic tool-only events excluded, same condition as
// SessionTotals).
type DailyTokenTotal struct {
	Date   string // YYYY-MM-DD, local calendar day
	Tokens int64
}

// DailyTokenTotals sums tokens per LOCAL calendar day for every real turn at
// or after from, oldest first (no upper bound; DailyTokenTotalsUntil adds
// one, for a caller that already knows its own range). F1's plan line
// (weekday-aware averages over the last four weeks) and its actual/live
// totals are both built from these.
//
// Bucketed in Go, not SQL: SQLite has no IANA timezone database, so a query
// cannot correctly turn a UTC instant into "which local calendar day" across
// a DST transition (a fixed offset, the only thing SQL here could apply,
// would mis-bucket events on the transition's own day, which is 23 or 25
// hours long in local time, not 24; verified by
// TestDailyTokenTotals_DSTEarlyMorning). from is itself typically 4-5 weeks
// back (F1's plan/live windows), so reading every matching row and grouping
// in Go costs at most a few thousand rows: nothing like the whole-table
// cost step 0 profiling measured for AllEvents (WS3 hypothesis 3, not
// confirmed as a driver of the reported symptom, so left as SQL aggregation
// everywhere it still was: see
// 04_assets\2026-09-26_ws3_profile_before_after.md).
func (s *Store) DailyTokenTotals(from time.Time) ([]DailyTokenTotal, error) {
	return s.dailyTokenTotals(from, time.Time{})
}

// DailyTokenTotalsUntil is DailyTokenTotals bounded above by to (exclusive),
// for a caller that already knows it only needs one specific range (a
// review of this session's own diff caught that actualTokensForWeek used to
// call DailyTokenTotals(ws) with no upper bound at all, unboundedly
// rescanning from ws to "now" every time EnsureScored found an unscored
// week, an O(weeks x rows-since-ws) cost after any real backlog).
func (s *Store) DailyTokenTotalsUntil(from, to time.Time) ([]DailyTokenTotal, error) {
	return s.dailyTokenTotals(from, to)
}

func (s *Store) dailyTokenTotals(from, to time.Time) ([]DailyTokenTotal, error) {
	query := `
SELECT at, input + COALESCE(cache_write,0) + COALESCE(cache_read,0) + output AS tokens
FROM events
WHERE at >= ? AND NOT (model = '' AND input = 0 AND output = 0)`
	args := []any{from.UTC().Format(time.RFC3339Nano)}
	if !to.IsZero() {
		query += ` AND at < ?`
		args = append(args, to.UTC().Format(time.RFC3339Nano))
	}
	query += `
ORDER BY at ASC`
	rows, err := s.readDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byDay := map[string]int64{}
	var order []string
	for rows.Next() {
		var atStr string
		var tokens int64
		if err := rows.Scan(&atStr, &tokens); err != nil {
			return nil, err
		}
		at, err := time.Parse(time.RFC3339Nano, atStr)
		if err != nil {
			continue
		}
		day := at.In(time.Local).Format("2006-01-02")
		if _, ok := byDay[day]; !ok {
			order = append(order, day)
		}
		byDay[day] += tokens
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]DailyTokenTotal, 0, len(order))
	for _, day := range order {
		out = append(out, DailyTokenTotal{Date: day, Tokens: byDay[day]})
	}
	return out, nil
}

func nullInt(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullFloat(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

const eventColumns = `vendor, agent, surface, session_id, request_id, parent_id, at, model,
	project, title, input, cache_write, cache_read, output, reasoning,
	vendor_cost, window_used, window_reset, tools, owner, client`

// scanEvents reads every row of rows (already SELECTed with eventColumns'
// exact column list and order) into Events. Shared by AllEvents and
// EventsSince so the two queries' row-decoding logic (nullable columns,
// the tools JSON blob, RFC3339Nano timestamps) is written once.
func scanEvents(rows *sql.Rows) ([]schema.Event, error) {
	defer rows.Close()
	var out []schema.Event
	for rows.Next() {
		var e schema.Event
		var atStr string
		var cacheWrite, cacheRead, reasoning sql.NullInt64
		var vendorCost, windowUsed sql.NullFloat64
		var windowReset sql.NullString
		var toolsJSON string
		if err := rows.Scan(&e.Vendor, &e.Agent, &e.Surface, &e.SessionID, &e.RequestID,
			&e.ParentID, &atStr, &e.Model, &e.Project, &e.Title, &e.Input,
			&cacheWrite, &cacheRead, &e.Output, &reasoning, &vendorCost, &windowUsed,
			&windowReset, &toolsJSON, &e.Owner, &e.Client); err != nil {
			return nil, err
		}
		var err error
		e.At, err = time.Parse(time.RFC3339Nano, atStr)
		if err != nil {
			return nil, fmt.Errorf("store: parse at %q: %w", atStr, err)
		}
		if cacheWrite.Valid {
			v := cacheWrite.Int64
			e.CacheWrite = &v
		}
		if cacheRead.Valid {
			v := cacheRead.Int64
			e.CacheRead = &v
		}
		if reasoning.Valid {
			v := reasoning.Int64
			e.Reasoning = &v
		}
		if vendorCost.Valid {
			v := vendorCost.Float64
			e.VendorCost = &v
		}
		if windowUsed.Valid {
			v := windowUsed.Float64
			e.WindowUsed = &v
		}
		if windowReset.Valid {
			t, err := time.Parse(time.RFC3339Nano, windowReset.String)
			if err == nil {
				e.WindowReset = &t
			}
		}
		if toolsJSON != "" && toolsJSON != "{}" {
			if err := json.Unmarshal([]byte(toolsJSON), &e.Tools); err != nil {
				return nil, fmt.Errorf("store: unmarshal tools: %w", err)
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AllEvents returns every stored event, oldest first by At. v0.1 scale
// (a developer's own machine, at most a few hundred thousand turns) makes
// loading the whole table fine for the CLI's one-shot report; the Now
// page's live poll uses EventsSince instead (see F1, SESSION_LOG.md), since
// polling AllEvents every 2 seconds re-groups the whole table in memory on
// every tick.
func (s *Store) AllEvents() ([]schema.Event, error) {
	rows, err := s.readDB.Query(`SELECT ` + eventColumns + ` FROM events ORDER BY at ASC`)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// EventsSince returns every stored event whose At is at or after from,
// oldest first, using the idx_events_at index. The Now page's live poll
// (bmLive) uses this with from = now - live.ChartWindow instead of
// AllEvents, so a 2-second poll only ever groups the chart window's worth
// of rows, not the whole history (F1: burnmon.exe was measured at 1,886 MB
// and thrashing before this fix).
func (s *Store) EventsSince(from time.Time) ([]schema.Event, error) {
	rows, err := s.readDB.Query(`SELECT `+eventColumns+` FROM events WHERE at >= ? ORDER BY at ASC`,
		from.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// EventsForSession returns every stored event for sessionID, any vendor,
// oldest first: `burnmon-cli insight <session-id>` (v0.2 I1) reads one
// session's whole history this way rather than through AllEvents, the same
// "windowed, not whole-table" instinct EventsSince already follows for the
// Now page. A bare session_id is not guaranteed unique across vendors
// (SessionKey's own doc comment), so a session id shared by two vendors
// returns both; insight.Analyze still groups by turn order within the
// result, which read as multiplexed turns of two sessions is a display bug,
// not a correctness one, no vendor has ever repeated another's id in
// practice.
func (s *Store) EventsForSession(sessionID string) ([]schema.Event, error) {
	rows, err := s.readDB.Query(`SELECT `+eventColumns+` FROM events WHERE session_id = ? ORDER BY at ASC`,
		sessionID)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// SessionModelTotal is one (vendor, session_id, model) group's SQL-summed
// token counts, turn count and time span. A running session can switch
// model mid-session, so SessionTotals groups by model too rather than
// collapsing straight to one row per session; ApplySessionTotals (internal/live)
// sums cost per group (both cost formulas are linear in token counts, so
// this is exact, not an approximation) then adds the groups together.
type SessionModelTotal struct {
	Vendor     string
	SessionID  string
	Model      string
	Input      int64
	CacheWrite int64
	CacheRead  int64
	Output     int64
	Reasoning  int64
	Count      int64
	MinAt      time.Time
	MaxAt      time.Time
}

// SessionKey identifies one running session by (Vendor, SessionID): a bare
// session_id is not unique across vendors (two adapters can in principle
// mint the same id), so SessionTotals is looked up by the vendor-qualified
// pair rather than session_id alone.
type SessionKey struct {
	Vendor    string
	SessionID string
}

// SessionTotals returns one row per (vendor, session_id, model) group for
// every (vendor, session_id) pair in keys, SQL-summed across the whole
// events table (not windowed), so a session's lifetime Tokens/Cost/Start
// stay correct even though the Now page's live poll only re-reads the last
// ChartWindow of events for everything else (F1). The claude adapter's
// synthetic tool-only events (model="" and input=output=0) are excluded,
// matching internal/live's own isTurn filter. Returns nil, nil for an empty
// keys.
func (s *Store) SessionTotals(keys []SessionKey) ([]SessionModelTotal, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	clauses := make([]string, len(keys))
	args := make([]any, 0, len(keys)*2)
	for i, k := range keys {
		clauses[i] = "(vendor = ? AND session_id = ?)"
		args = append(args, k.Vendor, k.SessionID)
	}
	query := `
SELECT vendor, session_id, model,
	SUM(input) AS input, SUM(COALESCE(cache_write,0)) AS cache_write,
	SUM(COALESCE(cache_read,0)) AS cache_read, SUM(output) AS output,
	SUM(COALESCE(reasoning,0)) AS reasoning, COUNT(*) AS cnt,
	MIN(at) AS min_at, MAX(at) AS max_at
FROM events
WHERE (` + strings.Join(clauses, " OR ") + `)
	AND NOT (model = '' AND input = 0 AND output = 0)
GROUP BY vendor, session_id, model`
	rows, err := s.readDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SessionModelTotal
	for rows.Next() {
		var t SessionModelTotal
		var minAtStr, maxAtStr string
		if err := rows.Scan(&t.Vendor, &t.SessionID, &t.Model, &t.Input, &t.CacheWrite,
			&t.CacheRead, &t.Output, &t.Reasoning, &t.Count, &minAtStr, &maxAtStr); err != nil {
			return nil, err
		}
		t.MinAt, err = time.Parse(time.RFC3339Nano, minAtStr)
		if err != nil {
			return nil, fmt.Errorf("store: parse min_at %q: %w", minAtStr, err)
		}
		t.MaxAt, err = time.Parse(time.RFC3339Nano, maxAtStr)
		if err != nil {
			return nil, fmt.Errorf("store: parse max_at %q: %w", maxAtStr, err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Cursor reports how far path has been read: ok is false when path has no
// cursor row yet (never seen before).
func (s *Store) Cursor(path string) (offset int64, mtime time.Time, size int64, ok bool, err error) {
	row := s.db.QueryRow(`SELECT offset, mtime, size FROM cursors WHERE path = ?`, path)
	var mtimeStr string
	err = row.Scan(&offset, &mtimeStr, &size)
	if err == sql.ErrNoRows {
		return 0, time.Time{}, 0, false, nil
	}
	if err != nil {
		return 0, time.Time{}, 0, false, err
	}
	mtime, err = time.Parse(time.RFC3339Nano, mtimeStr)
	if err != nil {
		return 0, time.Time{}, 0, false, err
	}
	return offset, mtime, size, true, nil
}

func (s *Store) SetCursor(path string, offset int64, mtime time.Time, size int64) error {
	_, err := s.db.Exec(`
INSERT INTO cursors (path, offset, mtime, size) VALUES (?, ?, ?, ?)
ON CONFLICT (path) DO UPDATE SET offset = excluded.offset, mtime = excluded.mtime, size = excluded.size`,
		path, offset, mtime.UTC().Format(time.RFC3339Nano), size)
	return err
}

// DeleteEventsForOtherPaths removes every stored event and cursor whose
// source transcript is not in keepPaths. burnmon reconstructs SessionID
// from a transcript's filename, but events carry no source-path column, so
// pruning by session id here would risk dropping a live session that
// merely shares an id pattern; instead the caller (dataset.ingest) is
// expected to pass every currently-resolved file path, and pruning targets
// cursors, whose path IS the source file, one-for-one with the events that
// file produced (same SessionID as its filename, per the claude adapter).
// A transcript that no longer exists (deleted, renamed, moved) therefore
// has its cursor removed here, and its events removed by SessionID in the
// same call, so it stops contributing to every future report.
func (s *Store) DeleteEventsForOtherPaths(keepPaths []string) error {
	rows, err := s.db.Query(`SELECT path FROM cursors`)
	if err != nil {
		return fmt.Errorf("store: list cursor paths: %w", err)
	}
	keep := make(map[string]bool, len(keepPaths))
	for _, p := range keepPaths {
		keep[p] = true
	}
	var stale []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return fmt.Errorf("store: scan cursor path: %w", err)
		}
		if !keep[p] {
			stale = append(stale, p)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: list cursor paths: %w", err)
	}
	rows.Close()
	if len(stale) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range stale {
		sessionID := sessionIDFromPath(p)
		if sessionID != "" {
			if _, err := tx.Exec(`DELETE FROM events WHERE session_id = ?`, sessionID); err != nil {
				return fmt.Errorf("store: delete events for %s: %w", p, err)
			}
		}
		if _, err := tx.Exec(`DELETE FROM cursors WHERE path = ?`, p); err != nil {
			return fmt.Errorf("store: delete cursor for %s: %w", p, err)
		}
	}
	return tx.Commit()
}

// rowsAffected reads a delete's row count and returns the driver's error when
// it cannot report one, so the caller rolls back instead of reporting zero.
func rowsAffected(res sql.Result) (int, error) {
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// DeleteEventsBefore removes events and tool calls recorded before cutoff.
// It compares instants with julianday, not text: RFC3339Nano strings with and
// without a fractional second do not sort as the instants they name. Cursors
// are left alone, so transcripts still on disk are not read back in.
func (s *Store) DeleteEventsBefore(cutoff time.Time) (events, toolCalls int, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	at := cutoff.UTC().Format(time.RFC3339Nano)
	res, err := tx.Exec(`DELETE FROM tool_calls WHERE julianday(at) < julianday(?)`, at)
	if err != nil {
		return 0, 0, fmt.Errorf("store: delete tool calls before %s: %w", at, err)
	}
	if toolCalls, err = rowsAffected(res); err != nil {
		return 0, 0, fmt.Errorf("store: count deleted tool calls before %s: %w", at, err)
	}
	res, err = tx.Exec(`DELETE FROM events WHERE julianday(at) < julianday(?)`, at)
	if err != nil {
		return 0, 0, fmt.Errorf("store: delete events before %s: %w", at, err)
	}
	if events, err = rowsAffected(res); err != nil {
		return 0, 0, fmt.Errorf("store: count deleted events before %s: %w", at, err)
	}
	return events, toolCalls, tx.Commit()
}

// DeleteSession removes one session's events and tool calls, and those of its
// subagents (events whose parent_id is the session). Cursors are left alone.
func (s *Store) DeleteSession(sessionID string) (events, toolCalls int, err error) {
	if sessionID == "" {
		return 0, 0, fmt.Errorf("store: delete session: empty session id")
	}
	rows, err := s.db.Query(`SELECT DISTINCT session_id FROM events WHERE session_id = ? OR parent_id = ?`, sessionID, sessionID)
	if err != nil {
		return 0, 0, fmt.Errorf("store: list sessions to delete: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, 0, fmt.Errorf("store: scan session id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, 0, fmt.Errorf("store: list sessions to delete: %w", err)
	}
	rows.Close()

	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	for _, id := range ids {
		res, err := tx.Exec(`DELETE FROM tool_calls WHERE session_id = ?`, id)
		if err != nil {
			return 0, 0, fmt.Errorf("store: delete tool calls of %s: %w", id, err)
		}
		n, err := rowsAffected(res)
		if err != nil {
			return 0, 0, fmt.Errorf("store: count deleted tool calls of %s: %w", id, err)
		}
		toolCalls += n
		res, err = tx.Exec(`DELETE FROM events WHERE session_id = ?`, id)
		if err != nil {
			return 0, 0, fmt.Errorf("store: delete events of %s: %w", id, err)
		}
		n, err = rowsAffected(res)
		if err != nil {
			return 0, 0, fmt.Errorf("store: count deleted events of %s: %w", id, err)
		}
		events += n
	}
	return events, toolCalls, tx.Commit()
}

// DeleteAllEvents empties events and tool_calls. Cursors, meta, forecast and
// the graph tables stay, so a delete is not undone by the next scan.
func (s *Store) DeleteAllEvents() (events, toolCalls int, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`DELETE FROM tool_calls`)
	if err != nil {
		return 0, 0, fmt.Errorf("store: delete all tool calls: %w", err)
	}
	if toolCalls, err = rowsAffected(res); err != nil {
		return 0, 0, fmt.Errorf("store: count deleted tool calls: %w", err)
	}
	res, err = tx.Exec(`DELETE FROM events`)
	if err != nil {
		return 0, 0, fmt.Errorf("store: delete all events: %w", err)
	}
	if events, err = rowsAffected(res); err != nil {
		return 0, 0, fmt.Errorf("store: count deleted events: %w", err)
	}
	return events, toolCalls, tx.Commit()
}

// ForgetCursorsForOtherPaths deletes only the cursor rows whose path is not in
// keepPaths and returns how many. Events stay: this is keep_history's prune,
// where a vanished transcript stops being tracked but keeps its history.
func (s *Store) ForgetCursorsForOtherPaths(keepPaths []string) (int, error) {
	rows, err := s.db.Query(`SELECT path FROM cursors`)
	if err != nil {
		return 0, fmt.Errorf("store: list cursor paths: %w", err)
	}
	keep := make(map[string]bool, len(keepPaths))
	for _, p := range keepPaths {
		keep[p] = true
	}
	var stale []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return 0, fmt.Errorf("store: scan cursor path: %w", err)
		}
		if !keep[p] {
			stale = append(stale, p)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("store: list cursor paths: %w", err)
	}
	rows.Close()
	if len(stale) == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, p := range stale {
		if _, err := tx.Exec(`DELETE FROM cursors WHERE path = ?`, p); err != nil {
			return 0, fmt.Errorf("store: forget cursor for %s: %w", p, err)
		}
	}
	return len(stale), tx.Commit()
}

// ResetCollapsedCodexSessions undoes the Codex ":0" collapse (WS2
// follow-up, 2026-09-29, item 0): a rollout whose token_count lines carry
// no top-level "ordinal" used to key every turn as sessionID+":0", so the
// upsert kept one event for the whole session. A real token_count line
// never has ordinal 0 (that is always the session_meta line), so every
// Codex row keyed exactly sessionID+":0" is such a collapsed row. This
// deletes those rows and the cursors of their transcripts, so the next
// ingest re-reads each file from byte 0 under the fixed key; rows the same
// sessions already hold under a real ordinal key stay and are re-upserted
// unchanged. Returns how many rows and cursors it removed.
func (s *Store) ResetCollapsedCodexSessions() (events, cursors int, err error) {
	rows, err := s.db.Query(`SELECT session_id FROM events
WHERE vendor = 'openai' AND agent = 'codex' AND request_id = session_id || ':0'`)
	if err != nil {
		return 0, 0, fmt.Errorf("store: list collapsed codex sessions: %w", err)
	}
	collapsed := map[string]bool{}
	for rows.Next() {
		var sid string
		if err := rows.Scan(&sid); err != nil {
			rows.Close()
			return 0, 0, err
		}
		collapsed[sid] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, 0, err
	}
	rows.Close()
	if len(collapsed) == 0 {
		return 0, 0, nil
	}

	pathRows, err := s.db.Query(`SELECT path FROM cursors`)
	if err != nil {
		return 0, 0, fmt.Errorf("store: list cursor paths: %w", err)
	}
	var paths []string
	for pathRows.Next() {
		var p string
		if err := pathRows.Scan(&p); err != nil {
			pathRows.Close()
			return 0, 0, err
		}
		if collapsed[sessionIDFromPath(p)] {
			paths = append(paths, p)
		}
	}
	if err := pathRows.Err(); err != nil {
		pathRows.Close()
		return 0, 0, err
	}
	pathRows.Close()

	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	for sid := range collapsed {
		res, err := tx.Exec(`DELETE FROM events WHERE vendor = 'openai' AND agent = 'codex' AND session_id = ? AND request_id = ?`, sid, sid+":0")
		if err != nil {
			return 0, 0, fmt.Errorf("store: delete collapsed %s: %w", sid, err)
		}
		n, _ := res.RowsAffected()
		events += int(n)
	}
	for _, p := range paths {
		if _, err := tx.Exec(`DELETE FROM cursors WHERE path = ?`, p); err != nil {
			return 0, 0, fmt.Errorf("store: delete cursor for %s: %w", p, err)
		}
		cursors++
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return events, cursors, nil
}

// BackfillClaudeParents sets parent_id on Claude subagent rows ingested
// before the adapter read it (R6). A subagent's trail sits at
// <parent>\subagents\agent-<id>.jsonl, so the cursor path names the parent;
// a row whose trail has no cursor left is not touched. With apply false it
// only counts the rows it would update. Returns the row count.
func (s *Store) BackfillClaudeParents(apply bool) (int, error) {
	rows, err := s.db.Query(`SELECT path FROM cursors`)
	if err != nil {
		return 0, fmt.Errorf("store: list cursor paths: %w", err)
	}
	parentOf := map[string]string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return 0, err
		}
		sid := sessionIDFromPath(p)
		dir := filepath.Dir(p)
		if strings.HasPrefix(sid, "agent-") && filepath.Base(dir) == "subagents" {
			parentOf[sid] = filepath.Base(filepath.Dir(dir))
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if len(parentOf) == 0 {
		return 0, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	count, err := tx.Prepare(`SELECT COUNT(*) FROM events WHERE vendor = 'anthropic' AND session_id = ? AND parent_id = ''`)
	if err != nil {
		return 0, err
	}
	defer count.Close()
	update, err := tx.Prepare(`UPDATE events SET parent_id = ? WHERE vendor = 'anthropic' AND session_id = ? AND parent_id = ''`)
	if err != nil {
		return 0, err
	}
	defer update.Close()
	total := 0
	for sid, parent := range parentOf {
		if !apply {
			var n int
			if err := count.QueryRow(sid).Scan(&n); err != nil {
				return 0, fmt.Errorf("store: count %s: %w", sid, err)
			}
			total += n
			continue
		}
		res, err := update.Exec(parent, sid)
		if err != nil {
			return 0, fmt.Errorf("store: backfill parent of %s: %w", sid, err)
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}
	if !apply {
		return total, nil
	}
	return total, tx.Commit()
}

// sessionIDFromPath mirrors the claude adapter's own session id derivation
// (the transcript file's base name, extension stripped).
func sessionIDFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// ReownEvents recomputes every event's owner via ownerFor(project) (P6's
// light client map) and its client via clientFor(project) (K1, v0.3), re-run
// after an owner or client rule change, and updates every row whose owner or
// client actually changes, in one transaction. Returns the number of rows
// updated. Used by `burnmon-cli reown`.
func (s *Store) ReownEvents(ownerFor, clientFor func(project string) string) (int, error) {
	rows, err := s.db.Query(`SELECT vendor, session_id, request_id, project, owner, client FROM events`)
	if err != nil {
		return 0, fmt.Errorf("store: list events for reown: %w", err)
	}
	type update struct {
		vendor, sessionID, requestID, owner, client string
	}
	var updates []update
	for rows.Next() {
		var vendor, sessionID, requestID, project, owner, client string
		if err := rows.Scan(&vendor, &sessionID, &requestID, &project, &owner, &client); err != nil {
			rows.Close()
			return 0, fmt.Errorf("store: scan event for reown: %w", err)
		}
		wantOwner := ownerFor(project)
		wantClient := clientFor(project)
		if wantOwner != owner || wantClient != client {
			updates = append(updates, update{vendor, sessionID, requestID, wantOwner, wantClient})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("store: list events for reown: %w", err)
	}
	rows.Close()
	if len(updates) == 0 {
		return 0, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`UPDATE events SET owner = ?, client = ? WHERE vendor = ? AND session_id = ? AND request_id = ?`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	for _, u := range updates {
		if _, err := stmt.Exec(u.owner, u.client, u.vendor, u.sessionID, u.requestID); err != nil {
			return 0, fmt.Errorf("store: reown %s/%s: %w", u.vendor, u.requestID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(updates), nil
}

func (s *Store) Meta(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return v, err == nil, err
}

func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`
INSERT INTO meta (key, value) VALUES (?, ?)
ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// DefaultPath is %LOCALAPPDATA%\burnmon\burnmon.db on Windows, the same
// folder the parse cache and app log already use; ~/.config/burnmon/burnmon.db
// on macOS and Linux via os.UserConfigDir.
func DefaultPath() (string, error) {
	if runtime.GOOS == "windows" {
		if lad := os.Getenv("LOCALAPPDATA"); lad != "" {
			return filepath.Join(lad, "burnmon", "burnmon.db"), nil
		}
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "burnmon", "burnmon.db"), nil
}

// PurgeAuditSessions removes what the live watcher's audit.jsonl bug left
// in the store (found 2026-10-12): Claude events and tool calls under
// session key "audit", which is a local Cowork session's audit.jsonl read
// as a session of its own and so counted twice, plus the cursor of every
// audit.jsonl file. The watcher now skips that file (scan.IsTranscriptPath),
// so nothing re-creates these rows. Run on every Collect, like
// ResetCollapsedCodexSessions: a burnmon.exe built before the fix shares
// this store. One indexed pass, a no-op once the store is clean.
func (s *Store) PurgeAuditSessions() (events, toolCalls, cursors int, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`DELETE FROM events WHERE vendor = 'anthropic' AND session_id = 'audit'`)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("store: delete audit events: %w", err)
	}
	n, _ := res.RowsAffected()
	events = int(n)
	res, err = tx.Exec(`DELETE FROM tool_calls WHERE vendor = 'anthropic' AND session_id = 'audit'`)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("store: delete audit tool calls: %w", err)
	}
	n, _ = res.RowsAffected()
	toolCalls = int(n)
	res, err = tx.Exec(`DELETE FROM cursors WHERE lower(replace(path, '\', '/')) LIKE '%/audit.jsonl' OR lower(path) = 'audit.jsonl'`)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("store: delete audit cursors: %w", err)
	}
	n, _ = res.RowsAffected()
	cursors = int(n)
	if err := tx.Commit(); err != nil {
		return 0, 0, 0, err
	}
	return events, toolCalls, cursors, nil
}

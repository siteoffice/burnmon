// Package migrations lists burnmon's store schema migrations in order.
// Every migration is additive only: add a table, add a column, or backfill
// data; none may drop or rewrite the events table. internal/store.Open runs
// every migration whose Version is greater than the store's recorded
// version, all inside one transaction, then records the new version.
package migrations

import "database/sql"

// Migration is one numbered, additive schema step.
type Migration struct {
	Version int
	Name    string
	Up      func(tx *sql.Tx) error
}

// All is every migration, in order. Version numbers start at 1 and are
// contiguous.
var All = []Migration{
	{1, "baseline v0.1 schema (events, cursors, meta)", migration1},
	{2, "owner column on events (P6 owner split)", migration2},
	{3, "tool_calls table (S2)", migration3},
	{4, "tool_calls.path column (I3)", migration4},
	{5, "forecast_scores table (F1)", migration5},
	{6, "session_id-only index on events (v0.2.1 hang patch)", migration6},
	{7, "client column on events (v0.3 K1 client map)", migration7},
	{8, "graph_nodes and graph_edges tables (R6 relation graph)", migration8},
}

// migration1 records the v0.1 schema as version 1 without changing it: the
// same CREATE TABLE IF NOT EXISTS statements store.go always ran on every
// startup, so a real v0.1 database on disk (already at this shape) is left
// untouched and a brand-new database gets the same tables.
func migration1(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS events (
	vendor      TEXT NOT NULL,
	agent       TEXT NOT NULL,
	surface     TEXT NOT NULL,
	session_id  TEXT NOT NULL,
	request_id  TEXT NOT NULL,
	parent_id   TEXT NOT NULL DEFAULT '',
	at          TEXT NOT NULL,
	model       TEXT NOT NULL,
	project     TEXT NOT NULL DEFAULT '',
	title       TEXT NOT NULL DEFAULT '',
	input       INTEGER NOT NULL,
	cache_write INTEGER,
	cache_read  INTEGER,
	output      INTEGER NOT NULL,
	reasoning   INTEGER,
	vendor_cost REAL,
	window_used REAL,
	window_reset TEXT,
	tools       TEXT NOT NULL DEFAULT '{}',
	PRIMARY KEY (vendor, session_id, request_id)
);
CREATE INDEX IF NOT EXISTS idx_events_session ON events (vendor, session_id);
CREATE INDEX IF NOT EXISTS idx_events_at ON events (at);
CREATE TABLE IF NOT EXISTS cursors (
	path   TEXT PRIMARY KEY,
	offset INTEGER NOT NULL,
	mtime  TEXT NOT NULL,
	size   INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`)
	return err
}

// migration2 adds P6's owner column: the light client map is applied at
// ingest and stored per event, empty by default, so a store with no owner
// rules configured carries owner = '' on every row.
func migration2(tx *sql.Tx) error {
	_, err := tx.Exec(`ALTER TABLE events ADD COLUMN owner TEXT NOT NULL DEFAULT ''`)
	return err
}

// migration3 adds S2's tool_calls table: one row per tool invocation, keyed
// by the vendor's own call id so a call and its later-arriving result can be
// upserted onto the same row. result_bytes is nullable: a call whose result
// was never seen in the same read (see schema.ToolCall) keeps it nil rather
// than 0, so "no result yet" stays distinguishable from "an empty result".
func migration3(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS tool_calls (
	vendor       TEXT NOT NULL,
	agent        TEXT NOT NULL,
	session_id   TEXT NOT NULL,
	call_id      TEXT NOT NULL,
	turn         TEXT NOT NULL,
	tool         TEXT NOT NULL,
	at           TEXT NOT NULL,
	input_bytes  INTEGER NOT NULL,
	result_bytes INTEGER,
	PRIMARY KEY (vendor, session_id, call_id)
);
CREATE INDEX IF NOT EXISTS idx_tool_calls_at ON tool_calls (at);
CREATE INDEX IF NOT EXISTS idx_tool_calls_tool ON tool_calls (tool);
`)
	return err
}

// migration4 adds I3's tool_calls.path column: the file path a Read, Edit,
// Write or NotebookEdit call touched, when the adapter could extract one
// from the tool's own input, "" otherwise (a shell/exec call, a search, or a
// trail whose input shape was not recognised).
func migration4(tx *sql.Tx) error {
	_, err := tx.Exec(`ALTER TABLE tool_calls ADD COLUMN path TEXT NOT NULL DEFAULT ''`)
	return err
}

// migration5 adds F1's forecast_scores table: one row per ISO week, the
// plan_tokens recorded the first time that week is seen (meant to be
// Monday, but a store that starts scoring mid-week records it the first
// time it runs instead of waiting for next Monday), actual_tokens and
// scored_at filled in once, after that week has fully elapsed. A week
// counts as scored (the forecast gate's condition) once both columns are
// non-null.
func migration5(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS forecast_scores (
	iso_year      INTEGER NOT NULL,
	iso_week      INTEGER NOT NULL,
	week_start    TEXT NOT NULL,
	plan_tokens   INTEGER NOT NULL,
	actual_tokens INTEGER,
	scored_at     TEXT,
	PRIMARY KEY (iso_year, iso_week)
);
`)
	return err
}

// migration6 adds a session_id-only index: EventsForSession and
// EventsForSession-like lookups filter WHERE session_id = ? with no vendor
// in the WHERE clause, so idx_events_session (vendor, session_id) cannot be
// used (session_id is not its leading column) and every such call fell back
// to a full table scan. Measured the v0.2.1 hang patch's dominant cost: the
// Sessions tab's bmSessionInsight fan-out (one EventsForSession call per
// kept session, all on the app's single UI thread) turned into hundreds of
// full scans of the whole events table. See SESSION_LOG.md.
func migration6(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_events_session_id ON events (session_id);`)
	return err
}

// migration7 adds K1's client column: the same shape as migration2's owner
// column (empty default, applied at ingest, reown re-applies it after a
// rule change). A v0.2 store or a v0.2 burnmon.json with owners-only rules
// keeps every event's client at '' until the next reown, at which point
// ClientFor's "unassigned" default takes over (Owners non-empty, no rule
// sets a Client).
func migration7(tx *sql.Tx) error {
	_, err := tx.Exec(`ALTER TABLE events ADD COLUMN client TEXT NOT NULL DEFAULT ''`)
	return err
}

// migration8 adds R6's relation graph history: the nodes and edges BurnMon
// sees only live (a process and what it started, an MCP server, a listening
// port, a Foreman link, who waits on you), so the map can show them after
// they are gone. Edges the events table can rebuild (a subagent's parent, a
// session's harness) are not stored here. One row per edge (from, to, kind)
// with the first and last time it was seen; times are RFC3339 UTC to the
// second, so they compare as text.
func migration8(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS graph_nodes (
	id         TEXT PRIMARY KEY,
	kind       TEXT NOT NULL,
	label      TEXT NOT NULL,
	vendor     TEXT NOT NULL DEFAULT '',
	attrs      TEXT NOT NULL DEFAULT '{}',
	source     TEXT NOT NULL,
	first_seen TEXT NOT NULL,
	last_seen  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS graph_edges (
	from_id    TEXT NOT NULL,
	to_id      TEXT NOT NULL,
	kind       TEXT NOT NULL,
	basis      TEXT NOT NULL,
	source     TEXT NOT NULL,
	first_seen TEXT NOT NULL,
	last_seen  TEXT NOT NULL,
	PRIMARY KEY (from_id, to_id, kind)
);
CREATE INDEX IF NOT EXISTS idx_graph_edges_last_seen ON graph_edges (last_seen);
`)
	return err
}

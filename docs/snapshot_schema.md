# BurnMon snapshot file, schema version 1 (frozen)

BurnMon core (`burnmon.exe`) rewrites one JSON file every tick (5 seconds):

    %USERPROFILE%\.burnmon\snapshot.json

It writes a temp file in the same folder, then renames it over the old one, so a reader
sees the old file or the new one, never half of one. The file is read-only data for other
tools (the Fleet mod). No port, no model call, no tokens. Code: `internal\snapshot\snapshot.go`.
Example: `testdata\snapshot_example.json`.

## Is BurnMon running

The file is stale when `written_at` is older than three ticks (`3 * tick_seconds`). A stale
or missing file means BurnMon is not running. Say that plainly, do not show old numbers as
current.

## Compatibility rule

Version 1 is frozen. Later rows (R6 adds `nodes[]` and `edges[]`) add new top-level fields
only. They never rename, remove or retype a version 1 field. Readers ignore fields they do
not know. A change that breaks a reader bumps `schema_version`.

## Fields

Every list is always an array, never null. Times are RFC3339 UTC. Every item carries its
`source` (`store` for BurnMon's own store, `sysmon` for the process sampler) and its
`age_seconds`.

| Field | Meaning |
|---|---|
| `schema_version` | 1 |
| `written_at` | when this tick was written |
| `tick_seconds` | the write interval |
| `sources[]` | `name`, `ok`, `read_at`, `error` (only when not ok). One entry for `store`, one for `sysmon`. A failing source is reported here and the file is still written |
| `sessions[]` | running sessions and subagents, flat. See below |
| `burn` | token rate over the last 5 minutes, all sessions |
| `processes[]` | per harness: `harness`, `cpu_pct`, `mem_mb`, `io_bps`, `source`, `age_seconds` |
| `waits_on_you[]` | sessions whose turn ended and who wait for a person: `vendor`, `session_id`, `project`, `since`, `waiting_seconds`, `source`, `age_seconds` |

### sessions[]

`vendor`, `agent`, `id`, `parent_id` (subagents only), `project`, `model`, `state`,
`state_since`, `tokens` (whole session), `cost`, `last_activity`, `source`, `age_seconds`
(seconds since `last_activity`).

`state` is one of the Station stages: `arriving`, `reading`, `fetching`, `planning`,
`coding`, `running`, `thinking`, `delegating`, `waiting`, `dormant`, `compacting`. It is
inferred from tool calls and silence, never observed (see `internal\stage\stage.go`).
`waiting` means the turn ended and the person is up.

### cost

`{ "usd", "eur", "label" }`. The label is always `API price equivalent`: what the tokens would
cost at API list price, not what anyone was billed. `eur` is `usd` times the configured rate
(`fx_usd_eur`, default 0.92). Show tokens first, then plan limits, then this figure.

### burn

`tokens_last_5min`, `tokens_per_minute`, `cost` (last 5 minutes, same shape as above),
`source`, `age_seconds` (seconds since the store was read).

## What is not in version 1

- Relation graph (`nodes[]`, `edges[]`): R6.
- Plan limit figures: not in version 1.
- A `processes[]` row for a harness with no running process: absent, not zero.
- Machine-wide CPU and memory: `processes[]` covers harnesses only.

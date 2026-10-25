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

Version 1 is frozen. Later rows (R6 added `nodes[]` and `edges[]`) add new top-level fields
only. They never rename, remove or retype a version 1 field. Readers ignore fields they do
not know. A change that breaks a reader bumps `schema_version`.

## Fields

Every list is always an array, never null. Times are RFC3339 UTC. Every item carries its
`source` (`store` for BurnMon's own store, `sysmon` for the process sampler, and for graph
items the graph's own sources) and its `age_seconds`.

| Field | Meaning |
|---|---|
| `schema_version` | 1 |
| `written_at` | when this tick was written |
| `tick_seconds` | the write interval |
| `sources[]` | `name`, `ok`, `read_at`, `error` (only when not ok). One entry per input: `store`, `sysmon`, and the graph's sources (see "More sources"). A failing source is reported here and the file is still written |
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

## Relation graph: nodes[] and edges[] (added in R6, still version 1)

Two arrays added under the compatibility rule above: new top-level fields only, so
`schema_version` stays 1. Both are always arrays. Code: `internal\graph\graph.go`.

The graph shows the running work on this device: which harness runs which session, which
subagents a session spawned, which processes it started, which MCP servers it uses and which
ports they listen on, which repo, worktree and branch it works in, which Foreman sprint or flow
it works for, and who waits on you. It is the live graph only. History is in BurnMon's store
(`burnmon-cli graph -since 24h`), so it outlives deleted session files.

### nodes[]

`id`, `kind`, `label`, `vendor` (sessions only), `attrs` (an object of strings, kind
specific), `source`, `age_seconds`.

| kind | id | notes |
|---|---|---|
| `device` | `device:local` | label `This PC`; no host name |
| `harness` | `harness:<name>` | `claude`, `claude-desktop`, `codex`, `copilot-cli`, `copilot-vscode`, `hermes`, `cowork-cloud` |
| `session` | `session:<vendor>:<id>` | `<id>` is `sessions[].id`; attrs `agent`, `state`, and `tool` while the session runs one (the tool's name only, never its arguments; an MCP tool's name carries its server's name, so a reader that shows it should not show it as is) |
| `subagent` | `session:<vendor>:<id>` | attrs `agent_type`, `model` when the harness records them |
| `process` | `process:<pid>:<start time>` | label is the image name; attrs `pid`, `harness`. The start time keeps a reused pid apart. Command lines are never written |
| `mcp` | `mcp:<name>` | the server name from the MCP config |
| `port` | `port:tcp:<n>` | a listening TCP port |
| `repo`, `worktree`, `branch` | `repo:<path>`, `worktree:<path>`, `branch:<repo path>#<name>` | paths lower case |
| `sprint`, `flow`, `gate` | `sprint:<id>`, `flow:<id>`, `gate:<flow id>:<gate>` | only when a Foreman folder is configured (`foreman_root` in `burnmon.json`) |
| `person` | `person:you` | the person the work waits on |

Labels and attrs carry no prompt text.

### edges[]

`from`, `to`, `kind`, `basis`, `source`, `age_seconds`, `first_seen`, `last_seen`. One edge per
(`from`, `to`, `kind`). Both ends are always in `nodes[]`.

`basis` says how BurnMon knows the edge: `vendor` when the source's own data states it,
`inferred` when BurnMon joined two facts that do not name each other. An edge BurnMon cannot
see is left out, never guessed.

| kind | from, to | basis |
|---|---|---|
| `runs_on` | harness, device | vendor |
| `runs_in` | session, harness; session, its own process; process, harness | vendor |
| `spawned` | session, subagent | vendor |
| `started` | process, child process | vendor |
| `uses` | session, mcp | inferred (a child process whose command line matches a configured server) |
| `runs_as` | mcp, process | inferred |
| `listens_on` | process, port | vendor |
| `works_for` | session, sprint | inferred (session folder equals the sprint worktree) |
| `works_for` | session, flow | vendor (the flow names the session) |
| `waits_on` | session, person | vendor from the harness's own idle status, else inferred from the stage |
| `waits_on` | sprint or gate, person | vendor |
| `touches` | session, worktree or repo or branch | inferred |
| `touches` | sprint, worktree | vendor |
| `part_of` | worktree, repo; gate, flow | vendor |

`first_seen` and `last_seen` are RFC3339 UTC. For edges only a live tick can see (everything
except `runs_on`, `spawned` and a session's `runs_in` to its harness), `first_seen` is the first
time BurnMon recorded it.

### What the graph cannot show

| | Claude Code | Codex | Copilot CLI | Copilot in VS Code | Hermes |
|---|---|---|---|---|---|
| `spawned` | yes | no parent id in its files | no | no | yes |
| session to its own process | yes | no | no | no | no |
| MCP servers | yes | no | no | no | no |
| messages between agents | no | no | no | no | no |

A session without its own process still hangs under its harness. Messages between agents are
not in the graph yet: no harness's local data names the receiver in a form BurnMon stores.

### More sources

`sources[]` lists one entry per input, not only `store` and `sysmon`. R6 adds `claude_pids`,
`mcp_config`, `net_tables`, `git`, and `foreman` when a Foreman folder is configured. A source
that fails is listed as not ok with its error, and its edges are left out of that tick. Readers
treat a source name as an opaque label.

## What is not in version 1

- Plan limit figures: not in version 1.
- A `processes[]` row for a harness with no running process: absent, not zero.
- Machine-wide CPU and memory: `processes[]` covers harnesses only.

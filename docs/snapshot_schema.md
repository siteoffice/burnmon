# BurnMon snapshot file, schema version 1 (frozen)

`burnmon.exe` or `burnmon-dev.exe`, whichever holds the writer lock, rewrites one JSON file every tick (5 seconds):

    %USERPROFILE%\.burnmon\snapshot.json

It writes a temp file in the same folder, then renames it over the old one, so a reader
sees the old file or the new one, never half of one. The file is read-only data for other
tools (the Fleet mod). No port, no model call, no tokens. Code: `internal\snapshot\snapshot.go`.
Example: `testdata\snapshot_example.json`.

## Is BurnMon running

The file is stale when `written_at` is older than three ticks (`3 * tick_seconds`). A stale
or missing file means BurnMon is not running. Say that plainly, do not show old numbers as
current.

## Who writes the file

One process writes at a time. The writer holds an OS file lock on
`%USERPROFILE%\.burnmon\snapshot.lock`, next to `snapshot.json`. The OS drops the lock the
moment the owner dies, so a dead owner never blocks. The lock file text is a diagnosis
note, never the proof of liveness: JSON with `pid`, `exe` (`burnmon.exe` or
`burnmon-dev.exe`) and `since` (RFC3339 UTC). A process that does not hold the lock tries
to take it at every tick, so when the owner exits the other one takes over at the next
tick. Code: `internal\snapshotwriter\lock.go`.

## Compatibility rule

Version 1 is frozen. Later rows (R6 added `nodes[]` and `edges[]`, R9 added `wait_items[]` and
`outcome_items[]`, R10 added `agent_sessions[]`, B8 added `agent_view`) add new top-level fields only. They never rename, remove or retype a version 1 field. Readers ignore fields they do
not know. A change that breaks a reader bumps `schema_version`.

## Fields

Every list is always an array, never null. Times are RFC3339 UTC. Every item carries its
`source` (`store` for BurnMon's own store, `sysmon` for the process sampler, for graph
items the graph's own sources, for wait and outcome items `foreman`, `agent_view` or the source
of the graph edge they come from) and its `age_seconds`. `agent_sessions[]` items carry neither.

| Field | Meaning |
|---|---|
| `schema_version` | 1 |
| `written_at` | when this tick was written |
| `tick_seconds` | the write interval |
| `sources[]` | `name`, `ok`, `read_at`, `error` (only when not ok). One entry per input: `store`, `sysmon`, and the graph's sources, and `agent_view` when it is on (see "More sources"). A failing source is reported here and the file is still written |
| `sessions[]` | running sessions and subagents, flat. See below |
| `burn` | token rate over the last 5 minutes, all sessions |
| `processes[]` | per harness: `harness`, `cpu_pct`, `mem_mb`, `io_bps`, `source`, `age_seconds` |
| `waits_on_you[]` | sessions whose turn ended and who wait for a person: `vendor`, `session_id`, `project`, `since`, `waiting_seconds`, `source`, `age_seconds` |
| `wait_items[]` | items that wait on the person (added in R9, still version 1). See "Waits and outcomes" |
| `outcome_items[]` | items finished or failed in the last 24 hours (added in R9, still version 1). See "Waits and outcomes" |
| `agent_sessions[]` | the last agent view poll (added in R10, still version 1). See "Agent sessions" |
| `agent_view` | the writer's agent view state (added in B8, still version 1). See "Agent view state" |

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
| `sprint`, `flow`, `gate` | `sprint:<id>`, `flow:<id>`, `gate:<flow id>:<gate>` | only when a Foreman folder is configured (`foreman_root` in `burnmon.json`). A sprint node has attrs `phase` and `status` and appears only when a session is linked to it or it waits on you. A flow node has attr `status` and appears only when a session is linked to it or its gate is open. A gate node appears only while the gate is open |
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

`agent_view` is listed only when `agent_view` is true in `burnmon.json`. It runs
`claude agents --json`, which Claude Code documents, at most every 30 seconds, with a 10 second
limit per run, in a background poll that a tick never waits for. `claude` is the file named by
`claude_path` in `burnmon.json` when that is set, else the first `claude` on PATH. It never
reads `~\.claude\jobs\`. Its states:

- ok, with `read_at` set to the end of the latest finished poll.
- not ok, "no poll has finished yet", until the first poll ends.
- not ok, with the error, when the poll fails or its output does not parse.
- not ok, with the reason, when `claude` is not on PATH, is a `.cmd` or `.bat` shim, has no
  `.exe` extension, or is not an executable file, or when `claude_path` is not absolute, is not
  a `.exe` or is not a regular file. BurnMon never runs a shim.

The `foreman` source's `read_at` is the time of the last real read of the Foreman files, which
can be older than `written_at` because those files are read at most every tick and a half.

## Waits and outcomes: wait_items[] and outcome_items[] (added in R9, still version 1)

Two arrays added under the compatibility rule above: new top-level fields only, so
`schema_version` stays 1. Both are always arrays, newest first (by `since`). `waits_on_you[]`
stays for older readers. The outcome window is 24 hours: an outcome older than that is not
listed.

### Item fields

`kind`, `id`, `title`, `project` (when known), `detail` (when there is one, cut to 120
characters with `...` added when it was cut), `since` (RFC3339 UTC), `age_seconds` (seconds from `since` to `written_at`), `spec_path` (when known),
`result_path` (when known), `source`, `parked[]` (sprint items only: `n`, `title`, `reason`
when given, one entry per parked task).

`id` is `gate:<flow id>:<gate>` for a gate, `sprint:<id>` for a sprint wait and for a
`finished` outcome, `check:<sprint id>:test` or `check:<sprint id>:verify` for a failed check,
and for a session the graph node id (`session:<vendor>:<id>`), or `agentview:<short id>` or
`agentview:pid:<pid>` when the agent view entry names no session. `title` is the Foreman slug,
the graph node label, or for an agent view entry the last folder name of its working folder.
`since` is the gate's `opened_at` (else the flow's `updated_at`); for a sprint wait the newest
`task_parked` entry of its action log (else its `updated_at`); for a failed check the check's
time (else the sprint's `updated_at`); for a finished sprint its `updated_at`; for a graph
session the tick time minus the edge's `age_seconds`; for an agent view entry the first poll that
saw it waiting. Items are sorted by `age_seconds`, then by `id`. `spec_path` and `result_path`
are the Foreman sprint's `spec_path` and its `report_path` (else a `report.md` in the sprint
folder); a gate takes the result of the newest sprint with the same spec.

### kind

| kind | list | meaning |
|---|---|---|
| `gate` | wait | an open Foreman gate |
| `sprint` | wait | a sprint waiting on you, with its parked tasks in `parked[]` |
| `failed_check` | wait or outcome | wait if the sprint stopped, outcome if it failed within the last 24 hours |
| `question` | wait | agent view `waitingFor` "input needed" |
| `prompt` | wait | agent view `waitingFor` "permission prompt" |
| `session` | wait | a graph `waits_on` person edge, a blocked agent view entry, or any other agent view `waitingFor` (`sandbox request`, `worker request`, `dialog open`, with that text in `detail`) |
| `finished` | outcome | a sprint that ended within the last 24 hours (by its `updated_at`): status `passing` or `done`, or phase `done`, or stopped as `capped`, `failed` or `escalated` |

A sprint with a `waiting-on-you` status shows `1 task parked` or `<n> tasks parked` in `detail`.
A gate shows its gate id in `detail`. A failed check shows `test exit <code>` or `verify fail`,
then `: ` and the check's summary when it has one. An agent view entry that waits has its
`waitingFor` text in `detail`, or `blocked` when its state is `blocked`, and an agent view item
replaces a graph item with the same `id`.

A stopped sprint (`capped`, `failed`, `escalated`) is a `finished` item with the status first in
`detail`, for example `capped, test -, verify fail` (`-` means no check recorded). A sprint
that already has a `failed_check` outcome is not listed a second time as `finished`: it keeps
its `failed_check` items only, one per failing check, so two when both test and verify failed
within the window. A sprint has at most one `finished` row. A sprint whose failing check is older than 24
hours has no `failed_check` outcome and shows as `finished`.

## Agent sessions: agent_sessions[] (added in R10, still version 1)

One array added under the compatibility rule above: a new top-level field only, so
`schema_version` stays 1. It is always an array, in the order `claude agents --json` gave. It
is filled only when `agent_view` is on, from the entries of the last finished poll, and is
empty when `agent_view` is off, when the last poll failed, and before the first poll has
finished.

Fields: `kind` (`background` or `interactive`), `id` (the agent view short id, background
sessions only), `state`, `status`, `waiting_for`, `session_id`, `pid`, `started_at` (Unix
milliseconds) and `project` (the last folder name of the working folder). `cwd` and `name` are
never written.

BurnMon Dev uses the array only to decide which control buttons show. It re-reads agent view
before any control runs, so a stale or edited snapshot cannot aim a control.

## Agent view state: agent_view (added in B8, still version 1)

One object added under the compatibility rule above: a new top-level field only, so
`schema_version` stays 1. It says how the process that wrote the file set up its agent view
at start, so a reader can tell why `agent_sessions[]` is empty:

- `{"state": "off"}`: `agent_view` is false in that process's `burnmon.json`.
- `{"state": "refused", "reason": "..."}`: `agent_view` is true but `claude` did not resolve;
  `reason` is the same text as the `agent_view` source's `error`.
- `{"state": "on"}`: the poller runs. Whether a poll worked is in the `agent_view` source.

It is absent when the writer has no graph sources (no home folder), and in files written
before B8. BurnMon Dev turns `off` and `refused` into one line in Waits on you.

## What is not in version 1

- Plan limit figures: not in version 1.
- A `processes[]` row for a harness with no running process: absent, not zero.
- Machine-wide CPU and memory: `processes[]` covers harnesses only.

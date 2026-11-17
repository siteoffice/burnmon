# BurnMon

`burnmon.exe` is the history app of BurnMon, the vendor-agnostic token and cost monitor for your own AI
coding sessions. It reads the session transcripts your tools already write on this machine.

Parent index: `README.md`. Companions: `cmd\burnmon-dev\README.md`, `cmd\controlcentre\README.md`,
`cmd\burnmon-cli\README.md`.

## 1. What it is

BurnMon shows tokens and cost per day, vendor, session, project and client, from local files. It needs
no API key, no admin rights, no network call and no telemetry. Everything stays on this device. BurnMon itself makes no network call. With Read Cowork (Cloud) sessions
on, it starts your own Claude Code CLI, which goes online as you.

On Windows there is also no server and no open port. On macOS and Linux, `burnmon` runs one web server
that only this computer can reach (127.0.0.1); see "macOS and Linux" in section 3.

BurnMon is the successor of [claudecost](https://github.com/siteoffice/claudecost), renamed and extended
from v0.2 onward. Repo: [siteoffice/burnmon](https://github.com/siteoffice/burnmon). Plan:
`02_roadmap\2026-09-22_burnmon_plan.md`; the spec building the v0.3 release: `02_roadmap\2026-09-23_v0.3_spec.md`.

## 2. Where it fits

| Part | Job |
|---|---|
| Foreman | the process: flows, gates, sprints |
| BurnMon (this app) | watching, long view: history per day, vendor, project, client |
| BurnMon Dev | watching, short view: the last 30 minutes, Waits on you, the agent map |
| Control Centre | steering: units, boards, gate decisions |
| Skills | the tools an agent uses |

BurnMon needs no other part, and nothing else needs it. Two Groundwork views, the Fleet and the Setup
Doctor, read the snapshot file `%USERPROFILE%\.burnmon\snapshot.json`, so they need `burnmon.exe` or
`burnmon-dev.exe` running. Without either, the file goes stale and they say BurnMon is not running.
Groundwork works without BurnMon.

### What needs BurnMon

Either exe writes the snapshot file. With both running, one writes: the one that holds the lock
`%USERPROFILE%\.burnmon\snapshot.lock`. The other only reads. If the writer exits, the other takes over at
its next tick (5 seconds). The lock is an operating system file lock, so a dead writer never blocks the other;
the file holds an owner record (process id, exe, the time the lock was taken) for diagnosis only. The snapshot
also carries `wait_items`, `outcome_items`, `agent_sessions` and `agent_view` (the writer's agent view state:
`off`, `refused` with the reason, or `on`); fields and kinds are in `docs\snapshot_schema.md`. Gates, sprints
and failed checks in `wait_items` and `outcome_items` come from Foreman and need `foreman_root`.

### What it reads

- **Claude Code** and **Cowork** (Claude's desktop agent mode): `claude-code-sessions` transcripts, on Windows,
  macOS, Linux and inside WSL.
- **Codex** CLI, Desktop and the VS Code extension: rollout transcripts, native and WSL roots. Codex keeps a
  rollout open for the whole chat, and Windows sends no change notice for appends to a file held open. So every
  recently active transcript is also re-checked every 2 seconds by its size.
- **Hermes**: its local SQLite `messages` database, polled every 5 seconds.
- **GitHub Copilot CLI**: its own SQLite session store, polled every 5 seconds. A session's numbers update live,
  the same as every other adapter.
- **GitHub Copilot Chat in VS Code**: the JSON file its own OpenTelemetry export writes, polled every 5 seconds.
  Each poll reads only the lines added since the last one (the whole file once after a start). It is off by
  default: VS Code sends no telemetry anywhere until you turn these two settings on yourself, in `settings.json`:

```json
"github.copilot.chat.otel.enabled": true,
"github.copilot.chat.otel.outfile": "C:\\path\\to\\copilot-otel.jsonl"
```

Then point `copilot_vscode_otel_file` in `burnmon.json` at that same path, and reload the VS Code window once.
A setting change alone does not start the exporter; the extension host needs a fresh start. No workspace or folder
attribute has been seen on any span the extension emits so far, so every event's client shows as "unassigned" until
GitHub adds one. Token counts and model are read regardless.

Coverage is these six, plus Cowork sessions that run in the cloud, through the optional plugin in section 4.
`claude.ai` in the browser keeps no local transcript BurnMon can read, so it does not appear here.

### How it compares

| Tool | What its source says | Where BurnMon differs |
|---|---|---|
| ccusage | "Reads only local logs on one machine", Claude Code spend, no team rollup ([Torii](https://www.toriihq.com/articles/five-claude-code-usage-dashboards-and-monitoring-tools)) | reads six tools, and keeps history in SQLite with client and owner rules |
| Claude Code Usage Monitor | terminal only, no shareable team dashboard, shows runway in the 5-hour window ([Torii](https://www.toriihq.com/articles/five-claude-code-usage-dashboards-and-monitoring-tools)) | a window, not a terminal; cost per day and client; not limited to Claude Code |
| Agent view in Claude Code (`claude agents`) | one row per session: needs your input or not, last response, last interaction ([Anthropic](https://claude.com/resources/articles/agent-view-in-claude-code)) | BurnMon Dev reads it too and adds other tools, tokens, cost and Foreman gates |
| gitpane | dirty, ahead and behind, worktree count and stash per repo ([docs.rs](https://docs.rs/crate/gitpane/0.13.0)); its page names no link to an agent session | the Control Centre Git page matches a worktree to the session that works in it, by folder name |

These rows come from the pages named, read on 2026-11-15. The tools change; check them before you decide.

## 3. Install

Windows: build from source. The release workflow does not attach Windows files.

```powershell
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon
Set-Location C:\ZND\50_projects\burnmon
.\build.ps1
```

Requires Go (`winget install GoLang.Go`). The first run needs internet access: it fetches `go-winres` for the
icons and `go mod tidy` resolves the WebView2 binding. It produces `burnmon-cli.exe`, `burnmon.exe`,
`burnmon-dev.exe` and `controlcentre.exe`, all portable single files. Copy them anywhere; no install. If a
`build.local.ps1` exists next to `build.ps1` (gitignored, machine-local), it runs afterward.

### macOS and Linux

`burnmon` and `burnmon-cli` also build for darwin and linux (amd64 and arm64, `CGO_ENABLED=0`, pure Go, no cgo),
attached to every tagged release as `burnmon-<os>-<arch>` and `burnmon-cli-<os>-<arch>`. From source:

```bash
# runs in: bash, cwd = your clone of siteoffice/burnmon
go build -o burnmon ./cmd/burnmon
go build -o burnmon-cli ./cmd/burnmon-cli
```

Neither platform has WebView2, so there is no app window there. `burnmon` serves the dashboard from a
localhost-only web server (`http://127.0.0.1:47615/?t=<key>`, change the port with `-port`), prints that link and
opens it in the system default browser (`-no-open` to skip). That server provides the same live functions the
Windows window binds (Now, History, the vendor strip, the forecast, session findings, the turn drawer, Refresh now
and Settings). It refuses requests from any other host or origin. The page reloads itself whenever the `-interval`
re-collection rewrites it. Run on macOS (Apple Silicon) since 2026-10-20; Linux is still untested on real hardware.

**The one open port.** On macOS and Linux, `burnmon` listens on `127.0.0.1:47615`. Other computers cannot reach it.
Other users and programs on the same computer can reach the port, so every request needs a secret that `burnmon`
makes at each start and keeps in a file only you can read (`bridge-token` in the data folder, mode 0600). A link
from an earlier start stops working; open the link `burnmon` printed, or start it again. Starting `burnmon` a
second time opens the running one. If another program already uses port 47615, `burnmon` says so; start it with
`-port <number>`. The Windows app has no server and no open port.

### Help test macOS and Linux

This repo is public, and the macOS and Linux builds need real hardware. If you have a Mac or a Linux box with
Claude Code, Codex, Copilot CLI or Hermes on it, please try it and open an issue with what you see. Things worth
checking:

- Does the browser open `dashboard.html`, and do your sessions show up on Now and Sessions?
- Are the default transcript roots in the table in section 5 right for your install?
- Does the store open: `~/.config/burnmon/burnmon.db` on Linux, `~/Library/Application Support/burnmon/burnmon.db`
  on macOS (Go's `os.UserConfigDir`)?
- Do file changes get picked up while the page is open? Neither platform has the native Windows watcher; they use
  the older per-directory watch, which was never run on a large tree.
- Does `burnmon-cli price-check` and `burnmon-cli export` work?

Please leave out prompts, project names and client names from anything you paste.

## 4. Use

Double-click `burnmon.exe`. One window opens, no console, no tray icon, no browser tab. It collects on startup,
again every 15 minutes (`-interval`, floor 5 minutes) while the window stays open, and whenever you press "Refresh
now". Close the window and the app is gone; nothing keeps running in the background. It opens no ports on Windows
and needs no admin rights (`asInvoker`). It needs the WebView2 runtime (it ships with Edge on Windows 10 and 11);
without it, the app writes the report once and opens it in your default browser instead.

Parsed transcripts are stored in `%LOCALAPPDATA%\burnmon\burnmon.db`, a SQLite database migrated forward
automatically on every startup (versioned, additive migrations only: nothing already stored is ever dropped).
Click the gear icon to edit subscription numbers, seat counts and seat prices from inside the window. Saving
re-reads everything, since cached sessions carry costs computed with the old numbers.

The first ten minutes: start it, wait for the first collect, open Now to see running sessions, then History for
the week. If the Sessions tab misses a tool, check its transcript root in section 5.

The page the app and every saved CLI report share has five tabs, deep-linkable (`#now`, `#history`, `#sessions`,
`#tools`, `#about`):

- **Now** (the start page). Every running session, live: polled every 2 seconds, with new sessions picked up
  within about a second of their first write. A 30-minute smoothed burn chart (per-session lines, the cost line
  off by default), the vendor strip (one row per vendor in the store: today, this week, this month, tokens,
  refreshed once a minute), the turn ticker, and the forecast chart.
- **History**. One page, filtered: period (day, week, month), range (last 7, 30, 90 days, custom), vendor, and
  owner once owner rules are configured. Totals, a "Burn per period" chart stacked one segment per harness (Claude
  Code, Codex, Copilot CLI and so on, fixed colours shared with the Now page), and a table for whatever is selected.
  The filter state lives in the URL so a view can be reopened. A per-client table appears once client rules exist.
- **Sessions**. Every session, sortable and searchable, with a Harness column and filter (the real harness a session
  ran under, for example Claude Code, Codex or Copilot CLI, not the Surface it shares with another vendor), a
  Findings column (count by kind) and an expandable row listing each finding. A click opens the same turn detail
  drawer the Now page uses. The owner column and filter appear once owner rules are configured. A session priced by
  a vendor with no book entry for its exact model shows "no price", not a guessed figure.
- **Tools**. Which connectors, plugins and skills your sessions actually call, as a chart and a table, over the
  whole window.
- **About**. How Claude's own seat and allowance model works, in plain language, plus where the numbers come from
  and what they cannot show.

### The Now page

Every running session shows a card with a context gauge. Under the chart, each session's turn ticker line can carry
a marker. A click on a marker or its ticker line opens a drawer with the model, token classes, tool calls that turn,
files read, the gap since the previous turn, and, when a finding applies, its cause and confidence.

Findings come from `internal\insight`, computed on the fly from the same windowed events the Now snapshot already
loads. Nothing about them is stored.

- **re-prefill**: a turn's cache write exceeds the configured threshold. The cause is inferred in order: compaction
  just happened, the model changed since the previous turn, the gap since the previous turn exceeded the cache TTL,
  first turn after a resume, or "unknown".
- **compaction**: context size drops by more than 30% between consecutive turns while the session continues.
- **context-runway**: a linear fit over the last 10 turns of context size, reported as turns remaining to 80% and
  90% of the model's context window.
- **expensive-turn**: a turn above the 95th percentile of the session's own turn cost or tokens, with its dominant
  token class named.

The forecast chart shows the plan line (the last four weeks, weekday-aware) and the live line (the current rate
carried to end of day and end of month), with an error band once at least one week has been scored. Until then it
shows history only, with the gate text "forecast unlocks after the first scored week". Tokens only.

### Cost

The Now page shows tokens, context fill and cache hit rate: what a developer watches while working. History,
Sessions and export also show cost, on the vendor's headline basis, named next to the figure: plan credits for
GitHub Copilot, your configured subscription share when `burnmon.json` carries one, else the API list price,
labelled "upper bound".

Cost comes from dated price books built into the binary (`internal\pricing\books`, each with its own source URL and
check date), overridable per model from `burnmon.json`. `burnmon-cli price-check` prints every book, its date and
its source, so you always know how current a number is. A vendor with no book at all (Hermes today), or no entry for
the exact model a session used, shows "no price" or "tokens only", not a guessed figure.

### Owner and client rules, active time

`burnmon.json` can carry an ordered `owners` table of rules, for example
`{"match": "C:\\dev\\work\\*", "owner": "Acme", "client": "ClientA", "remote": "github.com/org/repo"}`, applied to a
session's project path at ingest. Owner: the first rule whose `match` path prefixes the project wins; an unmatched
path gets `"personal"`. Client (optional, v0.3): a rule whose `remote` matches the project's git origin (read straight
from `.git\config`, no git binary) wins first, then a rule whose `match` path prefixes the project, else
`"unassigned"`. Empty `owners` (the default) means no owner or client column anywhere. A v0.2 `burnmon.json` with
`owners` only, no `client` or `remote`, keeps working unchanged. After changing the rules, re-apply them to sessions
already in the store with `burnmon-cli reown`. Nothing else re-runs ingest for you.

Each session also gets an **active time**: the sum of gaps between consecutive turns, with any gap over
`active_idle_minutes` (default 10 minutes) counting zero. It is labelled everywhere "active time, from transcript
timestamps, not billable": a signal, not a time-tracking replacement. Once client rules exist, History's per-client
table shows tokens, headline cost, active time and session count side by side for every client, comparable even with
one selected in the filter above it.

### Export and merge

Both apps write the same export file as `burnmon-cli export`, from Settings. You type the folder as an absolute path
(it starts as your Downloads folder) and can set since, until and a label. When `owners` rules exist, tick at least one
owner. The command line forms, and `burnmon-cli merge`, are in `cmd\burnmon-cli\README.md`. No paths, session ids,
prompts or project names ever leave in the file.

### Cowork (Cloud) sessions

Cowork sessions that run in Anthropic's cloud write no transcript to your disk. BurnMon can copy them with your own
Claude Code CLI, for free, when you turn on Read Cowork (Cloud) sessions: in Settings in `burnmon.exe`, or in the gear
in `burnmon-dev.exe`. It is off by default, and without it the Cowork (Cloud) row says not measured.

- What it does: it starts your Claude Code CLI to copy your cloud sessions to this computer, one at a time, at most one
  every 30 seconds. A session with a turn in the last 30 minutes is copied again every 2 minutes and goes before any
  session not copied yet; one with a turn in the last day every 30 minutes, an older one once a day, and one more than
  31 days old no more. So a quiet session that gets a new turn shows it within 30 minutes, or within a day when it had
  been quiet for more than a day. Each copy runs
  `claude --teleport <id> -p "/rename burnmon-pull" --model haiku --output-format json` and must report 0 turns and 0
  cost; anything else switches the copying off and says why on screen.
- What it needs: Claude Code installed and signed in to the same account as the Claude app.
- What it reads: cloud sessions you start in the Claude app on this computer, inside or outside a Project. Sessions
  started from your phone, the browser or another computer are not read, and neither is subagent usage inside a cloud
  session. With it on, a working pull shows plain numbers; a failing one shows not measured with the reason.

### Cowork in the cloud

New Cowork sessions can run in Anthropic's cloud instead of on your computer. Since 6 October 2026 that is the only
option for new tasks on Pro and Max, and on Team it is on by default. A cloud session writes no transcript to your
disk, so BurnMon cannot see it, and your Cowork number comes out too low without any warning. On one Team account on
2026-10-12, two new sessions in a row ran in the cloud even with "Only on this computer" switched on.

**Status: still under investigation.** The plugin below works, but only for a cloud session that has the folder
`~\.burnmon` connected. Claude does not connect that folder by itself, so today every cloud session needs that step by
hand (or once per Cowork project), and a session without it is simply not counted, with no warning. That is not
workable for most people. As of 2026-10-13 we found no local, offline token monitor that counts cloud sessions without
such a step: the tools that do count them send the numbers through an online service, or need an organization admin to
set up Cowork's OpenTelemetry export. If you know a better way, please share it: open an issue at
https://github.com/siteoffice/burnmon/issues.

A partial fix is an optional Claude plugin, `burnmon-cloud`, in `tools\burnmon-cloud`. Inside each cloud session it
reads that session's own transcript at the end of each turn, when a subagent finishes, and at most every 20 seconds
while Claude works with tools. It keeps only the usage numbers: time, session and request ids, model and token counts.
No prompts, no answers, no tool input, no paths. The Claude desktop app then copies that file into `~\.burnmon\cloud`
in your home folder (on Windows `C:\Users\<you>\.burnmon\cloud`), and BurnMon shows it as its own row, Cowork (Cloud).
The copy is a plugin hook, so it costs no model tokens. BurnMon itself still makes no network call. The copy goes
through the Claude app, into a folder you connected to the session.

Set it up once:

1. Start BurnMon once. It creates `~\.burnmon\cloud`.
2. Install the plugin "BurnMon cloud" from the ZND Skills Public marketplace (GitHub `zero-nonsense-dev/znd-skills-public`).
   Updates arrive through the marketplace. Without the marketplace, build it yourself (`tools\burnmon-cloud\build.ps1`,
   no arguments) and add `dist\burnmon-cloud-<version>.plugin` under Customize > Plugins > Add.
3. Connect the folder `~\.burnmon` to the cloud sessions you want counted. In a Cowork project, add it once to the
   project's folders.

The plugin also copies `diag.log` into the same folder: one line per hook run, with the time, the hook name and a row
count, and no content. BurnMon ignores it. It is there to find out where a delivery delay comes from. BurnMon still
reads `.burnmon\cloud` next to `burnmon.exe` too, for a plugin built for one computer (`build.ps1 -Target <that
folder>\latest.jsonl`).

A cloud subagent's requests are stored as their own session, `agent-<id>`, with the main session as its parent, and
the copy folded under the parent is removed so its tokens count once. The rows are built in
`internal\adapter\claude\cloud.go`, the folded copy is removed in `UpsertEvents` in `internal\store\store.go`, and the
tests are in `cloud_subagent_test.go`.

What it does not cover:

- A session without `~\.burnmon` connected is not counted. Neither is a session started on the web or on your phone:
  those cannot reach your computer.
- The numbers are a lower bound. The transcript misses some side calls. Measured on one session at seven turn
  boundaries: 0 to 7 percent of the main model's tokens (mostly cache reads), and every small Haiku call, which cost
  under 1 percent of that session.
- Cloud data arrives late, and BurnMon Dev says how late. That includes the work of a subagent (an agent Claude starts
  for research or parallel tasks), which arrives when the subagent finishes. The plugin writes within about 20 seconds;
  the copy to your laptop took about 3 minutes in one measurement on 2026-11-02. BurnMon Dev shows `cloud data N min old`
  next to every Cowork (Cloud) session, or `cloud data under a minute old`. Older versions deliver it with your next
  message, or about 4 minutes after the turn when the session goes idle. BurnMon shows each request at the minute it was
  actually spent, so the chart fills in afterwards.
- The usage file has no tool calls, so BurnMon Dev's Station cannot tell a cloud session that works from one that waits
  for you. It shows the session as Thinking until 3 minutes after its newest request, then as Waiting on you.
- Two cloud sessions running at the same time copy to the same file and overwrite each other there. BurnMon still keeps
  the rows of both: it adds rows and never removes them.
- The plugin reads Claude's transcript format, which Anthropic documents as internal and liable to change between
  versions. If BurnMon stops showing Cowork (Cloud) after a Claude update, that is the first place to look.

### WSL

Claude Code running inside a WSL distribution (Ubuntu under WSL, most commonly) is a Linux process with a Linux `$HOME`,
so its transcripts live inside the distro, not under any Windows folder. BurnMon finds them automatically: it reads
which WSL distributions are installed from the registry, then looks for each one's `.claude/projects` folder, including
one moved via a `CLAUDE_CONFIG_DIR` set in a shell startup file.

- Reading Linux files from Windows goes over WSL's own file-sharing layer and is slower than reading a native NTFS
  folder. WSL transcripts are re-read on their own, slower clock (`-wsl-interval`, default 4 hours) while native Windows
  transcripts keep the normal refresh. Refresh now and saving Settings always re-read everything, WSL included.
- Accessing a stopped WSL 2 distribution's files can start it in the background, which costs a few seconds and some
  memory. This only happens on the slower WSL cadence above.
- Set `"wsl_scan": "off"` in `burnmon.json` to turn WSL scanning off entirely; it is the first thing to try if a scan
  seems slow and you want to rule WSL out.
- If detection misses a distro, add the folder directly with `"extra_sources": ["..."]` in `burnmon.json`, or with a
  repeatable `-source` flag.

## 5. Settings

Prices and the subscription calibration are compiled-in illustrative defaults (see `internal\pricing\pricing.go`), not
a real invoice. Drop a `burnmon.json` in one of two places, both valid: next to the exe, or at
`%LOCALAPPDATA%\burnmon\burnmon.json`. The app and CLI both read it, and the file next to the exe wins when both exist.
Put your own numbers in it; any subset of fields overrides the defaults. `burnmon.example.json` lists these keys,
including `owners` (empty by default). Three keys are the ones a tester is likely to touch: `agent_view` (anyone who
wants background agents and the agent map), `foreman_root` (Foreman users only) and `claude_path` (only when BurnMon
cannot start `claude`). Their meaning is in `cmd\burnmon-dev\README.md`, section 5.

`cloud_pull` (default `false`) is the switch Read Cowork (Cloud) sessions, saved by either app's settings and read by
both every 30 seconds (see Cowork (Cloud) sessions above).

If no `burnmon.json` is found, both binaries still read a `claudecost.json` in the same two locations: the pre-rename
config name, kept working for anyone who has not renamed their file yet. `burnmon.json` always wins when both are
present. Rename to `burnmon.json` when convenient; there is no reason to keep using the old name once you have.

`keep_history` (default `true`) goes in `burnmon.json` and is read by both apps. It does not go in `burnmon-dev.json`.
While `keep_history` is on, deleting a transcript does not delete BurnMon's copy of it: its events stay in the store and
only the read position for that file is forgotten. Set it to `false` and a transcript that vanishes takes its events
with it. Both apps can also delete history from their own settings (`burnmon.exe` in Settings, `burnmon-dev.exe` in the
gear dialog): older than N days, one session, or all. Each delete needs two presses, the second on a button that now
reads "Confirm: ...". A delete keeps the read position, so deleted turns are not read back in from a transcript that is
still on disk.

Other keys: `active_idle_minutes`, `wsl_scan`, `extra_sources`, `copilot_vscode_otel_file`, `owners` (all above).

Default transcript roots, by OS:

| Tool | Windows | macOS | Linux |
|---|---|---|---|
| Claude Code / Cowork | `%USERPROFILE%\.claude\projects` | `~/.claude/projects` | `~/.claude/projects` |
| Codex | `%USERPROFILE%\.codex\sessions` (or `$CODEX_HOME`) | `~/.codex/sessions` | `~/.codex/sessions` |
| Hermes | `%HERMES_HOME%\state.db`, else `%LOCALAPPDATA%\Hermes\state.db` | `$HERMES_HOME/state.db`, else `~/.hermes/state.db` | `$HERMES_HOME/state.db`, else `~/.hermes/state.db` |
| GitHub Copilot CLI | `%COPILOT_HOME%\session-store.db`, else `~/.copilot/session-store.db` | `~/.copilot/session-store.db` | `~/.copilot/session-store.db` |
| Cowork in the cloud | `~\.burnmon\cloud` and `.burnmon\cloud` next to `burnmon.exe` (both created at start, filled by the `burnmon-cloud` plugin) | same | same |
| GitHub Copilot in VS Code | wherever `github.copilot.chat.otel.outfile` points, matched by `copilot_vscode_otel_file` | same (no default: the setting has none on any OS, you always choose the path) | same |

WSL distro detection (the registry-based scan for Claude Code and Codex transcripts inside a WSL distribution) only
exists on Windows; it is a no-op everywhere else.

### What BurnMon never does

No server on Windows, no network call to the internet of any kind, no telemetry, no account. BurnMon itself makes no network call. With Read Cowork (Cloud) sessions
on, it starts your own Claude Code CLI, which goes online as you. On macOS and Linux there is
one local web server on 127.0.0.1 for the dashboard, protected by a secret that changes every start. The optional cloud
plugin runs inside Claude's own cloud session, not in BurnMon, and the Claude app copies its file to your disk. BurnMon
reads only your own transcript folders, never aggregates or compares across people, and refuses no one because there is
nothing to refuse: other people's usage is simply not on your disk. Costs shown for Claude are allocated shares of a flat
monthly invoice, not money owed by anyone.

Three optional features start programs on this computer, and only after you switch them on: the agent view poller
(`agent_view`), the controls, and the cloud pull (`cloud_pull`), which runs `claude auth status` and the copy command in
Cowork (Cloud) sessions above. The poller runs only `claude agents --json` (`internal\snapshotwriter\agentview.go`).
`stop` and `attach` run `claude stop <id>` and `claude attach <id>`; `approve` and `send_back` run `python` on a script
that calls Foreman's own gate command. All of them use documented commands, no undocumented endpoint.

## 6. Limits and known issues

This is a beta, built and used daily by one person. It runs on your own machine and changes nothing outside the folders
you point it at. It comes with no warranty (see `LICENSE`).

- **Cloud Cowork sessions arrive late.** The plugin writes within about 20 seconds; the copy to your laptop took about 3
  minutes in one measurement on 2026-11-02. A long tool run writes no usage at all until the next model request, so a busy
  cloud session can show as idle. The session only counts when `~/.burnmon` is connected to it.
- **Prices are API-price equivalents**, not your bill. On a subscription seat you pay the seat.
- **macOS and Linux have no app window.** The dashboard opens in your browser, served by a local web server. Linux is
  untested on real hardware.
- **History is slow on a large store.** Every filter change re-reads the whole store, about 3 seconds at roughly 55,000
  events.
- **Time bounds compare text.** A stored timestamp in the first second of a range can be wrongly excluded or included,
  because trailing zeros drop out of RFC3339 strings. Cost is at most one event at a boundary.
- **Old forecast rows.** One ISO week scored before the v0.3.2 local-time change keeps a UTC week start.
- **About tab only describes Claude's seat model**, not the other vendors.
- **Copilot Business and Enterprise** have no per-seat price wired in, and their pooled billing does not fit the
  single-machine model, so only plan credits for individual plans are shown.
- **No price, no guess.** Hermes, and any model without a book entry, show tokens only.
- **Both exes share one store.** Running `burnmon.exe` and `burnmon-dev.exe` together makes both ingest the same files.
  Measured safe, but it is double work.
- **Copilot in VS Code** reports every client as "unassigned" until GitHub adds a workspace attribute to its telemetry.
- **Station CPU climbs while it is open** (BurnMon Dev): see `cmd\burnmon-dev\README.md`.

The full list, with measurements, is in `STATUS.md`.
## 7. Next steps and backlog

Priority order lives in `02_roadmap\roadmap.md`; this is the short public version, as of 2026-11-15.

- **Now**: `v0.4.0-beta.4` is tagged (2026-11-15). The rented Mac test day of 2026-11-16 runs the Mac checklist; findings
  go into `v0.4.0-beta.5`.
- **Next**: make Waits on you and the agent view correct and fast against real sessions; test and fix macOS and Linux
  with help from real users; a plan-limits panel (how much of a seat or allowance is left), briefed but not started.
- **Later, undecided**: a local OTLP receiver, signed Windows builds, a native window off Windows. A decision on whether
  and how to continue past 2026-12-19 is open.
- **Not planned**: a server, an account, telemetry, or any undocumented vendor endpoint.

Shipped so far: `v0.3.0` (price books, client map, export and merge, Copilot in VS Code, macOS and Linux browser-mode
builds), 2026-09-24; `v0.3.1`, a cleanup pass, 2026-09-24; `v0.3.2`, shared ingest performance and local time everywhere,
2026-09-26. `v0.4.0-beta.1`, 2026-10-30: all exes carry one version again; it adds the agent map, Waits on you,
Outcomes, the first controls and the section layout to BurnMon Dev. `v0.4.0-beta.2`, 2026-10-31: the agent view behind an
npm shim, a new Foreman gate on the page in about a second, folder and short id titles, a collapsed Done group and a
Windows toast for Blocked and Your turn. `v0.4.0-beta.3`, 2026-11-04: the release for the first test group: Waits on you
drops the failed checks of closed sprints, "Which app do I open?", the age of cloud data, the Burn Report (redacted by
default), gate controls that follow Foreman's gate names, and a macOS notifier that waits for the native Mac window.
`v0.4.0-beta.4`, 2026-11-15: the Control Centre (`controlcentre.exe`), with its root set in Settings and a Git page, and
BurnMon Dev's section sizes as typed percents, with Stop as its only control. In beta.1 the agent view stayed empty when
`claude` on `PATH` was a `.cmd` shim; since B8 BurnMon reads an npm `claude.cmd` shim for the `claude.exe` it names. See
`STATUS.md` for what is true at the current commit and `SESSION_LOG.md` for the session-by-session record.

## 8. Feedback, licence, contact

We want to hear what broke and what you expected instead.

1. **Bug**: open an issue with the "Bug" template. Include the version (the About tab), your OS, and what you did just
   before. Never paste prompts, keys or customer data.
2. **"It did not do what I expected"**: use that template. This is the most useful one for us.
3. **Idea**: use the "Idea" template, one idea per issue.
4. **After two weeks**: attach a Burn Report if you are willing to share it (BurnMon Dev, gear dialog, Export history,
   Burn Report button; project names are redacted unless you tick "show project names"; see `cmd\burnmon-dev\README.md`).

We read every issue within two working days. Security problems: do not open a public issue; mail info@zerononsense.dev.
Licence: see `LICENSE`. Repo: https://github.com/siteoffice/burnmon.

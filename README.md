# BurnMon

BurnMon is a vendor-agnostic token and cost monitor for your own AI coding sessions: a
portable Windows app (`burnmon.exe`) and a single-file CLI (`burnmon-cli.exe`), reading
the session transcripts your tools already write on this machine. No API key, no admin
rights, no network calls, no telemetry: everything happens by reading local files and
everything stays on this device. On Windows there is also no server and no open port. On
macOS and Linux, `burnmon` runs one web server that only this computer can reach
(127.0.0.1); see "macOS and Linux".

BurnMon is the successor of [claudecost](https://github.com/siteoffice/claudecost),
renamed and extended from v0.2 onward. Repo:
[siteoffice/burnmon](https://github.com/siteoffice/burnmon). Plan:
`02_roadmap\2026-09-22_burnmon_plan.md`; the spec building the current release:
`02_roadmap\2026-09-23_v0.3_spec.md`.

## Screenshots

The Now page, live burn across vendors:

![BurnMon Now page: live burn chart, session cards and vendor strip](assets/screenshots/now-page.png)

BurnMon Dev, the developer build with the station view and system panels:

<img src="assets/screenshots/dev-station.png" alt="BurnMon Dev station view with system panels" width="560">

Both screenshots were last committed on 2026-10-07 (`git log`), before the B-series
changes described below (sections, Waits on you, Outcomes, controls). New screenshots are
not made yet.

## What it reads

- **Claude Code** and **Cowork** (Claude's desktop agent mode): `claude-code-sessions`
  transcripts, on Windows, macOS, Linux and inside WSL.
- **Codex** CLI, Desktop and the VS Code extension: rollout transcripts, native and WSL
  roots. Codex keeps a rollout open for the whole chat, and Windows sends no change notice
  for appends to a file held open, so every recently active transcript is also re-checked
  every 2 seconds by its size.
- **Hermes**: its local SQLite `messages` database, polled every 5 seconds.
- **GitHub Copilot CLI**: its own SQLite session store, polled every 5 seconds; a session's
  numbers update live, the same as every other adapter.
- **GitHub Copilot Chat in VS Code**: the JSON file its own OpenTelemetry export writes,
  polled every 5 seconds; each poll reads only the lines added since the last one (the
  whole file once after a start). Off by default: VS Code sends no telemetry anywhere until you
  turn these two settings on yourself, in `settings.json`:

      "github.copilot.chat.otel.enabled": true,
      "github.copilot.chat.otel.outfile": "C:\\path\\to\\copilot-otel.jsonl"

  Then point `burnmon.json`'s `copilot_vscode_otel_file` at that same path, and reload the
  VS Code window once (a setting change alone does not start the exporter; the extension
  host needs a fresh start). No workspace/folder attribute has been seen on any span
  emitted by the extension so far, so every event's client shows as "unassigned" until
  GitHub adds one; token counts and model are read regardless.

Coverage is these six, plus Cowork sessions that run in the cloud, through the optional
plugin in the next section. `claude.ai` in the browser keeps no local transcript BurnMon can
read, so it does not appear here.

## Cowork in the cloud

New Cowork sessions can run in Anthropic's cloud instead of on your computer. Since
6 October 2026 that is the only option for new tasks on Pro and Max, and on Team it is on by
default. A cloud session writes no transcript to your disk, so BurnMon cannot see it, and
your Cowork number comes out too low without any warning. On one Team account on 2026-10-12,
two new sessions in a row ran in the cloud even with "Only on this computer" switched on.

**Status: still under investigation.** The plugin below works, but only for a cloud session
that has the folder `~\.burnmon` connected. Claude does not connect that folder by itself, so
today every cloud session needs that step by hand (or once per Cowork project), and a session
without it is simply not counted, with no warning. That is not workable for most people. As of
2026-10-13 we found no local, offline token monitor that counts cloud sessions without such a
step: the tools that do count them send the numbers through an online service, or need an
organization admin to set up Cowork's OpenTelemetry export. If you know a better way, please
share it: open an issue at https://github.com/siteoffice/burnmon/issues.

A partial fix is an optional Claude plugin, `burnmon-cloud`, in `tools\burnmon-cloud`. Inside each
cloud session it reads that session's own transcript at the end of each turn, when a
subagent finishes, and at most every 20 seconds while Claude works with tools. It keeps only
the usage numbers: time, session and request ids, model and token counts. No prompts, no
answers, no tool input, no paths. The Claude desktop app then copies that file into
`~\.burnmon\cloud` in your home folder (on Windows `C:\Users\<you>\.burnmon\cloud`), and
BurnMon shows it as its own row, Cowork (Cloud). The copy is a plugin hook, so it costs no
model tokens. BurnMon itself still makes no network call. The copy goes through the Claude
app, into a folder you connected to the session.

Set it up once:

1. Start BurnMon once. It creates `~\.burnmon\cloud`.
2. Install the plugin "BurnMon cloud" from the ZND Skills Public marketplace (GitHub
   `zero-nonsense-dev/znd-skills-public`). Updates arrive through the marketplace. Without the
   marketplace, build it yourself (`tools\burnmon-cloud\build.ps1`, no arguments) and add
   `dist\burnmon-cloud-<version>.plugin` under Customize > Plugins > Add.
3. Connect the folder `~\.burnmon` to the cloud sessions you want counted. In a Cowork
   project, add it once to the project's folders.

The plugin also copies `diag.log` into the same folder: one line per hook run, with the time,
the hook name and a row count, and no content. BurnMon ignores it. It is there to find out
where a delivery delay comes from.

BurnMon still reads `.burnmon\cloud` next to `burnmon.exe` too, for a plugin built for one
computer (`build.ps1 -Target <that folder>\latest.jsonl`).

A cloud subagent's requests are stored as their own session, `agent-<id>`, with the main
session as its parent, and the copy folded under the parent is removed so its tokens count
once (the rows are built in `internal\adapter\claude\cloud.go`, the folded copy is removed in
`UpsertEvents` in `internal\store\store.go`, tests in `cloud_subagent_test.go`).

What it does not cover:

- A session without `~\.burnmon` connected is not counted. Neither is a session started on
  the web or on your phone: those cannot reach your computer.
- The numbers are a lower bound. The transcript misses some side calls. Measured on one
  session at seven turn boundaries: 0 to 7 percent of the main model's tokens (mostly cache
  reads), and every small Haiku call, which cost under 1 percent of that session.
- Work done while a turn runs reaches BurnMon about 10 seconds after it is written. That
  includes the work of a subagent (an agent Claude starts for research or parallel tasks),
  which arrives when the subagent finishes. Plugin 1.1.3 delivers the end of a turn about 10
  to 15 seconds after it ends (measured on one session, 2026-10-12). Older versions deliver it
  with your next
  message, or about 4 minutes after the turn when the session goes idle. BurnMon shows each
  request at the minute it was actually spent, so the chart fills in afterwards.
- The usage file has no tool calls, so BurnMon Dev's Station cannot tell a cloud session that
  works from one that waits for you. It shows the session as Thinking until 3 minutes after
  its newest request, then as Waiting on you.
- Two cloud sessions running at the same time copy to the same file and overwrite each
  other there. BurnMon still keeps the rows of both: it adds rows and never removes them.
- The plugin reads Claude's transcript format, which Anthropic documents as internal and
  liable to change between versions. If BurnMon stops showing Cowork (Cloud) after a Claude
  update, that is the first place to look.

## The five tabs

The app and every saved CLI report share one page, five tabs, deep-linkable
(`#now`, `#history`, `#sessions`, `#tools`, `#about`):

- **Now** (the start page). Every running session, live: polled every 2 seconds, new
  sessions picked up within about a second of their first write. A 30-minute smoothed
  burn chart (per-session lines, the cost line off by default), the vendor strip (one
  row per vendor seen in the store: today, this week, this month, tokens, refreshed once
  a minute), the turn ticker, and the forecast chart. See "The Now page" below.
- **History**. One page, filtered: period (day, week, month), range (last 7, 30, 90 days,
  custom), vendor, and owner once owner rules are configured. Totals, a "Burn per period"
  chart stacked one segment per harness (Claude Code, Codex, Copilot CLI and so on, fixed
  colours shared with the Now page), and a table for whatever is selected; the filter
  state lives in the URL so a view can be reopened. A per-client table appears once client
  rules are configured.
- **Sessions**. Every session, sortable, searchable, with a Harness column and filter (the
  real harness a session ran under, e.g. Claude Code, Codex, Copilot CLI, not the Surface
  it happens to share with another vendor), a Findings column (count by kind) and an
  expandable row listing each finding; click one to open the same turn detail drawer the
  Now page uses. The owner column and filter appear once owner rules are configured. A
  session priced by a vendor with no book entry for its exact model shows "no price"
  rather than a guessed figure.
- **Tools**. Which connectors, plugins and skills your sessions actually call, as a chart
  and a table, over the whole window.
- **About**. How Claude's own seat and allowance model works, in plain language, plus
  where the numbers come from and what they cannot show.

## The Now page

Every running session shows a card with a context gauge. Under the chart, each session's
turn ticker line can carry a marker; clicking a marker or its ticker line opens a drawer
with the model, token classes, tool calls that turn, files read, the gap since the
previous turn, and, when a finding applies, its cause and confidence.

Findings come from `internal/insight`, computed on the fly from the same windowed events
the Now snapshot already loads (nothing about them is stored):

- **re-prefill**: a turn's cache write exceeds the configured threshold. The cause is
  inferred in order: compaction just happened, the model changed since the previous turn,
  the gap since the previous turn exceeded the cache TTL, first turn after a resume, or
  "unknown".
- **compaction**: context size drops by more than 30% between consecutive turns while the
  session continues.
- **context-runway**: a linear fit over the last 10 turns of context size, reported as
  turns remaining to 80% and 90% of the model's context window.
- **expensive-turn**: a turn above the 95th percentile of the session's own turn cost or
  tokens, with its dominant token class named.

The forecast chart shows the plan line (the last four weeks, weekday-aware) and the live
line (the current rate carried to end of day and end of month), with an error band once
at least one week has been scored. Until then it shows history only, with the gate text
"forecast unlocks after the first scored week". Tokens only.

## Cost

The Now page shows tokens, context fill and cache hit rate: what a developer watches
while working. History, Sessions and export also show cost, on the vendor's headline
basis, named next to the figure: plan credits for GitHub Copilot, your configured
subscription share when `burnmon.json` carries one, else the API list price labelled
"upper bound".

Cost comes from dated price books built into the binary (`internal/pricing/books`, each
with its own source URL and check date), overridable per model from `burnmon.json`.
`burnmon-cli price-check` prints every book, its date and its source, so you always know
how current a number is. A vendor with no book at all (Hermes today), or no entry for the
exact model a session used, shows "no price"/"tokens only" rather than a guessed figure.

## Owner and client rules, active time

`burnmon.json` can carry an ordered `owners` table of rules, e.g.
`{"match": "C:\\dev\\work\\*", "owner": "Acme", "client": "ClientA", "remote": "github.com/org/repo"}`,
applied to a session's project path at ingest. Owner: the first rule whose `match` path
prefixes the project wins, an unmatched path gets `"personal"`. Client (optional, v0.3):
a rule whose `remote` matches the project's git origin (read straight from `.git\config`,
no git binary) wins first, then a rule whose `match` path prefixes the project, else
`"unassigned"`. Empty `owners` (the default) means no owner or client column anywhere. A
v0.2 `burnmon.json` with `owners` only, no `client`/`remote`, keeps working unchanged.
After changing the rules, re-apply them to sessions already in the store with
`burnmon-cli reown`; nothing else re-runs ingest for you.

Each session also gets an **active time**: the sum of gaps between consecutive turns, any
gap over `active_idle_minutes` (config, default 10 minutes) counting zero. Labelled
everywhere it appears as "active time, from transcript timestamps, not billable": a
signal, not a time-tracking replacement. Once client rules exist, History's per-client
table shows tokens, headline cost, active time and session count side by side for every
client, comparable even with one selected in the filter above it.

## Export and merge

`burnmon-cli export --since <date> --until <date> --owner <name>... --label <name> --out
<file>` writes daily rows (owner, client, vendor, model, token classes, cost on every
basis available, active minutes); no paths, session ids, prompts or project names ever
leave in the file. When `owners` rules exist, `--owner` is required, so a row you did not
ask for never leaves by accident. `burnmon-cli merge <file>... --out <dir>` combines any
number of exports (free, no cap) into `merged.json` and a `report.html` that opens
offline with no network script, one column per export's `--label`, totals by client,
vendor and week.

Both apps write the same file as `burnmon-cli export`, from the same settings places as
above. You type the folder as an absolute path (it starts as your Downloads folder), and
can set since, until and a label. When `owners` rules exist, tick at least one owner.

## The CLI

- `burnmon-cli`: build the dashboard and open it in the browser.
- `burnmon-cli live -json`: one JSON snapshot of the Now page, for scripting.
- `burnmon-cli tools -since 30d`: tool_calls totals: tool, calls, sessions, bytes.
- `burnmon-cli insight -json <session-id>`: findings for one session, table or JSON.
- `burnmon-cli reown`: re-apply owner and client rules to every event in the store.
- `burnmon-cli price-check`: print every price book and when it was last checked.
- `burnmon-cli export -owner NAME`: write a client-safe daily-rows export, see "Export
  and merge".
- `burnmon-cli merge FILE... -out DIR`: combine exports into `merged.json` and an offline
  `report.html`.
- `burnmon-cli graph -since 24h`: print the relation graph for a past window as JSON
  (sessions and subagents from the store, the live-only edges it recorded, and Foreman
  sprints when a Foreman folder is set). `-foreman <dir>` overrides `foreman_root`.
- `burnmon-cli waits -json`: list what waits on you and the outcomes of the last 24 hours
  from Foreman's state, read only. It needs `foreman_root` or `-foreman <dir>`, and
  prints a message and exits 1 without one. Its Foreman items are the same as in the
  Waits on you and Outcomes sections of BurnMon Dev; session and agent view items are not
  in its output.
- `burnmon-cli backfill-parents`: set the parent session on Claude subagent rows stored
  before the adapter read it. A dry run that only counts rows; `-apply` writes. Copy the
  store file first.

`burnmon-cli` also keeps every claudecost-era flag: `-months`, `-seat`, `-no-open`,
`-json data.json`, `-source DIR`, `-no-cache`, `-out DIR`, `-config`, `-version`. See
`burnmon-cli -h` and each subcommand's own `-h` for the full list.

## The app

Double-click `burnmon.exe`. One window opens, no console, no tray icon, no browser tab.
It collects on startup, again every 15 minutes (`-interval`, floor 5 minutes) while the
window stays open, and whenever you press "Refresh now". Close the window and the app is
gone; nothing keeps running in the background. No open ports on Windows, no admin rights
(`asInvoker`). Needs the WebView2 runtime (ships with Edge on Windows 10/11); without it,
the app writes the report once and opens it in your default browser instead.

Parsed transcripts are stored in `%LOCALAPPDATA%\burnmon\burnmon.db`, a SQLite database
migrated forward automatically on every startup (versioned, additive migrations only:
nothing already stored is ever dropped). Click the gear icon to edit subscription
numbers, seat counts and seat prices from inside the window; saving re-reads everything,
since cached sessions carry costs computed with the old numbers.

### macOS and Linux

`burnmon` and `burnmon-cli` also build for darwin and linux (amd64 and arm64,
`CGO_ENABLED=0`, pure Go, no cgo), attached to every tagged release. Neither platform has
WebView2, so there is no app window there: `burnmon` serves the dashboard from a
localhost-only web server (`http://127.0.0.1:47615/?t=<key>`, change the port with
`-port`), prints that link and opens it in the system default browser (`-no-open` to
skip). That server provides the same live
functions the Windows window binds (Now, History, the vendor strip, the forecast, session
findings, the turn drawer, Refresh now and Settings), refuses requests from any other host
or origin, and is the one open port BurnMon has, on these two platforms only. The page
reloads itself whenever the `-interval` re-collection rewrites it. Run on macOS (Apple
Silicon) since 2026-10-20; Linux is still untested on real hardware.

**The one open port.** On macOS and Linux, `burnmon` listens on `127.0.0.1:47615`. Other
computers cannot reach it. Other users and programs on the same computer can reach the
port, so every request needs a secret that `burnmon` makes at each start and keeps in a
file only you can read (`bridge-token` in the data folder, mode 0600). A link from an
earlier start stops working; open the link `burnmon` printed, or start it again. Starting
`burnmon` a second time opens the running one. If another program already uses port
47615, `burnmon` says so; start it with `-port <number>`. The Windows app has no server
and no open port.

### Help test macOS and Linux

This repo is public, and the macOS and Linux builds need real hardware. If you have a
Mac or a Linux box with Claude Code, Codex, Copilot CLI or Hermes on it, please try it
and open an issue with what you see. Either build from source (Go, then
`go build -o burnmon ./cmd/burnmon` and `go build -o burnmon-cli ./cmd/burnmon-cli`), or
take the `burnmon-<os>-<arch>` files the release workflow attaches to a tagged release.
Things worth checking:

- Does the browser open `dashboard.html`, and do your sessions show up on Now and Sessions?
- Are the default transcript roots in the table below right for your install?
- Does the store open: `~/.config/burnmon/burnmon.db` on Linux, `~/Library/Application
  Support/burnmon/burnmon.db` on macOS (Go's `os.UserConfigDir`)?
- Do file changes get picked up while the page is open? Neither platform has the native
  Windows watcher; they use the older per-directory watch, which was never run on a large
  tree.
- Does `burnmon-cli price-check` and `burnmon-cli export` work?

Please leave out prompts, project names and client names from anything you paste.

## What needs BurnMon

The Fleet and the Setup Doctor read the snapshot file, `%USERPROFILE%\.burnmon\snapshot.json`,
so they need `burnmon.exe` or `burnmon-dev.exe` running. Without either, the file goes
stale and they say BurnMon is not running. Groundwork works without BurnMon.

Either exe writes the file. With both running, one writes: the one that holds the lock
`%USERPROFILE%\.burnmon\snapshot.lock`. The other only reads. If the writer exits, the
other takes over at its next tick (5 seconds). The lock is an operating system file lock,
so a dead writer never blocks the other; the file holds an owner record (process id,
exe, the time the lock was taken) for diagnosis only. The snapshot also carries `wait_items`,
`outcome_items` and `agent_sessions`; fields and kinds are in `docs\snapshot_schema.md`.
Gates, sprints and failed checks in `wait_items` and `outcome_items` come from Foreman
and need `foreman_root`.

## BurnMon Dev

`burnmon-dev.exe` (`v0.4.0-beta.1`) is a second, developer-facing window: what running
AI agents burn (tokens, cost, turns) on top, what the laptop does (CPU, per-core heat
grid, memory, disk, network, GPU, harness process groups) below, on one time axis, so a
performance problem and a token spike show up together. It reuses `burnmon.exe`'s own
`internal\` packages and shares the same store (`%LOCALAPPDATA%\burnmon\burnmon.db`)
whether or not `burnmon.exe` is open at the same time; system samples go to a separate
`burnmon-dev.db` next to it, retained 7 days by default (`retention_days` in
`burnmon-dev.json`). Dark, dense, amber-on-black, built for a portrait second screen but
responsive down to a single compact column.

- **Start**: double-click `burnmon-dev.exe`, built alongside the other two by
  `.\build.ps1`. First launch opens an ordinary 1280x860 window, centered; after that its
  size, position, maximized state and monitor are remembered and restored if that monitor
  still exists.
- **F11** toggles fullscreen; **Esc** also leaves it.
- **L** switches between the dark theme (the default) and a light one, the whole window
  at once, charts included; the choice is remembered for the next start
  (in `burnmon-dev-view.json`, next to `burnmon-dev.db`). Vendor colours
  stay the same families, darkened only where they would be too faint on white.
- **M** opens the Agents map and **M** again closes it. It opens on its LIVE tab when the
  snapshot is current and on Last 24 hours otherwise; Last 7 days is a third tab. A box
  opens the session it stands for; the live map reads through `bdevAgentMapStart`, which builds off the UI thread.
- **One tick**: every panel repaints from one snapshot a second. The System chart and
  process-group history arrive as deltas (only samples the page does not hold yet), so
  the payload stays the same size however long the window runs. A late snapshot skips
  its paint, but never for more than 2 ticks in a row: a slow moment shows as lag, not
  a frozen screen. The drive and wifi readout refreshes on its own, so a slow `netsh`
  or a stalled drive can no longer hold up a system sample.
- **Minimized**: painting stops and the whole System zone's own sampler (system reading
  and process-group walk together) slows to one sample per 10s; both resume immediately,
  with one fresh render, the moment the window is restored.
- **Session colour**: one colour family per vendor (burn chart, legend, session cards,
  vendor strip, process groups, harness heatmap all draw from the same `HARNESS_HUE`);
  several open sessions of one vendor get distinct lightness shades of that same hue
  rather than a different colour each, so a session's colour matches everywhere it
  appears and never gets mistaken for another vendor's.
- **Microsoft To Do** panel: off by default, `"microsoft_todo_enabled": true` in
  `burnmon-dev.json` turns it on. Its own device-code sign-in (shows the code and URL in
  the panel, opens the browser), its own token cache under
  `%LOCALAPPDATA%\burnmon\`, separate from `burnmon.exe`. Read-only, and scrolls rather
  than cropping once there are more tasks than fit; while it is shown, the System chart
  gives up 30% of its height and To Do fills the bottom of the window (a window too small
  for To Do still drops it first, as before); task text is live only, never written
  to an export, a log or a committed screenshot.
- **Export**: the header's "EXPORT" button, or
  `burnmon-dev.exe export --since --until --out DIR [--redact]` (`--since`/`--until` take
  a local day, YYYY-MM-DD, or a full RFC3339 timestamp), writes an AI-ready bundle to
  `DIR\burnmon-dev_<yyyy-mm-dd_hhmm>\`: `summary.md` (machine profile, totals per
  harness/model, top expensive turns with the system state at that moment, advisor
  findings, a ready prompt asking a model for concrete improvements), `data.json` (full
  timeline, schema-versioned) and `daily.csv`. `--redact` replaces project, client and
  owner names with stable hashes; never exports prompt or response text either way.
- `refresh_ms` in `burnmon-dev.json` (default and floor both 1000) sets the shared paint
  cadence every panel renders on; the whole System zone (CPU total bar, the 20 per-core
  bars, the main chart, the process-groups rows and their sparklines, Memory/Disks/Network)
  now samples together on its own fixed 1s cadence, independent of `refresh_ms` - the
  process-group walk (a single system-wide process snapshot per walk, not one query per
  process, reusing the same snapshot buffer across walks) no longer lags behind on its own
  slower 3s clock, and the main chart/sparklines read from an in-memory 1s-resolution
  buffer rather than the 10s-spaced rows `burnmon-dev.db` itself holds. Persistence to
  `burnmon-dev.db` stays fixed at 10s regardless, so a faster sample or paint cadence never
  multiplies disk-write cost. Go process measured 0.21 percent average whole-machine CPU
  over 10 minutes with `burnmon.exe` co-running, under the 2 percent target despite the
  process walk now running three times as often.

### The Station

A hidden screen in `burnmon-dev.exe`: every live session is a character standing in the
room of the stage it is in right now. It exists for fun and for glanceability, and it reads
the same data as the rest of the window. It draws and reads nothing while it is closed.

- **P** opens the full-window isometric view. **P** again, or **Esc**, closes it.
- **O** swaps the whole burn zone (chart, session cards, vendor strip, turn ticker) for a
  flat pixel-art plan view of the same floor. **O** again closes it.
- **I** mounts the isometric view inside the burn zone, with System still visible below.
- **T** switches theme while it is open: a space station (the default) or a construction
  site. `"station_theme": "site"` or `"space"` in `burnmon-dev.json` sets the one it
  starts in; the gear dialog saves the same key under "Station starts as".
- Drag pans, the wheel zooms, a double-click resets the camera. Hover shows a small card;
  a click opens the session's latest turn as a compact card. **Esc** closes a turn popup
  first, then the Station, then fullscreen. Keys are ignored while you type in a field.

The stage comes from the session's latest tool call (the rules live in `internal\stage`,
first match wins):

| Stage | Site room | Space room | When |
|---|---|---|---|
| arriving | Site entrance | Airlock | session started under 15 s ago |
| reading | Drawing office | Archive | Read, Grep, Glob and similar |
| fetching | Delivery gate | Uplink | web fetch or search, any `mcp__` tool |
| planning | Site office | Plan table | todo, task and plan-mode tools |
| coding | Build zone | Fabricator | Edit, Write, `apply_patch`, unknown tools |
| running | Inspection and QA | Test chamber | Bash, PowerShell, Codex `exec` |
| delegating | Toolbox meeting | Briefing | a subagent task is open |
| thinking | Foreman's hut | Think pods | tool result in, silent 8 s or more |
| waiting | Site canteen | Lounge | no tool called, or a prompt is probably open |
| compacting | Skip container | Recycler | a compaction in the last 60 s |
| dormant | Parking area | Cryo bay | silent 10 minutes or more |

A twelfth room, the crane (site) or reactor (space), is not a stage. Its pulse follows the
summed token rate of all sessions.

What it cannot do: the transcripts show tool calls and finished turns, not the model's
thought, so "thinking" is a silence gap, not an observed state. A fast tool shows its room
for about 8 seconds before thinking takes over, and an agent stays at least 3.5 seconds in
a room before it walks on. Codex sessions first appear about 50 seconds in, when their
first token count lands. Art is CC0 (Kenney, Quaternius and others), listed in
`tools\station_atlas\CREDITS.txt`. Open issue: it costs more CPU than the rest of the
window, see Known issues.

### Sections, Plain mode and the Overall scrollbar

The window is 15 switchable parts in four groups. The gear dialog has one list of part
switches per mode. Standard shows every part by default. Plain shows five by default
(Waits on you, Outcomes, Burn, Agents map, Sessions) and uses plain words; Plain mode makes
no model call. The Overall scrollbar switch (`overall_scrollbar`, default on) lets the
page scroll instead of dropping parts that do not fit; a note counts the sections a small
window hides. The choice is stored in `burnmon-dev-view.json` next to `burnmon-dev.db`
(`plain_mode`, `overall_scrollbar`, `parts_standard`, `parts_plain`, `controls`). A file
from the earlier seven-section layout is migrated in memory and written in the new shape
on the first save.

| id | label | group |
|---|---|---|
| `waits` | Waits on you | Status |
| `outcomes` | Outcomes | Status |
| `burn` | Burn | Status |
| `git` | Git | Status |
| `agents_map` | Agents map | Status |
| `sessions` | Sessions | Status |
| `burn_30min` | Burn last 30 min | Sessions last 30 min |
| `vendor_strip` | Vendor strip | Sessions last 30 min |
| `activity` | Activity | Sessions last 30 min |
| `heatmap` | Heatmap harness x minute | Sessions last 30 min |
| `turn_ticker` | Turn ticker | Sessions last 30 min |
| `system` | System | Processes last 30 min |
| `process_groups` | Process groups (with memory, disks and network) | Processes last 30 min |
| `processes` | Processes | Processes last 30 min |
| `todo` | To Do | Microsoft To Do |

### Waits on you and Outcomes

Two parts of BurnMon Dev list what needs you and what just ended. Waits on you holds open
Foreman gates, waiting sprints with parked tasks, stopped sprints with a failing check,
waiting sessions, and the prompts and questions the agent view reports. Outcomes holds
finished and stopped sprints and failed checks of the last 24 hours. A sprint has at most one
`finished` row, and none when a failing check of it is within the window: it then has one
`failed_check` row per failing check, so two when both test and verify failed.
Each item has one of these kinds in the snapshot:

| kind | what it is |
|---|---|
| `gate` | a Foreman flow with an open gate |
| `sprint` | a waiting sprint |
| `failed_check` | a sprint whose test or verify check failed: a wait while the sprint is stopped, an outcome when it failed in the last 24 hours |
| `question` | an agent view session that asks a question |
| `prompt` | an agent view session that waits on a permission prompt |
| `session` | a session the graph shows as waiting on you, or an agent view session that is blocked or waits on a dialog, sandbox or worker request |
| `finished` | a finished or stopped sprint (Outcomes) |

Both parts show every item in a bounded scroll list with a count. A spec or result link
opens through `bdevOpenItemFile`, which opens only a file named by the current snapshot
and only `.md`, `.html` or `.txt` (an `.html` file opens in the default browser). The
parts turn grey when the snapshot is older than two ticks. Foreman's state files are read
every 7.5 seconds. The B3 report works out that a new gate can take up to about 15.7
seconds to reach the page (calculated, not measured).

The agent view is off by default. With `agent_view` on, BurnMon runs `claude agents --json`
every 30 seconds, each run bounded to 10 seconds, with a hidden window. It starts only a
`claude.exe` that can be started directly. When `claude` on `PATH` is the npm shim
`claude.cmd`, BurnMon reads the shim as text, never runs it, and starts the `claude.exe` it
names; a `.ps1` or `.bat` is refused. When the agent view is off or refused, Waits on you in
BurnMon Dev opens with one line that says so and names the `burnmon.json` key that fixes it:
`agent_view` when it is off, `claude_path` with the refusal reason when `claude` does not
resolve. BurnMon Dev reads `burnmon.json` once at start, so a fix shows after a restart. It
has not been run
against the real `claude` (B3 report). Window level behaviour of these parts is not
checked here: uicheck `d36` renders both parts from the example snapshot, and `d29`, `d33`
and `d34` cover the layout, but none of them was run in B1 to B4 because the apps were open.

### Controls

BurnMon Dev can run four controls: `stop` and `attach` on a background agent view session
(`internal\controls\check.go` refuses any other kind, so an interactive session has none), and
`approve` and `send_back` on a Foreman gate. Every control is off by default and has its
own switch in the gear dialog. Besides the switch, `stop` and `attach` need `agent_view`
on and a `claude.exe` that BurnMon can start directly: `claude_path`, else the first
`claude` on `PATH` (see "Foreman and Claude Code settings"). `approve` and `send_back`
need `foreman_root`, the file `flow_transition.py` in its `flows` folder, subfolder `_lib`,
and a `python.exe` on `PATH` (the Microsoft Store alias is refused); they do not start `claude`. A control
that cannot be used shows its reason in the gear dialog.

- `stop` runs `claude stop <id>` and `attach` runs `claude attach <id>`. Both are refused for a
  session that has ended, because attach would restart a stopped session.
- `approve` writes the verdict approve at gate G1 and G2 and proceed at G3. `send_back` writes
  reject at G1 and G2. Send back at G3 and every G4 control are not offered. The verdict
  goes through Foreman's own gate command.
- A control first shows a preview of the exact command. A separate Confirm button runs it.
  The preview token is single use, lives 5 seconds and cannot be confirmed in the first 400
  ms. The target is checked again just before it runs.
- Each executed run writes a start line and a result line to `controls-audit.ndjson`, next
  to the store file, before and after it runs; if the audit cannot be written nothing
  runs. The file holds no output and no session text.
- BurnMon Dev refuses a session that is itself, one of its ancestors, one whose folder is
  its own or contains it, and it turns every control off when it runs as administrator.
- There is no reply control: Claude Code documents no shell command that replies to a
  blocked background session.

Not run live: no control was run against a real session, process or flow (B4 report),
and the audit lines of a live run are not quoted anywhere.

## Configuration

Prices and the subscription calibration are compiled-in illustrative defaults (see
`internal\pricing\pricing.go`), not a real invoice. Drop a `burnmon.json` next to the exe
(app and CLI both check there first, then `%LOCALAPPDATA%\burnmon\burnmon.json` for a
fixed, read-only-share install) with your own numbers; any subset of fields overrides the
defaults. `burnmon.example.json` lists these keys, including `owners` (empty by default);
three newer keys are not in it yet, see "Foreman and Claude Code settings".

If no `burnmon.json` is found, both binaries still read a `claudecost.json` in the same
two locations: the pre-rename config name, kept working for anyone who has not renamed
their file yet. `burnmon.json` always wins when both are present. Rename to
`burnmon.json` when convenient; there is no reason to keep using the old name once you
have.

`keep_history` (default `true`) goes in `burnmon.json` and is read by both apps. It does
not go in `burnmon-dev.json`. While `keep_history` is on, deleting a transcript does not delete BurnMon's copy of it: its events stay in the store and only the read position for
that file is forgotten. Set it to `false` and a transcript that vanishes takes its events
with it. Both apps can also delete history from their own settings (`burnmon.exe` in
Settings, `burnmon-dev.exe` in the gear dialog): older than N days, one session, or all.
Each delete needs two presses, the second on a button that now reads "Confirm: ...". A
delete keeps the read position, so deleted turns are not read back in from a transcript
that is still on disk.

### Foreman and Claude Code settings

Three more keys go in `burnmon.json`. `burnmon.example.json` does not list them yet.

```json
{
  "foreman_root": "C:\\example\\foreman",
  "agent_view": true,
  "claude_path": "C:\\example\\claude.exe"
}
```

- `foreman_root`: the folder that holds Foreman's `flows` and `sprints` folders. BurnMon reads
  `sprints\<project>\<slug>\sprint.json` and `flows\<project>\<flow>\flow.json` there, and
  the controls need `flows\_lib\flow_transition.py`. Empty (the
  default) means no Foreman: no Foreman links in the graph and no Foreman items in Waits on
  you and Outcomes. `burnmon-cli graph` and `burnmon-cli waits` take `-foreman <dir>` to
  override it.
- `agent_view`: `true` turns on the agent view poller. Default off.
- `claude_path`: an absolute path to `claude.exe`. It must be an existing file and a
  `.exe` on Windows; a `.cmd` shim is refused here. Empty means a `claude.exe` on `PATH`, or
  the `claude.exe` that the npm shim `claude.cmd` on `PATH` names, which the poller and the
  controls accept too.

`claude` on `PATH` is often an npm shim, `claude.cmd`. For an npm install, the shim runs a
real `claude.exe` under the npm prefix, in
`<npm prefix>\node_modules\@anthropic-ai\claude-code\bin\claude.exe`. BurnMon reads the shim as
text, never runs it, and takes the quoted `%dp0%` or `%~dp0` path that ends in `claude.exe`,
when that file exists. A shim that names no `claude.exe` (an older one runs `node.exe` with a
`.js` file) or names a missing one is refused, and so is a `.ps1` or `.bat` shim; the agent
view source says why. Then `claude_path` is the manual fix: open the shim in a text editor to
see the exact path, and put that path in `claude_path`. On the laptop where this was checked
(read only) the shim names that file and BurnMon finds it.

## WSL

Claude Code running inside a WSL distribution (Ubuntu under WSL, most commonly) is a
Linux process with a Linux `$HOME`, so its transcripts live inside the distro, not under
any Windows folder. BurnMon finds them automatically: it reads which WSL distributions
are installed from the registry, then looks for each one's `.claude/projects` folder,
including one moved via a `CLAUDE_CONFIG_DIR` set in a shell startup file.

A few things are worth knowing:

- Reading Linux files from Windows goes over WSL's own file-sharing layer and is slower
  than reading a native NTFS folder. WSL transcripts are re-read on their own, slower
  clock (`-wsl-interval`, default 4 hours) while native Windows transcripts keep the
  normal refresh. Refresh now and saving Settings always re-read everything, WSL
  included.
- Accessing a stopped WSL 2 distribution's files can start it in the background, which
  costs a few seconds and some memory. This only happens on the slower WSL cadence above.
- Set `"wsl_scan": "off"` in `burnmon.json` to turn WSL scanning off entirely; the first
  thing to try if a scan seems slow and you want to rule WSL out.
- If detection misses a distro, add the folder directly with `"extra_sources": ["..."]`
  in `burnmon.json`, or with a repeatable `-source` flag.

## What BurnMon never does

No server on Windows, no network call to the internet of any kind, no telemetry, no
account. On macOS and Linux there is one local web server on 127.0.0.1 for the dashboard,
protected by a secret that changes every start. The optional cloud plugin
runs inside Claude's own cloud session, not in BurnMon, and the Claude app copies its file to
your disk. BurnMon reads only your own transcript folders, never aggregates or compares
across people, and refuses no one because there is nothing to refuse: other people's
usage is simply not on your disk.
Costs shown for Claude are allocated shares of a flat monthly invoice, not money owed by
anyone.

Two optional features start programs on this computer, and only after you switch them on:
the agent view poller (`agent_view`) and the controls. The poller runs only
`claude agents --json` (`internal\snapshotwriter\agentview.go`). `stop` and `attach` run
`claude stop <id>` and `claude attach <id>`; `approve` and `send_back` run `python` on a
script that calls Foreman's own gate command. All of them use documented commands, no
undocumented endpoint.

## Build

Requires Go (winget install GoLang.Go). Then, in PowerShell, from this folder:

    .\build.ps1

First run needs internet access: it fetches `go-winres` for the icons and `go mod tidy`
resolves the WebView2 binding. Produces `burnmon-cli.exe`, `burnmon.exe` and `burnmon-dev.exe`, all portable
single files. Copy them anywhere; no install. If a `build.local.ps1` exists next to
`build.ps1` (gitignored, machine-local), it runs afterward.

## Default transcript roots, by OS

| Tool | Windows | macOS | Linux |
|---|---|---|---|
| Claude Code / Cowork | `%USERPROFILE%\.claude\projects` | `~/.claude/projects` | `~/.claude/projects` |
| Codex | `%USERPROFILE%\.codex\sessions` (or `$CODEX_HOME`) | `~/.codex/sessions` | `~/.codex/sessions` |
| Hermes | `%HERMES_HOME%\state.db`, else `%LOCALAPPDATA%\Hermes\state.db` | `$HERMES_HOME/state.db`, else `~/.hermes/state.db` | `$HERMES_HOME/state.db`, else `~/.hermes/state.db` |
| GitHub Copilot CLI | `%COPILOT_HOME%\session-store.db`, else `~/.copilot/session-store.db` | `~/.copilot/session-store.db` | `~/.copilot/session-store.db` |
| Cowork in the cloud | `~\.burnmon\cloud` and `.burnmon\cloud` next to `burnmon.exe` (both created at start, filled by the `burnmon-cloud` plugin) | same | same |
| GitHub Copilot in VS Code | wherever `github.copilot.chat.otel.outfile` points, matched by `copilot_vscode_otel_file` | same (no default: the setting has none on any OS, you always choose the path) | same |

WSL distro detection (the registry-based scan for Claude Code and Codex transcripts
inside a WSL distribution) only exists on Windows; it is a no-op everywhere else.

## Roadmap

Priority order lives in `02_roadmap\roadmap.md`; this is the short public version, as of
2026-10-06.

- **Now**: the Station polish pass (layout, seated poses, labels) and finding the cause of
  its CPU climb. The project is otherwise parked until 2026-11-01, so expect little else
  before then.
- **Next**: make Waits on you and the agent view correct and fast against real sessions;
  test and fix macOS and Linux with help from real users (see above);
  a plan-limits panel (how much of a seat or allowance is left), briefed but not started.
- **Later, undecided**: a local OTLP receiver, signed Windows builds, a native window off
  Windows. A decision on whether and how to continue past 2026-12-19 is open.
- **Not planned**: a server, an account, telemetry, or any undocumented vendor endpoint.

Ideas and bug reports are welcome as GitHub issues.

## Known issues

- **Station CPU climbs while it is open.** Two clean 30 minute runs did not reproduce the
  worst earlier ramp, but a Station left open costs about 4.5 points of one core more than
  closed, and CPU follows the page's animation rate. Cause not found. Closing it (Esc)
  removes the cost.
- **macOS and Linux have no app window**; the dashboard opens in the browser instead, served by a local web server on 127.0.0.1 (see "macOS and Linux"). Linux is untested on real hardware.
- **History is slow on a large store.** Every filter change re-reads the whole store, about
  3 seconds at roughly 55,000 events.
- **Time bounds compare text.** A stored timestamp in the first second of a range can be
  wrongly excluded or included, because trailing zeros drop out of RFC3339 strings. Cost
  is at most one event at a boundary.
- **Old forecast rows.** One ISO week scored before the v0.3.2 local-time change keeps a
  UTC week start.
- **About tab only describes Claude's seat model**, not the other vendors.
- **Copilot Business and Enterprise** have no per-seat price wired in, and their pooled
  billing does not fit the single-machine model, so only plan credits for individual
  plans are shown.
- **No price, no guess.** Hermes, and any model without a book entry, show tokens only.
- **Microsoft To Do** due dates are tested but were not checked against a live sign-in.
- **Both exes share one store.** Running `burnmon.exe` and `burnmon-dev.exe` together
  makes both ingest the same files. Measured safe, but it is double work.
- **Test suite flakes.** `scripts\uicheck.ps1` d19 and d20 are skipped on a single small
  screen, and w1 and w8 are known flaky; none has shown a user-visible bug.
- **Copilot in VS Code** reports every client as "unassigned" until GitHub adds a
  workspace attribute to its telemetry.

The full list, with measurements, is in `STATUS.md`.

## Status

Shipped: `v0.3.0` (price books, client map, export and merge, Copilot in VS Code, macOS and
Linux browser-mode builds), 2026-09-24; `v0.3.1`, a cleanup pass, 2026-09-24; `v0.3.2`,
shared ingest performance and local time everywhere, 2026-09-26. `v0.4.0-beta.1`, 2026-10-30: all three exes carry one version again; it adds
the agent map, Waits on you, Outcomes, the first controls and the section layout to
BurnMon Dev. Beta: the controls and the agent view are not yet proven against real
sessions, and the agent view stays empty when `claude` on `PATH` is a `.cmd` shim. See `STATUS.md` for what is true at the current
commit and `SESSION_LOG.md` for the session-by-session record. The BurnMon Dev text above
(sections, Waits on you, Outcomes, controls) is written from the code of the branch this
README sits on; `STATUS.md` says which of those parts are in `main`.

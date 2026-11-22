![Glim, the lamp of BurnMon Dev, plays a 48 second demo loop](../../assets/glim/glim-demo.svg)

# BurnMon Dev

`burnmon-dev.exe` (`v0.5.0-beta.2`) is the developer window of BurnMon. It shows what running AI
agents burn, what the laptop does, and what waits on you, on one time axis.

Parent index: `README.md`. Settings file `burnmon.json` and the transcript roots: `cmd\burnmon\README.md`.

## 1. What it is

Tokens, cost and turns of the running agents are on top. CPU, a per-core heat grid, memory, disk,
network, GPU and harness process groups are below. A performance problem and a token spike show up
together, so you can see which one caused the other.

It reuses the `internal\` packages of `burnmon.exe` and shares its store
(`%LOCALAPPDATA%\burnmon\burnmon.db`), whether or not `burnmon.exe` is open. System samples go to a
separate `burnmon-dev.db` next to it, kept 7 days by default (`retention_days` in `burnmon-dev.json`).
The look is dark, dense and amber on black. It is built for a portrait second screen and works down to
one compact column.

## 2. Where it fits

| Part | Job |
|---|---|
| Foreman | the process: flows, gates, sprints |
| BurnMon | watching, long view: tokens and cost per day, vendor, project, client |
| BurnMon Dev | watching, short view: the last 30 minutes, Waits on you, the agent map |
| Control Centre | steering: units, boards, Approve, Send back, comments |
| Skills | the tools an agent uses |

BurnMon Dev needs no other part. It reads every Claude Code, Codex and Copilot session on this
machine, wherever it runs. The Claude desktop app shows only its own sessions, up to the 20 most
recent. It cannot see sessions started in a terminal or in VS Code, or other coding tools.

What it does not need: `burnmon.exe` (both write the same snapshot file; see section 4), Foreman (only
for Foreman items), an account or a network. Its three optional programs, the agent view poller, Stop and the
cloud pull, start only after you switch them on. BurnMon itself makes no network call. With Read Cowork (Cloud)
sessions on (the gear, saved as `cloud_pull` in `burnmon.json`), it starts your own Claude Code CLI, which goes online
as you; the switch and what it reads are in `cmd\burnmon\README.md`, section Cowork (Cloud) sessions.

## 3. Install

On Windows, the release attaches a Windows amd64 zip; its first start is not tested. To build from source:

```powershell
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon
Set-Location C:\ZND\50_projects\burnmon
.\build.ps1
```

`.\build.ps1` builds `burnmon-dev.exe` next to the other exes. It is one portable file: copy it
anywhere. The first run needs internet access (it fetches `go-winres` for the icons). If a
`build.local.ps1` exists next to `build.ps1` (gitignored, machine-local), it runs afterward.
macOS has a native window, built with cgo in `.github\workflows\macos.yml` and shipped in the macOS zips as one
of three apps, unsigned (see the `xattr` line in `README.md`). Seen in the Mac check of 2026-11-22 on macOS
27.2.1, Apple silicon, with the arm64 zip: it starts from the zip and fits the 1024x768 screen. Not tested: the
amd64 zip, signing and notarizing.

## 4. Use

Double-click `burnmon-dev.exe`. The first launch opens an ordinary 1280x860 window, centered. After that
its size, position, maximized state and monitor are remembered, and restored if that monitor still exists.
Open Settings (the gear) first: the section sizes live there (section 5).

- **F11** toggles fullscreen; **Esc** also leaves it.
- **L** switches between the dark theme (the default) and a light one, the whole window at once, charts
  included. The choice is remembered in `burnmon-dev-view.json`, next to `burnmon-dev.db`. Vendor colours stay
  the same families, darkened only where they would be too faint on white.
- **M** opens the Agents map and **M** again closes it. It opens on its LIVE tab when the snapshot is
  current and on Last 24 hours otherwise. Last 7 days is a third tab. A box opens the session it stands
  for. The live map reads through `bdevAgentMapStart`, which builds off the UI thread.
- **P** opens the Station, the full-window isometric view. **P** again, or **Esc**, closes it.
- **O** swaps the whole burn zone (chart, session cards, vendor strip, turn ticker) for a flat pixel-art
  plan view of the same floor. **O** again closes it.
- **I** mounts the isometric view inside the burn zone, with System still visible below.
- **T** switches the Station theme while it is open: a space station (the default) or a construction site.

Other behaviour:

- **One tick.** Every panel repaints from one snapshot a second. The System chart and process-group
  history arrive as deltas, so the payload stays the same size however long the window runs. A late
  snapshot skips its paint, but never for more than 2 ticks in a row: a slow moment shows as lag, not a
  frozen screen. The drive and wifi readout refreshes on its own, so a slow `netsh` or a stalled drive
  cannot hold up a system sample.
- **Minimized.** Painting stops and the whole System zone's sampler (system reading and process-group
  walk together) slows to one sample per 10 s. Both resume at once, with one fresh render, when the
  window is restored.
- **Session colour.** One colour family per vendor: burn chart, legend, session cards, vendor strip,
  process groups and harness heatmap all draw from the same `HARNESS_HUE`. Several open sessions of one
  vendor get distinct lightness shades of that hue, so a colour is never mistaken for another vendor.
- **Header.** One line from 1280 px wide; the big number scales from 24 to 48 px. Every gear button sits
  at the right of its row.
- **Snapshot.** Either exe writes `%USERPROFILE%\.burnmon\snapshot.json`; Groundwork's Fleet and Setup
  Doctor read it. With both running, one writes: the one that holds the lock
  `%USERPROFILE%\.burnmon\snapshot.lock`. The other only reads. If the writer exits, the other takes
  over at its next tick (5 s). The lock is an operating system file lock, so a dead writer never blocks
  the other; the file holds an owner record (process id, exe, time the lock was taken) for diagnosis only.
  Fields and kinds are in `docs\snapshot_schema.md`. Groundwork works without BurnMon.
- **Microsoft To Do** panel. Off by default; `"microsoft_todo_enabled": true` in `burnmon-dev.json` turns
  it on. It has its own device-code sign-in (the panel shows the code and URL and opens the browser) and its
  own token cache under `%LOCALAPPDATA%\burnmon\`, separate from `burnmon.exe`. It is read only, and scrolls
  rather than cropping when more tasks exist than fit. Task text is live only, never written to an export, a
  log or a committed screenshot.
- **Notification.** When an item enters Blocked or Your turn in Waits on you, BurnMon Dev shows a Windows
  notification (a tray balloon, which Windows 10 and 11 show as a toast) with the item's kind and title
  only: no prompt text, no command, no path. One per item per 10 minutes. Several at once become one
  ("3 things wait on you"). Done never notifies. What already waits at start stays quiet, including what
  the startup backfill brings in (sessions that changed while BurnMon Dev was closed): it notifies only from
  the second snapshot written after that backfill. A snapshot that is missing, stale or written before the
  agent view's first poll is skipped, so none of these causes a burst. A click brings the window to the
  front. It is on by default: `"toast_enabled": false` in `burnmon-dev.json`, or the gear dialog's "Show a
  notification when something waits on you", turns it off at once (the file is read on every new snapshot).
  Turning it back on does not notify what entered while it was off. It runs on its own hidden window and
  thread, never the UI thread, and reads only the snapshot file and `burnmon-dev.json`. Its tray icon takes
  one of two IDs in turn, so Windows' list of tray icons (Settings, Taskbar) gains at most two entries for it.
  On macOS the banner is attributed to Script Editor and a click does nothing.
- **Export.** The header's "EXPORT" button, or `burnmon-dev.exe export --since --until --out DIR [--redact]`
  (`--since` and `--until` take a local day, YYYY-MM-DD, or a full RFC3339 timestamp). It writes an AI-ready
  bundle to `DIR\burnmon-dev_<yyyy-mm-dd_hhmm>\`: `summary.md` (machine profile, totals per harness and model,
  top expensive turns with the system state at that moment, advisor findings, and a ready prompt asking a model
  for concrete improvements), `data.json` (full timeline, schema-versioned) and `daily.csv`. `--redact` replaces
  project, client and owner names with stable hashes. Neither mode exports prompt or response text.
- **Burn Report.** The gear dialog's Export history card has a Burn Report button (binding `bdevBurnReport`) and
  a "show project names" checkbox, unticked, so project names are redacted by default. It covers the last 14 days
  and writes one zip, `burn-report-<YYYY-MM-DD>.zip`, into the folder typed in that card (it starts as the
  Downloads folder). The zip holds `burn-report.html`, which opens offline, and `bundle.json`. The page footer
  reads: "Want an analysis of this report? Send the zip to info@zerononsense.dev. One-off analysis: EUR 49."

### Waits on you and Outcomes

Two parts list what needs you and what just ended. Waits on you holds open Foreman gates, waiting sprints with
parked tasks, stopped sprints with a failing check, waiting sessions, and the prompts and questions the agent
view reports. Outcomes holds finished and stopped sprints and failed checks of the last 24 hours. A sprint has
at most one `finished` row, and none when a failing check of it is within the window: it then has one
`failed_check` row per failing check, so two when both test and verify failed. Each item has one of these kinds
in the snapshot:

| kind | what it is |
|---|---|
| `gate` | a Foreman flow with an open gate |
| `sprint` | a waiting sprint |
| `failed_check` | a sprint whose test or verify check failed: a wait while the sprint is stopped, an outcome when it failed in the last 24 hours |
| `question` | an agent view session that asks a question |
| `prompt` | an agent view session that waits on a permission prompt |
| `session` | a session the graph shows as waiting on you, or an agent view session that is blocked or waits on a dialog, sandbox or worker request |
| `finished` | a finished or stopped sprint (Outcomes) |

A session item is titled with its project folder and a short id of 8 characters, for example `burnmon 1a2b3c4d`,
so two sessions in one folder stay apart. The short id is the first 8 hex digits of the UUID in the session id
(Claude Code), the last 8 when it is a version-7 UUID (Codex: its start is a clock, the same for sessions started
a minute apart), or else the last 8 characters of the session id (Hermes ids start with the date, so their tail
differs). A session id shorter than 8 characters is shown whole. A session without a project folder, such as a
Cowork cloud session, shows its agent instead (`cowork-cloud 1a2b3c4d`), and one with neither shows
`session 1a2b3c4d`. The full session id is never the title.

Both parts show every item in a bounded scroll list with a count. A refresh leaves each list where it was
scrolled. In Waits on you the Done group (close-out, can-be-closed and branch-gone items) starts collapsed at
every start: only its header with the count shows, so Blocked and Your turn come first. A click on the header,
or Enter or Space on it, opens and closes it. The amber number in the part's header counts the Blocked items
only. A spec or result link opens through `bdevOpenItemFile`, which opens only a file named by the current
snapshot and only `.md`, `.html` or `.txt` (an `.html` file opens in the default browser). The parts turn grey
when the snapshot is older than two ticks.

Foreman's `sprints` and `flows` folders are watched. A change to a file BurnMon reads there (`sprint.json`,
`action-log.ndjson`, `report.md`, `flow.json`, `foreman.json`) is written to the snapshot within about a second,
one write per burst of changes. The index and board files Foreman rewrites on every step never cause a write. The
7.5-second read of Foreman's state files stays as the fallback, also when a folder cannot be watched. BurnMon Dev
checks the snapshot file's time and size every second and reads it again when they changed, so a new snapshot
reaches these parts within about a second more; its full read every 5 seconds stays as the fallback.

The agent view is off by default. With `agent_view` on, BurnMon runs `claude agents --json` every 30 seconds, each
run bounded to 10 seconds, with a hidden window. It starts only a `claude.exe` that can be started directly. When
`claude` on `PATH` is the npm shim `claude.cmd`, BurnMon reads the shim as text, never runs it, and starts the
`claude.exe` it names. A `.ps1` or `.bat` is refused. When the agent view is off or refused, Waits on you opens
with one line that says so and names the `burnmon.json` key that fixes it: `agent_view` when it is off,
`claude_path` with the refusal reason when `claude` does not resolve. The line follows the app that writes the
snapshot (`burnmon.exe` or BurnMon Dev, whichever holds the snapshot lock), which reads `burnmon.json` once at
start: a changed setting shows after restarting that app, `burnmon.exe` too when it runs. B8 ran it against the
real `claude` on the owner's laptop: with `agent_view` on, the snapshot listed a background agent started for the
test.

### Controls

BurnMon Dev has one control: Stop. It is off by default and has its own switch in the gear dialog. Stop runs
`claude stop <id>` on a background agent view session. `internal\controls\check.go` refuses any other kind, so an
interactive session has no Stop. Stop is refused for a session that has ended. It needs `agent_view` on and a
`claude.exe` that BurnMon can start directly: `claude_path`, else the first `claude` on `PATH` (see section 5).

The other controls moved to the Control Centre Settings page on 2026-11-15: `attach`, `approve`, `send_back` and
`close_sprint`. They keep the same keys in `burnmon-dev-view.json`, so both windows read one file, and BurnMon Dev
draws no chips for them. See `cmd\controlcentre\README.md`.

- A control first shows a preview of the exact command. A separate Confirm button runs it. The preview token is
  single use, lives 5 seconds and cannot be confirmed in the first 400 ms. The target is checked again just
  before it runs.
- Each executed run writes a start line and a result line to `controls-audit.ndjson`, next to the store file,
  before and after it runs. If the audit cannot be written, nothing runs. The file holds no output and no session
  text.
- BurnMon Dev refuses a session that is itself, one of its ancestors, or one whose folder is its own or contains
  it. It turns every control off when it runs as administrator.
- There is no reply control: Claude Code documents no shell command that replies to a blocked background session.

### The Station

A hidden screen: every live session is a character standing in the room of the stage it is in right now. It
exists for fun and for glanceability, and it reads the same data as the rest of the window. It draws and reads
nothing while it is closed. Keys are in section 4.

- `"station_theme": "site"` or `"space"` in `burnmon-dev.json` sets the theme it starts in. The gear dialog saves
  the same key under "Station starts as".
- Drag pans, the wheel zooms, a double-click resets the camera. Hover shows a small card; a click opens the
  session's latest turn as a compact card. **Esc** closes a turn popup first, then the Station, then fullscreen.
  Keys are ignored while you type in a field.

The stage comes from the session's latest tool call. The rules live in `internal\stage`; the first match wins.

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

A twelfth room, the crane (site) or reactor (space), is not a stage. Its pulse follows the summed token rate of
all sessions.

What it cannot do: the transcripts show tool calls and finished turns, not the model's thought, so "thinking" is a
silence gap, not an observed state. A fast tool shows its room for about 8 seconds before thinking takes over, and
an agent stays at least 3.5 seconds in a room before it walks on. Codex sessions first appear about 50 seconds in,
when their first token count lands. Art is CC0 (Kenney, Quaternius and others), listed in
`tools\station_atlas\CREDITS.txt`.

## 5. Settings

Three files hold settings. `burnmon.json` (shared with BurnMon, see `cmd\burnmon\README.md`) carries three keys
BurnMon Dev uses. `burnmon-dev.json` and `burnmon-dev-view.json` are BurnMon Dev's own.

```json
{
  "foreman_root": "C:\\example\\foreman",
  "agent_view": true,
  "claude_path": "C:\\example\\claude.exe"
}
```

- `foreman_root`: the folder that holds Foreman's `flows` and `sprints` folders. BurnMon reads
  `sprints\<project>\<slug>\sprint.json` and `flows\<project>\<flow>\flow.json` there, and the controls need
  `flows\_lib\flow_transition.py`. Empty (the default) means no Foreman: no Foreman links in the graph and no
  Foreman items in Waits on you and Outcomes. `burnmon-cli graph` and `burnmon-cli waits` take `-foreman <dir>` to
  override it.
- `agent_view`: `true` turns on the agent view poller. Default off.
- `claude_path`: an absolute path to `claude.exe`. It must be an existing file and a `.exe` on Windows; a `.cmd`
  shim is refused here. Empty means a `claude.exe` on `PATH`, or the `claude.exe` that the npm shim `claude.cmd` on
  `PATH` names, which the poller and Stop accept too.

`claude` on `PATH` is often an npm shim, `claude.cmd`. For an npm install, the shim runs a real `claude.exe` under
the npm prefix, in `<npm prefix>\node_modules\@anthropic-ai\claude-code\bin\claude.exe`. BurnMon reads the shim as
text, never runs it, and takes the quoted `%dp0%` or `%~dp0` path that ends in `claude.exe`, when that file exists.
A shim that names no `claude.exe` (an older one runs `node.exe` with a `.js` file) or names a missing one is
refused, and so is a `.ps1` or `.bat` shim. The agent view source says why. Then `claude_path` is the manual fix:
open the shim in a text editor to see the exact path, and put that path in `claude_path`. On the laptop where this
was checked (read only) the shim names that file and BurnMon finds it.

`burnmon-dev.json` keys: `retention_days` (default 7), `refresh_ms` (default and floor both 1000),
`microsoft_todo_enabled` (default off), `toast_enabled` (default on), `station_theme` (`space` or `site`).
`refresh_ms` sets the shared paint cadence every panel renders on. The whole System zone (CPU total bar, the 20
per-core bars, the main chart, the process-group rows and their sparklines, Memory, Disks, Network) samples
together on its own fixed 1 s cadence, independent of `refresh_ms`. The process-group walk is a single
system-wide process snapshot per walk, not one query per process, and reuses the same snapshot buffer across
walks. The main chart and the sparklines read from an in-memory buffer at 1 s resolution, not from the rows
`burnmon-dev.db` holds. Persistence to `burnmon-dev.db` stays fixed at 10 s, so a faster sample or paint cadence
never multiplies disk-write cost. Measured: the Go process used 0.21 percent of whole-machine CPU on average over
10 minutes with `burnmon.exe` co-running, under the 2 percent target, with the process walk running three times as
often as before.

`burnmon-dev-view.json` (next to `burnmon-dev.db`) holds what the gear dialog saves: `plain_mode`,
`overall_scrollbar`, `parts_standard`, `parts_plain`, `controls`, `sections_pct` and `panes_pct`. A file from the
earlier seven-section layout is migrated in memory and written in the new shape on the first save. An old
`heights` key is ignored on load and dropped on the next save.

### Sections, Plain mode and the Overall scrollbar

The window is 15 switchable parts in four groups. The gear dialog has one list of part switches per mode.
A fresh install starts in Standard with these parts on. Standard: Waits on you, Agents map, Sessions, Burn last
30 min, Vendor strip, Activity, Heatmap, Turn ticker, System and Process groups. Plain: Waits on you, Agents
map, Sessions, Burn last 30 min, Vendor strip and System, in plain words. Outcomes, Burn, Git, Processes and To Do
are off in both modes, and so is every control (Stop included). Plain mode makes no model call. The Overall
scrollbar switch (`overall_scrollbar`) is off by default; switched on, it lets the page scroll instead of
dropping parts that do not fit, and a note counts the sections a small window hides. A saved
`burnmon-dev-view.json` always wins over these defaults.

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

#### Section sizes are typed numbers

You set sizes with numbers, not by dragging. The gear dialog has one table, "Sections each mode shows": a name
column, then a tick and a percent box for Standard, then the same for Plain. Each section is a bold row on a
band with its own tick (all its parts; half ticked when only some are on) and its percent, plus "panes N of
100". The panes and parts sit indented below it. Under the table, one line per mode adds up only the sections
that count.

- A section box is a percent of the window height. A pane box is a percent of its section.
- Sections that are shown add up to at most 100. Under 100, Status gets the rest (if Status is hidden, the first
  shown section does).
- Over 100 is allowed only with Overall scrollbar on, because the page then scrolls. With it off, Save is
  disabled and one line says why, for example "Sections add up to 112 percent; switch on Overall scrollbar or
  lower a number". The page then draws the numbers scaled to fit.
- Panes inside one section add up to at most 100, and Save is disabled above 100. Under 100, the first shown pane
  gets the rest.
- Panes on one row share one box and one height: Activity and Heatmap, and the first five Status panels
  (`status_row1`).
- Hidden sections and panes do not count. Their box is greyed, with the hover text "hidden in this mode, not
  counted".
- An empty box means "share the rest equally", so a fresh install works with no numbers.
- Go validates the same rules as the page (`internal\sections\pct.go`). Values are stored per mode as
  `sections_pct` and `panes_pct`.
- A fresh install starts with these numbers, one set per mode. A part that is hidden at install keeps its
  number, so it has a size when you tick it. Sections, both modes: Status 15, `sessions_30min` 40,
  `processes_30min` 45, To Do 10 (hidden). The shown sections add up to 100, and so do the shown panes of each
  section. Panes, Standard: `status_row1` 60, `sessions` 40, `burn_30min` 40, `vendor_strip` 20,
  `activity_heatmap` 20, `turn_ticker` 20, `system` 50, `process_groups` 50, `processes` 20 (hidden). Panes,
  Plain: `status_row1` 60, `sessions` 40, `burn_30min` 50, `vendor_strip` 50, `activity_heatmap` 20 (hidden),
  `turn_ticker` 20 (hidden), `system` 100, `process_groups` 50 (hidden), `processes` 20 (hidden).
- From 1600 px wide, Sessions and Processes sit side by side on one row. The row takes the sum of their two
  percents, and Status and To Do keep theirs. The total line then says "wide layout: Sessions and Processes share
  one row of 70".
- No pane draws outside its box. A row its edge would cut is hidden with the rows after it, and one "+N more" line
  takes their place. A scrolling list (Waits, Outcomes, Sessions, Turn ticker, To Do) ends on a whole row at its
  top. The Activity grid scales to its box. A pane never gets less than its header plus one row: each percent box
  shows that minimum on hover and turns amber below it.

## The lamp

### Goal

The lamp tells you when an agent waits on you, so you can leave the BurnMon Dev window closed. It is a
slim ticker at the top edge of the primary screen, with Glim, a small character, hanging under it on a
cord. The ticker shows one row (the headline and Glim's line) and, if you ask for more rows, the items
that wait longest. Glim moves like a cartoon character: he idles, follows your pointer with his eyes and
plays a short act when something changes (`alarm`, `knock`, `stamp`, `welcome`, `wake`, `slip`, `coffee`
and `peek`). When nothing waits he can hide and peek out now and then.

### Where it fits

The lamp is a third window of `burnmon-dev.exe`, beside the main window and the Station. It reads the
snapshot file that BurnMon or BurnMon Dev writes (`docs\snapshot_schema.md`) and nothing else, so one of
the two must run. It has no control: no approve, no stop, no answer to an agent.

### Install

Nothing to install: the lamp is in `burnmon-dev.exe`. The gear dialog has a group Lamp. With
`lamp_enabled` true, BurnMon Dev starts `burnmon-dev.exe lamp` as a child of its own, passes its process
id in the environment variable `BURNMON_LAMP_PARENT`, and stops the lamp when you switch it off or close
BurnMon Dev. The lamp also ends by itself when that process is gone. A lamp you start by hand
(`burnmon-dev.exe lamp`) has no parent and stays until you stop it.

### Use

Hover the ticker or Glim to open it: up to 6 waiting items, tokens a minute and the cost of the last 5
minutes (labelled API price equivalent). It closes 2 seconds after the pointer leaves. Glim sweats
when the burn, in tokens a minute over the last 5 minutes with cache reads counted, is at least 3 times
his usual burn of the last 30 minutes and at least `lamp_burn_floor_tpm`; set that floor with the slider
"Glim sweats from" in the gear dialog. A click on Glim
shows another line; a double click opens BurnMon Dev. The ticker is as wide as its first row needs, from
300 to 440 px; a longer line is cut and shows in full on hover. The opened rows start at the same left
edge and have the same width as the first row. The lamp tries not to take the focus: the window is shown
without activating it, and a window that still takes the focus on its first show hands it back. BurnMon
Dev starts the lamp in a normal, visible window (an earlier build started it hidden, which made it
invisible; a check in `scripts\uicheck-lamp.ps1` now starts it the same way and reads from Windows that
its window is visible within 3 seconds). The focus part comes from the Windows documentation and is built,
not seen.

- **Drag the ticker** (press on the ticker, move more than 6 px): the lamp follows the pointer with Glim
  hanging under it, and the line says "Put me down." On release it snaps to the top edge of the monitor
  under the pointer, at the nearest of left, centre and right, and saves `lamp_monitor` and
  `lamp_position`. A drag never takes the keyboard focus. A saved monitor that is not connected counts as
  the primary one.
- **Grab Glim** (press on Glim, not on the ticker, and move): he swings on his cord toward the pointer, at
  most 60 degrees to either side, and swings back and comes to rest within 3 seconds. A gentle swing gets
  "That tickles." Pulled to the 60 degree stop, or with the pointer changing direction 3 times in 1.5
  seconds, it is too hard: he says "Don't do that." and both arms flail for 1.5 seconds; a second time
  within a minute he says "I said don't." With `lamp_motion` calm or off he does not swing, a press only
  shows the next line. A click without a move still shows the next line.
- **The eyes.** While Glim hangs and `lamp_motion` is lively, his eyes follow the pointer wherever it is,
  on any monitor. The lamp reads the pointer position and the idle seconds and keeps neither: they are
  never stored.
- **Close the lamp.** Right-click the ticker or Glim and pick "Close the lamp". The lamp writes its ledger
  and ends. For a lamp that BurnMon Dev started it also saves `lamp_enabled` false, so BurnMon Dev does not
  start it again. A lamp you started by hand just ends. Proven by a test on this laptop (opt-in, run by
  `.\scripts\uicheck-lamp.ps1`): a lamp started the way BurnMon Dev starts it, asked to close through the
  page's menu call, was gone 180 ms later (the test fails at 2.5 s or more) with `lamp_enabled` false saved, and a lamp whose
  `BURNMON_LAMP_PARENT` process is gone ends within 6 seconds. The cause of the earlier failure: the
  lamp asked its window to quit from another thread, which the window loop never sees; it now asks on the
  window's own thread, and exits after 3 seconds if it still lives. BurnMon Dev also stops a lamp child
  whose saved `lamp_enabled` is false within 2 seconds. Still unseen: the click on the menu item itself
  (the test calls what the click calls), and the Mac panel.

- **See it without any agents.** `burnmon-dev.exe lamp --demo` plays a fixed 48 second loop of made-up
  states and plays every act and every state that lasts once, with the word "demo" on the ticker. It
  reads no snapshot and writes no BurnMon file.
- **The pictures.** `burnmon-dev.exe lamp --poses <folder>` writes the eight poses, a sheet and the demo
  picture at the top of this file (`glim-demo.svg`) into an existing folder; `assets\glim\` holds that
  output. `burnmon-dev.exe lamp --card <folder>` writes the card of the running week as two SVG files
  (add `--demo` for made-up numbers, `--card-window` for a window with a Save PNG button).
- **Clicks through the empty part (Mac).** On a Mac the open lamp lets clicks through outside its visible parts (the ticker, Glim, the menu): a short poll turns the panel's mouse handling off while the pointer is outside them, and keeps it while a mouse button is down. Judged from a screenshot in the Mac check of 2026-11-22 (macOS 27.2.1, Apple silicon, arm64 zip): after a click on an empty corner of the open lamp, the menu bar showed Finder and the window buttons of the app that was in front before were grey. The robot's own check of that row could not decide.
- **A clear window.** With `lamp_backdrop` `clear` the lamp window shows only the ticker and Glim, and
  other windows show through around them; clicks outside the ticker and Glim go to the windows below. This
  is built from the WebView2 and DWM documentation and is built, not seen. If you see a black or stale
  square around Glim, set `lamp_backdrop` to `card`: Glim then hangs in front of a panel in the theme's
  colour.
- **What the lamp keeps on disk.** Two things, both in the data folder. The ledger `lamp-waits.json`:
  per day the seconds agents waited on you, the longest wait, how many distinct items waited and how
  long the lamp was on. It holds no id, title, project or path. And the WebView2 profile folder
  `wv2-lamp`, the browser's own cache for the lamp page. The ledger is the only file the lamp writes
  itself. The demo writes neither: its profile folder is under the temp folder.

### Do this, Glim does that

| You see | Glim is | Because |
|---|---|---|
| A thin dim line (with `lamp_quiet` hide) | off | neither BurnMon nor BurnMon Dev has written a fresh snapshot |
| A thin amber line (with `lamp_quiet` hide) | asleep | no agent runs |
| A thin amber line (with `lamp_quiet` hide) | working | agents run and nothing waits on you |
| Ticker, "N your turn" | your turn | an agent waits for your answer |
| Ticker, "N blocked" | blocked | an agent is stuck and needs a human |
| Ticker with a plank of bricks beside Glim | bored | the longest wait is 10 minutes or more; one brick a minute, at 30 the plank is full and the ticker shows 30+ |
| Ticker, "N tokens a minute" | sweating | the burn is at least 3 times the usual and above the floor |
| Ticker, "N ready to close" | done | something finished in the last 10 minutes |

Acts, one at a time, in this order when several are due: `alarm` (blocked items went up, 2.6 s), `knock`
(your turn items went up, 1.6 s), `stamp` (an item is gone, 2.4 s), `welcome` (you are back after 5
minutes idle and something waits, 2.4 s), `wake` (agents started, 1.6 s), `slip` (the longest wait
reached 15 minutes, once per bored stretch, 6.4 s), `coffee` (10:00 and 15:00 with nothing waiting, 4 s) and `peek` (quiet lamp
that hides and is not off, every 4th minute, 4.4 s).

### Settings

All keys are in `burnmon-dev.json`; the gear dialog group Lamp sets them. A bad value becomes the default.

| Key | Values | Default | What it does |
|---|---|---|---|
| `lamp_enabled` | true, false | false | BurnMon Dev starts and stops the lamp |
| `lamp_position` | top-left, top-center, top-right | top-center | where the ticker hangs |
| `lamp_language` | en, nl | en | the language of the lines |
| `lamp_lines` | true, false | true | one-liners on or off |
| `lamp_foreman_items` | true, false | true | count Foreman items |
| `lamp_burn_floor_tpm` | whole number, 0 or more | 300000 | tokens a minute from which Glim can sweat: all tokens of the last 5 minutes divided by 5, cache reads count. The gear dialog has a slider "Glim sweats from" from 100000 to 1000000 in steps of 50000 with min, middle and max under it, the number beside it as `300k` or `1M`; a saved value outside that range still holds, the slider then stands at its nearest end. 0 switches the floor off |
| `lamp_ticker_rows` | 1 to 4 | 1 | rows of the ticker: the headline, then the items that wait longest |
| `lamp_size` | 100 to 200 | 150 | Glim's size in percent. The gear dialog has a slider in steps of 5 with min, middle and max under it; `small` (120), `medium` (150) and `large` (190) from older files still load, any other value is 150 |
| `lamp_monitor` | a monitor device name, or empty | empty | the monitor the lamp hangs on; empty is the primary monitor. A drag saves it; the gear dialog shows the one in use and a button "Back to the primary screen" |
| `lamp_theme` | system, light, dark | system | colours of ticker, text and cord; the dome keeps its ambers |
| `lamp_motion` | lively, calm, off | lively | calm plays no act and only blinks; off shows still poses (reduced motion counts as off) |
| `lamp_quiet` | hide, stay | hide | when nothing waits Glim hides and peeks, or always hangs |
| `lamp_backdrop` | clear, card | clear | a clear window, or a panel behind Glim |

### Suggest a line

Glim's sentences are in `internal\lamp\lines.go`, in English and Dutch. Send a new one with the issue
form Glim line (`.github\ISSUE_TEMPLATE\glim-line.yml`): pick the mood, write the English line (at most
60 characters, no exclamation mark, no em dash), the Dutch line if you can, and the name for the credit.
A line you send in is MIT and is credited in `internal\lamp\CREDITS.md`. The drawing of Glim
is not part of that: see `LICENSE-ASSETS.md`.

### Next steps and backlog

Limits: it runs on Windows and on macOS (the macOS build needs cgo; BurnMon Dev starts the lamp as its
child on both, and the lamp ends when BurnMon Dev is gone; the Mac check of 2026-11-22 (macOS 27.2.1, Apple silicon, arm64 zip) saw the macOS lamp start as a child of BurnMon Dev, show no white box and open BurnMon Dev on a double-click, and its first row show one sentence twice (N6, open); the
parent check there is read, not run; on macOS without cgo and on Linux the lamp prints "This build has no
lamp window. The lamp runs in the Windows build and in the Mac app (the build with cgo)." and exits); it needs BurnMon or BurnMon Dev running to have a snapshot to read; it has no control (no
approve, no stop, no answer to an agent). Every window, backdrop, animation and position of the lamp is
built, not seen: nothing of it was looked at in a window in the build run.

- What is seen, and what is still unseen. Reported by the owner after the first look on 2026-11-18: the 48 second demo, the clear
  window around Glim, the acts, and a lamp started by hand over a terminal. Everything the fix lane
  changed is built, not seen: the visible start from BurnMon Dev, the slider, the drag to another monitor,
  the grab and swing, the eyes beyond the lamp window, the close menu, the ticker width and the aligned
  opened rows. Measured, not looked at: the child start (a real window found through Windows), the layout
  of the lamp page in the uicheck group `lamp` at 100, 150 and 200 percent, and the height of every pose.
- Next: the first look on Windows (the clear window, click through, the focus on the first show) and a
  look on a Mac.
- Backlog: window styles beyond the ticker; the phone face of Glim (SiteOffice); sounds, outfits, chat and
  approvals are dropped.

## 6. Limits and known issues

- **Station CPU climbs while it is open.** Two clean 30 minute runs did not reproduce the worst earlier ramp, but
  a Station left open costs about 4.5 points of one core more than closed, and CPU follows the page's animation
  rate. Cause not found. Closing it (Esc) removes the cost.
- **Controls were not proven against real sessions in beta.1.** B4 ran none live. B8 ran each control once, on a
  test agent and on throwaway Foreman flows, and saw the agent view list a real background agent. The audit lines
  of a live run are not quoted anywhere.
- **Window level checks.** uicheck `d36` renders Waits on you and Outcomes from the example snapshot, and `d29`, `d33`
  and `d34` cover the layout. None of them was run in B1 to B4 because the apps were open.
- **Both exes share one store.** Running `burnmon.exe` and `burnmon-dev.exe` together makes both ingest the same
  files. Measured safe, but it is double work.
- **Test suite flakes.** `scripts\uicheck.ps1` d19 and d20 are skipped on a single small screen, and w1 and w8 are
  known flaky. None has shown a user-visible bug. d48 at 1600x900 and 2560x1440 waits for external monitors.
- **Microsoft To Do** due dates are tested but were not checked against a live sign-in.
- **Copilot in VS Code** reports every client as "unassigned" until GitHub adds a workspace attribute to its
  telemetry.
- **Cloud Cowork data arrives late.** BurnMon Dev shows `cloud data N min old` next to every Cowork (Cloud) session.
  See `cmd\burnmon\README.md`.

## 7. Next steps and backlog

The Mac test day of 2026-11-16 covers the BurnMon Dev window, the percent settings and the notification on a real
Mac. Findings go into `v0.4.0-beta.5`. Next after that: make Waits on you and the agent view correct and fast
against real sessions, and find the cause of the Station CPU climb. A decision on whether and how to continue past
2026-12-19 is open. Priority order lives in `02_roadmap\roadmap.md`.

## 8. Feedback, licence, contact

Open an issue at https://github.com/siteoffice/burnmon/issues, with the "Bug", "It did not do what I expected" or
"Idea" template. After two weeks, attach a Burn Report if you are willing to share it. Never paste prompts, keys or
customer data. Licence: see `LICENSE`. Security problems: mail info@zerononsense.dev, not a public issue.

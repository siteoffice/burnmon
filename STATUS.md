# BurnMon, status

What is true on 2026-10-21: the repository lives at `github.com/siteoffice/burnmon`
(moved from `wilcodetree`, confirmed by Wilco 2026-10-21). `origin/main` is `14e96bf`
(checked by Wilco with `git ls-remote origin refs/heads/main`), the only branch on GitHub.
Merged today: PR #4 from merkendiep (`ce85753`, macOS and Linux serve the dashboard from a
local web server on 127.0.0.1 with the live functions; Windows keeps no server and no open
port, decision `02_roadmap\2026-10-21_pr4_macos_bridge_decision.md`), PR #5 (`7acd088`,
per-run secret on that server and the README text, park exception granted 2026-10-21) and
PR #6 (`14e96bf`, burnmon-cloud manifest no longer sets `defaultEnabled` false, which had
switched its hooks off in Cowork; plus `siteoffice.json`). No new tag; version constants
unchanged, so a build from `main` carries more than `v0.4.0-alpha.10` and `0.3.2`. The
paragraph below describes 2026-10-12 and is kept as history.

What is true at this commit (2026-10-12): `HEAD` and `origin/main` are `2728264` (checked
2026-10-12 with `git ls-remote origin HEAD`). No new tag: `burnmon-dev.exe` still reports
`0.4.0-alpha.10` and `burnmon.exe` and `burnmon-cli.exe` report `0.3.2` (version constants
unchanged), but a build from `main` carries more than those tags. Station backlog work from
2026-10-03 to 2026-10-11 (see `SESSION_LOG.md`). The live watcher no longer counts a Cowork
session's `audit.jsonl` as a second session (`c8d85d9`, roadmap row 10e); on Wilco's store the
purge removed at least 47.6M tokens from that day's Cowork total. Cowork (Cloud) (`a249bc5`, row
10d): cloud-run Cowork sessions leave no local transcript, so the `burnmon-cloud` plugin in
`tools\burnmon-cloud` delivers a usage-only extract into `.burnmon\cloud` next to the exe, shown
as agent `cowork-cloud`, a lower bound (spec test 4: 0 to 7 percent of the main model's tokens
missed). Project state: parked until 2026-11-01 except the Station, with two park exceptions
granted on 2026-10-12 (the audit fix and Cowork (Cloud)). Open: the "cloud sessions not
measured" counter; `uicheck w1` misses its 1.5 s first-paint budget on this laptop. The text
from here to the next heading describes 2026-10-03 and is kept as history.

What was true at 2026-10-03: **`burnmon-dev.exe` is v0.4.0-alpha.10**
(tag `v0.4.0-alpha.10` at `dd5125c`, 2026-10-02). `HEAD` and `origin/main` are `1b86eec`, two
commits later: Station UI pass steps 1 (`2908574`) and 2 (`1b86eec`), no version bump (checked
2026-10-03 with `git ls-remote origin`). **`burnmon.exe` and `burnmon-cli.exe` still report
`0.3.2`** (version constants in `cmd\burnmon\app.go` and `cmd\burnmon-cli\main.go`), but
share the ingest fixes of alpha.8 and alpha.9 once rebuilt. Project state: parked until
2026-11-01 except the Station feature (decision `C:\ZND\10_holding\03_logs\decisions.md`
2026-10-01 and 2026-10-02); see `C:\ZND\10_holding\01_projects\burnmon.md`. Newest work
is the "BurnMon Dev" section below, newest entry first (alpha.10: Station site theme and
light theme; alpha.9: the Station; alpha.8: Codex live tail). Open: the Station CPU
climb is not reproduced and its cause is not found (2026-10-03, see that section). The text from here to
the next heading describes v0.3.2 and is kept as history.

What was true at v0.3.2 (2026-09-26): **v0.3.2** (WS3, shared ingest performance and
local time everywhere, `02_roadmap\2026-09-26_ws3_shared_ingest_performance.md`). Step 0
profiling on Wilco's real store (54,855 events, ~12,700 Cowork session folders) found the
reported "climbs to 900MB+/13k handles within a minute" symptom was almost entirely one
`fsnotify` watch per subdirectory (13,125 of 13,541 open handles): `internal\watch` now
holds a small Windows-only recursive `ReadDirectoryChangesW` watcher
(`watch_windows.go`, one handle per native root) behind the same `OnChange` interface,
with the old per-directory fsnotify design kept only for B1's untested darwin/linux
build (`watch_other.go`). Fixed, measured for real over 10 minutes: peak RAM 957MB→87MB,
peak handles 13,541→371, both comfortably under the 250MB/2%-avg-CPU targets, alone and
with `burnmon-dev.exe` running alongside (its own numbers are unimproved until WS2
rebases onto this commit, out of this session's scope). The other three hypotheses
(the 400MB memory cap, whole-table `AllEvents` loads, double ingest between the two
exes) were tested and NOT confirmed as drivers of the reported symptom, so none were
changed; full reasoning and numbers: `04_assets\2026-09-26_ws3_profile_before_after.md`.
Separately, every day/week/month boundary ("today", History's buckets, the vendor
strip, the forecast, the export's `--since`/`--until`) now uses local time (Wilco's
decision), not UTC; storage stays UTC. Two genuine bugs found and fixed along the way,
both a bare `.Format(...)` silently rendering whatever Location a time.Time already
carried rather than the calendar day Wilco actually meant: `internal\dataset\fromstore.go`'s
per-session daily bucket (fed the Now page's own Days/Weeks/Months aggregation, wrong by
a UTC offset for early-morning local turns) and `internal\forecast\forecast.go`'s
`dateKey` (wrong specifically for a `WeekStart` round-tripped through SQLite storage,
which comes back UTC-located even though the instant it names is a local midnight).
Both caught by tests, not by inspection alone (`TestBuildOneScoredWeekShowsBand` broke
until `dateKey` was fixed). DST verified against both 2026 Europe/Amsterdam transition
dates (29 March, 25 October) with dedicated tests in `internal/history`,
`internal/store` and `internal/dataset`, each confirmed to actually fail without its
corresponding fix before being trusted.

What was true at v0.3.1 (2026-09-24), a cleanup pass over v0.3.0
(`02_roadmap\2026-09-24_ws1_burnmon_cleanup.md`). Dev is now the only mode: the
Dev/Business header toggle and Monitor mode are both deleted, code and all (not just the
buttons). The Sessions tab now labels every harness by its real name (Claude Code, Codex,
Copilot CLI, Copilot in VS Code, Cowork, Hermes), not "Claude Code" for every vendor that
happens to share Surface "cli"; a session priced by a vendor with no book entry for its
exact model shows "no price" instead of a guessed Claude figure. History's "Burn per
period" chart stacks one segment per harness, fixed colours shared with the Now page,
x-axis labelled by the period's own start date (`yyyy-mm-dd`) on every period. v0.3.0, the
release from `02_roadmap\2026-09-23_v0.3_spec.md`, shipped before it: price books (C1),
per-vendor cost on every basis a book covers (C2), the owner-then-client map with active
time (K1, K2), the per-client view (K3), export and merge (K4, K5), GitHub Copilot in VS
Code as a real adapter (A4), macOS and Linux browser-mode builds (B1), and the Now page
fixes and loading states (U1, U2). Release candidate Done-when and VERIFY pass:
`SESSION_LOG.md`'s v0.3.0 entry, all seven Done-when items pass against the built exe and
the real local store, no fail carried into the tag. v0.2.3 (real-window check harness),
v0.2.2, v0.2.1 and v0.2.0 shipped before that. Specs: `02_roadmap\2026-09-22_v0.2_spec.md`
through `02_roadmap\2026-09-23_v0.3_spec.md`.

## What it is

BurnMon is the successor of claudecost, a vendor-agnostic token and cost monitor for
your own AI coding usage: a portable Windows app (`burnmon.exe`) plus a single-file CLI
(`burnmon-cli.exe`), reading local session transcripts directly, no account or API key.
It is the token-cost slot in ZeroNonsense.dev's Groundwork Kit, per README's own section.

## Adapters

- **Claude** (`internal\adapter\claude`): Claude Code CLI, Cowork/desktop agent-mode
  sessions.
- **Codex** (`internal\adapter\codex`): Codex CLI/Desktop rollout transcripts, native and
  WSL roots.
- **Hermes** (`internal\adapter\hermes`, A1): local SQLite `messages`, WAL, 5-second poll.
  Default macOS/Linux path fixed this session: `~/.hermes/state.db` (or `$HERMES_HOME`),
  confirmed live against Hermes's own docs, replacing V3-5's guessed
  Application Support/XDG paths, which were wrong.
- **Copilot CLI** (`internal\adapter\copilotcli`, A2): reads `session-store.db`'s own
  `assistant_usage_events`, one row per API call while the turn is in progress, so a session
  is live the same way every other adapter's is (corrected here, v0.3.1 cleanup: this line
  used to say "totals read at shutdown, not live", the plan's assumption A2 itself disproved
  and replaced, per the adapter's own doc comment; stale, not code, was wrong).
- **Copilot in VS Code** (`internal\adapter\copilotvsc`, A4, v0.3): tails the OTel file
  the two VS Code settings produce, 5-second poll, last write wins per request. No
  workspace/project attribute exists in the real span data seen so far, so its events
  show client "unassigned" until GitHub adds one.

## Pages

Five tabs, deep-linkable: **Now** (default start page), **History**, **Sessions**,
**Tools**, **About**. English only.

- **Now**: every running session, live, polled every 2 seconds; a smoothed burn chart
  (per-session lines, cost line an opt-in legend toggle); the vendor strip; the turn
  ticker with finding markers; the forecast chart (tokens). Loading states throughout
  (U2): every visual shows "Loading..." until its first real data, never the
  saved-report fallback text while the app's bindings are still being injected.
- **History**: filters (period, range, harness, owner, client once client rules exist);
  a per-client table once client rules exist (tokens, headline cost, active time,
  sessions); "Burn per period" stacks one segment per harness (v0.3.1).
- **Sessions**: sortable/searchable table, a Harness column and filter (v0.3.1: the real
  harness a session ran under, Claude Code/Codex/Copilot CLI/etc., no longer relabelled
  by Surface's Claude-only vocabulary), Findings column, owner and client columns once
  configured.
- **Tools**: `tool_calls` totals as a chart and table.
- **About**: the Claude pricing explainer, plus "what this shows and what it cannot",
  plus the macOS/Linux untested note.

## Store

Versioned additive migrations. Schema version 7 at this commit (migration 7, v0.3 K1,
adds `client` on `events`; the spec's own text calls it "migration 6", stale against
`migrations.go`'s real `Version: 6`, which is the v0.2.1 session_id index). A v0.1 store
opens under this code with no event lost (`TestMigrateRealV01Store`, run against the real
store this session, schema version 7 confirmed, all events retained). `tool_calls` table
fed by the Claude and Codex adapters. `burnmon-cli live` and the app's Now page share the
same windowed query and `SessionTotals` path.

## Cost (C1, C2, C3)

Dated JSON price books built into the binary (`internal\pricing\books`), each with a
`date` and `source`, overridable per model from `burnmon.json`. `CostForEvents` (one Go
function, shared by every page and export) returns every basis a book covers plus the
headline basis per vendor: credits for GitHub Copilot, subscription share once
configured, else API list labelled "upper bound"; a vendor with no book (Hermes) is
"tokens only" everywhere, never a guessed figure. `pricing.Config.EventCost` (v0.3.1) is
the same per-vendor book routing at single-event granularity, used by Sessions' own
per-call/per-model breakdown (`internal\dataset\fromstore.go`'s `buildSession`) and the
Now page's per-session/per-minute cost (`internal\live\live.go`'s `turnCost` and
`ApplySessionTotals`), both of which used to fall through to Claude's family-generic
price table for every non-OpenAI vendor: a Copilot or Hermes call priced as if it were
Claude Sonnet, fixed this session
(`TestSessionsFromEventsPricesGitHubEventsThroughCopilotBook`,
`TestSessionsFromEventsNoPriceForUnbookedVendor`). Side effect, not just the
Copilot/Hermes fix: an anthropic event on these two pages is now also priced from
`AnthropicBook`'s exact model id, the same book History/export already used, rather than
the family-generic table these two call sites used on their own; a real, intended figure
shift (e.g. the family table's Sonnet was $3/$15, the book's `claude-sonnet-5` is
$2/$10), and a Claude model id absent from the book now shows "no price" instead of a
guessed Sonnet figure. Every model id seen live on Wilco's laptop is in the book
(`internal\pricing\books\anthropic_api.json`); an older or unconfirmed id is the risk
case.

## Owner and client map, active time (K1, K2)

`burnmon.json`'s `owners` table now carries optional `client` and `remote` per rule,
alongside the v0.2 `match`/`owner`. Match order per session: a `remote` rule (matched
against the project's git origin, read straight from `.git\config`, no git binary, worktree-
and submodule-aware) first, then a `match` path rule, else owner `"personal"`/client
`"unassigned"`. A v0.2 `burnmon.json` (`owners` only) loads unchanged
(`TestLoadV02ConfigWithOwnersOnlyIsUnchanged`). `burnmon-cli reown` re-applies both to
every stored event. Active time: the sum of gaps between a session's consecutive turns,
any gap over `active_idle_minutes` (default 10) counting zero; per-client active time
sums its sessions'. Spot-checked against three real sessions (V3-2); a proper comparison
against an external time log remains open, see Known gaps.

## Export and merge (K4, K5)

`burnmon-cli export` writes daily rows (day, owner, client, vendor, model, five token
classes, cost on every basis, active minutes); no paths, session ids, prompts or project
names, enforced by a test that scans the output bytes for a path separator or a
session-id pattern. `--owner` is required whenever `owners` rules exist. Verified for
real this session: an export with `--owner ZND` against the real local store held zero
"ClientA" occurrences, zero path separators, zero session-id patterns (35 rows). `burnmon-
cli merge` combines any number of exports (no cap) into `merged.json` and a `report.html`
that opens offline (zero `<script>` tags), one column per label, totals by client, vendor
and week; verified for real by merging two exports of the real store under different
labels.

## Platforms (B1)

`burnmon`/`burnmon-cli` cross-compile clean for darwin and linux, amd64 and arm64,
`CGO_ENABLED=0` (verified this session against the current tree). Off Windows, the app
collects once, writes `dashboard.html`, opens it in the OS default browser, then keeps
re-collecting on `-interval` in the background; no bound-JS live path exists there
(WebView2 is Windows-only), so a page there falls back to the same "only available in
the app window" text a saved report already shows. README, About and `STATUS.md` all say
untested: built and cross-compiled, never run on real macOS or Linux hardware.
`.github\workflows\release.yml` builds and attaches all four binaries to a pushed `v*`
tag; this only runs once the tag is actually pushed, see Known gaps.

## Insight (I1, I2, I3)

`internal\insight`, computed on the fly, no store writes. Four finding kinds:
`re-prefill`, `compaction`, `context-runway`, `expensive-turn`. Shown as markers on the
Now chart, lines in the turn ticker, the spike drawer, and per-session on the Sessions
tab.

## Forecast (F1)

Plan line (last four weeks, weekday-aware) and live line (current rate to end of day/
month), error band once a week is scored, gate text until then. Tokens only.

## Config

`burnmon.json` is the current name; `claudecost.json` in the same two locations is still
read as a fallback. `burnmon.example.json` documents every key including `owners` (with
one commented client/remote example), `copilot_plan` and `copilot_vscode_otel_file`. The
`mode` and `view` keys (v0.3's dev/business and monitor/full start state) are gone as of
v0.3.1: an old config carrying either still loads without error (unknown JSON keys are
ignored), it just no longer does anything.

## Known gaps at this commit

- A `forecast_scores` row written before this session's local-time change (v0.3.2)
  keeps a UTC-Monday `week_start` (post-fix rows are local-Monday, `store
  .scanForecastScore` converts on read either way, but `AddDate` arithmetic on a
  UTC-Monday value still steps in UTC calendar days). Affects at most the one ISO week
  that was unscored at the moment of upgrade; every week scored afterward is unaffected.
  Flagged rather than fixed (a fresh review's own finding, 2026-09-26): a future pass
  could snap an old row via `weekStart(sc.WeekStart)` in `EnsureScored` before scoring it,
  if this ever turns out to matter in practice.
- `store.go`'s event-time bounds (`at >= ?`, `at < ?`) compare RFC3339Nano strings
  lexically. RFC3339Nano drops trailing zero digits in the fractional seconds, and `.`
  sorts before `Z`, so a stored `...T22:00:00.5Z` sorts below a bound of
  `...T22:00:00Z`: an event in the first second of a range can be wrongly excluded at
  the lower bound, or wrongly included at the new upper bound `DailyTokenTotalsUntil`
  added (v0.3.2). Pre-existing pattern (the lower bound predates this session), not
  something v0.3.2 introduced, and `actualTokensForWeek`'s own in-Go `d.Date >= end`
  filter already catches the upper-bound case; flagged rather than fixed (a fresh
  review's own finding, 2026-09-26) since a real fix (binding in a fixed-width format,
  or comparing via SQLite's `julianday()`) touches every timestamp write in the store,
  well beyond WS3's own scope.
- `internal\report\template.html`'s Now chart cost-axis toggle: the right-hand cost axis
  used to stay hidden after the cost series is shown through the chart legend
  (`scripts\uicheck.ps1 w8`). Flagging a discrepancy rather than silently picking one: the
  v0.3.1 cleanup session ran `w8` live against the current build and it passed
  (`axis-when-shown=true`), with no code change to the cost-axis logic itself in this
  session. Left as an open question rather than marked fixed: could be genuinely resolved
  by an unrelated change, or timing-flaky; a future session should re-run `w8` a few times
  before either closing this gap or reopening the Chart.js debugging session it used to
  call for. Ran again this session (WS3, incidental, no cost-axis code touched):
  passed clean a second time; still left open rather than closed, same reasoning.
- `bmHistory` re-reads the whole store (`store.AllEvents()`, ~3s against Wilco's real
  ~55k-row store) on every History tab filter change, not just once: a real UI-latency
  cost, measured but not fixed this session (WS3 hypothesis 3, confirmed real but not a
  driver of the shared-ingest resource climb that session's Done-when targeted; see
  `04_assets\2026-09-26_ws3_profile_before_after.md`). A future pass could move this to a
  SQL aggregate or a daily summary table kept up to date on upsert.
- `burnmon.exe` and `burnmon-dev.exe` share the same `burnmon.db` (both call
  `store.DefaultPath()`/`store.Open()`), so both independently watch, parse and ingest
  the same source files when both run (WS3 hypothesis 4: measured safe, SQLite's WAL
  mode plus `busy_timeout=5000` plus idempotent upsert already cover it, no fix built).
  `burnmon-dev.exe`'s own RAM/handle numbers, previously high from its pre-rebase build
  still carrying the old per-directory watcher, are fixed now that WS2 has rebased onto
  this commit (`02_roadmap\2026-09-26_ws2_performance_patch.md`): peak handles 763-781
  across before/after measurement, no longer the ~13.9k WS2 phase 5 measured.
- WS2's own performance patch (below) could not set WebView2's own memory usage target to
  low while `burnmon-dev.exe` is minimized: `ICoreWebView2Controller4` (the interface that
  carries it) is not vendored in `go-webview2`, and hand-deriving its COM vtable layout
  without the official WebView2 SDK header risks a wrong method-slot offset, a crash risk
  in a tool run daily, for a soft (RAM-only) win. Not built; flagged rather than silently
  dropped. WebView2's own automatic memory management still measured a real drop while
  minimized (peak WebView2 RAM 499MB vs 671MB shown active, peak WebView2 CPU 0.11% vs
  1.36% active), just not as low as an explicit "Low" target might additionally buy.
- The Microsoft To Do panel's own due-date comparison (`internal\todo\todo.go`) is fixed
  as of alpha.3 (WS2 alpha.3 item 5): `localDueDate` now converts Graph's own
  `dueDateTime.timeZone` (documented default "UTC" without a `Prefer: outlook.timezone`
  header, which this package still does not send) to the local calendar date, instead of
  comparing `dueDateTime.DateTime[:10]` straight against local "today". Covered by unit
  tests (UTC, a same-zone passthrough, a 2026-10-25 Europe/Amsterdam DST crossing, an
  unrecognized-zone fallback, nil/short input), not by a live Graph read: no cached
  sign-in token existed on this machine this session (an unattended overnight run cannot
  complete an interactive device-code sign-in), so the actual shape of a real
  `dueDateTime.timeZone` value from this tenant is still unconfirmed. A future session
  with a live, signed-in To Do panel should confirm the real value matches what
  `localDueDate` assumes (falls back to treating an unrecognized zone name as UTC, a
  conservative but unverified choice for that one case).
- GitHub Copilot Business and Copilot Enterprise: their AI-credit allotments are now
  confirmed live (1,900/user/month and 3,900/user/month respectively, pooled at the
  billing entity, `docs.github.com/en/copilot/concepts/billing-and-usage/organizations-
  and-enterprises/billing`, checked 2026-09-24), but the plans page's "for businesses"
  tab still does not render in a plain page fetch, so no per-seat monthly price is
  confirmed for either, and neither is wired into `copilot_credits.json`'s `plans` block
  (their pooled, org-level billing also does not fit `CopilotCreditsLeft`'s
  single-machine model without further design work).
- A ChatGPT/Codex plan-credit table comparable to GitHub's AI Credits does not exist:
  confirmed live 2026-09-24 (`help.openai.com`'s own credits article) that ChatGPT
  personal plans use their own usage limits plus an optional pay-as-you-go credit
  top-up for overage, not a fixed monthly allotment; nothing to build a book from.
- Two backend figures survive the dev/business toggle's removal computed but unused: the
  vendor strip's `copilot_credits_left` (its own display line, `#vs_credits_left`, is now
  permanently hidden) and `live.Session.BusinessCost`/`business_cost` (its only reader,
  `sessionCardBusinessBody`, is deleted). Left in place this session (harmless, not a
  correctness bug, real but small ongoing compute cost every poll); a future pass should
  either remove both or find them a dev-mode use.
- Plan assumption A8 (active time usable): the spot-check from V3-2 stands, but
  `C:\ZND\10_holding\03_logs\time\` still does not exist on this laptop (checked again
  this session), so the comparison against an external time log remains unverified.
- macOS and Linux release artefacts: the workflow and the cross-compilation both verify
  clean, but no artefact has actually landed on a GitHub release yet, since that only
  happens once the `v0.3.0` tag is pushed; this session tags locally and stops before
  pushing, per house rules.
- The Now chart's per-vendor colour families define a lighter "desktop"/"VS Code" shade
  for Codex and Copilot, but Codex is always agent "codex" and Copilot in VS Code sets
  no distinct agent value either, so those two shades stay unreachable.
- `internal\report\template.html`'s About section still describes only Claude's own
  seat/allowance model, not the other vendors.
- History's per-client table (restored this session with the dev/business toggle
  removed, gated on client rules existing rather than on the removed mode) was verified
  by reading the JS logic and the unchanged, still-passing Go tests
  (`TestBuildClientFilterAndRows`), not by a fresh real-window click-through: doing that
  needs a scratch `owners`/`client` config dropped next to the exe and reverted after,
  same as V3-6's own Done-when check 2, and this session did not repeat that live pass.

## BurnMon Dev (`burnmon-dev` merged into `main` at `f7f1c26`; this and later work commits on `main` directly)

2026-10-03, Station UI pass step 2 (bugs; uncommitted, no version bump; roadmap
`02_roadmap\2026-10-03_station_ui_and_backlog.md`). Measurements and method:
`04_assets\2026-10-03_station_cpu_measurements.md`.

- **CPU climb, cause not found.** Two clean 30 minute runs on `HEAD`, five-minute windows of Go
  plus all WebView2 processes: Station open 22, 17, 13, 14, 18, 32 percent of one core; closed 15,
  9, 8, 9, 11, 12. The recorded ramp (24, 45, 64) did not appear. What the data does show: CPU
  follows the page's `requestAnimationFrame` rate (r 0.7 to 0.76 per minute), which jumps to 40 to
  60 a second while tokens flow, because every changing number starts an `animateNumber` tween; the
  open Station costs about 4.5 points more than closed in quiet minutes (12.9 against 8.4); and a
  slow creep of 0.2 to 0.3 points a minute exists open and closed, in the GPU process when closed,
  with `paintTick` growing from 3.3 ms to 8.5 ms as the history rings fill. Go does not climb in
  quiet minutes. Inferred only: `renderHistoryChart` and the heatmaps cause the creep, no bisect.
  Not repeated at 200 percent scaling (the earlier A/B screen). A second Claude session ran 3
  fake agents for 30 minutes: Go 3.2 to 3.5 percent in every window, no climb.
- **d19 and d20:** both pass now, with three monitors attached. They failed on a laptop-only
  screen because `uicheck` caps a window to the monitor (leaving 100 px) and these checks measure
  at 1600x1000, 1920x1080 and 2560x1300 CSS px. A capped window is now reported SKIPPED (d19 PARTIAL,
  d20 SKIPPED) instead of failing; `UICHECK_ORIGIN="x,y"` puts the window on another monitor.
  Measured on the laptop panel (`-3100,-125`): window capped to 1500x900, 4 of 5 d19 cases and
  all of d20 skipped, the 1280x860 case passed. `tools\uicheck` has its first unit tests
  (`capToScreen`, `parseOrigin`).
- **PLAN TABLE "E" (space):** the Test Chamber's top-row consoles at lx 1 and 5 hid it; now at lx 0
  and 6 (both themes, same tiles). `check_themes.js` has a new check (no room name under another
  room's sprite box); it failed before on `desk_computerScreen_SW@21,8`, passes now.
- **Not fixed:** d23 failed once of four runs with "0 agents on site" (fake feed missing) when run
  straight after d19 and d20, then passed alone and twice after d20; cause unknown.

`v0.4.0-alpha.10`: the Station gets a second theme, a construction site (now the default), and
BurnMon Dev gets a public light theme (spec `02_roadmap\2026-10-01_station_secret_screen.md`,
sections "Themes", "Theme engine contract" and "Light and dark").

- **Site theme:** same deck, rooms, stage rules, slots and paths as space; a theme swaps names,
  props, floors, background, sprites and HUD wording only (`THEME_DEF` in `station.js`). Site props
  keep every space prop's tile and blocks flag, so both themes share one blocked grid
  (`tools\station_atlas\preview\check_themes.js` asserts it). Core room: a tower crane whose
  code-drawn jib swings at a speed that follows the smoothed tok/min. The O plan view has its own
  pixel sprites (fences, containers, crane, excavator, scaffolding, cones, workers in hi-vis).
  **T** toggles the theme while the Station is open (lasts until restart); `"station_theme":
  "site" | "space"` in `burnmon-dev.json` sets the startup theme (moved from `burnmon.json`,
  Wilco 2026-10-02: that file is shared with `burnmon.exe`). The space theme renders
  pixel-identical to alpha.9 in both views (headless diff, frozen clock; the only difference is the
  "aboard" to "on board" wording already in the working tree).
- **Site art:** 88 sprites in `station_atlas_site.js` (152,614 bytes), rendered in headless Blender
  with `render_iso.py`'s own camera code (`render_site.py` runs its top half), so `proj` equals the
  space atlas's exactly (`build_atlas.py --site` refuses to build otherwise). All CC0: Kenney City
  Kit (Industrial), Factory Kit, Building Kit, Car Kit, Quaternius "Worker" (poly.pizza), an
  OpenGameArt excavator (author Iacox 2022, not "noway" 2014 as the spec had it). No CC-BY item was
  needed, so no public credit line; `tools\station_atlas\CREDITS.txt` lists every file. Props no kit
  had (fence, barrier, hut, skip, crane mast and others) are procedural in `site_proc.py`. Kits
  download into `tools\station_atlas\kits\` (gitignored). Spec deviation: Blender, not three.js.
- **Light theme (public, README):** **L** toggles dark (default, unchanged) and light. Tokens on
  `:root` plus a `[data-theme="light"]` override; session and harness colours get light shades
  held to 3:1 on white. Remembered in `burnmon-dev-view.json` next to `burnmon-dev.db`, written by
  the app through `bdevSetUITheme` and applied in the markup by `assemblePage`, so no dark flash.
  localStorage was dropped: the page is loaded with NavigateToString (opaque origin). Restart
  checked: L saved `light`, the next start came up light, L again saved `dark`.

Proof. `go vet ./...`, `go test ./... -count=1` and `.\build.ps1` green; `node --check` green on the
page script, `station.js`, both atlases and `wire_reference.js`; `check_themes.js` all pass.
Tests first: `TestLoadDevConfigStationTheme`, `TestUIThemeRoundTrip`, `TestUIThemeClamping`,
`TestAssemblePageUITheme` and the extended `TestAssemblePageSplicesStation` each failed before
their code. uicheck: d0 to d18 and d21 to d24 pass; d23 now also checks T both ways and T ignored
while closed; d24 is new (L to light with every series colour at least 3:1, L back restores the
dark colours exactly, the binding exists). d19 and d20 fail as recorded for alpha.9 on this
single 1600x1000 screen (To Do fit-dropped; process labels 0 px wide). Measurement, 10 + 10
minutes, same exe, state checked every 5 s (no bad samples): closed Go 5.70 and WebView2 4.26
percent of one core (alpha.9's closed runs: 13.1 and 8.0 summed), heap 9.5 MB; open, site theme,
Go 10.98 and WebView2 16.32, heap up to 31.6 MB. Live check with the Station open: this Claude Code
session went Grep to Reading, Thinking after the result, a 70 s PowerShell call to Running,
Thinking, then Waiting; a real Codex task arrived, then showed Waiting after its final reply, but
its two `exec` calls fell inside its Arriving window (Codex sessions first appear about 50 s in,
when its `token_count` lands), so Running was not seen for Codex this time.

**Known, not fixed (found this session, also in alpha.9):** with the Station open, CPU climbs with
uptime. In consecutive 5-minute windows, alpha.9 (built from `HEAD`) used 14.8, 25.7, then 37.1
percent of one core (Go plus WebView2), alpha.10 24.1, 44.6, then 63.9. Go climbs as well as
WebView2. Closed, nothing climbs (Go about 5 percent for 12 minutes, payload 38 to 44 KB). Ruled
out: frame and timer rates (steady, about 8.5 frames a second, one snapshot a second), snapshot
build time and size, canvas paths left open, canvas allocation, theme switches. In one process,
alternating themes every 5 minutes, site and space differ by about 3 points once the trend is
removed, so the theme does not drive it. alpha.9's "+8.7 points open" was a single 10-minute
sample and missed the climb. Wilco: ship alpha.10, find the cause in a follow-up.

`v0.4.0-alpha.9`: the Station, an undocumented secret screen (`02_roadmap\2026-10-01_station_secret_screen.md`,
"What is left" steps 1 to 8). **P** opens a full-window isometric deck where every live session
is an astronaut in the room of its current stage; **O** replaces the whole burn zone (chart to
turn ticker, everything above System; Wilco's call this session, the spec said the chart only)
with a pixel-art plan view; **Esc** closes a turn popup first, then the full Station, then
fullscreen. No README mention. Shared ingest code changed (late tool results), so `burnmon.exe`
(still `0.3.2`) gets that fix once rebuilt.

- **Store:** `LatestToolCalls(keys)`, the newest call per session in one query, ordered by
  `julianday(at)`, not the stored text: RFC3339Nano trims zeros, so `10:00:00Z` sorts after
  `10:00:00.5Z` as text (`TestLatestToolCalls` fails on a text `MAX(at)`, checked). About 5 ms warm
  for the 10 largest real sessions together.
- **Live:** `Session.Stage`, `StageSince`, `StageTool` (`stage`, `stage_since`, `stage_tool`,
  omitempty), filled by `live.ApplyStages`, a sibling of `ApplySessionTotals` that BurnMon Dev calls
  only while the Station is open (`cursor.stages`), so a closed Station adds no store read and
  `burnmon.exe` and the CLI emit no stage fields. Claude keys a tool call by its turn's `requestId`
  (a key match); Codex keys it by a rollout `turn_id` that never equals its event keys and writes a
  response's `token_count` only after that response's tool output (seen in real rollouts), so a
  call whose key matches no windowed turn counts as in the latest turn when it is newer than the
  second-newest turn (`TestApplyStages`, mutation-checked).
- **Real bugs found and fixed:** (1) Codex shell calls are stored as `exec` (93 rows since
  2026-09-25), which `stage.ToolStage` mapped to Coding; now Running (`TestCodexExecRuns`). (2) A
  tool result read in a later incremental pass than its call was dropped, so the call never got a
  result (1,812 of 3,805 Bash rows in the real store had none) and Thinking could never show for a
  slow tool. Both adapters now return a result-only row and `UpsertToolCalls` applies it as an
  update, never an insert (`TestParseLateToolResult` for Claude and Codex,
  `TestUpsertToolCallsResultOnly`). Seen working live: a 25 s Codex sleep got its result recorded.
- **Page:** both scripts embedded (`//go:embed`, spliced at `<!--BM_STATION-->` by `assemblePage`,
  `TestAssemblePageSplicesStation`). Nothing exists until the first P or O; closed or hidden, nothing
  draws or is fed; hidden unmounts, visible remounts. A popup opened from the Station shows on top.
  Fixed after a fresh Opus review: the popup class left behind, a popup hidden behind a newly opened
  Station, heatmaps left at width 0 after O closes, a double mount, a perf-stat skew.
- **Camera (Wilco, this session):** elevation 30 as in the kit, deck turned 15 degrees (azimuth 30,
  left corner closer, long edges about 16 degrees from horizontal). The kit's PNGs exist only at
  azimuth 45, so all 100 sprites are re-rendered from its GLB models in headless Blender 5.2
  (installed this session with winget), calibrated against the kit's own PNGs (mean IoU 0.906 on 20
  samples, edge pieces registered to within 1 px, walls 0.22 to 0.985). The atlas carries the
  camera's projection matrix; `station.js` projects, fits and depth-sorts from it. Pipeline and
  commands: the spec's Art section, `tools\station_atlas\render_iso.py`.
- **Render cost, found and fixed in `station.js`:** the first open measurement cost about +44 points
  of one core in WebView2. Profiled: JavaScript was 4 percent; the cost followed how often an
  animation frame was requested (a 30 fps loop requesting at 60 Hz). Now a timer paces frames at 15
  fps while anything moves and 8 fps idle, static walls and props are baked once per camera, and a
  restart check that spawned duplicate loops is fixed (`!raf` now also checks the timer).

Proof. Final 10-minute samples, same exe, Station state enforced and real key presses counted (none):
closed Go 3.18 and WebView2 4.84 percent of one core, JS heap 9.5 MB; open Go 3.28 and WebView2
13.49, JS heap 9.5 to 28.0 MB. **Open costs +8.7 points of one core against the spec's target of 6;
Wilco accepted that for alpha.9 (2026-10-02), so Done-when item 3 is met for memory (+18.5 MB, under
60) but not for CPU.** Closed against alpha.8: an alpha.8 build from `HEAD` measured 22.4 percent
(Go plus WebView2) over 10 minutes and alpha.9 closed 13.1 and 8.0 in two later runs, both with the
same 9.5 MB heap; run-to-run spread from live session activity is larger than any difference, so
"no worse than alpha.8" is the claim, not "faster". Live rooms check, one stage sample every 2.5 s
against the store's newest call: this Claude Code session went Read and Grep to Reading, Bash to
Running, Thinking 8 s after the Bash result, Write to Coding, an MCP search to Fetching, then
Waiting; a real `codex exec` task went Arriving, `exec` to Running, Thinking, Waiting. Every change
landed within one sample. `go vet ./...` and `go test ./... -count=1` green, `node --check` green
on all three page scripts, `.\build.ps1` green. uicheck: d0 to d18 and d21 to d23 pass (d23 new:
real P, O and Esc presses, both views draw, popup above the Station, Esc order). d19 and d20 fail
on this laptop's current single 1600x1000 screen at 200 percent, and the alpha.8 exe fails them
the same way (checked), so environmental, not this change.

Known, not fixed: an `ApplyStages` error is only logged, and the page then keeps the last stages;
events and tool calls are committed in separate transactions, so a tick can see a new Claude turn
before its call (one tick in the lounge at most, usually hidden by the 3.5 s dwell); Running is
exempt from the stuck rule, so a Codex `exec` waiting for approval reads Running; Thinking's
`stage_since` counts from the call, not from the result; parallel calls with one timestamp pick
arbitrarily; `performance.memory` is coarse (it read a flat 9.5 MB closed in every run). Spec
corrected: the atlas regenerates pixel-identical, not byte-identical (Pillow versions encode PNGs
differently). Committed as `44ba02e`, pushed to `origin` and tagged `v0.4.0-alpha.9` by Wilco
on 2026-10-02 (checked with `git ls-remote origin`: `main` and the tag both at `44ba02e`).

`v0.4.0-alpha.8`: WS2 follow-up, Codex sessions held open never reached the live watcher, plus
the Copilot in VS Code tail (`02_roadmap\2026-09-29_ws2_codex_live_tail.md`, items 0 to 5).
Shared ingest code changed, so `burnmon.exe` (still `0.3.2`) gets the same fixes once rebuilt.

- **Item 0, collapsed Codex turns (data):** current Codex builds (VS Code extension,
  `cli_version` 0.155.0-alpha.16.3) write no top-level `ordinal`, so every turn got the key
  `sessionID:0` and the upsert kept one event per session. Proof before the fix, on
  `rollout-2026-09-29T16-58-26-...5264.jsonl`: 26 `token_count` lines in the file (27 a few
  minutes later, Codex still writing), 1 Codex event in the store for that session. The
  store held 11 such `:0` rows: the two rollouts of 2026-09-29 (whose lines lack `ordinal`)
  and nine `guardian_review` sub-run rollouts of 2026-09-22/23 whose lines carry `ordinal`
  today but were still stored collapsed (for example 2 events for a file with 17 turns);
  how those nine came to be stored as `:0` was not established. Fix: `requestID` keeps the
  ordinal key, else `sessionID:b<line byte offset>`, stable across incremental reads.
  `store.ResetCollapsedCodexSessions` deletes every `:0` Codex row and its file's cursor so
  the next ingest re-reads it from byte 0; `Collect` runs it on every pass (not once behind a
  meta flag, review finding: an unfixed binary on the shared store could collapse again).
  After: 0 `:0` rows, all 11 sessions match their files' `token_count` counts. Codex month
  (local September, vendor-strip token definition): 165,847,499 tokens in 1,492 events
  before, 176,203,422 in 1,655 right after the re-ingest (Codex kept writing, so today's
  part keeps growing). Backup of the store before the re-ingest:
  `%LOCALAPPDATA%\burnmon\burnmon.db.bak-2026-09-29-alpha8`.
- **Item 1:** `FILE_NOTIFY_CHANGE_SIZE` added to the watch mask.
- **Item 2, tail poll (`internal\watch\tail.go`):** every 2 s, each tracked native `.jsonl`
  is opened for attributes and its size read from the handle; a moved size fires the same
  `IngestFile` a notification does. Tracked: files a notification named, files whose cursor
  an ingest moved past a non-zero start (a first or forced full read does not count, review
  finding: it would crowd the cap), and today's and yesterday's Codex rollouts, re-seeded
  every minute so an idle chat never drops out. 60 min window, cap 50, cap hit logged at most
  every 10 min.
- **Item 3, label:** this rollout's `session_meta` does carry `cwd`. The "rollout-" label came
  from the page knowing projects only for running sessions (the collapsed event was 13 min
  old, so not running). `live.Snapshot.ChartProjects` now gives every charted session's
  project; a session with no project at all reads agent plus `#` and the id's last 4.
- **Item 4:** follows from 0 and 2; the vendor strip's 60 s refresh reads the store.
- **Item 5, Copilot in VS Code:** `copilotvsc.Tail` keeps the offset and current session id in
  memory, resets on a path change, a shrink or a replaced file (`os.SameFile`). Poll time on
  the real 46.7 MB file: about 490 ms per poll before (five runs, 474 to 515 ms), after 478 ms
  once at start and then 0 to 24 ms per poll. Vendor strip "Copilot (VS Code)" TODAY: the
  strip's SQL gives 230,585 tokens in 16 requests, equal to the file's own today total. The 0
  Wilco saw at 18:36 was the strip's first build at the 18:35:44 restart, which ran before the
  Copilot poll's first tick; the next 60 s refresh counts them.

Proof. Real run 19:16:28 to 19:31:26 (15 min), alpha.8 running, a Codex chat in VS Code kept
open by Codex itself, no file touched (both rollouts still showed LastWriteTime 16:58:26 and
18:28:31 afterwards while they grew by megabytes): 13 turns, every one in the store within
the same once-a-second sampling pass (the sampler's loop took about 2 s per pass), 0 of 445
samples with the store behind the file, including the first turn after a 12 min idle gap.
The burn chart reads the store every 1 s tick, so "in the chart within 5 s" follows from
this; it was not screenshotted turn by turn. Vendor strip Codex TODAY (its own SQL) 12,896,168
tokens in 161 events, equal to the sum of `last_token_usage.total_tokens` over today's 161
`token_count` lines. Codex month after the run: 179,584,148 tokens in 1,681 events. Unit
tests: `TestParseNoOrdinalKeysByByteOffset`, `TestResetCollapsedCodexSessions`,
`TestTail_HeldOpenFileAppends` (file held open by the test while it appends, no directory
watch on it, so only the tail can pass it), `TestTail_TickerSeesHeldOpenAppend`,
`TestTail_CapKeepsMostRecent`, `TestTail_IncrementalMatchesFullRead` (mutation-checked: fails
when the session id is not carried), `TestTail_ResetsWhenFileShrinksOrIsReplaced`,
`TestBuildSnapshot_ChartProjectsCoversStoppedSession`. `go vet ./...` and `go test ./...`
green; `uicheck d0`-`d22` green, with `d11` (tick cadence) failing twice right after the
build (two ticks 153 and 174 ms apart, while the startup backfill took 26 s and `AllEvents`
6.6 s against 0.3 s an hour earlier, fresh exes being scanned) and passing on the third run.
A fresh read-only Opus review before handover: no blocking bug; its three medium findings
(one-shot reset gate, idle chat expiring from the tail, cap crowded by a full re-read) are
fixed as described above. Known, not fixed: a notification and the tail can ingest the same
append twice at once (harmless, the upsert is idempotent; seen in the log as two identical
"2244 bytes read" lines); the reset can in theory race a live ingest of the same file in the
first seconds after start (milliseconds window); a Codex rewrite of an existing rollout would
shift byte-offset keys. Not committed, pushed or tagged.

`v0.4.0-alpha.7`: WS2 smooth tick, System gaps, System and To Do layout, harness labels,
System boxes grid (`02_roadmap\2026-09-29_ws2_smooth_tick_and_system_layout.md`, items 1 to 5).
Measured on the real store, before and after, with a per-minute perf log (`cmd\burnmon-dev\perf.go`).

- **Freeze, cause 1 (payload growth):** `bdevSnapshotNow` returned the whole 30 min system
  history and 60 min process-group history every second, and go-webview2 hands every result
  back as one `ExecuteScript` of that size. Before (62 min): JSON grew to 4.38 MB, round trip
  p50 148 ms (worst minute p95 352 ms, max 466 ms), WebView2 renderer 2,970 MB and JS heap
  2,356 MB at minute 62, machine down to 3.5 GB free. Snapshot build itself stayed 1 to 7 ms,
  so no burn cache was added (spec item 1.2's "if yes" did not hold). Fix: history as deltas
  (`history_delta.go`), the page keeps its own rings, full window only on a first call, a
  reload or a stale cursor; `pruneFront` copies once the dropped prefix outgrows what is kept.
- **Freeze, cause 2 (lost answer), found in the first after run:** go-webview2's `Dispatch`
  wakes the UI thread with one `PostThreadMessage(WM_APP)`; a Win32 modal loop (window drag,
  resize, menu) drops it, and the queued resolve waited until another `Dispatch` got through:
  67 s once in the 90 min run, reproduced on demand with a modal move loop (36 s). Before
  alpha.7 nothing else dispatched, so the page froze for good, which matches Wilco's
  2026-09-28 freeze (Go kept logging, header clock stuck). Fix: a 1 s no-op `Dispatch`
  heartbeat (`main.go`) plus a page watchdog that abandons a call in flight for 2 ticks (at
  most one abandoned call outstanding, answers applied in call order), plus the spec's
  never-skip-forever paint rule.
- **System chart gaps (item 2):** the persisted samples of 2026-09-29 10:01 to 10:13 show the
  process awake (the persist loop kept writing the same stale process-group rows, 68 rows for
  one timestamp) while the sample loop produced nothing for up to 2 min 56 s: not sleep. The
  only slow pass logged was inside `sampler.Tick`; its phase log then named `netsh wlan show
  interfaces` (2.1 s and 2.65 s, drives 2 ms). Fix: the drive and wifi refresh runs in its own
  goroutine (`internal\sysmon\sample_windows.go` `runSlowRefresh`), logged when slow or when
  it keeps running. After: 0 slow passes and 0 gaps over 25 s in 182 min of runs. Not
  reproduced: the 3 minute stall itself did not recur, so the exact blocking call behind it
  is inferred (netsh or `disk.Usage`, both now off the sample path), not observed.
- **Layout (items 3 to 5):** with To Do visible the System chart takes 70% of its plain height
  and To Do fills the bottom (ratio 0.700 to 0.701 at 1600x1000, 1920x1080, 2560x1300; at
  1280x860 the existing drop order still drops To Do first, unchanged). Harness heatmap labels
  150 px from one constant, every label in full. Memory, Disks and Network on one 18 px row
  pitch, three network bars, matching Wilco's mock-up.
- **Also fixed:** `internal\watch\watch_windows.go` Close raced its own reader (`draining.Add`
  after unlocking, an intermittent "negative WaitGroup counter" panic in the package tests);
  go-webview2 reads `DataPath` from a Go buffer it does not keep alive, so some launches made a
  WebView2 profile folder in the working directory named after heap text (six such folders in
  the repo root today, recycled): both apps now set `WEBVIEW2_USER_DATA_FOLDER`.

Proof, spec targets: p95 round trip under 250 ms, met in both after runs (first after run:
worst minute p95 99 ms, median 10 ms; verification run: worst 23 ms). No dropped-paint streak
over 2 ticks: missed in the first after run (67 ticks, cause 2), met in the 92 min
verification run (max 2, during a forced 5 s modal loop; 0 otherwise). Flat WebView2 working
set in the second hour: missed in the first after run (128 to 207 MB), met in the verification
run (223 MB at minute 62, 223 MB at minute 92, range 223 to 230). The verification run used
the build before the review fixes, the `WEBVIEW2_USER_DATA_FOLDER` line and item 5; the final
exe is covered by `go vet ./...`, `go test ./... -count=1` and `uicheck d0`-`d22` (d18 delta
equals full, d19 To Do layout, d20 labels, d21 unanswered call, d22 box grid), not by a
further long run. Screenshots: `04_assets\reference\2026-09-29_smooth_tick\`. Not committed,
pushed or tagged.

`v0.4.0-alpha.6`: bug fix (2026-09-28, same day as alpha.4/alpha.5), reported live by Wilco - a
Cowork session's burn-chart segment, legend dot and session card all drew HARNESS_HUE.other's
grey while the vendor strip and the legend's own text both correctly read Cowork. Root cause:
`page.html`'s `sessionColor(sid, agent)` caches a session's colour once, forever, by design
("stable for life"); the burn chart's own `by_session` rendering loop can call it for a sid
whose vendor `agentBySessionMap` does not know yet (its last turn aged out of the running window,
and/or its own turns aged out of `Turns`' 50-turn ticker cap, while `buildChart` - uncapped -
still drew its bars), so the very first call locked in `HARNESS_HUE.other` under that sid
forever, even once the real vendor resolved on a later tick. Fixed both ends: `assignSessionColor`
now returns without touching `sessionColorMap`/`vendorShadeOwner` at all while `AGENT_COLOR[agent]`
is not yet a known vendor (retried every tick until it is, then coloured and cached for life as
originally intended), and `internal/live`'s new `buildChartAgents`/`Snapshot.ChartAgents` gives
`agentBySessionMap` an uncapped, always-current session-to-agent map (the same events/window/
`isTurn` filter `buildChart` itself uses) instead of relying on the ticker-capped `Turns` alone
for a session gone from `Sessions`. New `TestBuildSnapshot_ChartAgentsCoversSessionBeyondTurnCap`
(`internal/live`) reproduces the cap gap directly (a busy 60-turn session crowds a quiet session's
one old turn out of both `Sessions` and `Turns`, `ChartAgents` still has it). `check_d15.go`
extended: a second fake-mode paint proves a session whose tokens land in the chart bucket one or
more ticks before its vendor is known recolours to a real vendor shade (chart == card == legend
dot) once it arrives, rather than staying locked grey; the hue-vs-vendor-strip tolerance check
gained a saturation check alongside it, since `HARNESS_HUE.other`'s own hue sits only ~1.6deg
from Cowork's, so hue alone would not have caught a regression back to this exact bug for a
Cowork session. A fresh, independent review (Claude Opus 5.5, read-only, before commit) confirmed
the diagnosis and fix, found no remaining path that caches or reserves a colour before the vendor
is known, and one Important finding fixed here: the extended `d15` check reused a fixed
`fake-late-1` session id, so a second run against the same already-running window (uicheck
attaches rather than relaunching) found it already cached from the first run and false-failed
the regression guard - fixed with a per-run unique id (`fake-late-<UnixNano>`); confirmed by
running `d15 d15` back to back against one window, both clean. Two minor hardenings from the same
review: `renderBars`' sort comparator now treats an unranked session (mid-way through resolving
its vendor) as sorting last instead of computing `NaN` from an `undefined` rank subtraction; a
stale cross-reference in `live.go`'s own new comment corrected. `go vet ./...`, `go test
./... -count=1`, `.\build.ps1`, `node --check` on the page JS, and `uicheck d0`-`d17` all green
(one `d7` failure on the full sweep, confirmed transient by an immediate solo re-run - the same
desktop-contention pattern the alpha.2/alpha.3 entries above already document, not a code
regression). Not pushed, tagged or merged.

`v0.4.0-alpha.5`: WS2 follow-up fix (2026-09-28, same day as alpha.4) - alpha.4's own gap-break
threshold (a hard-coded 5s in `page.html`) hid every minimized stretch on the System chart and
the process-groups sparklines, because hidden sampling runs at `app.go`'s own
`hiddenSampleInterval` (10s), well over 5s. The threshold is now `histGapThreshold` (`app.go`,
2.5x `hiddenSampleInterval`, 25s today), sent to the page as `snapshotPayload.HistGapMs` and
read by `paintTick` into `HIST_GAP_MS` instead of a second hard-coded number in `page.html`; a
minimized stretch draws as a coarser line, sleep or a closed app still breaks it. New
`check_d17.go` proves the System chart stays one continuous line across a 5-minutes-at-1s plus
5-minutes-at-10s stretch and breaks exactly once at a real 60s gap, and pins the sparkline's own
25000ms boundary directly (24s holds, 26s breaks - a 60s gap cannot fit both its endpoints
inside the sparkline's 30-second window at once, so this case cannot replay the System chart's
literal 60s number). Full writeup in `SESSION_LOG.md`.

`v0.4.0-alpha.4`: WS2 follow-up (2026-09-28, same session as the month-labels,
vendor-colours and time-axis specs in `02_roadmap`) - the activity heatmap's month labels
show their full three-letter text again (`.heatmonth` is now position:absolute,
content-sized, instead of alpha.3's own fix which happened to pass `check_d9` by clipping
"Mar" down to "Ma"; new `check_d14.go` proves no rendered label is clipped); every
`tools\uicheck` run now forces the Microsoft To Do panel off (`todoEnabledForRun`,
`BURNMON_DEV_UICHECK`), so a live signed-in session never gets screenshotted or read by an
automated sweep; the burn chart, legend, session cards, vendor strip, process groups and
harness heatmap now draw every session/vendor colour from one source (`HARNESS_HUE`) -
several open sessions of one vendor get a lightness shade of that vendor's own hue instead
of an unrelated colour, Copilot CLI's hue moved off a near-duplicate of Cowork's, and two
legend entries with identical text get a session id appended (`check_d15.go`, fake
sessions only); the System chart's x-axis is now real elapsed time (`renderHistoryChart`,
`#histAxis`), not sample index, sharing its window and 5-minute labels with the burn chart
above it and breaking the line on a >5s gap instead of drawing across it - the
process-groups sparklines got the same time-based/gap-break treatment since they were
index-based too (`check_d16.go`, fake samples only). Full writeup in `SESSION_LOG.md`.

`v0.4.0-alpha.3`: WS2 alpha.3 (`02_roadmap\2026-09-27_ws2_alpha3_system_cadence_todo_scroll.md`,
`_bundle_and_overnight_rules.md`) - the To Do panel now scrolls vertically instead of
squeezing rows unreadable (its own second exception to the no-scroll patch, alongside the
turn ticker); all 20 core bars share one fixed track length (left/right label slots sized
in `ch`, `check_d13.go` proves min==max live); the whole System zone (CPU total, the 20
core bars, the main chart, process groups and their sparklines, Memory/Disks/Network) now
samples together on one fixed 1s cadence instead of the process walk lagging on its own
3s clock (`check_d11.go` extended, proved live: sysmon and processGroups both changed 30
times in the same 30s window); the process snapshot buffer is now reused across walks
instead of a fresh 2MB allocation every tick (cheap headroom for the 3s-to-1s cadence
change); the Microsoft To Do panel's own due-date comparison now converts Graph's
`dueDateTime.timeZone` before comparing dates, not a bare UTC-string substring (no live
Graph read was available this session, see Known gaps); `.heatmonth`'s own 1920x1080 `d9`
failure (flagged, not fixed, in the alpha.2 entry below) is fixed by cropping instead of
overflowing. Full writeup below the alpha.2 entry it builds on.

`v0.4.0-alpha.2`: a second, developer-facing window (`cmd\burnmon-dev`, `burnmon-dev.exe`)
showing token/cost burn and system load on one time axis, following
`02_roadmap\2026-09-24_ws2_burnmon_dev.md`'s phases 0 through 4, three UI patches
(`2026-09-25_ws2_ui_review_patch.md`, `_burn_chart_no_scroll_patch.md`,
`_ticker_headline_patch.md`), phase 5 (verify and release), and its own
performance patch (`02_roadmap\2026-09-26_ws2_performance_patch.md`). Reuses
`burnmon.exe`'s own `internal\` packages and store; system samples go to a separate
`burnmon-dev.db`. See README's own "BurnMon Dev" section for what it does and how to run
it; `04_assets\2026-09-24_burnmon_dev_design.md` for the design.

Phase 5 fixes this session: the headline total now adds tokens ingested since
`vendor_strip`'s own 60s cache refresh on every 2s tick (`cmd\burnmon-dev\headline.go`),
instead of only moving once a minute; `tools\uicheck`'s window sizing now reads
`GetDpiForWindow`/`AdjustWindowRectExForDpi` so a requested viewport size means real CSS
pixels regardless of display scaling, capped to the screen with a log line when it does
not fit; the process-groups panel's CPU percent is now normalized against the whole
machine (divided by core count client-side) instead of gopsutil's own unnormalized
per-process convention (100% meant one full core), which is what every other "%" on this
page already means; sampling now runs on three independent cadences instead of one
(paint/cheap system sample on `refresh_ms`, the process walk fixed at 3s, persistence to
`burnmon-dev.db` fixed at 10s wall clock), so a faster paint refresh no longer multiplies
process-scan or disk-write cost.

Rebased onto `main`'s `v0.3.2` (`69a071e`, WS3's shared-ingest watcher fix and local time
everywhere) this session; one conflict, in `SESSION_LOG.md` (both branches prepend entries
to the same file, four separate conflict points across the rebase), resolved by keeping
every entry in newest-on-top order by real commit timestamp, no other file conflicted.
The shared-ingest climb WS2 phase 5 flagged as out of scope is gone now that this
rebase brings in WS3's fix: post-rebase, pre-anything-else 10-minute baseline (Go process)
0.89% avg / 1.95% peak CPU, 252.5 MB peak RAM, 763 peak handles.

This session's own performance patch (items 1-6,
`02_roadmap\2026-09-26_ws2_performance_patch.md`): a single `NtQuerySystemInformation`
system-wide snapshot per process-walk tick replaces the old one-gopsutil-call-per-process
sampler (`internal\sysmon\process_windows.go`, new; `process_other.go` keeps the old
gopsutil path for the untested darwin/linux build), caching each process's command line
once per pid rather than every tick; the window pauses painting and slows its own system
sample to 10s while minimized (`document.visibilitychange`, which WebView2 already ties to
the host window's own minimize state, plus a new `hidden` flag app.go's sampler reads),
restoring with one immediate fresh tick on un-hide; the burn chart, session cards, ticker
and process-groups panels now update existing DOM nodes/rows in place instead of
rebuilding their whole HTML every tick (vendor strip already did this); the process-groups
"BurnMon Dev" row is split into "BurnMon Dev (Go)" and "BurnMon Dev (WebView2)"
(`internal\sysmon\harness.go`'s new `HarnessSelfWebview`, keyed off a webview2 process's
own parent-chain classification), and its CPU header now states the percent is of the
whole machine, matching the header/CPU box's own convention; `headline.go`'s
`headlineDayStart` (missed when vendorstrip's own switched, WS3) plus `app.go`'s
`closedDayRows` (the activity heatmap), `main.go`'s `bdevCacheBreakdown` (the vendor
strip's own click-through detail) and `export_run.go`'s `parseExportTime`
(`--since`/`--until` bare-day parsing, `endOfDay`'s `AddDate`) now use local time, along
with the activity heatmap's own JS-side grid math (`page.html`'s `mondayOnOrBefore`/
`renderHeatmap`, `toISOString()` replaced with a local `fmtLocalDateKey`); every `.UTC()`
call in `cmd\burnmon-dev` is now gone (two were removed - the heatmap and vendor-strip
breakdown day boundaries - beyond headline.go's own single fix the spec named directly;
`internal\devexport`'s own `.UTC()` calls on the exported bundle's `GeneratedAt`/`Since`/
`Until`/event `At` are kept, deliberately, matching WS3's own established convention that
a portable, machine-readable export stays UTC while the page's own display converts to
local at render time). Watched 30s during this session's own active Claude Code session:
9 headline changes out of 30 render ticks (measured span 29.0s).

Post-patch 10-minute measurement (active, `burnmon.exe` co-running): Go process 0.28% avg
/ 0.59% peak CPU (was 0.89%/1.95%), 195.4 MB peak RAM (was 252.5 MB, under the 250 MB
target), 781 peak handles (was 763, essentially unchanged - item 1 is a CPU fix, not a
handle-count one); WebView2 tree 0.51% avg / 1.36% peak CPU (was 0.71%/1.58%), 671.1 MB
peak RAM (was 680.9 MB), 6 processes peak, both runs. Minimized for 5 minutes: Go process
0.15% avg / 0.80% peak CPU, 297.2 MB peak RAM (handles 734); WebView2 tree 0.03% avg /
0.11% peak CPU, 499.0 MB peak RAM - CPU on both drops sharply versus the active run,
confirming the pause actually holds.

A fresh, independent Opus review over the full diff before committing found five real
issues, all fixed with a failing test (or a `node` repro for the JS-only one) confirmed
first: the heatmap's own week count silently dropped a whole week - including today's own
column - on any Monday whose 182-day lookback crossed a DST transition (a raw ms/604800000
division, now day-counted first); the process-groups crop's "-1 row" headroom guess hid
one row that actually fit whenever the real thead was shorter than a data row, now
measured against the thead's real height; `internal\advisor`'s `evalSelfOverhead` still
only read `HarnessSelf` after item 4's split, silently missing WebView2's own (larger)
share of this app's overhead, now sums both; `internal\devexport`'s `dailyRows` bucketed
by `t.At`'s own UTC calendar day even though `--since`/`--until` now select a local window
(the same class of bug item 5 was meant to close, in a file the spec did not name
directly), now buckets by local day, and summary.md's top-turns table gets the same local
-time fix; the process-registry cache (item 1) had no way to notice a pid reused by a
different process within one 3s walk, now guarded by the snapshot's own `CreateTime`. The
hand-derived `systemProcessInfoT` struct layout, the harness-split ordering, the DOM-reuse
reconciliation and the `hidden` atomic.Bool concurrency were all independently checked and
found correct. Not addressed: the Microsoft To Do panel's own due-date comparison against
Microsoft Graph's `dueDateTime` (named under item 5's "To Do"; correctness depends on an
undocumented Graph timezone behaviour this sandbox cannot verify live, so left unfixed
rather than guessed at) - a future session with a live, signed-in To Do panel should
confirm whether a due date set in local time reads correctly.

Full verify pass green after the review's own fixes: `go vet ./...`, `go test
./... -count=1`, `.\build.ps1`, `node --check` on both page JS files, `uicheck d0`-`d12`
re-run in full (one `d9` failure at 1920x1080 only, confirmed unrelated: a pre-existing
`.heatmonth{overflow:visible}` CSS from 2026-09-24 that `check_d9.go`'s own sweep
misreads as a scrollbar risk, today is not a Monday so the DST fix could not have changed
this day's rendering, and `d9` passed clean at the other four sizes and on an earlier
pass at a different display scale). `w0`/`w2`-`w8` against `burnmon.exe` all green; `w1`
(a fixed 1500ms WebView2 warm-up budget, unrelated to anything in this diff) passed once
earlier in the session but could not get a clean run against the final build - real
desktop contention (a screenshot from one attempt captured an entirely different,
unrelated foreground window, confirmed via `GetForegroundWindow`), the same category of
limitation `d7`'s own comment already documents for a locked screen, not a code
regression. Wilco should re-run `w1` alone once the desktop is idle. Not pushed, tagged or
merged; see the hub brief and the commands handed to Wilco for the exact next steps. Its
own `d9`/1920x1080 finding (a pre-existing `.heatmonth{overflow:visible}` CSS spilling
month labels wide enough to trip the scrollbar sweep at that one viewport) is fixed as of
alpha.3 below, by cropping the layout rather than the check.

`v0.4.0-alpha.3` (this session, `02_roadmap\2026-09-27_ws2_alpha3_*.md`), committed directly
on `main` (the branch was merged at `f7f1c26` before this session started): the To Do panel
scrolls vertically instead of cropping (`.todobody` `overflow-y:auto`, `.todorow`
`flex:0 0 auto`, `renderTodoTasks` no longer calls `cropToFit`; `check_d9.go` excludes
`#todoBody` alongside `#turnTicker`); all 20 core bars share one fixed track length
(`.corelabel-l`/`.corelabel-r`, fixed `ch`-based slots either side of `.corebar`, value
right-aligned; new `check_d13.go` measured all 20 bars live at 195px each, min==max); the
whole System zone now samples on one merged 1s ticker (`app.go`'s `startSampling`, replacing
the old `refreshInterval`-ticked system sample plus a separate `processWalkInterval = 3s`
process walk) instead of the process-groups panel lagging the rest on its own slower clock -
`check_d11.go` extended with a `window.__bdevChangeCounts` proof, ran clean live: sysmon and
processGroups both changed 30 times in the same 30s window. A fresh review (below) found
the main chart and the process-groups sparklines/harness heatmap still moved on a 10s beat
regardless, since `SysmonHistory`/`ProcessGroupsHistory` were still served from
`burnmon-dev.db` (only gains a row every 10s `persistInterval`); fixed with two in-memory
ring buffers (`app.go`'s `sysHistBuf`/`groupsHistBuf`, appended every 1s tick, pruned to a
60-minute `sysHistWindow`), `persistInterval` and the store's own write volume unchanged.
`internal\sysmon\process_windows.go`'s
`querySystemProcesses` now reuses its own snapshot buffer across walks (`ProcessSampler.buf`)
instead of a fresh 2MB allocation every tick, cheap headroom for the 3s-to-1s cadence change
(`TestProcessSampler_Tick_ReusesSnapshotBuffer` checks the backing array survives two ticks);
`.heatmonth` changed `overflow:visible` to `overflow:hidden` (crops instead of spilling, same
convention as every other narrow label on this page), fixing the `d9`/1920x1080 finding above
without touching `check_d9.go` itself; `internal\todo\todo.go`'s due-date comparison is fixed
per the Known gaps entry above. Cost guard held: Go process measured 0.21 percent avg / 0.37
percent peak CPU (was 0.28/0.59 at alpha.2), 111.5 MB avg / 113.5 MB peak RAM (was 195.4 MB
peak) over a 10-minute active run with `burnmon.exe` co-running - well under the 2
percent/250MB targets, and no worse than alpha.2 despite the process walk now running three
times as often, matching item 3's own prediction that the cost is dominated by one
`NtQuerySystemInformation` snapshot, not per-tick frequency. WebView2 tree: 0.45 percent avg
/ 1.26 percent peak CPU, 489.6 MB avg / 514.3 MB peak RAM, 6 processes peak (was 0.51/1.36,
671.1 MB peak). A first minimized attempt was interrupted at 20s (the tracked process
exited and was replaced by a fresh, differently-PID'd instance at the same moment the
desktop showed a foreign foreground window - a real desktop-session interruption, per the
overnight rules stopped rather than retried at that point in the session); a later,
review-fixed-build re-run completed the full 5 minutes clean, from an already-settled
process: Go process 0.06 percent avg / 0.19 percent peak CPU, 111.5 MB avg / 113.5 MB peak
RAM; WebView2 tree 0.00 percent avg / 0.04 percent peak CPU, 384.2 MB avg / 387.0 MB peak
RAM - both comfortably under target, confirming item 4's buffer reuse holds under the new
1s cadence with no growth while minimized. (A shorter spot-check taken immediately after a
fresh launch, before startup's own one-time backfill had settled, briefly showed RAM near
263 MB in both the active and minimized phases; a live re-check afterward found it back
down to 109 MB, confirming that figure was the known one-time startup cost, not sustained
growth - the settled numbers above are the ones that matter.) `go vet ./...`, `go test
./... -count=1` (every package, including the new tests above), `.\build.ps1`, `node
--check` on both `cmd\burnmon-dev\page.html` and `internal\report\template.html`'s
extracted script blocks, and `uicheck d0`-`d13` (the full five-size sweep, all clean
including `d9` at 1920x1080 and the new `d13`) all pass. `w0`, `w2`-`w8` pass against
`burnmon.exe`; `w1` (solo re-run) failed once, confirmed via its own saved screenshot to be
the same environmental-contention pattern the alpha.2 entry above already documents (the
screenshot showed an unrelated, actively-used Cowork chat window in the foreground, not
`burnmon.exe` - real desktop contention this session could observe directly, not a code
regression), not retried further given that live evidence. A fresh, independent Opus review
ran read-only over the full diff
before commit and found the chart/sparkline history gap above (fixed, not just reworded)
plus a weak DST test (`TestLocalDueDate_DST` originally tested an instant before the
actual transition; replaced with one after it that a lingering-CEST bug would land on the
wrong calendar day for) and a `check_d11.go` comment that overclaimed "value changes"
where it actually counts a new sample landing (corrected, the assertion itself was
already valid); the merged sampling goroutine's locking, the process-snapshot buffer's
aliasing safety, the CSS sizing and `check_d13`'s own logic all came back clean. Not
pushed, tagged,
merged, rebased or reset, per the overnight rules; committed directly on `main`.

## Next

Decision 2026-12-19: continue to a paid team line, keep free, or stop. Out of scope,
named in the v0.3 spec's section 3: a local OTLP receiver, a merge cap and paid team
line, the BurnRate time-log join, a native window off Windows, signed builds,
project-file tags, a team server. Plan: `02_roadmap\roadmap.md`.

# Layout, shared snapshot and kept history, spec

Status: decided, 2026-10-25. Park exception: Wilco, 2026-10-25, "all now". Builds in one Foreman
run together with R3, R5, R9 and R10 (scope: `C:\ZND\10_holding\02_roadmap\2026-10-22_agent_control_plan.md`
section 2). Builds on `main` at `00e042a` or later, where R6, R8 and R7 are merged (checked
2026-10-25 with `git ls-remote origin refs/heads/main`). Sprint specs: B1 (parts 1 and 2) and B2
(parts 3 and 4) in `C:\ZND\10_holding\05_docs\superpowers\specs\`.

## 1. Either exe writes the snapshot

Problem (measured in code, 2026-10-25): burnmon.exe and burnmon-dev.exe each scan and watch the
sources and write the same store (`%LOCALAPPDATA%\burnmon\burnmon.db`), but only burnmon.exe writes
`%USERPROFILE%\.burnmon\snapshot.json` (`cmd\burnmon\snapshot.go`). Without burnmon.exe, BurnMon Dev's
Status sections and live agent map read "not running".

Decision (Wilco, 2026-10-25): whichever exe runs writes the snapshot, guarded by a lock file next to
it, so one writer at a time. When the writer exits, the other takes over within a few seconds.

- The snapshot builder moves from `cmd\burnmon` into a shared package; both exes call it.
- The lock: a file in `%USERPROFILE%\.burnmon`, held open by the writer; a stale lock (writer pid
  gone) is taken over. No port, no network.
- Done when: with only burnmon-dev.exe running, the Status sections fill; closing burnmon.exe while
  both run hands the writer role over without a gap longer than two ticks.

## 2. Keep history when a transcript is deleted

Problem (measured in code): every full scan calls `DeleteEventsForOtherPaths`
(`internal\dataset\dataset.go:718`), which deletes the events of each transcript no longer on disk.
Vendors clean up old transcripts, so history and the Month column shrink.

Decision (Wilco, 2026-10-25): setting `keep_history`, on by default, in burnmon.json and in the
settings menu of both apps.

- On: a vanished transcript keeps its events; only its cursor goes. Off: today's behaviour.
- Delete history: older than N days, one session, or all; each asks for confirmation.
- Export history: a button in both apps on the existing `burnmon-cli export` code.
- The settings text says that deleting a transcript does not delete BurnMon's copy while
  `keep_history` is on.
- Done when: a test store keeps a deleted transcript's events with the setting on and drops them
  with it off; delete and export work from both apps.

## 3. BurnMon Dev sections, per mode

The gear list (R8) becomes four groups, each with its parts, one checkbox per part, in two columns,
Standard and Plain.

| Group | Parts |
|---|---|
| Status | Waits on you, Outcomes, Burn, Git, Agents map, Sessions |
| Sessions last 30 min | Burn last 30 min, Vendor strip, Activity, Heatmap harness x minute, Turn ticker |
| Processes last 30 min | System, Process groups (with memory, disks and network), Processes |
| Microsoft To Do | To Do |

On top of the list, one switch: Overall scrollbar, on by default (Wilco, 2026-10-25). On: the page
scrolls as now. Off: everything stays in the window; parts shrink within the limits below, and a
part that still does not fit is hidden with "N sections hidden, the window is too small".

Layout rules:

- Status: Waits on you, Outcomes, Burn, Git and Agents map fill the first row, in that order. A part
  switched off gives its width to the others in the row.
- Sessions last 30 min: Activity and Heatmap share one row; with one off, the other takes the row.
- Turn ticker: its height follows its content and the room left, so with Processes off it no
  longer stretches to the bottom of the window (Wilco's screenshot, 2026-10-25).
- Processes last 30 min: the line charts (CPU, RAM, disk, network) may shrink up to 40 percent in
  height when room is needed, never more.
- Agents map, full view: its columns spread over the whole window width.

## 4. Station opens in the space theme

`station_theme` in burnmon-dev.json already takes "site" or "space" (default "site",
`cmd\burnmon-dev\app.go:423`). Decision (Wilco, 2026-10-25): the default becomes "space"; the gear
menu gets "Station starts as: space / site", saved to that same setting. T keeps switching the
theme for the current run only.

## Checks

Tests first. `go vet ./...`, `go test ./... -count=1`, `.\build.ps1`, and `.\scripts\uicheck.ps1`
for every UI change (d29 rewritten for the groups, new checks for the row rules, the scrollbar
switch and the 40 percent floor).

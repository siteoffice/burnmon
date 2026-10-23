# Hub Agent Update - ZeroNonsense.dev

**Date:** 2026-10-23 - **Owner:** Wilco de Tree
**Project:** BurnMon
**Purpose:** Tells the hub that R6 (the relation graph) is code-complete on a branch, with the G4 design conditions met, and what is still unchecked live.
**Read order:** this file, then `C:\ZND\50_projects\burnmon\docs\snapshot_schema.md` (section "Relation graph"), then `C:\ZND\10_holding\02_roadmap\2026-10-22_agent_control_plan.md` (row R6)
**Supersedes:** nothing (follows `C:\ZND\50_projects\burnmon\04_assets\hub_agent_update_2026-10-23_r4_snapshot.md`)

## 1. Headline
R6 of lane A is code-complete on branch `feat/lane-a-r4-snapshot`. It is not committed. The snapshot now carries `nodes[]` and `edges[]` with `schema_version` still 1. The store keeps live-only edges through migration 8. A Claude adapter bug (subagents stored with no parent) is fixed. The backfill for existing rows is ready but not applied to the real store.

## 2. What changed on disk
- **Committed:** nothing in this step. The branch head is still `407fac0` (R4 hub brief) over `7fb93c4` (R4 snapshot).
- **On the branch, uncommitted** (repo `C:\ZND\50_projects\burnmon`):
  - (a) ParentID fix: `C:\ZND\50_projects\burnmon\internal\adapter\claude\claude.go` (reads `sessionId` from a subagent trail when `isSidechain` is true), `C:\ZND\50_projects\burnmon\internal\store\store.go` (`BackfillClaudeParents`), and `C:\ZND\50_projects\burnmon\cmd\burnmon-cli\main.go` (new `backfill-parents` command, a dry run unless `-apply`). Tests: `internal\adapter\claude\parent_test.go`, `internal\store\parents_test.go`.
  - (b) Graph: new package `C:\ZND\50_projects\burnmon\internal\graph\` (`graph.go`, `sources.go`, `history.go`, `listen_windows.go`, `listen_other.go`, plus tests). Also `C:\ZND\50_projects\burnmon\internal\sysmon\process_windows.go` (per-process rows via `Procs()`), `internal\sysmon\types.go`, `internal\sysmon\process_other.go`, `C:\ZND\50_projects\burnmon\internal\snapshot\snapshot.go` (`nodes`, `edges`), `C:\ZND\50_projects\burnmon\cmd\burnmon\snapshot.go` (graph sources wired into the tick), `C:\ZND\50_projects\burnmon\internal\pricing\pricing.go` (new config key `foreman_root`), `C:\ZND\50_projects\burnmon\testdata\snapshot_example.json` (example graph added), `C:\ZND\50_projects\burnmon\docs\snapshot_schema.md` (public-safe graph section, no hub path).
  - (c) Migration 8: `C:\ZND\50_projects\burnmon\internal\store\migrations\migrations.go` (tables `graph_nodes`, `graph_edges`) and `C:\ZND\50_projects\burnmon\internal\store\graph.go` (`RecordGraph`, `GraphBetween`, `SessionsBetween`). `burnmon-cli graph -since 24h` prints the history graph as JSON.
- **Untracked and ignored by design (G4 condition 3):** `C:\ZND\50_projects\burnmon\docs\relation_graph.md`, the private design note with an "As built" section. `.gitignore` excludes `docs\`.
- **Outside the repo:** store backup `%LOCALAPPDATA%\burnmon\burnmon.db.bak-2026-10-23-r6`, taken with SQLite `VACUUM INTO` (integrity ok, 29,652 events).
- **Left in the tree, not deleted:** `C:\ZND\50_projects\burnmon\.r6tmp\` (two throwaway programs: the backup and the live check). A Groundwork policy blocked the delete because no approver is named.

## 3. What did NOT happen (and why)
- Not committed, not pushed, not tagged. Commits are Wilco's step.
- The real store is untouched apart from read-only dry runs. Migration 8 and the backfill ran only on a scratch copy of the backup. The real store gets migration 8 the first time a new build opens it. The backfill on the real store waits on Wilco (`burnmon-cli backfill-parents -apply`, after he installs the new build).
- `foreman_root` is not set in Wilco's `%LOCALAPPDATA%\burnmon\burnmon.json`. Until it is, the live snapshot has no Foreman links.
- No live Foreman sprint was running, so the done-when line "a real Foreman sprint with its subagents shows live" is **not checked**. History was checked (see section 4).
- `burnmon.exe` was not run with the new code. The live graph was checked with a throwaway program that uses the same readers, not with the app's tick. Nothing measured the effect on Fleet's read time of a snapshot that is about 95 KB larger.
- Messages between agents: not built. No harness's local data names the receiver in a form BurnMon stores. The schema doc says so.
- R7 not started. No UI changed, so `uicheck.ps1` was not run.

## 4. Findings worth propagating
- [RESULT] The Claude adapter never set a subagent's parent. On the real store, a dry run counts **13,241 of 29,652 event rows** that would get a parent. On a scratch copy, `-apply` set 13,241 rows and a second dry run counted 0.
- [RESULT] History graph (scratch copy, last 10 days, Foreman folder passed as a runtime flag): 254 nodes, 657 edges. Sprint `10_holding__crlf-writers-lf__20261014T213427Z` shows its main session `a35d2f30` with 6 spawned subagents and an inferred `works_for` link.
- [RESULT] Live graph on this laptop: both running Claude Code sessions link to their own process through Claude Code's pid files, with procStart matching the process start time. One session uses 13 MCP servers and the other 12, matched by command line. 213 nodes and 251 edges, about 95 KB of JSON.
- [RESULT] Measured cost per tick, averaged over 10 ticks: pid files 1.0 ms, TCP table 1.9 ms, graph build (including MCP and repo lookups) 1.0 ms. The Foreman read is 151 ms uncached, so it runs once every 30 s.
- [RESULT] G4 conditions: (1) pid files used, and an unknown format marks `claude_pids` not ok and drops its edges (test `TestSnapshotOncePidFormatChangeDropsEdges`). (2) No hub path in code, tests or the schema doc: Foreman is found through `foreman_root`. (3) The design note stays untracked. (4) Backup and dry run were done before migration 8 and the backfill.
- [RESULT] `go vet ./...` clean, `go test ./... -count=1` all packages ok, `.\build.ps1` built all three binaries (2026-10-23).
- [STATE] An older build opening a version 8 store sees no pending migration and runs normally (read from `runMigrations`, not run).

## 5. Hub-level decision (if any)
Nothing. The G4 conditions are project-internal.

## 6. What the next hub read should update
- `C:\ZND\50_projects\burnmon\STATUS.md`: R6 is code-complete on the branch, live sprint check open.
- `C:\ZND\10_holding\01_projects\burnmon.md`: lane A progress.
- `C:\ZND\10_holding\02_roadmap\2026-10-22_agent_control_plan.md`: row R6 status.

Tracker rows moved: R6 of `C:\ZND\10_holding\02_roadmap\2026-10-22_agent_control_plan.md`: in_progress (code-complete, uncommitted, live sprint check and Wilco's G4 open).

## 7. Open flags for next session
- Wilco: commit the branch, install the new build, then run `burnmon-cli backfill-parents` (dry run) and `-apply`.
- Wilco: set `foreman_root` in `%LOCALAPPDATA%\burnmon\burnmon.json`, then run a Foreman sprint to check the live done-when line.
- Delete `C:\ZND\50_projects\burnmon\.r6tmp\` (blocked for the agent by policy).
- Actual minutes for row id `2026-10-22-acl-r6` in `C:\ZND\10_holding\04_engineering\estimation\estimates_log.ndjson` are not logged yet.
- Fleet reads a snapshot about 95 KB larger: read time not measured.

## 8. Related files
- `C:\ZND\10_holding\04_assets\2026-10-23_to_burnmon_lane_a_r4_r6_r7_build_brief.md`
- `C:\ZND\10_holding\04_assets\2026-10-22_agent_control_architecture.md`
- `C:\ZND\50_projects\burnmon\docs\snapshot_schema.md`
- `C:\ZND\50_projects\burnmon\04_assets\hub_agent_update_2026-10-23_r4_snapshot.md`

# Hub Agent Update - ZeroNonsense.dev

**Date:** 2026-10-23 - **Owner:** Wilco de Tree
**Project:** BurnMon
**Purpose:** Tells the hub that R4 (the snapshot file) is code-complete on a branch and its schema is frozen at version 1.
**Read order:** `C:\ZND\50_projects\burnmon\04_assets\hub_agent_update_2026-10-23_r4_snapshot.md`, then `C:\ZND\50_projects\burnmon\docs\snapshot_schema.md`, then `C:\ZND\10_holding\02_roadmap\2026-10-22_agent_control_plan.md` (row R4)
**Supersedes:** nothing

## 1. Headline
R4 of lane A is code-complete on branch `feat/lane-a-r4-snapshot`, not committed. BurnMon core writes `%USERPROFILE%\.burnmon\snapshot.json` every 5 s tick, atomically, and `schema_version` 1 is frozen for lane B.

## 2. What changed on disk
- **Committed:** nothing. Commits are Wilco's step.
- **On a branch, not merged:** `feat/lane-a-r4-snapshot` (created from `main` at `65c26b3`), working tree changes only.
- **Files touched** (full paths):
  - `C:\ZND\50_projects\burnmon\internal\snapshot\snapshot.go` and `snapshot_test.go` (schema, Build, atomic Write, Read, IsStale)
  - `C:\ZND\50_projects\burnmon\cmd\burnmon\snapshot.go` and `snapshot_test.go` (writer goroutine, `snapshotOnce`)
  - `C:\ZND\50_projects\burnmon\cmd\burnmon\main.go` and `main_other.go` (one line each, starts the writer)
  - `C:\ZND\50_projects\burnmon\testdata\snapshot_example.json` (fixture for lane B)
  - `C:\ZND\50_projects\burnmon\docs\snapshot_schema.md` (schema doc)
- **Untracked / outside a repo:** `docs\snapshot_schema.md` is ignored by `.gitignore` line 12 (`docs/`), so it needs `git add -f`. Not mine, already present before this session: `C:\ZND\50_projects\burnmon\burnmon-dev.exe~`, `C:\ZND\50_projects\burnmon\03_logs\blackbox\`, and a modified `README.md`.

## 3. What did NOT happen (and why)
- Not committed, not pushed, not tagged, not merged (Stays human rule in `C:\ZND\50_projects\burnmon\AGENTS.md`).
- The built `burnmon.exe` was not started. No snapshot file from a real run has been seen. The brief's "written on every tick on Windows" is covered by `snapshotOnce` tests, not by a live run.
- The CPU cost of the process sampler inside the core is not measured.
- The Fleet mod reading the file is not built here.
- R6 and R7 not started, as instructed.
- Estimate actuals for `2026-10-22-acl-r4` not yet appended to `C:\ZND\10_holding\04_engineering\estimation\estimates_log.ndjson`: the minutes come from Wilco at G4.

## 4. Findings worth propagating
- [RESULT] Tests written first and seen failing (compile errors, feature missing), then passing: 11 in `internal\snapshot`, 3 in `cmd\burnmon`. Source: `go test` runs this session.
- [RESULT] `go vet ./...` clean. `go test ./... -count=1` ok in every package. `.\build.ps1` exit 0. `GOOS=linux go vet ./cmd/burnmon ./internal/snapshot` clean. Source: runs this session.
- [RESULT] A failing store or process sampler does not stop the write: the file carries that source as `ok: false` with its error.
- [STATE] `cost.usd` is the `live.Session.Cost` figure the Now page already uses, `cost.eur` is that times `fx_usd_eur`, label "API price equivalent". Whether this is the right basis (versus `BusinessCost`) is undecided.
- [STATE] Version 1 has no plan-limit field. The brief's order (tokens, plan limits, euros) is only partly served.
- [STATE] Stale rule: the file is stale when `written_at` is older than 3 ticks (15 s). Tested, not seen live.

## 5. Hub-level decision (if any)
Nothing. The freeze of schema version 1 is project-internal. The park exception for this lane is already covered by the decision of 2026-10-22.

## 6. What the next hub read should update
- `C:\ZND\50_projects\burnmon\STATUS.md` and `SESSION_LOG.md` (not edited this session; the session-end paragraph is still to write).
- `C:\ZND\10_holding\01_projects\burnmon.md` (R4 state).
- The agent control plan row R4: state `code-complete on branch`, not `done`, until Wilco commits and a live run is seen.

Tracker rows moved: none checked, I did not read the tracker named in `roadmap.md` section 5.

## 7. Open flags for next session
- Wilco decides whether `docs\snapshot_schema.md` is force-added or `docs/` is un-ignored for that file.
- Cost basis question above (`Session.Cost` versus `BusinessCost`).
- Live run of `burnmon.exe` to see a real snapshot and measure sampler cost.
- Lane B can start from the fixture now.
- At G4: ask minutes per row, append the `actual` lines.

## 8. Related files
- `C:\ZND\10_holding\04_assets\2026-10-23_to_burnmon_lane_a_r4_r6_r7_build_brief.md`
- `C:\ZND\10_holding\04_assets\2026-10-22_agent_control_architecture.md` (section 3, C1)
- `C:\ZND\10_holding\02_roadmap\2026-10-22_agent_control_plan.md`

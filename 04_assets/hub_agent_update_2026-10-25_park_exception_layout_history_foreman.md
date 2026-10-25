# Hub Agent Update - ZeroNonsense.dev

**Date:** 2026-10-25 (Cowork cloud session) - **Owner:** Wilco de Tree
**Project:** BurnMon
**Purpose:** records a park exception ("all now") for one Foreman run, a Cowork (Cloud) subagent fix, and the decisions behind a new layout, snapshot and history spec.
**Read order:** this file, `C:\ZND\50_projects\burnmon\02_roadmap\2026-10-25_layout_snapshot_history_spec.md`, top paragraph of `C:\ZND\50_projects\burnmon\SESSION_LOG.md`
**Supersedes:** nothing

## 1. Headline
Wilco granted a park exception on 2026-10-25 ("all now"): the layout, shared snapshot and kept-history spec builds now, in one Foreman run together with R3, R5, R9, R10 and a documentation pass. The Foreman run is not created yet. A Cowork (Cloud) subagent fix is code-complete, uncommitted.

## 2. What changed on disk
- **Committed:** nothing.
- **Uncommitted in `C:\ZND\50_projects\burnmon` (main tree, after `00e042a`):** `C:\ZND\50_projects\burnmon\internal\adapter\claude\cloud.go`, `C:\ZND\50_projects\burnmon\internal\store\store.go`, `C:\ZND\50_projects\burnmon\internal\adapter\claude\cloud_test.go`, new `C:\ZND\50_projects\burnmon\internal\adapter\claude\cloud_subagent_test.go`, new `C:\ZND\50_projects\burnmon\internal\store\cloud_subagent_test.go`.
- **New spec:** `C:\ZND\50_projects\burnmon\02_roadmap\2026-10-25_layout_snapshot_history_spec.md`.
- **Session log:** paragraph added to the top of `C:\ZND\50_projects\burnmon\SESSION_LOG.md`.
- **Sprint specs (new, hub repo, uncommitted):** `C:\ZND\10_holding\05_docs\superpowers\specs\2026-10-25-burnmon-b1-shared-snapshot-keep-history.md`, `...-b2-sections-layout-station.md`, `...-b3-r9-waits-on-you-outcomes.md`, `...-b4-r10-first-controls.md`, `...-b5-docs-pass.md`, `2026-10-25-groundwork-g1-r5-doctor-readmes.md`, `2026-10-25-groundwork-g2-r3-fleet-git-lane-public.md`; all pass spec_lint.
- **Groundwork (uncommitted):** `C:\ZND\50_projects\groundwork\pyproject.toml`, pytest testpaths only, so spec_lint rule 3 passes for project groundwork.

## 3. What did NOT happen (and why)
- The subagent fix is not committed and not checked with `.\build.ps1` on Windows; Wilco runs that. Go vet, go test and a Windows cross-build passed in a Linux copy only.
- The Foreman run does not exist yet. This session could not read `C:\ZND\10_holding` (folder access refused) or Foreman's contract, so the R3, R5, R9 and R10 rows were not read here and no sprint files were written.
- Nothing pushed, tagged or released by this session. R4, R6, R8 and R7 are already merged on `main` (`00e042a`, checked live with `git ls-remote`).

## 4. Findings worth propagating
- [RESULT] Cloud Cowork subagents were missing from the Station: the burnmon-cloud plugin sends each subagent request with its agent id (measured live 2026-10-25), but BurnMon's parser ignored it and filed the request under the parent session. Fixed in code, see section 2.
- [RESULT] Only burnmon.exe writes `%USERPROFILE%\.burnmon\snapshot.json`, so BurnMon Dev's Status sections depend on burnmon.exe (read in code).
- [RESULT] BurnMon deletes a session's events when its transcript leaves the disk (`internal\dataset\dataset.go:718`); history does not survive vendor cleanup today (read in code).
- [STATE] Decisions for the spec: either exe writes the snapshot under a lock file; `keep_history` on by default with delete and export; four section groups per mode; overall scrollbar on by default; Station starts in the space theme.

## 5. Hub-level decision
  **Decision:** BurnMon park exception, 2026-10-25: build the layout, shared snapshot and kept-history spec now, in one Foreman run with R3 (Fleet and Git lane public), R5 (Doctor and READMEs), R9 (waits on you), R10 (first controls) and a pass over all documentation and READMEs, as automatic as possible.
  **Context:** BurnMon is parked until 2026-11-01 except the Station. A question from a user asked whether history survives vendor deletion; it does not. BurnMon Dev depends on burnmon.exe for its Status sections.
  **Alternatives considered:** build after the park (2026-11-01), or only keep_history now. Rejected by Wilco: "all now". Also weighed: proposal P5 of `C:\ZND\10_holding\04_assets\2026-10-25_premortem_plan_to_martijn_review.md` (freeze at merged plus R3 and R5, R9 to R12 after the review). Wilco chose "R9 and R10 now" on 2026-10-25, knowing the hands-on p80 of R5, R3, R9 and R10 is 17.2 h. P5's other parts (20 Nov date, R12) are not decided here.
  **Consequences:** the park ends early in practice for BurnMon. It builds on `main` at `00e042a`, where R6, R8 and R7 are merged; it rebuilds R8's section list. Commits, tags and pushes stay Wilco's.
  **Links:** `C:\ZND\50_projects\burnmon\02_roadmap\2026-10-25_layout_snapshot_history_spec.md`

## 6. What the next hub read should update
`C:\ZND\10_holding\03_logs\decisions.md` (section 5), `C:\ZND\10_holding\01_projects\burnmon.md` (park state), `C:\ZND\50_projects\burnmon\STATUS.md`.

Tracker rows moved: none (the tracker was not reachable from this session).

## 7. Open flags for next session
- Sprint specs B1 to B5, G1 and G2 written later this session in `C:\ZND\10_holding\05_docs\superpowers\specs\`; flows are created on the laptop (Foreman paths are Windows only).
- Windows check and commit of the subagent fix.

## 8. Related files
`C:\ZND\50_projects\burnmon\02_roadmap\2026-10-12_cowork_cloud_bucket_spec.md`, `C:\ZND\50_projects\burnmon\04_assets\hub_agent_update_2026-10-23_r8_sections_presets.md`, `C:\ZND\50_projects\burnmon\docs\relation_graph.md`.

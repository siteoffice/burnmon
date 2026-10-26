# Sprint report: burnmon B3 (R9), Waits on you and Outcomes show gates, prompts and failed checks

This copy lives on the sprint branch because the Git guard held the write of report.md into the sprint folder (C:\ZND\10_holding\sprints\burnmon\burnmon-b3-r9-waits-on-you-outcomes\report.md). Once you allow it, copy this file there as report.md and run the close-out gate:
python C:\ZND\10_holding\sprints\_lib\sprint_step.py C:\ZND\10_holding\sprints\burnmon\burnmon-b3-r9-waits-on-you-outcomes --phase report --action close_out --actor orchestrator --detail "close-out gate"

Status: passing (verify 3 passed after one fix), parked at waiting-on-you for the merge, the schema doc patch and the live checks.
Run mode: Unattended; parked tasks: 0; needed a person although Unattended: no
Iterations used: 3 of 5.
Base branch: main at 4c33094 (B2 merged and pushed).
Sprint dir: C:\ZND\10_holding\sprints\burnmon\burnmon-b3-r9-waits-on-you-outcomes
Spec: C:\ZND\10_holding\05_docs\superpowers\specs\2026-10-25-burnmon-b3-r9-waits-on-you-outcomes.md
Plan: C:\Users\WilcoDeTree\AppData\Local\Temp\claude\C--ZND\ba637e9b-d75d-4b35-b761-f32da5cd2155\scratchpad\b3_plan.md (8 tasks, plus tasks 9 and 10 for verify findings; held outside the sprint folder by the Git guard)
Branch: sprint/burnmon-b3-r9-waits-on-you-outcomes, head 39fcd8f before this file, 14 commits on main 4c33094
Worktree: C:\ZND\50_projects\burnmon\.worktrees\sprint-burnmon-b3-r9-waits-on-you-outcomes

## What was built

- internal\graph: ReadForeman reads spec and result paths, parked tasks and reasons, last test and verify checks, gate open time (6466fd7).
- internal\waits: builds wait items (open gates, waiting sprints with parked tasks, stopped sprints with a failing check, waiting sessions, agent view prompts and questions) and outcome items (finished and stopped sprints and failed checks of the last 24 hours, one row per sprint) (6e83ee3, 244f394, 39fcd8f).
- internal\snapshot: additive wait_items and outcome_items, schema version stays 1, example fixture updated (5f376c8, 6b3b73f).
- internal\snapshotwriter: items built each tick, Foreman read every 7.5 s, opt-in agent view poller (default off, refuses a .cmd shim, bounded) (5c3005e, 2f77d8b, 86a249d).
- burnmon-cli waits: lists waits and outcomes, read only (a1a4953).
- burnmon-dev: bdevOpenItemFile opens only a spec or result listed in the current snapshot, allowed types only (11a16e3, a362988).
- Page: Waits on you and Outcomes list every item in a bounded scroll list with counts, links that open spec and result, aged list greyed, plain wording (c08ddea, fb6eb8a, 86a249d).
- uicheck: d36 renders both sections from the unmodified example.

## Deviations from the plan

- The Git guard held the write of plan.md, report.md and the schema doc, so the plan stays in the scratchpad, this report lives in 04_assets on the branch and the schema doc edit is held as a patch (HAND below).
- Tasks 9 and 10 were added in iterations 2 and 3. The 5-row cap with "+k more" was replaced by a bounded inner scroll list because it hid gates and waiting sprints.
- Reviewer findings fixed along the way: the agent view run was not bounded by its timeout (2f77d8b); the file opener passed the page's spelling of a path (a362988).

## Test result

- go vet ./...: exit 0. go vet in tools\uicheck: exit 0. GOOS=linux go vet ./cmd/burnmon-dev: exit 0.
- go test ./... -count=1 -v: 505 PASS, 0 FAIL, 35 packages ok. Baseline on main 4c33094, same command: 455.
- Em dash scan over all changed files: 0 files.
- Not run: .\build.ps1 and .\scripts\uicheck.ps1 (your burnmon.exe and burnmon-dev.exe were open).
- Live read: burnmon-cli waits against C:\ZND\10_holding reported 0 open gates and 9 waiting sprints. Foreman's queue shows 0 flows with an awaiting gate (gates 0 and 0). On disk 9 sprint.json files have status waiting-on-you (9 and 9). Zero against zero says little about gate parity.

## Verify result

- Verify 1 failed: cap hid gates and waiting sprints, Foreman read every 30 s, claude.cmd shim not bounded.
- Verify 2 failed: stopped sprints without a failing check shown nowhere, wrong waitingFor mapping, read_at stamp.
- Verify 3 (blind, Opus, headless Chromium, live CLI read): scope items 1 to 5 pass in code and headless. One defect, a done sprint with a failing check listed twice, was fixed in 39fcd8f and retested by me (not by a fourth blind verify).
- Packs: security-reviewer, data-modelling-reviewer, testing-reviewer, all filled (source armory). No blocking finding left.
- Headless is not WebView2: window level claims are UNVERIFIED until uicheck d29, d33, d34, d36 run.

## Close-out

- DO done: tasks 1 to 10 committed on the sprint branch, each reviewed, based on main 4c33094.
- DO done: 505 tests pass against a baseline of 455, vet clean, no em dash in changed files.
- DO done: live read of the real Foreman state with burnmon-cli: gates 0 and 0, waiting sprints 9 and 9.
- ASK parked: latency. Foreman is read every 7.5 s and the page polls every 5 s, so a new gate reaches the page in up to about 15.7 s (10 s at the snapshot file). The spec says within two ticks. Recommend accepting; the alternative is a read every tick, about 400 ms of work per 5 s.
- ASK parked: the agent view poller is off by default and, on this laptop, claude resolves to claude.cmd so it would refuse to run. Recommend leaving it off until claude.exe is the resolved binary.
- ASK parked: .html spec or result files open in the default browser. Recommend accepting, or dropping .html from the allowed types.
- ASK parked: BurnMon treats a gate as open when its status is not decided, Foreman uses awaiting-owner. They agree on today's data. Recommend aligning BurnMon to Foreman's rule in a later change.
- ASK parked: BurnMon lists all 9 waiting sprints, six of them old and not attached to an active flow, where Foreman's queue shows 3. Recommend keeping it, the spec asks for every waiting sprint.
- HAND parked: merge. B3 is based on main 4c33094. Commit or stash your main-tree README.md first if it is dirty.

```
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon
Set-Location C:\ZND\50_projects\burnmon
git status --short README.md
git merge --no-ff sprint/burnmon-b3-r9-waits-on-you-outcomes -m "merge sprint/burnmon-b3-r9-waits-on-you-outcomes"
go test ./... -count=1
```

- HAND parked: apply the held schema doc patch (docs\snapshot_schema.md is tracked but the commit launcher refuses to stage it because of the docs/ ignore rule), then commit it.

```
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon
Set-Location C:\ZND\50_projects\burnmon
git apply --check --ignore-whitespace C:\Users\WilcoDeTree\AppData\Local\Temp\claude\C--ZND\ba637e9b-d75d-4b35-b761-f32da5cd2155\scratchpad\b3_schema_doc_v2.patch
git apply --ignore-whitespace C:\Users\WilcoDeTree\AppData\Local\Temp\claude\C--ZND\ba637e9b-d75d-4b35-b761-f32da5cd2155\scratchpad\b3_schema_doc_v2.patch
git add -f docs/snapshot_schema.md
git status --short docs
```

- HAND parked: live checks on WebView2 with burnmon.exe and burnmon-dev.exe closed (d29, d33, d34, d36 and the default set).

```
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon\.worktrees\sprint-burnmon-b3-r9-waits-on-you-outcomes
Set-Location C:\ZND\50_projects\burnmon\.worktrees\sprint-burnmon-b3-r9-waits-on-you-outcomes
.\build.ps1
.\scripts\uicheck.ps1
.\scripts\uicheck.ps1 d29 d33 d34 d36
```

- HAND parked: copy this report and the plan into the sprint folder (the Git guard holds my write there) and remove leftovers of this sprint: Playwright logs and a temporary exe, then after the merge the worktree and branch.

```
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon
Set-Location C:\ZND\50_projects\burnmon
Copy-Item C:\ZND\50_projects\burnmon\.worktrees\sprint-burnmon-b3-r9-waits-on-you-outcomes\04_assets\2026-10-26_b3_sprint_report.md C:\ZND\10_holding\sprints\burnmon\burnmon-b3-r9-waits-on-you-outcomes\report.md
Copy-Item C:\Users\WilcoDeTree\AppData\Local\Temp\claude\C--ZND\ba637e9b-d75d-4b35-b761-f32da5cd2155\scratchpad\b3_plan.md C:\ZND\10_holding\sprints\burnmon\burnmon-b3-r9-waits-on-you-outcomes\plan.md
Get-ChildItem C:\ZND\.playwright-mcp | Where-Object { $_.LastWriteTime -ge [datetime]'2026-10-25T21:00:00' } | Select-Object Name
# review the list above, then remove those files by hand, and remove $env:TEMP\b3_cli_t5.exe; then, after the merge:
git worktree remove .worktrees\sprint-burnmon-b3-r9-waits-on-you-outcomes
git branch -d sprint/burnmon-b3-r9-waits-on-you-outcomes
```

- DROP dropped: cutting Parked.Reason to 120 runes, because the spec and plan cut only detail.
- DROP dropped: mutation proof for the new refusal tests, because the tests assert error causes and no spec item asks for mutation proof.
- DROP dropped: a fourth blind verify after the double-listing fix, because the fix is a one-line condition covered by a changed test that I ran.

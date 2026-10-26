# Sprint report: burnmon B4 (R10), first controls, each previewed before it runs

This copy lives on the sprint branch because the Git guard holds writes into the sprint folder (C:\ZND\10_holding\sprints\burnmon\burnmon-b4-r10-first-controls\report.md). Once you allow it, copy this file there as report.md and run the close-out gate:
python C:\ZND\10_holding\sprints\_lib\sprint_step.py C:\ZND\10_holding\sprints\burnmon\burnmon-b4-r10-first-controls --phase report --action close_out --actor orchestrator --detail "close-out gate"

Status: passing for what could be tested here; parked at waiting-on-you for the merge, the live runs and the decisions below.
Run mode: Unattended; parked tasks: 0; needed a person although Unattended: no
Iterations used: 2 of 5.
Base branch: sprint/burnmon-b3-r9-waits-on-you-outcomes at afe6cac (STACKED on B3 by the owner's decision of 2026-10-26, instead of main; B3 is not merged yet). Merge B3 first, then B4.
Sprint dir: C:\ZND\10_holding\sprints\burnmon\burnmon-b4-r10-first-controls
Spec: C:\ZND\10_holding\05_docs\superpowers\specs\2026-10-25-burnmon-b4-r10-first-controls.md
Plan: C:\Users\WilcoDeTree\AppData\Local\Temp\claude\C--ZND\ba637e9b-d75d-4b35-b761-f32da5cd2155\scratchpad\b4_plan.md (9 tasks, plus tasks 10 and 11 for security findings; held outside the sprint folder by the Git guard)
Branch: sprint/burnmon-b4-r10-first-controls, head 9329514 before this file, 14 commits on afe6cac
Worktree: C:\ZND\50_projects\burnmon\.worktrees\sprint-burnmon-b4-r10-first-controls
Documentation cited for the commands (read 2026-10-26): https://code.claude.com/docs/en/agent-view ("Manage sessions from the shell", "List sessions as JSON") and https://code.claude.com/docs/en/cli-reference. Documented: `claude stop <id>`, `claude attach <id|name>`, `claude agents --json`.

## What was built

- claude_path in burnmon.json (absolute claude.exe only, no .cmd shim); python resolution that refuses the WindowsApps stub (923190e).
- agent_sessions in the snapshot (no cwd, no names) and the gate token in the Foreman reader (7515e08).
- internal\controls: stop and attach argv, the gate verdict table (approve = G1 approve, G2 approve, G3 proceed; send back = G1 and G2 reject; G3 send back and every G4 control are not offered), the gate script run through Foreman's own cmd_gate (8b76c71).
- Preview tokens (random, single use, 5 s, 400 ms minimum gap) and the audit file controls-audit.ndjson next to the store (dbc9459, a802bfa).
- Never-self (own pid, ancestors, own folder, and a folder that contains BurnMon Dev), fresh target checks, hidden runner and a real console for attach (8dfca22, b874531, 9329514).
- The Service: preview, confirm, fresh re-check, audit before run (759bf46).
- Control switches (all off by default) and the page and bindings: chips, preview line, separate Confirm button, result, gear block (c87734c, e096717, check_d37.go).
- Security fixes from two separate Opus reviews (9e3d900, 7aebeaf, 9329514).

## Deviations from the plan

- Reply is parked as an ASK (see below). Attach and stop are built.
- Controls default OFF for every control (the plan said on by default; your rule and the safer choice).
- Confirm is a separate button, not the chip pressed twice.
- The audit writes a start line and a result line per executed run (two lines), so nothing runs if the audit cannot be written.
- Never-self also refuses a session whose folder contains BurnMon Dev (the session that asked for the build is not otherwise identifiable).
- Stacked on B3 instead of main, and the schema doc change is a cumulative patch that replaces the B3 patch.
- The Git guard held the write of plan.md and report.md into the sprint folder; both live elsewhere (HAND).

## Test result

- go vet ./...: exit 0. go vet in tools\uicheck: exit 0. GOOS=linux go vet ./cmd/burnmon-dev ./internal/controls: exit 0.
- go test ./... -count=1 -v: 636 PASS, 0 FAIL, 5 SKIP, 36 packages ok. Baseline on afe6cac (B3 branch, same command): 505.
- Skips: TestElevatedIsFalseInTests and TestNewControlServiceWithoutClaude (this shell is elevated), TestLayoutRealGraphFile and TestReadForemanRealRoot (need real files, by design), TestResolveClaudeNonWindowsNeedsExecutableFile (non-Windows).
- Em dash scan over all changed files: 0 files.
- Not run: .\build.ps1 and .\scripts\uicheck.ps1 (your burnmon.exe and burnmon-dev.exe were open). No control was run against a real session, process or flow.

## Security review

Two separate Opus reviews, plus the final Opus verify (none wrote the code). Findings and what happened:

- BLOCKING, never-self cwd check failed open for NTFS junctions and unresolved paths: fixed (9e3d900), then a second hole (no existing prefix returned the lexical path) fixed (7aebeaf); proven with real junctions, symlinks, subst, 8.3 names and trailing dots.
- Elevated() failed open: fixed, any error counts as elevated.
- Preview label came from the snapshot: now from the fresh read.
- Duplicate flow ids: refused at preview and run.
- Attach and stop on ended sessions: allowlist working, blocked, done plus a live pid.
- Preview serialization: one `claude agents --json` at a time, panic-safe.
- Gate script status check: only awaiting-owner passes.
- Requesting session not identifiable: cwd that contains BurnMon Dev is refused (9329514).
- Accepted with a reason: TOCTOU on gates and agents (milliseconds, stop goes by id only); gate script imports Foreman's lib (same trust as /flow); gate token visible in the preview on screen (the same user can read flow.json); configured and PATH binaries are user-controlled; audit file not tamper-evident; same drive letter mapped differently in two namespaces; python comes from PATH.
- Final verdict of the last Opus review: SECURITY APPROVE WITH ACCEPTED RISKS, plus the narrow fix 9329514 retested by the orchestrator, not re-reviewed by Opus.

## Close-out

- DO done: tasks 1 to 11 committed on the sprint branch, based on the B3 branch afe6cac.
- DO done: 636 tests pass against a baseline of 505, vet clean, no em dash in changed files.
- DO done: separate Opus security review done twice, findings fixed or accepted with reasons above.
- ASK parked: two audit lines per run (start and result) where the spec says one. Recommend accepting: it is what makes the control fail closed when the audit cannot be written.
- ASK parked: reply is not built. The docs document `claude --resume <session> "prompt"`, but it needs an interactive terminal and sends no prompt while a session waits on a question, so there is no safe shell command that replies to a blocked background session. Recommend leaving reply parked.
- ASK parked: attach and stop are refused for sessions that have ended (stopped or failed), because attach would restart a stopped session. Recommend keeping that.
- ASK parked: never-self refuses any session whose working folder contains the BurnMon folder (for example a session in C:\ZND\50_projects\burnmon or C:\ZND). Recommend keeping it; it is the conservative reading of "the session that asked for the build".
- ASK parked: graph GatePending is not aligned to awaiting-owner only, because an existing fixture depends on the looser rule. Recommend aligning it with a fixture change in a later change.
- ASK parked: python for the gate script comes from PATH. Recommend a configurable python_path later.
- HAND parked: merge in stack order. B3 first, then B4. Commit or stash your main-tree README.md first if it is dirty.

```
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon
Set-Location C:\ZND\50_projects\burnmon
git status --short README.md
git merge --no-ff sprint/burnmon-b3-r9-waits-on-you-outcomes -m "merge sprint/burnmon-b3-r9-waits-on-you-outcomes"
git merge --no-ff sprint/burnmon-b4-r10-first-controls -m "merge sprint/burnmon-b4-r10-first-controls"
go test ./... -count=1
```

- HAND parked: apply the cumulative schema doc patch (it replaces the B3 patch, do not apply both; docs\snapshot_schema.md is tracked but the launcher refuses to stage it), then commit it.

```
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon
Set-Location C:\ZND\50_projects\burnmon
git apply --check --ignore-whitespace C:\Users\WilcoDeTree\AppData\Local\Temp\claude\C--ZND\ba637e9b-d75d-4b35-b761-f32da5cd2155\scratchpad\b4_schema_doc.patch
git apply --ignore-whitespace C:\Users\WilcoDeTree\AppData\Local\Temp\claude\C--ZND\ba637e9b-d75d-4b35-b761-f32da5cd2155\scratchpad\b4_schema_doc.patch
git add -f docs/snapshot_schema.md
git status --short docs
```

- HAND parked: enable the controls on this laptop. They stay off until burnmon.json names a real claude.exe and agent_view is true. Find the real binary behind the npm shim first.

```
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon
Set-Location C:\ZND\50_projects\burnmon
Get-Content C:\Users\WilcoDeTree\AppData\Roaming\npm\claude.cmd
Test-Path C:\Users\WilcoDeTree\AppData\Roaming\npm\node_modules\@anthropic-ai\claude-code\bin\claude.exe
# then add to burnmon.json: "claude_path": "<that claude.exe>", "agent_view": true
```

- HAND parked: live checks and one live run per built control (throwaway session for stop and attach, disposable flow under a disposable foreman_root for approve and send back), from a non-elevated BurnMon Dev, with the audit lines to be quoted from controls-audit.ndjson. Replace the placeholders before running.

```
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon\.worktrees\sprint-burnmon-b4-r10-first-controls
Set-Location C:\ZND\50_projects\burnmon\.worktrees\sprint-burnmon-b4-r10-first-controls
.\build.ps1
.\scripts\uicheck.ps1
.\scripts\uicheck.ps1 d29 d33 d34 d36 d37
# throwaway session: claude --bg "say hello"   then open BurnMon Dev, tick Stop in the gear, preview, Confirm
Get-Content "$env:LOCALAPPDATA\burnmon\controls-audit.ndjson" -Tail 6
```

- HAND parked: copy this report and the plan into the sprint folder (the Git guard holds my write there) and remove leftovers of this sprint: scratch junctions and folders of the reviewers, the Playwright logs, then after the merge the worktree and branch. Remove each junction with rmdir on the junction itself only.

```
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon
Set-Location C:\ZND\50_projects\burnmon
Copy-Item C:\ZND\50_projects\burnmon\.worktrees\sprint-burnmon-b4-r10-first-controls\04_assets\2026-10-26_b4_sprint_report.md C:\ZND\10_holding\sprints\burnmon\burnmon-b4-r10-first-controls\report.md
Copy-Item C:\Users\WilcoDeTree\AppData\Local\Temp\claude\C--ZND\ba637e9b-d75d-4b35-b761-f32da5cd2155\scratchpad\b4_plan.md C:\ZND\10_holding\sprints\burnmon\burnmon-b4-r10-first-controls\plan.md
Get-ChildItem C:\Users\WilcoDeTree\AppData\Local\Temp\claude\C--ZND\ba637e9b-d75d-4b35-b761-f32da5cd2155\scratchpad -Recurse -Force -ErrorAction SilentlyContinue | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint } | Select-Object FullName
# remove each listed junction with: cmd /c rmdir "<FullName>"   (never delete through the link)
# after the merge: git worktree remove .worktrees\sprint-burnmon-b4-r10-first-controls ; git branch -d sprint/burnmon-b4-r10-first-controls
```

- DROP dropped: aligning graph GatePending now, because an existing fixture depends on the looser rule and the controls re-check the gate themselves.
- DROP dropped: a third Opus pass on the last small fix (9329514), because it only adds a refusal and its tests were run by me.
- DROP dropped: masking the gate token in the on-screen preview, because the same user can read it in flow.json and the plan requires the exact command to be shown.

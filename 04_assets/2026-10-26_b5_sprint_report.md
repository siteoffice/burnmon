# Sprint report: burnmon B5, documentation and READMEs match what landed

This copy lives on the sprint branch because the Git guard holds writes into the sprint folder (C:\ZND\10_holding\sprints\burnmon\burnmon-b5-docs-pass\report.md). Once you allow it, copy this file there as report.md and run the close-out gate:
python C:\ZND\10_holding\sprints\_lib\sprint_step.py C:\ZND\10_holding\sprints\burnmon\burnmon-b5-docs-pass --phase report --action close_out --actor orchestrator --detail "close-out gate"

Status: passing; parked at waiting-on-you for the merge, the docs patches and the decisions below.
Run mode: Unattended; parked tasks: 0; needed a person although Unattended: no
Iterations used: 2 of 5.
Base branch: sprint/burnmon-b4-r10-first-controls at eeb2f26 (STACKED, owner decision of 2026-10-26; B4 is stacked on B3, which is based on main 4c33094). B3, B4 and B5 are not merged. Merge order: B3, B4, B5.
Sprint dir: C:\ZND\10_holding\sprints\burnmon\burnmon-b5-docs-pass
Spec: C:\ZND\10_holding\05_docs\superpowers\specs\2026-10-25-burnmon-b5-docs-pass.md
Plan: C:\Users\WilcoDeTree\AppData\Local\Temp\claude\C--ZND\ba637e9b-d75d-4b35-b761-f32da5cd2155\scratchpad\b5_plan.md (6 tasks, plus task 7 for verify findings; held outside the sprint folder by the Git guard)
Branch: sprint/burnmon-b5-docs-pass, head 8fcef14 before this file, 6 commits on eeb2f26
Worktree: C:\ZND\50_projects\burnmon\.worktrees\sprint-burnmon-b5-docs-pass

## What was built

- internal\doccheck: a Go test that checks the documents against the code in both directions (config keys, CLI subcommands and flags, bindings, uicheck ids, section ids and hotkeys named in the README must exist; code items the README leaves out are findings), with a planted-claim test that fails on a wrong key, binding, uicheck id, flag, path, section, subcommand and hotkey, and an explicit list of tracked docs (743a5f3, 07d1fcc).
- README.md: B1 to B4 (CLI subcommands, M key, section parts table, Waits on you and Outcomes, controls, Foreman and Claude Code settings, the npm claude.exe route, stale Station theme line fixed) (e5e00c8, d595314, 07d1fcc, 8fcef14).
- tools\burnmon-cloud\plugin\README.md (subagents as their own sessions), STATUS.md new top paragraph read live from git, one SESSION_LOG.md paragraph (7403b23, d595314, 07d1fcc).
- docs\snapshot_schema.md and docs\relation_graph.md: HAND patches in the scratchpad, see below.

## Deviations from the plan

- Stacked on B4 instead of main (B1 to B4 are not all merged), so the docs describe branches awaiting merge and say so.
- The Git guard held plan.md and report.md writes into the sprint folder; both live elsewhere (HAND).
- docs\ is matched by .gitignore line 12 and the commit launcher refuses to stage it, so the two docs changes are patches, not commits (task 4 commits nothing by design).
- The B4 report says controls need both claude_path and agent_view; the code is looser (approve and send back need foreman_root and python, not claude). The README follows the code. Reconcile the B4 report text when you copy it.
- A first Opus verification found 5 wrong claims and one real problem (the doc check scanned untracked docs, so go test would fail in the main tree after merge); fixed in 07d1fcc and re-verified.

## Test result

- go vet ./...: exit 0. go vet in tools\uicheck: exit 0.
- go test ./... -count=1 -v: 652 PASS, 0 FAIL, 5 SKIP, 37 packages ok. Baseline on eeb2f26 (B4 branch, same command): 636. The skips are the five named in the B4 report.
- Em dash scan over all changed files and the patch: 0 files.
- Simulated main tree after merge (by the verifier, read-only): the doc check passes with stray untracked docs present, with the schema doc patched and unpatched.
- Not run: .\build.ps1 and .\scripts\uicheck.ps1.

## Verify result

- Verify 1 (Opus, blind, about 70 claims checked, 5 wrong): schema patch lacked B1 text, STATUS said the docs file is ignored (it is tracked), doc check scanned untracked docs, wrong outcome row wording, README gaps. All fixed.
- Verify 2 (Opus, focused): VERIFY PASS, 52 new claims checked, 0 wrong, 2 imprecise (fixed in 8fcef14).
- Pack: testing-reviewer (filled, source armory). Security and data packs were not applicable to a docs change.
- Untraceable claim kept honest: that the commit launcher refuses docs\ is a repeated observation in this run, not a documented rule.

## Close-out

- DO done: tasks 1 to 7 committed on the sprint branch, based on the B4 branch eeb2f26.
- DO done: 652 tests pass against a baseline of 636, vet clean, no em dash in changed files.
- DO done: about 120 claims checked by two Opus verifications, wrong ones fixed.
- ASK parked: add foreman_root, agent_view and claude_path to burnmon.example.json. Recommend yes, in a small later change (this sprint does not change code beyond the doc check).
- ASK parked: refresh the README Roadmap section ("as of 2026-10-06"), which is stale. Recommend a short update by you, it is a priority statement.
- ASK parked: GLOSSARY.md was not touched. Recommend adding the new terms (waits, outcomes, controls, agent view) in a later pass.
- HAND parked: merge in stack order B3, B4, B5, after committing or stashing your main-tree README.md edits (lines 11 and 91 are yours; the branch hunks do not overlap them, a merge-tree check was clean).

```
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon
Set-Location C:\ZND\50_projects\burnmon
git status --short README.md
git merge --no-ff sprint/burnmon-b3-r9-waits-on-you-outcomes -m "merge sprint/burnmon-b3-r9-waits-on-you-outcomes"
git merge --no-ff sprint/burnmon-b4-r10-first-controls -m "merge sprint/burnmon-b4-r10-first-controls"
git merge --no-ff sprint/burnmon-b5-docs-pass -m "merge sprint/burnmon-b5-docs-pass"
go test ./... -count=1
```

- HAND parked: apply the ONE cumulative schema patch (B1, B3, B4 and B5 text; it replaces the B1 patch in the B1 sprint folder, the B3 patches and the B4 patch, never apply those), then commit it. The docs file is tracked but the launcher refuses to stage it.

```
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon
Set-Location C:\ZND\50_projects\burnmon
$sp = 'C:\Users\WilcoDeTree\AppData\Local\Temp\claude\C--ZND\ba637e9b-d75d-4b35-b761-f32da5cd2155\scratchpad'
git apply --check --ignore-whitespace "$sp\b5_docs.patch"
git apply --ignore-whitespace "$sp\b5_docs.patch"
git add -f docs/snapshot_schema.md
git commit -m "docs: snapshot schema for B1, B3, B4 (writer lock, waits, outcomes, agent sessions)"
```

- HAND parked: copy the updated relation graph note over your private untracked one (diff it first; it stays untracked and ignored by design).

```
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon
Set-Location C:\ZND\50_projects\burnmon
$sp = 'C:\Users\WilcoDeTree\AppData\Local\Temp\claude\C--ZND\ba637e9b-d75d-4b35-b761-f32da5cd2155\scratchpad'
git diff --no-index docs\relation_graph.md "$sp\b5_relation_graph.md"
Copy-Item -LiteralPath "$sp\b5_relation_graph.md" -Destination C:\ZND\50_projects\burnmon\docs\relation_graph.md
```

- HAND parked: build and live checks with the apps closed, and choose public screenshots yourself (none were made; the README says so).

```
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon
Set-Location C:\ZND\50_projects\burnmon
.\build.ps1
.\scripts\uicheck.ps1
```

- HAND parked: copy this report and the plan into the sprint folder (the Git guard holds my write there) and clean up leftovers of this sprint (scratch folders with junctions made by the B4 reviewers are listed in the B4 report).

```
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon
Set-Location C:\ZND\50_projects\burnmon
Copy-Item C:\ZND\50_projects\burnmon\.worktrees\sprint-burnmon-b5-docs-pass\04_assets\2026-10-26_b5_sprint_report.md C:\ZND\10_holding\sprints\burnmon\burnmon-b5-docs-pass\report.md
Copy-Item C:\Users\WilcoDeTree\AppData\Local\Temp\claude\C--ZND\ba637e9b-d75d-4b35-b761-f32da5cd2155\scratchpad\b5_plan.md C:\ZND\10_holding\sprints\burnmon\burnmon-b5-docs-pass\plan.md
# after the merge: git worktree remove .worktrees\sprint-burnmon-b5-docs-pass ; git branch -d sprint/burnmon-b5-docs-pass
```

- DROP dropped: fact-checking SESSION_LOG.md paragraphs claim by claim, because it is history and is only scanned for em dashes.
- DROP dropped: the stale comment in internal\waits\waits.go ("one outcome row per sprint"), because the spec forbids code changes beyond the doc check.
- DROP dropped: checks of inline bold keys in running prose by the doc check, because only list items are checked and the README has no such sentence.

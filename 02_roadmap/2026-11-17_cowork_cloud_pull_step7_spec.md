# Cowork cloud pull, step 7: clean up what the tests left

Flow `burnmon/cowork-cloud-pull`, step 7 of 7. Parent spec: section 13 ("Clean-up, ask Wilco's yes
first") of `C:\ZND\10_holding\04_assets\2026-11-17_burnmon_cowork_cloud_gap_research_and_proposal.md`.
Date: 2026-11-17. Status: spec, awaiting the spec gate. Approving this spec is the yes to delete
exactly the items below, nothing else.

## What goes (listed live 2026-11-17 13:25 UTC)

| # | Item | Path | Size |
|---|---|---|---|
| 1 | Teleport-test transcripts (section 11 and 12) | `C:\Users\WilcoDeTree\.claude\projects\C--Users-WilcoDeTree-AppData-Local-Temp-teleport-test` (folder) | 5 files, 21,019,356 bytes |
| 2 | Teleport-test scratch repo | `C:\Users\WilcoDeTree\AppData\Local\Temp\teleport-test` (folder) | 27 files, 28,345 bytes |
| 3 | Older hub pull, not in the state file | `...\C--Users-WilcoDeTree--burnmon-cloud-pull\8adec148-ac5c-44f5-a635-3922c79da278.jsonl` and its folder | 8,708,509 bytes |
| 4 | Older hub pull, not in the state file | `...\C--Users-WilcoDeTree--burnmon-cloud-pull\40f50e6f-246c-439a-8d3d-36bd579b1d4e.jsonl` and its folder | 8,708,509 bytes |
| 5 | Orphan pull cut off when the first test window closed | `...\C--Users-WilcoDeTree--burnmon-cloud-pull\263029fd-3cc2-4813-bdd5-143e703e3470.jsonl` and its folder | 5,580,849 bytes |
| 6 | Scratch worktree of `main` for the uicheck A/B | `C:\Users\WilcoDeTree\AppData\Local\Temp\claude\7db7c754ee90779d\00a3689d-cbc9-48da-8552-a11788381d14\scratchpad\main-wt`, removed with `git worktree remove` so the repo forgets it | 732 files, 54,069,344 bytes |
| 7 | gofmt check copies written to the wrong folder | `C:\Users\WilcoDeTree\AppData\Local\Temp\gf_*.go` | 11 files |

`...` is `C:\Users\WilcoDeTree\.claude\projects`.

## What stays

- Every pull file the state file maps, the state file `C:\Users\WilcoDeTree\.burnmon\cloud-pull-state.json`
  and the pull folder: they are the live feature (the switch stays on, Wilco 2026-11-17).
- The store. With `keep_history` on, deleting items 1, 3, 4 and 5 removes no rows. Item 1's 11 remaining
  Claude Code rows are real usage (the model turns sent inside the teleported sessions); items 3 to 5 have no
  rows of their own (step 4 purge, step 5 one-row-per-request).
- The session scratchpad folder (the harness clears it).

## How to check

- Before: list each path. After: each is gone; `git worktree list` shows only the repo; the pull folder still
  holds every mapped file (count before and after, minus items 3 to 5).
- The store totals per agent are the same before and after.
- If the Groundwork guard refuses an agent delete, the run stops and hands Wilco the exact PowerShell lines.

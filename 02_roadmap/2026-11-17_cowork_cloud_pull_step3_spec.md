# Cowork cloud pull, step 3: the pull runner

Flow `burnmon/cowork-cloud-pull`, step 3 of 7. Parent spec: section 13 of
`C:\ZND\10_holding\04_assets\2026-11-17_burnmon_cowork_cloud_gap_research_and_proposal.md`.
Measurements: `C:\ZND\50_projects\burnmon\04_assets\2026-11-17_cowork_cloud_pull_measurement.md`.
Date: 2026-11-17. Status: spec, awaiting G2.

## What changes

New code in `internal\cloudpull`, next to the step 1 list reader. Not started by any binary yet: step 5
wires it to the Settings switch. Step 4 reads what it writes.

1. **Pull folder.** `~\.burnmon\cloud-pull`, a plain folder (step 2: no git needed). Created on first use.
   Its Claude project folder is `<claude config>\projects\<mangled path>`, where every character of the
   pull folder path that is not a letter or digit becomes `-` (measured: `C--Users-WilcoDeTree--burnmon-cloud-pull`).
   `CLAUDE_CONFIG_DIR` is honoured.
2. **Finding the CLI.** `claude` on `PATH`. When that is the npm shim (`claude.cmd` or `claude.ps1`), use
   `node_modules\@anthropic-ai\claude-code\bin\claude.exe` next to it, so no `cmd.exe` quoting is involved.
   Not found: the reason "Claude Code is not installed".
3. **One pull.** `os/exec` with a 120 s timeout, working folder the pull folder, hidden window
   (`HideWindow` plus `CREATE_NO_WINDOW`, as `internal\snapshotwriter` does). Never WMI, CIM or PSExec.
   Arguments: `--teleport <id> -p "/rename burnmon-pull" --model haiku --output-format json`.
4. **Cost guard.** Parse the JSON result. `num_turns` above 0 or `total_cost_usd` above 0: stop all
   pulling, write `disabled` with the reason into the state file, return `ErrCost`. Nothing pulls again
   until the state is cleared (step 5: switching off and on). `is_error` true, a non-zero exit or no
   JSON: a failed pull with a reason, retried after 15 minutes; not a disable.
5. **State file.** `~\.burnmon\cloud-pull-state.json`, written atomically (temp file plus rename):
   per cloud id the local `session_id` from the result, the transcript path, pull time, last usage time
   and last error; plus `disabled` and `last_error` for the whole runner. Step 4 uses it to map a pull
   file to its cloud id.
6. **Last usage.** From the new transcript: the newest `timestamp` of a row that has `input_tokens`.
   Not the last row (step 2: each pull adds rows stamped with the pull time).
7. **Schedule.** Pure function `Due(now, list, state) []string`, tiers by last usage:
   New (never pulled) at once; Hot (usage in the last 30 min) every 2 minutes (Wilco, G4 step 2);
   Warm (last 24 h) every 30 minutes; Cold (up to 31 days) once a day; Archive (older) never.
   Order: New newest-first, then Hot, Warm, Cold, each oldest-pulled first.
8. **Pacing.** A loop ticks every 30 s and runs at most one pull per tick, the first id from `Due`.
   So at most one CLI at a time and at most about 27 percent of one core (8 CPU s per 30 s, step 2).
   Each tick re-reads the step 1 list and the CLI org (`claude auth status`, at most every 15 minutes).
9. **Clean-up.** After a pull of a cloud id succeeds and its state is saved, delete that id's previous
   transcript `<old session id>.jsonl` and the folder `<old session id>\`, only when both resolve inside
   the pull folder's project folder (checked with `filepath.Rel`, no `..`). Nothing else is ever deleted.

## What stays

Scanner, store, ingest, screens, settings. The five step 2 files and the teleport-test files stay
until step 7. Until step 4 the pull files count as Claude Code.

## How to check

- Tests first, with a fake runner (no real CLI): guard trips on `num_turns` 1 and on cost 0.01 and
  persists `disabled`; a failed pull is retried after 15 minutes, not disabled; `Due` per tier at the
  boundaries (29 and 31 minutes, 2 minutes, 24 hours, 31 days) and in the stated order; one pull per
  tick; clean-up deletes the old pair and refuses a path outside the project folder; last usage ignores
  rows without `input_tokens`; path mangling; shim resolution; state file round trip.
- One live test, run only with `BURNMON_LIVE_PULL=1`: one real pull of the hub session through the
  runner, then a second one; asserts guard pass, state saved, and the first pull's files deleted.
  Run once by the agent in this step; it adds one pull file to the step 7 list.
- `go vet ./...`, `go test ./... -count=1`, `.\build.ps1`. No UI change, no `uicheck`.

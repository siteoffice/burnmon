# Cowork cloud pull, step 2: measure one pull, propose the timer

Flow `burnmon/cowork-cloud-pull`, step 2 of 7. Parent spec: section 13 of
`C:\ZND\10_holding\04_assets\2026-11-17_burnmon_cowork_cloud_gap_research_and_proposal.md`.
Date: 2026-11-17. Status: spec, awaiting G2.

## What changes

No product code. A measurement, run by the agent in PowerShell, and one result file:
`C:\ZND\50_projects\burnmon\04_assets\2026-11-17_cowork_cloud_pull_measurement.md`.

Pull folder: `C:\Users\WilcoDeTree\.burnmon\cloud-pull` (the folder BurnMon will own, section 13 item 1).

Runs, in this order. Each is the pull command of section 13:
`claude --teleport <id> -p "/rename burnmon-pull" --model haiku --output-format json`

1. Pull folder as a plain folder, no git, large session `session_0131Y29YSSRdAPR2VmEG1ET3`
   (8.5 MB in section 12). Answers: does teleport need a git repository?
2. Only if run 1 refuses for lack of a repository: `git init` plus one empty commit in the pull folder
   (a local repository BurnMon owns, never pushed), then run 1 again.
3. Same large session again: the cost of a repeat pull (same bytes or a delta?).
4. A small session: the oldest entry in the list that sits in a Project. If its pull is not clearly
   smaller than the large one, take the next one until a small one is found, at most three tries.

Per run, measured:
- Wall time (start to exit), exit code.
- CPU seconds of the `claude` process and any children (sampled from the process tree every 0.5 s).
- Peak working set.
- Bytes and lines of the new transcript file, rows with `input_tokens`, rows with a `requestId`,
  distinct request ids.
- From the JSON result: `is_error`, `num_turns`, `total_cost_usd`, `duration_api_ms`.

Hard stop: if any run shows `num_turns` above 0 or `total_cost_usd` above 0, stop all runs and report.
A pull must never cost tokens.

## What stays

All BurnMon code and screens. The new transcripts land under
`C:\Users\WilcoDeTree\.claude\projects\C--Users-WilcoDeTree--burnmon-cloud-pull` and count as Claude Code in
BurnMon until step 4. They go on the step 7 clean-up list. Nothing is deleted in this step.

## Output

The result file holds the table of runs, the git answer, and a timer proposal derived from the
numbers. Starting point from section 13: sessions with a row in the last 30 minutes every 2 minutes,
others every 15 minutes. The proposal also states a first-run plan for all 141 listed sessions
(total bytes and time, from the measured per-pull figures) and when an old session stops being pulled.

## How to check

- Every number in the result file comes from a command output in this session; inferred figures are
  marked as inferred.
- No run with `num_turns` above 0. No delete.
- No product code changed, so vet, test and build are not rerun.

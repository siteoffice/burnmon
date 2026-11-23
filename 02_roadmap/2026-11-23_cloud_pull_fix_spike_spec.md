# Spike: can the Cowork (Cloud) pull load today's sessions again

Status: approved by Wilco, 2026-11-23 ("do both lanes: honesty and trying to fix"). No code: hand-run experiments and
a findings brief. Runs next to `2026-11-23_cloud_strip_honesty_lane_spec.md`.

## Question

Is "Couldn't load the history of cloud session" a change in the Claude Code CLI (then a CLI version decides it) or a
change in the service (then only Anthropic can fix it)?

## What is known (read 2026-11-23)

- Pulls worked on 2026-11-17 (newest good copy 16:09 UTC) and failed from 2026-11-20 (state file, hub brief).
- 68 sessions have a good copy from before and now fail with "Couldn't load" (state file, 21:23 UTC); for example
  `session_0131Y29YSSRdAPR2VmEG1ET3` (copied 17 Nov 16:07 UTC, last usage 17 Nov 16:01).
- Claude Code versions on npm (`npm view @anthropic-ai/claude-code time`, read 2026-11-23): 2.1.326 (16 Nov 18:43
  UTC), 2.1.327 (17 Nov 20:33), 2.1.328 (18 Nov 23:09), 2.1.329 (19 Nov), 2.1.330 (20 Nov 19:16), 2.1.331 (23 Nov
  18:36). Issue #111387 names 2.1.328; the laptop ran 2.1.330 this morning.
- INFERRED: the CLI that pulled on 17 Nov before 16:09 UTC was 2.1.326 or older (2.1.327 came out after it).

## Experiments

- E1, current CLI: one `claude --teleport <id>` on `session_0131Y29YSSRdAPR2VmEG1ET3` with `--debug-file`, after
  `claude --version`. Covers lead 1 if the laptop is on 2.1.331.
- E2, the 17 Nov CLI: the same command through `npx -y @anthropic-ai/claude-code@2.1.326`, also with `--debug-file`.
- Both run from an empty folder under `%TEMP%`, so the copies land under their own `.claude\projects` folder and do
  not mix with the pull's. Debug logs go to `04_assets` (ignored by git).
- Read: exit code, result JSON, and in the logs the teleport lines (the request the CLI makes and the answer). We
  read the CLI's own log; we call no endpoint ourselves.

## Outcomes and what each means

| E1 | E2 | Meaning | Next |
|---|---|---|---|
| fails | loads | CLI regression between 2.1.326 and the current CLI | Comment on #111387 with both logs' teleport lines; decide (Wilco) whether the pull may name a CLI path in Settings |
| loads | any | fixed in 2.1.331 | The daily retry picks the sessions up; check the strip tomorrow |
| fails | fails | service side | Comment on #111387 with the facts; nothing for BurnMon to build |

## Not in this spike

No undocumented endpoint, no private repo checkout, no change to BurnMon code. BurnMon never downloads a CLI.

## Close-out

- HAND, Wilco: run the E1 and E2 snippet; afterwards delete the folder under `%TEMP%` and the copies under
  `C:\Users\WilcoDeTree\.claude\projects\` whose name ends in `bm-teleport-test`.
- DO, Claude: read the logs and write `04_assets\hub_agent_update_2026-11-23_cloud_pull_fix_spike.md`.

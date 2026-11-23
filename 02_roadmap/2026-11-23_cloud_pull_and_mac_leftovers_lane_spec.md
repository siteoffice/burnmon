# Spec: Cowork (Cloud) pull, vendor strip, N6 and N7 (one small lane)

Status: DRAFT at the spec gate, 2026-11-23. Nothing is built. Owner and approver: Wilco de Tree.
Inputs: `C:\ZND\10_holding\04_assets\2026-11-23_to_burnmon_cloud_pull_fails.md`, the Mac findings in
`C:\ZND\50_projects\burnmon\cmd\burnmon-dev\README.md` section 7 (the root README has no section 7), and
`C:\ZND\50_projects\burnmon\04_assets\2026-11-22_mac_check_v0.5.0-beta.2\mac_check_report.md`.

## What I measured now, and what I only inferred

Measured today (2026-11-23, about 12:00 local), by running the exact pull command by hand in
`C:\Users\WilcoDeTree\.burnmon\cloud-pull` (Claude Code 2.1.330):

- The hub chat session `session_0131Y29XwYcLk9RHSQHPeNgR` fails exactly as in the brief:
  `Couldn't load the history of cloud session <id>. Check it at https://claude.ai/code/<id>?from=cli`, exit 1.
- The one session that worked this morning, `session_01AbHCJ1k9V9uEkMguVJ2HPH`, now fails differently: the
  history loads, then the CLI returns JSON with `is_error: true`, `result: "Prompt is too long"`,
  `num_turns: 1`, `total_cost_usd: 0`, exit 1. So it is not "works": it is "loads, but is too big to rename".
- State file `C:\Users\WilcoDeTree\.burnmon\cloud-pull-state.json`: 157 sessions. 67 hold a good copy (all from
  17 Nov), 68 hold an old copy and fail now, 18 were never copied, 4 are gone. Of the 86 failing, 83 say
  "Couldn't load the history", 3 say `Run claude --teleport <id> from a checkout of <owner/repo>` (a session with
  a git repo needs a working folder that is a checkout of it).
- The Claude Code package file `C:\Users\WilcoDeTree\AppData\Roaming\npm\node_modules\@anthropic-ai\claude-code\package.json`
  was changed today at 07:11. Pulls worked on 17 Nov (134 sessions) and failed from 20 Nov. I have no record of
  which CLI version ran on which day.

Inferred, not run:

- `ParseResult` in `C:\ZND\50_projects\burnmon\internal\cloudpull\runner.go` tests `NumTurns > 0` before `IsError`.
  The "Prompt is too long" result has `num_turns` 1 and cost 0, so it would be read as `ErrCost`, which switches
  pulling off for every session with the message "a pull cost tokens". No token was spent. Not yet shown through
  the runner; the first test of the lane must show it.

## 1. Cowork (Cloud) pull

Goal: know why `claude --teleport` fails, fix what is ours, and say plainly what is not.

Diagnosis steps (all read-only, no undocumented endpoints):

1. Wilco opens `https://claude.ai/code/session_0131Y29XwYcLk9RHSQHPeNgR` in the browser. If the page also fails
   to load, the fault is on Anthropic's side or the session is of a kind the CLI cannot teleport.
2. Compare the sessions that failed with "Couldn't load" against the 67 that loaded on 17 Nov, using only what
   is on disk: the entry in `remote-session-spaces.json`, creation date, repo or no repo.
3. Run the same command with the CLI on the version of 17 Nov if the CLI keeps it (`claude --version` then the
   npm version list); if the old version loads the history, the CLI changed. If I cannot get the old version, I
   say so.
4. Dropped by Wilco: no private repo is cloned for the "checkout of" sessions.
5. Wilco's browser check of the session page: it loads (2026-11-23). So the session exists for the account and the
   CLI path fails where the browser does not; this points at the CLI or the account route, not a deleted session.

Verdict rule, one of two, written in the hub brief:

- FIXED: a change on our side makes at least one failing session load, shown by a by-hand run and a state entry.
- CANNOT BE FIXED FROM OUR SIDE: the CLI's documented flag fails for sessions the browser can open, and nothing
  BurnMon controls changes that. Then BurnMon stops hammering and says so on screen (section 2), and the hub
  brief carries the exact command, versions and output for a report to Anthropic (Wilco sends it).

Code changes that are ours whatever the verdict (tests first):

- **A.** `ParseResult`: a result with `is_error: true` and cost 0 is a plain session failure with the CLI's
  `result` text, not `ErrCost`. `ErrCost` stays for cost above 0 or a real model answer. Test with the real
  JSON above as fixture.
- **B.** Circuit breaker: when 5 different sessions in a row fail with the same first error line, the runner
  pauses for 6 hours and records one runner-wide reason, instead of 49 attempts in 25 minutes. A good pull
  ends the pause. (Decision D1.)
- **C.** The "checkout of <repo>" failure is recorded as its own reason ("needs a checkout of <repo>") and
  the session is retried once a day, not every 15 minutes.

## 2. Vendor strip: what "0+" means, and the fix

What `0+` means today: the cell is `NotePartial`. `ReadStatus` in `runner.go` calls the pull OK as soon as any
session has ever been copied (67 have, from 17 Nov), and `Build` in
`C:\ZND\50_projects\burnmon\internal\vendorstrip\vendorstrip.go` then marks the Cowork (Cloud) row `partial`
whenever one session is unread. The page prints the number plus `+`, and the reason lives only in the hover title.
So `0+` says "0 tokens so far, there may be more". It is not a statement that the pull works.

The gap: the README promise (`C:\ZND\50_projects\burnmon\cmd\burnmon\README.md`, "a failing one shows not measured
with the reason") holds only when no session has ever been copied. With 86 of 153 failing, today shows `0+`.

Change (tests first, in `vendorstrip` and the page test):

- `Status` gains `LastPulledAt` (newest `pulled_at` in the state file).
- Per period: if the period starts after `LastPulledAt` and unread sessions exist, the cell says `not measured`
  (same words as the failed case). If the period holds `LastPulledAt`, it stays `N+`. Same logic as the
  existing `cloudNote` for the drop rows. Today and this week would read `not measured`, the month `2743.2M+`.
- The reason is visible, not only a title: one dim line under the strip, "Cowork (Cloud): 86 sessions not
  read. <reason>", from `CloudTip`. Hover text stays. (Decision D2.)
- README text in `cmd\burnmon\README.md` and `cmd\burnmon-dev\README.md` says what the three cell forms mean.

## 3. N6 and N7

- **N6** (lamp row one shows the sentence twice): cause, from the Mac report and the code. `Compute` in
  `C:\ZND\50_projects\burnmon\internal\lamp\state.go` sets the headline `BurnMon is not running` (no full stop);
  `Line` in `lines.go` returns `BurnMon is not running.` (with one). The L3 guard in `row1HTML` of the lamp page
  compares with `line!==st.headline`, so the two differ and both print. Fix: compare after trimming trailing
  `.`, `!`, `?` and spaces, case-insensitive. Test: feed the two real Go strings through the page function.
- **N7** (Control Centre, "Settings. No folder is chosen yet: pic..." cut): `pageSettings` in
  `C:\ZND\50_projects\burnmon\cmd\controlcentre\page.html` writes a 99 character sentence into `.lead`, which is
  `white-space:nowrap; overflow:hidden`. Fix: shorten the lead to "Settings. Choose a folder to show." and
  keep the long help as the pane's own paragraph, so nothing is cut at 1024 wide. Test: the lead text is
  under 60 characters when no root is set. Not read yet: whether `.lead` has a hover title on the Mac.
- D5 (scale on a larger screen) stays as decided 2026-11-22: accepted, looked at again with this lane only if a
  larger Mac screen exists. Not part of the work.

## 4. Lamp "1 your turn" against Waits on you "Nothing waits on you"

Read from code, not yet reproduced. Both the lamp (`lamp.Compute`) and the panel (`secBodyWaits` in
`cmd\burnmon-dev\page.html`) read `wait_items` of a snapshot, and both count `your_turn` as a waiting item (the
panel has a "Your turn" tier). So the code does not intend two answers. The panel says "Nothing waits on you" only
when its `wait_items` list is empty. The likely reasons, in order:

1. They read different snapshots: the lamp reads the snapshot file of whichever writer is running, the panel reads
   BurnMon Dev's own. The Herdr session came in as an agent-view `your_turn` (`waits.go` lines 525 to 540,
   `a.YourTurn()`); if only one of the two had that agent-view state, only one lists it.
2. Timing: the two refresh on different clocks and the session left `your_turn` between them.
3. Your turn rows hidden by a setting (the lamp has a Foreman switch; the panel's rule is not read).

Step: reproduce with a Herdr session in `waiting`, capture the snapshot file and the panel's list in the same
minute, and report which of 1 to 3 holds. Verdict in the brief: MEANT (then say so in the README, one sentence) or
BUG (then fix in this lane if it is one change, otherwise ASK).

### 4a. Added at approval: the hook phase as a cause (Wilco, 2026-11-23)

At 11:58 the lamp said "1 your turn" and Fleet said "main thread, waiting for you" while this session ran its
UserPromptSubmit hook and Herdr showed it as Working. Read from code: `AgentEntry.YourTurn()` in
`C:\ZND\50_projects\burnmon\internal\waits\agentview.go` is true for `status == "idle"` with no `waitingFor`, taken
from `claude agents --json`. If Claude Code reports `idle` while a hook runs (the turn has ended, the next
has not started), BurnMon makes a `your_turn` row from it. Measured now: `claude agents --json` lists this session as
`busy` between hooks. Not measured: the value during a hook. Test: a fixture entry `idle` plus the poller's
`Since` rule; then a live read of `claude agents --json` in a loop while a slow UserPromptSubmit hook runs. Fix if
confirmed: an `idle` entry counts as your turn only after it was idle on 2 polls in a row (the poller already
tracks `Since`). The lamp and the panel share the rule, so one change covers both.

## 5. Added at approval: the pull's own runs must not be sessions (Wilco, 2026-11-23)

Fleet listed "cloud-pull-d5, no Fleet here, busy" under Other sessions. Fleet is Groundwork's; its code is not in
this repo, and BurnMon never reads a session name (`agentview.go` says so), so BurnMon did not produce that name.
Read from code: every pull starts `claude` with cwd `C:\Users\WilcoDeTree\.burnmon\cloud-pull`, and BurnMon's own
agent view read (`claude agents --json`) has no cwd filter, so a pull that is running would appear as an entry.
`claude agents --json` showed no pull entry at the moment I read it (the pull was not running), so this is
inferred, not proven. Steps: (1) a live read of `claude agents --json` during a pull to see whether an entry with
that cwd appears and what status it has; (2) if yes, drop entries whose cwd is `cloudpull.PullDir()` in
`ParseAgentView` consumers (waits, sessions list, snapshot `agent_sessions`), tests first; (3) say in the hub brief that
Fleet's own list is Groundwork's to filter, with the cwd as the rule to give them. The name "cloud-pull-d5" stays
unexplained until step 1: it may also be a worktree or terminal label unrelated to the pull.

## Decisions I need from you at the gate

- **D1** Circuit breaker: 5 same errors in a row, 6 hour pause. Recommended. Pro: stops 49 CLI starts in 25 minutes
  (each costs about 10 s, 8 CPU s, 650 MB). Con: a real session that would load waits up to 6 hours.
- **D2** Visible reason line under the strip. Recommended. Pro: the README promise is kept on screen. Con: one more
  line in a dense strip. Alternative: title only (today).

## Verification (after approval)

Tests first. Then, in PowerShell, cwd `C:\ZND\50_projects\burnmon`: `go vet ./...`, `go test ./... -count=1`,
`.\build.ps1`, and `.\scripts\uicheck.ps1` for the page changes. Say what was measured and what only inferred.
Commits, tags and pushes stay Wilco's step. Hub brief into `C:\ZND\50_projects\burnmon\04_assets` at the end.

## Close-out

- DO done: both by-hand pulls, state file read, code read for sections 1 to 4.
- ASK answered: D1 (the breaker) and D2 (the visible reason line) -> both yes, Wilco, 2026-11-23
- DROP dropped: step 4 of section 1 (clone a private repo for the checkout test) because Wilco ruled it out
- HAND handed: open the session URL in the browser (section 1, step 1), verified by Wilco's reply that the page loads
- HAND parked: delete the 12 MB copy my by-hand pull of `session_01AbHCJ1k9V9uEkMguVJ2HPH` left (not in the state file); deleting is Wilco's under the Groundwork guard

```powershell
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon
Set-Location C:\ZND\50_projects\burnmon
Remove-Item -LiteralPath "C:\Users\WilcoDeTree\.claude\projects\C--Users-WilcoDeTree--burnmon-cloud-pull\81ac3b9c-5783-4435-a76e-2a9d17b950d7.jsonl"
```

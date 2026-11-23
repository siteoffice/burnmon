# Spec: Cowork (Cloud) strip honesty, one small lane

Status: approved by Wilco, 2026-11-23 ("do both lanes: honesty and trying to fix"). Nothing built yet. Base: `main` at `4850b6a` (pull request #74), read live.
Owner and approver: Wilco de Tree.

## Why

On the build of `4850b6a`, BurnMon Dev shows (Wilco's screenshot, 2026-11-23 22:22 local) Cowork (Cloud) today `0+`,
week `0+`, and under the strip: "91 sessions are not read: 88 are not found by the Claude Code service for your login,
3 failed: exit status 1: Run claude --teleport session_01G9gfX... from a checkout of w." Two faults and one risk.

## What I measured, and what I only inferred

Measured (read 2026-11-23, `C:\Users\WilcoDeTree\.burnmon\cloud-pull-state.json` written 21:23:45 UTC, and the code at `4850b6a`):
- 165 sessions: 91 "Couldn't load", 3 "from a checkout of", 4 gone, 67 without error. 135 have a copy.
- The 3 checkout entries carry only `failed_at`, `failures`, `last_error`, `last_usage`, `pulled_at`: no
  `needs_checkout`. The runner of `4850b6a` sets `NeedsCheckout` only when it pulls such a session again, so these
  entries come from the older runner.
- `reasonOf` (`internal\cloudpull\runner.go`) sends them to `ReasonOther`, which cuts the text at 90 runes. The three
  texts are 112 to 124 characters after the prefix, so the cut lands inside the repo name: "checkout of w".
- `Status.LastPulledAt` is the newest `PulledAt` of any copied session: 2026-11-23 17:40 UTC, a re-copy of
  `session_01AbHCJ1k9V9uEkMguVJ2HPH`, whose `last_usage` is 2026-08-05. `vendorstrip.go` marks a period `NotePartial`
  (`N+`) when it does not start after that time. So today and this week show `0+`.
- The newest usage among all copies is 2026-11-17 16:01 UTC. Measured by that, today and this week hold no measured
  cloud usage (not measured) and November holds some (`N+`).
- The other two checkout entries name `zero-nonsense-dev/burnrate-private` and an outside organisation's repo. A cut
  at another length would put either on screen.

Inferred: the 3 entries would reclassify themselves at their next daily retry; the screen is wrong until then, and
again for any entry the older runner wrote. Not checked: whether 88 against 91 not found is the minute between the
screenshot and the state write.

## The fix

1. Classify on the words, not only the flag. `reasonOf` returns `ReasonCheckout` when `NeedsCheckout` is set OR
   `checkoutRepo(LastError)` matches. `Reason.Plain()` for checkout stays "N need a checkout of their repo": no repo
   name on screen.
2. No repo or organisation name in an `other` reason. Before the 90-rune cut, `reasonOf` removes any
   "from a checkout of <repo>" tail and any "session_<id>" token, so the line keeps the CLI's words without names
   or ids. The compare link keeps its one session id (it is the user's own page).
3. Anchor "not measured" on usage, not on copy time. `Status` gets `LastMeasured`: the newest `LastUsage` over
   sessions with a copy (`Path` set), `LastUsage` already ignoring the CLI's synthetic zero-token rows (pull request
   #74). `vendorstrip.go` uses `LastMeasured` where it used `LastPulledAt` for the notes. `LastPulledAt` stays for
   the runner. With today's state: today and week read `not measured`, month reads `2743.2M+`.
4. README, `cmd\burnmon\README.md` "Cowork (Cloud) sessions": "the period holds the newest good copy" becomes "the
   period holds the newest cloud usage that was copied", same for "not measured".

Out: the teleport fault itself (anthropics/claude-code #111387), the runner's retry timing, the Fleet filter, any
second cloud route (grill note `04_assets\2026-11-23_cloud_usage_route_architecture.md`).

## Tests first

- `reasonOf` on an entry with only `last_error` "...Run claude --teleport session_X from a checkout of org/repo."
  returns `ReasonCheckout`; `Plain()` has no "org" and no "/".
- An `other` reason built from a text with "from a checkout of org/repo" and a session id contains neither.
- `ReadStatus` on a state with a fresh re-copy of an August session and newest real usage 17 Nov gives
  `LastMeasured` 17 Nov, `LastPulledAt` today.
- `vendorstrip` with that status on 23 Nov: today and week `NoteNotMeasured`, month `NotePartial`; the total row keeps `+` in every period (measured
  numbers plus an unknown cloud part), as today.
- The tip text for the screenshot's state contains "3 need a checkout of their repo" and no "checkout of w".

## Verification

PowerShell, cwd `C:\ZND\50_projects\burnmon`: `go vet ./...`, `go test ./... -count=1`, `.\build.ps1`, and
`.\scripts\uicheck.ps1` for the strip (case `d52` covers the reason line). Then BurnMon Dev restarted and the line
read against the state file. Say what was measured and what only inferred. Commits, tags and pushes are Wilco's.

## Close-out

- DO done: state file and code read, this spec.
- HAND, Wilco: approve or change this spec; the lane runs in a Claude Code session on a work branch from `main`.

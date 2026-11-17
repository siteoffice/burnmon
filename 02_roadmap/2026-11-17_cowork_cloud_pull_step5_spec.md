# Cowork cloud pull, step 5: the switch, the screen and the README

Flow `burnmon/cowork-cloud-pull`, step 5 of 7. Parent spec: section 13 ("Screen", "README") of
`C:\ZND\10_holding\04_assets\2026-11-17_burnmon_cowork_cloud_gap_research_and_proposal.md`.
Date: 2026-11-17. Status: spec, awaiting the spec gate.

## What the code does today (read 2026-11-17)

- burnmon.exe: one settings overlay built in Go (`cmd\burnmon\app.go` `settingsModalHTML`), saved with
  `writeConfigKey` into the loaded `burnmon.json`. No boolean switch in it yet.
- BurnMon Dev: gear panel `#secSettings` in `cmd\burnmon-dev\page.html`, checkboxes saved to its own
  `burnmon-dev.json` (for example `toast_enabled`, read again every tick). It reads `burnmon.json` but
  never writes it.
- The marker: `internal\vendorstrip` sets `not_measured` or `partial` on the Cowork (Cloud) row from the
  old plugin's drop files; both pages show `not measured` or a `+` with the tooltip
  "measured until <time>, later cloud sessions are not measured".
- Nothing starts the step 3 runner. Both apps can be open at once; the snapshot writer already shares
  one job between them with a file lock (`internal\snapshotwriter\lock.go`).

## What changes

1. **One key, one file.** `cloud_pull` (true or false, default false) in `burnmon.json`, the file both
   apps read. `writeConfigKey` moves to `internal\pricing` as `WriteConfigKey` so both apps write it
   the same way; BurnMon Dev writes the `burnmon.json` it loaded, else `%LOCALAPPDATA%\burnmon\burnmon.json`.
2. **The switch, in both settings menus.** A checkbox "Read Cowork (Cloud) sessions", off by default,
   with these lines under it:
   - Starts your Claude Code CLI to copy your cloud sessions to this computer.
   - Needs Claude Code installed and signed in to the same account.
   - Reads cloud sessions you start in the Claude app on this computer. Sessions started from your
     phone, the browser or another computer are not read, and neither is subagent usage inside a
     cloud session.
   burnmon.exe: in `settingsModalHTML`, saved with the existing Save. BurnMon Dev: in `#secSettings`,
   saved at once through a new binding `bdevSetCloudPull`, like the toast switch.
3. **One runner for both apps.** Each app starts a supervisor goroutine that reads `cloud_pull` every
   30 s. When it is on, the app that holds `~\.burnmon\cloud-pull.lock` (the snapshot writer's lock type,
   a second file) runs one runner pass per tick; the other app waits and takes over when the first
   quits. When it goes from on to off, the supervisor stops pulling, releases the lock and clears a
   cost-guard stop, so switching on again starts clean. The runner's `Loop` body becomes `Step`, one
   pass, so the supervisor drives it.
4. **A session that is gone stays gone.** A pull whose CLI output says "Session not found" marks the
   entry `gone`: never pulled again, not a feature error.
5. **The marker follows the switch.** `internal\vendorstrip` gets the switch and the runner state:
   - Off: today's `not measured` or `+`, and the tooltip ends with
     "Turn on Read Cowork (Cloud) sessions in Settings to read them."
   - On, and the last pass was good (no stop, no error): plain numbers, no `+`, no note.
   - On, and it failed (no CLI, not signed in, no session list, a cost-guard stop):
     `not measured`, tooltip "Cowork (Cloud) not measured: <reason>".
   Both pages read the same payload fields; the Total row follows as today.
6. **README.** `cmd\burnmon\README.md`: a section "Cowork (Cloud) sessions" with the switch, the three
   facts and what is not read; `cloud_pull` in section 5; the CLI added to the list of optional
   programs BurnMon starts; next to every "no network" line: "BurnMon itself makes no network call.
   With Read Cowork (Cloud) sessions on, it starts your own Claude Code CLI, which goes online as you."
   The same honest line in `README.md` and `cmd\burnmon-dev\README.md`. `burnmon.example.json` gets the
   key with a comment.

## What stays

The pull schedule, the cost guard, the ingest of step 4, every other setting, the old plugin and its
drop parser (step 6 reports on it). No pull runs until someone turns the switch on.

## How to check

- Tests first: `WriteConfigKey` keeps other keys; supervisor (fake config, fake runner, real lock in a
  temp folder): off runs nothing, on runs one pass per tick in one app only, off releases the lock and
  clears a stop, the second app takes over; `gone` on "Session not found" and `Due` skips it; vendorstrip
  notes and tooltips for off, on-good and on-failed; Dev binding list and page strings; doccheck passes
  with the new key and binding.
- uicheck: the existing d49 updated for the new tooltip; one new Dev case that opens the gear, finds the
  switch with its text, and paints the strip in the on-good state (plain number, no `+`).
- Live, once, on this laptop: turn the switch on in BurnMon Dev built from this branch, wait one tick,
  and see the hub session as Cowork (Cloud) with tokens; turn it off and see `not measured` again.
  The real BurnMon is closed for this (it holds the store); the result is reported with times.
- `go vet ./...`, `go test ./... -count=1`, `.\build.ps1`, `.\scripts\uicheck.ps1`.

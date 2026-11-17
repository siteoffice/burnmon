# Cowork cloud pull, step 4: read pulled rows as Cowork (Cloud)

Flow `burnmon/cowork-cloud-pull`, step 4 of 7. Parent spec: section 13 items 4 and 5 of
`C:\ZND\10_holding\04_assets\2026-11-17_burnmon_cowork_cloud_gap_research_and_proposal.md`.
Date: 2026-11-17. Status: spec, awaiting G2.

## What the code does today (read 2026-11-17)

- `internal\adapter\claude\claude.go` names a session after its file and labels every row under
  `~\.claude\projects` as `claude-code`. A pull file is therefore a Claude Code session with the
  local pull id. Its first `cwd` is the cloud container's (`/ccs-homes/...`); later rows say
  `C:\Users\<user>\.burnmon\cloud-pull`. Every row has entrypoint `sdk-cli`.
- The store keys rows on `(vendor, session_id, request_id)`. A second pull of the same cloud
  session under a new local id is a second set of rows: double counting.
- Deleted transcripts: with `keep_history` (the default) rows stay and only the cursor goes. Without
  it, rows go by the session id taken from the file name. This answers the question from step 2:
  BurnMon keeps a session's rows when its file is deleted (default setting).
- The old cloud plugin's drop rows (`cowork-cloud`, 8,911 rows, 2026-10-12 to 2026-11-12) are keyed
  on the cloud container's own session id or `agent-<id>`, with the same API request ids a pull
  carries. A pull of a session from that period would count those requests twice.

## What changes

1. **Pull files are Cowork (Cloud).** A transcript inside the pull folder's project folder
   (`cloudpull.ProjectDir()`) is parsed by the normal Claude parser, then: agent `cowork-cloud`,
   surface `cloud`, session id = the cloud session id from `~\.burnmon\cloud-pull-state.json`
   (looked up by the file's local id), project empty (a cloud session has no local folder, as the
   drop rows), whatever the entrypoint says. The same for its tool calls.
2. **Not yet in the state file:** the parser returns no rows and keeps offset 0, so the file is read
   again on the next pass, after the runner has saved its state. No row is ever stored under a local
   pull id.
3. **One request, one row.** A pulled row is a `cowork-cloud` row whose session id starts with
   `session_`. When the store writes one, it deletes every other `anthropic` row with the same request
   id (an earlier teleport copy stored as Claude Code, or an old drop row). When it writes any other
   `anthropic` row whose request id already has a pulled row, it skips it. So re-reading an old drop
   file or a teleport copy cannot bring the duplicate back. A new index on `(vendor, request_id)`
   keeps these lookups fast.
4. **Clean-up of what already landed as Claude Code.** On every collect, like `PurgeAuditSessions`:
   delete the events and tool calls of sessions whose cursor path lies inside the pull folder's project
   folder and that are not `cowork-cloud`, and their cursors, so the files are read again under item 1.
   A no-op once clean. On this laptop it removes the rows the six step 2 and step 3 pull files left as
   Claude Code.

## What stays

The Claude Code parser for every other folder, the drop parser, the screens, the `not measured` marker
(step 5). The three teleport-test files in `...-AppData-Local-Temp-teleport-test` stay Claude Code
until a pulled row with the same request id arrives (item 3 then removes their copies), and are
deleted in step 7.

## How to check

- Tests first: a pull file mapped through a state file becomes `cowork-cloud` with the cloud id and an
  empty project; a pull file missing from the state gives no rows and offset 0; two pulls of one cloud
  id give one row per request; a pulled row replaces a Claude Code copy and a drop row with the same
  request id, and a later drop or Claude Code row with that id is skipped; a Claude Code file outside
  the pull folder is unchanged; the purge removes only sessions under the pull folder; the index exists
  after migration.
- Measured on the laptop with the built `burnmon-cli.exe` against a copy of `burnmon.db` in the
  scratchpad (never the live store): before and after, rows per agent; the hub session
  `session_0131Y29YSSRdAPR2VmEG1ET3` as one `cowork-cloud` session with its token totals; no row left
  under a local pull id.
- Subagent coverage: whether a pull carries a session's subagent requests is not known (the hub pull
  has no sidechain rows). Pull the newest list entries before the hub session until one with drop rows
  is found (at most three pulls), and compare its request ids and tokens with the drop rows. Report the
  result; a gap is a finding for step 5, not a blocker here.
- `go vet ./...`, `go test ./... -count=1`, `.\build.ps1`. No UI change, no `uicheck`.

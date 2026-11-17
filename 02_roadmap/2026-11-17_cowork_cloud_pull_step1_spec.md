# Cowork cloud pull, step 1: the session list reader

Flow `burnmon/cowork-cloud-pull`, step 1 of 7. Parent spec: section 13 of
`C:\ZND\10_holding\04_assets\2026-11-17_burnmon_cowork_cloud_gap_research_and_proposal.md`.
Date: 2026-11-17. Status: spec, awaiting G2.

## What changes

A new package `internal\cloudpull` with three pure functions and their tests. No exec, no network,
no screen change. Later steps call them.

1. `DesktopDataRoots() []string`: the Claude desktop app's data folders that exist on this computer.
   Adds `%LOCALAPPDATA%\Claude-Data\claude_*` (where the app keeps its data on this laptop since
   2026-08-18) to the folders `internal\scan` already knows. Used only by this package for now; the
   scan gap for local Cowork is a separate flow.
2. `ParseAuthOrg(statusJSON []byte) (orgID string, err error)`: reads `orgId` from the output of
   `claude auth status`. Error when not logged in or the field is missing.
3. `SessionList(roots []string, orgID string) ([]string, error)`: finds
   `local-agent-mode-sessions\<account>\<orgID>\remote-session-spaces.json` under the roots, reads
   `entries[].sessionId`, and returns the unique ids that start with `session_`, in file order.
   A missing file returns a typed `ErrNoList`. A list for another org is never read.

## What stays

The scanner, the store, the ingest and every screen. The `not measured` marker stays as it is.

## Coverage of the list (measured 2026-11-17)

| Where the cloud session was started | In the list? | Evidence |
|---|---|---|
| Claude desktop app on this computer, inside a Project | yes | 10 of 141 entries carry a `spaceId` that `spaces.json` lists |
| Claude desktop app on this computer, outside any Project | yes | 131 entries have no `spaceId`, the hub session `session_0131Y29Y...ET3` among them |
| Phone app, a Cowork task (Wilco, about 11:05 local) | no | file unchanged (141 entries, written 08:46:04); session gives a `claude.ai/chat/...` link |
| Claude Code on the web, `claude.ai/code` (Wilco, about 11:09 local) | no | `session_0131Y29Y8wD9...` not in the file |
| Claude desktop app on another computer | not tested | |

Second source for the missing ones: none found.
- No file under `%LOCALAPPDATA%\Claude-Data` or `%USERPROFILE%\.claude` written after 10:00 names
  the phone or web sessions. The only hits were this session's own transcript, prompt history
  and fleet note, which carry the pasted links.
- The CLI has no listing that works without a person: `claude agents --json` lists local sessions,
  `--teleport` without an id opens an interactive picker. An undocumented web endpoint is ruled out
  by the project rules.

So the feature reads cloud sessions started in this computer's Claude app. It does not see cloud
sessions started from the phone, the browser or another computer.

Settings text, under the switch "Read Cowork (Cloud) sessions" (replaces the two lines in section 13):
- Starts your Claude Code CLI to copy your cloud sessions to this computer.
- Needs Claude Code installed and signed in to the same account.
- Reads cloud sessions you start in the Claude app on this computer. Sessions started from your
  phone, the browser or another computer are not read.

The same three lines go into the README section "Cowork (Cloud) sessions".

## How to check

- Tests first: fixtures under `internal\cloudpull\testdata` with made-up ids and folders. Cases: list
  for the matching org, list for another org ignored, missing file gives `ErrNoList`, broken JSON
  gives an error, empty and duplicate ids skipped, file order kept, auth status logged out.
- One test on the real laptop file, skipped when the file is absent: returns 141 or more ids and
  includes `session_0131Y29YSSRdAPR2VmEG1ET3`.
- `go vet ./...`, `go test ./... -count=1`, `.\build.ps1`: baseline lines plus the new package.
  No UI change, so no `uicheck`.

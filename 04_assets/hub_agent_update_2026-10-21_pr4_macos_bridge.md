# Hub Agent Update - ZeroNonsense.dev

**Date:** 2026-10-21 - **Owner:** Wilco de Tree
**Project:** BurnMon
**Purpose:** BurnMon's public "no server, no ports" promise now holds on Windows only; Wilco accepted a localhost bridge for macOS and Linux from outside PR #4, merge pending on three conditions.
**Read order:** this file, `C:\ZND\50_projects\burnmon\02_roadmap\2026-10-21_pr4_macos_bridge_decision.md`
**Supersedes:** nothing

## 1. Headline
Update, later 2026-10-21: PR #4 is merged (`origin/main` `ce85753`), and the follow-up (per-run secret and README text) is built on branch `fix/pr4-bridge-secret` under a third park exception; not tagged. Original headline: decided, not merged: PR #4 (merkendiep) adds a 127.0.0.1 web server to `burnmon` on macOS and Linux so the browser dashboard gets live data. Accepted on three conditions: a per-run secret, plain README text, a Windows verify after the park ends 2026-11-01.

## 2. What changed on disk
- **Committed:** nothing.
- **On a branch, not merged:** PR #4, `merkendiep:macos-support`, head `3292cb1`, base `2e20ebb` (5 commits, in the contributor's fork).
- **Files touched** (uncommitted in `C:\ZND\50_projects\burnmon`): `C:\ZND\50_projects\burnmon\02_roadmap\2026-10-21_pr4_macos_bridge_decision.md` (new), `C:\ZND\50_projects\burnmon\02_roadmap\roadmap.md` (row 10a marked replaced), `C:\ZND\50_projects\burnmon\SESSION_LOG.md` (new top entry), this brief.

## 3. What did NOT happen (and why)
Not merged, not tagged, no build from this PR on Windows. The PR comment asking for the conditions is drafted, not posted (posting as ZND is Wilco's step). No run of `.\build.ps1` or `.\scripts\uicheck.ps1`. The PR's GitHub checks were not readable from this session. Nothing on macOS or Linux was run by us; the macOS run is the contributor's claim. No park exception was used: the merge waits until after 2026-11-01.

## 4. Findings worth propagating
- [RESULT] On the PR head in a Linux container: `go vet ./...` clean, `go test ./... -count=1` green in every package; `GOOS=windows` `go vet` and `go build` clean; `GOOS=darwin GOARCH=arm64 go build` clean.
- [RESULT] The PR's README still says "no server" (opening paragraph) and "No server, no network call of any kind" ("What BurnMon never does") while adding a port. Corrected text is in the decision file.
- [STATE] Three bug fixes ride along: Copilot CLI NULL token columns (also affects Windows), arm64 context runway, cross-platform tests. Pending the same merge.
- [STATE] Mac and Linux get the full live dashboard once merged; until then browser mode stays a static report.

## 5. Hub-level decision
  **Decision:** BurnMon may open one local port (127.0.0.1) on macOS and Linux only, for the browser dashboard. Windows keeps no server and no open port. The README says so in the opening paragraph and in "What BurnMon never does".
  **Context:** Outside PR #4 (merkendiep) found the macOS browser dashboard never loads its live sections, because the live functions exist only inside the Windows WebView2 window. His fix is a localhost bridge. BurnMon's public positioning was "no network, no server, no ports".
  **Alternatives considered:** Reject the bridge and do roadmap row 10a (hide the Refresh button), keeping browser mode a static report and the promise intact everywhere.
  **Consequences:** The "no ports" claim becomes platform-scoped in public material. Merge needs a per-run secret (localhost is shared by all users on a machine), the README text, and a Windows verify after 2026-11-01. More code to maintain on platforms Wilco does not run.
  **Links:** `C:\ZND\50_projects\burnmon\02_roadmap\2026-10-21_pr4_macos_bridge_decision.md`, https://github.com/siteoffice/burnmon/pull/4

## 6. What the next hub read should update
`C:\ZND\10_holding\03_logs\decisions.md` (block above), `C:\ZND\10_holding\01_projects\burnmon.md` (open item: PR #4 merge after the park). STATUS: new top paragraph, already written in the project. DEADLINES: no change.

Tracker rows moved: none.

## 5b. Park exception
Third park exception (Wilco, 2026-10-21): the per-run secret for the macOS and Linux bridge, so `main` does not carry an unprotected port until 2026-11-01.

## 7. Open flags for next session
Superseded the same day: no PR comment, Wilco merged PR #4 after a Windows verify (go vet, go test, build.ps1 green; uicheck w1 failed on the PR and twice on `main` `2e20ebb`, so not a regression). Open: commit, push and merge of `fix/pr4-bridge-secret` (Wilco), then a tag decision. Linux still untested on real hardware. STATUS.md got a new top paragraph.

## 8. Related files
`C:\ZND\50_projects\burnmon\02_roadmap\2026-10-21_pr4_macos_bridge_decision.md`, `C:\ZND\50_projects\burnmon\02_roadmap\roadmap.md` row 10a, `C:\ZND\50_projects\burnmon\SESSION_LOG.md`.

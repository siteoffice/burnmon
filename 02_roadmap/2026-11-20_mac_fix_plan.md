# Mac fix lane: plan file and turn log

Lane file: `C:\ZND\10_holding\02_roadmap\2026-11-20_mac_fix_lane.md`. Branch: `work/wilco-de-tree/mac-fix-lane`.
Starts from: `main` at `d44bd33` (v0.4.0-beta.9). Run mode: Unattended.

## Baseline (T1, on d44bd33)

- `go vet ./...`: exit 0 (measured).
- `go test ./... -count=1`: the baseline run overlapped my first edits, so it is not clean. The last clean figure
  is the beta.9 line of `C:\ZND\50_projects\burnmon\STATUS.md`: 50 packages ok, 0 FAIL (read, not re-measured on d44bd33).
  The numbers after T1 are in the T1 turn log line.

## T1: D9, the loading screen

Cause found (read in code, not seen in a window):

1. `C:\ZND\50_projects\burnmon\cmd\burnmon\main.go` (Windows and Mac native window) reports a failed first
   collection with `w.Eval("ccWarmFail(...)")` into the page that `w.SetHtml(warmingPageHTML())` queued. Eval
   evaluates asynchronously and is lost when the page has not loaded. With no source folder the collection
   fails at once (`dataset.ErrNoSources` is returned before any file is read), so the Eval runs before the
   first page has loaded. This is inferred from the code and the Mac log ("initial collection failed" and then
   nothing); it was not reproduced in a window.
2. After a failure the goroutine returned. Nothing looked again until the 15 minute tick, and a success of that
   tick only calls `ccReload`, which reloads a `SetHtml` page and does not navigate to `dashboard.html`. So a
   session folder that appeared later never brought the dashboard up without a restart.
3. Browser mode (`C:\ZND\50_projects\burnmon\cmd\burnmon\main_other.go`, used by Linux and by a Mac build
   without cgo) already delivered the text (`setStartErr`, `/api/built`, `warmingPollScript` calls `ccWarmFail`
   every second), but it also never retried.
4. The Windows window has the same gap as the Mac window: same code, same Eval, same missing retry. The Windows
   race is less likely only because a Windows machine usually has `%USERPROFILE%\.claude`.

Repair:

- `C:\ZND\50_projects\burnmon\cmd\burnmon\warming.go` (new): `firstCollection` runs the first collection until it
  succeeds (every 30 s), shows each changed failure text once, waits out a busy rebuild; `sourceFoldersLine(goos)`
  names the folders per operating system; `warmingFailPageHTML(msg)` is the warming page with the text in place of
  "Starting...".
- `main.go`: the native window now calls `w.SetHtml(warmingFailPageHTML(msg))` (a new page, no race) and the
  loop keeps looking; on success the code that was already there navigates to the dashboard.
- `main_other.go`: the same loop; the text goes to `setStartErr`, cleared on success.
- `app.go`: the message for `ErrNoSources` says no session folders were found, that BurnMon keeps looking, and
  ends with the folders line; the text element keeps its line break (`white-space:pre-line`).
- `C:\ZND\50_projects\burnmon\internal\dataset\dataset.go`: the text of `ErrNoSources` names Claude Code,
  Cowork, Codex and the other supported tools.

Not changed, named: the CLI text for the same error in `cmd\burnmon-cli\main.go` (Claude only; the lane names
the window, not the CLI). The live watcher is seeded with the roots found at start, so a folder that appears
later is read by the collection, not by the live view, until a restart (inferred from `startLiveWatch`).

Tests (`C:\ZND\50_projects\burnmon\cmd\burnmon\warming_test.go`, no build tag, so they run on Windows, Mac and
Linux): written first and seen failing (the package did not compile: `sourceFoldersLine` undefined); pass now.
Not testable here: a real window, and the browser bridge test file (`browser_test.go` is tagged off Windows and
was not changed).

## Turn log


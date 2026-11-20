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

## T2: D1, the lamp crash

Step 1, the inferred cause checked against the stack in
`C:\ZND\50_projects\burnmon\04_assets\2026-11-19_mac_test\maccheck-full\lamp-demo.log` and the webview_go
header (`webview.h`, module `github.com/webview/webview_go` at `v0.0.0-20240831120633-6173450d4dd6`). What holds:

- Read: webview_go makes a plain `NSWindow`, sets the `WKWebView` as its content view and calls
  `makeKeyAndOrderFront:` before `webview.New` returns. So the web view is in the window, and observes it, before
  `lampStyle` runs.
- Read: `lampStyle` made a run-time subclass of `[w class]` (`BurnMonLampPanel`) and swapped the window to it with
  `object_setClass`, then called `setStyleMask:`. The stack runs `setStyleMask:` → `removeFromSuperview` of the
  content view → `WKWindowVisibilityObserver stopObserving` → `removeObserver:forKeyPath:` on the window, which
  throws "not registered as an observer" for `<BurnMonLampPanel>`.
- Inferred, not proven inside AppKit: the observation was registered against the window's class before the swap
  (key-value observing swaps the class of an observed object, and `[w class]` hides that), so after the swap the
  registration is not found. The swap is the trigger in every reading.

Step 2, proof on a real Mac runner (measured): `C:\ZND\50_projects\burnmon\scripts\maccheck.sh --ci` now runs the
`lamp-demo` step, and in CI mode prints the head and end of a failed step's log (CI keeps no files). Commit
`c72cfcb`: the `macos` check went red (run 44762558000) with a crash in `lamp-demo`, but the 40 line tail held only
registers. Commit `7f55ca0` prints the head too: run 44763572057 red with
`NSRangeException ... Cannot remove an observer <WKWindowVisibilityObserver ...> for the key path
"contentLayoutRect" from <BurnMonLampPanel ...>`, thrown in `-[NSWindow setStyleMask:]` from `lampStyle`. So a
window can open on the `macos-latest` runner (macOS 26.7.2, arm64), and D1 reproduces there. To save the script
change alone, the uncommitted lamp fix was taken out by Edit and put back after the save (no stash, no scratch).

Step 3, the repair, in `C:\ZND\50_projects\burnmon\cmd\burnmon-dev\lamp_window_darwin.go` only: no class swap, no
run-time class, no added method. "Never key, never main" now comes from the borderless style: AppKit documents that
`canBecomeKeyWindow` and `canBecomeMainWindow` are NO for a window without a title bar or resize bar. The lamp logs
a line if a macOS says otherwise. Every other property is unchanged (borderless, `NSStatusWindowLevel`, all Spaces,
accessory policy so no Dock icon, clear or card backdrop). The Windows lamp is not touched. Chosen over "make the
panel before the web view is attached" because webview_go makes window and web view in one call, and over "detach,
swap, reattach" because any other observer of the window would still break.

Known and unchanged (reviewer, inferred): webview_go makes the window key before `lampStyle`; the old overrides did
not undo that either. Built, not seen: the panel was not looked at; a green check means "ran for 6 seconds on a Mac
runner".

Tests: `TestLampDarwinWindowFileSwapsNoClass` in `C:\ZND\50_projects\burnmon\cmd\burnmon-dev\buildtags_test.go`
(reads the file as text: no `object_setClass`, `objc_allocateClassPair`, `class_addMethod`, `class_replaceMethod`,
`method_setImplementation`), seen failing on the old file, passes now. Changed test, named:
`TestMaccheckScriptShape` in `C:\ZND\50_projects\burnmon\internal\repocheck\maccheck_test.go` now expects
`lamp-demo` among the steps and not among the window steps, because the lane moves it into CI mode.

## Turn log

T1: done; commit 394bd2b; measured: go vet 0, go test 50 packages ok and 0 FAIL, build.ps1 0, linux and darwin build and vet 0 with CGO off; open: nothing for D9 (built, not seen in a window; the retry and SetHtml fix is inferred from code, the 6 new tests in cmd\burnmon\warming_test.go failed first as a compile error)
T2: done; commit 51c6a9a; measured: go vet 0, go test 50 packages ok and 0 FAIL, build.ps1 0, linux and darwin build and vet 0 with CGO off, macos check red with the D1 NSRangeException on 7f55ca0 (run 44763572057) and green with lamp-demo ok on 51c6a9a (run 44764868390); open: nothing for D1 (ran for 6 seconds on a Mac runner, not seen; the log line that would say the borderless panel can still become key or main is not printed by CI on a pass, so the documented default is not measured)

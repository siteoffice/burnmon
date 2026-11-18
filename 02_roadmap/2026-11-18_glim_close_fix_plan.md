---
type: plan
title: Glim close fix, plan and turn log
created: 2026-11-18
lane: C:\ZND\10_holding\02_roadmap\2026-11-18_glim_close_fix.md
branch: work/wilco-de-tree/lamp-glim-1117
---

# Glim close fix, plan and turn log

Run mode: Unattended. One turn, T1. Started from commit `d56b906` on `work/wilco-de-tree/lamp-glim-1117`
(tracked tree clean, no stash). The Git lane guard was checked: a raw `git push --dry-run` was denied.

## Baseline

On `d56b906` before any change: the two new window tests (below) were run against the unrepaired
window code and both failed. The full suite was not run on the untouched tree; the last measured
state is in `C:\ZND\50_projects\burnmon\STATUS.md` (50 packages ok at the end of the fix lane).

## The cause, as read

Read in `C:\Users\WilcoDeTree\go\pkg\mod\github.com\jchv\go-webview2@v0.0.0-20260205173254-56598839c808\webview.go`:

- `Terminate()` (line 381) is `PostQuitMessage(0)`. That Win32 call posts WM_QUIT to the message queue
  of the thread that calls it.
- `Run()` (line 353) is a `GetMessageW` loop and returns only when it receives WM_QUIT itself.
- `Dispatch(f)` (line 443) queues f and posts a thread message to `w.mainthread`; `Run` runs the
  queue on that thread.
- `cmd\burnmon-dev\main.go` locks the main goroutine to its OS thread, and `Run` runs there.

So the suspicion in the lane file was right: the lamp called `Terminate` from a goroutine in two
places (`lampClose` through `go lampCloseNow(...)`, and the 2 second parent watcher). The quit message
went to another thread and the window loop never ended. Read from source; the failure was then
reproduced by the two window tests.

## What changed

| File | Change |
|---|---|
| `cmd\burnmon-dev\lamp_child.go` | `lampEnd` (terminate inside `Dispatch`, then `os.Exit(0)` after 3 s as a second line), `lampEndGrace`, `(*lampChild).Tick` (stops a child while the saved `lamp_enabled` is false) |
| `cmd\burnmon-dev\lamp_window_windows.go` | both end paths call `lampEnd` |
| `cmd\burnmon-dev\lamp_window_darwin.go` | the same two calls (built, not seen; cgo, not compiled here) |
| `cmd\burnmon-dev\main.go` | a 2 second ticker calls `lampKid.Tick()` (not under uicheck) |
| `cmd\burnmon-dev\lamp_child_test.go` | unit tests for `Tick` and `lampEnd` |
| `cmd\burnmon-dev\lamp_attr_windows_test.go` | the two opt-in window tests and their helpers |
| `scripts\uicheck-lamp.ps1` | runs the two window tests with `TestLampChildStartShowsWindow` |
| `cmd\burnmon-dev\README.md` | the close sentence says what is proven and what is unseen |
| this file, `STATUS.md`, `SESSION_LOG.md` | docs |

`cmd\burnmon-dev\page.html` is not edited. No earlier test was edited.

## Tests

- `TestLampCloseFromMenuEndsTheProcess` (opt-in, `BURNMON_LAMP_WINDOW_TEST=1`): starts the lamp
  through `execLampStart`, calls the page's `window.lampClose()` (what the menu item's click handler
  calls) through the dev eval channel on port 9336, expects the process gone within 5 s and
  `lamp_enabled` false in the temp settings folder. Failed on the unrepaired code ("still running 5 s
  after Close the lamp"). Passes now; the lamp ended 180 ms after the call (the test fails at 2.5 s or more), so the window loop ended,
  not the 3 second exit.
- `TestLampEndsWhenItsParentIsGone` (opt-in): `BURNMON_LAMP_PARENT` is the id of a process that has
  just ended; expects the process gone within 6 s. Failed on the unrepaired code, passes now.
- `TestLampEndRunsTerminateOnTheWindowThread`, `TestLampChildTickStopsAChildTheFileSaysOff`,
  `TestLampChildTickWithoutReaderIsSafe`: unit tests, written before the code (they did not compile
  until `lampEnd` and `Tick` existed; I did not run them red separately).

## Review (L5), what was taken and what was not

Taken: `Tick` reads the file under the lock (a `Sync(true)` just before could be undone by an older
read); `Tick` acts only on a file that was read and says false (`savedOff`), never on the default a failed
read gives, test `TestLampChildTickIgnoresAnUnreadableFile`; the parent-gone path writes the ledger
before `lampEnd` (the exit after 3 s skips the deferred write; `Ledger.Close` twice is safe, it writes
only while dirty); the menu test starts its clock before the call and fails at 2.5 s or more.
Not taken: `TestLampEndsWhenItsParentIsGone` keeps its 6 s limit, so it cannot tell the window loop
from the exit after 3 s (the first parent check is up to 2 s after the start); that distinction is
proven only by the menu test. The 2 second file read of `Tick` in BurnMon Dev is the one change outside
the lamp, a small disk read.

## Measured on this laptop

At the end of T1, after the review fixes: `go vet ./...` 0, `go test ./... -count=1` exit 0 with 50
packages ok and no FAIL line, `.\build.ps1` 0, linux and darwin build and vet 0 with CGO off,
`.\scripts\uicheck-lamp.ps1` exit 0 (menu test: the lamp ended 180 ms after the call). Built, not seen:
the click on the menu item in a window, the Mac cgo file.

## Turn log


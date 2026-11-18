---
type: plan
title: Glim start fix, plan and turn log (one turn, T1)
created: 2026-11-18
lane: C:\ZND\10_holding\02_roadmap\2026-11-18_glim_start_flash_fix.md
---

# Glim start fix, plan and turn log

Starts from commit `bceeffe` on `work/wilco-de-tree/lamp-glim-1117` (pull request 60), clean tree, no stash.
Guard check: raw `git push --dry-run` was denied, so the guard is active.

## Step 1: how go-webview2 makes and first shows its window

Read in `webview.go` of `github.com/jchv/go-webview2@v0.0.0-20260205173254-56598839c808`
(`CreateWithOptions`, lines 269 to 345), from the source:

1. `CreateWindowExW` with the fixed style `0xCF0000` (WS_OVERLAPPEDWINDOW: title bar, thick frame, system menu, minimize and maximize), no extended style, at `CW_USEDEFAULT` unless `Center` is set, size from the options.
2. At once `ShowWindow(SW_SHOW)`, `UpdateWindow`, `SetFocus`, all before `NewWithOptions` returns.
3. Then `browser.Embed`, which creates the WebView2 environment and controller while pumping messages. The window stays on screen, white and titled, for that whole time.

What the lamp cannot change before that first show: the style, the extended style, the place and the visibility, because the library offers no option for them and the lamp has no window handle until `NewWithOptions` returns. What it can do from outside: a thread hook on the lamp's own thread sees the window's messages while it is made, and DWM can cloak a window (`DWMWA_CLOAK`), which composes it as nothing without hiding it for the ShowWindow bookkeeping. `HideWindow` stays out (it hides the web view, finding 1).

Measured on the start commit with the new test: the plain window, 300 by 206 px, style `0x14cf0000`, was visible in every one of the first five failing readings the test prints (108 ms to 205 ms after the process started); the test prints no more than five.

## Way chosen

A `WH_CALLWNDPROC` hook on the lamp's thread (`cmd\burnmon-dev\lamp_cloak_windows.go`) cloaks the first top-level window with a caption as soon as DWM accepts the call; `runLampWindow` installs it before `NewWithOptions`, removes it after, and uncloaks the window after `lampStyleWindow` and `pl.place()`. A first try with a `WH_CBT` hook at window creation failed: DWM answered `0x80070006` (invalid handle) for a window that is only being created. The same code serves a lamp started by hand and the demo. The title stays "Glim". Not touched: the page, the settings, the week card window.

## Files changed

- `cmd\burnmon-dev\lamp_cloak_windows.go` (new): the hook and the cloak.
- `cmd\burnmon-dev\lamp_window_windows.go`: install and remove the hook, uncloak after place.
- `cmd\burnmon-dev\lamp_attr_windows_test.go`: `TestLampStartNeverShowsAPlainWindow` (opt-in, `BURNMON_LAMP_WINDOW_TEST=1`).
- `scripts\uicheck-lamp.ps1`: the new test joins the child start run.
- `STATUS.md`, `SESSION_LOG.md`, this file.

## Tests

`TestLampStartNeverShowsAPlainWindow`: reads every top-level window of the lamp process every 20 ms for 3 s from the moment the process exists; fails on a visible, uncloaked window with a caption or a thick frame, or off the top edge of its monitor; also requires `lampWindowProblem` to come back empty at least once (visible lamp window with a visible web view child). Seen failing on `bceeffe` (readings from 108 ms with a caption, style `0x14cf0000`), passing with the repair. The earlier window tests (`TestLampChildStartShowsWindow`, `TestLampCloseFromMenuEndsTheProcess`, `TestLampEndsWhenItsParentIsGone`, `TestLampChildAttrDoesNotHideOnWindows`) were run on the first repair attempt and passed; the final run is in the turn log. No earlier test was edited.

Built, not seen: whether the cloak leaves no pixel on screen is read from Windows (visibility, cloak state, style, rectangle), not from a screenshot.

## Turn log

T1: done; commit d1f1580; measured: go vet 0, go test ./... -count=1 50 packages ok and no FAIL, build.ps1 0, linux and darwin build and vet 0 (CGO off), uicheck-lamp.ps1 exit 0 (sampling test failed on bceeffe, passes); reviewer gap (a second cloaked window stays cloaked) closed; no earlier test edited; open: nothing in scope, built not seen. Left outside the repo: scratch folder C:\Users\WilcoDeTree\AppData\Local\Temp\glimdbg (mine, delete is Wilco's under rule L2)

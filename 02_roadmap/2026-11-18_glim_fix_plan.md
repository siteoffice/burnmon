---
type: plan
title: Glim fix lane, the plan of turns T2 to T6
created: 2026-11-18
by: turn T1 of C:\ZND\10_holding\02_roadmap\2026-11-18_glim_fix_lane.md (Claude Code, Opus 5.5, unattended)
branch: work/wilco-de-tree/lamp-glim-1117
start_commit: 633760486f8eb663e90848bf10e81863e862adf2
---

# Glim fix lane, plan

## Start

- Branch `work/wilco-de-tree/lamp-glim-1117`, HEAD `6337604` (`docs(burnmon): Log glim turn T14`), no tracked file changed at the start of T1, so no stash. `git stash list` is empty.
- Pull request 60 is open. `gh pr checks 60`: `macos pass` (run 44312468940).
- The lane file wins over this plan. This plan only fixes the readings T1 chose (section "Choices").
- Raw `git push --dry-run` was denied by the Groundwork policy: the guard holds.

## Baseline, measured in T1 on `6337604`

From C:\ZND\50_projects\burnmon in PowerShell, in the order of the session prompt.

| Command | Result |
|---|---|
| `go vet ./...` | exit 0 |
| `go test ./... -count=1 -json` | exit 0: 1748 tests pass, 7 skip, 0 fail; 50 packages ok, 4 without tests |
| `.\build.ps1` | exit 0, four exes |
| `GOOS=linux CGO_ENABLED=0`: `go build ./...`, `go vet ./...` | exit 0, exit 0 |
| `GOOS=darwin CGO_ENABLED=0`: `go build ./...`, `go vet ./...` | exit 0, exit 0 |

Nothing is red at the start. Every red in a later VERIFY is the lane's.

## The cause per finding

Found by reading the code at `6337604`. Nothing was run in a window in T1.

### Finding 1, a lamp started by BurnMon Dev is invisible. Sure: high that the hidden start is the cause; medium on the exact mechanism

- `cmd\burnmon-dev\lamp_child.go` `execLampStart` sets `cmd.SysProcAttr = lampChildAttr()`, and on Windows `lampChildAttr` (`cmd\burnmon-dev\lamp_window_windows.go:219`) is `&syscall.SysProcAttr{HideWindow: true}`. Go turns that into `STARTF_USESHOWWINDOW` with `SW_HIDE` in the child's STARTUPINFO.
- A hand start and a child start differ in three things only: that STARTUPINFO, `BURNMON_LAMP_PARENT`, and the inherited environment. The environment of BurnMon Dev adds only `WEBVIEW2_USER_DATA_FOLDER`, which the lamp overwrites (`lamp_window_windows.go:87`). `BURNMON_LAMP_PARENT` only feeds `lampParentGone`, and the lamp process was still running in Wilco's process list, so it did not end. That leaves the hidden start, and Wilco's hand start (without it) shows.
- The mechanism, read, not proven: go-webview2 (`webview.go` `CreateWithOptions`, module `v0.0.0-20260205173254-56598839c808`) creates the window without `WS_VISIBLE` and calls `ShowWindow(SW_SHOW)` before `Embed`. The ShowWindow documentation says the first call ignores its value when the parent gave a STARTUPINFO show value, so that call hides the window, and the WebView2 controller is made under a hidden host. `lampStyleWindow` hides it again; `lampPlacer.place` then calls `SetWindowPos` with `SWP_SHOWWINDOW`. Two readings remain: the host never becomes visible, or it becomes visible but the web view, made under a hidden host, paints nothing; with the clear backdrop a window that paints nothing looks exactly like no window. The repair is the same for both (no `HideWindow`), but the check of T2 must catch the second one too (choice C3).

### Finding 3, the ticker borders are cut. Sure: high that the window region cuts them; medium on which part of the region

- The page (`cmd\burnmon-dev\lamp.html`, `draw`) sends `area.ticker` with `h = ROW_H * rows` (`tH`). The ticker element is `rows * 26` plus its 1 px bottom border (`.ticker` has `border:1px solid; border-top:0` and no fixed height). `setRegion` passes `y + rh + 1` as the exclusive bottom, so at scale 1 the border row is just inside; at 125 and 150 percent the 1.25 or 1.5 device px border is split by the `+0.5` rounding of `lampRegionRects` and can lose its lower part. Glim's box (`area.glim`, from `y = tH`, x 51 to 249 at size 1.5) is a plain rectangle and covers the border above Glim whatever the rounding.
- `lampPlacer.setRegion` (`lamp_window_windows.go:311`) rounds the ticker's region with `CreateRoundRectRgn` at radius `8 * scale` (an ellipse of 2r, started r above the top so only the bottom corners round). Its stair-stepped arc does not follow the CSS corner with its 1 px border and clips the side borders over the lowest 8 CSS px, which on a one-row ticker of 26 px is a third of the side. That is "the side borders partly missing". (The T1 review found this the stronger cause; "the bottom border shows only right above Glim" along the whole straight bottom edge is not fully explained by reading.)
- So T2 first measures, with the check of C3 on this laptop, the region box (`GetWindowRgnBox`) against the ticker's own rectangle at the monitor's scale, then repairs.
- During `alarm`, `area.extra` is the whole window rectangle (`lamp.html`, `extra:(act==='alarm'||...)`), so every border is inside the region. That is "whole during alarm".
- `lampRegionRects` (`cmd\burnmon-dev\lamp_run.go:195`) also rounds each edge to the nearest device pixel, which can drop a fractional pixel at any edge on a 125 or 150 percent monitor.

### Finding 4, the opened ticker's burn row is out of line with row 1. Sure: low

The ticker is one element of a fixed 300 px with `overflow:hidden`, so no row can be wider than another by layout. What reading shows: the burn row (`openedHTML`) is the only row whose content is a bare `<b>` plus a bare text node inside a `display:flex; white-space:nowrap` row. A bare text node becomes an anonymous flex item that cannot shrink or show an ellipsis, so its text runs past the row's 8 px right padding to the ticker's edge, while row 1 ends with an ellipsis inside its padding. That explains "wider", not "starts further left". A second reading: the region of finding 3 rounds the bottom corners of the last row, and in the opened ticker that is the burn row, so its edges look different from row 1's. Neither is proven. T3 replaces the burn row content with sized spans and makes every row take the ticker's width; T6's uicheck group measures, in a real window, the left edge and width of every opened row against row 1 (criterion 6), which catches both readings.

### Finding 9, the Activity month names. Sure: low; the cause is not found by reading

- Wilco's window: `burnmon-dev-window.json` says maximized on `\\.\DISPLAY3`. Read in T1 with `System.Windows.Forms.Screen`: DISPLAY3 is 1152 by 2048 (work area 1152 by 2000) in the units that call reports, which fits a 1440 by 2560 portrait monitor at 125 percent. The 1432 by 1942 px of the screenshot are then device pixels, about 1146 by 1554 CSS px. The primary, DISPLAY2, is 3440 by 1440; DISPLAY1 is 1600 by 1000.
- His saved percents (`burnmon-dev-view.json`, standard) are the finding's: sections status 15, sessions 40, processes 35, todo 10; panes burn 40, vendor strip 20, activity and heatmap 20, turn ticker 20.
- Read: `.panel` is a flex column with `overflow:hidden`; `.heatmapwrap` shrinks (its automatic minimum is 0 because it clips); `pctFitCharts` (`page.html` near line 3699) scales `#heatmapGrid` by `room / scrollHeight` with a floor of 0.1; `pctWhole` then hides (`pct-cut`) any text a pane edge cuts. At about 1554 CSS px tall the activity pane gets roughly 95 to 120 px by the percents, against a grid of about 88 px plus 16 px padding: the grid should fit at a scale near 1, with the month line visible. So reading predicts labels, and Wilco sees none.
- Why PR 57 did not settle it: uicheck `panes/d51` measures only sizes that fit the monitor the uicheck window opens on (`ensureWindowSizeWH`, `windowCapped`), so a window as tall as Wilco's was never measured (it is skipped on the 1440 px primary).
- Candidates for T5 to measure, not to assume: `.heatmonths` itself is `height:10px; overflow:hidden` (`page.html:282`) and sits under the grid's `scale()`; the overlap pass in `renderHeatmap` (labels blanked when `offsetLeft` and `scrollWidth` are read while the grid is not laid out); the fit pass hiding `#heatMonthsRow` with `pct-cut`; the turn ticker's `min-height:80px` and the vendor table taking more than their percent so the activity pane is far shorter than the percents say; the 125 percent scale. d51 prints the panel, wrap, grid and month line boxes and whether the line carries `pct-cut`, which tells these apart.
- Note: Wilco said on 2026-11-18 (memory of an earlier session) that he would keep this on his own backlog. The lane file, written later the same day, puts it in T5. The lane wins.

### The other findings, for the turns (sure: high for each)

- 2: `.ticker` is a fixed 300 px and `.head` takes at most 48 percent; the line is cut with an ellipsis.
- 5: the coffee act plays in the current mood's pose. In mood `asleep` the rig has closed lids (`.mood-asleep .lid{animation:none}`), so he sips asleep.
- 6: the headline and the mood lines `{count} agents at work`, `1 agent at work`, `{count} things are blocked` and `{rate} tokens a minute` say the same thing (`internal\lamp\lines.go:26`, `:39`, `:45`, `:61`).
- 7: the eyes follow `mousemove` of the lamp page only (`lamp.html`), which fires only inside the lamp window.
- 8: the gear selects have no rule at all; the gear's other inputs are styled by id selectors (`#secSizeGrid input[type=number]` and `#histAdmin input[...]` at `page.html:553` and `:578`: `background:var(--bg); color:inherit; border:1px solid var(--bd)`), so the selects are the browser default, white.
- 10: `sheetSVG` (`internal\lamp\anim.go`) draws the bored pose at height 150 and the others at 190, because the wall makes the picture 320 wide; `poseSVG` does the same at 155 against 180.

## Shared shapes (made in T2 and T3, used later)

- `lamp.Settings` gains `Monitor string` (`lamp_monitor`, T2) and `SizePct int` (100 to 200, T3). The old `Size string` field stays untouched in T3, so `lampGear`, `lampGearPayload` and their tests still compile; T4 removes `Size` and moves the gear to `SizePct` (C18). `SizeScale(pct int) float64` is `pct / 100`.
- Pure seams in `cmd\burnmon-dev\lamp_run.go` (T2), each tested without a window:
  - `lampMonitor{Name string; X, Y, W, H int; Primary bool; Scale float64}` (device pixels, the full monitor rectangle) and the OS reader `lampMonitors func() []lampMonitor`.
  - `lampPlaceOn(mons []lampMonitor, name, position string, w int) (x, y int, used string)`: the top edge of the named monitor at left, centre or right; a name not in the list, or empty, is the primary.
  - `lampSnap(mons []lampMonitor, px, py, w int) (name, position string)`: the monitor under the pointer (nearest when none holds it), and the nearest of left, centre and right by the window's centre, returned as the saved values `top-left`, `top-center` and `top-right`.
  - `lampCursor func() (x, y int, ok bool)` (OS seam) and `lampPointerCSS(px, py, winX, winY int, scale float64) (x, y float64)`: the pointer in the page's CSS pixels, which may be far outside the window.
  - `lampAliveFrom(opened, denied bool, code uint32) bool`: not opened and denied is alive, not opened otherwise is gone, opened with exit code 259 is alive. No `syscall` or `windows` import in `lamp_run.go` (`TestLampRunHoldsNoSyscall` bans it, and the file builds for the Mac); `lampProcessAlive` in `lamp_window_windows.go` maps `windows.ERROR_ACCESS_DENIED` to `denied`.
  - `lampCloseNow(writeLedger func(), parentSet bool, save func(key string, v any) error, end func())`: ledger first, then `lamp_enabled` false only when `parentSet`, then end.
  - `(e *lampEngine) swing(maxDeg float64, changes int, now time.Time) string`: T2 makes it return "" and binds it; T3 fills it from `lamp.Protest`.
- Page bindings, made in T2 on both OSes (the Mac ones do nothing, built, not seen): `lampDragStart()`, `lampClose()`, `lampSwing(maxDeg, changes)`. The lamp process calls the page's `lampPointer(x, y)` 10 times a second while Glim is down and `lamp_motion` is `lively`.
- `lamp.Protest(maxDeg float64, changes int, now, last time.Time, gentle bool) (part string)` in `internal\lamp\acts.go` (T3): `protest2` when too hard and the last protest is under 60 s ago, `protest1` when too hard, `tickle` when gentle, else "". Too hard is `maxDeg >= 60` or `changes >= 3` (the page counts direction changes inside 1.5 s).
- New act lines (T3), in the act table of `lines.go` with `ActLine`: act `carry` parts `drag` and `view`; act `swing` parts `tickle`, `protest1`, `protest2`.
- `lamp.ActMood(act, mood string) string` (T3): `working` for `coffee`, else the mood. The payload carries it as `pose`; the page draws the rig from `pose`.

## Per turn: files and the tests written first

Each list is the most a turn changes through Edit or Write (L7: at most 12). Golden files and `assets\glim\` written by a command do not count. A file not listed may be touched only if the turn log names it with the reason.

### T2, the window: 12 files

| File | Change |
|---|---|
| `cmd\burnmon-dev\lamp_attr_windows_test.go` | first: `lampChildAttr()` on Windows is nil or has `HideWindow` false (lane-required change of `TestLampChildAttrHidesOnWindows`, named in the turn log); new `TestLampChildStartShowsWindow`, runs only with `BURNMON_LAMP_WINDOW_TEST=1` (choice C3) |
| `cmd\burnmon-dev\lamp_window_windows.go` | `lampChildAttr` without `HideWindow`; `lampProcessAlive` through `lampAliveFrom`; region from outward-rounded rectangles, no round region (C4); placement through `lampMonitors` and `lampPlaceOn`; the drag loop; the 10 per second pointer feed; the close binding; the backdrop setup that does not need a restart (C7) |
| `cmd\burnmon-dev\lamp_window_darwin.go` | the three new bindings as no-ops, so the page runs; the build stays green |
| `cmd\burnmon-dev\lamp_bindings.go` | `lampBindingNames` gains `lampDragStart`, `lampClose` and `lampSwing`, or `TestLampBindingNamesEqualPageCalls` fails |
| `cmd\burnmon-dev\lamp_run.go` | the seams of "Shared shapes"; `lampRegionRects` rounds outward (floor of x and y, ceiling of right and bottom); `lampWindowSize(set lamp.Settings)` takes the whole settings, and both window files call it so, so T3 can change its body without touching the window files |
| `cmd\burnmon-dev\lamp_run_test.go` | first: `lampRegionRects` holds every box whole at scales 1, 1.25, 1.5 and 2 with fractional boxes; `lampPlaceOn` left, centre, right, a gone monitor, an empty name; `lampSnap` on two and three monitors, a pointer between monitors; `lampPointerCSS` for a pointer 3000 px outside; `lampAliveFrom` denied, not opened, 259, 0; `lampCloseNow` order and the `parentSet` rule; `lampBackdropEnv` (C7, lane-required change named); the marker test of C17 |
| `cmd\burnmon-dev\lamp_child.go` | `died` reads `lamp_enabled` through a seam before a restart, so a lamp closed by its menu is not started again (C8) |
| `cmd\burnmon-dev\lamp_child_test.go` | first: a child that dies after the file says `lamp_enabled` false is not restarted |
| `cmd\burnmon-dev\lamp.html` | `area.ticker` from the ticker element's own rectangle (border included); `area.glim` grows while a `held` flag is set (T3 sets it from the grab; T2 tests it by the page text only); pointer down on the ticker calls `lampDragStart` after 6 px; right-click menu "Close the lamp" inside the area; `window.lampPointer` sets the eyes |
| `cmd\burnmon-dev\lamp_page_test.go` | first: the page text asks the ticker's rectangle for the area, has the 6 px drag threshold, the menu item, `contextmenu`, `lampPointer`; the area includes the menu while it is open |
| `internal\lamp\settings.go` | `Monitor` from `lamp_monitor` (any string; empty is the primary) |
| `internal\lamp\settings_test.go` | first: `lamp_monitor` read, missing, wrong type |

The drag runs in the lamp process, not the page (C5): on `lampDragStart` it polls `lampCursor` and the left button (`GetAsyncKeyState`) every 16 ms, moves the window with `SetWindowPos` and `SWP_NOACTIVATE`, and on release calls `lampSnap`, places the window, and saves `lamp_monitor` and `lamp_position` with `saveDevConfigKey`. The loop runs only during a drag.

### T3, state, lines and page: 12 files plus goldens

| File | Change |
|---|---|
| `internal\lamp\settings.go` | `SizePct int` from `lamp_size`, default 150; `small` 120, `medium` 150, `large` 190; a number from 100 to 200 kept as it is (C9); anything else 150. `Size string` untouched (C18) |
| `internal\lamp\settings_test.go` | first: 100, 200, 150, 99, 201, 137, a fraction, the three words, a bad word, a bool |
| `internal\lamp\lines.go` | the echo rule in the pick (a mood with only echo lines left gives ""); the five new lines in the act table; `ActMood` |
| `internal\lamp\lines_test.go` | first: for every mood and both languages at counts 1, 2 and 5, no picked line starts with `{count}`, `{rate}` or `1 agent`; the five lines in both languages, at most 60 runes |
| `internal\lamp\acts.go` | `Protest` |
| `internal\lamp\acts_test.go` | first: 59 and 60 degrees, 2 and 3 changes, 59 and 61 s between two protests, gentle gives `tickle` |
| `internal\lamp\glim.go` | the carried pose parts the page animates (both arms, a `carry` class on the rig); colours from the theme classes |
| `internal\lamp\glim_test.go` | first: the class names exist; goldens regenerated with `-update` and named |
| `cmd\burnmon-dev\lamp.html` | ticker width (C10); every opened row in the same box as row 1, the burn row in spans with an ellipsis; size from `look.size / 100`; the pose from `pose`; the carry pose and its keyframes; grab and swing with flailing arms (C11); `calm` and `off` do not swing |
| `cmd\burnmon-dev\lamp_page_test.go` | first: 300 and 440 in the width rule; no `SIZE` word map; the swing loop is started only on pointer down and stops at rest (no `setInterval` or `requestAnimationFrame` outside the swing); the flail keyframes; `pose` used for the rig |
| `cmd\burnmon-dev\lamp_run.go` | `lampWindowSize` from `SizePct`; `lampLook.Size int` from `SizePct` (the pin `p.Look.Size != "medium"` in `lamp_run_test.go:473` becomes 150, lane-required, named); payload `pose`; `swing` from `lamp.Protest` with the last protest time kept in the engine |
| `cmd\burnmon-dev\lamp_run_test.go` | first: a settings file changed between two `payload` calls shows in the second (criterion 3); the coffee act gives `pose` `working` in mood `asleep`; `swing` keeps the 60 s memory |

### T4, the gear group: 8 files

| File | Change |
|---|---|
| `cmd\burnmon-dev\page.html` | Lamp group: the size slider (`type=range`, 100 to 200, step 5) with the words min, middle and max under it and the number beside it; the monitor in use and the button "Back to the primary screen"; every select, the slider and the button carry the classes of C12 |
| `cmd\burnmon-dev\lamp_gear_page_test.go` | first: the slider attributes, the three words, the button text, the classes on every control of `#secLamp` |
| `cmd\burnmon-dev\lamp_child.go` | `setLampKey`: `lamp_size` takes a whole number 100 to 200 that is a multiple of 5; `lamp_monitor` takes "" only (the button); `lampGear` sends both |
| `cmd\burnmon-dev\lamp_child_test.go` | first: 100, 105, 200 accepted; 95, 102, 205, "medium" refused; `lamp_monitor` "" accepted, any other value refused; the gear pin at `:229` and `:234` (`Size` "large") becomes 190, lane-required, named |
| `cmd\burnmon-dev\sections_view.go` | `lampGearPayload.Size int`, `Monitor string` |
| `internal\lamp\settings.go`, `internal\lamp\settings_test.go` | the old `Size string` field and its tests removed; `SizePct` is the one size (C18) |
| `cmd\burnmon-dev\main.go` | the `bdevSetLamp` comment says the keys it takes, not "five keys" |

### T5, the Activity month names: at most 6 files

| File | Change |
|---|---|
| `tools\uicheck\group_panes.go` | first: d51 can take Wilco's window (C13): the section and pane percents of the finding and a size; it places the window on a monitor where the size fits |
| `tools\uicheck\group_panes_test.go` | the size and percent parse, pure |
| `tools\uicheck\win32.go` | only if the placement on another monitor needs it |
| `cmd\burnmon-dev\page.html` | the repair, after the check is seen red |
| `scripts\uicheck.ps1` | only if a variable has to pass through |

Two rounds; not found: set aside with what was ruled out (the four candidates of finding 9 are the list to rule out).

### T6, demo, pictures, checks, documents: at most 11 files

| File | Change |
|---|---|
| `internal\lamp\demo.go`, `internal\lamp\demo_test.go` | first: at the coffee second the pose is `working` with open lids |
| `internal\lamp\anim.go` and its test file | first: every pose of the sheet and of the pose files at one height (C14) |
| `internal\lamp\demosvg.go` | coffee awake in `glim-demo.svg` |
| `tools\uicheck\group_lamp.go`, `tools\uicheck\group_lamp_test.go` | ticker width 300 to 440 by text, opened rows in line with row 1, sizes 100, 150 and 200, the child-start check |
| `scripts\uicheck-lamp.ps1` | also runs `TestLampChildStartShowsWindow` with `BURNMON_LAMP_WINDOW_TEST=1` |
| `README.md`, `cmd\burnmon-dev\README.md`, `cmd\burnmon\readme_test.go` | settings table (`lamp_size` as a percentage, `lamp_monitor`), the drag, grab and swing, the eyes, the close, what is seen and what is still unseen |

Then `burnmon-dev.exe lamp --poses assets\glim` and `.\scripts\uicheck-lamp.ps1` once.

## Choices on ambiguities

- C1. Wilco's screenshot size 1432 by 1942 px is read as device pixels of DISPLAY3 at 125 percent, about 1146 by 1554 CSS px (see finding 9). T5 checks that size; if its own measure of the monitor says otherwise, it uses the monitor's real scale and says so.
- C2. The drag, the monitor, the pointer feed, the grab window area and the close are built for Windows. On the Mac the page bindings exist and do nothing (built, not seen); the README says so. The lane names no Mac work and L1 keeps it out.
- C3. The check of T2 (`TestLampChildStartShowsWindow`, Windows only, opt-in by `BURNMON_LAMP_WINDOW_TEST=1` so a normal `go test` opens no window) builds `burnmon-dev.exe` into a new temp folder, points `LOCALAPPDATA` and `APPDATA` at a temp folder, starts the lamp through `execLampStart(exe, []string{"lamp"})`, the function BurnMon Dev uses, and within 3 s reads from Windows for a top-level window of that process: `IsWindowVisible`, not cloaked (`DwmGetWindowAttribute` `DWMWA_CLOAKED` is 0), a rectangle that meets a monitor, and a non-empty window region (`GetWindowRgnBox`). The last one also proves the page ran: the region comes from the page's `lampArea`. It stops the child at the end. Whether pixels show is still for the first look.
- C4. The window region is the union of plain rectangles, each rounded outward. The rounded corners come from CSS only. Reason: the region's arc cut the borders (finding 3); outside the drawn corners the clear backdrop shows the windows below anyway, and with `card` the panel draws its own corners.
- C5. The drag runs in the lamp process from the OS pointer (`GetCursorPos`), not from page events, because the page's coordinates are CSS pixels of one monitor and a drag crosses monitors with other scales. The grab and swing run in the page with pointer capture, because they never leave the window area.
- C6. "Within one tick" for the window settings (`lamp_position`, `lamp_monitor`, `lamp_size`) is the lamp's 2 s placement tick; for the page settings it is the 1 s state poll. Both read the file each time (they already do for the page).
- C7. `lamp_backdrop` must change without a restart. The WebView2 background colour is fixed when the window is made, so the lamp always starts with the transparent background and DWM glass; `card` is the page's panel only. This changes `TestLampBackdropEnv` (card set none), lane-required by the Definition "a change shows in a running lamp", named in the turn log. If glass fails on a machine, the card panel still covers the ticker and Glim's box.
- C8. Close of a lamp BurnMon Dev started: the lamp saves `lamp_enabled` false, then ends. BurnMon Dev's child supervisor reads `lamp_enabled` from the file before it restarts a child that died, so it does not start it again, and the gear checkbox shows off at its next poll.
- C9. `lamp_size` numbers from 100 to 200 that are not a multiple of 5 are kept as saved (the Definition says a whole number from 100 to 200 loads); the gear only writes multiples of 5.
- C10. Ticker width: the page measures row 1's natural width (headline plus line plus paddings, without the ellipsis) and sets the ticker to `clamp(300, that, 440)` CSS px; the opened ticker keeps that width and left edge for every row. The window width follows from the ticker and Glim's box as today.
- C11. Grab and swing: angle toward the pointer from Glim's cord top, capped at 60 degrees; on release a damped swing that rests within 3 s. The page counts direction changes of the pointer's horizontal move within 1.5 s and asks `lampSwing(maxDeg, changes)` for the line on release or at the stop; the 60 s memory lives in the lamp process. A press shorter than 4 px of move is a click (next line). The window area grows to Glim's swing room (60 degrees either side) while held and until rest.
- C12. The gear classes: today the gear's inputs carry no class and are styled by id selectors (`page.html:553`, `:578`), and its buttons carry `setbtn`. T4 defines one class `setin` once, with that look (`background:var(--bg); color:inherit; border:1px solid var(--bd); font:inherit`) plus `color-scheme` from the theme so an open list is dark in the dark theme, and puts it on the gear's existing number and text inputs too (the id rules stay), so it is a class the other gear controls use. Every select and the slider of the Lamp group get `setin`, the button `setbtn`. Criterion 10 is tested on these two classes.
- C13. T5's check is d51 with the window placed on the monitor where the size fits (DISPLAY3 for Wilco's size). If no monitor fits it, the check at that size cannot fail or pass here; T5 then records criterion 11 as set aside with what was ruled out at the largest size that fits, never as a pass.
- C18. `lamp_size` moves to a number in two steps so every turn builds: T3 adds `Settings.SizePct` and moves the lamp (page, window size, payload) to it; the gear keeps the old `Size` word until T4, which moves the gear and removes `Size`. Between T3 and T4 the gear still writes the words, and they still load (120, 150, 190).
- C19. The uicheck group `lamp` (`tools\uicheck\group_lamp.go:107`, `:141`, `:151`, `:212`) sends size words in its fake payloads. From T3 the page reads a number, so the group is red until T6 rewrites it. No turn before T6 runs it; T6 runs it once.
- C14. One height for every pose: the sheet draws every pose at height 170 (the bored picture is then 272 wide, inside its 300 column); the pose files at height 155 (the bored picture is 248 wide on the 256 canvas). The pose files change size too; the lane asks the same size in the sheet and the regenerated files follow.
- C15. Echo lines: the rule is on the line text as the Definition says, also for Dutch (`{count}`, `{rate}`, "1 agent" open the Dutch lines too). Bored lines with `{minutes}` are not echo lines.
- C16. The carry line "Put me down." shows while dragged; "Nice view." shows once after a drop on a monitor other than the one the drag started on; a drop on the same monitor shows the mood line again.
- C17. The pointer for the eyes is read only while Glim is down and `lamp_motion` is `lively`; at any other time the 10 per second read does not run. The lamp writes the pointer nowhere; T2's marker test runs the engine and the ledger with a marker cursor value and scans the data folder.

## Turn log


T1: done; commit 59eacd4; measured: vet 0, test exit 0 (1748 pass, 7 skip, 0 fail, 50 packages ok), build 0, linux build and vet 0, darwin build and vet 0 (CGO off), doccheck ok on the plan, gh pr checks 60 macos pass, guard holds (raw push denied); causes by reading: finding 1 the hidden child start (high, mechanism medium), 3 the window region (high, which part medium, T2 measures first), 4 low, 9 not found by reading (low, T5 measures); reviewer: T3 would not build and was over budget (fixed by C18, SizePct in two steps), T2 missed lamp_bindings.go (now 12 files), lampAliveFrom moved off syscall types, the finding 3 cause weakened, C12 and C13 rewritten, C19 added; no stash; open: nothing
T2: done; commit b9aaa5d; measured: vet 0, test exit 0 (1760 pass, 8 skip, 0 fail; the new skip is the opt-in window check), build.ps1 0, linux build and vet 0, darwin build and vet 0 (CGO off, so lamp_window_darwin.go was not compiled); TestLampChildStartShowsWindow with BURNMON_LAMP_WINDOW_TEST=1 passes on this laptop (a visible lamp window with a region and a visible web view child within 3 s) and fails with "no visible web view child window" when HideWindow is put back, so the hidden start is the cause of finding 1; built, not seen: the pixels, the drag, the eyes, the close menu, the window region of finding 3 (outward-rounded plain rectangles, no round region, the ticker area measured from the element). Test edits named: TestLampChildAttrHidesOnWindows became TestLampChildAttrDoesNotHideOnWindows (lane-required), the lampWindowSize test passes lamp.Settings. Left for T3 (not done in T2): the lamp_backdrop change without a restart (C7, the lane puts settings changes in T3; TestLampBackdropEnv unchanged), the page binding lampSwing (the page test needs the page to call every binding), and setting held in the page (area and window width for a grabbed Glim exist, the page text test only proves the code is there). dragged now suppresses the Glim click. Reviewer found a leak of one windows.NewCallback per place() (fixed: one callback for the process), a marker test that proved less than it claimed (comment corrected), held and dragged unused (dragged used, held is T3). TestPointerReachesNoFile covers the engine and the ledger only. Scratch files outside the tree that I cannot delete: /tmp/edit_run.py, /tmp/lww.bak, /tmp/attr_head.go, /tmp/attr_tail.go, /tmp/st.go, /tmp/t2.diff; no stash; open: finding 3 repair is by reading plus the check, T6 measures; the first look sees the rest
T3: done; commit 7041afd; measured: vet 0, test exit 0 (1771 pass, 8 skip, 0 fail), build.ps1 0, linux and darwin build and vet 0 (CGO off, so lamp_window_darwin.go was not compiled); built, not seen: ticker width, opened rows, grab and swing, flail, carry lines, coffee pose, every pixel. Done: lamp_size as SizePct (100 to 200, old words 120/150/190, bad 150), lamp_monitor resolved in the payload (look.monitor, gone falls back to the primary), echo rule in the pick, the five lines, lamp.Protest and ProtestTooHard, ActMood and payload pose, carry_lines, lampSwing binding on both OSes returning {line, hard}, page: size from look.size/100, ticker 300 to 440 by a hidden measure of row 1, burn row in spans, grab and swing with one rAF loop only while held, flail arms, carry class and lean. Plan deviations: PctScale(pct) added instead of changing SizeScale(string) (T4 removes the word); Protest sits in lines.go not acts.go; glim.go untouched (arms and classes already in the rig, carry pose is page CSS); lampSwing registered in lamp_bindings.go and both window files; the C7 backdrop change without a restart is NOT done (needs the window files beyond the 12-file budget; TestLampBackdropEnv unchanged), left for T6 or T8. Test edits named: TestSingularRule lost the two "1 agent" expectations (echo lines, lane-required), TestSettingsDefaults, TestNewLampKeys and TestSetLampKey gained SizePct, TestLampWindowSize and the look pin moved to percentages, TestEngineSwingHasNoLineYet replaced, TestLampPageTickerSizes lost the size word map. Reviewer: carry could stick if the drop ends outside the window (fixed: ends on a changed monitor or after 20 s), swung stuck after a calm press (fixed: reset at every pointerdown); a gentle swing always gives the tickle line (kept, by the Definition); no behaviour change outside the lamp found; no stash; open: C7 backdrop without restart
T4: done; commit 11c1751; measured: vet 0, test exit 0 (50 packages ok, no FAIL line), build.ps1 0, linux and darwin build and vet 0 (CGO off); built, not seen: the slider, the monitor line, the button, the dark select list. Done: size slider (100 to 200, step 5, min, middle, max, number beside), the monitor in use and one button "Back to the primary screen" (sends lamp_monitor empty), class setin on every select, the slider and the gear's other inputs (number and text) with color-scheme from the theme, class setbtn on the button, setLampKey takes lamp_size as a multiple of 5 from 100 to 200 and lamp_monitor only empty, lampGear sends size as a number and monitor, Settings.Size removed (SizePct is the one size), main.go comment. Test edits named: lamp_child_test.go pins at the old size "large" became 190 and gained refusal cases, settings_test.go lost Size from its expectations and the small case checks SizePct 120, lamp_gear_page_test.go lost the three size options (lane-required) and gained two new tests. Reviewer found a dead back button (the gear pane stops click bubbling, so the listener now sits on the button), a poll that could move the slider while dragged (guarded), a gear fallback object without size and monitor (added); each has a page text test. Not done in T4 and open: the gear shows a saved monitor name even if that monitor is gone (T3 resolves it for the lamp, the gear shows the saved name); gofmt still flags sections_view.go and main.go as at HEAD; the C7 backdrop change without restart is still open for T6 or T8; no stash; no scratch files
T5: done; commit 57b079b; measured: vet 0 (root and tools\uicheck), test exit 0 (no FAIL line; uicheck module ok), build.ps1 0, linux and darwin build and vet 0 (CGO off), gofmt clean on the three Go files; uicheck panes/d51 at 1136x1554 CSS px with Wilco's section and pane percents on DISPLAY3 (UICHECK_ORIGIN=3440,-225): RED before the fix (7 month labels, 0 visible, each label display:none), GREEN after (7 of 7 visible); d51 at its other default sizes (1500x650 to 2560x1440) exit 0 after the fix. Cause, measured: the labels carried display:none (the pct-cut class of the whole-row pass is inferred, not read); the 8px label's normal line height is taller than its 10px clipping .heatmonths box, so pctWhole counted it as cut. Repair: .heatmonth gets line-height:10px (page.html, one rule). Check: d51 gets the size 1136x1554 with Wilco's percents (his 1146 is capped on that monitor at the 100 percent the uicheck process sees, so ten px narrower, a deviation from the finding's size), the UICHECK_D51_SIZE suffix p, the label rectangles in its output; the first run at 1146 was skipped (capped), the second failed on pane percents (Processes panes added to 120 with every part on, now 100). Plus a page text test (activity_months_page_test.go). Four ruled-out candidates: heatmonths height, the overlap pass, the scale (grid was not scaled: wrap 104 px, grid 88 px), the percents (panel 164 px). Reviewer: no behaviour change outside the month labels, no test weakened; the page test is a string match and was never seen red on its own (the d51 run is the red/green proof); the default size now 1136 (was 1146 in the first draft). Not seen: the pixels in Wilco's own window at 125 percent (the screenshot d51-1136x1554-pct.png in testdata\uicheck\out was read, which is a 100 percent window). No stash; no scratch files; open: Wilco's DISPLAY3 look at 125 percent
T6: done; commit 0791ce2; measured: vet 0 (root and tools\uicheck), test exit 0 (50 packages ok, no FAIL line), build.ps1 0, linux and darwin build and vet 0 (CGO off), gofmt clean (carriage returns removed first); .\scripts\uicheck-lamp.ps1 exit 0 (TestLampChildStartShowsWindow PASS in 39 s, group lamp OK at sizes 100, 150, 200, ticker 300 for a short line and 440 for a huge one, every opened row in line with row 1); built, not seen: every window and pixel. Done: coffee awake (the demo svg draws the working pose in the coffee seconds, test first), every pose one height (sheet 170, pose files 155; the bored one is 1.6 times wider for its wall, so width is not equal, ambiguity: "same size" read as height), burnmon-dev.exe lamp --poses assets\glim run (nine files changed, glim-bored.svg unchanged), uicheck group lamp rewritten (size as percent, text lengths short and huge, row boxes, pose class), scripts\uicheck-lamp.ps1 runs the child-start check first, both READMEs (settings table, drag, grab and swing, eyes, close, what is seen and unseen). Found by the first full uicheck run: the group read .head and .sub inside the hidden #meas row of the page (black background, so light-theme contrast failed and dark passed by luck); the read is now scoped to #tick (check fix, not a page fix; this was the second run of uicheck-lamp.ps1, a repair round). Test edits named: lampWantWindow takes the ticker width, lampCase.Size is a percent, TestLampFixturesCoverEveryCase and TestLampCasesCoverTheNewLayout pinned size words (lane-required), TestReadmeLampSection gained strings. Reviewer: no behaviour change outside the lamp; the rig test was weak (fixed: root class asserted), README wording overstated "seen" (fixed: "reported by the owner"), window width is derived from the measured ticker width so only the 300 to 440 range and the short and huge cases prove the width (kept, named). Scratch file outside the tree that I cannot delete: %TEMP%\edit_lamp_test.py; no stash; open: C7 backdrop change without a restart (not in T6 row, still open since T3), the first look
T7: done; commit 4266e5b graded (verify file 04_assets\2026-11-18_glim_fix_verify.md, local only); measured: vet 0, test exit 0 (1776 pass, 8 skip, 0 fail, 50 packages ok), build.ps1 0, linux and darwin build and vet 0 (CGO off), tools\uicheck vet and build 0, uicheck-lamp.ps1 exit 1 then exit 0 unchanged (flake, cause not found), poses regenerated byte-identical, d51 at 1136x1554p on DISPLAY3 green on HEAD and red with the start commit's page.html (pct-cut measured), gh pr checks 60 macos pending; grades: 15 PASS, 2 FAIL (3: lamp_backdrop needs a restart, C7; 4: drag follow and save untested, inline in lampPlacer.drag), 0 NOT BUILT; no reviewer (no code diff); no stash; open: the two FAILs for T8, the uicheck-lamp flake, the macos check

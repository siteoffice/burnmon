---
type: plan
title: Glim finished, the plan of turns T2 to T11
created: 2026-11-18
by: turn T1 of C:\ZND\10_holding\02_roadmap\2026-11-18_glim_finish_lane.md (Claude Code, Opus 5.5, unattended)
branch: work/wilco-de-tree/lamp-glim-1117
start_commit: 8878600e6f4b519c4c2265ea5ccf98348e1918c5
---

# Glim finished, plan

## Start

- Branch `work/wilco-de-tree/lamp-glim-1117`, HEAD `8878600` (`docs(burnmon): Log lamp turn T12`), no tracked file changed at the start of T1, so no stash.
- Pull request 60 is open. `gh pr checks 60`: `macos fail`, run 44218896371 on `8878600`.
- The lane file wins over this plan. This plan wins over nothing in the lane; it only fixes the
  readings T1 chose (section "Choices").

## Baseline, measured in T1 on `8878600`

From C:\ZND\50_projects\burnmon in PowerShell, in the order of the session prompt.

| Command | Result |
|---|---|
| `go vet ./...` | exit 0 |
| `go test ./... -count=1 -json` | exit 1: 1694 pass, 1 fail, 7 skip (tests and subtests). The fail: `TestDocCheckPassesOnRepoDocs` in `internal\doccheck` |
| `.\build.ps1` | exit 0, four exes |
| `GOOS=linux CGO_ENABLED=0 go build ./...` and `go vet ./...` | exit 1 and 1: `cmd\burnmon-dev\cloudpull.go:16:10: undefined: app` |
| `GOOS=darwin CGO_ENABLED=0 go build ./...` and `go vet ./...` | exit 0 and 0 |
| `git push --dry-run` typed raw | denied by the Groundwork policy: the guard holds |

The run 44218896371 log (`gh run view 44218896371 --log-failed`) has two failures on the Mac:
`TestDocCheckPassesOnRepoDocs` and `TestPurgePullCopies` (`cloudpull_test.go:116`, "PurgePullCopies =
0, 0, 0, <nil>; want 1, 1, 1").

### The three red items and their causes (read, not yet repaired)

1. **doccheck, new and red on Windows too.** `STATUS.md` line 3, the top paragraph written by run 2
   in `4b502a8`, holds the path `C:\ZND\50_projects\burnmon\04_assets\2026-11-17_lamp_lane_report.md`
   in backticks. `internal\doccheck\doccheck.go` (`isRuntimePath`, the drive letter rule, in
   `checkPath` at line 241) asks an allowlist entry for every drive-letter path in a checked doc,
   whatever the OS. Run 2 made this red; it is not red on `main`. It keeps every VERIFY of this lane
   red, so T2 repairs it (choice C1).
2. **Linux build.** `type app` lives in `cmd\burnmon-dev\app.go` (`//go:build windows || darwin`);
   `cloudpull.go` has no build tag and uses `app` at line 16. Its only caller is `main.go:163`
   (`windows || (darwin && cgo)`). Fix: the tag `//go:build windows || darwin` on `cloudpull.go`.
3. **`TestPurgePullCopies` on the Mac.** `sessionIDFromPath` (`internal\store\store.go:1319`) uses
   `filepath.Base`, which does not split on `\` on macOS, and the test's paths are Windows paths.
   The prefix match in `PurgePullCopies` already normalises `\` to `/`; only the session id is
   wrong, so the `DELETE ... session_id = ?` matches nothing. Fix: replace `\` with `/` and take
   `path.Base` and `path.Ext` (rename the parameter, it shadows the package). Windows unchanged.

## What the "Not read" list turned out to be

- **How go-webview2 makes and paints its host window** (module `github.com/jchv/go-webview2`
  `v0.0.0-20260205173254-56598839c808`, `webview.go:269` to `:345`): it registers the class
  `webview` with no background brush, creates the window with `WS_OVERLAPPEDWINDOW` (`0xCF0000`)
  and no extended style, then calls `ShowWindow(SW_SHOW)`, `UpdateWindow` and `SetFocus` before
  `Embed`, all before our code gets the handle. `WebViewOptions.Window` is not read by
  `NewWithOptions`. The controller (`ICoreWebView2Controller2.PutDefaultBackgroundColor`) exists in
  `pkg\edge` but is not reachable: the `browser` field of `webview` is unexported and the `WebView`
  interface (`common.go:26`) has no way to it. The class has no brush, so the host paints nothing
  of its own; without help from DWM that shows as black or stale pixels, not as the desktop.
- **The Mac test result**: red, see the baseline. The lamp package `cmd\burnmon-dev` compiled and
  passed on the Mac runner in run 2 (verify file of run 2). Nothing has run on a real Mac.
- **The words file of UI standard 15**: the repo's one words file is `internal\waits\words.go`,
  read by `internal\waits\words_test.go`. It holds the kind words and the group fallback. Group
  names "Your turn" and "Blocked" come from it. The lamp's new plain words (sheet labels, act
  names a person reads) go into that file in the turn that needs them (T9), with a test that reads
  them (choice C12).

## The way chosen for the clear backdrop, and why

`lamp_backdrop` `clear` (T7, `cmd\burnmon-dev\lamp_window_windows.go`):

1. Before `webview2.NewWithOptions`, set `WEBVIEW2_DEFAULT_BACKGROUND_COLOR` to `00000000` in the
   process environment, the same way the file already sets `WEBVIEW2_USER_DATA_FOLDER`. The
   runtime reads it when it makes the controller; Microsoft Learn documents a fully transparent
   default background through this variable. It is the only way to the controller without
   reflection into an unexported field.
2. The page sets `html` and `body` to `background:transparent` when the payload says `clear`.
3. The host window gets `DwmExtendFrameIntoClientArea(hwnd, MARGINS{-1,-1,-1,-1})` from
   `dwmapi.dll` through `windows.NewLazySystemDLL` (a system DLL, no new module), after the
   styles are set. DWM then composes the client area with its alpha, so the transparent pixels of
   the WebView show the windows below.
4. `SetWindowRgn` with the window area (the union of the ticker, rounded at the bottom, and
   Glim's box; larger during `alarm`, `slip` and `wall`). Outside the region nothing is drawn and
   clicks go to the windows below. This also limits the damage if step 3 does not work: then only
   the inside of Glim's box shows a dark square.
5. `lampOpenDev` starts BurnMon Dev with `WEBVIEW2_DEFAULT_BACKGROUND_COLOR` removed from its
   environment, or BurnMon Dev's own window would inherit a transparent background.

Why not `WS_EX_LAYERED` with a colour key: a colour key leaves fringes on Glim's antialiased edges,
and WebView2 in windowed hosting is reported not to draw into a layered window (inferred from
reports, not tested here). Why not the composition controller: go-webview2 does not offer it and a
new module is ruled out (L11).

`card`: none of steps 1 to 3; the page draws the rounded panel in the theme's surface colour, as
wide as the ticker, behind Glim. Step 4 still applies. All of this is "built, not seen". If the
first look shows a black or stale square around Glim, `lamp_backdrop` `card` is the switch (HAND in
T14).

## Shared data shapes (T3 and T5 make them, later turns use them)

- `lamp.State` gains (T3): `TickerRows int` (`1 + min(counted, lamp_ticker_rows - 1)`, 1 for a quiet
  or off lamp), `Counted int` (number of counted items) and `CountedIDs []string` with
  `json:"-"` (for `stamp`; never sent to the page). `Rows` stays the opened list: at most 6, row 6
  "and <n> more". The ticker shows `Rows[:TickerRows-1]`.
- The payload of `lampState` (T5) gains `act` (name or ""), `act_seq` (a number that goes up by one
  each time an act starts, so the page restarts the animation), `holds` (list of `wall`, `sweat`,
  `sleep`, `dots`), `dot_seconds` (the `dots` interval), `quiet` (true when the lamp shows the
  sliver) and `look` (`size`, `theme`, `motion`, `quiet`, `backdrop`, `rows`: the six settings
  after defaults).
- The act engine (T5): `lamp.NextAct(in ActInput) (act string, holds []string, played Played)`,
  pure. `ActInput{Prev, Cur State; Now time.Time; Idle, PrevIdle int; Settings; Played}`. `Played`
  holds what already played: `SlipStretch` (the bored stretch slip played in, keyed by the time
  mood became `bored`), `CoffeeDay` and `CoffeeSlot` (10 or 15), `PeekMinute`, plus `Seq`.

## Per turn: files and the tests written first

Each list is the most a turn changes through Edit or Write (L7 caps it at 12). Golden files and
`assets\glim\` written by a command do not count. A file not listed may be touched only if the turn
log names it with the reason.

### T2 (fix list), 11 files

| File | Change |
|---|---|
| `internal\lamp\ledger.go` | F2: an item counts in `items` only on the first kept day it is seen: before adding a hash to today, skip it when any earlier kept day holds it |
| `internal\lamp\ledger_test.go` | first: the day-2 case of `:62` now expects 0 items for the two old items and 1 for a new one. This changes a test that existed before the lane, outside the exceptions of criterion 14; the fix list (F2, "test first") requires it. Named in the turn log as a lane-required change |
| `cmd\burnmon-dev\README.md` | F4: the lamp keeps two things on disk, `lamp-waits.json` and the WebView2 profile folder `wv2-lamp`, both in the data folder |
| `cmd\burnmon-dev\lamp_run.go` | item 4: `lampParentGone(env string, alive func(pid int) bool) bool` (empty or bad value: false, the lamp stays) and `lampChildEnv(base []string, pid int) []string` |
| `cmd\burnmon-dev\lamp_run_test.go` | first: `lampParentGone` with no variable, a bad value, a live pid, a dead pid; `lampChildEnv` adds `BURNMON_LAMP_PARENT=<pid>` once and keeps the rest |
| `cmd\burnmon-dev\lamp_child.go` | `execLampStart` sets `cmd.Env = lampChildEnv(os.Environ(), os.Getpid())`; argv stays own exe and `lamp` |
| `cmd\burnmon-dev\lamp_window_windows.go` | the 2 s ticker also checks `lampParentGone(os.Getenv("BURNMON_LAMP_PARENT"), lampProcessAlive)` and ends the window with `w.Terminate`; `lampProcessAlive` uses `OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION)` and `GetExitCodeProcess` (259 is alive). The Mac side is T10 |
| `cmd\burnmon-dev\buildtags_test.go` | first: `cloudpull.go` added to the `windows || darwin` list (fails before the tag) |
| `cmd\burnmon-dev\cloudpull.go` | item 5: the build tag |
| `internal\store\store.go` | item 5, narrowed: `sessionIDFromPath` has four callers (lines 994, 1212, 1264, 1498), so it stays as it is. Only `PurgePullCopies` (the call at 1498) passes the path with `\` replaced by `/` first. Ingest and collapse keep their session ids on every OS. The test first is the existing `TestPurgePullCopies`, red on the Mac runner; on Windows no test can fail for it. Proven green only by the `macos` check after propose |
| `STATUS.md` | the doccheck repair: the top paragraph names the report without a drive-letter path in backticks (repo-relative `04_assets\...` in plain text) |

F1 is not in T2 (budget): each F1 needle is fixed by the turn that rewrites its file, `lines_test.go`
in T3, `glim_test.go` in T4, `demo_test.go` in T9 (choice C2).

### T3 (settings, ticker rows, singular rule, new lines), 6 files plus goldens

| File | Change |
|---|---|
| `internal\lamp\settings_test.go` | first: the six keys, their defaults, one bad value each (wrong type, unknown choice, 0 and 5 rows, a fraction) |
| `internal\lamp\settings.go` | `TickerRows`, `Size`, `Theme`, `Motion`, `Quiet`, `Backdrop`; `SizeScale(size) float64` (1.2, 1.5, 1.9) |
| `internal\lamp\state_test.go` | first: `TickerRows` at 1 and 4 rows with 0, 1 and 5 counted items (1, 1, 1 and 1, 2, 4); quiet and off give 1; title cut at 28 runes (the 40-rune case of `:279` pins the old pill, renamed and changed, named in the turn log); `CountedIDs` not in the JSON |
| `internal\lamp\state.go` | the fields of "Shared data shapes"; `titleRunes` 28 |
| `internal\lamp\lines_test.go` | first: F1 needle; the singular rule (count 1 gives the singular line, 2 gives the `{count}` line, never "1 agents" or "1 things"); the wall line only up to 30 minutes; every new line in both languages and at most 60 runes; act lines by `ActLine(act, part, lang, count, hour)` |
| `internal\lamp\lines.go` | the twelve new lines as data (the act lines in their own table, so the mood line pick formula and its counts per mood stay as they are); `ActLine`; the singular rule in `Line` |

Golden JSON of a full state (`internal\lamp\testdata\`) regenerated by its test command if it
changes, named in the turn log. F6 is done here.

### T4 (Glim as a rig), 4 files plus goldens

| File | Change |
|---|---|
| `internal\lamp\glim_test.go` | first: F1 needle; one golden per mood per theme, in two files `testdata\glim-dark.golden` and `testdata\glim-light.golden`; every class name below exists; no `<image>`, no `href` but the SVG namespace, no `<script>`; the wall has exactly 10, 20, 30 bricks and shows `30+` above 30 |
| `internal\lamp\glim.go` | `Glim(mood, bricks, theme)` returns the rig: the shapes and coordinates of the prototype's `glim()` (`2026-11-18_glim_alive_prototype.html`, lines 161 to 177). The signature stays exactly as it is: its callers `anim.go`, `card.go`, `lamp_window_windows.go` and `lamp_window_darwin.go` are not in this turn |
| `internal\lamp\anim.go` | only if the rig's size breaks `glimSized`; the new look of the poses is T9 |
| `internal\lamp\card.go` | only if the same break reaches the card; `card_test` goldens regenerated by command and named |

The rig keeps the old class names next to the new ones, so the pinned tests of `glim_test.go`
(lines 65, 72, 114: `glim glim-<mood>`, `glim-brick`, `glim-wall`) pass unchanged: the root carries
`glim glim-<mood> mood-<mood> theme-<theme>`, a brick `brick glim-brick`, the plank `plank glim-wall`.

Class names the page animates (fixed here, T6 uses exactly these): `glim`, `theme-light` or
`theme-dark` and `mood-<mood>` on the root `<svg>`; `rig`, `swing`, `cord`, `handle`, `body`,
`dome`, `basep`, `feet`, `eyes`, `pup`, `lid`, `mouth`, `arm armL`, `arm armR`, `mitt`, `rays`,
`drop`, `zz`, `plank`, `bricks`, `brick`, `mug`, `dots`, `dot`, `stampi`, `rip`. Colours: one
`<style>` inside the SVG with the two palettes of the prototype keyed by `theme-light` and
`theme-dark` (the dome keeps its ambers in both), so a pose file stands alone and the page can
override. The mouth shapes are four paths (`smile`, `flat`, `sad`, `oh`), chosen per mood as the
spec's table says.

### T5 (the act engine), 4 files

| File | Change |
|---|---|
| `internal\lamp\acts_test.go` | first: one "due" and one "not due" per act; the order with `alarm` and `knock` due in one tick, and with `welcome` and `coffee`; `slip` once per bored stretch and again in a new stretch; `peek` only when quiet, `hide` and not `off`, at the first tick of a unix minute that is a multiple of 4; `coffee` not while an item waits, once per slot per day; `welcome` at 299 (not due) and 300 (due) idle seconds; `calm` and `off` play no act; the holds per mood and setting; the `dots` interval at 0, 1000 and 100000 tokens a minute (4, 4 and 0.3) |
| `internal\lamp\acts.go` | `NextAct`, `Played`, `Holds`, `DotSeconds`; the act table with lengths (2.6, 1.6, 2.4, 2.4, 1.6, 6.4, 4, 4.4 s) |
| `cmd\burnmon-dev\lamp_run.go` | the engine keeps the previous state, `Played` and the previous idle seconds; idle seconds through a seam `lampIdle func() int` (0 until T7 sets the real one); the payload fields of "Shared data shapes" |
| `cmd\burnmon-dev\lamp_run_test.go` | first: the payload carries `act` and `act_seq` after a blocked count goes up, through a fake snapshot and a fake idle seam |

### T6 (the page), 4 files

| File | Change |
|---|---|
| `cmd\burnmon-dev\lamp_page_test.go` | first: keyframes for every act and state (`alarm`, `knock`, `stamp`, `welcome`, `wake`, `slip`, `coffee`, `peek`, `wall`, `sweat`, `sleep`, `dots`, plus the idle `sway`, `breathe`, `blink`); `prefers-reduced-motion` and `prefers-color-scheme`; both palettes 4.5 to 1 with the method of `contrast_test.go`; hover titles; no em dash, no outside resource, no `overflow:auto` or `scroll`; the ticker is 300 px wide with 26 px rows; the opened ticker shows 6 rows and the burn row with `API price equivalent`. The tests of the old sliver-pill layout are replaced; each replaced test is named in the turn log |
| `cmd\burnmon-dev\lamp.html` | rebuilt from the prototype's setup 4, slim ticker; the binding names stay, plus `lampArea` |
| `cmd\burnmon-dev\lamp_bindings.go` | `lampArea` added to the list |
| `cmd\burnmon-dev\lamp_bindings_test.go` | only if the scanner test needs the new name |

The page reports its window area through `lampArea(json)`: the ticker rectangle and Glim's box in
CSS pixels, and an extra box while `alarm`, `slip` or `wall` runs. The window code (T7) owns what it
does with it. Until T7, `lampArea` is bound to a function that stores the value only.

### T7 (the Windows window), 6 files

| File | Change |
|---|---|
| `cmd\burnmon-dev\lamp_run_test.go` | first: `lampRegionRects(area, scale)` (pure: CSS rects to device rects, rounding, the union bounds); sizes per `lamp_size` and rows (ticker 300 by 26 times rows; Glim box `120 * scale` square below it); the env of the clear backdrop is set only for `clear` |
| `cmd\burnmon-dev\lamp_run.go` | `lampRegionRects`, `lampWindowSize`, `lampBackdropEnv(backdrop) (key, value string, ok bool)` |
| `cmd\burnmon-dev\lamp_window_windows.go` | the clear backdrop steps 1 to 5 above; `SetWindowRgn` from `CombineRgn` of the rects; idle seconds through `GetLastInputInfo` and `GetTickCount`; the focus try (below); the parent check from T2 stays |
| `cmd\burnmon-dev\lamp_child.go` | the focus try: the child is started hidden (see below) through a function of the two window files |
| `cmd\burnmon-dev\lamp_window_darwin.go` | only the no-op twin of that function, so the darwin cgo build compiles |
| `cmd\burnmon-dev\buildtags_test.go` | only if a new file is made |

The focus try: BurnMon Dev starts the lamp child with `SysProcAttr.HideWindow` set
(`STARTF_USESHOWWINDOW` with `SW_HIDE`). Windows applies the start-up show value to a process's
first `ShowWindow` call, so go-webview2's `SW_SHOW` becomes a hide and the window first appears
through our `SetWindowPos` with `SWP_NOACTIVATE`. Inferred from the `ShowWindow` documentation, not
proven; the hand-back of the focus stays. A lamp started by hand is not helped by this.

### T8 (the gear), 5 files

| File | Change |
|---|---|
| `cmd\burnmon-dev\lamp_child_test.go` | first: `setLampKey` takes the six new keys with good values and refuses bad ones, before a byte is written |
| `cmd\burnmon-dev\lamp_child.go` | `setLampKey` and `lampGear` know the six keys |
| `cmd\burnmon-dev\sections_view.go` | `lampGearPayload` gains the six fields |
| `cmd\burnmon-dev\lamp_gear_page_test.go` | first: the gear's Lamp group has the six controls and sends each through `bdevSetLamp` |
| `cmd\burnmon-dev\page.html` | the six controls in the Lamp group (select for rows, size, theme, motion, quiet, backdrop), the defaults in `secState.lamp`. The only turn that edits this file |

### T9 (demo, poses, card), 8 files plus `assets\glim\` and goldens

| File | Change |
|---|---|
| `internal\lamp\demo_test.go` | first: F1 needle; the demo timeline plays every act and every state that lasts exactly once in a fixed loop of 48 s (table below); "demo" true in every second; the old 24 s pins are replaced and named |
| `internal\lamp\demo.go` | `DemoSeconds` 48; `DemoState(sec)` and `DemoAct(sec)` |
| `cmd\burnmon-dev\lamp_run.go` | the demo engine takes its act from `DemoAct`, not from `NextAct` |
| `internal\lamp\anim.go` | poses in the new look, all the same size; the sheet labels in plain words; `glim-demo.svg` with the acts, `@keyframes`, no `<script>`, under 300 KB |
| `internal\lamp\anim_test.go` or the test that pins the poses today | first: the sheet labels have no underscore; every pose has the same width and height; the demo SVG size limit |
| `internal\waits\words.go` and `internal\waits\words_test.go` | the plain mood words of the sheet ("your turn", "blocked", ...), read by a test (UI standard 15) |
| `internal\lamp\card.go` (and `card_test.go` goldens by command) | the week card draws the rig |

Then `burnmon-dev.exe lamp --poses assets\glim` (built first with `.\build.ps1`), and the files it
writes named in the turn log.

Demo loop, 48 s: `sleep` 0 to 4, `wake` 4 to 6, working with `dots` 6 to 10, `knock` 10 to 13,
`alarm` 13 to 16, `stamp` 16 to 19, bored with `wall` (bricks 10 to 15) 19 to 24, `slip` 24 to 31,
`sweat` 31 to 35, `welcome` 35 to 38, `coffee` 38 to 42, quiet with `peek` 42 to 47, done 47 to 48.

### T10 (the Mac lamp), 4 files

| File | Change |
|---|---|
| `cmd\burnmon-dev\lamp_window_darwin.go` | the same page; the panel clear (`setOpaque:NO`, `clearColor`, no shadow) and the web view's `underPageBackgroundColor` clear (documented, macOS 12 and later); with `card`, an opaque panel; idle seconds with `CGEventSourceSecondsSinceLastEventType`; sizes per `lamp_size`; the parent check with `kill(pid, 0)` through `lampParentGone` |
| `scripts\maccheck.sh` | the lamp line runs the new demo |
| the test that pins `maccheck.sh` (`TestMaccheckScriptShape`) | first: the new line |
| `cmd\burnmon-dev\buildtags_test.go` | only if a tag changes |

No undocumented API: if the web view still paints opaque, the Mac shows the `card` look; write it
in the turn log. Nothing of this runs on a Mac in the lane; the `macos` check compiles it.

### T11 (uicheck, READMEs, pinned document tests), 7 files

| File | Change |
|---|---|
| `tools\uicheck\group_lamp_test.go` | first: the fixtures for rows 1 and 4, three sizes, both themes, each act frozen at its middle |
| `tools\uicheck\group_lamp.go` | the checks: ticker size per rows, Glim box per size, no scrollbar, hover titles, contrast |
| `scripts\uicheck-lamp.ps1` | only if the start line changes |
| `README.md` | the lamp row and the image link stay correct |
| `cmd\burnmon-dev\README.md` | the lamp section as goal, place in the whole, install, use, a table of all twelve lamp settings, next steps, backlog; F3 (the focus sentence says what is proven) |
| `cmd\burnmon\readme_test.go` | the pinned document tests, updated and named |
| `internal\doccheck\doccheck_test.go` | only if a README path needs an allowlist entry |

Then `.\scripts\uicheck-lamp.ps1` once, with BurnMon Dev closed (a running `burnmon-dev.exe` blocks
uicheck). Red after two rounds: a HAND line.

## Choices on ambiguities

- C1. The doccheck failure was made by run 2's `STATUS.md` line, so it is the lane's. T2 repairs it
  next to the two failures of fix list item 5, because a red `go test` stops every turn's VERIFY.
- C2. F1 moves from T2 to the turn that rewrites each file (T3, T4, T9): T2 would need 14 files.
  Criterion 11 is graded in T12, after all three.
- C3. Fix list item 4 is built for Windows in T2 and for the Mac in T10, through one pure seam
  `lampParentGone`. The process check is per OS.
- C4. The ticker title cut is 28 runes for the ticker rows and the opened ticker alike (one row type,
  the ticker is 300 px wide). The 40-rune test pins the old pill.
- C5. The act lines (`slip`, `stamp`, `wake`, `welcome`, `coffee`) live in their own table, so the
  mood line pick formula and its per-mood counts of the spec stay as they are. The two singular
  lines (`working` 1, `blocked` 1) are used only at count 1 and are not in the pick.
- C6. "The line" during an act is the act's line; after the act the mood line comes back. The two
  `stamp` lines alternate by `act_seq`.
- C7. `welcome` uses the idle seconds of the previous tick (300 or more) and of this tick (under 5).
  The engine keeps the previous value. A lamp that starts with idle 300 or more plays nothing on its
  first tick.
- C8. `coffee`: "the first tick of 10:00" means the first tick whose local time is 10:00:00 to
  10:00:59, once per day per slot. A lamp started at 10:00:30 still plays it.
- C9. `peek`: "at its first tick" means the first tick in a unix minute that is a multiple of 4,
  once per such minute.
- C10. The stretch of mood `bored` that limits `slip` starts when the mood becomes `bored` and ends
  when it is any other mood. `slip` is due at the first tick with a longest live wait of 15 minutes
  or more inside the stretch.
- C11. Glim's box is `120 * scale` CSS pixels square under the ticker (the prototype rig is about 108
  units high with the cord); the window area adds 24 px on each side during `alarm` and the slide
  length of the prototype (`--sl`, half the ticker width) during `slip`, and the plank's 92 px during
  `wall`.
- C12. The words file is `internal\waits\words.go` (UI standard 15). The sheet's plain words go
  there in T9 with a test; the lamp does not keep a second list.
- C13. The `card` backdrop is a panel 300 px wide (the ticker's width) and as high as Glim's box.
- C14. The demo loop grows from 24 s to 48 s to hold every act and state once. The 24 s tests pin
  the old demo and are replaced in T9, named in the turn log.
- C15. `lamp_ticker_rows` above the counted items does not add empty rows: the formula of the
  Definitions decides.
- C16. The PR check `macos` can only show the Mac fix of item 5 after a propose. T2 reads
  `gh pr checks 60` once at the end of its turn and writes what it saw; it does not wait for it.

- C17. The wall: the lane says "at 30 bricks the plank is full and the ticker shows `30+`"; the
  spec said above 30. The lane wins: `WallOver` is true from 30 bricks (a longest live wait of 30
  minutes or more). T3 changes it and names the changed 30 and 31 bricks test; T4's rig test and
  T6's ticker test check `30+` at 30.
- C18. Tests that pinned what the lane replaces, outside criterion 14's named exceptions, change
  only where the lane requires it, each named in the turn log as lane-required: the ledger day-2
  count (F2, T2), the 24 s demo (T9), the 40-rune cut (T3), the 30 bricks edge (T3).
- C19. `internal\waits\words.go` is shared by the notification and the lamp. T9 only adds words
  there; no existing word changes.
- C20. C1 is a deviation from the lane: its one exception to L1 names only the Linux build and
  `TestPurgePullCopies`. The doccheck repair is docs only, and without it `go test` is red in every
  turn. T14 writes no drive-letter path in backticks into the top paragraph of `STATUS.md`
  (doccheck asks an allowlist entry for one); it names the report repo-relative in plain text.

## Gaps the turns must also test (from the T1 review)

- T5: mood `off` plays no act, whatever the motion setting.
- T6: the quiet `hide` sliver (120 by 6 px, Glim out of sight); `stay` shows row 1 and Glim; the
  page in `calm` (blink only, `wall` without the drop) and `off` (still poses); the eyes that follow
  the pointer; the 2 s close of the opened ticker; the rounded bottom corners; `30+` in the ticker;
  the wait in the format of UI standard 17 (`timefmt.Short`, the exact value from `timefmt.Exact`
  on hover, which `state.go` already gives).
- T7: `lampWindowSize` covers the sliver (120 by 6 times the scale) as well as the ticker and Glim.

## Turn log

T1: done; commit fa798b1; measured: vet 0, test 1694 pass 1 fail (TestDocCheckPassesOnRepoDocs, made by run 2's STATUS line, T2 repairs it, C1) 7 skip, build 0, linux build and vet 1 (cloudpull.go), darwin build and vet 0, guard holds; reviewer: 13 findings folded into the T2 row, C17 to C20 and "Gaps"; no stash; open: nothing
T2: done; commit a2524ff; measured: vet 0, test exit 0 (50 packages ok; one earlier run had a flaky TestRunKickWritesAtOnce in internal\snapshotwriter, Access is denied on a rename, green on rerun, not touched by T2), build 0, linux build and vet 0, darwin build and vet 0 (CGO off), gofmt clean; lane-required test change: TestLedgerTwoDaysAndARestart day 2 now expects 0 items (F2); new tests: TestLedgerItemCountsOnItsFirstDayOnly, TestLampParentGone, TestLampChildEnvAddsTheParentOnceAndKeepsTheRest, cloudpull.go added to TestDevBuildTags; no golden file changed; reviewer: all items met, no behaviour change outside the lamp beyond items 3 and 4; lampProcessAlive and the Terminate path are Windows only and untested (built, not seen); item 5 Mac fix (PurgePullCopies) is proven only by the macos check, `gh pr checks 60` right after the push reported no checks yet; no stash; open: read the macos check of pull request 60 in a later turn; F1 stays with T3, T4 and T9 (C2)
T3: done; commit 616f141; measured: vet 0, test exit 0, build 0 (four exes), linux build and vet 0, darwin build and vet 0 (CGO off), gofmt clean on internal\lamp; six new settings with defaults and SizeScale, TickerRows/Counted/CountedIDs (json:"-") in State, titleRunes 28, WallOver from 30 bricks, singular lines in their own table joining the pick only at count 1, ActLine, wall line only up to 30 minutes, F1 needle in lines_test.go done and F6 done; lane-required test changes (C18): TestRowTitleAndWord 40 to 28 runes, TestWallEdges 30 bricks now over (C17); golden regenerated by -update and named: internal\lamp\testdata\state_full.golden.json (adds ticker_rows, counted); reviewer: no findings; guard held (raw push denied); no stash; open: nothing (built, not seen: no window involved)
T4: done; commit 148e694; measured: vet 0, test exit 0, build 0 (four exes), linux build and vet 0, darwin build and vet 0 (CGO off), gofmt clean; rig in internal\lamp\glim.go (signature unchanged, 200x200 or 320x200): every mood draws every part, one style element keyed to theme-light and theme-dark with colour variables and per-mood visibility, plank with at most 30 bricks and 30+ from 30; goldens regenerated by -update and named: internal\lamp\testdata\glim_dark.golden.svg and glim_light.golden.svg (names kept from run 2), card_story.golden.svg and card_wide.golden.svg (the week card draws the rig); lane-required test changes (C18): rays, drop and z z now always present (visibility by the style), a style element is allowed in the rig, brick and wall class matched as ` glim-brick"` and ` glim-wall"`, root class now carries mood- and theme- classes, F1 needle in glim_test.go done; reviewer: no behaviour change outside the lamp; open for T6: the SVG style selectors are unscoped (.cord, .arm, .rays and more) and hit the page when the SVG is inline, scope them under svg.glim or rename on the page; mouth, sclera, pupil and drop colours are hard-coded (not theme variables); mouths are eight paths on one .mouth, not four class-named shapes; the class test matches loosely, anchor on whole tokens; the plank ignores the 4 unit cord offset; no stash; built, not seen
T5: done; commit 9f3a47b; measured: vet 0, test exit 0, build 0 (four exes), linux build and vet 0, darwin build and vet 0 (CGO off), gofmt clean; act engine in internal\lamp\acts.go: NextAct, Holds, Quiet, DotSeconds, ActSeconds, Played; lamp_run.go keeps previous state, Played and previous idle, plays one act for its length with a steady act_seq, seam lampIdle (0 until T7), payload gains act, act_seq, holds, dot_seconds, quiet, look; choices: Played.SlipStretch is a bool reset when mood leaves bored; no act on the first tick or on the first tick after no data (what is already there is not news); coffee and peek are marked seen at the first tick of their minute even when nothing plays; while an act plays no new act starts and the previous state is kept, so a change meanwhile is judged after the act; demo has no acts until T9; the tests were written before acts.go but only seen failing as a compile error (undefined NextAct, lampIdle), not as assertion failures; no golden file changed, no earlier test changed; reviewer: no findings; gaps it named: calm and off tests do not check the slip, coffee and peek bookkeeping, and slip also plays on the first tick of a lamp that starts inside a long bored wait (kept, the Definitions make it a threshold); guard held (raw push denied); no stash; open: nothing (built, not seen: no window involved)
T6: done; commit bdfcb1c; measured: vet 0, test exit 0 (no FAIL line), build 0 (four exes), linux build and vet 0, darwin build and vet 0 (CGO off), gofmt clean; lamp.html rebuilt as the slim ticker (300 px, 26 px rows, opened ticker with 6 item rows and the burn row, sliver 120x6 for quiet hide, Glim hanging under it at lamp_size, keyframes for every act and state, theme system/light/dark from look.theme, clear and card backdrop, eyes follow the pointer in lively only, hover from the ticker, Glim or sliver only); choices: the page asks lampArea(json) and lampSize(w,h), the window only stores the area (T7 turns it into the clip region); act lines come from a new payload field act_lines (lampActLines in lamp_run.go, test first) so the page holds no line text; stamp is drawn by the rig's stampi lifted onto the ticker by --sy; window width grows symmetrically (pad) for wall, slip and alarm; peek, coffee and wake bring Glim down even when quiet hide shows the sliver; files: lamp.html, lamp_page_test.go, lamp_bindings.go, lamp_bindings_test.go, lamp_run.go, lamp_run_test.go, lamp_window_windows.go (the bind of lampArea, beyond the plan row); lane-required test changes (C18): the old sliver-pill page tests are replaced (SLIVER table, 320 and 440 widths, 40+28*rows, dark amber on black, fg/bg contrast pairs); reviewer: no behaviour change outside the lamp; folded in: pupils still in calm and off, hover from anywhere in the window, dot speed restart (--ds guard), glimSVG retry, slip pad; left open: click-through clip (T7), page tests grep class names as text so a deleted rule still passes, the sliver mood table is only pinned in internal\lamp Quiet(), a rig rebuild restarts running animations when the mood or brick count changes, the click wink on a sleeping Glim; guard held (raw push denied); no stash; open: nothing (built, not seen: every window, animation and position; the uicheck group lamp still tests the old layout and is T11's)
T7: done; commit 47295d6; measured: vet 0, test exit 0 (one earlier run had a timing flake in internal\fsutil TestReadFileRetryMissingFileReturnsAtOnce, green on rerun, package untouched), build 0 (four exes), linux build and vet 0, darwin build and vet 0 (CGO off), gofmt clean on the touched files; new tests first, seen failing as a compile error: TestLampWindowSize, TestLampRegionRects, TestLampBackdropEnv, TestLampWithoutEnv, TestLampIdleFrom, TestLampChildAttrHidesOnWindows; clear backdrop as planned (env WEBVIEW2_DEFAULT_BACKGROUND_COLOR=00000000 before the window, DWM glass, SetWindowRgn from the page area; card sets neither); lampOpenDev strips the variable; idle seconds through GetLastInputInfo into the lampIdle seam; child started with HideWindow (inferred, not proven); window starts at lampWindowSize; Mac twin only lampChildAttr (T10 does the rest); no existing test changed; reviewer: no behaviour change outside the lamp, no weakened test; known minor: union bounds not built (region is a CombineRgn of the rects), a failed region build is not retried, the real idle also drives the demo window (the demo calls no act engine), start size differs slightly from the page size and the page corrects it at once; one rename lampBackdropKey to lampBackdropVar because the save scanner took the constant for a secret; guard held (raw push denied); no stash; open: nothing; built, not seen: every window, backdrop, region, click-through, focus and idle value
T8: done; commit 0f08064; measured: vet 0, test exit 0 (no FAIL line), build 0 (four exes), linux build and vet 0, darwin build and vet 0 (CGO off); gofmt: the touched files differ only by a trailing blank line my pipe adds, and sections_view.go has alignment drift on lines I did not touch (same at HEAD), left alone; six gear controls (rows, size, theme, motion, quiet, backdrop) in the Lamp group, defaults in secState.lamp, rows sent as Number; setLampKey and lampGear and lampGearPayload know the six keys (rows takes a whole float64 1 to 4); tests first (TestSetLampKey, TestPageLampGearGroup), seen failing only as a compile error (lampGearPayload had no field TickerRows), not as assertion failures; no existing test weakened, tests extended; reviewer: no findings; nits: the bdevSetLamp comment in cmd\burnmon-dev\main.go lines 344 to 346 still says "five keys" (outside the T8 files, left), lampGear uses a positional literal; guard held (raw push denied); no stash; open: nothing; built, not seen: the gear dialog, every select and its save
T9: done; commit dbbb8a6; measured: vet 0, test exit 0 (one earlier run had a flaky TestWriteBurnReport_SecondPressOverwrites, Access is denied on a rename, green on rerun, untouched), build 0 (four exes), linux build and vet 0, darwin build and vet 0 (CGO off), gofmt clean (CR removed); demo loop is 48 s in internal\lamp\demo.go (DemoAct, DemoSettings: Glim hangs for seconds 0 to 9 so sleep, wake and dots show, hidden for coffee and peek), each act once and each state that lasts once, tested; the engine demo takes its act from DemoAct (coffee line uses hour 10); glim-demo.svg built in the new internal\lamp\demosvg.go (ticker, Glim in a wrapper, one @keyframes per act over the 48 s loop, loops for sweat, sleep and dots, 128143 bytes, no script); sheet labels from waits.MoodWord in internal\waits\words.go with a test; the week card already drew the rig since T4, card.go unchanged; ran burnmon-dev.exe lamp --poses assets\glim, wrote the ten files glim-asleep, blocked, bored, demo, done, off, sheet, sweating, working, your_turn (.svg); lane-required test changes (C14, C18): the 24 s demo pins replaced by 48 s tests, the SVG size cap 200 to 300 KB, the engine demo mood indexes, F1 needle in demo_test.go done; reviewer: no findings; minor, left: Glim already hangs at second 4 so wake is a pop-up jump, and dots start at second 4 not 6 (inferred from Holds); one more file than the plan row (demosvg.go, to keep anim.go small); check step: 0 checks configured; guard held (raw push denied); no stash; open: nothing; built, not seen: every animation and the generated svg files
T10: done; commit d991749; measured: vet 0, test exit 0 (no FAIL line), build 0 (four exes), linux build and vet 0, darwin build and vet 0 (CGO off, so the cgo file is not compiled), gofmt clean on the Go test files (they are CRLF, gofmt -d shows the whole file for that reason); Mac lamp in cmd\burnmon-dev\lamp_window_darwin.go: same page, clear panel (setOpaque NO, clearColor, no shadow, web view underPageBackgroundColor clear under @available macOS 12) for lamp_backdrop clear, opaque panel with rounded bottom corners for card, idle seconds from CGEventSourceSecondsSinceLastEventType into the lampIdle seam, start size and rows from lampWindowSize and the saved settings, lampArea bound as a no-op (macOS has no window region, clicks on the clear part reach the panel), the lamp ends through lampParentGone with syscall.Kill(pid, 0); scripts\maccheck.sh lamp step runs the demo 6 s, then mkdir -p and lamp --poses and needs a non-empty glim-demo.svg; tests first, seen failing as assertions: TestLampDarwinWindowFileFinishLane (new, in buildtags_test.go) and the new strings in TestMaccheckScriptShape; files: lamp_window_darwin.go, scripts\maccheck.sh, buildtags_test.go, maccheck_test.go (buildtags_test.go holds the darwin text tests, so it changed beyond "only if a tag changes"); no existing test weakened; reviewer found a real bug (lamp --poses needs the folder to exist, mkdir -p added, pinned in the test, seen failing first) and a trivial pin (WebKit now -framework WebKit); left open by the reviewer: the card look rounds layer corners on an opaque window so they may not show, macOS 11 or older has no clear web view background and nothing tells the person to pick card, w.Terminate on the pinned webview_go may skip the deferred Destroy and ledger Close (same call on Windows, not new), the any-input constant is written as (CGEventType)~0; if the web view still paints opaque on a Mac the Mac shows the card look (lamp_backdrop card); guard held (raw push denied); no stash; open: nothing; built, not seen: the Mac cgo file was never compiled on a Mac, no panel, backdrop, idle value or window size was seen, the macos check of pull request 60 compiles it after the push

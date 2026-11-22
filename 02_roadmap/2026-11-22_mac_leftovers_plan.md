---
type: plan
title: Mac leftovers lane, plan and turn log
created: 2026-11-22
lane: C:\ZND\10_holding\02_roadmap\2026-11-22_mac_leftovers_lane.md
branch: work/wilco-de-tree/mac-leftovers
---

# Mac leftovers lane: plan

Written in T1 (Opus 5.5, unattended). The lane file holds the items L1 to L5, the turns and the
success criteria; this file holds what T1 read in the code, the files and tests per turn, every
choice on an ambiguity and the turn log.

## Start

- Branch `work/wilco-de-tree/mac-leftovers`, started with siteoffice-git `start` from `main` at
  `1283a62` (the merge of pull request 67).
- Guard check: a raw `git push --dry-run` was denied by the Groundwork policy. The guard holds.

## Baseline, measured on `1283a62` before any edit (Windows, T1)

- `go vet ./...` exit 0.
- `go test ./... -count=1` exit 0: 50 packages ok, 1849 tests passed, 12 skipped, 0 failed.
- `.\build.ps1` exit 0.

## T1, L1 and L5 (done in this turn, see the turn log)

- L1: `cmd\burnmon-dev\lamp_unsupported.go` prints "This build has no lamp window. The lamp runs in the
  Windows build and in the Mac app (the build with cgo)." The existing test
  `TestLampUnsupportedSaysTheSentenceAndExits2` in `cmd\burnmon-dev\lamp_run_test.go` pins the new
  sentence and that it holds no "Windows only" (it failed first). The Limits paragraph of
  `cmd\burnmon-dev\README.md` quotes the new sentence; the pin in `TestReadmeLampSection`
  (`cmd\burnmon\readme_test.go`) changed from "Windows only" to the new sentence (it failed first).
- L5: `scripts\uicheck.ps1` runs `w0` to `w9` by default (comment line 8 and the call on line 56).
  No Go test pins that list, and `tools\uicheck\` is outside T1's file list, so no test was added;
  the run of the script is the check.
- L5 run, measured on Windows (WebView2) with the apps closed, exes from `.\build.ps1` on this tree:
  `.\scripts\uicheck.ps1` with the default list exits 1. The burnmon.exe pass stops at `w1`
  ("window looked blank/unpainted 1.5s after launch"), so `w2` to `w9` did not run in it; the
  burnmon-dev.exe groups exit 0 (d19 partial: the screen is too small for 4 of 5 cases) and the
  controlcentre.exe groups exit 0. `w1` alone: exit 1 again. `.\scripts\uicheck.ps1 w0 w2 w3 w4 w5
  w6 w7 w8 w9`: exit 0; `w9` measured at 960 x 611 and 1280 x 860, "What it was" 206 px at 960, no
  cut header, no cut cell without a title. `w1` is not caused by this lane (no burnmon.exe code
  changed; STATUS records `w1` red in the baselines of 2026-11-16 and 2026-11-17); it was not run on
  `1283a62` itself in this turn, so "red on main" is inferred, not measured. Success criterion 5
  (the default run exits 0) is not met while `w1` is red; T4 grades it.

## T2, L2: click-through outside the lamp's rectangles on a Mac

Cause, read in the code: `cmd\burnmon-dev\lamp_window_darwin.go` binds `lampArea` to a function that
does nothing (around line 212), so the panel keeps its whole frame for clicks. The page sends
`lampArea` as JSON with the boxes `ticker`, `glim`, `extra` and `menu` in CSS pixels
(`cmd\burnmon-dev\lamp.html` around line 255), only when the JSON changes. On Windows
`lampRegionRects` in `cmd\burnmon-dev\lamp_run.go` turns it into device rectangles (scaled, rounded
outward); bad JSON gives none.

Files (5 of the 12 allowed):
1. `cmd\burnmon-dev\lamp_run.go`: the pure function.
2. `cmd\burnmon-dev\lamp_run_test.go`: its tests, written first.
3. `cmd\burnmon-dev\lamp_window_darwin.go`: keep the rectangles, the poll, the C calls.
4. `cmd\burnmon-dev\lamp_window_darwin_src_test.go`: a source-text test of the darwin file (it
   already holds such tests; the darwin file cannot run on Windows).
5. `cmd\burnmon-dev\README.md`: one sentence in the lamp section.

Tests first (Windows, Go):
- `lampPointIn(rects []lampRect, x, y int) bool`: inside, each of the four edges (left and top edge
  inside, right and bottom edge outside, as a half-open rectangle), outside; an empty list returns
  true (the whole window takes clicks, as today); a list with the ticker and Glim's box: a point in
  the gap between them is outside.
- Rectangles come from `lampRegionRects(area, 1)`: on a Mac points equal CSS pixels, so the scale is 1.
- Source test of the darwin file: `lampArea` stores the rectangles, the file calls
  `setIgnoresMouseEvents:` and `[NSEvent mouseLocation]` and `pressedMouseButtons`, and the poll is
  not faster than 100 ms.

Darwin code, choices:
- A Go `time.Ticker` of 100 ms that runs the check through `w.Dispatch`, so every AppKit call is on
  the main thread. Chosen over an `NSTimer` in C because the Go side already holds the rectangles
  and the mutex pattern of `lampMacPlacer`.
- One C function returns the pointer in window coordinates with the origin top left:
  `x = mouse.x - frame.origin.x`, `y = frame.size.height - (mouse.y - frame.origin.y)`, plus
  whether a mouse button is down (`[NSEvent pressedMouseButtons] != 0`). A second C function sets
  `setIgnoresMouseEvents:`. Go calls the setter only when the value changes.
- While a button is down the window keeps the clicks (a drag of the ticker or of Glim goes on even
  when the pointer leaves the rectangles).
- Ambiguity: in the quiet sliver state the page sends `lampSize` (120 x 6) but no new `lampArea`, so
  the stored rectangles are the last ones of the ticker. Chosen: when the page reports a size whose
  width or height is smaller than the stored rectangles need, the rectangles are cleared and the
  whole window takes clicks, as today. T2 may pick a simpler rule if the code shows one; name it in
  the turn log.
- Built on the macos runner, not started. The `macos` check is read after propose.

## T3, L3 and L4

L3, cause read in the code: `row1HTML` in `cmd\burnmon-dev\lamp.html` (around line 169) writes the
headline as `.head` (`max-width:48%`, ellipsis) and the line as `.sub`. When BurnMon is not running
both carry the same sentence, so it shows twice. `tickerWidth` measures row 1 in the hidden `#meas`
with no max width and clamps it between `TICKER_W` 300 and `TICKER_MAX` 440.

Read for the Mac window width: the page calls `lampSize(w, h)` whenever its size changes; the darwin
binding stores it in `lampMacPlacer.setSize` and runs `place` on the main thread, and the C function
`lampPlace` sets the frame to that width. So the Mac window follows the ticker width already.
Inferred, not measured: the doubled sentence measured under 300 px, so the ticker stayed at 300 and
the head was cut at 48 percent of it. No change to the Mac window width is planned.

L3 files: `cmd\burnmon-dev\lamp.html`, `cmd\burnmon-dev\lamp_page_test.go`.
Tests first (page tests read the page as text, as the existing ones do, plus the hover title):
- headline equal to the line: row 1 holds the text once, in an element with the full text as its
  title, and no `.sub` with the same text.
- headline and line differ: the head keeps its own width (`flex:none`, no `max-width` cut while the
  ticker is below `TICKER_MAX`); the line shortens first (`.sub` has `min-width:0` and the ellipsis).
  At `TICKER_MAX` the head may still be cut, with its title.
- Then `.\scripts\uicheck-lamp.ps1`, result in the turn log.

L4, read: the Sessions rows are built in `internal\report\template.html` around line 1061: the
cells Calls (`n(s.calls)`), Tokens (`tok(s.tokens)`), Per call and Cost (`money(...)`) carry no
`title`. The other cells of the row do.
L4 files: `internal\report\template.html`, `internal\report\sessions_layout_test.go` (the D10 test
of the fix lane), `tools\uicheck\check_w9.go`. Golden files that change are regenerated by their
test command and named in the turn log.
Tests first: a template test that the four number cells carry `title=` with the full value (the
number as shown, not the raw float). Then a new `w9` case with 1,234,567 calls and 9,876.54 EUR at
960 x 611: no number cut, or a cut number has its title, no horizontal scrollbar. Choice: the title
holds the value as the cell shows it in full, unrounded where the cell rounds (cost to 2 decimals,
per call to 3), so the hover says more than the cell.
Write "measured at <width> x <height> on Windows (WebView2)" for the table and "not seen" for the
lamp on a Mac.

T3 files in all: 5 of the 12 allowed (6 with `tools\uicheck\groups.go` if the case needs a list entry).

## Choices on ambiguities, T1

- The darwin stub `runCardWindow` (`cmd\burnmon-dev\lamp_window_darwin.go` near the end) also calls
  `lampUnsupported`, so `lamp --card --card-window` in the Mac app prints the new sentence, whose
  first half ("This build has no lamp window") is not true there. The file is outside T1's list and
  the item is outside T2's row. Not changed; T4 names it as an open flag. Recommended later fix: a
  sentence of its own for the card window.
- The comment on line 8 of `scripts\uicheck.ps1` says "the seven d groups" while `$devGroups`
  holds nine. Stale before this lane, not touched (surgical change only).
- L1 and L5 had no new test file: the existing sentence test and the README pin changed instead
  (named in the turn log). The lane's "one test for the sentence" is that changed test.

## Turn log


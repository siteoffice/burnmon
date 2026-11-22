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

## T4, grading of the success criteria

Measured in T4 on Windows on `d413651` (the T3 head; T4 changed only documents): `go vet ./...` 0;
`go test ./... -count=1` 0, 50 packages ok, 1855 passed, 12 skipped, 0 failed (1849 at the
baseline plus the tests of T2 and T3); `.\build.ps1` 0; linux and darwin build and vet without cgo
0; `tools\uicheck` vet and build 0; `.\scripts\uicheck-lamp.ps1` 0; `.\scripts\uicheck.ps1 w9` 0;
`.\scripts\uicheck.ps1` with the default list 1 (`w1` blank window); checks of pull request 68 on
`d413651`: `macos`, `macos (amd64)`, `macos (arm64)`, `windows` pass.

1. L1, met. `cmd\burnmon-dev\lamp_unsupported.go` prints "This build has no lamp window. The lamp
   runs in the Windows build and in the Mac app (the build with cgo)."; no "Windows only". The
   README Limits paragraph and the pin in `cmd\burnmon\readme_test.go` changed in the same commit
   `c449bb4`; `TestLampUnsupportedSaysTheSentenceAndExits2` pins the sentence.
2. L2, met in code and tests, behaviour not measurable. `lampPointIn` has `TestLampPointIn` and
   `TestLampPointInTickerAndGlim`; the darwin file keeps the rectangles, polls every 100 ms and calls
   `setIgnoresMouseEvents:` only on a change, keeps the clicks while a button is down (source tests
   in `lamp_window_darwin_src_test.go`). Built on the macos runner (checks pass), not started:
   whether clicks really pass through on a Mac is not measured.
3. L3, met. Equal headline and line show once with the full text as title (page tests of T3);
   `uicheck-lamp.ps1` exit 0 in T3 and again in T4. Row 1 on a Mac: not seen.
4. L4, met. Every number cell carries its full value as a title (`sessions_layout_test.go`); `w9`
   large values (1,234,567 calls, 9,876.54 EUR) measured at 960 x 611 on Windows (WebView2): What it
   was 206 px, box scroll 905 = client 905 (no horizontal scrollbar), no cut header, no cut cell
   without a title.
5. L5, not met. `w9` is in the default list (line 8 and line 56 of `scripts\uicheck.ps1`), but the
   default run exits 1: the burnmon.exe pass stops at `w1` ("window looked blank/unpainted 1.5s after
   launch"), so `w2` to `w9` do not run in it. `w1` is red since the baselines of 2026-11-16 and
   2026-11-17 (STATUS); no burnmon.exe code changed in this lane. Not repaired here: outside the
   lane's items and file lists. `w9` alone exits 0.
6. Met. vet, test and build 0, linux and darwin without cgo 0, every check of pull request 68 green
   on `d413651`; the checks on the T4 commits are read after propose (turn log).
7. Met. Two earlier tests changed, both named in the T1 turn log with the reason (the old
   sentence): `TestLampUnsupportedSaysTheSentenceAndExits2` and the pin in `TestReadmeLampSection`.
   No golden file changed.
8. Met. Pull request 68 is open on `work/wilco-de-tree/mac-leftovers`; nothing landed; no tag on
   origin points at or contains a lane commit (`git ls-remote --tags origin`, newest
   `v0.5.0-beta.1`).

Choice in T4: no reviewer subagent, because the T4 diff holds only documents (STATUS, SESSION_LOG,
this plan); no behaviour or test can change through it.

## Turn log

T1: done; commit c449bb4; measured: vet 0, test 50 packages ok, 1849 passed, 12 skipped, 0 failed, build 0, linux and darwin build and vet without cgo 0, uicheck default exit 1 (w1 blank window, not this lane's), w0 and w2 to w9 exit 0; pull request 68 opened; changed tests: TestLampUnsupportedSaysTheSentenceAndExits2 and the TestReadmeLampSection pin now hold the new sentence (both failed first); reviewer: no behaviour change outside L1 and L5, no test weaker; open: w1 red keeps success criterion 5 open, the Mac card-window stub prints the new sentence (T4 flag)
T2: done; commit d51f543; measured on Windows: vet 0, test 0 failed, build 0, linux and darwin build and vet without cgo 0, gofmt clean; tests first (lampPointIn and the darwin source tests failed to build/pass before the code); no earlier test changed; no uicheck run (no page touched); darwin code built on the macos runner, not started, `macos` check read after propose; choices: the poll runs only for the clear backdrop (reviewer: card stays unchanged), pointer floored not truncated (reviewer), a lampSize height of 6 or less clears the rectangles (the sliver sends no new area; the page sends area before size on a grow, read in lamp.html lines 255 to 259), button down keeps the current state; open: a click within 100 ms of the pointer entering a rectangle can fall through (poll design, noted); the source test proves text, not behaviour

T3: done; commit bdd326a; measured on Windows (WebView2): vet 0, test 50 packages ok and 0 failed, build 0, linux and darwin build and vet without cgo 0, uicheck-lamp exit 0, uicheck w0 and w2 to w9 exit 0 (w1 not run, red since T1 and not this lane's); w9 large values (1,234,567 calls, 9,876.54 EUR) measured at 960 x 611: What it was 206 px, no horizontal scrollbar, no cut header, every number cell has a title; lamp on a Mac not seen, row 1 is checked as page text only; tests first (3 new, each failed before the code); no earlier test changed; no golden file changed; L3: equal headline and line show once (head title holds the full text), .head is flex:0 1 auto with min-width:0 and max-width:100% (reviewer: a flex:none head could overflow the row, changed), .sub shortens first, the Mac window width is unchanged (lampSize already follows the ticker); L4: titles hold the unrounded value (Per call 6 decimals, Cost 4, Tokens in full, Calls in full), none on a "no price" cell, the total row has none; w9 asserts a title on every number cell; reviewer: no behaviour change outside L3 and L4, no test weaker, orphaned comment moved back; open: row 1 equality is an exact string match, so a headline that differs by a space or case from the line shows twice; check_w9.go was already not gofmt clean at HEAD
T4: done; commit 1ed8e69 (grading, STATUS, SESSION_LOG) and 5657bdc (repair); measured on Windows on d413651: vet 0, test 50 packages ok, 1855 passed, 12 skipped, 0 failed, build 0, linux and darwin build and vet without cgo 0, uicheck-lamp 0, uicheck w9 0, uicheck default 1 (w1); grading: seven criteria met, L5 not met (default run stops at w1); repair round 1: the macos check failed on 1ed8e69 in TestDocCheckPassesOnRepoDocs because the STATUS paragraph held the hash d413651 in a code span, which doccheck reads as a uicheck id; the hash is now plain text, doccheck and the full tests 0 locally, all four checks of pull request 68 pass on 5657bdc (read 4 of 10); no stash; no reviewer (documents only); hub brief written in 04_assets; open: w1 red, the Mac card-window stub sentence, land and tag are Wilco's

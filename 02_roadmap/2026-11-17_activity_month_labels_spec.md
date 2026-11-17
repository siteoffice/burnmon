# Activity month labels

Flow `burnmon/activity-month-labels`, one step. Date: 2026-11-17. Source:
`C:\ZND\10_holding\04_assets\2026-11-17_burnmon_dev_ui_items.md`, item 1.

## Problem

In BurnMon Dev, Standard layout, the Activity grid shows its seven rows but the line with the month names under it is
gone.

## Cause: the source's reason is refuted, its outcome is right (measured 2026-11-17)

The source inferred that `.heatmapwrap` sizes to its content, so the scale stays 1 and the panel cuts the month line.
Measured with `panes/d51` on the unfixed build: the wrap is NOT content sized, it shrinks to the panel (64 px at
1500x900, 29 px at 1500x650). `pctFitCharts` does scale the grid, but `Math.max(0.3, scale)` floors the scale at 0.3,
so a grid that needs 0.15 stays 26 px tall in 13 px of room. The rows (top of the grid) show, the month line (bottom)
lies wholly below the panel edge, 6 px beyond it at 1500x650: 7 labels, 0 visible. `pctWhole` does not hide it
either (`pct-cut` is not set): a line wholly outside the edge is not "cut". At 1280x900 and 1500x900 the labels
already show, so Wilco's window is one with a short Activity panel (under about 60 px).

## What changes

1. A uicheck case `panes/d51` in `C:\ZND\50_projects\burnmon\tools\uicheck\group_panes.go`: Standard default layout,
   Activity filled with 120 days, at 1500x650, 1280x900, 1500x900, 1600x900, 1920x1080 and 2560x1440 (a size the screen cannot hold prints SKIPPED;
   `UICHECK_D51_SIZE=1500x650,1500x700` picks others). It asserts at least one month label with text
   lies inside the visible box of `#activityPanel` (the panel rect cut by every clipping ancestor). The decision is a
   pure function with a unit test in `group_panes_test.go`, written first.
2. Run d51 on the unfixed build: it must fail and its numbers confirm or refute the cause.
3. The fix in `page.html`: the `pctFitCharts` scale floor goes from 0.3 to 0.1, so the grid and its month line shrink
   to the room the panel really leaves.
4. Run d51 again: green. `go vet`, `go test`, `build.ps1` and the other uicheck groups give their baseline lines.

## What stays

The Heatmap harness x minute pane, the section percents, the fit engine for lists, everything else in `page.html`.

## How to check

`.\scripts\uicheck.ps1 panes/d51` red before, green after; `.\scripts\uicheck.ps1 dev` baseline lines per group
(compare against `main` for any group that fails: a known flake is named, never hidden).

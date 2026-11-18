---
type: plan
title: Glim sweat slider, plan and turn log (one turn, T1)
created: 2026-11-18
lane: C:\ZND\10_holding\02_roadmap\2026-11-18_glim_sweat_slider.md
branch: work/wilco-de-tree/glim-sweat-slider
---

# Glim sweat slider, plan and turn log

## Start

- Started from `main` at `568236c`, no tracked file changed. The branch was made with the siteoffice-git step `start glim-sweat-slider`. The guard check (raw `git push --dry-run`) was denied, so the guard is active.
- Baseline on `568236c`, measured: `go vet ./...` exit 0, `go test ./... -count=1` no FAIL line (48 packages ok in the first count, 50 in the later runs, the difference is packages with no test files counted by the grep, not a failure).

## Files changed

Save 1, the Go side with its tests (commit `fcce224`):

- `C:\ZND\50_projects\burnmon\internal\lamp\settings.go`: default `BurnFloorTPM` 300000, comment says what the key is.
- `C:\ZND\50_projects\burnmon\internal\lamp\demo.go`: the sweat scene uses a rate of 400000 over samples of 100000 (4 times the usual, above the default floor).
- `C:\ZND\50_projects\burnmon\cmd\burnmon-dev\lamp_child.go`: setter case for `lamp_burn_floor_tpm` (whole number of 0 or more, else refused); `lampGear` fills the floor.
- `C:\ZND\50_projects\burnmon\cmd\burnmon-dev\sections_view.go`: `BurnFloor` (`burn_floor`) in `lampGearPayload`.
- `C:\ZND\50_projects\burnmon\cmd\burnmon-dev\lamp_run.go`: `BurnFloor` in `lampLook`.
- Tests: `C:\ZND\50_projects\burnmon\internal\lamp\sweat_floor_test.go` (new), `settings_test.go`, `state_test.go`, `C:\ZND\50_projects\burnmon\cmd\burnmon-dev\lamp_child_test.go`.

Save 2, the page, the README, the logs:

- `C:\ZND\50_projects\burnmon\cmd\burnmon-dev\page.html`: slider `secLampSweat` and its number `secLampSweatNum` (short form, `sweatShort`), dim line, save on release, a poll does not move it while it has focus.
- `C:\ZND\50_projects\burnmon\cmd\burnmon-dev\lamp_gear_page_test.go`: `TestPageLampGearSweatSlider`.
- `C:\ZND\50_projects\burnmon\cmd\burnmon-dev\README.md`: the settings row and one sentence in Use.
- `C:\ZND\50_projects\burnmon\assets\glim\glim-demo.svg`: regenerated with `burnmon-dev.exe lamp --poses` (only this file changed of the ten; 128183 bytes, under 300 KB).
- This file, `STATUS.md`, `SESSION_LOG.md`.

## Tests, each seen failing first

- `TestSweatDefaultFloorIs300000`: failed (20000). `TestSweatRateUnderDefaultFloorDoesNotSweat`: failed (250000 sweated at the default). `TestDemoSweatsAtDefaultFloor`: failed once the default changed (demo rate 60000). Pass now.
- `TestSetLampKey` (extended): failed ("not a lamp setting of the gear"). Pass now.
- `TestPageLampGearSweatSlider`: failed (slider, range, step, number, line all missing). Pass now.
- The gear payload test (`TestSetLampKey`, `lampGear`) failed to compile before `BurnFloor` existed.

## Earlier tests changed, and why

1. `internal\lamp\settings_test.go` line 6 (`TestSettingsDefaults`): the expected default floor 20000 became 300000, because the default is the point of the change.
2. `internal\lamp\state_test.go`, `TestMoodOrder`, two rows ("your turn beats sweating", "sweating beats done"): the rate 90000 became 400000 with the same samples of 30000, because 90000 is under the new default floor and the rows meant "3 times the usual and above the floor". They still test the same order.
3. `cmd\burnmon-dev\lamp_child_test.go`, `TestSetLampKey`: the row `{"lamp_burn_floor_tpm", 5.0}` in the refused list said "a real lamp key, but not a gear key". It is now a gear key, so the row is replaced by refused rows for -1, 250000.5, a string, true and nil, plus a row for `lamp_retention_days` that keeps the "not a gear key" check; accepted rows for 0, 1000000 and 350000 were added.
4. `TestDemoTimelineMoodsInOrder` and `TestDemoShowsEveryStateThatLastsOnce` failed after the default change and passed after the demo change, with no edit to them.

## Choices on ambiguities

- The slider number is the number of the saved value in short form (`300k`, `1M`, `1.25M`, `75k`, `0`); `sweatShort` rounds 999950 and above to `1M`, found by the reviewer.
- The demo's token dots: the sweat scene has no agents, so it shows no dots, and the dot gap for any rate of 100000 or more is the clamp minimum of 0.3 seconds (`DotSeconds`, unchanged). The scene is as readable as before.
- No uicheck covers the gear group Lamp (searched `scripts` and `tools`), so none was extended.
- Two saves were made by holding the page, the page test, the README and the svg out of the tree during save 1 (the launcher commits every changed path).
- `sections_view.go` was already not gofmt clean at `568236c` (lines unrelated to this change); left alone.
- `internal\lamp\settings.go` line 8 says "twelve lamp keys" and was already stale; left alone.

## Measured

On the tree before save 1: `go vet ./...` 0; `go test ./... -count=1` 50 packages ok, no FAIL line; `.\build.ps1` 0; linux and darwin `go build ./...` and `go vet ./...` with CGO off, all 0; `.\scripts\uicheck-lamp.ps1` exit 0 on the first run (no flake). Built, not seen: the slider in the gear dialog, no screenshot; the page is checked as text.

## Turn log

T1: done; commits fcce224 (Go side) and 61c58bb (page, README, logs); measured: vet 0, test 50 packages ok, build 0, linux and darwin 0, uicheck-lamp 0 with no flake; open: nothing

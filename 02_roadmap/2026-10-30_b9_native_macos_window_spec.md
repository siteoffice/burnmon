# BurnMon B9, native macOS window for burnmon.exe and BurnMon Dev

Date: 2026-10-30. Owner and approver: Wilco de Tree. Runs after B8 is merged. Roadmap row 14.
Decided by Wilco 2026-10-30: B9 comes right after B8; the window library is webview_go
(`..\04_assets\2026-10-30_b9_macos_webview_research.md`); both `burnmon` and `burnmon-dev` get a
native macOS window; testing happens on a Mac rented online for 24 hours (renting is Wilco's step).
This replaces the line in `cmd\burnmon-dev\main_other.go` that says BurnMon Dev is Windows-only.

## Why
On macOS both apps either do not exist (BurnMon Dev) or open in the browser from a 127.0.0.1 server
with a per-run secret (burnmon, PR #4). A native window removes the open port on macOS, keeps the
"no ports" promise, and gives Mac users the agent monitor.

## What the code looks like today (read 2026-10-30)
- `cmd\burnmon`: the window is `go-webview2` with 10 `Bind` calls in `main.go`; `main_other.go`
  serves the same page to the browser on macOS and Linux.
- `cmd\burnmon-dev`: 18 files are `//go:build windows` (app, main, sections, agent map, controls,
  snapshot writer, station theme, To Do gate, uicheck dev server, view and window state, and more);
  `main_other.go` is a stub that prints that it is Windows-only.
- `internal`: Windows-only with a non-Windows twin already in place: `controls` (cmdline, sys),
  `graph` (listen), `snapshotwriter` (hide, lock), `watch`. Windows-only with a stub on other systems:
  `sysmon` (sample, process, pressure, machine profile, Wi-Fi). `gopsutil/v4` is already in `go.mod`.
- Builds are `CGO_ENABLED=0`. webview_go needs cgo, so macOS builds happen on macOS.
- CI: `.github\workflows\release.yml` runs on `ubuntu-latest` only.

## Items

### 1. One window layer, two backends
Split the window code from the app code in both commands: the binding names and the page stay
shared, the window type comes from `go-webview2` on Windows and from webview_go on macOS
(`//go:build darwin`). Linux keeps the browser mode until roadmap row 17.
Done when: the Windows build and `.\scripts\uicheck.ps1 dev` are unchanged and green, and every
binding has one shared registration list with a test that both backends register the same names.

### 2. BurnMon Dev builds and runs on macOS
Move the platform-free parts of the 18 Windows-only files into shared files. Sample the System zone
on macOS through `gopsutil/v4` (CPU total and per core, memory, disks, network, process groups);
show "not on macOS" for what has no documented source there (for example the Windows pressure
score and Wi-Fi details). No undocumented API, no new dependency besides webview_go.
Done when: unit tests for the macOS sampler on recorded inputs; `GOOS=darwin go vet ./...` clean
from Windows for every file that does not need cgo.

### 3. macOS compile check in CI, before the rental
Add a `macos-latest` job that builds both apps with cgo and runs `go test ./...`. Whether runner
minutes cost money for this repo is not checked; Wilco confirms before the job is enabled.
Done when: the job is green on the sprint branch. The rental does not start before this.

### 4. A scripted test run for the rental day
One script for the Mac (`scripts/maccheck.sh`) that installs nothing silently, builds both apps,
runs the tests, starts each app with its eval channel on, runs every uicheck case that only reads
the DOM, takes screenshots with `screencapture`, and writes one report file. A short manual list
covers what a script cannot judge (window feel, menus, sleep and wake, a second screen).
Done when: the script runs end to end in CI apart from the window steps, and the manual list is in
the report template.

### 5. The rental day (Attended, Wilco)
Wilco rents the Mac. In the 24 hours: run `scripts/maccheck.sh` on real Claude Code and Codex
sessions, work the manual list, fix what breaks with a test first, and re-run.
Done when: the report holds the script output, screenshots of both apps, and one line per manual
check; every fix is committed on the sprint branch.

## Out of scope
The B8 toast on macOS (a notification on macOS likely needs an app bundle and signing; not checked).
Signing and notarization. A Linux window (row 17). Apple silicon and Intel in one universal binary:
the build targets the rented Mac's architecture and says which.

## Rules for the run
Tests first. Windows verify as always: `go vet ./...`, `go test ./... -count=1`, `.\build.ps1`,
`.\scripts\uicheck.ps1 dev`. Say what was measured on the Mac and what was only inferred. Run mode:
Unattended for items 1 to 4, Attended for item 5. No tag at the end.

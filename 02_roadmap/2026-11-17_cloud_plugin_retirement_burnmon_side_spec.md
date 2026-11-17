# BurnMon side of the burnmon-cloud plugin retirement

Flow `burnmon/burnmon-side-of-the-burnmon`, one step. Date: 2026-11-17. Source:
`C:\ZND\50_projects\burnmon\04_assets\2026-11-17_burnmon_cloud_plugin_retirement.md`, section 4.

## What changes

1. `scan.EnsureCloudDropDir` is removed, with its three callers
   (`C:\ZND\50_projects\burnmon\cmd\burnmon\main.go`, `main_other.go`, `cmd\burnmon-dev\main.go`).
   BurnMon no longer creates `~\.burnmon\cloud` or the exe-adjacent drop folder at start.
2. The folder is still read and watched when it exists: `CloudDropDirs`, the sources list, the drop parser
   and the "not measured" marker stay. Old rows stay readable.
3. `C:\ZND\50_projects\burnmon\cmd\burnmon\README.md`: the section "Cowork in the cloud" becomes a short note
   (the plugin is retired, use Read Cowork (Cloud) sessions). The table row for the drop folders says it is no
   longer created. The doccheck allowlist and doc list follow the README.
4. `tools\burnmon-cloud` moves to `C:\ZND\50_projects\burnmon\_archive\burnmon-cloud` with `git mv`, tests
   with it. This is a delete of the old path, so it waits for Wilco's yes.

## What stays

The drop parser, the 8,911 stored drop rows, `CloudLastMeasured` and the vendor strip marker, the cloud pull.

## How to check

- Tests first: a test that `EnsureCloudDropDir` is gone is not needed; the old create-folder tests are
  rewritten to assert that nothing creates the folder and that an existing folder is still a source.
- `go vet ./...`, `go test ./... -count=1`, `.\build.ps1`. No UI change, so uicheck only as a baseline.
- Gate 2: Wilco confirms the move before it runs.

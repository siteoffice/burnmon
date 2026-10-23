---
type: hub_agent_update
project: burnmon
created: 2026-10-23
topic: R4 snapshot file, schema version 1 frozen
---

# R4 done: snapshot file, schema version 1 frozen

Plan row R4 of `C:\ZND\10_holding\02_roadmap\2026-10-22_agent_control_plan.md`, brief
`C:\ZND\10_holding\04_assets\2026-10-23_to_burnmon_lane_a_r4_r6_r7_build_brief.md`.

## What exists (branch `feat/lane-a-r4-snapshot`, nothing committed)

- `C:\ZND\50_projects\burnmon\internal\snapshot\snapshot.go`: schema in code, Build, atomic Write, Read, IsStale.
- `C:\ZND\50_projects\burnmon\cmd\burnmon\snapshot.go`: writer goroutine, one write per 5 s tick to `%USERPROFILE%\.burnmon\snapshot.json`; a failing input is reported in `sources[]`, the file is still written.
- Wired into `cmd\burnmon\main.go` and `cmd\burnmon\main_other.go`.
- `C:\ZND\50_projects\burnmon\docs\snapshot_schema.md` and `C:\ZND\50_projects\burnmon\testdata\snapshot_example.json` (lane B fixture).

## Measured

- Tests written first and seen failing (compile errors, feature missing), then passing: 11 in `internal\snapshot`, 3 in `cmd\burnmon`.
- `go vet ./...` clean, `go test ./... -count=1` all packages ok, `.\build.ps1` exit 0, `GOOS=linux go vet` of `cmd\burnmon` and `internal\snapshot` clean.

## Not checked live

- The built `burnmon.exe` was not started, so no snapshot file from a real run has been seen, and the per-tick CPU cost of the process sampler inside the core is not measured.
- The Fleet mod reading the file: not built here.
- `cost.usd` is the `live.Session.Cost` figure the Now page already uses. Whether that is the right "API price equivalent" basis (versus `BusinessCost`) is not decided.

## Open

- `docs\` is in `.gitignore` (line 12), so `docs\snapshot_schema.md` will not be committed unless force-added or un-ignored.
- Tokens first, then plan limits, then cost: version 1 has no plan limit field.

# Cowork (Cloud) not measured marker, step spec

Source: `C:\ZND\10_holding\04_assets\2026-11-17_burnmon_cowork_cloud_gap_research_and_proposal.md`, section 9.

## What changes

- `internal\scan\clouddrop.go`: `CloudLastMeasured(dirs []string) (time.Time, bool)`, the newest
  row `ts` over every `.jsonl` in the drop folders. `false` when there is no row.
- `internal\vendorstrip`: `Row` gets `today_note`, `week_note`, `month_note` (`""`,
  `"not_measured"`, `"partial"`). `Payload` gets `cloud_measured_until` (display text, empty when
  unknown). Only the `cowork-cloud` row and the Total row can carry a note. Rule, L = last
  measured, S = period start: S after L gives `not_measured`; S at or before L gives `partial`.
  Total carries `partial` where the cloud cell does. No drop row: no note anywhere.
- `cmd\burnmon-dev\page.html` and `internal\report\template.html`: a `not_measured` cell prints
  `not measured`; a `partial` cell prints the number and `+`; tooltip "measured until <date>,
  later cloud sessions are not measured".

## What stays

Numeric fields, headline, snapshot, store rows, adapter, History, and everything under
`tools\burnmon-cloud`.

## How it is checked

Go tests for the scan function and the note rules (fixture: L = 2026-11-12, now = 2026-11-17,
expect not measured, not measured, month partial, Total month partial). Page string tests. One new
uicheck case. `go vet ./...`, `go test ./... -count=1`, `.\build.ps1`, `.\scripts\uicheck.ps1`.

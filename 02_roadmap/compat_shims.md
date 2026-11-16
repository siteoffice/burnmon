# Compatibility code with an end date

Code that stays only so old installs and old files keep working. Each row ends in one
pull request with the v0.5.0 release, which removes it. End version: v0.5.0, decided 2026-11-15
(audit batch G, finding 7). Line numbers are from 2026-11-16 on branch
work/wilco-de-tree/audit-batches-night-1116 and move with later edits.

| Shim | Where | Kept for |
|---|---|---|
| `claudecost.json` as a config name | `C:\ZND\50_projects\burnmon\internal\pricing\findconfig.go` 10 (`configNames`), used through `pricing.FindConfig` by `cmd\burnmon\main.go` 109, `cmd\burnmon\main_other.go` 57, `cmd\burnmon-dev\app.go` 423, `cmd\burnmon-cli\main.go` 1119; the Control Centre follows the same order in `cmd\controlcentre\config.go` 23 | installs of v0.1.2 and earlier, before 2026-09-22 |
| The R8 keys, `r8Fill` and `migrateR8` | `C:\ZND\50_projects\burnmon\internal\sections\sections.go` 222 to 261 | view files of an early alpha |
| The `heights_*` keys removed on save | `C:\ZND\50_projects\burnmon\internal\sections\sections.go` 323 and 324 | view files from before PR #28 |
| `PurgeAuditSessions` on every Collect | `C:\ZND\50_projects\burnmon\internal\dataset\dataset.go` 660 (store side: `internal\store\store.go` 1389) | a bug fixed on 2026-10-12 |

The v0.5.0 pull request deletes each row above, its tests, and then this file.

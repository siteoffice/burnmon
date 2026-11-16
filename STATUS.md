# BurnMon, status

What is true on branch work/wilco-de-tree/audit-batches-night-1116 (2026-11-16, pull request 40, head `21a1ae8`, not merged; read `git log` and `gh pr view 40` live): the night run built audit batches A to G as behaviour-preserving cleanup in nine commits: safe write for the view and window files (`internal\fsutil`), six unused functions and the old "Claude Cost" text removed, one config lookup (`pricing.FindConfig`) and one data folder rule (`store.DataDirFrom`), one reader for `.git` files and remotes (`gitrefs.WorktreeGitDir`, `gitrefs.Remotes`), history moved out of STATUS and roadmap, `02_roadmap\compat_shims.md` with end version v0.5.0, and the shared pollers, snapshot writer, controls Env, elevation text and single-instance pair (`internal\ingest`, `controls.NewEnv`, `controls.WhyElevated`, `internal\instance`). Measured on the branch head by the T6 turn: `go vet` 0, `go test` 1491 pass, 0 fail, 6 skip (one earlier run of the same command in the same turn reported one failed test that a rerun did not repeat; not identified), `.\build.ps1` 0, linux build and vet 0, darwin build 0, `macos` check on the pull request pass. Not measured: uicheck, and a live run of any app. The items left for an attended day run are listed in the night plan in the hub.

What is true at v0.4.0-beta.4 (2026-11-15, written on the release branch before its merge; read `git log main`
and `git ls-remote --tags origin` live): the release commit sets the three exes, controlcentre.exe and the
`winres\*.json` ProductVersion to `0.4.0-beta.4` (numeric `0.4.0.0` unchanged; controlcentre has no Go version
constant, its version is the winres value). Since beta.3 `main` gained, each through siteoffice-git after
Wilco's Accept: Control Centre slice 1 (PR #23, `803c952`), its first look fixes (PR #25 and #26), the
Control Centre root set in Settings and the Git page (PR #27, `070ac3d`), the BurnMon Dev percent layout
(PR #28, `204a344`), and the first and second look fixes with Stop as BurnMon Dev's only control (PR #29,
`f278675`). Also merged since beta.3: B9 native macOS window items 1 to 4 (PR #21, `0a4b370`; nothing has run
on a Mac) and the README comparison with Claude Desktop (PR #22, `1c2e359`). Measured on this release tree
before the commit: `go vet` 0, `go test` 1407 pass, 0 fail, 6 skip, `.\build.ps1` 0.
The paragraphs below are kept as history.

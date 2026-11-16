# BurnMon, status

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

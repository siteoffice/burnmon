# PR #4: macOS and Linux localhost bridge, decision and conditions

Date: 2026-10-21. Decided by: Wilco de Tree. PR: https://github.com/wilcodetree/burnmon/pull/4
(merkendiep, branch `merkendiep:macos-support`, head `3292cb1`, base `2e20ebb`).

## Decision

Accepted: BurnMon on macOS and Linux may run one local web server on 127.0.0.1 so the
browser-mode dashboard gets the same live functions as the Windows WebView2 window. Windows
keeps "no server, no open port": the server code (`cmd/burnmon/browser.go`) has the build
tag `!windows` and is not in `burnmon.exe`. This replaces roadmap row 10a (hide the Refresh
button in browser mode).

Order changed later on 2026-10-21 (Wilco): merge first, then add conditions 1 and 2 on
`main` ourselves. No tag or release until both are on `main`. Condition 3 is met (see
"Windows verify" below).

Park exception (Wilco, 2026-10-21): conditions 1 and 2 built the same day on branch
`fix/pr4-bridge-secret`, see `SESSION_LOG.md`.

The three conditions:

1. A per-run secret. `burnmon` makes a random 32-byte token at start, writes it to a file
   only the user can read (mode 0600) in its data folder, and opens
   `http://127.0.0.1:<port>/?t=<token>`. Every request, the page included, needs that token
   (query on the page, a header on `/api/*`). A second start that finds the port taken reads
   the token file and opens that URL; if the running server does not answer with a valid
   BurnMon response, it says the port is used by another program and names `-port`.
   Reason: localhost is shared by every user and process on the machine, and the page itself
   holds token, project and session data.
2. The README says it plainly (text below).
3. Verified on Windows after the park ends (2026-11-01): `go vet ./...`,
   `go test ./... -count=1`, `.\build.ps1` and `.\scripts\uicheck.ps1`, because the PR moves
   the Windows bindings from `main.go` into `bindings.go`.

## What was measured on 2026-10-21

In a Linux build container, on the PR head: `go vet ./...` clean, `go test ./... -count=1`
all packages pass; `GOOS=windows go vet ./...` and `go build ./...` clean; `GOOS=darwin
GOARCH=arm64 go build ./...` clean. Not measured: any run on Windows, `build.ps1`, `uicheck`,
the PR's GitHub checks (not readable from this session). The contributor reports
`go test ./...` green on macOS arm64; that is his claim, not checked here.

## README text required before merge

Opening paragraph, replace the sentence that ends "everything stays on this device." with:

> No API key, no admin rights, no network calls, no telemetry: everything happens by reading
> local files and everything stays on this device. On Windows there is also no server and no
> open port. On macOS and Linux, `burnmon` runs one web server that only this computer can
> reach (127.0.0.1); see "macOS and Linux".

Windows section, "No open ports, no admin rights" becomes "No open ports on Windows, no admin
rights".

"What BurnMon never does", first sentence becomes:

> No server on Windows, no network call to the internet of any kind, no telemetry, no
> account. On macOS and Linux there is one local web server on 127.0.0.1 for the dashboard,
> protected by a secret that changes every start.

"macOS and Linux" section, add after the paragraph that names the port:

> **The one open port.** On macOS and Linux, `burnmon` listens on `127.0.0.1:47615`. Other
> computers cannot reach it. Other users and programs on the same computer can reach the
> port, so every request needs a secret that `burnmon` makes at each start and keeps in a
> file only you can read. If another program already uses port 47615, start with
> `-port <number>`. The Windows app has no server and no open port.

## Accepted as part of the same PR

The Copilot CLI NULL token columns fix (`91607a3`), the arm64 context runway fix
(`4da6626`) and the tests that pass off Windows (`f51657b`). No conditions beyond item 3.

## Windows verify, 2026-10-21 (run by Wilco on the laptop)

PR head `3292cb1` in a separate worktree: `go vet ./...` clean, `go test ./... -count=1`
green in every package, `.\build.ps1` built all three exes, `.\scripts\uicheck.ps1` failed
w1 ("window looked blank/unpainted 1.5s after launch"). Same check on `main` (`2e20ebb`),
run twice: w1 failed both times the same way. So w1 is the known open item in STATUS.md, not
a regression from PR #4.

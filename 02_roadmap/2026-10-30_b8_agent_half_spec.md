# BurnMon B8, the agent half: correct, fast, and it tells you

Date: 2026-10-30. Owner and approver: Wilco de Tree. Base: `main` at `775e0b5` (`v0.4.0-beta.1`).
Decided by Wilco 2026-10-30: one sprint on the agent half after the beta, items 1 to 4 as
recommended, plus a toast (item 5). Applies to `burnmon-dev.exe`; shared code lives in `internal\`.

## Why

The beta shows waits, gates and agents, but on the owner's laptop the agent view is empty, a new
gate can take up to about 15.7 s to reach the page (B3 report, calculated, not measured), the one
your-turn item shows a raw session UUID as its title, and no control has ever run against a real
session. An agent monitor is only useful when what it shows is right and arrives while it matters.

## Items

### 1. Agent view finds claude.exe behind the npm shim
Facts read 2026-10-30: `internal\snapshotwriter\resolve.go` refuses a `.cmd` on purpose (a timeout
would kill only cmd.exe). `agent_view` is off by default; `claude_path` already exists as the manual
fix; the README says the shim names `<npm prefix>\node_modules\@anthropic-ai\claude-code\bin\claude.exe`.
Change: when `claude` resolves to `claude.cmd`, read the shim as text, take the `claude.exe` path it
names, and accept it only if that file exists and ends in `.exe`. Never run the shim. When the agent
view is off or refused, the page says why in one line, with the setting that fixes it.
Done when: unit tests cover a real-shaped npm shim, a shim naming a missing file, a shim naming a
non-exe, and a `.ps1`; on the laptop with `agent_view` on, the live snapshot has `agent_sessions`
above 0 while a background agent runs (measured, snapshot excerpt in the report).

### 2. A new Foreman gate reaches the page in under 2 s
Facts: Foreman is read every 7.5 s (`foremanEvery` in `internal\snapshotwriter\writer.go`), one read
costs 250 to 680 ms, the snapshot is written every 5 s, the page polls every 5 s.
Change: watch the Foreman `sprints` and `flows` folders with the existing Windows watcher
(`internal\watch`), and on a change read Foreman and write the snapshot at once (200 ms debounce).
Keep the 7.5 s read as the fallback. The page reads waits on its own 1 s tick.
Done when: a harness touches a test sprint's `sprint.json` 10 times and the item shows on the page
in under 2 s every time (p95 and max in the report); the idle CPU of the Go process stays under
the 2 percent target over 10 minutes.

### 3. Waits on you reads right
Change: a session item never shows a raw ID as its title; it shows the project folder name and an
8-character short ID. The Done group (close-out and branch-gone items) starts collapsed with its
count, so Blocked and Your turn are what you see first. No change to what counts as the amber number.
Done when: tests on `internal\waits` for the title rule and a uicheck case for the collapsed Done group.

### 4. Every control runs once for real (Attended)
Change: none planned. Run `approve` and `send_back` on a throwaway Foreman sprint made for the test,
and `stop` and `attach` on a background agent started for the test, never on real work. Fix what breaks.
Done when: the report has one line per control (command preview, result, audit line), and any fix
has a test first.

### 5. A toast when something waits on you
Change: `burnmon-dev.exe` shows a Windows notification through `Shell_NotifyIcon` (balloon, shown as
a toast on Windows 10 and 11) from a hidden message window on its own locked OS thread, never the
WebView2 UI thread (the B7.2 freeze). It fires when an item enters Blocked or Your turn, never for
Done; one per item ID per 10 minutes; several in one tick become one toast ("3 things wait on you").
A click brings the window to the front. Text holds the kind and title only: no prompt text, no
command, no path. Off switch `toast_enabled` in `burnmon-dev.json`, default on, also in the gear
dialog. No new dependency (`golang.org/x/sys` is already in `go.mod`), no network, no port.
Done when: unit tests for the fire, dedupe and batch rules on a fake clock; a manual check on the
laptop with a screenshot of one toast in the report.

## Out of scope
The reply control. Closing the 19 waiting sprints (a hub task). Station work. macOS and Linux toasts.
w1. Plan-limits panel.

## Rules for the run
Tests first. Verify with `go vet ./...`, `go test ./... -count=1`, `.\build.ps1` and
`.\scripts\uicheck.ps1 dev`. Say what was measured and what was inferred. No undocumented vendor
endpoints. Run mode: Unattended for items 1, 2, 3 and 5; Attended for item 4 (Wilco present).
No tag at the end; the next tag is decided after the report.

## Next step for this file
Copy to the Foreman specs folder and pass `spec_lint.py`. Not done in this session: `C:\ZND\10_holding`
was not connected.

![Glim, the lamp of BurnMon Dev, plays a 48 second demo loop](assets/glim/glim-demo.svg)

# BurnMon

BurnMon is a vendor-agnostic token and cost monitor for your own AI coding sessions. It reads the
transcripts your tools already write on this machine, so it needs no API key, no network and no account.
BurnMon itself makes no network call. With Read Cowork (Cloud) sessions on, it starts your own Claude Code CLI,
which goes online as you (`cmd\burnmon\README.md`, section Cowork (Cloud) sessions).

BurnMon is the successor of [claudecost](https://github.com/siteoffice/claudecost), renamed and extended
from v0.2 onward. Repo: [siteoffice/burnmon](https://github.com/siteoffice/burnmon). Current release:
`v0.4.0-beta.7`.

## Which app do I open?

| App | Open it for | Read |
|---|---|---|
| **BurnMon** (`burnmon.exe`) | history: tokens and cost per day, vendor, project and client, with its live Now page for a glance | `cmd\burnmon\README.md` |
| **BurnMon Dev** (`burnmon-dev.exe`) | the last 30 minutes and acting now: Waits on you and the agent map | `cmd\burnmon-dev\README.md` |
| **The lamp** (`burnmon-dev.exe lamp`) | a slim ticker at the top edge of the screen with Glim hanging under it, that shows when an agent waits on you, and nothing else; drag it to another monitor, grab Glim, close it with a right-click (Windows; the Mac panel is built, not seen) | `cmd\burnmon-dev\README.md` |
| **Control Centre** (`controlcentre.exe`) | steering: the units, files, plan and boards under one root, gate decisions, Stop, comments, a Git overview | `cmd\controlcentre\README.md` |
| **CLI** (`burnmon-cli.exe`) | scripts, exports and merges | `cmd\burnmon-cli\README.md` |

Settings live in `burnmon.json`, next to the exe or else in `%LOCALAPPDATA%\burnmon\burnmon.json`; see `cmd\burnmon\README.md`, section 5.

The Claude desktop app can show your other Claude Code sessions that run in the app, up to the 20 most recent. It cannot see sessions you started in a terminal or in VS Code, and it cannot see other coding tools at all. BurnMon Dev reads every Claude Code, Codex and Copilot session on this machine, wherever it runs, and shows which ones are waiting on you.

## Screenshots

![BurnMon Now page: live burn chart, session cards and vendor strip](assets/screenshots/now-page.png)

<img src="assets/screenshots/dev-station.png" alt="BurnMon Dev station view with system panels" width="560">

Both screenshots were committed on 2026-10-07 (`git log`), before the B-series changes (sections, Waits on you,
Outcomes, controls). New screenshots are not made yet.

## Install and build

Windows: `.\build.ps1` from `C:\ZND\50_projects\burnmon` builds all four exes. macOS and Linux: each tagged release
attaches `burnmon` and `burnmon-cli`. Details are in each README, section 3.

## More

- Status now: `STATUS.md`. Priorities: `02_roadmap\roadmap.md`. Dates: `DEADLINES.md`.
- The wait contract other tools read (Groundwork's Fleet): `docs\snapshot_schema.md`.
- Bugs and ideas: https://github.com/siteoffice/burnmon/issues. Security problems: mail info@zerononsense.dev.
  Licence: see `LICENSE`.

# BurnMon CLI

`burnmon-cli` is the single-file command line of BurnMon. It builds the dashboard, prints the
live snapshot as JSON, writes client-safe exports and merges them.

Parent index: `README.md`. Settings file and transcript roots: `cmd\burnmon\README.md`.

## 1. What it is

The CLI reads the same local session transcripts as the BurnMon app and keeps them in the same
SQLite store. It is for scripts, for a second opinion on a number, and for machines with no window.
It makes no network call, needs no account and needs no admin rights.

## 2. Where it fits

| Part | Job | Needs the CLI |
|---|---|---|
| BurnMon (`burnmon.exe`) | history: tokens and cost per day, vendor, project, client | no |
| BurnMon Dev (`burnmon-dev.exe`) | the last 30 minutes, Waits on you, the agent map | no |
| Control Centre (`controlcentre.exe`) | steering: units, boards, gate decisions | no |
| `burnmon-cli` | scripts, exports, merges, one-off reports | this part |

The CLI needs nothing from the other three. It does not need BurnMon to run. Two subcommands read
what a running app wrote: `burnmon-cli waits -json` without a Foreman folder reads the snapshot file.

## 3. Install

From a release (macOS and Linux): the release workflow attaches `burnmon-cli-<os>-<arch>` to each tagged
[release](https://github.com/siteoffice/burnmon/releases). Copy it anywhere. No installer. It does not
attach Windows files: on Windows, build from source.

From source, on Windows (this makes `burnmon-cli.exe` and the other exes):

```powershell
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon
Set-Location C:\ZND\50_projects\burnmon
.\build.ps1
```

From source, on macOS or Linux (Go installed):

```bash
# runs in: bash, cwd = your clone of siteoffice/burnmon
go build -o burnmon-cli ./cmd/burnmon-cli
```

## 4. Use

Run `burnmon-cli` with no argument and it builds the dashboard and opens it in the browser. The
first ten minutes: run it, look at Now and Sessions, then run `burnmon-cli price-check` to see how
current each price is. Every subcommand has its own `-h`.

- `burnmon-cli`: build the dashboard and open it in the browser.
- `burnmon-cli live -json`: one JSON snapshot of the Now page, for scripting.
- `burnmon-cli tools -since 30d`: tool_calls totals: tool, calls, sessions, bytes.
- `burnmon-cli insight -json <session-id>`: findings for one session, table or JSON.
- `burnmon-cli reown`: re-apply owner and client rules to every event in the store.
- `burnmon-cli price-check`: print every price book and when it was last checked.
- `burnmon-cli export -owner NAME`: write a client-safe daily-rows export, see "Export and merge".
- `burnmon-cli merge FILE... -out DIR`: combine exports into `merged.json` and an offline `report.html`.
- `burnmon-cli graph -since 24h`: print the relation graph for a past window as JSON (sessions and
  subagents from the store, the live-only edges it recorded, and Foreman sprints when a Foreman folder
  is set). `-foreman <dir>` overrides `foreman_root`.
- `burnmon-cli waits -json`: list what waits on you and the outcomes of the last 24 hours from
  Foreman's state, read only. With `foreman_root` or `-foreman <dir>` its Foreman items are the same
  as in the Waits on you and Outcomes sections of BurnMon Dev; session and agent view items are not
  in its output. Without a Foreman folder it lists the wait items in the running app's snapshot
  (`-snapshot <file>` overrides the path), with no outcomes, and prints a note and exits 0 when
  there is no snapshot yet.
- `burnmon-cli backfill-parents`: set the parent session on Claude subagent rows stored before the
  adapter read it. A dry run that only counts rows; `-apply` writes. Copy the store file first.

### Export and merge

`burnmon-cli export --since <date> --until <date> --owner <name>... --label <name> --out <file>`
writes daily rows: owner, client, vendor, model, token classes, cost on every basis available,
active minutes. Paths, session ids, prompts and project names never leave in the file. When `owners`
rules exist, `--owner` is required, so a row you did not ask for never leaves by accident.

`burnmon-cli merge <file>... --out <dir>` combines any number of exports (free, no cap) into
`merged.json` and a `report.html` that opens offline with no network script. It has one column per
export `--label`, and totals by client, vendor and week.

Both apps write the same file as `burnmon-cli export`, from their Settings. You type the folder as
an absolute path (it starts as your Downloads folder) and can set since, until and a label. When
`owners` rules exist, tick at least one owner.

## 5. Settings

The CLI reads `burnmon.json`, the same file as the app: next to the exe, else in
`%LOCALAPPDATA%\burnmon\burnmon.json`. Every key is in `cmd\burnmon\README.md`, section 5.

`burnmon-cli` also keeps every claudecost-era flag: `-months`, `-seat`, `-no-open`, `-json data.json`,
`-source DIR`, `-no-cache`, `-out DIR`, `-config`, `-version`. See `burnmon-cli -h` for the full list.

## 6. Limits and known issues

- `waits` and `graph` show Foreman items only with a Foreman folder. Without one, `waits` shows the
  wait items of the running app's snapshot and no outcomes.
- History is slow on a large store: every filter change re-reads it, about 3 seconds at roughly
  55,000 events.
- `backfill-parents` changes the store. It is a dry run until you pass `-apply`, and there is no undo.

## 7. Next steps and backlog

Nothing is planned for the CLI alone. It follows the apps: a new app feature that has a scriptable
form gets a subcommand. Priority order lives in `02_roadmap\roadmap.md`.

## 8. Feedback, licence, contact

Open an issue at https://github.com/siteoffice/burnmon/issues, or use the templates described in
`README.md`. Never paste prompts, keys or customer data. Licence: see `LICENSE`. Security problems:
mail info@zerononsense.dev, not a public issue.

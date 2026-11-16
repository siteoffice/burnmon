# burnmon

BurnMon, the vendor-agnostic token and cost monitor, a ZeroNonsense.dev product. Successor of
claudecost, forked 2026-09-22. Owner and approver: Wilco de Tree. Language: English.

It reads the session trails of Claude Code, Cowork, Codex, Copilot CLI, Copilot in VS Code
and Hermes from local disk, keeps them in SQLite, and shows tokens and cost per vendor,
session, project and client. Portable Windows exe, no account, no network, no ports on Windows. Four
binaries: `burnmon.exe`, `burnmon-cli.exe`, `burnmon-dev.exe`, `controlcentre.exe`.

Root tree instructions: `C:\ZND\AGENTS.md`. Read it too.

## Source of truth

Never state status, version, deadlines or priorities from memory. Read the source.

| Question | Source |
|---|---|
| What is true now | `C:\ZND\50_projects\burnmon\STATUS.md` (top block only; older sections are in `C:\ZND\50_projects\burnmon\03_logs\status_history.md`, they can lag, say so) |
| Priority order | `C:\ZND\50_projects\burnmon\02_roadmap\roadmap.md` |
| Hard dates | `C:\ZND\50_projects\burnmon\DEADLINES.md` |
| Last sessions | `C:\ZND\50_projects\burnmon\SESSION_LOG.md` |
| Hub view, park state | `C:\ZND\10_holding\01_projects\burnmon.md` |
| Cross-project decisions | `C:\ZND\10_holding\03_logs\decisions.md` |
| Plan | `C:\ZND\50_projects\burnmon\02_roadmap\2026-09-22_burnmon_plan.md` |
| Architecture (decided) | `C:\ZND\50_projects\burnmon\04_assets\2026-09-22_token_monitor_architecture.md` |

If two sources disagree, say so. Do not pick one silently. Before any build work, check
`DEADLINES.md` and the park decision in the hub one-pager.

## Where files go

Specs and plans in `02_roadmap`, briefs and proofs in `04_assets`, date-prefixed
`YYYY-MM-DD_topic.md`. Never put proposals or analyses in the project root. At session end,
add one paragraph to the top of `SESSION_LOG.md`. When a session ships or decides something
the hub should absorb, write one hub agent update brief into `04_assets`
(`hub_agent_update_<date>_<topic>.md`; Claude has the hub-agent-update skill for it).

## Engineering rules

- Tests first, then code.
- Verify with `go vet ./...`, `go test ./... -count=1` and `.\build.ps1`. UI changes also
  with `.\scripts\uicheck.ps1`.
- Say what you measured and what you only inferred.
- A comment says what the code does and why. Who found it and when goes in the commit message.
  Apply this to a file when the file is touched.
- Never claim a commit, push or tag from a brief. Check it live with `git rev-parse` and
  `git ls-remote --tags origin`.
- No undocumented vendor endpoints, ever.

## Mechanics

- Shell on the laptop is PowerShell. Every snippet for Wilco carries a `cd` line and a
  "runs in:" label, prefixed with `#`.
- Cowork mount: no git writes (no add, commit, tag, push). Read-only git is fine, but not
  `git status` or `git diff`, they can leave an `index.lock`. Commits, tags and pushes are
  Wilco's step: hand him the exact commands.
- Cowork mount: Write and Edit truncate files over about 7 KB. Write large files through
  bash and verify the byte count.
- Never start agents via WMI, CIM or PSExec. Use Start-Process.
- No em dashes anywhere: chat, files, code comments.

## Stays human, always

Commits, tags, pushes and releases. Anything that spends money. Anything published or sent
as ZeroNonsense.dev. The paid team line and its price (open until the 2026-12-19 decision).
The ClientA wall: BurnMon is a ZND product only, with no ClientA handover and no ClientA
pilot. Never mix the ClientA address or ClientA client names into BurnMon files or exports.
Never copy client names, paths or prompts into exports or public material.

## Open questions

Ask them one at a time, with a recommendation, before presenting a finished file.

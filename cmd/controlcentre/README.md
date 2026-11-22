# Control Centre

`controlcentre.exe` is the BurnMon window for steering rather than watching. It shows the units
below one root folder, their files, the flow and sprint boards, a plan, every git repo and what
waits on you. It records gate decisions, Stop and comments.

Parent index: `README.md`. Built, and accepted by Wilco as it stands on 2026-11-15. Not yet run on a Mac.

## 1. What it is

A unit is every folder with a `siteoffice.json`, found up to 4 levels below the root. The Control
Centre is the one place that shows all of them together, and the one place that writes a decision
back. It needs no server, no port and no network call. It reads below the root and writes only its
settings file, the root's `_comments` folder and the controls audit file.

The data layer is `internal\centre`, the window is `cmd\controlcentre` with its page in
`cmd\controlcentre\page.html`.

## 2. Where it fits

| Part | Job |
|---|---|
| Foreman | the process: flows, gates, sprints |
| BurnMon, BurnMon Dev | watching: tokens, cost, the last 30 minutes |
| Control Centre | steering: units, boards, Approve, Send back, Stop, comments, git overview |
| Skills (Groundwork and others) | the tools an agent uses |

What it needs: a root folder with units. For gate decisions and Stop it also needs BurnMon Dev
running, because the gate and agent checks read its snapshot. What it does not need: Foreman for the
unit, file and Git pages, an account, a network, `gh` (unless you switch the pull request list on).

## 3. Install

From source on Windows (builds `controlcentre.exe` next to the other three exes):

```powershell
# runs in: PowerShell, cwd C:\ZND\50_projects\burnmon
Set-Location C:\ZND\50_projects\burnmon
.\build.ps1
```

On macOS it builds with cgo, like `burnmon-dev`, in `.github\workflows\macos.yml` and
`scripts\maccheck.sh`. The release workflow does not attach Windows files or a Control Centre
bundle yet; there is no macOS bundle.

```bash
# runs in: bash on a Mac, cwd = your clone of siteoffice/burnmon
./scripts/maccheck.sh
```

## 4. Use

Start `controlcentre.exe`. With no `--root` and no saved root it reads nothing and opens on Settings,
asking for the folder to show, on Windows and on a Mac alike (it never falls back to the current folder:
a double-clicked Mac app starts in `/`). Every other page then says that no folder is chosen yet and
offers to open Settings. The first ten minutes are: press Browse, pick the folder that holds your units,
press Save. Every page reloads at once, no restart.

- **Root**: `--root` names the folder to show; without it, the `root` in `controlcentre.json`;
  without that, no folder (the page asks for one). On the Settings page the Root field has a Browse button (the
  native folder picker on Windows and macOS) and Save. Typing or picking checks that the folder exists
  and is readable and shows how many units it holds before you save. Saving writes `root` and reloads
  every page. `--root` still wins: the field then shows "set by --root" and is read only.
- **Pages**: Overview, a unit with its files and flows, a file, a flow or sprint board inside the main
  pane, Waiting on you, Agents, Git, Planning and Settings.
- **Plan**: `siteoffice.plan.json` at the root holds the goals, lines and items of the 2 or 4 week
  Planning page. A file that does not validate is reported and treated as absent.
- **Comments**: on a unit, a file, a block of a file, a flow, a sprint, a date or a plan item, written
  as records under `_comments` in the root, with at most 5 attachments of at most 10 MB each.
- **Decisions**: Approve and Send back on a waiting flow, and Stop on a background agent, go through
  the same controls and the same audit file (`controls-audit.ndjson`, next to the store) as
  BurnMon Dev, with the same preview and Confirm step. Decisions need BurnMon Dev running, because the gate and agent checks read its snapshot.
- **Open and Back**: Double-click a box, an item or a file name to open its next page or the file;
  Enter does the same and Space selects. Back, at the left of the top bar, returns to the previous
  view with its selection; the back button of the mouse and Alt+Left do the same. A flow waits on you
  only when its gate is open (its awaiting field is true), a sprint when its status is waiting-on-you;
  a flow whose gate was decided but that did not move on shows "gate decided, flow not advanced".
- **Flows and Sprints tables** in a unit share one column grid: Project, Name, Stage or Phase, Waits
  (right aligned), State.
- **Right panel**: 320 px by default. Approve, Send back and Open board fit on one line. Drag its
  edge to resize; a double-click on the edge goes back to the preset.

### The Git page

An overview of every git repo below the root, in the side menu after Agents. It changes nothing.

- Repos are grouped by unit like the other pages. Inside a unit the open repos come first, by the
  earliest stage they stop at, then the published ones.
- One dense row per repo: name (with "+N" for other open branches and worktrees), branch, the lane
  (six dots, Changes to Released), where it stops, last commit (short sha and age; the subject on
  hover). Dirty count, ahead and behind, stashes, worktrees and pull requests are in the detail pane.
- The strip on top says how many repos are open and counts them per stage. A click on a stage shows
  only the repos that stop there; a second click clears it. While a reading runs, a bar moves above
  the rows (it holds still when the system asks for reduced motion).
- **The lane** is the line of the Fleet's Git tab plus one stage: Changes, Saved, Shared, Review,
  Merged, Released. A dot is done (filled), here now (ring), waiting (amber), blocked (red), not
  reached (empty) or not read (dashed). "Stops at" is the first stage that is not done; a Review that
  is off (pull requests off, or no GitHub remote) is skipped. Released is done without version tags,
  or when the newest version tag is the head of the default branch (`main`, else `master`); else it
  says "N since vX", counted on the first-parent line.
- **Published** means every stage is done for the checkout and for every other local branch and
  worktree that is not merged or has unsaved files. The default branch counts as open beside a
  checkout on another branch while it is not released.
- Limit: with the pull request switch off, a squash-merged branch reads as not merged until it is
  deleted, because no local ref shows the squash. With the switch on, the pull request state
  corrects it. A branch not checked out has no upstream beyond `origin/<name>`.
- A double-click opens the repo: its worktrees (path, branch, dirty, and which agent session works
  there, matched on the folder name in BurnMon Dev's snapshot), local branches with merged or
  not merged, tags, remotes and pull requests.
- Refs, HEAD, worktrees, stashes, tags and remotes come from the `.git` files (`internal\gitrefs`).
  Dirty state and ahead and behind come from one command, run read only with a 5 second limit per
  repo and per linked worktree: `git --no-optional-locks status --porcelain=v2 --branch`. It writes
  no `index.lock`. Nothing else runs and nothing writes inside `.git`. The lane's reads (is a tip in
  `origin/main`, commits a branch has that `main` lacks, the newest tag and the commits since it)
  also come from the `.git` files, with a 20000 commit and 3 second limit; a read that hits a limit
  shows "?", never a guess.
- The scanner skips a git folder without a HEAD, as git does. Two stray folders on the owner's laptop
  showed as rows until this was fixed: a shell had eaten the backslashes of a path and made a `.git`
  that held only `info\exclude`.
- Pull requests are off by default. The switch "Show GitHub pull requests (runs gh, needs network)"
  in Settings runs `gh pr list --state all --limit 100 --json` per repo with a GitHub remote, cached for 5 minutes, with a
  refresh button on the page. If `gh` is missing or not signed in, the page says so in one line.

## 5. Settings

`controlcentre.json`, found next to the exe, then in the same app data folder as `burnmon.json`
(`%LOCALAPPDATA%\burnmon` on Windows). Everything is set from the Settings page.

| Key | Values | Default |
|---|---|---|
| `root` | a folder | empty (no folder; Settings asks for one) |
| `width` | 60, 80 or 100 percent | 80 |
| `menu` | text or icons | text |
| `weeks` | 2 or 4 | 2 |
| `theme` | system, light or dark | system |
| `git_prs` | on or off | off |
| `panel` | narrow (256 px), normal (320), wide (400) | normal |
| `panel_px` | a dragged width, 220 to 640 px; wins while set | 0 (use the preset) |

The Settings page also holds the switches of four controls: Attach, Approve, Send back and Close
sprint. They are off by default and stored in `burnmon-dev-view.json` under `controls`, the same
file BurnMon Dev reads. Stop is not here: BurnMon Dev switches Stop. A control that cannot be used
shows its reason. Approve and Send back need `foreman_root`, `flows\_lib\flow_transition.py` and a
`python.exe` on `PATH`; Attach needs `agent_view` and a `claude.exe` BurnMon can start. See
`cmd\burnmon-dev\README.md`, "Controls".

The window has its own icon: the tree. A `controlcentre.icns` exists for a future macOS bundle.

## 6. Limits and known issues

- Not run on a Mac. `controlcentre` builds there in CI and in `scripts\maccheck.sh`, and nothing more.
- The one exception to "no port": a dev-only eval channel on `127.0.0.1:9335` that opens only when
  `CONTROLCENTRE_UICHECK` is set. Release builds and this README never set it. It exists for
  `.\scripts\uicheck.ps1 centre`, which runs the `gitpage` group (loading bar, strip against rows,
  stage filter, lane dots, detail pane, no sideways scroll at 1280, 1600 and 2560 px) in the real
  window on a fixture of five git repos in a temp folder. A size the screen cannot hold is skipped,
  not passed.
- Decisions need BurnMon Dev running, with its snapshot fresh.
- The snapshot holds only the last folder name of a session's working folder. So the Git page matches
  a worktree to a session by folder name alone. Two worktrees with the same folder name can show
  the same agent.
- No control was run against a real session, process or flow in the B4 report, and the audit lines
  of a live run are not quoted anywhere. That holds for the Control Centre as well.

## 7. Next steps and backlog

The Mac test day of 2026-11-16 covers the root picker and the Git page on a real Mac. Findings go into
`v0.4.0-beta.5`. Priority order lives in `02_roadmap\roadmap.md`.

## 8. Feedback, licence, contact

Open an issue at https://github.com/siteoffice/burnmon/issues. Never paste prompts, keys or customer
data. Licence: see `LICENSE`. Security problems: mail info@zerononsense.dev, not a public issue.

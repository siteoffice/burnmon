---
type: checklist
title: Mac day, Mon 16 Nov 2026 (rented Mac, BurnMon v0.4.0-beta.4, Groundwork v0.4.3)
created: 2026-11-16
from: Lane F of C:\ZND\10_holding\02_roadmap\2026-11-15_big_run_plan.md
status: ready; nothing here has run on a Mac yet
---

# Mac day checklist

## TL;DR

Twelve steps, each one copyable bash block that says where it runs. Steps 1 to 4 prepare the Mac
and run `scripts/maccheck.sh`. Steps 5 to 9 are the manual list, with a screenshot each. Step 10
is signing, which defaults to unsigned. Step 11 checks the Codex 0.167 reader. Step 12 brings the
results home. Findings become beta.5.

Tags used: BurnMon `v0.4.0-beta.4` (tag object `1a64982`, commit `3d227fc`), Groundwork `v0.4.3`
(commit `43eae89`). BurnMon `main` (`c175bfe`) differs from the tag only in `README.md` and
`SESSION_LOG.md`, so the build is the same code.

Every block runs in Terminal (zsh) on the rented Mac. Each block starts with `cd`, so you never
depend on the shell of the block before.

## 0. Decide before you leave (on the laptop)

- [ ] The PR with the new `scripts/maccheck.sh` is merged. The tag holds the OLD script (no Python
      check, shorter manual list). Step 2 copies the new script from `main` onto the tag checkout.
      If the PR is not merged, use its branch name in place of `origin/main` in step 2.
- [ ] Signing (OF26): do you have BOTH an Apple Developer ID Application certificate AND an
      app-specific password for `notarytool`, in your Apple Developer account? If either is
      missing, run unsigned (step 10). I could not check this from here. Default: unsigned.
- [ ] You can sign in to Claude Code on the Mac (Claude account). Sign out before you hand the Mac
      back (step 12).
- [ ] For step 11 you can sign in to the Codex CLI (OpenAI account). Without one, skip step 11 and
      write "skipped, no OpenAI sign-in" in the report. Sign out of it too in step 12.

## 1. Install Go, Python 3.9+ and the Xcode command line tools

`scripts/maccheck.sh` installs nothing and stops with a list of what is missing. Go must be 1.25
or newer (`go.mod` says `go 1.25.0`). The first build needs cgo, which needs the Xcode tools.

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: any.
xcode-select -p || xcode-select --install
go version
python3 --version
git --version
```

Missing Go: download the macOS package from https://go.dev/dl/ and run it. Missing Python 3.9 or
newer: the installer from https://www.python.org/downloads/ . With Homebrew, `brew install go
python` does both. Then open a new Terminal tab and run the block again. Done when `go version`
prints 1.25 or newer and `python3 --version` prints 3.9 or newer.

## 2. Clone BurnMon and Groundwork at the tags

Both go under `~/work`, so the Control Centre in step 8 can use `~/work` as its root and the Git
page shows exactly these two repos.

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: ~ (the block sets it).
mkdir -p ~/work && cd ~/work
git clone https://github.com/siteoffice/burnmon.git
git clone https://github.com/siteoffice/groundwork.git
cd ~/work/burnmon && git checkout v0.4.0-beta.4
cd ~/work/groundwork && git checkout v0.4.3
```

The tag holds the old `scripts/maccheck.sh`. Take the current one from `main`:

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: ~/work/burnmon (the block sets it).
cd ~/work/burnmon
git fetch origin main
git checkout origin/main -- scripts/maccheck.sh
git describe --tags --always
git log -1 --format=%h
```

Expect `v0.4.0-beta.4` and `3d227fc`. Write the architecture down for the report:

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: any.
uname -m
sw_vers -productVersion
```

## 3. Build all four apps

`burnmon`, `burnmon-dev`, `burnmon-cli` and `controlcentre`. The script in step 4 builds them
again into `maccheck-out`; this block shows a build failure early, in one place.

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: ~/work/burnmon (the block sets it).
cd ~/work/burnmon
mkdir -p maccheck-out
CGO_ENABLED=1 go build -o maccheck-out/burnmon ./cmd/burnmon
CGO_ENABLED=1 go build -o maccheck-out/burnmon-dev ./cmd/burnmon-dev
CGO_ENABLED=1 go build -o maccheck-out/burnmon-cli ./cmd/burnmon-cli
CGO_ENABLED=1 go build -o maccheck-out/controlcentre ./cmd/controlcentre
ls -l maccheck-out
```

Done when four files exist. Any build error: copy the text into the report and stop this step;
steps 4 to 9 need the apps.

## 4. Run the script

Full mode builds, vets, tests, starts `burnmon` and `burnmon-dev` with their eval channels,
runs the five DOM-only uicheck cases, takes two screenshots and writes `maccheck-out/report.md`.
Give Terminal the Screen Recording permission first (System Settings, Privacy and Security,
Screen Recording), or the screenshots come out black. It takes several minutes.

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: ~/work/burnmon (the block sets it).
cd ~/work/burnmon
bash scripts/maccheck.sh
echo "exit code: $?"
```

Done when the exit code is 0 and every row of the table says `ok`. A `FAILED` row names its log
in `maccheck-out`. Keep the whole folder.

OF25 proof, part 1 (go test with the toast package; the five darwin tests must RUN, not skip):

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: ~/work/burnmon (the block sets it).
cd ~/work/burnmon
go test ./internal/toast/ -count=1 -v 2>&1 | tee maccheck-out/toast-tests.log
grep -c "^--- PASS" maccheck-out/toast-tests.log
grep -c "^--- SKIP" maccheck-out/toast-tests.log
```

Expect five or more PASS lines and no SKIP line. OF25 part 2 (osascript honours `--` and the item
order). Expect ONE banner titled "BurnMon" with the text "-e foo", and nothing else running:

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: any.
/usr/bin/osascript -e 'on run argv' -e 'display notification (item 2 of argv) with title (item 1 of argv)' -e 'end run' -- 'BurnMon' '-e foo'
```

Note in the report which app the banner is attributed to (the plan expects Script Editor).

## 5. Give BurnMon something to show: one short Claude Code session

BurnMon reads session files from disk. A fresh rented Mac has none, so "opens with history" would
prove nothing. Step 8 and 9 need Claude Code anyway. Install it per
https://docs.claude.com/en/docs/claude-code/setup, then run one short session.

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: ~/work/groundwork (the block sets it).
cd ~/work/groundwork
claude
```

Ask one small question ("list the folders in this repo"), wait for the answer, then type `/exit`.
Done when `~/.claude/projects` holds a folder with a `.jsonl` file:

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: any.
ls ~/.claude/projects
```

## 6. BurnMon opens with history

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: ~/work/burnmon (the block sets it).
cd ~/work/burnmon
./maccheck-out/burnmon
```

Check and write down what you see (the plan does not say whether a window or the browser opens;
`cmd/burnmon/window_darwin.go` exists, so record which one you get):

- [ ] A window or a browser tab opens on its own.
- [ ] Now and Sessions list the session from step 5. History shows a row for today.
- [ ] Resize, Retina text sharp, dark mode switches the page.
- [ ] Screenshot: `screencapture -w ~/Desktop/mac-burnmon.png`, then click the window.

Stop it with Ctrl+C in the Terminal, or Cmd+Q in its window.

## 7. BurnMon Dev: window, percent settings (lane A), notification (OF25)

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: ~/work/burnmon (the block sets it).
cd ~/work/burnmon
./maccheck-out/burnmon-dev
```

- [ ] The window opens and the System zone shows CPU, memory, disks and network. Anything that
      says "not on macOS" is expected for the Windows pressure score and Wi-Fi details; write
      down anything else that is empty.
- [ ] Gear, "Sections each mode shows": set Status 30, Sessions 50, Processes 20. Save. The three
      sections draw about 30, 50 and 20 percent of the window height. Screenshot.
- [ ] Set Status 60, Sessions 50. With Overall scrollbar OFF, Save is disabled and one line says
      why. Switch Overall scrollbar ON: Save works and the page scrolls. Screenshot of each.
- [ ] Put the numbers back (or delete the settings file, see step 11).
- [ ] OF25 real banner: in the gear, tick "Show a notification when something waits on you".
      In a second Terminal start `claude` in `~/work/groundwork` and ask something that makes it
      stop and ask for permission. One banner must appear within 2 seconds of the item entering
      Blocked. Write down the title, the text and which app it is attributed to. A click does
      nothing on macOS (documented). I did not verify how to make a session Blocked, so say in the
      report how you did it.
- [ ] Screenshot: `screencapture -w ~/Desktop/mac-burnmon-dev.png`.

## 8. Control Centre: root picker and Git page (lane B)

First with no `--root`, so the Settings picker is the thing under test:

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: ~/work/burnmon (the block sets it).
cd ~/work/burnmon
./maccheck-out/controlcentre
```

- [ ] Settings, Root: the field shows the current root. Browse opens the native macOS folder
      picker. Choose `~/work`. Before Save, it shows how many units it holds. Save. Every page
      reloads without a restart. Screenshot of the picker and of the field after Save.
- [ ] Side menu, Agents, Git page: one dense row per repo below the root (expect burnmon and
      groundwork). Branch, dirty count, ahead/behind, last commit, stash and worktree count show.
      Both repos sit at a tag (detached HEAD), so "needs attention" for both is correct. Screenshot.
- [ ] Double-click a row: the repo detail opens (worktrees, branches, tags, remotes). Back, and
      the mouse back button, return to the list. No horizontal scrollbar. Screenshot.
- [ ] Settings, "Show GitHub pull requests": leave it OFF first. Then, only if `gh` is installed
      and signed in on the Mac, switch it on and press refresh; otherwise it must show one line
      saying `gh` is missing or not signed in. Write down which case you got.
- [ ] Light and dark, at 60, 80 and 100 percent width, on every page. Screenshot of the Git page
      in dark at 100 percent.

Then with `--root`, which must win and make the field read-only:

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: ~/work/burnmon (the block sets it).
cd ~/work/burnmon
./maccheck-out/controlcentre --root ~/work/groundwork
```

- [ ] The Root field says "set by --root" and cannot be edited.

## 9. Groundwork: plugin install in Claude Code and the first 15 minutes (GW26)

The source is `~/work/groundwork/README.md`, sections 5 and 6. Time it with a clock. The README
calls fifteen minutes "my target, not a measurement", so the real number is the finding.

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: ~/work/groundwork (the block sets it).
cd ~/work/groundwork
python3 --version
claude
```

Inside Claude Code type these three lines, one at a time (README section 5), choose user scope,
then restart Claude Code:

```text
# runs in: inside Claude Code, any session.
/plugin marketplace add siteoffice/groundwork
/plugin install groundwork@groundwork
/plugin install groundwork-mods@groundwork
```

- [ ] Both plugins install. Write down the Claude Code version (`claude --version`).
- [ ] `/setup-doctor` runs. Write down what it lists and whether it names Python.
- [ ] "Start a flow for <a small change in this repo>." `one-loop` stops at the plan gate (G1).
- [ ] Answer G1, G2, G4 and G3 as in `docs/tour.md`. A receipt lands in `flows/<project>/<slug>/receipts/`.
- [ ] Minutes from step 1 of the README to the receipt: ___
- [ ] Where you got stuck (this is question 1 of `FEEDBACK.md`): ___
- [ ] Screenshot of `/setup-doctor`, the plan gate and the receipt.

This test writes a `flows` folder into the clone. It is a throwaway clone; do not push it.

## 10. Signing and notarization (OF26): default unsigned

The apps you built in step 3 on this Mac carry no quarantine flag, so they run without any of
this. The flag appears on a file you DOWNLOAD (a browser, AirDrop from another Mac). Record which
case applies.

Unsigned run of a downloaded binary:

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: the folder holding the downloaded app.
xattr -d com.apple.quarantine ./burnmon
```

Write in the report: "ran unsigned, quarantine removed with xattr". Gatekeeper may still show a
warning; write down its exact text.

Only if you hold BOTH items from step 0, check that the certificate is on this Mac. A rented Mac
will not have it unless you export and import it, so this is likely a laptop-side job:

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: any.
security find-identity -v -p codesigning
```

A line with "Developer ID Application" means signing is possible. The notarization commands are
not scripted here and I have not run them. Treat OF26 as open unless you do both and record the
result.

## 11. Codex 0.167 reader check (plan Research item 6)

BurnMon's Codex reader was built against Codex 0.155.x rollouts. The plan says Codex is now at
0.167. This step checks that BurnMon still reads a fresh transcript from the version you install.
It needs an OpenAI sign-in.

Codex rollouts sit in `~/.codex/sessions/YYYY/MM/DD/rollout-<ts>-<uuid>.jsonl`, or under
`$CODEX_HOME/sessions` when that is set. Each answered turn writes one `token_count` line. Install
the Codex CLI from OpenAI's own install page, then:

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: ~/work/groundwork (the block sets it).
cd ~/work/groundwork
codex --version
codex
```

Write the exact version in the report (the plan says 0.167; any other number is a finding). Ask
Codex two short questions in one session ("list the folders in this repo", then "how many files
are in plugins"), then exit. Now compare the file with what BurnMon shows:

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: ~/work/burnmon (the block sets it).
cd ~/work/burnmon
F="$(ls -t ~/.codex/sessions/*/*/*/rollout-*.jsonl | head -1)"
echo "$F"
grep -c '"type":"token_count"' "$F"
grep -o '"type":"[a-z_]*"' "$F" | sort | uniq -c | sort -rn | head -12
./maccheck-out/burnmon
```

- [ ] The `token_count` count is 2 or more (one per answered turn). If it is 0, the format changed:
      keep the file out of the repo (it holds your prompts) and copy only the `uniq -c` list into
      the report.
- [ ] In BurnMon (Now, then Sessions) the Codex session is listed and its turn count equals the
      `token_count` count. Fewer turns than lines is the collapse seen before alpha.8: record both
      numbers.
- [ ] Tokens are not zero and the model name is filled in. Record the model name.
- [ ] Ask a third question while the Codex chat is still open: the new turn shows in BurnMon within
      a few seconds (Codex holds the file open, BurnMon polls the tail).
- [ ] Any BurnMon log line in `~/Library/Application Support/burnmon` that names Codex and an
      error: copy that line only.
- [ ] Screenshot of the Sessions page with the Codex row: `screencapture -w ~/Desktop/mac-codex.png`.

I have not run this on any machine. The commands use only the folder and line type named in
`internal\adapter\codex\codex.go`. The turn-count rule comes from the alpha.8 fix, not from a test
I ran today.

## 12. Bring the results home and leave the Mac clean

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: ~/work/burnmon (the block sets it).
cd ~/work/burnmon
mkdir -p ~/Desktop/mac-day
cp maccheck-out/report.md ~/Desktop/mac-day/
cp maccheck-out/*.log maccheck-out/*.png ~/Desktop/mac-day/
cp ~/Desktop/mac-*.png ~/Desktop/mac-day/
cd ~/Desktop && zip -r mac-day.zip mac-day
ls -l ~/Desktop/mac-day.zip
```

Fill in the manual boxes of `maccheck-out/report.md` (it ends with the list), or copy the boxes of
this file into the report. Leave out prompts, project names and client names from anything you
keep. Move `mac-day.zip` to the laptop by the route you prefer.

Before you hand the Mac back (it holds your transcripts and a Claude login):

```bash
# runs in: Terminal on the rented Mac (zsh). cwd: any.
claude /logout
ls ~/.claude ~/work "$HOME/Library/Application Support/burnmon"
```

Sign out of the Codex CLI with its own sign-out command as well. Then delete `~/.claude`,
`~/.codex`, `~/work` and `~/Library/Application Support/burnmon` yourself, once the
zip is on the laptop. I did not put the delete in a block on purpose: it ends the test data.

## Close-out

- DO: checklist, `scripts/maccheck.sh` update and its test, all measured on the laptop (see the
  PR). Nothing in this file has run on a Mac.
- HAND (you): step 0 decisions (signing certificate, Claude sign-in), and the whole Mac day.
- DO: the Codex 0.167 reader check is step 11 (plan Research item 6, added on Wilco's request).
- DROP: nothing.

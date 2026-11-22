---
type: plan
title: Mac retest fixes, plan and turn log (N1 to N4, D10)
created: 2026-11-22
lane: C:\ZND\10_holding\02_roadmap\2026-11-22_mac_retest_fix_lane.md
branch: work/wilco-de-tree/mac-retest-fixes
---

# Mac retest fixes: plan and turn log

## Start and baseline

- Branch `work/wilco-de-tree/mac-retest-fixes`, made by siteoffice-git `start` from `main` at
  `6a4023e` (v0.5.0-beta.1 is an ancestor).
- Baseline on `6a4023e`, measured in T1 on Windows: `go vet ./...` exit 0; `go test ./... -count=1`
  exit 0, 50 packages ok, 1837 tests passed, 11 skipped, 0 failed; `.\build.ps1` exit 0.
  `TestMigrateRealV01Store` passed on this laptop, on a store at schema version 9 (205 MB, with a
  5.2 MB `-wal`): it migrated nothing and so checked no v0.1 migration.

## T1, N3: cause and repair (done in T1)

Cause, measured on Windows. The store runs in WAL mode (`setPragmas` in
`C:\ZND\50_projects\burnmon\internal\store\store.go`). A store whose app still holds it open, before a
checkpoint, has only its first page in the main file and its tables in `burnmon.db-wal`. The test
copied the main file alone (`copyFile`), so the copy held no table. A new test makes exactly that
store (v0.1 schema, WAL, a connection left open, `wal_autocheckpoint=0`) and, with the old copy, failed
with the Mac's own message: `query raw events in ...: SQL logic error: no such table: events`. The
Mac's store was 4096 bytes (one page), made by v0.5.0-beta.1 minutes earlier, which fits this.
Second cause, read: that store was made by v0.5.0-beta.1, so even read whole it is at schema version
9, not a v0.1 store. On the Mac both held. Not an app defect: the store itself is sound, only the
test read it wrongly.

Repair, test code only (`C:\ZND\50_projects\burnmon\internal\store\store_test.go`):
- `snapshotStore` copies through SQLite with `VACUUM INTO` on a read-only connection
  (`file:...?mode=ro`), so rows still in the `-wal` file are in the copy and the real store is never
  written. `copyFile` is gone (no other caller).
- `notV01Reason`: a store with no `events` table, or with a `schema_version` table, is not a v0.1
  store; the test skips with that reason.
- `checkV01Migration` holds the old check, unchanged in what it asserts (head version 9, every event
  kept with the same ids, owner empty).
- New: `TestMigrateV01StoreHeldOpenInWAL` (through `checkV01Migration(..., mustRun true)`, so a
  failed snapshot or a store read as not v0.1 fails it instead of skipping) and `TestNotV01Reason`.
- Review (L5, one fresh reviewer): no behaviour change outside N3, no assertion removed. It found a
  guard after `t.Skipf` that could never run (Skipf ends the test); repaired with `mustRun`. Cost
  named by the review: on any machine with a v0.2 or later store the real-store test now skips, and
  the v0.1 migration is covered by the held-open test. The hard-coded `version != 9` is unchanged.
- Seen on Windows: with BurnMon Dev (pid 66960) holding `burnmon.db` open and a 5.2 MB `-wal` beside
  it, `TestMigrateRealV01Store` skipped with "it already has schema version 9" and both new tests
  passed. Not seen on a Mac: the Mac check in T6 and the pinned Mac run after the land.

Choice on an ambiguity: the lane says skip on "a schema version that is already current". T1 skips on
any `schema_version` table, because the test's own definition of v0.1 is "no schema_version table",
and on a v0.2 to v0.8 store its owner-empty check would be wrong. The migration from such versions is
not what this test is for.

## T2, N2: the lamp as a child on a Mac

Cause, read: `newLampChild` in `C:\ZND\50_projects\burnmon\cmd\burnmon-dev\lamp_child.go` line 53 sets
`supported: runtime.GOOS == "windows"`, and `Sync` and `Tick` return at once when not supported. The
file builds for `windows || (darwin && cgo)`, so on a Mac the child exists and does nothing.

Parent check, read: `lampParentGone` (`lamp_run.go`) is plain Go; on darwin the lamp passes it
`lampProcessAlive` (`lamp_window_darwin.go` line 245), `syscall.Kill(pid, 0)` with EPERM as alive.
`execLampStart` sets `BURNMON_LAMP_PARENT` through `lampChildEnv` on every system. So the check works
on darwin as written (read, not run on a Mac). One limit, inferred: a reused process id reads as
alive; the same holds on Windows. T2 names it, it does not repair it.

Files (at most 4): `lamp_child.go`, a test file beside it (`lamp_child_test.go` or the one that
exists), `cmd\burnmon-dev\README.md`. `page.html` only if a lamp text there names Windows only (T1
found none by search; README line 487 does: "Limits: Windows only in this version").
Tests first: `lampChildSupported(goos string) bool`, yes for `windows` and `darwin`, no for `linux`.

## T3, N4: the clear backdrop on a Mac

Cause, inferred: `lampStyle` in `lamp_window_darwin.go` makes the panel clear and sets the web view's
`underPageBackgroundColor` clear, but WKWebView still draws its own opaque background. Repair as the
lane says: guarded `drawsBackground` NO through key-value coding, one log line when the web view does
not answer to the key, the card backdrop unchanged.
Files (at most 3): `lamp_window_darwin.go`, one test that reads the darwin source and `lamp.html` as
text (the darwin file does not build on Windows), `lamp.html` only if the backdrop needs it.
Named, not repaired: with the clear backdrop the open panel keeps its whole 300 x 241 rectangle for
clicks (macOS has no window region, comment at line 197), so the invisible part takes clicks.

Done in T3 (read in code, not run on a Mac): `lampStyle`, clear branch, now sets `drawsBackground` to
NO through `setValue:forKey:` only when `[wk respondsToSelector:@selector(_setDrawsBackground:)]`
(the private selector behind the key; an unguarded `setValue:forKey:` on an unknown key throws), and
logs `lamp: the web view does not answer to drawsBackground ...; set lamp_backdrop to card`
otherwise. `underPageBackgroundColor` stays clear; the card branch is untouched. The comment says the
key is not documented, that wry takes the same route and what the log line means. New test file
`cmd\burnmon-dev\lamp_window_darwin_src_test.go` (no build tag, reads the darwin source and
`lamp.html` as text, so it runs on Windows): the guard, the key, the log line, the card branch
untouched, the clear default and `lamp_backdrop card` still selecting the card. Seen failing first.
Changed earlier test, with the reason: `TestLampDarwinWindowFileFinishLane` in `buildtags_test.go`
banned the strings `drawsBackground`, `_setDrawsBackground` and `setValue:` in the darwin file ("no
undocumented API"). The lane decides this one exception, so the ban becomes: exactly one key-value
write (`forKey:@`), exactly one `_setDrawsBackground`, and no `numberWithBool:YES`. Not weaker for any
other private call. `lamp.html` and `lamp_run.go` are unchanged, so no uicheck run (no page touched).
Choice on an ambiguity: the lane offers `respondsToSelector` or a guarded `setValue:forKey:`; T3 uses
the `respondsToSelector` guard because Objective-C has no try for a missing key in plain C code.
Built on the macos runner, not started; the look is not seen.

## T4, N1: the Control Centre with no root

Cause, read: `C:\ZND\50_projects\burnmon\cmd\controlcentre\main.go` line 44 and 45 pass the current
folder to `centre.ResolveRoot`, which falls back to it (`internal\centre\settings.go` line 130). A
double-clicked Mac app starts in `/` (inferred). The binding `ccState` (`bindings.go` line 74) calls
`centre.Build(root, ...)`, which walks below the root; from `/` that is the whole disk, and it touches
`~/Desktop`, which raises the macOS question. White window, inferred, to be read in T4: the page's
first call is `ccState`, and if webview_go runs bindings on the window thread, the walk blocks the
first paint.
Files (at most 8): `internal\centre\settings.go` and its test, the `cmd\controlcentre` binding and
page files that draw the no-folder state, their page tests, the Control Centre README.
Tests first: `ResolveRoot("", "", cwd)` is ""; a relative flag still joins cwd; a saved root wins.

Done in T4 (read in code and measured on Windows by tests; not seen on a Mac):
- Root rule: `ResolveRoot` in `internal\centre\settings.go` returns "" when there is no flag and no saved
  root, whatever the current folder is. A relative flag or saved root still joins the current folder; a
  saved root still wins over the current folder. `Build("")` in `internal\centreuild.go` returns an
  empty state at once (`filepath.Abs("")` is the current folder, so the guard sits before it).
- Bindings: with an empty root `ccFile`, `ccBoard` and `ccComment` refuse ("no folder is chosen yet; choose
  one in Settings"), `ccGit` returns an empty view with a line, `ccGitRepo` an empty detail. `ccRoot` and
  `ccRootSet` need no change: the Root row starts empty and Save points every page at the folder.
- Page (`page.html`): a state with an empty root opens on Settings once (`rootAsked`); Settings leads with
  "No folder is chosen yet: pick the folder to show with Browse, or type it, then Save."; every other page
  draws `pageNoRoot`: one muted line "No folder is chosen yet." and an "Open Settings" button, in the place
  the empty pages use. The crumb reads "No folder chosen".
- The white window, cause inferred, not measured on a Mac: the page drew its shell (menu, theme) with an
  empty pane and an empty lead until the first `ccState` returned, and `ccState` walked the whole disk from
  `/` (about 45 s, per the retest). So the first paint happened and was white. With no root the walk does
  not run. Added: `start()` sets the lead to "Reading the folder." right after the shell is drawn, so a
  slow first state on a big folder is no longer a blank window. Whether webview_go runs a binding on the
  window thread is not read; the Mac pinned run checks the window.
- Tests first, each seen failing: `TestResolveRoot` (changed, see below), `TestBuildEmptyRootScansNothing`
  (in `settings_test.go`), `TestNoRootBindingsReadNothing`, `TestNoRootSaveThenPagesWork`, `TestPageNoRoot`
  (new `cmd\controlcentre
oroot_test.go`). Changed earlier test, with the reason: `TestResolveRoot`
  expected the current folder for no flag and no saved root; the lane decides it is empty. Its other cases
  are unchanged; the relative-cwd case now uses a relative flag.
- Measured on Windows: `uicheck-centre.ps1` exit 0 (gitpage group). Its fixture has a root, so no uicheck
  case draws the no-root page; the page tests read the page text only. Not seen in a window.
- Choice on an ambiguity: the lane says "scans nothing". The folder picker still opens at the current root;
  with no root `rootBrowse` passes an empty start to the picker (the picker's own default folder).
- Note for the close-out: a Windows user who started the Control Centre in a folder with no saved root now
  chooses the folder once in Settings.

## T5, D10: the Sessions table at 960 px

Cause, read: `C:\ZND\50_projects\burnmon\internal\report\template.html` lines 158 to 173 give every
column but "What it was" a fixed width: 812 px without Owner and Client, 882 px with Client, 952 px
with both. `.wrap` (line 50) leaves a 920 px pane in a 960 px window, so with Client shown "What it
was" keeps about 30 px. Client shows because `hasClient` (line 1246) is true for any non-empty
client, and `pricing.ClientFor` gives every unmapped session the client "unassigned"
(`internal\pricing\pricing.go` line 345 to 398, `internal\merge\merge.go` line 102). Inferred: the
column is meant to show only when a real client is set. T5 reads and decides.
Files (at most 4): `template.html`, `internal\report\sessions_layout_test.go`, golden files by their
test command, one uicheck case under `tools\uicheck\`.

## Turn log

T1: done; commit 255f8f9 (pull request 67 opened); measured: vet 0, test 0 (50 packages ok, 1838 pass, 12 skip, 0 fail), build 0, linux and darwin build and vet 0 with CGO off; open: nothing for N3. Note: the branch also carries 6a4023e "Roadmap item 21" (Wilco, 02_roadmap\roadmap.md only), which was on local main and not on origin when start ran; raw git to drop it is denied, so it rides in pull request 67
T2: done; commit 7e203d8; measured: vet 0, test 0, build 0, linux and darwin build and vet 0 with CGO off; open: darwin code is built on the macos runner, not started (read gh pr checks in T6); parent check (BURNMON_LAMP_PARENT through lampProcessAlive) read on darwin, not run; a reused process id reads as alive, same as Windows, not repaired. Choice: README Limits keeps the quoted sentence "The lamp runs on Windows only in this version." (what Linux prints) because cmd\burnmon\readme_test.go pins "Windows only" and is outside the T2 file list. No earlier test changed.
T3: done; commit 4525606; measured: vet 0, test 0 (50 packages ok, no FAIL line), build 0, linux and darwin build and vet 0 with CGO off, reviewer ok; open: darwin code is built on the macos runner, not started (read gh pr checks in T6); the look is not seen; N4 click area named in the T3 section, not repaired; one earlier test changed with the reason: TestLampDarwinWindowFileFinishLane in buildtags_test.go (the lane allows the one drawsBackground key). No page touched, no uicheck.
T4: done; commit 1365bf5; measured: vet 0, test 0 (50 packages ok, no FAIL line), build 0, linux and darwin build and vet 0 with CGO off, uicheck-centre.ps1 0 (fixture has a root, no case draws the no-root page), reviewer found nothing outside N1; open: white-window cause inferred not measured (a Mac is needed), no-root page not seen in a window; one earlier test changed with the reason: TestResolveRoot (the lane makes the no-root result empty, not the current folder). Close-out note: a Windows user who started the Control Centre in a folder with no saved root now chooses the folder once in Settings.

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
  saved root still wins over the current folder. `Build("")` in `internal\centre\build.go` returns an
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
  (new `cmd\controlcentre\noroot_test.go`). Changed earlier test, with the reason: `TestResolveRoot`
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

Done in T5 (measured at 960 x 611 and 1280 x 860 on Windows (WebView2); not seen on a Mac):
- Why "unassigned" showed the Client column, read: `hasClient` counted any non-empty client, and the
  merge and pricing code give every unmapped session the client "unassigned". Repaired: the column and
  its filter show only for a client other than "unassigned". Choice on an ambiguity: Owner was left
  alone (its default is empty here), the lane names Client only.
- The 200 px was not reachable by the Client repair alone: the eight fixed columns of the usual table
  add up to 812 px and the pane at 960 px is 905 px (920 less the 15 px vertical scrollbar), so "What
  it was" kept 93 px. Choice (the lane lists three ways): narrower columns plus tighter cell padding
  (`7px 6px`, was `7px 9px`), one model pill with a count (was two), and Owner and Client hidden at a
  window width of 1100 px or less. Fixed columns now add up to 698 px (116, 88, 108, 84, 62, 74, 70,
  96), so "What it was" keeps 206 px at 960 and 440 px at 1280. With Owner and Client shown it keeps
  300 px at 1280. Reason for hiding Owner and Client below 1100 px: with both shown the table needs
  842 px, which leaves less than 200 px in any window under about 1100 px.
- No information is lost without a hover title: the Harness cell's title now ends with " / Owner: <owner>"
  and " / Client: <client>" when set (so a hidden column keeps its values), the Owner and Client cells
  carry their value as a title, the model cell's title still lists every model, and the Findings cell
  title is set by the page as before.
- Tests first, seen failing: in `internal\report\sessions_layout_test.go`
  `TestSessionsWhatItWasKeepsRoomAtANarrowWindow` (108 px free before) and
  `TestSessionsClientColumnShowsOnlyForARealClient`; `TestSessionsUICheckHookExists`. The existing CSS
  tests stay unchanged. New uicheck check `w9` (`tools\uicheck\check_w9.go`, run with
  `.\scripts\uicheck.ps1 w9`): four cases (960 and 1280, with the Codex-like fixture and with a real owner
  and client) measure "What it was" at least 200 px, no cut header, last column inside the pane, no
  horizontal scroll, and a title on every cut cell. It is not in the default list of
  `scripts\uicheck.ps1` (outside T5's file list). The check was seen green only: its red state against the
  old CSS was not captured, because the page needed the hook first (the page keeps `D` private); the
  first run against the old exe failed with "D is not defined", and a run against the new exe with the
  first widths failed at 188 px for "What it was" and for two untitled Owner and Client cells, then went
  green after the widths and titles changed.
- The page got one hook, `window.__bmSetSessions(list, insights)` (like `__bmTurnResolve`), because the
  check cannot reach `D`. A fixture of one session; `null` puts the real sessions back.
- Golden files: none changed. uicheck: `w0` to `w9` green, `w1` green.
- Review (L5, one fresh reviewer): no test weakened. Named, not repaired: (a) the Client filter now also
  disappears when every session is "unassigned", so "unassigned" can no longer be filtered on (same
  `hasClient` drives both); (b) the number cells (Calls, Tokens, Per call, Cost) have no hover title and
  clip with `text-overflow:clip`; the fixture holds only 123.4M tokens and 12.34 USD, so a value such as
  "$1,234.56" at 70 px is not measured; (c) margins are thin (206 and 208 px). One fix taken: the w9 case
  field `wantOwnerCol` is renamed `wantClientCol` (it tests Client).

## T6, verify against the success criteria

Measured in T6 on Windows, on HEAD `734992d` (the T5 log commit), before any T6 edit: `go vet ./...`
exit 0; `go test ./... -count=1` exit 0, 50 packages ok, 1849 tests passed, 12 skipped, 0 failed
(baseline on `6a4023e`: 1837 passed, 11 skipped); `.\build.ps1` exit 0; with CGO_ENABLED=0 the linux
and darwin build and vet exit 0; `tools\uicheck` vet and build exit 0; `.\scripts\uicheck.ps1 w9` exit
0. `gh pr checks 67` on `734992d`: `macos`, `macos (amd64)`, `macos (arm64)` and `windows` pass.

1. N3: met. `TestMigrateRealV01Store` copies through `VACUUM INTO` on a read-only connection and skips
   with a reason (`notV01Reason`) on a store that is not v0.1; on this laptop it skips ("it already has
   schema version 9"). `TestMigrateV01StoreHeldOpenInWAL` builds the Mac's case and passes. Cause in the
   T1 section. Not run on a Mac.
2. N2: partly met. Met: `lampChildSupported` says yes for `windows` and `darwin`, no for `linux`, and
   `newLampChild` uses it (`lamp_child.go` line 56 and 61); the darwin parent check is named in the T2
   section (read, not run). Not met: `C:\ZND\50_projects\burnmon\cmd\burnmon-dev\lamp_unsupported.go`
   line 13 still prints "The lamp runs on Windows only in this version." on Linux and on a Mac build
   without cgo, and the README Limits paragraph quotes that line. It is no longer the message a Mac user
   with the released build sees, but it is still a text that says the lamp is Windows only. Kept by T2
   because `cmd\burnmon\readme_test.go` line 236 pins "Windows only"; outside every turn's file list, so
   T6 does not change it (L1). HAND in the Close-out.
3. N4: met in code, not seen. `lampStyle` sets `drawsBackground` to NO through `setValue:forKey:` only
   when `respondsToSelector:@selector(_setDrawsBackground:)` holds, and logs one line otherwise
   (`lamp_window_darwin.go` line 72 to 75); the card branch is unchanged; the click area is named in the
   T3 section. Built on the macos runner (`macos` pass), not started.
4. N1: met in code and page tests, not seen in a window. `ResolveRoot("", "", cwd)` returns "" (settings.go
   line 137); `Build("")` scans nothing (`TestBuildEmptyRootScansNothing`); Settings leads with the folder
   question and other pages draw "No folder is chosen yet." (`page.html` line 744 and 1432); tests for both
   states in `cmd\controlcentre\noroot_test.go`. The white-window cause is named in the T4 section, inferred,
   not measured on a Mac.
5. D10: met, measured at 960 x 611 and 1280 x 860 on Windows (WebView2), again in T6: "What it was" 206 px
   at 960 (both fixtures), 440 px at 1280 without Owner and Client and 300 px with them; cut headers none;
   last column right 925 = pane right 925 at 960 and 1210 = 1210 at 1280; box scroll width equals client
   width (no horizontal scroll); cut cells without a title none. Not seen on a Mac.
6. Met: vet, test and build exit 0, linux and darwin build and vet without cgo exit 0, `macos` check pass
   (read with `gh pr checks 67`). This T6 commit gets its own checks, read after propose (Turn log).
7. Met with three changes named: `TestLampDarwinWindowFileFinishLane` (T3), `TestResolveRoot` (T4), and the
   N3 restructure of `store_test.go` (T1, `copyFile` gone, assertions kept). Each is in the turn log with
   its reason. No other earlier test changed (`git diff main...HEAD` on `*_test.go`: 7 files, 4 of them new
   or new tests only).
8. Met: pull request 67 open on `work/wilco-de-tree/mac-retest-fixes`; nothing landed, nothing tagged.

Repaired in T6, in this plan only: two paths in the T4 section had lost a backslash in an earlier turn's
write (`internal\centre\build.go` held a backspace byte, `cmd\controlcentre\noroot_test.go` held a line
break). The other 19 files of the lane were scanned: no control byte, tab or carriage return.

## Turn log

T1: done; commit 255f8f9 (pull request 67 opened); measured: vet 0, test 0 (50 packages ok, 1838 pass, 12 skip, 0 fail), build 0, linux and darwin build and vet 0 with CGO off; open: nothing for N3. Note: the branch also carries 6a4023e "Roadmap item 21" (Wilco, 02_roadmap\roadmap.md only), which was on local main and not on origin when start ran; raw git to drop it is denied, so it rides in pull request 67
T2: done; commit 7e203d8; measured: vet 0, test 0, build 0, linux and darwin build and vet 0 with CGO off; open: darwin code is built on the macos runner, not started (read gh pr checks in T6); parent check (BURNMON_LAMP_PARENT through lampProcessAlive) read on darwin, not run; a reused process id reads as alive, same as Windows, not repaired. Choice: README Limits keeps the quoted sentence "The lamp runs on Windows only in this version." (what Linux prints) because cmd\burnmon\readme_test.go pins "Windows only" and is outside the T2 file list. No earlier test changed.
T3: done; commit 4525606; measured: vet 0, test 0 (50 packages ok, no FAIL line), build 0, linux and darwin build and vet 0 with CGO off, reviewer ok; open: darwin code is built on the macos runner, not started (read gh pr checks in T6); the look is not seen; N4 click area named in the T3 section, not repaired; one earlier test changed with the reason: TestLampDarwinWindowFileFinishLane in buildtags_test.go (the lane allows the one drawsBackground key). No page touched, no uicheck.
T4: done; commit 1365bf5; measured: vet 0, test 0 (50 packages ok, no FAIL line), build 0, linux and darwin build and vet 0 with CGO off, uicheck-centre.ps1 0 (fixture has a root, no case draws the no-root page), reviewer found nothing outside N1; open: white-window cause inferred not measured (a Mac is needed), no-root page not seen in a window; one earlier test changed with the reason: TestResolveRoot (the lane makes the no-root result empty, not the current folder). Close-out note: a Windows user who started the Control Centre in a folder with no saved root now chooses the folder once in Settings.
T5: done; commit 830450a; measured: vet 0, test 0 (50 packages ok, no FAIL line), build 0, linux and darwin build and vet 0 with CGO off, uicheck w0 to w9 and w1 exit 0 (w9 at 960 x 611 and 1280 x 860 on Windows (WebView2): What it was 206 px at 960, 440 px at 1280, no cut header, no horizontal scroll), reviewer ok; open: not seen on a Mac; number cells have no hover title and large values are not measured; the Client filter is gone when every client is unassigned; w9 is not in the default list of scripts\uicheck.ps1 (outside the T5 file list); no earlier test changed; two scratch files from this turn remain in the temp folder /tmp (edit.ps1, w9patch.go), the delete policy denied removing them.
T6: done; commit 349265a; measured: vet 0, test 0 (50 packages ok, 1849 pass, 12 skip, 0 fail), build 0, linux and darwin build and vet 0 with CGO off, uicheck w9 0, gh pr checks 67 on 349265a: macos, macos (amd64), macos (arm64), windows pass (read 2 of 10, 16:33); grade: criteria 1 and 3 to 8 met, criterion 2 partly (cmd\burnmon-dev\lamp_unsupported.go still prints "Windows only" on Linux and no-cgo Mac, HAND); repaired two damaged paths in this plan; no stash; open: land and tag (Wilco), the HAND lines in the hub brief Close-out; a docs check failure on a full C:\ path in the STATUS top block was repaired before the save (doccheck allows repo-relative paths there).

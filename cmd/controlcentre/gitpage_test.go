//go:build windows || darwin

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"burnmon/internal/centre"
	"burnmon/internal/snapshot"
)

func writeTo(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// rootEnv is a bindEnv with a root box at a, a settings file in its own
// folder, and units in a (one) and b (two).
func rootEnv(t *testing.T, byFlag bool) (bindEnv, string, string) {
	t.Helper()
	a, b := t.TempDir(), t.TempDir()
	writeTo(t, a, "siteoffice.json", `{"name":"a"}`)
	writeTo(t, b, "siteoffice.json", `{"name":"b"}`)
	writeTo(t, b, "x/siteoffice.json", `{"name":"x"}`)
	e := bindEnv{roots: newRootBox(a, byFlag), settingsPath: filepath.Join(t.TempDir(), centre.SettingsFile),
		settings: newSettingsBox(centre.DefaultSettings()), now: func() time.Time { return rigT0 }}
	return e, a, b
}

// TestRootSetSavesAndReloads: Save checks the folder, writes root to
// controlcentre.json and every binding reads the new root at once.
func TestRootSetSavesAndReloads(t *testing.T) {
	e, a, b := rootEnv(t, false)
	if v := rootView(e); v.Root != a || v.ByFlag || !v.Check.OK || v.Check.Units != 1 {
		t.Fatalf("before: %+v", v)
	}
	v, err := rootSet(e, b)
	if err != nil {
		t.Fatal(err)
	}
	if v.Root != b || v.Check.Units != 2 {
		t.Errorf("after: %+v", v)
	}
	if got := e.rootNow(); got != b {
		t.Errorf("rootNow = %q, want %q", got, b)
	}
	if st := centre.Build(e.rootNow(), centre.Options{Now: rigT0}); st.Root != b || st.Counts.Units != 2 {
		t.Errorf("state after Save: root %q, %d units", st.Root, st.Counts.Units)
	}
	loaded, _ := centre.LoadSettings("", filepath.Dir(e.settingsPath))
	if loaded.Root != b {
		t.Errorf("file root = %q, want %q", loaded.Root, b)
	}
	if e.settings.get().Root != b {
		t.Errorf("settings box root = %q", e.settings.get().Root)
	}
}

func TestRootSetRefusesBadFolder(t *testing.T) {
	e, a, _ := rootEnv(t, false)
	_, err := rootSet(e, filepath.Join(a, "missing"))
	if err == nil || !strings.HasPrefix(err.Error(), "No folder at ") {
		t.Errorf("err = %v", err)
	}
	if e.rootNow() != a {
		t.Error("a refused Save changed the root")
	}
	if _, serr := os.Stat(e.settingsPath); serr == nil {
		t.Error("a refused Save wrote the settings file")
	}
}

// TestRootByFlagReadOnly: with --root the view says so and Save is refused.
func TestRootByFlagReadOnly(t *testing.T) {
	e, a, b := rootEnv(t, true)
	if v := rootView(e); !v.ByFlag || v.Root != a {
		t.Errorf("view = %+v", v)
	}
	if _, err := rootSet(e, b); !errors.Is(err, ErrRootByFlag) {
		t.Errorf("err = %v, want ErrRootByFlag", err)
	}
	if e.rootNow() != a {
		t.Error("--root was overridden")
	}
}

func TestRootCheckCountsUnits(t *testing.T) {
	e, _, b := rootEnv(t, false)
	if c := rootCheck(e, b); !c.OK || c.Units != 2 || c.Line != "2 units below this folder." {
		t.Errorf("rootCheck = %+v", c)
	}
}

func TestRootBrowse(t *testing.T) {
	e, a, b := rootEnv(t, false)
	if r := rootBrowse(e, ""); r.Picked || r.Line == "" {
		t.Errorf("no picker: %+v", r)
	}
	var gotStart string
	e.pickFolder = func(start string) (string, bool, error) { gotStart = start; return b, true, nil }
	if r := rootBrowse(e, "  "); !r.Picked || r.Path != b || gotStart != a {
		t.Errorf("picked: %+v, start %q (want the current root)", r, gotStart)
	}
	e.pickFolder = func(string) (string, bool, error) { return "", false, nil }
	if r := rootBrowse(e, b); r.Picked || r.Line != "" {
		t.Errorf("cancelled: %+v", r)
	}
	e.pickFolder = func(string) (string, bool, error) { return "", false, errors.New("boom") }
	if r := rootBrowse(e, b); !strings.Contains(r.Line, "boom") {
		t.Errorf("error: %+v", r)
	}
	if e.rootNow() != a {
		t.Error("Browse alone must not change the root")
	}
}

func TestSettingsSetKeepsRootSavesGitPRs(t *testing.T) {
	e, _, _ := rootEnv(t, false)
	cur := e.settings.get()
	cur.Root = "kept"
	e.settings.set(cur)
	saved, err := settingsSet(e, centre.Settings{Width: 80, Menu: "text", Weeks: 2, Theme: "system", GitPRs: true})
	if err != nil {
		t.Fatal(err)
	}
	if !saved.GitPRs || saved.Root != "kept" {
		t.Errorf("saved = %+v", saved)
	}
}

// gitRigRoot is a root with one repo whose origin is on GitHub.
func gitRigRoot(t *testing.T) string {
	root := t.TempDir()
	writeTo(t, root, "siteoffice.json", `{"name":"r"}`)
	writeTo(t, root, "repo/.git/HEAD", "ref: refs/heads/main\n")
	writeTo(t, root, "repo/.git/config", "[remote \"origin\"]\n\turl = https://github.com/example/repo.git\n")
	return root
}

// TestGitViewFollowsSwitchAndRoot: gh runs only with the PR switch on; the
// view reads the root the box holds now.
func TestGitViewFollowsSwitchAndRoot(t *testing.T) {
	root := gitRigRoot(t)
	var mu sync.Mutex
	ghRuns := 0
	git := centre.NewGitScanner(centre.GitDeps{
		Status: func(string) (string, error) { return "# branch.head main\n", nil },
		PRs: func(slug string) ([]centre.GitPR, error) {
			mu.Lock()
			ghRuns++
			mu.Unlock()
			return []centre.GitPR{{Number: 1, State: "OPEN"}}, nil
		},
		Now: func() time.Time { return rigT0 },
	})
	e := bindEnv{roots: newRootBox(root, false), settings: newSettingsBox(centre.DefaultSettings()), git: git}
	read := func() centre.GitView {
		gitView(e, GitRequest{Rows: 20})
		git.Wait()
		return gitView(e, GitRequest{Rows: 20})
	}
	v := read()
	if v.Root != root || len(v.Repos) != 1 || v.Repos[0].PRs != -1 || ghRuns != 0 {
		t.Fatalf("switch off: root %q, repos %+v, gh runs %d", v.Root, v.Repos, ghRuns)
	}
	s := e.settings.get()
	s.GitPRs = true
	e.settings.set(s)
	v = read()
	if v.Repos[0].PRs != 1 || ghRuns != 1 {
		t.Errorf("switch on: PRs %d, gh runs %d", v.Repos[0].PRs, ghRuns)
	}
	other := t.TempDir()
	e.roots.set(other)
	if v := read(); v.Root != other || len(v.Repos) != 0 {
		t.Errorf("after a root change: %+v", v)
	}
	if d := gitRepo(e, "repo"); d.Found {
		t.Error("a repo of the old root was found after the root changed")
	}
}

func TestGitViewWithoutScanner(t *testing.T) {
	v := gitView(bindEnv{root: "x", settings: newSettingsBox(centre.DefaultSettings())}, GitRequest{})
	if v.Line == "" || v.Repos == nil || v.Pages == nil {
		t.Errorf("view = %+v", v)
	}
}

// TestSessionsByFolder: the snapshot's project (last folder name of the
// cwd) keys the sessions; the full cwd is never there.
func TestSessionsByFolder(t *testing.T) {
	rd := snapshot.Reading{Status: snapshot.StatusOK, Snapshot: &snapshot.Snapshot{
		AgentSessions: []snapshot.AgentSession{
			{Kind: "background", ID: "ab12", Project: "burnmon-wt-git"},
			{Kind: "interactive", SessionID: "0123456789abcdef", Project: "burnmon-wt-git"},
			{Kind: "background", ID: "noproj"},
		},
		Sessions: []snapshot.Session{
			{Vendor: "codex", ID: "fedcba9876543210", Project: "website"},
			{Vendor: "claude", ID: "0123456789abcdef", Project: "burnmon-wt-git"},
		},
	}}
	got := sessionsByFolder(rd)
	want := map[string][]string{
		"burnmon-wt-git": {"background agent ab12", "interactive agent 01234567", "claude 01234567"},
		"website":        {"codex fedcba98"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sessionsByFolder = %v, want %v", got, want)
	}
	if got := sessionsByFolder(snapshot.Reading{Status: snapshot.StatusMissing}); len(got) != 0 {
		t.Errorf("no snapshot: %v", got)
	}
}

func TestAppleChooseFolder(t *testing.T) {
	if got := appleChooseFolder(""); got != `POSIX path of (choose folder with prompt "Pick the Control Centre root")` {
		t.Errorf("no start: %s", got)
	}
	dir := t.TempDir()
	got := appleChooseFolder(dir)
	q := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(dir)
	if !strings.Contains(got, ` default location (POSIX file "`+q+`")`) {
		t.Errorf("with start: %s", got)
	}
	for in, want := range map[string]string{"/Users/w/ZND/\n": "/Users/w/ZND", "/\n": "/"} {
		if got := osascriptPath(in); got != want {
			t.Errorf("osascriptPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// --- page ---

func TestPageGitInMenuUnderAgents(t *testing.T) {
	if !strings.Contains(pageHTML, "['agents', 'Agents', 'agents'], ['git', 'Git', 'git'], ['planning'") {
		t.Error("the Git page is not in the side menu right under Agents")
	}
	mustContain(t, "render", "git: pageGit", "gitrepo: pageGitRepo")
	mustContain(t, "crumbs", "view.page === 'gitrepo'", "go('git')")
}

// TestPageGitRows: one dense row per repo (Repo, Branch, the lane, Stops at,
// Last), a unit line before each unit's rows, double-click opens; the dirty
// and ahead/behind cells moved into the detail pane.
func TestPageGitRows(t *testing.T) {
	mustContain(t, "drawGit", "['Repo', 'g1']", "['Branch', 'g2']", "['Changes to Released', 'glane']", "['Stops at', 'gst']",
		"['Last', 'gl']", "'rw uh'", "'Outside any unit'", "laneEl(r.lane", "r.stops_at", "r.published", "wireOpen(b, pick)",
		"gv.pages", "pager(", "r.last_subject", "picked.click()")
	body := pageFunc(t, "drawGit")
	for _, gone := range []string{"['Dirty'", "['+/-'", "['Stash'", "['WT'", "['PRs'"} {
		if strings.Contains(body, gone) {
			t.Errorf("drawGit still has the column %s", gone)
		}
	}
	mustContain(t, "gitRowDetail", "go('gitrepo', r.id)", "openSel", "open:", "lane: r.lane", "next: r.next", "others: r.open_branches", "gitAB(r)", "r.dirty")
	mustContain(t, "gitAB", "'none'", "'?'")
}

// TestPageGitStrip: the strip on top shows how many repos are open, one count
// per stage; a click on a stage filters the rows, a second click clears it.
func TestPageGitStrip(t *testing.T) {
	mustContain(t, "drawGit", "gv.summary", "gitStrip(")
	mustContain(t, "gitStrip", "sm.open", "sm.total", "sm.published", "sm.stages", "LANE_STAGES", "gitStage === ", "aria-pressed", "sub.page = 0", "render()")
	mustContain(t, "pageGit", "stage: gitStage")
	if !strings.Contains(pageHTML, "var LANE_STAGES = ['Changes', 'Saved', 'Shared', 'Review', 'Merged', 'Released'];") {
		t.Error("page.html has no LANE_STAGES with the six stage names in order")
	}
}

// TestPageGitLaneDots: a state is not carried by colour alone: done is filled,
// here-now a ring with a halo, todo empty, off dashed.
func TestPageGitLaneDots(t *testing.T) {
	for state, want := range map[string]string{".dot.done": "background:var(--accent)", ".dot.active": "box-shadow", ".dot.off": "border-style:dashed",
		".dot.warn": "background:var(--amber)", ".dot.bad": "background:var(--red)"} {
		if !strings.Contains(compact(ruleFor(t, state)), want) {
			t.Errorf("%s has no %s", state, want)
		}
	}
	mustContain(t, "laneEl", "'dot ' +", "LANE_STAGES", "title")
}

// TestPageGitAsksAgainWhileLoading: the page never waits on git; it asks
// again while the Go side reads, and stops when the page changes.
func TestPageGitAsksAgainWhileLoading(t *testing.T) {
	mustContain(t, "pageGit", "api.git({rows: gitRows(), refresh: !!again, stage: gitStage})", "gv.loading", "setTimeout", "seq === drawSeq", "GIT_POLL_MS")
	mustContain(t, "pageGitRepo", "api.gitRepo(", "v.loading", "setTimeout", "seq === drawSeq")
	mustContain(t, "drawGit", "'Refresh'", "pageGit(true)", "drawSeq++", "gv.line", "gv.pr_line")
}

// TestPageGitRepoSections: worktrees with sessions, branches, PRs, tags and
// remotes; each section keeps its size and puts the rest behind "+N more".
func TestPageGitRepoSections(t *testing.T) {
	mustContain(t, "drawRepo", "laneEl(w.lane", "laneEl(br.lane", "'Worktrees, sessions matched by folder name'", "'Local branches'", "'Pull requests'",
		"'Tags, newest first'", "'Remotes'", "w.sessions", "br.merged", "d.pr_line", "'Switch on GitHub pull requests in Settings to see them.'")
	mustContain(t, "gitSection", "'+' + (list.length - fit) + ' more'", "more.title", "data-title")
}

// TestPageGitChangesNothing: the Git page functions call only the two read
// bindings: no decision, run, comment or setting.
func TestPageGitChangesNothing(t *testing.T) {
	for _, fn := range []string{"pageGit", "drawGit", "gitRowDetail", "pageGitRepo", "gitSection", "drawRepo"} {
		body := pageFunc(t, fn)
		for _, bad := range []string{"api.run", "api.preview", "decide(", "api.comment", "saveSetting", "api.rootSet"} {
			if strings.Contains(body, bad) {
				t.Errorf("%s contains %s; the Git page changes nothing", fn, bad)
			}
		}
	}
}

// TestPageRootRow: Root field, Browse and Save on the Settings page; the
// check runs before Save, Save reloads every page, --root is read-only.
func TestPageRootRow(t *testing.T) {
	mustContain(t, "pageSettings", "rootRow();")
	mustContain(t, "rootRow", "'Root'", "h('input')", "'Browse'", "'Save'", "api.root()", "api.rootCheck(inp.value)",
		"api.rootSet(inp.value)", "api.rootBrowse(inp.value)", "save.disabled = !c.ok || c.path === cur",
		"rv.by_flag", "inp.readOnly = true", "'Set by --root.", "loadState()", "hist = []")
}

func TestPageGitHoverCells(t *testing.T) {
	for _, sel := range []string{".g1", ".g2", ".gl"} {
		if !strings.Contains(compact(ruleFor(t, sel)), "text-overflow:ellipsis") {
			t.Errorf("%s does not cut with an ellipsis", sel)
		}
	}
	mustContain(t, "gitCell", "c.title = title", "data-title")
}

// TestPageGitStopsAtColumnIsWidest: "Stops at" is the cell a person reads to
// know what is open ("Changes: 1 file", "Shared: 1 new online"), so it takes
// the most of the room that is left; the repo and branch names, which are
// cut and shown in full on hover, take less.
func TestPageGitStopsAtColumnIsWidest(t *testing.T) {
	// The grow factors decide who takes the room that is left; the basis and the shrink factor
	// were added for D15 (TestGitRowKeepsRoomForItsTexts). compact drops the spaces.
	if !strings.Contains(compact(ruleFor(t, ".gst")), "flex:2.2") {
		t.Errorf(".gst does not grow by 2.2: %s", compact(ruleFor(t, ".gst")))
	}
	for _, sel := range []string{".g1", ".g2"} {
		if !strings.Contains(compact(ruleFor(t, sel)), "flex:.8") {
			t.Errorf("%s does not grow by .8: %s", sel, compact(ruleFor(t, sel)))
		}
	}
}

// TestPageGitRowFitsTheNarrowPane (Wilco, 2026-11-23): the "Changes to Released" header wrapped
// onto two lines in its 112 px cell and the "Stops at" text was cut ("Changes: 1 ..." at about
// 79 px at 1280 wide). The lane is now a fixed 124 px, which holds the header on one line, and
// the room comes from Repo and Branch, whose short names left most of their cell empty (their
// names are still cut with a hover). The five bases add up to 468 px, which is what a row has at
// 1280 wide: 80 percent of 1280, less the 176 px menu, the 320 px panel, the 28 px pane padding
// and four 8 px gaps. So at 1280 nothing has to shrink and "Stops at" keeps its 120 px; wider,
// it grows by 2.2 of the 3.8 shared, the most of the free room. Equal halves of the lane and
// "Stops at" were tried first and left "Stops at" narrower than before, so they were dropped.
func TestPageGitRowFitsTheNarrowPane(t *testing.T) {
	flex := func(sel string) (grow, shrink, basis float64) {
		m := regexp.MustCompile(`flex:([\d.]+)\s+([\d.]+)\s+([\d.]+)px`).FindStringSubmatch(ruleFor(t, sel))
		if m == nil {
			t.Fatalf("%s has no flex: grow shrink basis: %s", sel, ruleFor(t, sel))
		}
		fmt.Sscan(m[1], &grow)
		fmt.Sscan(m[2], &shrink)
		fmt.Sscan(m[3], &basis)
		return
	}
	_, _, b1 := flex(".g1")
	_, _, b2 := flex(".g2")
	lg, ls, lb := flex(".glane")
	sg, _, sb := flex(".gst")
	_, _, bl := flex(".gl")
	if lg != 0 || ls != 0 || lb < 124 {
		t.Errorf("the lane is grow %v shrink %v basis %v, want a fixed cell of at least 124 px for its header", lg, ls, lb)
	}
	if sum := b1 + b2 + lb + sb + bl; sum > 468 {
		t.Errorf("the five bases add up to %v px, more than the 468 px of a row at 1280 wide, so Stops at would shrink", sum)
	}
	if sg < 2.2 || sb < 120 {
		t.Errorf("Stops at is grow %v basis %v, want at least 2.2 and 120 px: it takes the most of the room", sg, sb)
	}
	// The small lane of the worktree rows keeps its fixed 96 px.
	if !strings.Contains(compact(ruleFor(t, ".glane.sm")), compact("flex:0 0 96px")) {
		t.Errorf(".glane.sm = %s, want flex:0 0 96px", compact(ruleFor(t, ".glane.sm")))
	}
}

// TestPageGitNoHorizontalScroll: the pane clips, every flexible cell and the
// strip can shrink (min-width:0), and the fixed cells of a row (lane 150, last
// 104, four 8 px gaps) leave room at 1600 px wide: 80 percent of 1600, less the
// 176 px menu, the 320 px panel and the 28 px pane padding, is 756 px.
func TestPageGitNoHorizontalScroll(t *testing.T) {
	if !strings.Contains(compact(ruleFor(t, ".pane")), "overflow:hidden") {
		t.Error(".pane does not clip, so a wide row would scroll")
	}
	for _, sel := range []string{".g1", ".g2", ".gst", ".glane", ".lstrip", ".lbig", ".lstrip button.lst", ".lane"} {
		if !strings.Contains(compact(ruleFor(t, sel)), "min-width:0") {
			t.Errorf("%s cannot shrink (no min-width:0)", sel)
		}
	}
	for _, width := range []float64{1600, 2560} {
		room := width*0.8 - 176 - 320 - 28
		if fixed := 150.0 + 104 + 4*8; room-fixed < 300 {
			t.Errorf("at %.0f px a row has %.0f px for the flexible cells, want at least 300", width, room-fixed)
		}
	}
}

// TestPageGitLoadingBar: while a reading runs (first read, a re-read, the repo
// page) the pane shows an indeterminate bar; it moves only without
// prefers-reduced-motion and is gone once the rows are drawn.
func TestPageGitLoadingBar(t *testing.T) {
	mustContain(t, "loadBar", "'loadbar'", "'progressbar'", "aria-label")
	mustContain(t, "pageGit", "loadBar()")
	mustContain(t, "drawGit", "gv.loading", "loadBar()")
	mustContain(t, "pageGitRepo", "loadBar()")
	mustContain(t, "drawRepo", "pane.textContent = ''")
	if !strings.Contains(compact(ruleFor(t, ".loadbar i")), "animation:") {
		t.Error(".loadbar i does not animate")
	}
	if !strings.Contains(pageHTML, "prefers-reduced-motion") {
		t.Error("the loading bar does not respect prefers-reduced-motion")
	}
}

// TestPageGitRowsCarryTheirRepo: each row names its repo (data-repo), so
// tools\uicheck can click and read a row by name.
func TestPageGitRowsCarryTheirRepo(t *testing.T) {
	mustContain(t, "drawGit", "setAttribute('data-repo', r.id)")
}

// TestUICheckServerIsDevOnly: the Control Centre opens no port for a user.
// The dev eval channel for tools\uicheck starts only with its own environment
// variable, on its own port beside 9333 and 9334, and main starts it.
func TestUICheckServerIsDevOnly(t *testing.T) {
	src, err := os.ReadFile("uicheck_devserver.go")
	if err != nil {
		t.Fatal(err)
	}
	code := codeLines(string(src))
	for _, want := range []string{`"CONTROLCENTRE_UICHECK"`, `"127.0.0.1:9335"`, `"ccUICheckResult"`, "winbind.StartUICheckServer"} {
		if !strings.Contains(code, want) {
			t.Errorf("uicheck_devserver.go does not contain %s", want)
		}
	}
	for _, other := range []string{"9333", "9334"} {
		if strings.Contains(code, other) {
			t.Errorf("uicheck_devserver.go uses port %s, which belongs to another exe", other)
		}
	}
	main, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(main), "startUICheckServer(w)") {
		t.Error("main.go does not start the dev eval server")
	}
	if strings.Contains(codeLines(string(main)), "CONTROLCENTRE_UICHECK") {
		t.Error("main.go reads the variable itself; only winbind.StartUICheckServer may")
	}
}

// codeLines is src without its // comment lines.
func codeLines(src string) string {
	var out []string
	for _, l := range strings.Split(src, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "//") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

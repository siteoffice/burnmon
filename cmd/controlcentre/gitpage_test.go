//go:build windows || darwin

package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
			return []centre.GitPR{{Number: 1}}, nil
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

// TestPageGitRows: one dense row per repo with the eight columns, a unit
// line before each unit's rows, attention rows marked, double-click opens.
func TestPageGitRows(t *testing.T) {
	mustContain(t, "drawGit", "['Repo', 'g1']", "['Branch', 'g2']", "['Dirty', 'gn'", "['+/-', 'gab'",
		"['Last commit', 'gl']", "['Stash', 'gn'", "['WT', 'gn'", "['PRs', 'gn'",
		"'rw uh'", "'Outside any unit'", "r.attention.length > 0", "wireOpen(b, pick)", "gv.pages", "pager(",
		"r.last_subject", "picked.click()")
	mustContain(t, "gitRowDetail", "go('gitrepo', r.id)", "openSel", "open:")
	mustContain(t, "gitAB", "'none'", "'?'")
}

// TestPageGitAsksAgainWhileLoading: the page never waits on git; it asks
// again while the Go side reads, and stops when the page changes.
func TestPageGitAsksAgainWhileLoading(t *testing.T) {
	mustContain(t, "pageGit", "api.git({rows: rowsFit(), refresh: !!again})", "gv.loading", "setTimeout", "seq === drawSeq", "GIT_POLL_MS")
	mustContain(t, "pageGitRepo", "api.gitRepo(", "v.loading", "setTimeout", "seq === drawSeq")
	mustContain(t, "drawGit", "'Refresh'", "pageGit(true)", "drawSeq++", "gv.line", "gv.pr_line")
}

// TestPageGitRepoSections: worktrees with sessions, branches, PRs, tags and
// remotes; each section keeps its size and puts the rest behind "+N more".
func TestPageGitRepoSections(t *testing.T) {
	mustContain(t, "drawRepo", "'Worktrees, sessions matched by folder name'", "'Local branches'", "'Pull requests'",
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

package centre

import (
	"crypto/sha256"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"burnmon/internal/gitrefs"
)

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *testClock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newTestClock() *testClock {
	return &testClock{t: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)}
}

// readView asks, waits for the reading and asks again.
func readView(s *GitScanner, root string, prs, refresh bool) GitView {
	s.View(root, prs, refresh, 40)
	s.Wait()
	return s.View(root, prs, false, 40)
}

func readDetail(t *testing.T, s *GitScanner, root, id string) *GitDetail {
	t.Helper()
	s.Detail(root, id)
	s.Wait()
	v := s.Detail(root, id)
	if !v.Found || v.Detail == nil {
		t.Fatalf("Detail(%s) = %+v", id, v)
	}
	return v.Detail
}

func realDeps(clock *testClock) GitDeps {
	status, why := GitStatusRunner(exec.LookPath)
	return GitDeps{Status: status, StatusWhy: why, Merged: gitrefs.NewChecker().Merged, Now: clock.now,
		Sessions: func() map[string][]string { return map[string][]string{"wt-a": {"claude abc123"}} }}
}

func repoByID(t *testing.T, v GitView, id string) GitRepo {
	t.Helper()
	for _, r := range v.Repos {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("repo %s not in the view", id)
	return GitRepo{}
}

// TestFindReposFixture: every repo to MaxDepth, linked worktrees left out.
func TestFindReposFixture(t *testing.T) {
	root := gitFixtures(t)
	var ids []string
	for _, r := range FindRepos(root) {
		ids = append(ids, r.ID)
	}
	want := []string{"detached", "noupstream", "projects/ahead", "projects/behind", "projects/dirty", "projects/normal", "projects/twowt"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("FindRepos = %v, want %v", ids, want)
	}
}

func TestFindReposDepthAndSkips(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"a/b/c/d/.git", "a/b/c/d/e/.git", "node_modules/x/.git", "_archive/y/.git"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel), "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := FindRepos(root)
	if len(got) != 1 || got[0].ID != "a/b/c/d" {
		t.Errorf("FindRepos = %+v, want only a/b/c/d (depth 4)", got)
	}
}

// TestReadRepoFilesFixture: branch, stash, worktrees and remotes come from
// the .git files.
func TestReadRepoFilesFixture(t *testing.T) {
	root := gitFixtures(t)
	n, err := ReadRepoFiles(filepath.Join(root, "projects", "normal"))
	if err != nil {
		t.Fatal(err)
	}
	if n.Branch != "main" || n.Detached || n.Stashes != 1 || n.Last.Subject != "first commit" || len(n.Worktrees) != 1 {
		t.Errorf("normal = %+v", n)
	}
	if len(n.Remotes) != 1 || n.Remotes[0].Name != "origin" || GitHubSlug(n.Remotes) != "example/normal" {
		t.Errorf("normal remotes = %+v", n.Remotes)
	}
	w, _ := ReadRepoFiles(filepath.Join(root, "projects", "twowt"))
	if len(w.Worktrees) != 3 || !w.Worktrees[0].Main || w.Worktrees[1].Name != "wt-a" || w.Worktrees[1].Branch != "wt-a" || w.Worktrees[2].Branch != "wt-b" {
		t.Errorf("twowt worktrees = %+v", w.Worktrees)
	}
	d, _ := ReadRepoFiles(filepath.Join(root, "detached"))
	if !d.Detached || d.Branch != "" || len(d.HeadSHA) != 40 {
		t.Errorf("detached = %+v", d)
	}
}

// TestScanFixtureRealGit: the full reading with the real git status.
func TestScanFixtureRealGit(t *testing.T) {
	root := gitFixtures(t)
	clock := newTestClock()
	s := NewGitScanner(realDeps(clock))
	first := s.View(root, false, false, 40)
	if !first.Loading || first.ReadAt != "" || len(first.Repos) != 0 {
		t.Errorf("first view = %+v, want loading and empty", first)
	}
	s.Wait()
	v := s.View(root, false, false, 40)
	if v.Loading || v.ReadAt == "" || v.Line != "" {
		t.Errorf("view = loading %v, read_at %q, line %q", v.Loading, v.ReadAt, v.Line)
	}
	var order []string
	for _, r := range v.Repos {
		order = append(order, r.ID)
	}
	// Within a unit the lane orders the repos: Changes (dirty, then twowt
	// through its dirty worktree), Saved (ahead), Shared (behind), published last.
	want := []string{"detached", "noupstream", "projects/dirty", "projects/twowt", "projects/ahead", "projects/behind", "projects/normal"}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
	check := func(id string, f func(GitRepo) bool) {
		t.Helper()
		if r := repoByID(t, v, id); !f(r) {
			t.Errorf("%s = %+v", id, r)
		}
	}
	check("projects/normal", func(r GitRepo) bool {
		return r.Unit == "projects" && r.Branch == "main" && r.Dirty == 0 && r.Stashes == 1 && r.Upstream == "origin/main" &&
			r.Ahead == 0 && r.Behind == 0 && r.LastSHA != "" && r.LastSubject == "first commit" && r.LastAge == "14 d" &&
			r.GitHub == "example/normal" && r.PRs == -1 && len(r.Attention) == 0
	})
	check("projects/dirty", func(r GitRepo) bool { return r.Dirty == 2 && reflect.DeepEqual(r.Attention, []string{"dirty"}) })
	check("projects/ahead", func(r GitRepo) bool { return r.Ahead == 1 && r.Behind == 0 && len(r.Attention) == 0 })
	check("projects/behind", func(r GitRepo) bool { return r.Behind == 1 && reflect.DeepEqual(r.Attention, []string{"behind"}) })
	check("projects/twowt", func(r GitRepo) bool { return r.Worktrees == 2 && r.Dirty == 0 })
	check("detached", func(r GitRepo) bool {
		return r.Detached && r.Branch == "" && r.Unit == "." && reflect.DeepEqual(r.Attention, []string{"detached"})
	})
	check("noupstream", func(r GitRepo) bool { return r.Upstream == "" && r.Ahead == 0 && r.StatusError == "" })

	d := readDetail(t, s, root, "projects/twowt")
	if len(d.Worktrees) != 3 {
		t.Fatalf("worktrees = %+v", d.Worktrees)
	}
	a, b := d.Worktrees[1], d.Worktrees[2]
	if a.Name != "wt-a" || !reflect.DeepEqual(a.Sessions, []string{"claude abc123"}) || a.Dirty != 0 {
		t.Errorf("wt-a = %+v", a)
	}
	if b.Name != "wt-b" || b.Dirty != 1 || len(b.Sessions) != 0 {
		t.Errorf("wt-b = %+v", b)
	}
	wantBr := []GitBranch{{Name: "main", Current: true, Merged: "trunk"}, {Name: "wt-a", Merged: "not merged"}, {Name: "wt-b", Merged: "not merged"}}
	for i := range d.Branches {
		if len(d.Branches[i].Lane) != len(LaneStages) {
			t.Errorf("branch %s has %d lane stages", d.Branches[i].Name, len(d.Branches[i].Lane))
		}
		d.Branches[i].Lane = nil
	}
	if !reflect.DeepEqual(d.Branches, wantBr) {
		t.Errorf("branches = %+v, want %+v", d.Branches, wantBr)
	}
	if !reflect.DeepEqual(d.Tags, []string{"v0.4.0-beta.10", "v0.4.0-beta.3"}) {
		t.Errorf("tags = %v", d.Tags)
	}
	if len(d.Remotes) != 1 || d.Remotes[0].Name != "origin" {
		t.Errorf("remotes = %+v", d.Remotes)
	}
}

// gitTreeHashes hashes every file inside every .git folder below base.
func gitTreeHashes(t *testing.T, base string) map[string][32]byte {
	t.Helper()
	out := map[string][32]byte{}
	_ = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.Contains(filepath.ToSlash(p), "/.git/") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err == nil {
			out[p] = sha256.Sum256(data)
		}
		return nil
	})
	return out
}

// TestScanNeverWritesInsideGit: a reading and every repo page leave every
// file inside every .git folder as it was, and add none.
func TestScanNeverWritesInsideGit(t *testing.T) {
	root := gitFixtures(t)
	clock := newTestClock()
	s := NewGitScanner(realDeps(clock))
	// A first reading lets git settle anything a fresh clone does once.
	readView(s, root, false, false)
	before := gitTreeHashes(t, root)
	v := readView(s, root, false, true)
	for _, r := range v.Repos {
		readDetail(t, s, root, r.ID)
	}
	after := gitTreeHashes(t, root)
	if !reflect.DeepEqual(before, after) {
		for p, h := range after {
			if before[p] != h {
				t.Errorf("changed or added inside .git: %s", p)
			}
		}
		for p := range before {
			if _, ok := after[p]; !ok {
				t.Errorf("removed inside .git: %s", p)
			}
		}
	}
}

// TestOnlyGitStatusRuns: through a fake git, a reading and every repo page
// run GitStatusArgs only: once per repo and once per linked worktree.
func TestOnlyGitStatusRuns(t *testing.T) {
	root := gitFixtures(t)
	dir := installFake(t, "git")
	log := t.TempDir()
	t.Setenv(fakeToolEnv, "git")
	t.Setenv("CENTRE_FAKE_LOG", log)
	status, _ := GitStatusRunner(func(string) (string, error) { return exec.LookPath(filepath.Join(dir, "git")) })
	clock := newTestClock()
	s := NewGitScanner(GitDeps{Status: status, Now: clock.now})
	v := readView(s, root, true, false)
	for _, r := range v.Repos {
		readDetail(t, s, root, r.ID)
	}
	lines := fakeCalls(t, log)
	if len(lines) != 9 {
		t.Errorf("git ran %d times, want 9 (7 repos, 2 linked worktrees)", len(lines))
	}
	for _, l := range lines {
		if strings.TrimSpace(l) != strings.Join(GitStatusArgs, " ") {
			t.Errorf("git ran with %q", l)
		}
	}
}

// fakeGhOnPath puts the fake gh first on PATH with the given mode and
// returns its log folder.
func fakeGhOnPath(t *testing.T, mode string) string {
	t.Helper()
	dir := installFake(t, "gh")
	log := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(fakeToolEnv, "gh")
	t.Setenv("CENTRE_FAKE_GH", mode)
	t.Setenv("CENTRE_FAKE_LOG", log)
	return log
}

func ghCalls(t *testing.T, log string) []string {
	t.Helper()
	return fakeCalls(t, log)
}

// fixtureDepsNoGit reads the fixture without git, so the fake tool env does
// not reach a git run.
func fixtureDepsNoGit(clock *testClock) GitDeps {
	return GitDeps{StatusWhy: GitMissingLine, Now: clock.now, PRs: GhPRRunner(exec.LookPath)}
}

// TestPRSwitchRunsFakeGh: with the switch on, gh runs once per repo with a
// GitHub remote, with the fixed arguments and -R; the result is cached for 5
// minutes; Refresh reads again.
func TestPRSwitchRunsFakeGh(t *testing.T) {
	root := gitFixtures(t)
	log := fakeGhOnPath(t, "ok")
	clock := newTestClock()
	s := NewGitScanner(fixtureDepsNoGit(clock))
	v := readView(s, root, true, false)
	if n := repoByID(t, v, "projects/normal").PRs; n != 2 {
		t.Errorf("normal PRs = %d, want 2", n)
	}
	if n := repoByID(t, v, "projects/dirty").PRs; n != -1 {
		t.Errorf("dirty (no GitHub remote) PRs = %d, want -1", n)
	}
	if v.PRLine != "" || !v.PRsOn {
		t.Errorf("pr_line %q, prs_on %v", v.PRLine, v.PRsOn)
	}
	calls := ghCalls(t, log)
	want := strings.Join(GhArgs, " ") + " -R example/normal"
	if len(calls) != 1 || calls[0] != want {
		t.Fatalf("gh calls = %q, want [%q]", calls, want)
	}
	d := readDetail(t, s, root, "projects/normal")
	if len(d.PRs) != 2 || d.PRs[0].Number != 1 || d.PRs[1].Branch != "work/b" {
		t.Errorf("detail PRs = %+v", d.PRs)
	}
	clock.add(GitRescanAfter + time.Second) // a new reading, inside the PR cache
	readView(s, root, true, false)
	if n := len(ghCalls(t, log)); n != 1 {
		t.Errorf("gh ran %d times within 5 minutes, want 1", n)
	}
	clock.add(PRCacheTTL)
	readView(s, root, true, false)
	if n := len(ghCalls(t, log)); n != 2 {
		t.Errorf("gh ran %d times after 5 minutes, want 2", n)
	}
	readView(s, root, true, true)
	if n := len(ghCalls(t, log)); n != 3 {
		t.Errorf("gh ran %d times after Refresh, want 3", n)
	}
}

// TestPRSwitchOffNeverRunsGh: off by default, gh never runs.
func TestPRSwitchOffNeverRunsGh(t *testing.T) {
	root := gitFixtures(t)
	log := fakeGhOnPath(t, "ok")
	s := NewGitScanner(fixtureDepsNoGit(newTestClock()))
	v := readView(s, root, false, false)
	if n := len(ghCalls(t, log)); n != 0 {
		t.Errorf("gh ran %d times with the switch off", n)
	}
	for _, r := range v.Repos {
		if r.PRs != -1 {
			t.Errorf("%s PRs = %d with the switch off, want -1", r.ID, r.PRs)
		}
	}
}

func TestPRSignedOutLine(t *testing.T) {
	root := gitFixtures(t)
	fakeGhOnPath(t, "signedout")
	s := NewGitScanner(fixtureDepsNoGit(newTestClock()))
	v := readView(s, root, true, false)
	if v.PRLine != GhSignedOutLine {
		t.Errorf("pr_line = %q, want %q", v.PRLine, GhSignedOutLine)
	}
	if repoByID(t, v, "projects/normal").PRs != -1 {
		t.Error("PRs must stay unknown when gh is signed out")
	}
}

func TestPRMissingGhLine(t *testing.T) {
	root := gitFixtures(t)
	t.Setenv("PATH", t.TempDir())
	s := NewGitScanner(fixtureDepsNoGit(newTestClock()))
	v := readView(s, root, true, false)
	if v.PRLine != GhMissingLine {
		t.Errorf("pr_line = %q, want %q", v.PRLine, GhMissingLine)
	}
}

// TestGitMissingShowsLine: without git the rows still come from the .git
// files and the view carries the one line.
func TestGitMissingShowsLine(t *testing.T) {
	root := gitFixtures(t)
	s := NewGitScanner(fixtureDepsNoGit(newTestClock()))
	v := readView(s, root, false, false)
	if v.Line != GitMissingLine {
		t.Errorf("line = %q", v.Line)
	}
	r := repoByID(t, v, "projects/normal")
	if r.Branch != "main" || r.StatusError != GitMissingLine || r.Stashes != 1 {
		t.Errorf("normal without git = %+v", r)
	}
}

// TestScannerRootChangeAndRescan: a new root drops the rows and reads again;
// a reading is reused for GitRescanAfter.
func TestScannerRootChangeAndRescan(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	for _, d := range []string{filepath.Join(a, "r1", ".git"), filepath.Join(b, "r2", ".git"), filepath.Join(b, "r3", ".git")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	clock := newTestClock()
	runs := 0
	var mu sync.Mutex
	s := NewGitScanner(GitDeps{Now: clock.now, Status: func(string) (string, error) {
		mu.Lock()
		runs++
		mu.Unlock()
		return "# branch.head main\n", nil
	}})
	if v := readView(s, a, false, false); len(v.Repos) != 1 || runs != 1 {
		t.Fatalf("root a: %d repos, %d runs", len(v.Repos), runs)
	}
	if v := s.View(a, false, false, 40); v.Loading || runs != 1 {
		t.Errorf("a second view within 30 s read again (loading %v, runs %d)", v.Loading, runs)
	}
	v := s.View(b, false, false, 40)
	if !v.Loading || len(v.Repos) != 0 || v.Root != b {
		t.Errorf("new root must drop the old rows and read: %+v", v)
	}
	s.Wait()
	if v := s.View(b, false, false, 40); len(v.Repos) != 2 {
		t.Errorf("root b: %d repos, want 2", len(v.Repos))
	}
	clock.add(GitRescanAfter + time.Second)
	if v := s.View(b, false, false, 40); !v.Loading {
		t.Error("a reading over 30 s old must start a new one")
	}
	s.Wait()
	if d := s.Detail(a, "r1"); d.Found {
		t.Error("Detail of another root must not be found")
	}
}

// laneRepo builds, with real git, a bare upstream and a clone of it under one
// temp root, and returns a runner for git in the clone and the clone's id.
func laneRepo(t *testing.T) (root string, git func(args ...string), write func(name, body string)) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(base, "empty.gitconfig")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+empty, "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
			"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid",
			"GIT_AUTHOR_DATE=2026-11-01T10:00:00Z", "GIT_COMMITTER_DATE=2026-11-01T10:00:00Z")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	root = filepath.Join(base, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	up := filepath.Join(base, "up.git")
	run(base, "init", "-q", "--bare", "-b", "main", up)
	run(root, "clone", "-q", up, "work")
	work := filepath.Join(root, "work")
	git = func(args ...string) { t.Helper(); run(work, args...) }
	write = func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(work, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, git, write
}

func laneOf(t *testing.T, root string) GitRepo {
	t.Helper()
	s := NewGitScanner(realDeps(newTestClock()))
	return repoByID(t, readView(s, root, false, false), "work")
}

func laneStates(r GitRepo) string {
	out := []string{}
	for _, s := range r.Lane {
		out = append(out, s.State)
	}
	return strings.Join(out, ",")
}

// TestLaneFixturePublishedAndReleased: clean on main with an annotated tag at
// the head is published; one pushed commit after the tag reads Released
// "1 since v1.0.0"; the pull request switch is off, so Review is skipped.
func TestLaneFixturePublishedAndReleased(t *testing.T) {
	root, git, write := laneRepo(t)
	write("a.txt", "one\n")
	git("add", "a.txt")
	git("commit", "-q", "-m", "first")
	git("tag", "-a", "v1.0.0", "-m", "release")
	git("push", "-q", "--follow-tags", "origin", "main")
	r := laneOf(t, root)
	if !r.Published || r.StopsAt != "" || r.Next != "Nothing open." {
		t.Errorf("tag at head: published=%v stops=%q next=%q lane=%s", r.Published, r.StopsAt, r.Next, laneStates(r))
	}
	if got := r.Lane[5]; got.State != "done" || got.Note != "v1.0.0" {
		t.Errorf("Released = %+v, want done v1.0.0", got)
	}
	write("b.txt", "two\n")
	git("add", "b.txt")
	git("commit", "-q", "-m", "second")
	git("push", "-q", "origin", "main")
	r = laneOf(t, root)
	if r.Published || r.StopsAt != "Released" || r.StopsNote != "1 since v1.0.0" {
		t.Errorf("one commit after the tag: published=%v stops=%q %q lane=%s", r.Published, r.StopsAt, r.StopsNote, laneStates(r))
	}
}

// TestLaneFixtureFeatureBranch: a local branch with one commit and no upstream
// stops at Saved; main, left behind unreleased, is listed as open beside it.
func TestLaneFixtureFeatureBranch(t *testing.T) {
	root, git, write := laneRepo(t)
	write("a.txt", "one\n")
	git("add", "a.txt")
	git("commit", "-q", "-m", "first")
	git("tag", "v1.0.0")
	write("b.txt", "two\n")
	git("add", "b.txt")
	git("commit", "-q", "-m", "second")
	git("push", "-q", "origin", "main")
	git("switch", "-q", "-c", "feat")
	write("c.txt", "three\n")
	git("add", "c.txt")
	git("commit", "-q", "-m", "feature work")
	r := laneOf(t, root)
	if r.Published || r.StopsAt != "Saved" || r.StopsNote != "1 to share" {
		t.Errorf("feature: published=%v stops=%q %q lane=%s", r.Published, r.StopsAt, r.StopsNote, laneStates(r))
	}
	if got := laneStates(r); got != "done,active,todo,off,todo,todo" {
		t.Errorf("feature lane = %s", got)
	}
	if len(r.OpenBranches) != 1 || r.OpenBranches[0].Name != "main" || r.OpenBranches[0].Kind != "branch" {
		t.Fatalf("open branches = %+v", r.OpenBranches)
	}
	if o := r.OpenBranches[0]; o.Lane[5].State != "warn" || o.Lane[5].Note != "1 since v1.0.0" {
		t.Errorf("main lane Released = %+v", o.Lane[5])
	}
}

// TestLaneFixtureMergedBranchIsPublished: a branch merged into origin/main
// and checked out reads Merged done through the refs alone.
func TestLaneFixtureMergedBranchIsPublished(t *testing.T) {
	root, git, write := laneRepo(t)
	write("a.txt", "one\n")
	git("add", "a.txt")
	git("commit", "-q", "-m", "first")
	git("push", "-q", "origin", "main")
	git("switch", "-q", "-c", "feat")
	write("b.txt", "two\n")
	git("add", "b.txt")
	git("commit", "-q", "-m", "feature work")
	git("push", "-q", "-u", "origin", "feat")
	git("switch", "-q", "main")
	git("merge", "-q", "--no-ff", "-m", "merge feat", "feat")
	git("push", "-q", "origin", "main")
	git("switch", "-q", "feat")
	r := laneOf(t, root)
	if got := laneStates(r); got != "done,done,done,off,done,done" {
		t.Errorf("merged feat lane = %s", got)
	}
	if !r.Published {
		t.Errorf("published = false, stops %q %q, open %+v", r.StopsAt, r.StopsNote, r.OpenBranches)
	}
}

// TestViewStageFilter: a stage filter keeps only the open repos that stop at
// that stage, pages the filtered rows, and leaves the summary (the strip) on
// all repos; the strip counts equal the rows each filter shows.
func TestViewStageFilter(t *testing.T) {
	root := gitFixtures(t)
	s := NewGitScanner(realDeps(newTestClock()))
	all := readView(s, root, false, false)
	if all.Summary.Total != len(all.Repos) || all.Summary.Open+all.Summary.Published != all.Summary.Total {
		t.Fatalf("summary = %+v for %d repos", all.Summary, len(all.Repos))
	}
	if all.Summary.Open == 0 || all.Summary.Published == 0 {
		t.Fatalf("the fixture should have open and published repos: %+v", all.Summary)
	}
	sum := 0
	for _, stage := range LaneStages {
		v := s.ViewStage(root, false, false, 40, stage)
		if len(v.Repos) != all.Summary.Stages[stage] {
			t.Errorf("filter %s shows %d rows, the strip says %d", stage, len(v.Repos), all.Summary.Stages[stage])
		}
		for _, r := range v.Repos {
			if r.Published || r.StopsAt != stage {
				t.Errorf("filter %s shows %s, which stops at %q (published %v)", stage, r.ID, r.StopsAt, r.Published)
			}
		}
		if v.Summary.Total != all.Summary.Total {
			t.Errorf("filter %s changed the summary: %+v", stage, v.Summary)
		}
		sum += len(v.Repos)
	}
	if sum != all.Summary.Open {
		t.Errorf("the stage counts add up to %d, open is %d", sum, all.Summary.Open)
	}
	if v := s.ViewStage(root, false, false, 40, "nonsense"); len(v.Repos) != 0 {
		t.Errorf("an unknown stage shows %d rows", len(v.Repos))
	}
}

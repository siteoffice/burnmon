package centre

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// The fixture repos are built by real git at test time, once per run: a
// nested .git cannot be checked in, and a linked worktree's .git file holds
// an absolute path. The porcelain samples are checked in under
// testdata\gitstatus. Fixture root layout (every repo is a clone of
// upstream, which lies outside the root):
//
//	siteoffice.json            the root unit
//	projects/siteoffice.json   unit "projects"
//	projects/normal            clean, one stash, GitHub origin URL
//	projects/dirty             one changed and one untracked file
//	projects/ahead             one local commit
//	projects/behind            upstream moved on, fetched
//	projects/twowt             two linked worktrees, wt-a and wt-b
//	projects/wt-a, wt-b        the linked worktrees (not repos)
//	detached                   HEAD detached
//	noupstream                 git init, one commit, no remote

const fakeToolEnv = "CENTRE_FAKE_TOOL"

// TestMain runs the test binary as a fake gh or git when asked to.
func TestMain(m *testing.M) {
	switch os.Getenv(fakeToolEnv) {
	case "gh":
		os.Exit(fakeGh())
	case "git":
		os.Exit(fakeGit())
	case "warm":
		os.Exit(0)
	}
	code := m.Run()
	if fixtureBase != "" {
		os.RemoveAll(fixtureBase)
	}
	if fakeBase != "" {
		os.RemoveAll(fakeBase)
	}
	os.Exit(code)
}

// logArgs writes the arguments to a new file in the CENTRE_FAKE_LOG folder:
// one file per call, since parallel appends to one file lose lines on Windows.
func logArgs() {
	if dir := os.Getenv("CENTRE_FAKE_LOG"); dir != "" {
		if f, err := os.CreateTemp(dir, "call-*"); err == nil {
			fmt.Fprint(f, strings.Join(os.Args[1:], " "))
			f.Close()
		}
	}
}

// fakeCalls returns the logged calls in the folder, one per call.
func fakeCalls(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(data))
	}
	return out
}

// fakeGh: mode ok prints two pull requests built with encoding/json; mode
// signedout prints gh's sign-in hint and exits 4.
func fakeGh() int {
	logArgs()
	if os.Getenv("CENTRE_FAKE_GH") == "signedout" {
		fmt.Fprintln(os.Stderr, "To get started with GitHub CLI, please run:  gh auth login")
		return 4
	}
	type pr struct {
		Number      int    `json:"number"`
		Title       string `json:"title"`
		HeadRefName string `json:"headRefName"`
		State       string `json:"state"`
	}
	data, _ := json.Marshal([]pr{{1, "one", "work/a", "OPEN"}, {2, "two", "work/b", "OPEN"}})
	os.Stdout.Write(data)
	return 0
}

// fakeGit logs its arguments and prints a clean status.
func fakeGit() int {
	logArgs()
	io.WriteString(os.Stdout, "# branch.oid 0000000000000000000000000000000000000000\n# branch.head main\n")
	return 0
}

var (
	fakeMu   sync.Mutex
	fakeDirs = map[string]string{}
	fakeBase string
)

// installFake copies the test binary into a folder of its own as name
// (name.exe on Windows), once per run, and runs it once without a timeout:
// a virus scanner checks a new exe on its first starts, which would
// otherwise eat the 5 s and 20 s timeouts of the timed runs.
func installFake(t *testing.T, name string) string {
	t.Helper()
	fakeMu.Lock()
	defer fakeMu.Unlock()
	if dir, ok := fakeDirs[name]; ok {
		return dir
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if fakeBase == "" {
		if fakeBase, err = os.MkdirTemp("", "centre-fakes-"); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(fakeBase, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := name
	if runtime.GOOS == "windows" {
		file += ".exe"
	}
	src, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, file)
	if err := os.WriteFile(exe, src, 0o755); err != nil {
		t.Fatal(err)
	}
	warm := exec.Command(exe)
	warm.Env = append(os.Environ(), fakeToolEnv+"=warm")
	if out, err := warm.CombinedOutput(); err != nil {
		t.Fatalf("warm-up run of the fake %s: %v\n%s", name, err, out)
	}
	fakeDirs[name] = dir
	return dir
}

var (
	fixtureOnce sync.Once
	fixtureBase string
	fixtureRoot string
	fixtureErr  error
)

// gitFixtures builds the fixture root once and returns it; it skips the
// test when git is not on PATH.
func gitFixtures(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	fixtureOnce.Do(func() { fixtureBase, fixtureRoot, fixtureErr = buildGitFixtures() })
	if fixtureErr != nil {
		t.Fatal(fixtureErr)
	}
	return fixtureRoot
}

func buildGitFixtures() (string, string, error) {
	base, err := os.MkdirTemp("", "centre-git-")
	if err != nil {
		return "", "", err
	}
	// Resolve 8.3 names and symlinks (macOS /var -> /private/var), so paths
	// git writes match the paths the test compares.
	if r, err := filepath.EvalSymlinks(base); err == nil {
		base = r
	}
	empty := filepath.Join(base, "empty.gitconfig")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		return base, "", err
	}
	date := "2026-11-01T10:00:00Z"
	run := func(dir string, args ...string) error {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+empty, "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
			"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid",
			"GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
		}
		return nil
	}
	write := func(p, body string) error {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		return os.WriteFile(p, []byte(body), 0o644)
	}
	up := filepath.Join(base, "upstream")
	root := filepath.Join(base, "root")
	proj := filepath.Join(root, "projects")
	steps := []func() error{
		func() error { return os.MkdirAll(up, 0o755) },
		func() error { return run(up, "init", "-q", "-b", "main") },
		func() error { return write(filepath.Join(up, "a.txt"), "one\n") },
		func() error { return run(up, "add", "a.txt") },
		func() error { return run(up, "commit", "-q", "-m", "first commit") },
		func() error { return run(up, "tag", "v0.4.0-beta.3") },
		func() error { return run(up, "tag", "v0.4.0-beta.10") },
		func() error { return write(filepath.Join(root, "siteoffice.json"), `{"name":"fixture-root"}`) },
		func() error { return write(filepath.Join(proj, "siteoffice.json"), `{"name":"projects"}`) },
	}
	for _, name := range []string{"normal", "dirty", "ahead", "behind", "twowt"} {
		name := name
		steps = append(steps, func() error { return run(proj, "clone", "-q", up, name) })
	}
	steps = append(steps, func() error { return run(root, "clone", "-q", up, "detached") })
	normal, dirty, ahead, behind, twowt := filepath.Join(proj, "normal"), filepath.Join(proj, "dirty"),
		filepath.Join(proj, "ahead"), filepath.Join(proj, "behind"), filepath.Join(proj, "twowt")
	noup := filepath.Join(root, "noupstream")
	steps = append(steps,
		// normal: one stash, then a GitHub origin URL (never fetched again).
		func() error { return write(filepath.Join(normal, "a.txt"), "stashed\n") },
		func() error { return run(normal, "stash", "-q") },
		func() error { return run(normal, "remote", "set-url", "origin", "https://github.com/example/normal.git") },
		// dirty: one changed, one untracked.
		func() error { return write(filepath.Join(dirty, "a.txt"), "changed\n") },
		func() error { return write(filepath.Join(dirty, "new.txt"), "new\n") },
		// ahead: one local commit, later than the rest.
		func() error { return write(filepath.Join(ahead, "b.txt"), "b\n") },
		func() error { return run(ahead, "add", "b.txt") },
		func() error { date = "2026-11-10T10:00:00Z"; return run(ahead, "commit", "-q", "-m", "local work") },
		// behind: upstream moves on, the clone fetches.
		func() error { return write(filepath.Join(up, "c.txt"), "c\n") },
		func() error { return run(up, "add", "c.txt") },
		func() error { date = "2026-11-05T10:00:00Z"; return run(up, "commit", "-q", "-m", "upstream moved") },
		func() error { return run(behind, "fetch", "-q") },
		// twowt: two linked worktrees beside it.
		func() error { return run(twowt, "worktree", "add", "-q", "-b", "wt-a", filepath.Join(proj, "wt-a")) },
		func() error { return run(twowt, "worktree", "add", "-q", "-b", "wt-b", filepath.Join(proj, "wt-b")) },
		func() error { return write(filepath.Join(proj, "wt-b", "x.txt"), "x\n") },
		// detached and noupstream.
		func() error { return run(filepath.Join(root, "detached"), "checkout", "-q", "--detach") },
		func() error { return os.MkdirAll(noup, 0o755) },
		func() error { return run(noup, "init", "-q", "-b", "main") },
		func() error { return write(filepath.Join(noup, "n.txt"), "n\n") },
		func() error { return run(noup, "add", "n.txt") },
		func() error { date = "2026-11-12T10:00:00Z"; return run(noup, "commit", "-q", "-m", "no upstream") },
	)
	for _, s := range steps {
		if err := s(); err != nil {
			return base, "", err
		}
	}
	return base, root, nil
}

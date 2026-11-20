package centre

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// D18: a clean checkout detached exactly at a tag is a release checkout, not a
// red "detached" repo. Detached with changed files, or at a commit with no tag,
// keep today's reading.

func atTagLane() LaneIn {
	return LaneIn{Detached: true, StatusRead: true, AtTag: "v1.0.0"}
}

func TestLaneOfDetachedAtTagCleanIsNotOpen(t *testing.T) {
	lane := LaneOf(atTagLane())
	if len(lane) != len(LaneStages) {
		t.Fatalf("lane has %d stages", len(lane))
	}
	if lane[0].State != LaneDone || lane[0].Note != "at tag v1.0.0" {
		t.Errorf("Changes = %s %q, want done %q", lane[0].State, lane[0].Note, "at tag v1.0.0")
	}
	if _, _, pub := StopsAt(lane); !pub {
		t.Errorf("a clean checkout at a tag stops at a stage: %+v", lane)
	}
	if LaneOpen(lane) {
		t.Errorf("a clean checkout at a tag counts as open: %+v", lane)
	}
	if got := NextOf(lane); got != "Nothing open." {
		t.Errorf("NextOf = %q, want %q", got, "Nothing open.")
	}
}

func TestLaneOfDetachedAtTagWithChangesKeepsToday(t *testing.T) {
	for name, mod := range map[string]func(*LaneIn){
		"dirty":     func(l *LaneIn) { l.Dirty = 2 },
		"conflicts": func(l *LaneIn) { l.Dirty, l.Conflicts = 1, 1 },
		"not read":  func(l *LaneIn) { l.StatusRead = false },
	} {
		in := atTagLane()
		mod(&in)
		lane := LaneOf(in)
		if lane[0].State != LaneBad || lane[0].Note != "detached" {
			t.Errorf("%s: Changes = %s %q, want bad %q", name, lane[0].State, lane[0].Note, "detached")
		}
		if got := NextOf(lane); got != "HEAD is detached: put it on a branch." {
			t.Errorf("%s: NextOf = %q", name, got)
		}
	}
}

func TestLaneOfDetachedWithoutTagKeepsToday(t *testing.T) {
	in := LaneIn{Detached: true, StatusRead: true}
	lane := LaneOf(in)
	if lane[0].State != LaneBad || lane[0].Note != "detached" {
		t.Errorf("Changes = %s %q, want bad %q", lane[0].State, lane[0].Note, "detached")
	}
	if got := NextOf(lane); got != "HEAD is detached: put it on a branch." {
		t.Errorf("NextOf = %q", got)
	}
}

// tagRepo builds one repo with real git: a commit tagged v1.0.0 (annotated when
// annotated is true), a second commit with no tag, and then the checkout the
// case names. It returns the root that holds the repo.
func tagRepo(t *testing.T, name string, annotated bool, setup func(run func(args ...string), repo string)) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	root := t.TempDir()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	empty := filepath.Join(root, "empty.gitconfig")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, name)
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+empty, "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
			"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid",
			"GIT_AUTHOR_DATE=2026-11-01T10:00:00Z", "GIT_COMMITTER_DATE=2026-11-01T10:00:00Z")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	write := func(file, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, file), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "siteoffice.json"), []byte(`{"name":"tagroot"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	origin := filepath.Join(root, "origin.git")
	run("init", "-q", "-b", "main")
	write("a.txt", "one\n")
	run("add", "a.txt")
	run("commit", "-q", "-m", "first")
	if annotated {
		run("tag", "-a", "-m", "release", "v1.0.0")
	} else {
		run("tag", "v1.0.0")
	}
	// main and the tag are online, so main itself has nothing left to share.
	run("init", "-q", "--bare", "-b", "main", origin)
	run("remote", "add", "origin", origin)
	run("push", "-q", "-u", "origin", "main", "v1.0.0")
	setup(run, repo)
	return root
}

// commitMore adds a second, untagged commit on main and shares it.
func commitMore(t *testing.T, run func(args ...string), repo string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "b.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "b.txt")
	run("commit", "-q", "-m", "second")
	run("push", "-q", "origin", "main")
}

func scanOne(t *testing.T, root, id string) GitRepo {
	t.Helper()
	s := NewGitScanner(realDeps(newTestClock()))
	return repoByID(t, readView(s, root, false, false), id)
}

func TestScanDetachedAtTagCleanIsPublished(t *testing.T) {
	for _, annotated := range []bool{false, true} {
		name := fmt.Sprintf("annotated=%v", annotated)
		root := tagRepo(t, "rel", annotated, func(run func(args ...string), repo string) {
			run("checkout", "-q", "--detach", "v1.0.0")
		})
		r := scanOne(t, root, "rel")
		if !r.Detached || r.AtTag != "v1.0.0" {
			t.Errorf("%s: Detached %v AtTag %q, want true and v1.0.0", name, r.Detached, r.AtTag)
		}
		if !r.Published || r.StopsAt != "" {
			t.Errorf("%s: Published %v StopsAt %q, want published", name, r.Published, r.StopsAt)
		}
		if r.Next != "Nothing open." {
			t.Errorf("%s: Next = %q", name, r.Next)
		}
		if r.Lane[0].State != LaneDone || r.Lane[0].Note != "at tag v1.0.0" {
			t.Errorf("%s: Changes = %+v", name, r.Lane[0])
		}
		if len(r.Attention) != 0 {
			t.Errorf("%s: Attention = %v, want none", name, r.Attention)
		}
	}
}

func TestScanDetachedAtTagWithChangesStaysRed(t *testing.T) {
	root := tagRepo(t, "rel", false, func(run func(args ...string), repo string) {
		run("checkout", "-q", "--detach", "v1.0.0")
		if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	r := scanOne(t, root, "rel")
	if !r.Detached || r.AtTag != "" || r.Published {
		t.Errorf("Detached %v AtTag %q Published %v, want detached, no tag, open", r.Detached, r.AtTag, r.Published)
	}
	if r.Lane[0].State != LaneBad || r.Lane[0].Note != "detached" {
		t.Errorf("Changes = %+v, want bad detached", r.Lane[0])
	}
	if !reflect.DeepEqual(r.Attention, []string{"dirty", "detached"}) {
		t.Errorf("Attention = %v", r.Attention)
	}
}

func TestScanDetachedAtCommitWithoutTagStaysRed(t *testing.T) {
	root := tagRepo(t, "rel", false, func(run func(args ...string), repo string) {
		commitMore(t, run, repo)
		run("checkout", "-q", "--detach", "main")
	})
	r := scanOne(t, root, "rel")
	if !r.Detached || r.AtTag != "" || r.Published {
		t.Errorf("Detached %v AtTag %q Published %v, want detached, no tag, open", r.Detached, r.AtTag, r.Published)
	}
	if r.Lane[0].State != LaneBad || r.Lane[0].Note != "detached" {
		t.Errorf("Changes = %+v, want bad detached", r.Lane[0])
	}
	if !reflect.DeepEqual(r.Attention, []string{"detached"}) {
		t.Errorf("Attention = %v", r.Attention)
	}
}

// Sitting at an old tag must not hide main's own open work: main is one commit
// past the tag, so it still shows as an open branch ("Released: 1 since v1.0.0").
func TestScanAtTagStillListsMainWhenItIsPastTheTag(t *testing.T) {
	root := tagRepo(t, "rel", false, func(run func(args ...string), repo string) {
		commitMore(t, run, repo)
		run("checkout", "-q", "--detach", "v1.0.0")
	})
	r := scanOne(t, root, "rel")
	if r.AtTag != "v1.0.0" {
		t.Fatalf("AtTag = %q, want v1.0.0", r.AtTag)
	}
	if r.Published || len(r.OpenBranches) != 1 || r.OpenBranches[0].Name != "main" {
		t.Errorf("Published %v OpenBranches %+v, want main listed as open", r.Published, r.OpenBranches)
	}
}

// A linked worktree detached at a tag reads the same way as the checkout.
func TestScanWorktreeDetachedAtTag(t *testing.T) {
	root := tagRepo(t, "rel", false, func(run func(args ...string), repo string) {
		run("worktree", "add", "-q", "--detach", filepath.Join(filepath.Dir(repo), "rel-wt"), "v1.0.0")
	})
	s := NewGitScanner(realDeps(newTestClock()))
	readView(s, root, false, false)
	d := readDetail(t, s, root, "rel")
	if len(d.Worktrees) != 2 {
		t.Fatalf("worktrees = %+v", d.Worktrees)
	}
	w := d.Worktrees[1]
	if !w.Detached || w.AtTag != "v1.0.0" || w.Lane[0].Note != "at tag v1.0.0" {
		t.Errorf("worktree = detached %v at tag %q changes %+v", w.Detached, w.AtTag, w.Lane[0])
	}
}

package gitrefs

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	mergedBranch = "sprint/example-merged"
	openBranch   = "sprint/example-open"
)

func writeRefs(t *testing.T, gitDir string, f fixture) {
	t.Helper()
	setRef(t, gitDir, "refs/heads/main", f.m)
	setRef(t, gitDir, "refs/heads/"+mergedBranch, f.b)
	setRef(t, gitDir, "refs/heads/"+openBranch, f.d)
}

func expect(t *testing.T, c *Checker, repo, branch string, wantMerged, wantKnown bool) {
	t.Helper()
	m, k := c.Merged(repo, branch)
	if m != wantMerged || k != wantKnown {
		t.Fatalf("Merged(%q) = (%v, %v), want (%v, %v)", branch, m, k, wantMerged, wantKnown)
	}
}

func TestMergedSprintBranch(t *testing.T) {
	repo, gd := newRepo(t)
	writeRefs(t, gd, buildGraph(t, gd))
	expect(t, NewChecker(), repo, mergedBranch, true, true)
}

func TestUnmergedSprintBranch(t *testing.T) {
	repo, gd := newRepo(t)
	writeRefs(t, gd, buildGraph(t, gd))
	expect(t, NewChecker(), repo, openBranch, false, true)
}

func TestFastForwardIsMerged(t *testing.T) {
	repo, gd := newRepo(t)
	a := writeCommit(t, gd)
	b := writeCommit(t, gd, a)
	setRef(t, gd, "refs/heads/main", b)
	setRef(t, gd, "refs/heads/"+mergedBranch, b)
	expect(t, NewChecker(), repo, mergedBranch, true, true)
}

func TestPackedRefsOnly(t *testing.T) {
	repo, gd := newRepo(t)
	f := buildGraph(t, gd)
	setPacked(t, gd, map[string]string{
		"refs/heads/main":            f.m,
		"refs/heads/" + mergedBranch: f.b,
		"refs/heads/" + openBranch:   f.d,
		"refs/tags/example-v1":       f.a,
	})
	c := NewChecker()
	expect(t, c, repo, mergedBranch, true, true)
	expect(t, c, repo, openBranch, false, true)

	// A loose ref wins over a packed one with the same name.
	setRef(t, gd, "refs/heads/"+openBranch, f.b)
	got, err := ResolveRef(gd, "refs/heads/"+openBranch)
	if err != nil || got != f.b {
		t.Fatalf("loose ref should win: got %q err %v, want %q", got, err, f.b)
	}
	expect(t, NewChecker(), repo, openBranch, true, true)
}

func TestHandmadePack(t *testing.T) {
	repo, gd := newRepo(t)
	a, ab := commitObj()
	b, bb := commitObj(a)
	c, cb := commitObj(a)
	m, mb := commitObj(c, b)
	d, db := commitObj(b)
	writePack(t, gd, [][]byte{ab, bb, cb, mb, db})
	setRef(t, gd, "refs/heads/main", m)
	setRef(t, gd, "refs/heads/"+mergedBranch, b)
	setRef(t, gd, "refs/heads/"+openBranch, d)
	c2 := NewChecker()
	expect(t, c2, repo, mergedBranch, true, true)
	expect(t, c2, repo, openBranch, false, true)
}

func TestApplyDelta(t *testing.T) {
	base := []byte("hello world")
	// src size 11, dst size 14: copy 6 bytes from offset 0, insert "there", copy "ld" (offset 9, size 2) ... built by hand.
	// copy off=0 size=6 ("hello "), insert "there!", copy off=9 size=2 ("ld")
	delta := []byte{11, 14,
		0x80 | 0x10, 6, // copy, size byte only, offset 0
		6, 't', 'h', 'e', 'r', 'e', '!',
		0x80 | 0x01 | 0x10, 9, 2, // copy offset 9, size 2
	}
	got, err := applyDelta(base, delta)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello there!ld" {
		t.Fatalf("got %q", got)
	}
	// A delta that claims a different source size is refused.
	if _, err := applyDelta([]byte("x"), delta); err == nil {
		t.Fatal("wrong source size must fail")
	}
	// A copy beyond the base is refused.
	bad := []byte{11, 3, 0x80 | 0x01 | 0x10, 10, 3}
	if _, err := applyDelta(base, bad); err == nil {
		t.Fatal("copy past the base must fail")
	}
}

func TestUnknownCases(t *testing.T) {
	t.Run("missing branch ref", func(t *testing.T) {
		repo, gd := newRepo(t)
		f := buildGraph(t, gd)
		setRef(t, gd, "refs/heads/main", f.m)
		expect(t, NewChecker(), repo, mergedBranch, false, false)
	})
	t.Run("missing main ref", func(t *testing.T) {
		repo, gd := newRepo(t)
		f := buildGraph(t, gd)
		setRef(t, gd, "refs/heads/"+mergedBranch, f.b)
		expect(t, NewChecker(), repo, mergedBranch, false, false)
	})
	t.Run("not a repo", func(t *testing.T) {
		expect(t, NewChecker(), t.TempDir(), mergedBranch, false, false)
	})
	t.Run("invalid branch name", func(t *testing.T) {
		repo, gd := newRepo(t)
		writeRefs(t, gd, buildGraph(t, gd))
		expect(t, NewChecker(), repo, "--status=running", false, false)
	})
	t.Run("commit object missing from the walk", func(t *testing.T) {
		repo, gd := newRepo(t)
		f := buildGraph(t, gd)
		writeRefs(t, gd, f)
		if err := os.Remove(looseObjectPath(gd, f.a)); err != nil {
			t.Fatal(err)
		}
		expect(t, NewChecker(), repo, openBranch, false, false)
	})
	t.Run("corrupt zlib object", func(t *testing.T) {
		repo, gd := newRepo(t)
		f := buildGraph(t, gd)
		writeRefs(t, gd, f)
		if err := os.WriteFile(looseObjectPath(gd, f.c), []byte("not zlib at all"), 0o644); err != nil {
			t.Fatal(err)
		}
		expect(t, NewChecker(), repo, openBranch, false, false)
	})
	t.Run("truncated pack file", func(t *testing.T) {
		repo, gd := newRepo(t)
		a, ab := commitObj()
		b, bb := commitObj(a)
		m, mb := commitObj(b)
		d, db := commitObj(a)
		pk, _ := writePack(t, gd, [][]byte{ab, bb, mb, db})
		if err := os.Truncate(pk, 12); err != nil {
			t.Fatal(err)
		}
		setRef(t, gd, "refs/heads/main", m)
		setRef(t, gd, "refs/heads/"+openBranch, d)
		expect(t, NewChecker(), repo, openBranch, false, false)
	})
	t.Run("walk limit", func(t *testing.T) {
		repo, gd := newRepo(t)
		prev := writeCommit(t, gd)
		for i := 0; i < 8; i++ {
			prev = writeCommit(t, gd, prev)
		}
		other := writeCommit(t, gd)
		setRef(t, gd, "refs/heads/main", prev)
		setRef(t, gd, "refs/heads/"+openBranch, other)
		old := walkLimit
		walkLimit = 4
		defer func() { walkLimit = old }()
		expect(t, NewChecker(), repo, openBranch, false, false)
	})
}

func TestWorktreeGitFile(t *testing.T) {
	mainRepo, gd := newRepo(t)
	wtMeta := filepath.Join(gd, "worktrees", "wt")
	if err := os.MkdirAll(wtMeta, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt := t.TempDir()
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+wtMeta+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := GitDir(wt)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(gd) {
		t.Fatalf("GitDir = %q, want %q", got, gd)
	}
	// And the plain repo resolves to its own .git.
	got, err = GitDir(mainRepo)
	if err != nil || filepath.Clean(got) != filepath.Clean(gd) {
		t.Fatalf("GitDir(main) = %q, %v", got, err)
	}
	// A worktree folder works for Merged too.
	f := buildGraph(t, gd)
	writeRefs(t, gd, f)
	expect(t, NewChecker(), wt, mergedBranch, true, true)
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-c", "user.name=example", "-c", "user.email=e@example.com", "-c", "commit.gpgsign=false", "-c", "core.autocrlf=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func TestPackedObjects(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q", "-b", "main")
	gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "A")
	gitIn(t, repo, "checkout", "-q", "-b", mergedBranch)
	gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "B")
	gitIn(t, repo, "checkout", "-q", "main")
	gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "C")
	gitIn(t, repo, "merge", "-q", "--no-ff", "-m", "M", mergedBranch)
	gitIn(t, repo, "checkout", "-q", "-b", openBranch, mergedBranch)
	gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "D")
	gitIn(t, repo, "checkout", "-q", "main")
	for i := 0; i < 3; i++ {
		gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "filler "+strconv.Itoa(i))
	}
	gitIn(t, repo, "gc", "-q", "--aggressive", "--prune=now")

	// Prove the objects really are packed now.
	packs, _ := filepath.Glob(filepath.Join(repo, ".git", "objects", "pack", "*.idx"))
	if len(packs) == 0 {
		t.Fatal("git gc left no pack")
	}
	c := NewChecker()
	expect(t, c, repo, mergedBranch, true, true)
	expect(t, c, repo, openBranch, false, true)
}

func TestValidBranch(t *testing.T) {
	good := []string{"sprint/burnmon-b6-map-and-waits", "main", "fix/a_b.c"}
	for _, n := range good {
		if !ValidBranch(n) {
			t.Errorf("ValidBranch(%q) = false, want true", n)
		}
	}
	bad := []string{"", "-x", "--status=running", "a b", "a..b", "a~1", "a^", "a:b", "a?", "a*", "a[", `a\b`,
		"a\n", "a\x00b", "a\x7f", "a.lock", "a/", "/a", "a//b", "a/.b", "@", "a@{b", "x.", "sprint/ok.lock"}
	for _, n := range bad {
		if ValidBranch(n) {
			t.Errorf("ValidBranch(%q) = true, want false", n)
		}
	}
}

func TestNoGitCommand(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), n, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			if strings.Trim(imp.Path.Value, `"`) == "os/exec" {
				t.Errorf("%s imports os/exec", n)
			}
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no non-test .go files found")
	}
}

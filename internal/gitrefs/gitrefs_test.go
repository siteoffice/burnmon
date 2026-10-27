package gitrefs

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/hex"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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

// A fast-forward, or a branch that never got a commit of its own, leaves the
// branch tip on main's first-parent history: not merged (Foreman merges with
// --no-ff), but known.
func TestFastForwardOrZeroCommitBranchIsNotMerged(t *testing.T) {
	repo, gd := newRepo(t)
	a := writeCommit(t, gd)
	b := writeCommit(t, gd, a)
	setRef(t, gd, "refs/heads/main", b)
	setRef(t, gd, "refs/heads/"+mergedBranch, b)
	expect(t, NewChecker(), repo, mergedBranch, false, true)
}

func TestBranchAtOlderMainCommitIsNotMerged(t *testing.T) {
	repo, gd := newRepo(t)
	a := writeCommit(t, gd)
	b := writeCommit(t, gd, a)
	c := writeCommit(t, gd, b)
	setRef(t, gd, "refs/heads/main", c)
	setRef(t, gd, "refs/heads/"+openBranch, a)
	expect(t, NewChecker(), repo, openBranch, false, true)
}

func TestCacheIsBounded(t *testing.T) {
	repo, gd := newRepo(t)
	a := writeCommit(t, gd)
	setRef(t, gd, "refs/heads/main", a)
	old := maxCacheEntries
	maxCacheEntries = 3
	defer func() { maxCacheEntries = old }()
	c := NewChecker()
	for i := 0; i < 10; i++ {
		setRef(t, gd, "refs/heads/"+openBranch, writeCommit(t, gd))
		expect(t, c, repo, openBranch, false, true)
		if len(c.cache) > 3 {
			t.Fatalf("cache holds %d entries after %d calls, want at most 3", len(c.cache), i+1)
		}
	}
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

// A corrupt fanout (an entry above the object count, or one that goes down)
// must make the pack unreadable, never panic in a lookup.
func TestCorruptFanoutIsUnknown(t *testing.T) {
	repo, gd := newRepo(t)
	a, ab := commitObj()
	b, bb := commitObj(a)
	m, mb := commitObj(b)
	d, db := commitObj(a)
	_, idxPath := writePack(t, gd, [][]byte{ab, bb, mb, db})
	idx, err := os.ReadFile(idxPath)
	if err != nil {
		t.Fatal(err)
	}
	// Make every lookup's upper bound huge: fan[b0] = 0xFFFFFFF0 for each name's first byte.
	for _, sha := range []string{a, b, m, d} {
		raw, _ := hex.DecodeString(sha)
		binary.BigEndian.PutUint32(idx[8+int(raw[0])*4:], 0xFFFFFFF0)
		if raw[0] > 0 {
			binary.BigEndian.PutUint32(idx[8+(int(raw[0])-1)*4:], 0)
		}
	}
	if err := os.WriteFile(idxPath, idx, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := openPack(idxPath); err == nil {
		t.Fatal("openPack accepted a corrupt fanout")
	}
	setRef(t, gd, "refs/heads/main", m)
	setRef(t, gd, "refs/heads/"+openBranch, d)
	expect(t, NewChecker(), repo, openBranch, false, false)

	// A decreasing fanout is refused too.
	idx2, _ := os.ReadFile(idxPath)
	for i := 0; i < 256; i++ {
		binary.BigEndian.PutUint32(idx2[8+i*4:], 0)
	}
	binary.BigEndian.PutUint32(idx2[8+10*4:], 4)
	binary.BigEndian.PutUint32(idx2[8+255*4:], 4)
	if err := os.WriteFile(idxPath, idx2, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := openPack(idxPath); err == nil {
		t.Fatal("openPack accepted a decreasing fanout")
	}
}

// buildDeltaChain writes a pack file with one 400-byte blob and levels OFS
// deltas stacked on it (each copies the whole base), returns the pack and the
// offset of the last object.
func buildDeltaChain(t *testing.T, levels int) (*os.File, uint64) {
	t.Helper()
	z := func(b []byte) []byte {
		var buf bytes.Buffer
		w := zlib.NewWriter(&buf)
		w.Write(b)
		w.Close()
		return buf.Bytes()
	}
	var pk bytes.Buffer
	pk.WriteString("PACK\x00\x00\x00\x02\x00\x00\x00\x00")
	last := pk.Len()
	pk.Write([]byte{0xB0, 0x19}) // blob, size 400
	pk.Write(z(bytes.Repeat([]byte("a"), 400)))
	delta := []byte{0x90, 0x03, 0x90, 0x03, 0xB0, 0x90, 0x01} // src 400, dst 400, copy 400 from 0
	for i := 0; i < levels; i++ {
		start := pk.Len()
		rel := start - last
		if rel >= 128 {
			t.Fatalf("fixture: ofs distance %d needs more than one byte", rel)
		}
		pk.Write([]byte{0x60 | byte(len(delta)), byte(rel)})
		pk.Write(z(delta))
		last = start
	}
	f, err := os.Create(filepath.Join(t.TempDir(), "chain.pack"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(pk.Bytes()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f, uint64(last)
}

func TestDeltaChainTotalAllocationIsCapped(t *testing.T) {
	f, off := buildDeltaChain(t, 3)
	p := &pack{f: f}
	s := newStore(t.TempDir())
	if _, body, err := p.read(off, s, 0); err != nil || len(body) != 400 {
		t.Fatalf("uncapped read: %d bytes, err %v; want 400 bytes", len(body), err)
	}
	old := maxChainBytes
	maxChainBytes = 1000 // the chain allocates 400 + 3*(400+7) bytes
	defer func() { maxChainBytes = old }()
	s = newStore(t.TempDir())
	if _, _, err := p.read(off, s, 0); err == nil {
		t.Fatal("a chain over the byte cap was read")
	}
}

func TestDeltaChainHonoursDeadline(t *testing.T) {
	f, off := buildDeltaChain(t, 3)
	p := &pack{f: f}
	s := newStore(t.TempDir())
	s.deadline = time.Now().Add(-time.Second)
	if _, _, err := p.read(off, s, 0); err == nil {
		t.Fatal("a read past the walk deadline succeeded")
	}
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
	got, err := applyDelta(base, delta, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello there!ld" {
		t.Fatalf("got %q", got)
	}
	// A delta that claims a different source size is refused.
	if _, err := applyDelta([]byte("x"), delta, nil); err == nil {
		t.Fatal("wrong source size must fail")
	}
	// A copy beyond the base is refused.
	bad := []byte{11, 3, 0x80 | 0x01 | 0x10, 10, 3}
	if _, err := applyDelta(base, bad, nil); err == nil {
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

func writeRefsOn(t *testing.T, gitDir, trunk string, f fixture) {
	t.Helper()
	setRef(t, gitDir, "refs/heads/"+trunk, f.m)
	setRef(t, gitDir, "refs/heads/"+mergedBranch, f.b)
	setRef(t, gitDir, "refs/heads/"+openBranch, f.d)
}

// A repo with only master (no main) uses master as the trunk.
func TestMasterTrunkWhenNoMain(t *testing.T) {
	repo, gd := newRepo(t)
	writeRefsOn(t, gd, "master", buildGraph(t, gd))
	c := NewChecker()
	expect(t, c, repo, mergedBranch, true, true)
	expect(t, c, repo, openBranch, false, true)
}

func TestMasterTrunkPacked(t *testing.T) {
	repo, gd := newRepo(t)
	f := buildGraph(t, gd)
	setPacked(t, gd, map[string]string{
		"refs/heads/master":          f.m,
		"refs/heads/" + mergedBranch: f.b,
		"refs/heads/" + openBranch:   f.d,
	})
	c := NewChecker()
	expect(t, c, repo, mergedBranch, true, true)
	expect(t, c, repo, openBranch, false, true)
}

// With both, main wins: master points at an unrelated commit that would give
// the opposite answer.
func TestMainPreferredOverMaster(t *testing.T) {
	repo, gd := newRepo(t)
	f := buildGraph(t, gd)
	writeRefs(t, gd, f)
	setRef(t, gd, "refs/heads/master", f.a)
	expect(t, NewChecker(), repo, mergedBranch, true, true)
}

func TestNeitherMainNorMasterIsUnknown(t *testing.T) {
	repo, gd := newRepo(t)
	writeRefsOn(t, gd, "trunk", buildGraph(t, gd))
	expect(t, NewChecker(), repo, mergedBranch, false, false)
}

// The trunk is never taken from HEAD, which may point at a work branch.
func TestTrunkIgnoresHead(t *testing.T) {
	repo, gd := newRepo(t)
	writeRefsOn(t, gd, "trunk", buildGraph(t, gd))
	if err := os.WriteFile(filepath.Join(gd, "HEAD"), []byte("ref: refs/heads/trunk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expect(t, NewChecker(), repo, mergedBranch, false, false)
}

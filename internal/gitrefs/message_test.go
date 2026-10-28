package gitrefs

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCommitMsg writes a loose commit with the given message and parents.
func writeCommitMsg(t *testing.T, gitDir, msg string, parents ...string) string {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("tree 4b825dc642cb6eb9a060e54bf8d69288fbee4904\n")
	for _, p := range parents {
		b.WriteString("parent " + p + "\n")
	}
	b.WriteString("author example <e@example> 0 +0000\ncommitter example <e@example> 0 +0000\n\n")
	b.WriteString(msg + "\n")
	body := b.Bytes()
	sha := objectSHA("commit", body)
	var raw bytes.Buffer
	fmt.Fprintf(&raw, "commit %d\x00", len(body))
	raw.Write(body)
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	zw.Write(raw.Bytes())
	zw.Close()
	p := looseObjectPath(gitDir, sha)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, z.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return sha
}

// goneRepo builds main = M, a no-ff merge of a branch whose ref no longer
// exists, with the given merge message. Returns the repo folder.
func goneRepo(t *testing.T, mergeMsg string) string {
	t.Helper()
	repo, gd := newRepo(t)
	a := writeCommitMsg(t, gd, "root")
	b := writeCommitMsg(t, gd, "work", a)
	c := writeCommitMsg(t, gd, "main work", a)
	m := writeCommitMsg(t, gd, mergeMsg, c, b)
	setRef(t, gd, "refs/heads/main", m)
	return repo
}

func expectMsg(t *testing.T, c *Checker, repo, branch string, wantFound, wantKnown bool) {
	t.Helper()
	f, k := c.MergedByMessage(repo, branch)
	if f != wantFound || k != wantKnown {
		t.Fatalf("MergedByMessage(%q) = (%v, %v), want (%v, %v)", branch, f, k, wantFound, wantKnown)
	}
}

func TestMergedByMessageNoFF(t *testing.T) {
	repo := goneRepo(t, "Merge branch 'sprint/crlf-writers-lf'")
	c := NewChecker()
	if !c.BranchGone(repo, "sprint/crlf-writers-lf") {
		t.Fatal("a branch with no ref must be gone")
	}
	expectMsg(t, c, repo, "sprint/crlf-writers-lf", true, true)
}

func TestMergedByMessageGitHubStyle(t *testing.T) {
	repo := goneRepo(t, "Merge pull request #79 from owner/work/wilco-de-tree/cron-check-post-deploy\n\nCheck the cron after deploy.")
	expectMsg(t, NewChecker(), repo, "work/wilco-de-tree/cron-check-post-deploy", true, true)
}

func TestMergedByMessagePrefixIsNotAMatch(t *testing.T) {
	repo := goneRepo(t, "Merge branch 'sprint/ab'")
	c := NewChecker()
	expectMsg(t, c, repo, "sprint/a", false, true)
	expectMsg(t, c, repo, "sprint/ab", true, true)
	// The name inside a longer word or longer branch does not count either.
	repo = goneRepo(t, "Merge branch 'xsprint/a' and 'sprint/a/b'")
	expectMsg(t, NewChecker(), repo, "sprint/a", false, true)
}

func TestMergedByMessageNeedsAMergeCommit(t *testing.T) {
	repo, gd := newRepo(t)
	a := writeCommitMsg(t, gd, "root")
	b := writeCommitMsg(t, gd, "mention sprint/example-gone in a plain commit", a)
	setRef(t, gd, "refs/heads/main", b)
	expectMsg(t, NewChecker(), repo, "sprint/example-gone", false, true)
}

// A merge that sits only on a second-parent line is not on the first-parent
// history, so it does not count.
func TestMergedByMessageFirstParentOnly(t *testing.T) {
	repo, gd := newRepo(t)
	a := writeCommitMsg(t, gd, "root")
	x := writeCommitMsg(t, gd, "side", a)
	inner := writeCommitMsg(t, gd, "Merge branch 'sprint/example-inner'", a, x)
	outer := writeCommitMsg(t, gd, "Merge branch 'sprint/example-outer'", a, inner)
	setRef(t, gd, "refs/heads/main", outer)
	c := NewChecker()
	expectMsg(t, c, repo, "sprint/example-outer", true, true)
	expectMsg(t, c, repo, "sprint/example-inner", false, true)
}

// mergeAtDepth builds a first-parent chain where the commit `depth` steps
// below main's tip is the merge naming the branch.
func mergeAtDepth(t *testing.T, depth int) string {
	t.Helper()
	repo, gd := newRepo(t)
	root := writeCommitMsg(t, gd, "root")
	side := writeCommitMsg(t, gd, "side", root)
	tip := writeCommitMsg(t, gd, "Merge branch 'sprint/example-deep'", root, side)
	for i := 0; i < depth; i++ {
		tip = writeCommitMsg(t, gd, fmt.Sprintf("commit %d", i), tip)
	}
	setRef(t, gd, "refs/heads/main", tip)
	return repo
}

func TestMergedByMessageSearchesFiveHundredCommits(t *testing.T) {
	// depth 499 is the 500th commit from the tip: found. depth 500 is the 501st.
	expectMsg(t, NewChecker(), mergeAtDepth(t, 499), "sprint/example-deep", true, true)
	expectMsg(t, NewChecker(), mergeAtDepth(t, 500), "sprint/example-deep", false, true)
}

func TestBranchGoneOnlyForAMissingRef(t *testing.T) {
	repo, gd := newRepo(t)
	a := writeCommitMsg(t, gd, "root")
	b := writeCommitMsg(t, gd, "work", a)
	setRef(t, gd, "refs/heads/main", a)
	setRef(t, gd, "refs/heads/sprint/example-open", b)
	setPacked(t, gd, map[string]string{"refs/heads/sprint/example-packed": b})
	c := NewChecker()
	for _, br := range []string{"sprint/example-open", "sprint/example-packed"} {
		if c.BranchGone(repo, br) {
			t.Fatalf("%s has a ref, it is not gone", br)
		}
	}
	if !c.BranchGone(repo, "sprint/example-missing") {
		t.Fatal("a name in neither loose nor packed refs is gone")
	}
	// A folder where the ref file would be is a read problem, not a deleted branch.
	if c.BranchGone(repo, "sprint") {
		t.Fatal("a read error must not read as gone")
	}
	// No trunk, no verdict; an unsafe name, no verdict.
	repo2, gd2 := newRepo(t)
	_ = gd2
	if c.BranchGone(repo2, "sprint/example-missing") {
		t.Fatal("without a trunk the repo cannot say a branch is gone")
	}
	if c.BranchGone(repo, "-x") || c.BranchGone(repo, "a b") {
		t.Fatal("an unsafe branch name must not read as gone")
	}
	// A ref that exists but is unmerged is not searched by message.
	if f, k := c.MergedByMessage(repo, "sprint/example-open"); f || k {
		t.Fatalf("MergedByMessage on an existing ref = (%v, %v), want (false, false)", f, k)
	}
}

func TestMergedByMessageUnreadableIsUnknown(t *testing.T) {
	repo, gd := newRepo(t)
	setRef(t, gd, "refs/heads/main", strings.Repeat("a", 40))
	expectMsg(t, NewChecker(), repo, "sprint/example-gone", false, false)
}

func TestMergedByMessageIsCached(t *testing.T) {
	repo := goneRepo(t, "Merge branch 'sprint/example-cached'")
	c := NewChecker()
	expectMsg(t, c, repo, "sprint/example-cached", true, true)
	gd := filepath.Join(repo, ".git")
	if err := os.RemoveAll(filepath.Join(gd, "objects")); err != nil {
		t.Fatal(err)
	}
	expectMsg(t, c, repo, "sprint/example-cached", true, true)
}

func TestMergedOrGone(t *testing.T) {
	// Branch ref gone, merge message found: merged.
	repo := goneRepo(t, "Merge branch 'sprint/example-gone'")
	if m, k := NewChecker().MergedOrGone(repo, "sprint/example-gone"); !m || !k {
		t.Fatalf("gone and named = (%v, %v), want (true, true)", m, k)
	}
	// Branch ref gone, no merge message: known, not merged.
	if m, k := NewChecker().MergedOrGone(repo, "sprint/example-other"); m || !k {
		t.Fatalf("gone and unnamed = (%v, %v), want (false, true)", m, k)
	}
	// A ref that exists answers as Merged does.
	r2, gd := newRepo(t)
	f := buildGraph(t, gd)
	writeRefs(t, gd, f)
	if m, k := NewChecker().MergedOrGone(r2, mergedBranch); !m || !k {
		t.Fatalf("merged ref = (%v, %v), want (true, true)", m, k)
	}
	if m, k := NewChecker().MergedOrGone(r2, openBranch); m || !k {
		t.Fatalf("open ref = (%v, %v), want (false, true)", m, k)
	}
}

package centre

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// TestFindReposWorktreeGitdirAndStrayGit builds a root by hand, with no git
// binary: a repo, a linked worktree whose .git file holds an absolute gitdir
// line, and a stray .git folder holding only info\exclude. That stray is what
// a shell writes when it eats the backslashes of C:\ZND\10_holding\.git\info\exclude:
// Windows reads C:ZND10_holding as relative to the current folder. Git does not
// treat a folder without HEAD as a repo, so the scanner skips it too.
func TestFindReposWorktreeGitdirAndStrayGit(t *testing.T) {
	root := t.TempDir()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	write := func(p, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	repo := filepath.Join(root, "10_holding")
	wt := filepath.Join(root, "10_holding-wt")
	gd := filepath.Join(repo, ".git")
	wgd := filepath.Join(gd, "worktrees", "10_holding-wt")
	write(filepath.Join(gd, "HEAD"), "ref: refs/heads/main\n")
	write(filepath.Join(gd, "refs", "heads", "main"), strings.Repeat("a", 40)+"\n")
	write(filepath.Join(gd, "config"), "[core]\n")
	write(filepath.Join(wgd, "HEAD"), "ref: refs/heads/wt\n")
	write(filepath.Join(wgd, "commondir"), "../..\n")
	write(filepath.Join(wgd, "gitdir"), filepath.Join(wt, ".git")+"\n")
	write(filepath.Join(wt, ".git"), "gitdir: "+wgd+"\n")
	write(filepath.Join(repo, "ZND10_holding", ".git", "info", "exclude"), "\r\n.worktrees/\r\n")

	if runtime.GOOS == "windows" {
		data, _ := os.ReadFile(filepath.Join(wt, ".git"))
		if !strings.Contains(string(data), `:\`) {
			t.Fatalf("fixture gitdir line is not a Windows path: %q", data)
		}
	}

	var ids []string
	for _, d := range FindRepos(root) {
		ids = append(ids, d.ID)
	}
	if want := []string{"10_holding"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("FindRepos = %v, want %v (no worktree, no stray .git)", ids, want)
	}

	rf, err := ReadRepoFiles(repo)
	if err != nil {
		t.Fatal(err)
	}
	if rf.Branch != "main" {
		t.Errorf("branch = %q, want main", rf.Branch)
	}
	if len(rf.Worktrees) != 2 {
		t.Fatalf("worktrees = %+v, want main plus one linked", rf.Worktrees)
	}
	got := rf.Worktrees[1]
	if got.Path != wt || got.Name != "10_holding-wt" || got.Branch != "wt" || got.Missing {
		t.Errorf("linked worktree = %+v, want path %q, name 10_holding-wt, branch wt, present", got, wt)
	}

	// The worktree read on its own resolves its common git dir.
	wrf, err := ReadRepoFiles(wt)
	if err != nil {
		t.Fatal(err)
	}
	if wrf.GitDir != gd || wrf.Branch != "wt" {
		t.Errorf("worktree read: gitdir %q branch %q, want %q and wt", wrf.GitDir, wrf.Branch, gd)
	}
}

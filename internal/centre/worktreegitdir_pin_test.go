package centre

import (
	"os"
	"path/filepath"
	"testing"
)

// worktreeGitDir gives "" for anything it cannot read, and otherwise the
// per-worktree git dir (it does not follow commondir).
func TestWorktreeGitDirShapes(t *testing.T) {
	file := func(t *testing.T, p, s string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("normal repo", func(t *testing.T) {
		repo := t.TempDir()
		dot := filepath.Join(repo, ".git")
		os.Mkdir(dot, 0o755)
		if got := worktreeGitDir(repo); got != dot {
			t.Errorf("got %q, want %q", got, dot)
		}
	})
	t.Run("linked worktree", func(t *testing.T) {
		wt, gd := t.TempDir(), t.TempDir()
		file(t, filepath.Join(wt, ".git"), "gitdir: "+gd+"\n")
		if got := worktreeGitDir(wt); got != filepath.Clean(gd) {
			t.Errorf("got %q, want %q", got, gd)
		}
	})
	t.Run("relative gitdir", func(t *testing.T) {
		wt := t.TempDir()
		file(t, filepath.Join(wt, ".git"), "gitdir: real\n")
		if got, want := worktreeGitDir(wt), filepath.Join(wt, "real"); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("no gitdir prefix", func(t *testing.T) {
		wt := t.TempDir()
		file(t, filepath.Join(wt, ".git"), "somewhere\n")
		if got := worktreeGitDir(wt); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
	t.Run("empty gitdir", func(t *testing.T) {
		wt := t.TempDir()
		file(t, filepath.Join(wt, ".git"), "gitdir:\n")
		if got := worktreeGitDir(wt); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
	t.Run("no .git", func(t *testing.T) {
		if got := worktreeGitDir(t.TempDir()); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

// readRemotes lists every remote with its first url, in file order.
func TestReadRemotesTable(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config")
	body := "[core]\n\turl = not-a-remote\n[remote \"upstream\"]\n\turl = https://example.com/a.git\n\turl = https://example.com/second.git\n[remote \"origin\"]\n\tURL = git@example.com:b/c.git\n[remote \"empty\"]\n"
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := readRemotes(cfg)
	want := []GitRemote{{"upstream", "https://example.com/a.git"}, {"origin", "git@example.com:b/c.git"}, {"empty", ""}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("remote %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if r := readRemotes(filepath.Join(t.TempDir(), "missing")); r == nil || len(r) != 0 {
		t.Errorf("missing file: got %#v, want an empty non-nil list", r)
	}
}

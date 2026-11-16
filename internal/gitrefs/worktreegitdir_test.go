package gitrefs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkdir(t *testing.T, p string) string {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// WorktreeGitDir is the per-worktree git dir: the .git folder, or the
// gitdir line of a .git file. It does not follow commondir.
func TestWorktreeGitDir(t *testing.T) {
	t.Run("normal repo", func(t *testing.T) {
		repo := t.TempDir()
		dot := mkdir(t, filepath.Join(repo, ".git"))
		got, err := WorktreeGitDir(repo)
		if err != nil || got != dot {
			t.Errorf("got %q, %v; want %q", got, err, dot)
		}
	})
	t.Run("linked worktree, absolute gitdir", func(t *testing.T) {
		main, wt := t.TempDir(), t.TempDir()
		gd := mkdir(t, filepath.Join(main, ".git", "worktrees", "w"))
		write(t, filepath.Join(wt, ".git"), "gitdir: "+gd+"\n")
		got, err := WorktreeGitDir(wt)
		if err != nil || got != gd {
			t.Errorf("got %q, %v; want %q", got, err, gd)
		}
	})
	t.Run("relative gitdir is taken from the folder", func(t *testing.T) {
		wt := t.TempDir()
		mkdir(t, filepath.Join(wt, "real"))
		write(t, filepath.Join(wt, ".git"), "gitdir: real\n")
		got, err := WorktreeGitDir(wt)
		if want := filepath.Join(wt, "real"); err != nil || got != want {
			t.Errorf("got %q, %v; want %q", got, err, want)
		}
	})
	t.Run("file without the gitdir prefix", func(t *testing.T) {
		wt := t.TempDir()
		write(t, filepath.Join(wt, ".git"), "somewhere\n")
		if got, err := WorktreeGitDir(wt); err == nil || !strings.Contains(err.Error(), "no gitdir line") {
			t.Errorf("got %q, %v; want the no gitdir line error", got, err)
		}
	})
	t.Run("empty gitdir", func(t *testing.T) {
		wt := t.TempDir()
		write(t, filepath.Join(wt, ".git"), "gitdir:   \n")
		if _, err := WorktreeGitDir(wt); err == nil || !strings.Contains(err.Error(), "empty gitdir") {
			t.Errorf("err = %v, want the empty gitdir error", err)
		}
	})
	t.Run("no .git", func(t *testing.T) {
		if _, err := WorktreeGitDir(t.TempDir()); err == nil {
			t.Error("want an error")
		}
	})
}

// GitDir keeps its results for every shape of .git, now that it builds on
// WorktreeGitDir.
func TestGitDirShapes(t *testing.T) {
	t.Run("normal repo", func(t *testing.T) {
		repo := t.TempDir()
		dot := mkdir(t, filepath.Join(repo, ".git"))
		if got, err := GitDir(repo); err != nil || got != dot {
			t.Errorf("got %q, %v; want %q", got, err, dot)
		}
	})
	t.Run("linked worktree follows commondir", func(t *testing.T) {
		main, wt := t.TempDir(), t.TempDir()
		common := mkdir(t, filepath.Join(main, ".git"))
		gd := mkdir(t, filepath.Join(common, "worktrees", "w"))
		write(t, filepath.Join(gd, "commondir"), "../..\n")
		write(t, filepath.Join(wt, ".git"), "gitdir: "+gd+"\n")
		if got, err := GitDir(wt); err != nil || got != common {
			t.Errorf("got %q, %v; want %q", got, err, common)
		}
	})
	t.Run("relative gitdir", func(t *testing.T) {
		wt := t.TempDir()
		real := mkdir(t, filepath.Join(wt, "real"))
		write(t, filepath.Join(wt, ".git"), "gitdir: real\n")
		if got, err := GitDir(wt); err != nil || got != real {
			t.Errorf("got %q, %v; want %q", got, err, real)
		}
	})
	t.Run("no gitdir prefix", func(t *testing.T) {
		wt := t.TempDir()
		write(t, filepath.Join(wt, ".git"), "somewhere\n")
		if _, err := GitDir(wt); err == nil || err.Error() != "gitrefs: .git file has no gitdir line" {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("gitdir folder missing", func(t *testing.T) {
		wt := t.TempDir()
		write(t, filepath.Join(wt, ".git"), "gitdir: nowhere\n")
		if _, err := GitDir(wt); err == nil || err.Error() != "gitrefs: git dir not found" {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("empty commondir", func(t *testing.T) {
		wt := t.TempDir()
		gd := mkdir(t, filepath.Join(wt, "real"))
		write(t, filepath.Join(gd, "commondir"), "\n")
		write(t, filepath.Join(wt, ".git"), "gitdir: real\n")
		if _, err := GitDir(wt); err == nil || err.Error() != "gitrefs: empty commondir" {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("no .git", func(t *testing.T) {
		if _, err := GitDir(t.TempDir()); err == nil {
			t.Error("want an error")
		}
	})
}

// Remotes lists the [remote "name"] url lines of a git config, in file order.
func TestRemotes(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config")
	write(t, cfg, strings.Join([]string{
		"[core]",
		"\turl = not-a-remote",
		`[remote "upstream"]`,
		"\turl = https://example.com/a.git",
		"\turl = https://example.com/second-url-ignored.git",
		`[remote "origin"]`,
		"\tfetch = +refs/heads/*:refs/remotes/origin/*",
		"\tURL = git@example.com:b/c.git",
		`[remote "empty"]`,
		"",
	}, "\n"))
	got := Remotes(cfg)
	want := []Remote{
		{Name: "upstream", URL: "https://example.com/a.git"},
		{Name: "origin", URL: "git@example.com:b/c.git"},
		{Name: "empty"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("remote %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if r := Remotes(filepath.Join(t.TempDir(), "missing")); r == nil || len(r) != 0 {
		t.Errorf("missing file: got %#v, want an empty non-nil list", r)
	}
}

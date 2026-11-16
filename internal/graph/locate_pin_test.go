package graph

import (
	"os"
	"path/filepath"
	"testing"
)

// locate stays its own reader: these pin what it returns today, including
// the result for a .git file without the gitdir prefix, which a shared
// reader would turn into an error.
func TestLocateShapes(t *testing.T) {
	put := func(t *testing.T, p, s string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("normal repo", func(t *testing.T) {
		repo := t.TempDir()
		put(t, filepath.Join(repo, ".git", "HEAD"), "ref: refs/heads/main\n")
		sub := filepath.Join(repo, "a", "b")
		os.MkdirAll(sub, 0o755)
		r, ok := locate(sub)
		if !ok || r.Root != repo || r.Main != "" || r.Branch != "main" {
			t.Errorf("got %+v, %v", r, ok)
		}
	})
	t.Run("linked worktree", func(t *testing.T) {
		main, wt := t.TempDir(), t.TempDir()
		gd := filepath.Join(main, ".git", "worktrees", "w")
		put(t, filepath.Join(gd, "HEAD"), "ref: refs/heads/feature\n")
		put(t, filepath.Join(wt, ".git"), "gitdir: "+gd+"\n")
		r, ok := locate(wt)
		if !ok || r.Root != wt || r.Main != main || r.Branch != "feature" {
			t.Errorf("got %+v, %v", r, ok)
		}
	})
	t.Run("relative gitdir", func(t *testing.T) {
		wt := t.TempDir()
		put(t, filepath.Join(wt, "real", "HEAD"), "ref: refs/heads/rel\n")
		put(t, filepath.Join(wt, ".git"), "gitdir: real\n")
		r, ok := locate(wt)
		if !ok || r.Root != wt || r.Main != "" || r.Branch != "rel" {
			t.Errorf("got %+v, %v", r, ok)
		}
	})
	t.Run("file without the gitdir prefix still counts as a repo", func(t *testing.T) {
		wt := t.TempDir()
		put(t, filepath.Join(wt, ".git"), "somewhere\n")
		r, ok := locate(wt)
		if !ok || r.Root != wt || r.Branch != "" {
			t.Errorf("got %+v, %v", r, ok)
		}
	})
	t.Run("no .git", func(t *testing.T) {
		if r, ok := locate(t.TempDir()); ok {
			// The temp folder sits under no repo, so no match is expected.
			t.Errorf("got %+v, %v", r, ok)
		}
	})
}

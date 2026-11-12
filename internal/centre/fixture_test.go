package centre

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fixtureTree copies testdata/tree into a fresh temp folder and returns its
// path. Line endings become LF so sizes match on every OS, every mtime is
// set to 2026-11-01T00:00:00Z (projects/kettle/CHANGELOG.md to
// 2026-08-01T00:00:00Z), and folders the walk must skip get a unit file
// each. Those folders are made here because .git cannot be committed.
// Unit files are checked in as siteoffice.fixture.json and renamed to
// siteoffice.json in the copy: git_flow treats siteoffice.json as a policy
// file that only a person commits, and real unit files in the repo would
// look like nested units to Siteoffice tools.
func fixtureTree(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	src := filepath.Join("testdata", "tree")
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.Name() == "siteoffice.fixture.json" {
			target = filepath.Join(filepath.Dir(target), unitFileName)
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy fixture tree: %v", err)
	}
	skipped := []string{".git", ".worktrees", "node_modules", "_archive", "__pycache__", ".venv", "graphify-out", filepath.Join("_board", "x"), "_log", ".hidden"}
	for _, folder := range skipped {
		dir := filepath.Join(dst, folder)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("make %s: %v", folder, err)
		}
		body := []byte(`{"name":"skipped-` + filepath.Base(folder) + `"}` + "\n")
		if err := os.WriteFile(filepath.Join(dir, "siteoffice.json"), body, 0o644); err != nil {
			t.Fatalf("write unit file in %s: %v", folder, err)
		}
	}
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	var dirs []string
	err = filepath.WalkDir(dst, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			dirs = append(dirs, p)
			return nil
		}
		return os.Chtimes(p, base, base)
	})
	if err != nil {
		t.Fatalf("set mtimes: %v", err)
	}
	// Folders last, deepest first, so writing files does not move them again.
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Chtimes(dirs[i], base, base); err != nil {
			t.Fatalf("set folder mtime: %v", err)
		}
	}
	old := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(dst, "projects", "kettle", "CHANGELOG.md"), old, old); err != nil {
		t.Fatalf("date CHANGELOG.md: %v", err)
	}
	return dst
}

// writeFile writes body to root/rel (forward slashes), making folders.
func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFixtureTreeHelper(t *testing.T) {
	root := fixtureTree(t)
	info, err := os.Stat(filepath.Join(root, "projects", "kettle", "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("CHANGELOG.md mtime %v", info.ModTime().UTC())
	}
	info, err = os.Stat(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("AGENTS.md mtime %v", info.ModTime().UTC())
	}
	if info.Size() <= 200 {
		t.Errorf("AGENTS.md is %d bytes, want over 200", info.Size())
	}
	info, err = os.Stat(filepath.Join(root, "STATUS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() >= 200 {
		t.Errorf("STATUS.md is %d bytes, want under 200", info.Size())
	}
	data, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("\r")) {
		t.Error("AGENTS.md still holds a carriage return")
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "siteoffice.json")); err != nil {
		t.Errorf(".git unit file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "projects", "kettle", "siteoffice.json")); err != nil {
		t.Errorf("kettle unit file not renamed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "projects", "kettle", "siteoffice.fixture.json")); err == nil {
		t.Error("siteoffice.fixture.json left in the copy")
	}
}

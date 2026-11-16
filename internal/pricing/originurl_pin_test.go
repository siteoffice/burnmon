package pricing

import (
	"os"
	"path/filepath"
	"testing"
)

// readOriginURL stays its own reader. These pin what it returns today, the
// two cases (upper-case section header, key written URL) where a reader that
// lists every remote would give another answer.
func TestReadOriginURLTable(t *testing.T) {
	cases := []struct {
		name, config, want string
		ok                 bool
	}{
		{"origin first", "[remote \"origin\"]\n\turl = https://example.com/a.git\n[remote \"other\"]\n\turl = https://example.com/b.git\n", "https://example.com/a.git", true},
		{"origin second", "[remote \"other\"]\n\turl = https://example.com/b.git\n[remote \"origin\"]\n\turl = https://example.com/a.git\n", "https://example.com/a.git", true},
		{"upper-case section header matches", "[REMOTE \"ORIGIN\"]\n\turl = https://example.com/a.git\n", "https://example.com/a.git", true},
		{"key written URL does not match", "[remote \"origin\"]\n\tURL = https://example.com/a.git\n", "", false},
		{"first url wins", "[remote \"origin\"]\n\turl = https://example.com/a.git\n\turl = https://example.com/second.git\n", "https://example.com/a.git", true},
		{"no origin", "[remote \"other\"]\n\turl = https://example.com/b.git\n", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config")
			if err := os.WriteFile(p, []byte(c.config), 0o644); err != nil {
				t.Fatal(err)
			}
			got, ok := readOriginURL(p)
			if got != c.want || ok != c.ok {
				t.Errorf("readOriginURL = %q, %v; want %q, %v", got, ok, c.want, c.ok)
			}
		})
	}
}

// gitRemoteURL on a .git file: relative gitdir, and no gitdir prefix.
func TestGitRemoteURLGitFileShapes(t *testing.T) {
	t.Run("relative gitdir", func(t *testing.T) {
		wt := t.TempDir()
		if err := os.MkdirAll(filepath.Join(wt, "real"), 0o755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(wt, "real", "config"), []byte("[remote \"origin\"]\n\turl = https://example.com/rel.git\n"), 0o644)
		os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: real\n"), 0o644)
		if got, ok := gitRemoteURL(wt); !ok || got != "https://example.com/rel.git" {
			t.Errorf("got %q, %v", got, ok)
		}
	})
	t.Run("no gitdir prefix", func(t *testing.T) {
		wt := t.TempDir()
		os.WriteFile(filepath.Join(wt, ".git"), []byte("somewhere\n"), 0o644)
		if got, ok := gitRemoteURL(wt); ok {
			t.Errorf("got %q, %v; want no remote", got, ok)
		}
	})
}

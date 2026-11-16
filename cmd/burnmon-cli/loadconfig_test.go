package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// loadConfig looks in the current folder after the exe's folder, and prints
// the relative name it found.
func TestLoadConfigCurrentFolderOrder(t *testing.T) {
	cases := []struct {
		name        string
		files       map[string]string // file name -> foreman_root
		wantRoot    string
		wantPrinted string
	}{
		{"burnmon.json", map[string]string{"burnmon.json": "new"}, "new", "Using config overrides from burnmon.json\n"},
		{"claudecost.json only", map[string]string{"claudecost.json": "old"}, "old", "Using config overrides from claudecost.json\n"},
		{"both: burnmon.json wins", map[string]string{"burnmon.json": "new", "claudecost.json": "old"}, "new", "Using config overrides from burnmon.json\n"},
		{"none", nil, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cwd := t.TempDir()
			for name, root := range c.files {
				if err := os.WriteFile(filepath.Join(cwd, name), []byte(`{"foreman_root":"`+root+`"}`), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			old, _ := os.Getwd()
			if err := os.Chdir(cwd); err != nil {
				t.Fatal(err)
			}
			defer os.Chdir(old)
			r, w, _ := os.Pipe()
			stdout := os.Stdout
			os.Stdout = w
			cfg, err := loadConfig("", false)
			os.Stdout = stdout
			w.Close()
			printed, _ := io.ReadAll(r)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.ForemanRoot != c.wantRoot {
				t.Errorf("ForemanRoot = %q, want %q", cfg.ForemanRoot, c.wantRoot)
			}
			if string(printed) != c.wantPrinted {
				t.Errorf("printed %q, want %q", printed, c.wantPrinted)
			}
		})
	}
}

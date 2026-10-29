//go:build windows

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BurnMon Dev writes the snapshot itself, so "is not running" is never true
// while it runs (B7.3). No file of this package may say it. Test files are
// skipped: they hold the phrase only to prove it is absent.
func TestNoIsNotRunning(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(path, "_test.go") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.Contains(strings.ToLower(line), "is not running") {
				t.Errorf("%s:%d says %q", path, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

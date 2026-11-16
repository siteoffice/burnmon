package pricing

import (
	"os"
	"path/filepath"
)

// configNames are the config file names in the order they are tried in one
// folder. claudecost.json is the name before the rename (v0.1.2 and earlier).
var configNames = []string{"burnmon.json", "claudecost.json"}

// FindConfig returns the config file to load, "" when there is none: burnmon.json
// then claudecost.json next to the exe (skipped when exeDir is empty), then the
// same two in fallbackDir. An empty fallbackDir gives relative names, which is
// how the CLI looks in the current folder. A path counts when os.Stat finds it.
func FindConfig(exeDir, fallbackDir string) string {
	return FindConfigFunc(exeDir, fallbackDir, func(p string) bool {
		_, err := os.Stat(p)
		return err == nil
	})
}

// FindConfigFunc is FindConfig with the caller's own test for "this path is a
// usable file".
func FindConfigFunc(exeDir, fallbackDir string, usable func(path string) bool) string {
	dirs := []string{fallbackDir}
	if exeDir != "" {
		dirs = []string{exeDir, fallbackDir}
	}
	for _, d := range dirs {
		for _, n := range configNames {
			if p := filepath.Join(d, n); usable(p) {
				return p
			}
		}
	}
	return ""
}

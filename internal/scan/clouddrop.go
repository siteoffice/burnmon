package scan

import (
	"os"
	"path/filepath"
	"strings"
)

// cloudDropDir returns the folder the BurnMon cloud plugin delivers its
// usage extracts to: .burnmon\cloud next to the running exe. A var so tests
// can point it at a temporary folder.
var cloudDropDir = func() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), ".burnmon", "cloud")
}

// homeCloudDropDir returns ~/.burnmon/cloud, where the universal BurnMon
// cloud plugin copies to on every computer (its target is "~/.burnmon/cloud/
// latest.jsonl"; the copy tool expands "~", measured 2026-10-13). A var so
// tests can point it at a temporary folder.
var homeCloudDropDir = func() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".burnmon", "cloud")
}

// CloudDropDirs lists every drop folder: the exe-adjacent one (plugins built
// for one computer) and ~/.burnmon/cloud (the universal plugin), each once.
func CloudDropDirs() []string {
	var out []string
	for _, d := range []string{cloudDropDir(), homeCloudDropDir()} {
		if d == "" {
			continue
		}
		dup := false
		for _, o := range out {
			if strings.EqualFold(filepath.Clean(o), filepath.Clean(d)) {
				dup = true
			}
		}
		if !dup {
			out = append(out, d)
		}
	}
	return out
}

// EnsureCloudDropDir creates every drop folder that is missing, so the live
// watcher can watch them from startup, the user can connect ~/.burnmon to
// cloud sessions, and the plugin's copy has a folder to land in. The apps
// call it once at startup; a failure only means drop files are picked up by
// the next full scan instead of live.
func EnsureCloudDropDir() error {
	var first error
	for _, d := range CloudDropDirs() {
		if err := os.MkdirAll(d, 0o755); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// IsCloudDropPath reports whether path is a Cowork (Cloud) drop file: a
// .jsonl directly inside a "cloud" folder whose parent is ".burnmon". The
// BurnMon cloud plugin writes these from inside cloud Cowork sessions, one
// usage-only line per model request
// (02_roadmap\2026-10-12_cowork_cloud_bucket_spec.md). Both separators are
// accepted, so a Windows path classifies the same way on every OS.
func IsCloudDropPath(path string) bool {
	segs := strings.Split(strings.ToLower(strings.ReplaceAll(path, `\`, "/")), "/")
	n := len(segs)
	if n < 3 || !strings.HasSuffix(segs[n-1], ".jsonl") {
		return false
	}
	return segs[n-2] == "cloud" && segs[n-3] == ".burnmon"
}

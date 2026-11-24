package scan

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
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

// CloudLastMeasured returns the newest row time over every .jsonl in the given
// drop folders: the last moment BurnMon measured any Cowork (Cloud) usage. The
// vendor strip uses it to show "not measured" instead of 0 after the plugin
// stops delivering. false when no folder holds a row with a valid time. Only
// rows from the BurnMon cloud plugin count, as in the adapter's drop reader.
func CloudLastMeasured(dirs []string) (time.Time, bool) {
	var newest time.Time
	for _, d := range dirs {
		files, err := filepath.Glob(filepath.Join(d, "*.jsonl"))
		if err != nil {
			continue
		}
		for _, p := range files {
			if t, ok := newestRowTime(p); ok && t.After(newest) {
				newest = t
			}
		}
	}
	return newest, !newest.IsZero()
}

// CloudModNewest returns the newest modification time over every mod-*.jsonl
// in the given drop folders: when the burnmon-cloud mod last delivered a file
// from a Cowork (Cloud) session. false when no folder holds one, which today
// means mods do not run in cloud sessions yet (decision 3 of
// 02_roadmap\2026-11-24_cloud_mod_route_plan.md).
func CloudModNewest(dirs []string) (time.Time, bool) {
	var newest time.Time
	for _, d := range dirs {
		files, err := filepath.Glob(filepath.Join(d, "mod-*.jsonl"))
		if err != nil {
			continue
		}
		for _, p := range files {
			if fi, err := os.Stat(p); err == nil && fi.ModTime().After(newest) {
				newest = fi.ModTime().UTC()
			}
		}
	}
	return newest, !newest.IsZero()
}

func newestRowTime(path string) (time.Time, bool) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, false
	}
	defer f.Close()
	var newest time.Time
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var r struct {
			Source string `json:"source"`
			TS     string `json:"ts"`
		}
		if json.Unmarshal(sc.Bytes(), &r) != nil || !strings.HasPrefix(r.Source, "burnmon-cloud") {
			continue
		}
		if t, err := time.Parse(time.RFC3339Nano, r.TS); err == nil && t.After(newest) {
			newest = t.UTC()
		}
	}
	return newest, !newest.IsZero()
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

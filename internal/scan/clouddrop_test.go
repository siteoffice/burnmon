package scan

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIsCloudDropPath pins what counts as a Cowork (Cloud) drop file: a
// .jsonl directly inside a "cloud" folder whose parent is ".burnmon". The
// BurnMon cloud plugin delivers those from cloud Cowork sessions
// (02_roadmap\2026-10-12_cowork_cloud_bucket_spec.md).
func TestIsCloudDropPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{`C:\ZND\50_projects\burnmon\.burnmon\cloud\latest.jsonl`, true},
		{`C:\ZND\50_projects\burnmon\.burnmon\cloud\551731f0-5334.jsonl`, true},
		{"/home/u/.burnmon/cloud/latest.jsonl", true},
		{`C:\X\.BURNMON\Cloud\latest.jsonl`, true},
		{`C:\ZND\.burnmon\cloud\sub\latest.jsonl`, false},
		{`C:\ZND\burnmon\cloud\latest.jsonl`, false},
		{`C:\ZND\.burnmon\cloud\latest.json`, false},
		{`C:\Users\u\.claude\projects\p\s.jsonl`, false},
	}
	for _, c := range cases {
		if got := IsCloudDropPath(c.path); got != c.want {
			t.Errorf("IsCloudDropPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// TestDefaultSourcesIncludesCloudDropDir: the exe-adjacent .burnmon\cloud
// folder joins the Claude sources when it exists (it is never created), so the full scan, the
// adapter roots and the live watcher all pick up drop files with no extra
// wiring in cmd.
func TestDefaultSourcesIncludesCloudDropDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".burnmon", "cloud")
	old, oldHome := cloudDropDir, homeCloudDropDir
	cloudDropDir = func() string { return dir }
	homeCloudDropDir = func() string { return "" }
	defer func() { cloudDropDir, homeCloudDropDir = old, oldHome }()

	has := func(list []string) bool {
		for _, s := range list {
			if s == dir {
				return true
			}
		}
		return false
	}
	if has(DefaultSourcesWithOptions(false)) {
		t.Fatal("drop dir listed before it exists")
	}
	// Looking for sources never creates the folder: the plugin is retired, so
	// BurnMon only reads a folder that already exists (2026-11-17).
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("listing sources created %s (err %v)", dir, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if !has(DefaultSourcesWithOptions(false)) {
		t.Fatal("drop dir missing from DefaultSourcesWithOptions after it exists")
	}
	files := FindJSONL([]string{dir})
	if len(files) != 0 {
		t.Fatalf("empty drop dir listed %d files", len(files))
	}
	p := filepath.Join(dir, "latest.jsonl")
	if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if files := FindJSONL([]string{dir}); len(files) != 1 || files[0] != p {
		t.Fatalf("FindJSONL = %v, want [%s]", files, p)
	}
}

// TestHomeCloudDropDir: the retired BurnMon cloud plugin copied to
// ~/.burnmon/cloud, the same path on every computer (the copy tool expands
// "~", measured 2026-10-13). That folder is a second drop folder: listed as a
// source next to the exe-adjacent one when it exists. One folder serving both
// roles is listed once.
func TestHomeCloudDropDir(t *testing.T) {
	root := t.TempDir()
	exeDir := filepath.Join(root, "app", ".burnmon", "cloud")
	homeDir := filepath.Join(root, "home", ".burnmon", "cloud")
	old, oldHome := cloudDropDir, homeCloudDropDir
	cloudDropDir = func() string { return exeDir }
	homeCloudDropDir = func() string { return homeDir }
	defer func() { cloudDropDir, homeCloudDropDir = old, oldHome }()

	if got := CloudDropDirs(); len(got) != 2 || got[0] != exeDir || got[1] != homeDir {
		t.Fatalf("CloudDropDirs = %v, want [%s %s]", got, exeDir, homeDir)
	}
	for _, d := range []string{exeDir, homeDir} {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Fatalf("listing the drop folders created %s (err %v)", d, err)
		}
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	count := func(d string) int {
		n := 0
		for _, s := range DefaultSourcesWithOptions(false) {
			if s == d {
				n++
			}
		}
		return n
	}
	if count(exeDir) != 1 || count(homeDir) != 1 {
		t.Fatalf("sources list exe dir %d times and home dir %d times, want 1 and 1", count(exeDir), count(homeDir))
	}

	// The exe lives in the home folder: one folder, listed once.
	homeCloudDropDir = func() string { return exeDir }
	if got := CloudDropDirs(); len(got) != 1 {
		t.Fatalf("CloudDropDirs = %v, want one entry", got)
	}
	if count(exeDir) != 1 {
		t.Fatalf("shared folder listed %d times, want 1", count(exeDir))
	}
}

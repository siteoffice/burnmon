package centre

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckRoot: a root must be an existing, readable folder; the check
// counts its units before Save.
func TestCheckRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "siteoffice.json", `{"name":"r"}`)
	writeFile(t, root, "a/siteoffice.json", `{"name":"a"}`)
	writeFile(t, root, "a/b/siteoffice.json", `{"name":"b"}`)
	c := CheckRoot(root, "")
	if !c.OK || c.Units != 3 || c.Path != root || c.Line != "3 units below this folder." {
		t.Errorf("CheckRoot(root) = %+v", c)
	}
	// relative to cwd
	if c := CheckRoot("a", root); !c.OK || c.Units != 2 || c.Path != filepath.Join(root, "a") {
		t.Errorf("CheckRoot(relative) = %+v", c)
	}
	if c := CheckRoot("  ", root); c.OK || c.Line != "Type a folder or pick one with Browse." {
		t.Errorf("CheckRoot(blank) = %+v", c)
	}
	if c := CheckRoot(filepath.Join(root, "nope"), ""); c.OK || !strings.HasPrefix(c.Line, "No folder at ") {
		t.Errorf("CheckRoot(missing) = %+v", c)
	}
	if c := CheckRoot(filepath.Join(root, "siteoffice.json"), ""); c.OK || !strings.HasSuffix(c.Line, "is a file, not a folder.") {
		t.Errorf("CheckRoot(file) = %+v", c)
	}
	empty := t.TempDir()
	if c := CheckRoot(empty, ""); !c.OK || c.Units != 0 || c.Line != "No units below this folder: no siteoffice.json in it or 4 levels down." {
		t.Errorf("CheckRoot(empty) = %+v", c)
	}
	one := t.TempDir()
	writeFile(t, one, "siteoffice.json", `{}`)
	if c := CheckRoot(one, ""); c.Line != "1 unit below this folder." {
		t.Errorf("CheckRoot(one) line = %q", c.Line)
	}
}

func TestSettingsGitPRsOffByDefault(t *testing.T) {
	if DefaultSettings().GitPRs {
		t.Error("the PR switch must be off by default")
	}
	dir := t.TempDir()
	s := DefaultSettings()
	s.GitPRs = true
	s.Root = `C:\some\root`
	if err := SaveSettings(filepath.Join(dir, SettingsFile), s); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, SettingsFile))
	if !strings.Contains(string(data), `"git_prs": true`) {
		t.Errorf("file lacks git_prs: %s", data)
	}
	got, _ := LoadSettings("", dir)
	if !got.GitPRs || got.Root != s.Root {
		t.Errorf("loaded %+v", got)
	}
}

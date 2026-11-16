package pricing

import (
	"os"
	"path/filepath"
	"testing"
)

// touch makes an empty JSON file, or a folder when asDir is set.
func touch(t *testing.T, dir, name string, asDir bool) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if asDir {
		if err := os.Mkdir(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestFindConfigOrder pins the lookup every app has used: burnmon.json then
// claudecost.json next to the exe, then the same two in the fallback folder.
func TestFindConfigOrder(t *testing.T) {
	cases := []struct {
		name                         string
		exeNew, exeOld, fbNew, fbOld bool
		noExe                        bool   // pass "" as the exe folder
		want                         string // exeNew, exeOld, fbNew, fbOld or ""
	}{
		{"burnmon.json next to the exe", true, false, false, false, false, "exeNew"},
		{"only in the fallback folder", false, false, true, false, false, "fbNew"},
		{"both folders: the exe wins", true, false, true, false, false, "exeNew"},
		{"claudecost.json only, next to the exe", false, true, false, false, false, "exeOld"},
		{"claudecost.json only, in the fallback", false, false, false, true, false, "fbOld"},
		{"both names next to the exe: burnmon.json wins", true, true, false, false, false, "exeNew"},
		{"exe claudecost.json before fallback burnmon.json", false, true, true, false, false, "exeOld"},
		{"both names in the fallback: burnmon.json wins", false, false, true, true, false, "fbNew"},
		{"no file anywhere", false, false, false, false, false, ""},
		{"empty exe folder skips that tier", true, false, false, true, true, "fbOld"},
		{"empty exe folder, nothing in the fallback", true, false, false, false, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			exe, fb := t.TempDir(), t.TempDir()
			paths := map[string]string{}
			if c.exeNew {
				paths["exeNew"] = touch(t, exe, "burnmon.json", false)
			}
			if c.exeOld {
				paths["exeOld"] = touch(t, exe, "claudecost.json", false)
			}
			if c.fbNew {
				paths["fbNew"] = touch(t, fb, "burnmon.json", false)
			}
			if c.fbOld {
				paths["fbOld"] = touch(t, fb, "claudecost.json", false)
			}
			arg := exe
			if c.noExe {
				arg = ""
			}
			if got := FindConfig(arg, fb); got != paths[c.want] {
				t.Errorf("FindConfig = %q, want %q", got, paths[c.want])
			}
		})
	}
}

// A folder named burnmon.json counts as present, as the apps' own check
// (os.Stat without a type test) has always said.
func TestFindConfigCountsAFolderNamedLikeTheFile(t *testing.T) {
	fb := t.TempDir()
	want := touch(t, fb, "burnmon.json", true)
	if got := FindConfig("", fb); got != want {
		t.Errorf("FindConfig = %q, want %q", got, want)
	}
}

// With an empty fallback the names stay relative, which is how the CLI looks
// in the current folder and prints what it found.
func TestFindConfigEmptyFallbackIsRelative(t *testing.T) {
	cwd := t.TempDir()
	touch(t, cwd, "claudecost.json", false)
	old, _ := os.Getwd()
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if got := FindConfig("", ""); got != "claudecost.json" {
		t.Errorf("FindConfig = %q, want claudecost.json", got)
	}
}

// FindConfigFunc lets a caller say what counts as a file.
func TestFindConfigFuncUsesTheCallersTest(t *testing.T) {
	fb := t.TempDir()
	touch(t, fb, "burnmon.json", true)
	want := touch(t, fb, "claudecost.json", false)
	isFile := func(p string) bool {
		fi, err := os.Stat(p)
		return err == nil && !fi.IsDir()
	}
	if got := FindConfigFunc("", fb, isFile); got != want {
		t.Errorf("FindConfigFunc = %q, want %q", got, want)
	}
}

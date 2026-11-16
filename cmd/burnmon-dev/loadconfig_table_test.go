//go:build windows || darwin

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// loadConfigAt: which file loads, for every mix of files.
func TestLoadConfigAtWhichFileLoads(t *testing.T) {
	cases := []struct {
		name                         string
		exeNew, exeOld, fbNew, fbOld bool
		noExe                        bool
		want                         string
	}{
		{"next to the exe", true, false, false, false, false, "exeNew"},
		{"only in the fallback", false, false, true, false, false, "fbNew"},
		{"both: exe wins", true, false, true, false, false, "exeNew"},
		{"claudecost.json only", false, true, false, false, false, "exeOld"},
		{"none", false, false, false, false, false, ""},
		{"empty exe folder", true, false, false, true, true, "fbOld"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			exe, fb := t.TempDir(), t.TempDir()
			made := map[string]string{}
			mk := func(on bool, key, dir, name string) {
				if on {
					p := filepath.Join(dir, name)
					if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
						t.Fatal(err)
					}
					made[key] = p
				}
			}
			mk(c.exeNew, "exeNew", exe, "burnmon.json")
			mk(c.exeOld, "exeOld", exe, "claudecost.json")
			mk(c.fbNew, "fbNew", fb, "burnmon.json")
			mk(c.fbOld, "fbOld", fb, "claudecost.json")
			arg := exe
			if c.noExe {
				arg = ""
			}
			if _, got := loadConfigAt(arg, fb); got != made[c.want] {
				t.Errorf("loaded %q, want %q", got, made[c.want])
			}
		})
	}
}

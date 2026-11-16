package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDataDirFrom(t *testing.T) {
	ok := func() (string, error) { return "unused", nil }
	mac := func() (string, error) { return "/Users/a/Library/Application Support", nil }
	empty := func() (string, error) { return "", errors.New("x") }
	blank := func() (string, error) { return "", nil }
	cases := []struct {
		name string
		lad  string
		cfg  func() (string, error)
		want string
	}{
		{"local app data wins", `C:\L`, ok, filepath.Join(`C:\L`, "burnmon")},
		{"user config dir fallback", "", mac, filepath.Join("/Users/a/Library/Application Support", "burnmon")},
		{"temp dir when the config dir errors", "", empty, filepath.Join(os.TempDir(), "burnmon")},
		{"temp dir when the config dir is blank", "", blank, filepath.Join(os.TempDir(), "burnmon")},
	}
	for _, c := range cases {
		if got := DataDirFrom(c.lad, c.cfg); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

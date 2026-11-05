//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAppDataDirFrom(t *testing.T) {
	ok := func() (string, error) { return "unused", nil }
	mac := func() (string, error) { return "/Users/a/Library/Application Support", nil }
	empty := func() (string, error) { return "", errors.New("x") }
	cases := []struct {
		name string
		lad  string
		cfg  func() (string, error)
		want string
	}{
		{"local app data wins", `C:\L`, ok, filepath.Join(`C:\L`, "burnmon")},
		{"user config dir fallback", "", mac, filepath.Join("/Users/a/Library/Application Support", "burnmon")},
		{"temp dir last", "", empty, filepath.Join(os.TempDir(), "burnmon")},
	}
	for _, c := range cases {
		if got := appDataDirFrom(c.lad, c.cfg); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

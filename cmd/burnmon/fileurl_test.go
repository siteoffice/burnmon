package main

import (
	"runtime"
	"testing"
)

func TestToFileURL(t *testing.T) {
	if got := toFileURL("/Users/a/dashboard.html"); got != "file:///Users/a/dashboard.html" {
		t.Errorf("posix path: got %q", got)
	}
	if runtime.GOOS == "windows" {
		if got := toFileURL(`C:\Users\a\dashboard.html`); got != "file:///C:/Users/a/dashboard.html" {
			t.Errorf("windows path: got %q", got)
		}
	}
}

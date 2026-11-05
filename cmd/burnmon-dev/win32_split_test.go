package main

import (
	"os"
	"strings"
	"testing"
)

func TestSharedFilesHaveNoWin32Imports(t *testing.T) {
	banned := []string{`"golang.org/x/sys/windows"`, `"unsafe"`, `"syscall"`, "user32.NewProc", "kernel32"}
	for _, f := range []string{"app.go", "item_files.go", "perf.go", "toast_watch.go", "main.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range banned {
			if strings.Contains(string(b), bad) {
				t.Errorf("%s contains %s", f, bad)
			}
		}
	}
	b, err := os.ReadFile("payloads.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "func startupMark(") {
		t.Error("payloads.go does not define func startupMark(")
	}
}

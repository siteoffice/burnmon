// Command uicheck drives the real burnmon.exe WebView2 window so a session
// can prove a fix against the running window instead of a proxy check
// (02_roadmap\2026-09-23_v0.2.3_window_check_patch.md, W0). It finds the
// window by title and screenshots/clicks/types via pure Win32 (PrintWindow,
// SendInput), and reads/evaluates the page's own DOM/JS through a small
// dev-only TCP channel burnmon.exe opens itself when BURNMON_UICHECK=1
// (cmd\burnmon\uicheck_devserver.go); release builds and the README never
// set that variable, so this never opens anything for a real user.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// isDevCheck reports whether name targets burnmon-dev.exe (its own window
// title and dev eval port) rather than burnmon.exe. Since the group checks
// (groups.go) a prefix no longer tells: "waits" starts with "w", so this
// asks the group registry instead.
func isDevCheck(name string) bool {
	return devNames[name]
}

type checkFunc func(hwnd uintptr, args []string) error

// checks is the registry every W-item's check lives in; scripts\uicheck.ps1
// runs one or more of these by name against the running burnmon.exe.
var checks = map[string]checkFunc{}

// selfManaged holds checks that start and stop their own burnmon.exe
// instance instead of using one scripts\uicheck.ps1 already started (W1
// tests startup itself, so it needs a truly cold launch, not a window
// that's already finished loading).
var selfManaged = map[string]bool{"w1": true}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "uicheck: "+format+"\n", args...)
	os.Exit(1)
}

func checkNames() string {
	names := make([]string, 0, len(checks))
	for n := range checks {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func main() {
	if len(os.Args) < 2 {
		fatal("usage: uicheck <check> [args...]\navailable checks: %s", checkNames())
	}
	name := os.Args[1]
	args := os.Args[2:]

	fn, ok := checks[name]
	if !ok {
		fatal("unknown check %q\navailable checks: %s", name, checkNames())
	}

	var hwnd uintptr
	if !selfManaged[name] {
		var err error
		if isDevCheck(name) {
			currentEvalAddr = evalAddrDev
			hwnd, err = findBurnmonDevWindow()
			if err != nil {
				fatal("%v", err)
			}
			// Each d* check sets its own window size (the design doc's two
			// distinct viewports, 1152x2048 and 1024x1152), rather than one
			// size main.go picks for every check the way burnmon.exe's own
			// fixed 1280x860 does.
		} else {
			hwnd, err = findBurnmonWindow()
			if err != nil {
				fatal("%v", err)
			}
			if err := ensureWindowSize(hwnd); err != nil {
				fatal("%v", err)
			}
		}
	}

	if err := fn(hwnd, args); err != nil {
		fatal("check %q failed: %v", name, err)
	}
	fmt.Printf("uicheck: %s OK\n", name)
}

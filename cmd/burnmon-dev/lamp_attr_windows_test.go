//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Fix lane T2, finding 1: the lamp child is not started hidden. A hidden start
// (STARTF_USESHOWWINDOW with SW_HIDE) made the window of a lamp that BurnMon
// Dev started invisible. The lane replaces the earlier test that required the
// hidden start; this is that edit.
func TestLampChildAttrDoesNotHideOnWindows(t *testing.T) {
	if a := lampChildAttr(); a != nil && a.HideWindow {
		t.Errorf("lampChildAttr = %+v, want no HideWindow on Windows", a)
	}
}

// TestLampChildStartShowsWindow starts the lamp through execLampStart, the
// function BurnMon Dev uses, and reads from Windows that the lamp's window is
// visible within 3 seconds: visible, not cloaked, a rectangle that meets a
// monitor, a window region and a visible web view child. It
// opens a real window, so it runs only with BURNMON_LAMP_WINDOW_TEST=1 (the
// script scripts\uicheck-lamp.ps1 sets it). Whether pixels show is for the first look.
func TestLampChildStartShowsWindow(t *testing.T) {
	if os.Getenv("BURNMON_LAMP_WINDOW_TEST") != "1" {
		t.Skip("opens a real window; set BURNMON_LAMP_WINDOW_TEST=1")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "burnmon-dev.exe")
	build := exec.Command("go", "build", "-o", exe, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	// The child gets its own data folders so it touches nothing of Wilco's.
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "local"))
	t.Setenv("APPDATA", filepath.Join(dir, "roaming"))
	for _, d := range []string{"local", "roaming"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	p, err := execLampStart(exe, []string{"lamp"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		p.Stop()
		<-p.Done()
	}()
	ep, ok := p.(*execLampProc)
	if !ok {
		t.Fatalf("execLampStart gave %T", p)
	}
	pid := uint32(ep.cmd.Process.Pid)

	var last string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if msg := lampWindowProblem(pid); msg == "" {
			return
		} else {
			last = msg
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("no visible lamp window of process %d within 3 s: %s", pid, last)
}

// lampWindowProblem is "" when pid owns a visible, uncloaked top-level window
// that meets a monitor, has a region and a visible web view child; else what is missing.
func lampWindowProblem(pid uint32) string {
	user32 := windows.NewLazySystemDLL("user32.dll")
	enumWindows := user32.NewProc("EnumWindows")
	enumChildren := user32.NewProc("EnumChildWindows")
	getPid := user32.NewProc("GetWindowThreadProcessId")
	isVisible := user32.NewProc("IsWindowVisible")
	getRect := user32.NewProc("GetWindowRect")
	monFromRect := user32.NewProc("MonitorFromRect")

	var found uintptr
	cb := windows.NewCallback(func(h, _ uintptr) uintptr {
		var owner uint32
		getPid.Call(h, uintptr(unsafe.Pointer(&owner)))
		if owner == pid {
			if vis, _, _ := isVisible.Call(h); found == 0 || vis != 0 {
				found = h
			}
		}
		return 1
	})
	enumWindows.Call(cb, 0)
	if found == 0 {
		return "no top-level window yet"
	}
	if vis, _, _ := isVisible.Call(found); vis == 0 {
		return "window exists but is not visible"
	}
	dwm := windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute")
	var cloaked uint32
	const dwmwaCloaked = 14
	if r, _, _ := dwm.Call(found, dwmwaCloaked, uintptr(unsafe.Pointer(&cloaked)), 4); r == 0 && cloaked != 0 {
		return "window is cloaked"
	}
	var rc struct{ left, top, right, bottom int32 }
	getRect.Call(found, uintptr(unsafe.Pointer(&rc)))
	if rc.right <= rc.left || rc.bottom <= rc.top {
		return "window rectangle is empty"
	}
	if m, _, _ := monFromRect.Call(uintptr(unsafe.Pointer(&rc)), 0); m == 0 { // MONITOR_DEFAULTTONULL
		return "window rectangle meets no monitor"
	}
	var box struct{ left, top, right, bottom int32 }
	getBox := windows.NewLazySystemDLL("user32.dll").NewProc("GetWindowRgnBox")
	// 0 is an error (no region), 1 NULLREGION.
	if r, _, _ := getBox.Call(found, uintptr(unsafe.Pointer(&box))); r <= 1 {
		return "window has no region"
	}
	// The web view is a visible child window: the page was made under a shown host.
	var child bool
	kids := windows.NewCallback(func(h, _ uintptr) uintptr {
		if vis, _, _ := isVisible.Call(h); vis != 0 {
			child = true
		}
		return 1
	})
	enumChildren.Call(found, kids, 0)
	if !child {
		return "no visible web view child window"
	}
	return ""
}

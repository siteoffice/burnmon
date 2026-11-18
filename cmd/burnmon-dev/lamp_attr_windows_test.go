//go:build windows

package main

import (
	"bufio"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"burnmon/internal/lamp"
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

// lampTestExe builds burnmon-dev.exe into dir and points the data folders of
// this test (and of every child it starts) at folders under dir, so nothing of
// the person's is touched. It returns the exe and the settings folder.
func lampTestExe(t *testing.T) (exe, setDir string) {
	t.Helper()
	dir := t.TempDir()
	exe = filepath.Join(dir, "burnmon-dev.exe")
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	setDir = filepath.Join(dir, "settings")
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "local"))
	t.Setenv("APPDATA", filepath.Join(dir, "roaming"))
	t.Setenv(settingsDirEnv, setDir)
	for _, d := range []string{"local", "roaming", "settings"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return exe, setDir
}

// waitLampWindow waits up to 10 s for pid's window to be visible.
func waitLampWindow(t *testing.T, pid uint32) {
	t.Helper()
	var last string
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if last = lampWindowProblem(pid); last == "" {
			return
		}
	}
	t.Fatalf("no visible lamp window of process %d within 10 s: %s", pid, last)
}

// Close fix T1, finding 11: "Close the lamp" must end the process. The lamp
// starts through execLampStart (the start BurnMon Dev uses), the test runs the
// page's own menu call lampClose() through the dev eval channel (the click
// handler of the menu item calls exactly that), and the process must be gone
// within 5 seconds with lamp_enabled false saved. Opt-in like the test above.
func TestLampCloseFromMenuEndsTheProcess(t *testing.T) {
	if os.Getenv("BURNMON_LAMP_WINDOW_TEST") != "1" {
		t.Skip("opens a real window; set BURNMON_LAMP_WINDOW_TEST=1")
	}
	exe, setDir := lampTestExe(t)
	if err := saveDevConfigKey(setDir, "lamp_enabled", true); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BURNMON_LAMP_UICHECK", "1")
	p, err := execLampStart(exe, []string{"lamp"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		p.Stop()
		<-p.Done()
	}()
	waitLampWindow(t, uint32(p.(*execLampProc).cmd.Process.Pid))

	var conn net.Conn
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if conn, err = net.Dial("tcp", "127.0.0.1:9336"); err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("the lamp eval channel never opened (is another lamp demo running?): %v", err)
	}
	closed := time.Now()
	if _, err := conn.Write([]byte(`{"script":"window.lampClose()"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	// The answer may not come: the lamp can end before it writes it.
	_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, _ = bufio.NewReader(conn).ReadString('\n')
	conn.Close()

	select {
	case <-p.Done():
		// Under 3 s means the window loop ended; 3 s or more means the second line (exit) did.
		took := time.Since(closed)
		t.Logf("the lamp ended %v after the menu call", took.Round(10*time.Millisecond))
		if took >= lampEndGrace-500*time.Millisecond {
			t.Fatalf("the lamp took %v: the window loop did not end, only the exit after the grace time did", took)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the lamp process is still running 5 s after Close the lamp")
	}
	b, err := os.ReadFile(devConfigReadPath(setDir))
	if err != nil {
		t.Fatal(err)
	}
	if lamp.ParseSettings(b).Enabled {
		t.Fatalf("lamp_enabled is not false after Close the lamp: %s", b)
	}
}

// Close fix T1: a lamp whose BURNMON_LAMP_PARENT names a process that does not
// exist ends within 6 seconds. The id is the one of a process that has just ended.
func TestLampEndsWhenItsParentIsGone(t *testing.T) {
	if os.Getenv("BURNMON_LAMP_WINDOW_TEST") != "1" {
		t.Skip("opens a real window; set BURNMON_LAMP_WINDOW_TEST=1")
	}
	exe, _ := lampTestExe(t)
	gone := exec.Command("cmd", "/c", "exit", "0")
	if err := gone.Run(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "lamp")
	cmd.Env = lampChildEnv(os.Environ(), gone.Process.Pid)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	defer func() {
		_ = cmd.Process.Kill()
		<-done
	}()
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatalf("the lamp is still running 6 s after its parent %d ended", gone.Process.Pid)
	}
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

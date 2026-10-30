//go:build windows

// windowstate.go is the UI review patch's section 10 ("window, fullscreen,
// responsive"): remember size, position, maximized state and monitor
// across launches (restoring them only if that monitor still exists), and
// F11/Esc fullscreen. Built on the same raw Win32 pattern app.go's own
// single-instance code and tools\uicheck\win32.go already use (this
// library, go-webview2, exposes only Bind/Dispatch/Eval/SetSize/Navigate on
// its WebView interface, nothing for window placement or styles), reusing
// app.go's own `user32` LazyDLL handle rather than opening a second one.
package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procGetWindowPlacement  = user32.NewProc("GetWindowPlacement")
	procSetWindowPlacement  = user32.NewProc("SetWindowPlacement")
	procMonitorFromWindow   = user32.NewProc("MonitorFromWindow")
	procMonitorFromPoint    = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
	procSetWindowPosWS      = user32.NewProc("SetWindowPos")
	procGetWindowLongPtrW   = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW   = user32.NewProc("SetWindowLongPtrW")
	procShowWindowWS        = user32.NewProc("ShowWindow")
	procRegisterHotKey      = user32.NewProc("RegisterHotKey")
	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	kernel32WS              = windows.NewLazySystemDLL("kernel32.dll")
	procGetCurrentThreadID  = kernel32WS.NewProc("GetCurrentThreadId")
)

type point32 struct{ X, Y int32 }
type rect32 struct{ Left, Top, Right, Bottom int32 }

// windowPlacementT mirrors Win32's WINDOWPLACEMENT: 44 bytes on real Win32
// (winuser.h's own rcDevice tail field is inside "#ifdef _MAC", the old Mac
// port, not present on Windows at all). Getting this wrong by even one
// field makes Length never match what Get/SetWindowPlacement expect, and
// both calls then simply fail (found by review of this file's own first
// draft, which included an extra RcDevice field, sizing Length at 60 rather
// than 44: SetWindowPlacement's return value went unchecked and unlogged,
// so the restore silently no-opped every time and window-state restore
// looked, at a glance, like it worked because a first-launch centered
// window and the "restore failed, stayed at the default centered position"
// symptom are visually identical).
type windowPlacementT struct {
	Length           uint32
	Flags            uint32
	ShowCmd          uint32
	PtMinPosition    point32
	PtMaxPosition    point32
	RcNormalPosition rect32
}

// monitorInfoExW mirrors Win32's MONITORINFOEXW.
type monitorInfoExW struct {
	CbSize    uint32
	RcMonitor rect32
	RcWork    rect32
	DwFlags   uint32
	SzDevice  [32]uint16
}

const (
	gwlStyle = ^uintptr(15) // GWL_STYLE (-16), sign-extended for a 64-bit call

	wsCaption     = 0x00C00000
	wsThickFrame  = 0x00040000
	wsMinimizeBox = 0x00020000
	wsMaximizeBox = 0x00010000
	wsSysMenu     = 0x00080000
	// wsOverlappedWindowFrame is every style bit a normal titled, resizable,
	// sysmenu'd window carries beyond the plain client area: removing these
	// (and only these) is the standard "borderless fullscreen" recipe.
	wsOverlappedWindowFrame = wsCaption | wsThickFrame | wsMinimizeBox | wsMaximizeBox | wsSysMenu

	swpFrameChanged   = 0x0020
	swpNoOwnerZOrder  = 0x0200
	swpShowWindowFlag = 0x0040
	swpNoMove         = 0x0002
	swpNoSize         = 0x0001
	swpNoZOrder       = 0x0004
	hwndTop           = 0

	monitorDefaultToNearest = 2
	monitorDefaultToNull    = 0

	swShowNormal     = 1
	swShowMinimized  = 2
	swShowMaximized  = 3
	// wpfRestoreToMaximized: WINDOWPLACEMENT.Flags bit set when the window
	// is currently minimized FROM a maximized state, so ShowCmd alone
	// (SW_SHOWMINIMIZED) would otherwise lose that it was maximized before
	// (found by review, 2026-09-25).
	wpfRestoreToMaximized = 0x0002
)

func getWindowPlacementDev(hwnd uintptr) (windowPlacementT, bool) {
	var wp windowPlacementT
	wp.Length = uint32(unsafe.Sizeof(wp))
	ret, _, _ := procGetWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&wp)))
	return wp, ret != 0
}

func setWindowPlacementDev(hwnd uintptr, wp *windowPlacementT) bool {
	ret, _, _ := procSetWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(wp)))
	return ret != 0
}

// monitorInfoAt returns the full MONITORINFOEXW (bounds, work area and
// device name) of the monitor whose bounds contain the given point, or
// ok=false when no monitor covers it (monitorDefaultToNull).
// MonitorFromPoint takes a POINT BY VALUE, which the x64 calling convention
// packs into one 8-byte argument (x in the low 32 bits, y in the high 32
// bits), not two separate Call() arguments (found by review of this file's
// own first draft: passing x and y separately shifted dwFlags into an
// unused third register, so this always returned 0/no monitor found,
// silently wiping every saved window state's Monitor field to "").
func monitorInfoAt(x, y int32) (monitorInfoExW, bool) {
	pt := uintptr(uint32(x)) | uintptr(uint32(y))<<32
	hmon, _, _ := procMonitorFromPoint.Call(pt, monitorDefaultToNull)
	if hmon == 0 {
		return monitorInfoExW{}, false
	}
	var mi monitorInfoExW
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	ret, _, _ := procGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi)))
	return mi, ret != 0
}

func monitorDeviceNameOf(mi monitorInfoExW) string {
	n := 0
	for n < len(mi.SzDevice) && mi.SzDevice[n] != 0 {
		n++
	}
	return string(utf16Decode(mi.SzDevice[:n]))
}

// monitorDeviceAt is monitorInfoAt's device name alone, the "does that
// monitor still exist" check currentWindowState's own save path needs.
func monitorDeviceAt(x, y int32) string {
	mi, ok := monitorInfoAt(x, y)
	if !ok {
		return ""
	}
	return monitorDeviceNameOf(mi)
}

func utf16Decode(u []uint16) []rune {
	out := make([]rune, 0, len(u))
	for _, c := range u {
		out = append(out, rune(c))
	}
	return out
}

func monitorRectForWindow(hwnd uintptr) (rect32, bool) {
	hmon, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultToNearest)
	if hmon == 0 {
		return rect32{}, false
	}
	var mi monitorInfoExW
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	ret, _, _ := procGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi)))
	if ret == 0 {
		return rect32{}, false
	}
	return mi.RcMonitor, true
}

// windowState is what gets persisted (burnmon-dev-window.json, settingsDir):
// section 10's "remember size, position, maximized state and monitor;
// restore them if that monitor still exists".
type windowState struct {
	X, Y, Width, Height int32
	Maximized           bool
	// Monitor is the device name (MonitorFromPoint on the saved window's
	// center) at save time, checked again at restore time.
	Monitor string
}

func windowStatePath(dataDir string) string {
	return filepath.Join(dataDir, "burnmon-dev-window.json")
}

// loadWindowState returns the last saved placement and whether its own
// monitor still exists: a caller restoring a not-still-existing monitor's
// coordinates would otherwise plant the window off-screen (a docked laptop
// undocked, or an external monitor unplugged, since the last run). Also
// clamps the saved rect into that monitor's current work area: the device
// name matching is not enough on its own if the monitor's resolution
// changed since the save (e.g. a lower resolution now, found by review,
// 2026-09-25) - a saved rect larger than or partly outside the current work
// area would otherwise still restore off-screen or oversized.
func loadWindowState(dataDir string) (windowState, bool) {
	b, err := os.ReadFile(windowStatePath(dataDir))
	if err != nil {
		return windowState{}, false
	}
	var st windowState
	if err := json.Unmarshal(b, &st); err != nil || st.Width <= 0 || st.Height <= 0 {
		return windowState{}, false
	}
	centerX := st.X + st.Width/2
	centerY := st.Y + st.Height/2
	mi, ok := monitorInfoAt(centerX, centerY)
	if !ok || monitorDeviceNameOf(mi) != st.Monitor || st.Monitor == "" {
		return windowState{}, false
	}

	work := mi.RcWork
	if maxW := work.Right - work.Left; st.Width > maxW {
		st.Width = maxW
	}
	if maxH := work.Bottom - work.Top; st.Height > maxH {
		st.Height = maxH
	}
	if st.X < work.Left {
		st.X = work.Left
	}
	if st.Y < work.Top {
		st.Y = work.Top
	}
	if st.X+st.Width > work.Right {
		st.X = work.Right - st.Width
	}
	if st.Y+st.Height > work.Bottom {
		st.Y = work.Bottom - st.Height
	}
	return st, true
}

// applyWindowState positions/sizes/maximizes hwnd to a previously saved
// state, called once right after window creation (main.go).
func applyWindowState(hwnd uintptr, st windowState) {
	showCmd := uint32(swShowNormal)
	if st.Maximized {
		showCmd = swShowMaximized
	}
	wp := windowPlacementT{
		Length:           uint32(unsafe.Sizeof(windowPlacementT{})),
		ShowCmd:          showCmd,
		RcNormalPosition: rect32{Left: st.X, Top: st.Y, Right: st.X + st.Width, Bottom: st.Y + st.Height},
	}
	if !setWindowPlacementDev(hwnd, &wp) {
		log.Println("window state: could not restore the saved placement")
	}
}

// startWindowStateSaver polls the window's own placement every few seconds
// and persists it on change: go-webview2's WebView interface has no
// move/resize/close hook to save on, and polling also means a state that
// ends by being killed (taskkill, a crash) still has its last-seen size and
// position recorded, not just a clean exit. Skips every tick while fs is
// fullscreen (found by review, 2026-09-25: entering fullscreen moves the
// window via plain SetWindowPos, not SetWindowPlacement, which also
// overwrites GetWindowPlacement's own rcNormalPosition to the monitor
// -covering rect; saving that as "the normal window state" would restore a
// captioned window the size of the whole monitor, partly off-screen, if the
// app were killed while fullscreen). fs.active is an atomic.Bool rather than
// a plain bool since this goroutine reads it while enterFullscreen/
// exitFullscreen write it from a different one (the hook/bind-callback
// thread).
func startWindowStateSaver(hwnd uintptr, dataDir string, fs *fullscreenState) {
	go func() {
		var last windowState
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if fs.active.Load() {
				continue
			}
			cur, ok := currentWindowState(hwnd)
			if !ok || cur == last {
				continue
			}
			last = cur
			if err := saveWindowState(dataDir, cur); err != nil {
				log.Println("window state: could not save:", err)
			}
		}
	}()
}

// saveWindowState writes one placement; split out of the saver so a test can
// prove where the file lands without a window.
func saveWindowState(dataDir string, st windowState) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(windowStatePath(dataDir), b, 0o644)
}

func currentWindowState(hwnd uintptr) (windowState, bool) {
	wp, ok := getWindowPlacementDev(hwnd)
	if !ok {
		return windowState{}, false
	}
	r := wp.RcNormalPosition
	// A window minimized from a maximized state reports ShowCmd
	// SW_SHOWMINIMIZED, which alone would lose that it was maximized;
	// WPF_RESTORETOMAXIMIZED in Flags is what actually carries that
	// (found by review, 2026-09-25).
	maximized := wp.ShowCmd == swShowMaximized ||
		(wp.ShowCmd == swShowMinimized && wp.Flags&wpfRestoreToMaximized != 0)
	st := windowState{
		X: r.Left, Y: r.Top, Width: r.Right - r.Left, Height: r.Bottom - r.Top,
		Maximized: maximized,
	}
	if st.Width <= 0 || st.Height <= 0 {
		return windowState{}, false
	}
	st.Monitor = monitorDeviceAt(st.X+st.Width/2, st.Y+st.Height/2)
	return st, true
}

// fullscreenState holds what toggleFullscreen needs to restore on exit: the
// pre-fullscreen window style (borderless fullscreen removes WS_CAPTION
// etc.) and placement (SetWindowPlacement restores maximized/normal and the
// exact rect in one call, the same struct currentWindowState/
// applyWindowState already use). active is an atomic.Bool, not a plain
// bool: startWindowStateSaver's own goroutine reads it on a timer while
// enterFullscreen/exitFullscreen write it from the hook/bind-callback
// thread (found by review, 2026-09-25).
type fullscreenState struct {
	active    atomic.Bool
	prevStyle uintptr
	prevPlace windowPlacementT
}

// enterFullscreen and exitFullscreen implement section 10's F11 (both call
// this through toggleFullscreen below) and Esc (exitFullscreen only, a
// no-op when not already fullscreen so the page's Esc handler can call it
// unconditionally alongside its own turn-popup close).
func enterFullscreen(hwnd uintptr, fs *fullscreenState) {
	if fs.active.Load() {
		return
	}
	place, ok := getWindowPlacementDev(hwnd)
	if !ok {
		return
	}
	style, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlStyle)
	monRect, ok := monitorRectForWindow(hwnd)
	if !ok {
		return
	}
	fs.prevStyle = style
	fs.prevPlace = place
	fs.active.Store(true)

	newStyle := style &^ uintptr(wsOverlappedWindowFrame)
	procSetWindowLongPtrW.Call(hwnd, gwlStyle, newStyle)
	procSetWindowPosWS.Call(hwnd, hwndTop,
		uintptr(uint32(monRect.Left)), uintptr(uint32(monRect.Top)),
		uintptr(uint32(monRect.Right-monRect.Left)), uintptr(uint32(monRect.Bottom-monRect.Top)),
		uintptr(swpFrameChanged|swpNoOwnerZOrder|swpShowWindowFlag))
}

func exitFullscreen(hwnd uintptr, fs *fullscreenState) {
	if !fs.active.Load() {
		return
	}
	procSetWindowLongPtrW.Call(hwnd, gwlStyle, fs.prevStyle)
	place := fs.prevPlace
	setWindowPlacementDev(hwnd, &place)
	procSetWindowPosWS.Call(hwnd, 0, 0, 0, 0, 0, uintptr(swpFrameChanged|swpNoMove|swpNoSize|swpNoZOrder))
	fs.active.Store(false)
}

func toggleFullscreen(hwnd uintptr, fs *fullscreenState) {
	if fs.active.Load() {
		exitFullscreen(hwnd, fs)
	} else {
		enterFullscreen(hwnd, fs)
	}
}

// ---------------------------------------------------------------------------
// F11 key delivery.
//
// The page's own keydown listener (page.html) never sees a real F11
// keypress: WebView2/Chromium reserves F11 as one of its default "browser
// accelerator keys" (ICoreWebViewSettings.AreBrowserAcceleratorKeysEnabled,
// on by default) and consumes it before it reaches the DOM at all, found
// empirically this session (window.bdevToggleFullscreen() called directly
// through the uicheck dev eval channel worked immediately; a real F11
// keypress sent through SendInput to the focused window did nothing).
// go-webview2's WebView interface exposes no settings object to turn that
// off (only Bind/Dispatch/Eval/SetSize/Navigate/Window), so this instead
// bypasses the page entirely for this one key: a global hotkey
// (RegisterHotKey) delivers a WM_HOTKEY message regardless of what has
// focus or what Chromium's own accelerator table wants, and a thread
// -local WH_GETMESSAGE hook on the same thread go-webview2's own message
// loop already runs on (installed before w.Run()) is what actually reacts
// to it, since wndproc itself is unexported inside go-webview2's package.
// ---------------------------------------------------------------------------

const (
	vkF11        = 0x7A
	modNoRepeat  = 0x4000
	wmHotkey     = 0x0312
	whGetMessage = 3
	hcAction     = 0
	f11HotkeyID  = 1
)

// msgT mirrors Win32's MSG, enough of it (through wParam) for this hook to
// read which message and hotkey id a WM_HOTKEY carries; Go's own struct
// layout already inserts the same alignment padding before WParam a C
// compiler would, so the trailing fields this file never reads (Time, Pt)
// need no explicit padding of their own.
type msgT struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point32
}

// installF11Hotkey registers VK_F11 as a global hotkey and installs the
// thread-local hook that reacts to it, both against hwnd's own owning
// thread (go-webview2's LockOSThread'd main goroutine, the same thread
// callers of this function run on). Must be called before w.Run() starts
// pumping that thread's message loop.
func installF11Hotkey(hwnd uintptr, fs *fullscreenState) {
	if ret, _, err := procRegisterHotKey.Call(hwnd, uintptr(f11HotkeyID), modNoRepeat, vkF11); ret == 0 {
		log.Println("could not register the F11 hotkey:", err)
		return
	}
	threadID, _, _ := procGetCurrentThreadID.Call()

	// The callback's lParam parameter is typed unsafe.Pointer, not uintptr,
	// specifically so reading the MSG it points to is an ordinary (*T)(Pointer)
	// conversion rather than a Pointer(uintptr) one: go vet's unsafeptr check
	// flags the latter (it cannot know a hook's own lParam is a genuine,
	// OS-kept-alive address, not stale Go-GC-tracked memory), and this
	// project's own verify step runs a clean `go vet ./...` (found by
	// review, 2026-09-25: an earlier draft wrote unsafe.Pointer(lParam) with
	// lParam uintptr and vet rightly flagged it). windows.NewCallback marshals
	// unsafe.Pointer as the same machine word a uintptr would be, so this is
	// a same-ABI, vet-clean rewrite, not a behavior change.
	var hHook uintptr
	hookProc := windows.NewCallback(func(nCode int32, wParam uintptr, lParam unsafe.Pointer) uintptr {
		if nCode == hcAction {
			m := (*msgT)(lParam)
			// RegisterHotKey is system-wide: without this check, F11 would
			// toggle this window's fullscreen even while some other app has
			// focus, and would steal F11 from every other program for as
			// long as BurnMon Dev runs (found by review, 2026-09-25). Only
			// act when this window is the foreground one, or when there is
			// no foreground window at all (fg==0: no other app to protect
			// -  observed for real in a disconnected/non-interactive
			// session while testing this, where GetForegroundWindow never
			// returns a real window at all).
			if fg, _, _ := procGetForegroundWindow.Call(); m.Message == wmHotkey && m.WParam == f11HotkeyID && (fg == 0 || fg == hwnd) {
				toggleFullscreen(hwnd, fs)
			}
		}
		ret, _, _ := procCallNextHookEx.Call(hHook, uintptr(nCode), wParam, uintptr(lParam))
		return ret
	})
	ret, _, err := procSetWindowsHookExW.Call(whGetMessage, hookProc, 0, threadID)
	if ret == 0 {
		log.Println("could not install the F11 key hook:", err)
		return
	}
	hHook = ret
}

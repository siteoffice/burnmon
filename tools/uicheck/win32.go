//go:build windows

package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW         = user32.NewProc("FindWindowW")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procGetClientRect       = user32.NewProc("GetClientRect")
	procClientToScreen      = user32.NewProc("ClientToScreen")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procSetCursorPos        = user32.NewProc("SetCursorPos")
	procSendInput           = user32.NewProc("SendInput")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procGetWindowTextW      = user32.NewProc("GetWindowTextW")
	procIsWindow            = user32.NewProc("IsWindow")
	procSetProcessDPIAware  = user32.NewProc("SetProcessDPIAware")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procAttachThreadInput   = user32.NewProc("AttachThreadInput")

	procSetWindowPos = user32.NewProc("SetWindowPos")
	procShowWindow   = user32.NewProc("ShowWindow")

	procGetDpiForWindow          = user32.NewProc("GetDpiForWindow")
	procAdjustWindowRectExForDpi = user32.NewProc("AdjustWindowRectExForDpi")
	procGetWindowLongW           = user32.NewProc("GetWindowLongW")
	procMonitorFromWindow        = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW          = user32.NewProc("GetMonitorInfoW")

	gdi32               = windows.NewLazySystemDLL("gdi32.dll")
	procCreateCompatDC  = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatBmp = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject    = gdi32.NewProc("SelectObject")
	procDeleteDC        = gdi32.NewProc("DeleteDC")
	procDeleteObject    = gdi32.NewProc("DeleteObject")
	procGetDIBits       = gdi32.NewProc("GetDIBits")
	procBitBlt          = gdi32.NewProc("BitBlt")
)

const windowTitle = "BurnMon"

// windowTitleDev is burnmon-dev.exe's own window title (cmd\burnmon-dev
// app.go's windowTitle constant), distinct from "BurnMon" so FindWindowW's
// exact-match lookup never confuses the two exes when both run at once
// (the design doc's own normal-case scenario).
const windowTitleDev = "BurnMon Dev"

// Without this, GetWindowRect/ClientToScreen/PrintWindow all return
// DPI-virtualized coordinates and a scaled-down bitmap for a DPI-aware
// window like burnmon.exe's (any WebView2 host is DPI-aware by default),
// which both mis-sizes screenshots and throws off click coordinate math in
// eval.go's screenCoords.
func init() {
	procSetProcessDPIAware.Call()
}

type rect struct{ Left, Top, Right, Bottom int32 }
type point struct{ X, Y int32 }

// findBurnmonWindow returns the HWND of the running burnmon.exe window
// (scripts\uicheck.ps1 starts it before running any check), or an error if
// no window with that exact title exists yet.
func findBurnmonWindow() (uintptr, error) {
	title, err := windows.UTF16PtrFromString(windowTitle)
	if err != nil {
		return 0, err
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		return 0, fmt.Errorf("no window titled %q found; is burnmon.exe running?", windowTitle)
	}
	return hwnd, nil
}

// findBurnmonWindowFast is findBurnmonWindow without the "not found" error
// wrapping, for a caller that polls in a tight loop right after launch (W1)
// and expects "not found yet" as a normal, frequent result rather than
// something worth formatting an error for on every attempt.
func findBurnmonWindowFast() uintptr {
	title, err := windows.UTF16PtrFromString(windowTitle)
	if err != nil {
		return 0
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title)))
	return hwnd
}

// findBurnmonDevWindow is findBurnmonWindow for burnmon-dev.exe's own
// window title.
func findBurnmonDevWindow() (uintptr, error) {
	title, err := windows.UTF16PtrFromString(windowTitleDev)
	if err != nil {
		return 0, err
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		return 0, fmt.Errorf("no window titled %q found; is burnmon-dev.exe running?", windowTitleDev)
	}
	return hwnd, nil
}

func isWindow(hwnd uintptr) bool {
	ret, _, _ := procIsWindow.Call(hwnd)
	return ret != 0
}

var procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")

// isRealBurnmonWindow rules out a Windows "ghost window": when a GUI app is
// force-killed while its window still exists, explorer.exe temporarily
// takes ownership of the orphaned HWND to show a "(Not Responding)"
// placeholder in Alt-Tab/the taskbar, and FindWindowW still finds it by
// title. This laptop has seen exactly that this session (repeated
// Stop-Process -Force during testing), so checks that gate on "is burnmon
// already running" must confirm the window's owning process is actually
// burnmon.exe, not just that a window with the right title exists.
func isRealBurnmonWindow(hwnd uintptr) bool {
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return false
	}
	name := strings.ToLower(windows.UTF16ToString(buf[:size]))
	return strings.HasSuffix(name, "burnmon.exe")
}

func windowTitleText(hwnd uintptr) string {
	buf := make([]uint16, 512)
	n, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// ensureWindowSize forces burnmon's window to a known size/position
// (restored, not minimized/maximized, at (100,100), 1280x860 like main.go's
// own WindowOptions), regardless of whatever state it was left in by a
// previous run or manual resize. Every check calls this first so results
// are deterministic and comparable across runs.
func ensureWindowSize(hwnd uintptr) error {
	return ensureWindowSizeWH(hwnd, 1280, 860)
}

// ensureWindowSizeWH is ensureWindowSize with an explicit size, for
// burnmon-dev.exe's own two viewports (1152x2048 primary, 1024x1152
// secondary, design doc "Viewports") rather than burnmon.exe's fixed
// 1280x860. width/height are CSS pixels, the unit every design doc and
// spec actually means by "1152x2048": on a scaled display (this laptop
// runs its sandboxed sessions at ~200%) a plain SetWindowPos in physical
// pixels lands the page's real CSS viewport at roughly half the intended
// size, which is what SESSION_LOG's 2026-09-26 entry found. This converts
// the requested CSS size to physical window pixels using the target
// monitor's real DPI, then caps to that monitor so a too-small screen
// gets a real (logged) window instead of an off-screen or clipped one.
const hwndTopmost = ^uintptr(0) // -1
const swpShowWindow = 0x0040

// windowOriginX/Y is where ensureWindowSizeWH puts the window: (100,100) on
// the primary screen, or UICHECK_ORIGIN="x,y" in virtual-screen pixels to
// run a check on another monitor (for example a small laptop panel next to
// a large primary one, to see what a laptop-only setup does).
var windowOriginX, windowOriginY = func() (int32, int32) { x, y, _ := parseOrigin(os.Getenv("UICHECK_ORIGIN")); return x, y }()

// windowCapped is true when the last ensureWindowSizeWH could not give the
// window the size asked for because its monitor is too small. A check whose
// numbers only hold at the named size reports such a case as skipped instead
// of failing on a window that was never that size.
var windowCapped bool

// parseOrigin reads "x,y"; anything else gives the default (100,100), false.
func parseOrigin(s string) (int32, int32, bool) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 100, 100, false
	}
	x, errX := strconv.Atoi(strings.TrimSpace(parts[0]))
	y, errY := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errX != nil || errY != nil {
		return 100, 100, false
	}
	return int32(x), int32(y), true
}

// capToScreen limits a window that starts at (originX, originY) so it ends
// no further right or down than scr, and says whether it had to.
func capToScreen(outerW, outerH, originX, originY int32, scr rect) (int32, int32, bool) {
	capped := false
	if maxW := scr.Right - originX; outerW > maxW {
		outerW, capped = maxW, true
	}
	if maxH := scr.Bottom - originY; outerH > maxH {
		outerH, capped = maxH, true
	}
	return outerW, outerH, capped
}

func ensureWindowSizeWH(hwnd uintptr, cssWidth, cssHeight int32) error {
	const swRestore = 9
	procShowWindow.Call(hwnd, swRestore)

	// Position first, size untouched, so GetDpiForWindow/MonitorFromWindow
	// below read the monitor the window is actually about to sit on
	// (matters on a multi-monitor, mixed-DPI setup) rather than wherever a
	// previous check happened to leave it.
	const swpNoSize = 0x0001
	ret, _, err := procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(windowOriginX), uintptr(windowOriginY), 0, 0, swpShowWindow|swpNoSize)
	if ret == 0 {
		return fmt.Errorf("SetWindowPos (position): %w", err)
	}
	// BitBlt-based screenshot (screenshot, win32.go) samples whatever DWM
	// has actually composited on screen at these coordinates, not this HWND
	// specifically, so another ordinary window merely occupying the same
	// screen region on top of it would silently capture (and receive
	// clicks aimed at) that window instead. A plain HWND_TOP request loses
	// to a window the user is actively working in (Windows' own
	// foreground-lock behaviour); toggling this window TOPMOST and back
	// forces it to the front regardless, the standard trick for that.
	bringToFront(hwnd)

	dpi, _, _ := procGetDpiForWindow.Call(hwnd)
	if dpi == 0 {
		dpi = 96
	}
	scale := float64(dpi) / 96.0

	clientW := int32(math.Round(float64(cssWidth) * scale))
	clientH := int32(math.Round(float64(cssHeight) * scale))

	outerW, outerH := clientW, clientH
	if adjW, adjH, err := outerSizeForClient(hwnd, clientW, clientH, uint32(dpi)); err == nil {
		outerW, outerH = adjW, adjH
	} else {
		fmt.Printf("uicheck: could not compute window chrome size (%v); using client size %dx%d physical px directly\n", err, clientW, clientH)
	}

	requestedW, requestedH := outerW, outerH
	windowCapped = false
	if scr, err := screenRect(hwnd); err == nil {
		outerW, outerH, windowCapped = capToScreen(outerW, outerH, windowOriginX, windowOriginY, scr)
	} else {
		fmt.Printf("uicheck: could not read screen bounds to cap window size (%v)\n", err)
	}
	if windowCapped {
		fmt.Printf("uicheck: %dx%d CSS px at %.0f%% scale needs %dx%d physical px, which does not fit this screen; capping the window to %dx%d physical px\n",
			cssWidth, cssHeight, scale*100, requestedW, requestedH, outerW, outerH)
	}

	ret, _, err = procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(windowOriginX), uintptr(windowOriginY), uintptr(outerW), uintptr(outerH), swpShowWindow)
	if ret == 0 {
		return fmt.Errorf("SetWindowPos: %w", err)
	}
	bringToFront(hwnd)
	return nil
}

// outerSizeForClient returns the window (outer) size needed for hwnd's
// current style/exstyle to have a client area of clientW x clientH
// physical pixels at dpi, via AdjustWindowRectExForDpi (Windows 10 1607+,
// same release GetDpiForWindow needs).
func outerSizeForClient(hwnd uintptr, clientW, clientH int32, dpi uint32) (int32, int32, error) {
	gwlStyle := int32(-16)
	gwlExStyle := int32(-20)
	style, _, _ := procGetWindowLongW.Call(hwnd, uintptr(gwlStyle))
	exStyle, _, _ := procGetWindowLongW.Call(hwnd, uintptr(gwlExStyle))

	r := rect{0, 0, clientW, clientH}
	ret, _, err := procAdjustWindowRectExForDpi.Call(uintptr(unsafe.Pointer(&r)), style, 0, exStyle, uintptr(dpi))
	if ret == 0 {
		return 0, 0, fmt.Errorf("AdjustWindowRectExForDpi: %w", err)
	}
	return r.Right - r.Left, r.Bottom - r.Top, nil
}

// monitorInfo mirrors Win32's MONITORINFO.
type monitorInfo struct {
	cbSize    uint32
	rcMonitor rect
	rcWork    rect
	dwFlags   uint32
}

// screenRect returns the full bounds (not just the work area, matching
// "capped at the screen") of the monitor hwnd currently sits on.
func screenRect(hwnd uintptr) (rect, error) {
	const monitorDefaultToNearest = 2
	hMon, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultToNearest)
	if hMon == 0 {
		return rect{}, fmt.Errorf("MonitorFromWindow returned no monitor")
	}
	var mi monitorInfo
	mi.cbSize = uint32(unsafe.Sizeof(mi))
	ret, _, err := procGetMonitorInfoW.Call(hMon, uintptr(unsafe.Pointer(&mi)))
	if ret == 0 {
		return rect{}, fmt.Errorf("GetMonitorInfoW: %w", err)
	}
	return mi.rcMonitor, nil
}

func getWindowRect(hwnd uintptr) (rect, error) {
	var r rect
	ret, _, err := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	if ret == 0 {
		return r, fmt.Errorf("GetWindowRect: %w", err)
	}
	return r, nil
}

func clientOrigin(hwnd uintptr) (point, error) {
	p := point{0, 0}
	ret, _, err := procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&p)))
	if ret == 0 {
		return p, fmt.Errorf("ClientToScreen: %w", err)
	}
	return p, nil
}

// bringToFront activates the window (needed before a key press: keyboard
// input goes to whichever window has focus, unlike a mouse click, which
// lands on whatever window is physically under the cursor regardless of
// focus). Windows restricts a background process calling
// SetForegroundWindow on its own (the "foreground lock" behavior) unless it
// shares input state with whatever currently owns the foreground; since
// uicheck.exe is exactly such an unrelated background process, that call
// can silently do nothing on its own (ensureWindowSizeWH's own HWND_TOPMOST
// toggle only affects z-order, not this). AttachThreadInput briefly shares
// input state with the current foreground thread so SetForegroundWindow
// actually takes effect, the standard workaround for this restriction; this
// only started mattering once windowstate.go's F11 hook began gating on
// GetForegroundWindow()==hwnd for real F11 safety (found by review,
// 2026-09-25: check_d7.go started failing because this window was never
// actually foreground, despite every other check's clicks/keypresses
// working fine without needing true foreground status).
func bringToFront(hwnd uintptr) {
	var targetPID uint32
	targetTID, _, _ := procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&targetPID)))

	fgHwnd, _, _ := procGetForegroundWindow.Call()
	var fgPID uint32
	fgTID, _, _ := procGetWindowThreadProcessId.Call(fgHwnd, uintptr(unsafe.Pointer(&fgPID)))

	attached := fgTID != 0 && fgTID != targetTID
	if attached {
		procAttachThreadInput.Call(fgTID, targetTID, 1)
	}
	procSetForegroundWindow.Call(hwnd)
	if attached {
		procAttachThreadInput.Call(fgTID, targetTID, 0)
	}
	time.Sleep(150 * time.Millisecond)
}

// screenCoords turns a DOM element's CSS-pixel center (as returned by
// getBoundingClientRect) into absolute screen pixel coordinates
// SendInput/SetCursorPos expect. This host reports window.devicePixelRatio
// as the OS scale factor (2 at 200%), but actually renders content at
// System DPI awareness, not Per-Monitor: its CSS viewport is already
// numerically 1:1 with physical pixels (confirmed empirically: an
// element's rect.x routinely exceeds half the window's physical width,
// impossible if CSS pixels were really half of physical ones here), so no
// dpr multiplication belongs in this mapping, unlike a true
// per-monitor-DPI-aware page.
func screenCoords(hwnd uintptr, cssX, cssY float64) (int32, int32, error) {
	origin, err := clientOrigin(hwnd)
	if err != nil {
		return 0, 0, err
	}
	x := origin.X + int32(cssX)
	y := origin.Y + int32(cssY)
	return x, y, nil
}

const (
	inputMouse    = 0
	inputKeyboard = 1

	mouseeventfMove     = 0x0001
	mouseeventfLeftDown = 0x0002
	mouseeventfLeftUp   = 0x0004

	keyeventfKeyUp = 0x0002
)

// mouseInput mirrors Win32's MOUSEINPUT (x64 layout).
type mouseInput struct {
	dx, dy      int32
	mouseData   uint32
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

// keybdInput mirrors Win32's KEYBDINPUT (x64 layout).
type keybdInput struct {
	vk          uint16
	scan        uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

// input mirrors Win32's INPUT (x64 layout): a DWORD type tag, 4 bytes of
// padding (the union that follows is 8-byte aligned on x64), then the
// largest union member (MOUSEINPUT, 32 bytes) sized to fit either variant.
type input struct {
	typ  uint32
	_    uint32
	data [32]byte
}

func mouseInputRecord(flags uint32) input {
	mi := mouseInput{dwFlags: flags}
	var in input
	in.typ = inputMouse
	*(*mouseInput)(unsafe.Pointer(&in.data[0])) = mi
	return in
}

func keyInputRecord(vk uint16, up bool) input {
	flags := uint32(0)
	if up {
		flags = keyeventfKeyUp
	}
	ki := keybdInput{vk: vk, dwFlags: flags}
	var in input
	in.typ = inputKeyboard
	*(*keybdInput)(unsafe.Pointer(&in.data[0])) = ki
	return in
}

func sendInputs(inputs []input) error {
	if len(inputs) == 0 {
		return nil
	}
	ret, _, err := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(inputs[0]),
	)
	if ret != uintptr(len(inputs)) {
		return fmt.Errorf("SendInput: sent %d of %d: %w", ret, len(inputs), err)
	}
	return nil
}

// clickAt moves the real OS cursor to (x,y) and sends a real left button
// down+up, exactly as SendInput describes it: indistinguishable from a
// physical mouse click by anything downstream, including the page's own
// event listeners.
func clickAt(x, y int32) error {
	ret, _, err := procSetCursorPos.Call(uintptr(x), uintptr(y))
	if ret == 0 {
		return fmt.Errorf("SetCursorPos: %w", err)
	}
	time.Sleep(50 * time.Millisecond)
	return sendInputs([]input{
		mouseInputRecord(mouseeventfLeftDown),
		mouseInputRecord(mouseeventfLeftUp),
	})
}

// dragPath returns steps+1 points from (x1,y1) to (x2,y2), the first and the
// last exact, by linear interpolation with integer rounding. steps below 1 is 1.
func dragPath(x1, y1, x2, y2 int32, steps int) [][2]int32 {
	if steps < 1 {
		steps = 1
	}
	pts := make([][2]int32, 0, steps+1)
	for i := 0; i <= steps; i++ {
		switch i {
		case 0:
			pts = append(pts, [2]int32{x1, y1})
		case steps:
			pts = append(pts, [2]int32{x2, y2})
		default:
			f := float64(i) / float64(steps)
			pts = append(pts, [2]int32{
				x1 + int32(math.Round(float64(x2-x1)*f)),
				y1 + int32(math.Round(float64(y2-y1)*f)),
			})
		}
	}
	return pts
}

// dragAt presses the real left button at (x1,y1), moves the real cursor to
// (x2,y2) in 12 steps with SetCursorPos, and releases. The button-up is sent
// even when a move fails, so a failed drag never leaves the button held.
func dragAt(x1, y1, x2, y2 int32) error {
	pts := dragPath(x1, y1, x2, y2, 12)
	ret, _, err := procSetCursorPos.Call(uintptr(pts[0][0]), uintptr(pts[0][1]))
	if ret == 0 {
		return fmt.Errorf("SetCursorPos: %w", err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := sendInputs([]input{mouseInputRecord(mouseeventfLeftDown)}); err != nil {
		return err
	}
	var moveErr error
	for _, p := range pts[1:] {
		time.Sleep(20 * time.Millisecond)
		r, _, e := procSetCursorPos.Call(uintptr(p[0]), uintptr(p[1]))
		if r == 0 {
			moveErr = fmt.Errorf("SetCursorPos: %w", e)
			break
		}
	}
	// Hold on the last point so the page has handled every move before the release.
	time.Sleep(150 * time.Millisecond)
	upErr := sendInputs([]input{mouseInputRecord(mouseeventfLeftUp)})
	if moveErr != nil {
		return moveErr
	}
	return upErr
}

// VK_ESCAPE and VK_F11 (d7's fullscreen toggle, UI review patch section 10).
const (
	vkEscape = 0x1B
	vkF11    = 0x7A
)

func pressKey(vk uint16) error {
	return sendInputs([]input{
		keyInputRecord(vk, false),
		keyInputRecord(vk, true),
	})
}

// screenshot captures burnmon's whole window (title bar included, matching
// "a screenshot of the running window") by BitBlt-ing from the screen DC
// over the window's actual screen rect: this is what an ordinary
// screenshot tool does, and unlike PrintWindow (which asks the app to
// render itself into an off-screen buffer, unreliable against a
// GPU-composited WebView2 host under load) it just samples whatever DWM
// has already composited on screen, so it needs the window to actually be
// visible/unoccluded, which bringToFront/ensureWindowSize already ensure.
// Saved as testdata\uicheck\out\<name>.png, and also returned so a caller
// (W1) can inspect pixels directly.
func screenshot(hwnd uintptr, name string) (image.Image, error) {
	r, err := getWindowRect(hwnd)
	if err != nil {
		return nil, err
	}
	width := int(r.Right - r.Left)
	height := int(r.Bottom - r.Top)
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("window has non-positive size %dx%d", width, height)
	}

	hdcScreen, _, _ := procGetDC.Call(0)
	if hdcScreen == 0 {
		return nil, fmt.Errorf("GetDC(0) failed")
	}
	defer procReleaseDC.Call(0, hdcScreen)

	hdcMem, _, _ := procCreateCompatDC.Call(hdcScreen)
	if hdcMem == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(hdcMem)

	hBitmap, _, _ := procCreateCompatBmp.Call(hdcScreen, uintptr(width), uintptr(height))
	if hBitmap == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap failed")
	}
	defer procDeleteObject.Call(hBitmap)

	procSelectObject.Call(hdcMem, hBitmap)

	const srccopy = 0x00CC0020
	ret, _, err := procBitBlt.Call(hdcMem, 0, 0, uintptr(width), uintptr(height), hdcScreen, uintptr(r.Left), uintptr(r.Top), srccopy)
	if ret == 0 {
		return nil, fmt.Errorf("BitBlt: %w", err)
	}

	img, err := bitmapToImage(hdcMem, hBitmap, width, height)
	if err != nil {
		return nil, err
	}

	dir := filepath.Join("..", "..", "testdata", "uicheck", "out")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, name+".png")
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return nil, err
	}
	fmt.Println("uicheck: saved", path)
	return img, nil
}

// looksBlankWhite samples a grid across img and reports whether almost all
// of it is very light: the default, unpainted look of a Win32 window before
// WebView2 has navigated anywhere, as opposed to the startup page's own
// dark background (#1f2733).
func looksBlankWhite(img image.Image) bool {
	b := img.Bounds()
	const grid = 24
	total, light := 0, 0
	for i := 0; i < grid; i++ {
		for j := 0; j < grid; j++ {
			x := b.Min.X + (b.Dx() * i / grid)
			y := b.Min.Y + (b.Dy() * j / grid)
			r, g, bl, _ := img.At(x, y).RGBA()
			total++
			if r>>8 > 240 && g>>8 > 240 && bl>>8 > 240 {
				light++
			}
		}
	}
	return total > 0 && float64(light)/float64(total) > 0.7
}

// bitmapInfoHeader mirrors Win32's BITMAPINFOHEADER.
type bitmapInfoHeader struct {
	size          uint32
	width         int32
	height        int32
	planes        uint16
	bitCount      uint16
	compression   uint32
	sizeImage     uint32
	xPelsPerMeter int32
	yPelsPerMeter int32
	clrUsed       uint32
	clrImportant  uint32
}

func bitmapToImage(hdcMem, hBitmap uintptr, width, height int) (image.Image, error) {
	hdr := bitmapInfoHeader{
		width:    int32(width),
		height:   -int32(height), // negative: top-down DIB, matches screen row order
		planes:   1,
		bitCount: 32,
	}
	hdr.size = uint32(unsafe.Sizeof(hdr))

	buf := make([]byte, width*height*4)
	ret, _, err := procGetDIBits.Call(
		hdcMem,
		hBitmap,
		0,
		uintptr(height),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&hdr)),
		0, // DIB_RGB_COLORS
	)
	if ret == 0 {
		return nil, fmt.Errorf("GetDIBits: %w", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			i := (row*width + col) * 4
			b, g, r, _ := buf[i], buf[i+1], buf[i+2], buf[i+3]
			img.SetRGBA(col, row, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img, nil
}

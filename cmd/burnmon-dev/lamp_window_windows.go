//go:build windows

package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"burnmon/internal/fsutil"
	"burnmon/internal/instance"
	"burnmon/internal/lamp"
	"burnmon/internal/snapshot"
	"burnmon/internal/winbind"
)

var (
	gdi32Lamp               = windows.NewLazySystemDLL("gdi32.dll")
	procSetWindowRgn        = user32.NewProc("SetWindowRgn")
	procGetCursorPosLmp     = user32.NewProc("GetCursorPos")
	procGetAsyncKeyStateLmp = user32.NewProc("GetAsyncKeyState")
	procEnumMonitorsLmp     = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoLmp   = user32.NewProc("GetMonitorInfoW")
	procGetDpiForMonitorLmp = windows.NewLazySystemDLL("shcore.dll").NewProc("GetDpiForMonitor")
	procGetWindowRectLmp    = user32.NewProc("GetWindowRect")
	procGetDpiForWindow     = user32.NewProc("GetDpiForWindow")
	procGetSystemMetricsLmp = user32.NewProc("GetSystemMetrics")
	procFindWindowWLamp     = user32.NewProc("FindWindowW")
	procCreateRectRgn       = gdi32Lamp.NewProc("CreateRectRgn")
	procCombineRgn          = gdi32Lamp.NewProc("CombineRgn")
	procDeleteObject        = gdi32Lamp.NewProc("DeleteObject")
	procGetLastInputInfo    = user32.NewProc("GetLastInputInfo")
	procGetTickCountLamp    = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetTickCount")
	procDwmExtendFrame      = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmExtendFrameIntoClientArea")
)

const (
	lampGwlExStyle     = ^uintptr(19) // GWL_EXSTYLE (-20), sign-extended
	lampWsPopup        = 0x80000000
	lampWsVisible      = 0x10000000
	lampWsExToolWindow = 0x00000080
	lampWsExAppWindow  = 0x00040000
	lampWsExNoActivate = 0x08000000
	lampSwHide         = 0
	lampSwpNoActivate  = 0x0010
	lampSwpFrameChange = 0x0020
	lampSwpShowWindow  = 0x0040
	lampSwpNoSize      = 0x0001
	lampSmCxScreen     = 0
	lampVkLButton      = 0x01
	lampTopmost        = ^uintptr(0) // HWND_TOPMOST (-1)
	lampRgnOr          = 2           // RGN_OR
)

// runLampWindow opens the lamp window and runs until it is closed. A second
// lamp process (or a second demo) exits at once with 0.
func runLampWindow(o lampOpts) int {
	name, profile := lampMutexName, ""
	var setDir, snapPath string
	if o.demo {
		name = lampDemoMutexName
		// The demo leaves nothing in a BurnMon folder: its browser profile
		// lives under the temp folder.
		profile = filepath.Join(os.TempDir(), "burnmon-lamp-demo")
	} else {
		setDir = settingsDir()
		profile = filepath.Join(setDir, "wv2-lamp")
		p, err := snapshot.DefaultPath()
		if err != nil {
			log.Println("lamp: snapshot path:", err)
			return 1
		}
		snapPath = p
	}
	if instance.AlreadyRunning(name) {
		return 0
	}
	_ = os.MkdirAll(profile, 0o755)
	// Same reason as window_windows.go: the loader reads the folder from the
	// process environment, not from Go memory.
	_ = os.Setenv("WEBVIEW2_USER_DATA_FOLDER", profile)

	set := lamp.DefaultSettings()
	if !o.demo {
		set = readLampSettings(setDir)
	}
	// The window always starts clear: the WebView2 default background is fully
	// transparent (the runtime reads this variable when it makes the controller)
	// and the host window gets DWM glass below. Built, not seen. lamp_backdrop
	// card is the page's own panel, so a change needs no restart.
	k, v := lampStartBackdrop()
	_ = os.Setenv(k, v)
	glass := true
	startW, startH := lampWindowSize(set)

	prev, _, _ := procGetForegroundWindow.Call()
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath: profile,
		WindowOptions: webview2.WindowOptions{
			Title: "Glim", Width: uint(startW), Height: uint(startH), IconId: 1,
		},
	})
	if w == nil {
		log.Println("lamp: could not create the WebView2 window; the lamp needs the WebView2 runtime")
		return 1
	}
	defer w.Destroy()
	hwnd := uintptr(w.Window())
	lampStyleWindow(hwnd, glass)
	// go-webview2 shows the window and may activate it while it is made. Give
	// the focus back to the program the person was typing in, if Windows lets
	// us (best effort, not proven).
	if prev != 0 && prev != hwnd {
		procSetForegroundWindow.Call(prev)
	}

	lampIdle = lampIdleSeconds
	lampMonitors = lampReadMonitors
	lampCursor = lampReadCursor
	e := newLampEngine(o.demo, setDir, snapPath)
	if !o.demo {
		// The ledger of the waits (lane scope X2): written at most once a
		// minute and when the window closes. The demo has none.
		e.ledger = lamp.LoadLedger(filepath.Join(setDir, lamp.LedgerFile))
		defer func() { e.ledger.Close(time.Now()) }()
	}
	pl := &lampPlacer{hwnd: hwnd, setDir: setDir, demo: o.demo, w: startW, h: startH}
	pl.place()

	bind := func(n string, f interface{}) {
		if err := w.Bind(n, f); err != nil {
			log.Println("lamp: could not bind", n, ":", err)
		}
	}
	bind("lampState", func() lampPayload { return e.payload(time.Now()) })
	bind("lampGlim", func(mood string, bricks int, theme string) string { return lamp.Glim(mood, bricks, theme) })
	bind("lampNextLine", func() string { return e.nextLine() })
	bind("lampOpenDev", func() { lampOpenDev() })
	// The window area (ticker, Glim's box, an act's extra box) becomes the
	// window region: outside it nothing is drawn and clicks go to the windows below.
	bind("lampArea", func(area string) {
		pl.setArea(area)
		w.Dispatch(pl.place)
	})
	bind("lampSize", func(cssW, cssH int) {
		pl.setSize(cssW, cssH)
	})
	// A pointer down on the ticker that moved more than 6 px: the lamp process
	// follows the pointer until the button is up, then snaps and saves.
	bind("lampDragStart", func() {
		if !o.demo {
			go pl.drag(func() { w.Dispatch(pl.place) })
		}
	})
	// A grab and swing of Glim ends or reaches the stop: the line to show and
	// whether the grab was too hard (the 60 s memory lives in the engine).
	bind("lampSwing", func(maxDeg float64, changes int) lampSwingResult {
		return e.swing(maxDeg, changes, time.Now())
	})
	// "Close the lamp" from the menu: ledger first, lamp_enabled false only for
	// a lamp BurnMon Dev started, then the lamp ends.
	bind("lampClose", func() {
		go lampCloseNow(func() { e.ledger.Close(time.Now()) }, os.Getenv(lampParentEnv) != "",
			func(key string, v any) error {
				if o.demo {
					return nil
				}
				return saveDevConfigKey(setDir, key, v)
			}, func() { lampEnd(w.Dispatch, w.Terminate, os.Exit, lampEndGrace) })
	})
	// Dev-only: tools\uicheck drives the lamp page through this channel. It
	// opens only when BURNMON_LAMP_UICHECK is set; a release never sets it.
	winbind.StartUICheckServer(w, winbind.UICheckOptions{
		Env:     "BURNMON_LAMP_UICHECK",
		Addr:    "127.0.0.1:9336",
		Binding: "lampUICheckResult",
		Page:    func() string { return lampHTML },
	})
	w.SetHtml(lampHTML)

	// A change of lamp_position in the gear dialog moves the lamp within 2 s.
	// The lamp also ends when the BurnMon Dev that started it is gone. A lamp
	// started by hand has no BURNMON_LAMP_PARENT and stays.
	parent := os.Getenv(lampParentEnv)
	go func() {
		for range time.Tick(2 * time.Second) {
			if lampParentGone(parent, lampProcessAlive) {
				e.ledger.Close(time.Now()) // the exit after the grace time skips the deferred write
				lampEnd(w.Dispatch, w.Terminate, os.Exit, lampEndGrace)
				return
			}
			w.Dispatch(pl.place)
		}
	}()
	// The eyes: 10 times a second, only while Glim is down and the motion is
	// lively, the pointer position goes to the page as a position in CSS pixels
	// and is stored nowhere. The page's lampPointer may be called with a point
	// far outside the window, on any monitor.
	go func() {
		for range time.Tick(100 * time.Millisecond) {
			if !e.eyesOn() {
				continue
			}
			px, py, ok := lampCursor()
			if !ok {
				continue
			}
			w.Dispatch(func() {
				var rc windows.Rect
				procGetWindowRectLmp.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
				x, y := lampPointerCSS(px, py, int(rc.Left), int(rc.Top), lampWindowScale(hwnd))
				w.Eval(fmt.Sprintf("window.lampPointer&&window.lampPointer(%.1f,%.1f)", x, y))
			})
		}
	}()
	w.Run()
	return 0
}

// lampProcessAlive reports whether process pid still runs. A process that
// cannot be opened is gone, except when access is denied: that one is alive.
// An opened one runs while its exit code is 259 (STILL_ACTIVE).
func lampProcessAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return lampAliveFrom(false, errors.Is(err, windows.ERROR_ACCESS_DENIED), 0)
	}
	defer windows.CloseHandle(h)
	var code uint32
	if windows.GetExitCodeProcess(h, &code) != nil {
		return false
	}
	return lampAliveFrom(true, false, code)
}

// lampWindowScale is the scale of the monitor the window is on (1 is 96 dpi).
func lampWindowScale(hwnd uintptr) float64 {
	if dpi, _, _ := procGetDpiForWindow.Call(hwnd); dpi > 0 {
		return float64(dpi) / 96
	}
	return 1
}

// lampReadCursor is the pointer position in device pixels of the whole desktop.
// It reads and stores nothing.
func lampReadCursor() (x, y int, ok bool) {
	var pt struct{ x, y int32 }
	if r, _, _ := procGetCursorPosLmp.Call(uintptr(unsafe.Pointer(&pt))); r == 0 {
		return 0, 0, false
	}
	return int(pt.x), int(pt.y), true
}

// lampMonitorCB is the one EnumDisplayMonitors callback of the process. Go
// never frees a callback made by windows.NewCallback and allows about 2000, so
// it is made once; it fills lampMonitorOut under lampMonitorMu.
var (
	lampMonitorMu  sync.Mutex
	lampMonitorOut []lampMonitor
	lampMonitorCB  = windows.NewCallback(func(hMon, _, _, _ uintptr) uintptr {
		var mi struct {
			size    uint32
			monitor windows.Rect
			work    windows.Rect
			flags   uint32
			device  [32]uint16
		}
		mi.size = uint32(unsafe.Sizeof(mi))
		if r, _, _ := procGetMonitorInfoLmp.Call(hMon, uintptr(unsafe.Pointer(&mi))); r == 0 {
			return 1
		}
		scale := 1.0
		var dx, dy uint32
		if r, _, _ := procGetDpiForMonitorLmp.Call(hMon, 0, uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy))); r == 0 && dx > 0 {
			scale = float64(dx) / 96
		}
		lampMonitorOut = append(lampMonitorOut, lampMonitor{
			Name: windows.UTF16ToString(mi.device[:]),
			X:    int(mi.monitor.Left), Y: int(mi.monitor.Top),
			W: int(mi.monitor.Right - mi.monitor.Left), H: int(mi.monitor.Bottom - mi.monitor.Top),
			Primary: mi.flags&1 != 0, // MONITORINFOF_PRIMARY
			Scale:   scale,
		})
		return 1
	})
)

// lampReadMonitors lists the connected monitors with their device names.
func lampReadMonitors() []lampMonitor {
	lampMonitorMu.Lock()
	defer lampMonitorMu.Unlock()
	lampMonitorOut = nil
	procEnumMonitorsLmp.Call(0, 0, lampMonitorCB, 0)
	return lampMonitorOut
}

// lampStyleWindow removes the caption and border, takes the window out of
// the taskbar and marks it never-activate. It uses only the window handle.
func lampStyleWindow(hwnd uintptr, glass bool) {
	style, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlStyle)
	style = (style &^ uintptr(wsOverlappedWindowFrame)) | lampWsPopup | lampWsVisible
	procSetWindowLongPtrW.Call(hwnd, gwlStyle, style)
	ex, _, _ := procGetWindowLongPtrW.Call(hwnd, lampGwlExStyle)
	ex = (ex &^ uintptr(lampWsExAppWindow)) | lampWsExToolWindow | lampWsExNoActivate
	procSetWindowLongPtrW.Call(hwnd, lampGwlExStyle, ex)
	if glass {
		// DWM composes the whole client area with its alpha, so the transparent
		// pixels of the web view show the windows below.
		margins := [4]int32{-1, -1, -1, -1}
		procDwmExtendFrame.Call(hwnd, uintptr(unsafe.Pointer(&margins[0])))
	}
	// A tool window is taken out of the taskbar only when it is shown again.
	procShowWindowWS.Call(hwnd, lampSwHide)
}

// lampChildAttr is nil: the lamp child is not started hidden. HideWindow puts
// SW_HIDE in the child's start-up info, Windows applies it to the first
// ShowWindow call of the process, and go-webview2 makes the web view under a
// window that call hid. The window of a lamp that BurnMon Dev started stayed
// invisible. Not taking the focus is the lamp's own best effort.
func lampChildAttr() *syscall.SysProcAttr { return nil }

// lampIdleSeconds is the seconds since the last keyboard or mouse input on
// this computer. It reads and stores nothing.
func lampIdleSeconds() int {
	info := struct{ size, time uint32 }{size: 8}
	if r, _, _ := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		return 0
	}
	now, _, _ := procGetTickCountLamp.Call()
	return lampIdleFrom(uint32(now), info.time)
}

// lampPlacer puts the window at the top edge of the monitor in lamp_monitor
// (the primary one when it is empty or not connected). Sizes come from the page
// in CSS pixels and are multiplied by the window's scale.
type lampPlacer struct {
	hwnd   uintptr
	setDir string
	demo   bool

	dragging atomic.Bool // a drag loop runs

	mu         sync.Mutex
	w, h       int
	appX, appW int // what the window was last set to, so place does nothing when nothing changed
	appY       int
	appH       int
	area       string // the last lampArea from the page, JSON
	rgnKey     string // area, size and scale of the region last set
}

// setArea remembers the rectangles the page asked the window to keep.
func (p *lampPlacer) setArea(area string) {
	p.mu.Lock()
	p.area = area
	p.mu.Unlock()
}

func (p *lampPlacer) setSize(cssW, cssH int) {
	if cssW < 1 || cssH < 1 || cssW > 2000 || cssH > 2000 {
		return
	}
	p.mu.Lock()
	p.w, p.h = cssW, cssH
	p.mu.Unlock()
	p.place()
}

// settings are the lamp keys now: read from the file each time, so a change
// in the gear dialog reaches the window within the next tick. The demo has none.
func (p *lampPlacer) settings() lamp.Settings {
	if p.demo {
		return lamp.DefaultSettings()
	}
	return readLampSettings(p.setDir)
}

// place sizes and positions the window without activating it and keeps it on
// top. While a drag runs the drag loop owns the position.
func (p *lampPlacer) place() {
	if p.dragging.Load() {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	cssW, cssH := p.w, p.h
	scale := lampWindowScale(p.hwnd)
	w, h := int(float64(cssW)*scale+0.5), int(float64(cssH)*scale+0.5)
	set := p.settings()
	var x, y int
	if mons := lampMonitors(); len(mons) > 0 {
		x, y, _ = lampPlaceOn(mons, set.Monitor, set.Position, w)
	} else {
		screenW, _, _ := procGetSystemMetricsLmp.Call(lampSmCxScreen)
		x = (int(screenW) - w) / 2
		switch set.Position {
		case "top-left":
			x = 0
		case "top-right":
			x = int(screenW) - w
		}
	}
	if x != p.appX || y != p.appY || w != p.appW || h != p.appH {
		p.appX, p.appY, p.appW, p.appH = x, y, w, h
		procSetWindowPosWS.Call(p.hwnd, lampTopmost,
			uintptr(uint32(int32(x))), uintptr(uint32(int32(y))), uintptr(uint32(int32(w))), uintptr(uint32(int32(h))),
			uintptr(lampSwpNoActivate|lampSwpFrameChange|lampSwpShowWindow))
	}
	key := fmt.Sprintf("%s|%d|%d|%.2f", p.area, w, h, scale)
	if key == p.rgnKey {
		return
	}
	p.rgnKey = key
	p.setRegion(w, h, scale)
}

// setRegion keeps only the window area: the ticker, Glim's box, an act's extra
// box and the open menu, as plain rectangles each rounded outward, so every
// border of the ticker lies inside it. The rounded corners come from the page's
// CSS only: a round region cut the side borders. Without a usable area the whole
// window rectangle is kept. Clicks outside the region go to the windows
// below. Built, not seen.
func (p *lampPlacer) setRegion(w, h int, scale float64) {
	rects := lampRegionRects(p.area, scale)
	if len(rects) == 0 {
		rects = []lampRect{{0, 0, w, h}}
	}
	rectRgn := func(r lampRect) uintptr {
		rgn, _, _ := procCreateRectRgn.Call(uintptr(uint32(int32(r.X))), uintptr(uint32(int32(r.Y))),
			uintptr(uint32(int32(r.X+r.W))), uintptr(uint32(int32(r.Y+r.H))))
		return rgn
	}
	rgn := rectRgn(rects[0])
	for _, r := range rects[1:] {
		part := rectRgn(r)
		if part != 0 && rgn != 0 {
			procCombineRgn.Call(rgn, rgn, part, lampRgnOr)
		}
		if part != 0 {
			procDeleteObject.Call(part)
		}
	}
	if rgn != 0 {
		procSetWindowRgn.Call(p.hwnd, rgn, 1) // the window owns the region now
	}
}

// drag follows the pointer with the window until the left button is up, then
// snaps the lamp to the top edge of the monitor under the pointer at left,
// centre or right, and saves lamp_monitor and lamp_position the way BurnMon Dev
// saves a key. The primary monitor is saved as the empty name, so a lamp on
// the primary follows it. The drag never takes the focus (SWP_NOACTIVATE). The
// loop runs only during a drag. done runs after the drop, on the window thread.
func (p *lampPlacer) drag(done func()) {
	if !p.dragging.CompareAndSwap(false, true) {
		return
	}
	var rc windows.Rect
	px, py, ok := lampCursor()
	if !ok {
		p.dragging.Store(false)
		return
	}
	procGetWindowRectLmp.Call(p.hwnd, uintptr(unsafe.Pointer(&rc)))
	offX, offY := px-int(rc.Left), py-int(rc.Top)
	for lampButtonDown() {
		if cx, cy, ok := lampCursor(); ok {
			x, y := lampDragPos(cx, cy, offX, offY)
			procSetWindowPosWS.Call(p.hwnd, lampTopmost,
				uintptr(uint32(int32(x))), uintptr(uint32(int32(y))), 0, 0,
				uintptr(lampSwpNoActivate|lampSwpNoSize))
		}
		time.Sleep(16 * time.Millisecond)
	}
	if x, y, ok := lampCursor(); ok {
		_, _, err := lampDrop(lampMonitors(), x, y, int(rc.Right-rc.Left), func(key string, v any) error {
			return saveDevConfigKey(p.setDir, key, v)
		})
		if err != nil {
			log.Println("lamp: could not save the drop:", err)
		}
	}
	p.mu.Lock()
	p.appW = 0 // force the next place to set the position again
	p.mu.Unlock()
	p.dragging.Store(false)
	if done != nil {
		done()
	}
}

// lampButtonDown is whether the left mouse button is down.
func lampButtonDown() bool {
	r, _, _ := procGetAsyncKeyStateLmp.Call(lampVkLButton)
	return uint16(r)&0x8000 != 0
}

// lampOpenDev brings the BurnMon Dev window to the front, or starts BurnMon
// Dev (this exe, no arguments, no shell) when no such window exists. It never
// takes BurnMon Dev's single-instance mutex.
func lampOpenDev() {
	title, err := windows.UTF16PtrFromString(windowTitle)
	if err != nil {
		return
	}
	if hwnd, _, _ := procFindWindowWLamp.Call(0, uintptr(unsafe.Pointer(title))); hwnd != 0 {
		bringToFront(hwnd)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		log.Println("lamp: own exe path:", err)
		return
	}
	cmd := exec.Command(exe)
	// BurnMon Dev must not inherit the lamp's transparent background.
	cmd.Env = lampWithoutEnv(os.Environ(), lampBackdropVar)
	if err := cmd.Start(); err != nil {
		log.Println("lamp: could not start BurnMon Dev:", err)
		return
	}
	_ = cmd.Process.Release()
}

// runCardWindow shows the story card with one button, Save PNG, that writes
// both cards as PNG into dir next to the SVG files (the page draws them on a
// canvas). It is a plain window with its own browser profile under the temp
// folder, so it leaves nothing in a BurnMon folder. Built, not seen.
func runCardWindow(dir string, wk lamp.Week, lang string) int {
	storyName, wideName := lamp.CardFileNames(wk)
	cards := lampCards{
		Story: lamp.CardStory(wk, lang), Wide: lamp.CardWide(wk, lang),
		StoryPNG: strings.TrimSuffix(storyName, ".svg") + ".png",
		WidePNG:  strings.TrimSuffix(wideName, ".svg") + ".png",
	}
	profile := filepath.Join(os.TempDir(), "burnmon-lamp-card")
	_ = os.MkdirAll(profile, 0o755)
	_ = os.Setenv("WEBVIEW2_USER_DATA_FOLDER", profile)
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:      profile,
		WindowOptions: webview2.WindowOptions{Title: "Glim week card", Width: 540, Height: 960, IconId: 1},
	})
	if w == nil {
		log.Println("lamp: could not create the WebView2 window; the card window needs the WebView2 runtime")
		return 1
	}
	defer w.Destroy()
	if err := w.Bind("lampCardSvgs", func() lampCards { return cards }); err != nil {
		log.Println("lamp: could not bind lampCardSvgs:", err)
	}
	if err := w.Bind("lampCardSavePNG", func(name, b64 string) error { return saveCardPNG(dir, cards, name, b64) }); err != nil {
		log.Println("lamp: could not bind lampCardSavePNG:", err)
	}
	w.SetHtml(lampCardHTML)
	w.Run()
	return 0
}

// saveCardPNG writes one PNG the page made, but only under one of the two
// names the cards have, so the page cannot write any other file.
func saveCardPNG(dir string, c lampCards, name, b64 string) error {
	if name != c.StoryPNG && name != c.WidePNG {
		return errors.New("not a card file name")
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(filepath.Join(dir, name), raw, 0o644)
}

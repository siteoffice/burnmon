//go:build windows

package main

import (
	"encoding/base64"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"burnmon/internal/fsutil"
	"burnmon/internal/instance"
	"burnmon/internal/lamp"
	"burnmon/internal/snapshot"
)

var (
	gdi32Lamp               = windows.NewLazySystemDLL("gdi32.dll")
	procCreateRoundRectRgn  = gdi32Lamp.NewProc("CreateRoundRectRgn")
	procSetWindowRgn        = user32.NewProc("SetWindowRgn")
	procGetDpiForWindow     = user32.NewProc("GetDpiForWindow")
	procGetSystemMetricsLmp = user32.NewProc("GetSystemMetrics")
	procFindWindowWLamp     = user32.NewProc("FindWindowW")
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
	lampSmCxScreen     = 0
	lampCornerCSS      = 8
	lampTopmost        = ^uintptr(0) // HWND_TOPMOST (-1)
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

	prev, _, _ := procGetForegroundWindow.Call()
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath: profile,
		WindowOptions: webview2.WindowOptions{
			Title: "Glim", Width: 320, Height: 40, IconId: 1,
		},
	})
	if w == nil {
		log.Println("lamp: could not create the WebView2 window; the lamp needs the WebView2 runtime")
		return 1
	}
	defer w.Destroy()
	hwnd := uintptr(w.Window())
	lampStyleWindow(hwnd)
	// go-webview2 shows the window and may activate it while it is made. Give
	// the focus back to the program the person was typing in, if Windows lets
	// us (best effort, not proven).
	if prev != 0 && prev != hwnd {
		procSetForegroundWindow.Call(prev)
	}

	e := newLampEngine(o.demo, setDir, snapPath)
	if !o.demo {
		// The ledger of the waits (lane scope X2): written at most once a
		// minute and when the window closes. The demo has none.
		e.ledger = lamp.LoadLedger(filepath.Join(setDir, lamp.LedgerFile))
		defer func() { e.ledger.Close(time.Now()) }()
	}
	pl := &lampPlacer{hwnd: hwnd, setDir: setDir, demo: o.demo, w: 320, h: 40}
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
	bind("lampSize", func(cssW, cssH int) {
		pl.setSize(cssW, cssH)
	})
	w.SetHtml(lampHTML)

	// A change of lamp_position in the gear dialog moves the lamp within 2 s.
	go func() {
		for range time.Tick(2 * time.Second) {
			w.Dispatch(pl.place)
		}
	}()
	w.Run()
	return 0
}

// lampStyleWindow removes the caption and border, takes the window out of
// the taskbar and marks it never-activate. It uses only the window handle.
func lampStyleWindow(hwnd uintptr) {
	style, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlStyle)
	style = (style &^ uintptr(wsOverlappedWindowFrame)) | lampWsPopup | lampWsVisible
	procSetWindowLongPtrW.Call(hwnd, gwlStyle, style)
	ex, _, _ := procGetWindowLongPtrW.Call(hwnd, lampGwlExStyle)
	ex = (ex &^ uintptr(lampWsExAppWindow)) | lampWsExToolWindow | lampWsExNoActivate
	procSetWindowLongPtrW.Call(hwnd, lampGwlExStyle, ex)
	// A tool window is taken out of the taskbar only when it is shown again.
	procShowWindowWS.Call(hwnd, lampSwHide)
}

// lampPlacer puts the window at the top edge of the primary monitor. Sizes
// come from the page in CSS pixels and are multiplied by the window's scale.
type lampPlacer struct {
	hwnd   uintptr
	setDir string
	demo   bool

	mu         sync.Mutex
	w, h       int
	appX, appW int // what the window was last set to, so place does nothing when nothing changed
	appH       int
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

func (p *lampPlacer) position() string {
	if p.demo {
		return "top-center"
	}
	if b, err := os.ReadFile(devConfigReadPath(p.setDir)); err == nil {
		return lamp.ParseSettings(b).Position
	}
	return lamp.DefaultSettings().Position
}

// place sizes and positions the window without activating it, rounds its
// bottom corners and keeps it on top.
func (p *lampPlacer) place() {
	p.mu.Lock()
	defer p.mu.Unlock()
	cssW, cssH := p.w, p.h
	scale := 1.0
	if dpi, _, _ := procGetDpiForWindow.Call(p.hwnd); dpi > 0 {
		scale = float64(dpi) / 96
	}
	w, h := int(float64(cssW)*scale+0.5), int(float64(cssH)*scale+0.5)
	screenW, _, _ := procGetSystemMetricsLmp.Call(lampSmCxScreen)
	x := (int(screenW) - w) / 2
	switch p.position() {
	case "top-left":
		x = 0
	case "top-right":
		x = int(screenW) - w
	}
	if x == p.appX && w == p.appW && h == p.appH {
		return
	}
	p.appX, p.appW, p.appH = x, w, h
	procSetWindowPosWS.Call(p.hwnd, lampTopmost,
		uintptr(uint32(int32(x))), 0, uintptr(uint32(int32(w))), uintptr(uint32(int32(h))),
		uintptr(lampSwpNoActivate|lampSwpFrameChange|lampSwpShowWindow))
	// The round rectangle reaches above the window, so only the bottom
	// corners are rounded.
	r := min(int(lampCornerCSS*scale+0.5), h)
	rgn, _, _ := procCreateRoundRectRgn.Call(0, uintptr(uint32(int32(-r))), uintptr(w+1), uintptr(h+1), uintptr(2*r), uintptr(2*r))
	if rgn != 0 {
		procSetWindowRgn.Call(p.hwnd, rgn, 1) // the window owns the region now
	}
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

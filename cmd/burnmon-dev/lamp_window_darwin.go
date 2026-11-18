//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework QuartzCore -framework WebKit -framework CoreGraphics
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <CoreGraphics/CoreGraphics.h>
#import <QuartzCore/QuartzCore.h>
#import <objc/runtime.h>

static BOOL lampNever(id self, SEL _cmd) { return NO; }

// BOOL is a signed char on Intel and a bool on arm64; the method type string
// must say which.
#if defined(__arm64__)
#define LAMP_BOOL_METHOD "B@:"
#else
#define LAMP_BOOL_METHOD "c@:"
#endif

// lampFindWebView returns the WKWebView inside v (v itself or a subview).
static WKWebView *lampFindWebView(NSView *v) {
	if ([v isKindOfClass:[WKWebView class]]) return (WKWebView *)v;
	for (NSView *s in [v subviews]) {
		WKWebView *f = lampFindWebView(s);
		if (f != nil) return f;
	}
	return nil;
}

// lampIdleC is the seconds since the last keyboard or mouse event of this
// login session. It reads and stores nothing.
static double lampIdleC(void) {
	return CGEventSourceSecondsSinceLastEventType(kCGEventSourceStateCombinedSessionState, (CGEventType)~0);
}

// lampStyle turns the webview's window into the lamp panel: no title bar,
// above normal windows, on every Space, never the key or main window, no Dock
// icon. clear is 1 for the clear backdrop: a clear panel with no shadow and a
// clear web view background (underPageBackgroundColor, documented, macOS 12 and
// later), the page draws the ticker and Glim on it. clear is 0 for the card
// backdrop: an opaque panel with its bottom corners rounded. It runs on the
// main thread.
static void lampStyle(void *win, int clear) {
	NSWindow *w = (NSWindow *)win;
	static Class cls = nil;
	if (cls == nil) {
		cls = objc_allocateClassPair([w class], "BurnMonLampPanel", 0);
		class_addMethod(cls, @selector(canBecomeKeyWindow), (IMP)lampNever, LAMP_BOOL_METHOD);
		class_addMethod(cls, @selector(canBecomeMainWindow), (IMP)lampNever, LAMP_BOOL_METHOD);
		objc_registerClassPair(cls);
	}
	object_setClass(w, cls);
	[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
	[w setStyleMask:NSWindowStyleMaskBorderless];
	[w setLevel:NSStatusWindowLevel];
	[w setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces |
		NSWindowCollectionBehaviorStationary | NSWindowCollectionBehaviorIgnoresCycle |
		NSWindowCollectionBehaviorFullScreenAuxiliary];
	[w setHidesOnDeactivate:NO];
	if (clear) {
		[w setOpaque:NO];
		[w setBackgroundColor:[NSColor clearColor]];
		[w setHasShadow:NO];
		WKWebView *wk = lampFindWebView([w contentView]);
		if (wk != nil) {
			if (@available(macOS 12.0, *)) {
				wk.underPageBackgroundColor = [NSColor clearColor];
			}
		}
		return;
	}
	[w setOpaque:YES];
	[w setBackgroundColor:[NSColor windowBackgroundColor]];
	[w setHasShadow:YES];
	NSView *v = [w contentView];
	[v setWantsLayer:YES];
	v.layer.cornerRadius = 8;
	v.layer.maskedCorners = kCALayerMinXMinYCorner | kCALayerMaxXMinYCorner;
	v.layer.masksToBounds = YES;
}

// lampPlace sizes the panel and puts it at the top edge of the main screen
// (the one with the menu bar), below the menu bar. pos: 0 centre, 1 left,
// 2 right. orderFrontRegardless shows it without making it the key window.
static void lampPlace(void *win, double w, double h, int pos) {
	NSWindow *win_ = (NSWindow *)win;
	NSScreen *s = [[NSScreen screens] objectAtIndex:0];
	NSRect f = [s visibleFrame];
	double x = f.origin.x + (f.size.width - w) / 2;
	if (pos == 1) x = f.origin.x;
	if (pos == 2) x = f.origin.x + f.size.width - w;
	double y = f.origin.y + f.size.height - h;
	[win_ setFrame:NSMakeRect(x, y, w, h) display:YES];
	[win_ orderFrontRegardless];
}
*/
import "C"

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"

	webview "github.com/webview/webview_go"

	"burnmon/internal/lamp"
	"burnmon/internal/snapshot"
)

// lampLockHeld keeps the locked folder open for the life of the process;
// closing the descriptor releases the flock.
var lampLockHeld *os.File

// lampAlreadyRunning is the single-instance check of the Mac lamp (plan C7):
// a flock on the data folder itself, opened read-only, so the lamp writes no
// lock file. The demo locks the temp folder, so it runs next to a real lamp.
// The names lampMutexName and lampDemoMutexName are the Windows form of the
// same two instances.
func lampAlreadyRunning(demo bool) bool {
	dir := appDataDir()
	if demo {
		dir = os.TempDir()
	}
	f, err := os.Open(dir)
	if err != nil {
		log.Println("lamp: lock folder:", err)
		return false
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return errors.Is(err, syscall.EWOULDBLOCK)
	}
	lampLockHeld = f
	return false
}

// runLampWindow opens the lamp panel and runs until it is closed. A second
// lamp process exits at once with 0. Built, not seen: no Mac was used.
func runLampWindow(o lampOpts) int {
	var setDir, snapPath string
	if !o.demo {
		setDir = settingsDir()
		p, err := snapshot.DefaultPath()
		if err != nil {
			log.Println("lamp: snapshot path:", err)
			return 1
		}
		snapPath = p
	}
	if lampAlreadyRunning(o.demo) {
		return 0
	}
	set := lamp.DefaultSettings()
	if !o.demo {
		set = readLampSettings(setDir)
	}
	// The clear backdrop and the card backdrop: the Windows environment variable
	// is not used here, only the choice. Built, not seen: if the web view still
	// paints an opaque square, lamp_backdrop card is the look that works.
	_, _, glass := lampBackdropEnv(set.Backdrop)
	startW, startH := lampWindowSize(set)

	w := webview.New(false)
	if w == nil {
		log.Println("lamp: could not create the window")
		return 1
	}
	defer w.Destroy()
	w.SetTitle("Glim")
	w.SetSize(startW, startH, webview.HintFixed)
	style := 0
	if glass {
		style = 1
	}
	C.lampStyle(w.Window(), C.int(style))
	lampIdle = lampIdleSeconds

	e := newLampEngine(o.demo, setDir, snapPath)
	if !o.demo {
		e.ledger = lamp.LoadLedger(filepath.Join(setDir, lamp.LedgerFile))
		defer func() { e.ledger.Close(time.Now()) }()
	}
	pl := &lampMacPlacer{win: w.Window(), setDir: setDir, demo: o.demo, w: startW, h: startH}
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
	// macOS has no window region: the page still reports its area, the panel
	// keeps its whole rectangle and clicks on the clear part reach the panel.
	bind("lampArea", func(string) {})
	// The drag to another monitor is built for Windows only (fix lane T2): the
	// binding exists so the page runs, and does nothing here. Built, not seen.
	bind("lampDragStart", func() {})
	// Grab and swing: the same answer as on Windows. Built, not seen.
	bind("lampSwing", func(maxDeg float64, changes int) lampSwingResult {
		return e.swing(maxDeg, changes, time.Now())
	})
	// "Close the lamp": ledger first, lamp_enabled false only for a lamp BurnMon
	// Dev started, then the lamp ends. Built, not seen.
	bind("lampClose", func() {
		go lampCloseNow(func() { e.ledger.Close(time.Now()) }, os.Getenv(lampParentEnv) != "",
			func(key string, v any) error {
				if o.demo {
					return nil
				}
				return saveDevConfigKey(setDir, key, v)
			}, func() { lampEnd(w.Dispatch, w.Terminate, os.Exit, lampEndGrace) })
	})
	bind("lampSize", func(cssW, cssH int) {
		pl.setSize(cssW, cssH)
		w.Dispatch(pl.place)
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
	w.Run()
	return 0
}

// lampProcessAlive reports whether process pid still runs: signal 0 checks
// without sending anything. A process owned by another user answers EPERM and
// is alive.
func lampProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// lampIdleSeconds is the seconds since the last keyboard or mouse input on this
// computer. It reads and stores nothing.
func lampIdleSeconds() int {
	s := float64(C.lampIdleC())
	if s < 0 {
		return 0
	}
	return int(s)
}

// lampMacPlacer holds the size the page asked for (points equal CSS pixels on
// macOS) and puts the panel there. place must run on the main thread.
type lampMacPlacer struct {
	win    unsafe.Pointer
	setDir string
	demo   bool

	mu   sync.Mutex
	w, h int
}

func (p *lampMacPlacer) setSize(cssW, cssH int) {
	if cssW < 1 || cssH < 1 || cssW > 2000 || cssH > 2000 {
		return
	}
	p.mu.Lock()
	p.w, p.h = cssW, cssH
	p.mu.Unlock()
}

func (p *lampMacPlacer) position() int {
	pos := lamp.DefaultSettings().Position
	if p.demo {
		pos = "top-center"
	} else if b, err := os.ReadFile(devConfigReadPath(p.setDir)); err == nil {
		pos = lamp.ParseSettings(b).Position
	}
	switch pos {
	case "top-left":
		return 1
	case "top-right":
		return 2
	}
	return 0
}

func (p *lampMacPlacer) place() {
	p.mu.Lock()
	w, h := p.w, p.h
	p.mu.Unlock()
	C.lampPlace(p.win, C.double(w), C.double(h), C.int(p.position()))
}

// lampChildAttr is the Windows first-show focus try; the Mac has none.
func lampChildAttr() *syscall.SysProcAttr { return nil }

// lampOpenDev starts BurnMon Dev (this exe, no arguments, no shell). A
// BurnMon Dev that already runs holds its own lock and logs that it will not
// raise its window: macOS gives no documented way to raise another process's
// window (platform_darwin.go). The lamp never takes BurnMon Dev's lock.
func lampOpenDev() {
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

// runCardWindow is a stub: the card window runs on Windows only in this
// version (the SVG cards of --card are written anyway).
func runCardWindow(string, lamp.Week, string) int { return lampUnsupported(os.Stderr) }

//go:build windows

// notify_windows.go shows the toast: a Shell_NotifyIcon balloon, which
// Windows 10 and 11 show as a toast. All of it runs on one hidden window on
// its own locked OS thread, never on the WebView2 UI thread (a blocking call
// there froze BurnMon Dev in B7.2). golang.org/x/sys only, no new dependency.
package toast

import (
	"errors"
	"fmt"
	"log"
	"runtime"
	"sync"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")

	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procPostMessageW     = user32.NewProc("PostMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procLoadIconW        = user32.NewProc("LoadIconW")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
)

const (
	wmDestroy = 0x0002
	wmClose   = 0x0010
	wmShow    = 0x8000 + 1 // WM_APP+1: Show stored new text
	wmTray    = 0x8000 + 2 // WM_APP+2: the shell reports on our icon

	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nimSetVersion = 4

	nifMessage = 0x1
	nifIcon    = 0x2
	nifTip     = 0x4
	nifInfo    = 0x10

	niifInfo           = 0x1
	notifyIconVersion4 = 4

	ninBalloonHide      = 0x403
	ninBalloonTimeout   = 0x404
	ninBalloonUserClick = 0x405

	idiApplication = 32512
	className      = "BurnMonToastWindow"
)

// notifyIconData mirrors NOTIFYICONDATAW (976 bytes on 64-bit Windows; the
// layout test pins every offset Shell_NotifyIconW reads).
type notifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32 // a union with uTimeout, which Windows ignores since Vista
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     uintptr
}

// wndClassExW mirrors WNDCLASSEXW.
type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

// msgW mirrors MSG.
type msgW struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
	Private uint32
}

var (
	regOnce  sync.Once
	regErr   error
	classPtr *uint16
	// notifiers maps a window to its Notifier for wndProc.
	notifiers sync.Map
)

// register makes the window class once per process; the callback is made
// once too, since Windows callbacks from Go are a limited resource.
func register() error {
	regOnce.Do(func() {
		classPtr, regErr = windows.UTF16PtrFromString(className)
		if regErr != nil {
			return
		}
		inst, _, _ := procGetModuleHandleW.Call(0)
		wc := wndClassExW{WndProc: windows.NewCallback(wndProc), Instance: inst, ClassName: classPtr}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			regErr = fmt.Errorf("RegisterClassExW: %w", err)
		}
	})
	return regErr
}

// Notifier shows toasts from its own hidden window. Show and Close are safe
// from any goroutine.
type Notifier struct {
	hwnd    uintptr
	tip     string // the icon's tooltip and the toast's title
	onClick func()
	report  func(error) // tests: the result of each show; nil in the app
	done    chan struct{}

	mu      sync.Mutex
	text    string
	pending bool // a wmShow is posted and not handled yet
	closed  bool

	// Window thread only: the ID of the icon on show (0: none) and the last
	// ID handed out.
	id, lastID uint32
}

// Start creates the hidden window on its own locked OS thread. appName is the
// toast's title and the icon's tooltip. onClick runs, on a goroutine of its
// own, when the person clicks a toast.
func Start(appName string, onClick func()) (*Notifier, error) {
	return start(appName, onClick, nil)
}

func start(appName string, onClick func(), report func(error)) (*Notifier, error) {
	if err := register(); err != nil {
		return nil, err
	}
	n := &Notifier{tip: appName, onClick: onClick, report: report, done: make(chan struct{})}
	ready := make(chan error, 1)
	go n.run(ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return n, nil
}

// run owns the window and pumps its messages. It never unlocks the thread,
// so Go retires the thread with the goroutine once the window is gone.
func (n *Notifier) run(ready chan<- error) {
	runtime.LockOSThread()
	defer close(n.done)
	inst, _, _ := procGetModuleHandleW.Call(0)
	// No WS_VISIBLE and never ShowWindow: the window only receives messages.
	// No title either: the app finds its own window with FindWindowW by its
	// title (a second start, uicheck), and must never find this one.
	hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(classPtr)), 0,
		0, 0, 0, 0, 0, 0, 0, inst, 0)
	if hwnd == 0 {
		ready <- fmt.Errorf("CreateWindowExW: %w", err)
		return
	}
	n.hwnd = hwnd
	notifiers.Store(hwnd, n)
	ready <- nil
	var m msgW
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 { // 0 is WM_QUIT, -1 an error
			return
		}
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// Show puts text in a toast. It stores the text and posts a message, so it
// never blocks. A Show before the last one was handled replaces its text.
// After Close it does nothing.
func (n *Notifier) Show(text string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return
	}
	n.text = text
	if !n.pending {
		n.pending = true
		procPostMessageW.Call(n.hwnd, wmShow, 0, 0)
	}
}

// Close removes the icon, destroys the window and waits for its goroutine.
func (n *Notifier) Close() {
	n.mu.Lock()
	if !n.closed {
		n.closed = true
		procPostMessageW.Call(n.hwnd, wmClose, 0, 0)
	}
	n.mu.Unlock()
	<-n.done
}

// wndProc runs on the window's own thread, inside DispatchMessageW or
// GetMessageW.
func wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	v, ok := notifiers.Load(hwnd)
	if !ok {
		r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
		return r
	}
	n := v.(*Notifier)
	switch msg {
	case wmShow:
		n.mu.Lock()
		text := n.text
		n.pending = false
		n.mu.Unlock()
		err := n.show(text)
		if err != nil {
			log.Println("toast:", err)
		}
		if n.report != nil {
			n.report(err)
		}
		return 0
	case wmTray:
		click, remove := trayEvent(lParam, n.id)
		if click && n.onClick != nil {
			// Not on this thread: bringing a window forward sends to its
			// thread, which at exit may be waiting in Close for this one.
			go n.onClick()
		}
		if remove {
			n.remove()
		}
		return 0
	case wmClose:
		n.remove()
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		notifiers.Delete(hwnd)
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

// show puts one toast up. Each toast gets a new icon under a new ID, and the
// icon of an earlier toast goes first, so a late event of that toast (its
// hide comes after the new one is up) cannot remove the new one.
func (n *Notifier) show(text string) error {
	n.remove()
	n.lastID = n.lastID%0xFFFF + 1 // the ID comes back in 16 bits of lParam; 0 means none
	d := n.data(n.lastID)
	d.UFlags = nifMessage | nifIcon | nifTip
	d.UCallbackMessage = wmTray
	d.HIcon = appIcon()
	putUTF16(d.SzTip[:], n.tip)
	if !notifyIcon(nimAdd, &d) {
		return errors.New("Shell_NotifyIconW NIM_ADD returned FALSE")
	}
	n.id = n.lastID
	// Version 4 puts the event and the icon ID in lParam, which trayEvent reads.
	d.UVersion = notifyIconVersion4
	if !notifyIcon(nimSetVersion, &d) {
		n.remove()
		return errors.New("Shell_NotifyIconW NIM_SETVERSION returned FALSE")
	}
	d = n.data(n.id)
	d.UFlags = nifInfo
	d.DwInfoFlags = niifInfo
	putUTF16(d.SzInfoTitle[:], n.tip)
	putUTF16(d.SzInfo[:], text)
	if !notifyIcon(nimModify, &d) {
		n.remove()
		return errors.New("Shell_NotifyIconW NIM_MODIFY returned FALSE")
	}
	return nil
}

// remove deletes the icon on show, if any.
func (n *Notifier) remove() {
	if n.id == 0 {
		return
	}
	d := n.data(n.id)
	notifyIcon(nimDelete, &d)
	n.id = 0
}

func (n *Notifier) data(id uint32) notifyIconData {
	d := notifyIconData{HWnd: n.hwnd, UID: id}
	d.CbSize = uint32(unsafe.Sizeof(d))
	return d
}

func notifyIcon(op uintptr, d *notifyIconData) bool {
	r, _, _ := procShellNotifyIconW.Call(op, uintptr(unsafe.Pointer(d)))
	return r != 0
}

// appIcon is the exe's icon resource 1 (the "#1" group winres embeds). A
// binary without one, such as a test, gets the stock application icon.
func appIcon() uintptr {
	inst, _, _ := procGetModuleHandleW.Call(0)
	if h, _, _ := procLoadIconW.Call(inst, 1); h != 0 {
		return h
	}
	h, _, _ := procLoadIconW.Call(0, idiApplication)
	return h
}

// trayEvent reads a tray callback: with version 4 the event is the low word
// of lParam and the icon ID the high word. A click on a toast brings the app
// forward. A click, timeout or hide of the toast on show (cur) removes its
// icon, so none stays in the tray; an event of a replaced toast removes
// nothing, its icon is already gone.
func trayEvent(lParam uintptr, cur uint32) (click, remove bool) {
	ev, id := uint32(lParam&0xFFFF), uint32(lParam>>16&0xFFFF)
	switch ev {
	case ninBalloonUserClick, ninBalloonTimeout, ninBalloonHide:
		remove = cur != 0 && id == cur
	}
	return ev == ninBalloonUserClick, remove
}

// putUTF16 copies s with its NUL into a fixed field of UTF-16 units. A text
// that does not fit is cut on a rune boundary, never between the halves of a
// surrogate pair, and ends in "...": 200 runes of emoji need 400 units and
// szInfo holds 256 with the NUL.
func putUTF16(dst []uint16, s string) {
	u := utf16.Encode([]rune(s))
	if len(u) > len(dst)-1 {
		n := len(dst) - 1 - len("...")
		if r := u[n-1]; r >= 0xD800 && r < 0xDC00 {
			n-- // a high surrogate whose low half would be cut off
		}
		u = append(u[:n:n], '.', '.', '.')
	}
	copy(dst, u)
	dst[len(u)] = 0
}

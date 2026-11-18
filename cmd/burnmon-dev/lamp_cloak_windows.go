//go:build windows

package main

import (
	"log"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procUnhookWindowsHookExLmp = user32.NewProc("UnhookWindowsHookEx")
	procDwmSetWindowAttrLmp    = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	procGetParentLmp           = user32.NewProc("GetParent")
)

const (
	lampWhCallWndProc = 4        // WH_CALLWNDPROC
	lampDwmwaCloak    = 13       // DWMWA_CLOAK
	lampWsCaptionBits = 0xC00000 // WS_CAPTION, set on a window of go-webview2 until the lamp styles it
)

// lampCwpStruct is the CWPSTRUCT a WH_CALLWNDPROC hook receives.
type lampCwpStruct struct {
	lParam, wParam uintptr
	message        uint32
	hwnd           uintptr
}

// lampCloaked is the window the hook has cloaked; the hook and the code that
// installs it run on the same thread, so it needs no lock.
var lampCloaked uintptr

// lampCloakCB is the one hook procedure of the process (a callback made by
// windows.NewCallback is never freed). go-webview2 creates a titled window and
// shows it with SW_SHOW before the lamp can style it. The hook sees the
// messages of that window while it is made and cloaks the first top-level
// window with a caption as soon as DWM accepts the call (not at its creation:
// DWM answers invalid handle then). A cloaked window is composed as nothing,
// so the plain window never reaches the screen. runLampWindow uncloaks it
// after the style and the place are set.
var lampCloakCB = windows.NewCallback(func(nCode int32, wParam uintptr, lParam unsafe.Pointer) uintptr {
	if nCode >= 0 && lampCloaked == 0 {
		cw := (*lampCwpStruct)(lParam)
		if cw.hwnd != 0 {
			style, _, _ := procGetWindowLongPtrW.Call(cw.hwnd, gwlStyle)
			if parent, _, _ := procGetParentLmp.Call(cw.hwnd); parent == 0 && style&lampWsCaptionBits == lampWsCaptionBits {
				if lampSetCloak(cw.hwnd, true) {
					lampCloaked = cw.hwnd
				}
			}
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, uintptr(lParam))
	return r
})

// lampSetCloak cloaks or uncloaks hwnd through DWM and says whether DWM took it.
// It never activates the window.
func lampSetCloak(hwnd uintptr, on bool) bool {
	var v int32
	if on {
		v = 1
	}
	r, _, _ := procDwmSetWindowAttrLmp.Call(hwnd, lampDwmwaCloak, uintptr(unsafe.Pointer(&v)), 4)
	return r == 0
}

// lampHookCloak installs the hook on this thread and returns the function that
// removes it. When Windows refuses the hook the lamp still runs, and the plain
// window shows for a moment as it did before.
func lampHookCloak() (remove func()) {
	lampCloaked = 0
	tid, _, _ := procGetCurrentThreadID.Call()
	h, _, err := procSetWindowsHookExW.Call(lampWhCallWndProc, lampCloakCB, 0, tid)
	if h == 0 {
		log.Println("lamp: could not install the window cloak hook:", err)
		return func() {}
	}
	return func() { procUnhookWindowsHookExLmp.Call(h) }
}

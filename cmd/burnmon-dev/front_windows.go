//go:build windows

package main

var procIsIconic = user32.NewProc("IsIconic")

const swRestore = 9 // SW_RESTORE

// bringToFront is a toast click: restore the window when it is minimized,
// then raise it.
func bringToFront(hwnd uintptr) {
	if r, _, _ := procIsIconic.Call(hwnd); r != 0 {
		procShowWindowWS.Call(hwnd, swRestore)
	}
	procSetForegroundWindow.Call(hwnd)
}

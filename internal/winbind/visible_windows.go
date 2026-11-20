//go:build windows

package winbind

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const spiGetWorkArea = 0x0030

// VisibleSize is the size of the work area of the primary monitor (the
// screen minus the taskbar), or 0, 0 when Windows does not answer.
func VisibleSize() (int, int) {
	var r struct{ Left, Top, Right, Bottom int32 }
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SystemParametersInfoW")
	ok, _, _ := proc.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&r)), 0)
	if ok == 0 {
		return 0, 0
	}
	return int(r.Right - r.Left), int(r.Bottom - r.Top)
}

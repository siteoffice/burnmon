//go:build windows

package main

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"burnmon/internal/winbind"
)

// The native Windows folder picker: IFileOpenDialog with FOS_PICKFOLDERS,
// the dialog Explorer uses. It runs on the window thread, which the WebView2
// loader has already put in a single-threaded COM apartment; bindings run on
// that thread, so the dialog is modal to the Control Centre window.

var (
	ole32                           = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance            = ole32.NewProc("CoCreateInstance")
	shell32                         = windows.NewLazySystemDLL("shell32.dll")
	procSHCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")

	clsidFileOpenDialog = windows.GUID{Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE, Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidIFileOpenDialog  = windows.GUID{Data1: 0xD57C7288, Data2: 0xD4AD, Data3: 0x4768, Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
	iidIShellItem       = windows.GUID{Data1: 0x43826D1E, Data2: 0xE718, Data3: 0x42EE, Data4: [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFE}}
)

const (
	fosPickFolders     = 0x20
	fosForceFileSystem = 0x40
	fosPathMustExist   = 0x800
	sigdnFileSysPath   = 0x80058000
	clsctxInprocServer = 0x1
	hrCancelled        = 0x800704C7 // HRESULT_FROM_WIN32(ERROR_CANCELLED)

	// vtable slots: IUnknown 0-2, IModalWindow.Show 3, IFileDialog 4-26.
	slotRelease     = 2
	slotShow        = 3
	slotSetOptions  = 9
	slotGetOptions  = 10
	slotSetFolder   = 12
	slotSetTitle    = 17
	slotGetResult   = 20
	slotDisplayName = 5 // IShellItem.GetDisplayName
)

// comCall calls slot idx of the COM object obj with args after the this pointer.
func comCall(obj unsafe.Pointer, idx int, args ...uintptr) uint32 {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return uint32(r)
}

// newFolderPicker returns the picker owned by the window w.
func newFolderPicker(w winbind.Window) func(start string) (string, bool, error) {
	var owner uintptr
	if hw, ok := w.(interface{ Window() unsafe.Pointer }); ok {
		owner = uintptr(hw.Window())
	}
	return func(start string) (string, bool, error) { return pickFolder(owner, start) }
}

// pickFolder shows the dialog in start; it returns the folder, or ok false
// when the person cancels.
func pickFolder(owner uintptr, start string) (string, bool, error) {
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err == nil || err == syscall.Errno(1) {
		defer windows.CoUninitialize() // S_OK and S_FALSE both need the matching call
	}
	var dlg unsafe.Pointer
	r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)), uintptr(unsafe.Pointer(&dlg)))
	if r != 0 || dlg == nil {
		return "", false, fmt.Errorf("CoCreateInstance: 0x%08X", uint32(r))
	}
	defer comCall(dlg, slotRelease)

	var opts uint32
	if hr := comCall(dlg, slotGetOptions, uintptr(unsafe.Pointer(&opts))); hr != 0 {
		return "", false, fmt.Errorf("GetOptions: 0x%08X", hr)
	}
	if hr := comCall(dlg, slotSetOptions, uintptr(opts|fosPickFolders|fosForceFileSystem|fosPathMustExist)); hr != 0 {
		return "", false, fmt.Errorf("SetOptions: 0x%08X", hr)
	}
	if title, err := windows.UTF16PtrFromString("Pick the Control Centre root"); err == nil {
		comCall(dlg, slotSetTitle, uintptr(unsafe.Pointer(title)))
	}
	if start != "" {
		if p, err := windows.UTF16PtrFromString(start); err == nil {
			var item unsafe.Pointer
			r, _, _ := procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(p)), 0,
				uintptr(unsafe.Pointer(&iidIShellItem)), uintptr(unsafe.Pointer(&item)))
			if r == 0 && item != nil {
				comCall(dlg, slotSetFolder, uintptr(item))
				comCall(item, slotRelease)
			}
		}
	}
	hr := comCall(dlg, slotShow, owner)
	if hr == hrCancelled {
		return "", false, nil
	}
	if hr != 0 {
		return "", false, fmt.Errorf("Show: 0x%08X", hr)
	}
	var item unsafe.Pointer
	if hr := comCall(dlg, slotGetResult, uintptr(unsafe.Pointer(&item))); hr != 0 || item == nil {
		return "", false, fmt.Errorf("GetResult: 0x%08X", hr)
	}
	defer comCall(item, slotRelease)
	var name *uint16
	if hr := comCall(item, slotDisplayName, sigdnFileSysPath, uintptr(unsafe.Pointer(&name))); hr != 0 || name == nil {
		return "", false, fmt.Errorf("GetDisplayName: 0x%08X", hr)
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(name))
	p := windows.UTF16PtrToString(name)
	if p == "" {
		return "", false, errors.New("the picked item has no file system path")
	}
	return p, true, nil
}

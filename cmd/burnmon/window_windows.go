//go:build windows

package main

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"burnmon/internal/winbind"
)

var _ winbind.Window = webview2.WebView(nil)

// newAppWindow creates the WebView2 window, or returns a literal nil when the
// runtime is missing so the caller can fall back to the browser.
func newAppWindow(dataDir string) winbind.Window {
	wv2Dir := filepath.Join(dataDir, "wv2")
	_ = os.MkdirAll(wv2Dir, 0o755)

	// WEBVIEW2_USER_DATA_FOLDER (alpha.7, found 2026-09-29): go-webview2
	// (pkg/edge/chromium.go) passes DataPath to WebView2Loader as an inline
	// windows.StringToUTF16Ptr that nothing keeps alive, and the loader reads
	// it after the Go buffer can already be freed and reused, so some launches
	// got a user-data folder named after whatever heap text was there (prompt
	// and session-path fragments), created relative to the working directory.
	// The loader also reads this environment variable, which lives in the
	// process environment, not in Go memory, so the folder is always wv2Dir.
	_ = os.Setenv("WEBVIEW2_USER_DATA_FOLDER", wv2Dir)
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath: wv2Dir,
		WindowOptions: webview2.WindowOptions{
			Title:  windowTitle,
			Width:  1280,
			Height: 860,
			Center: true,
			IconId: 1, // matches the "#1" icon group winres/app.json embeds
		},
	})
	if w == nil {
		log.Println("could not create the WebView2 window; falling back to the default browser")
		return nil
	}
	return w
}

// ---------------------------------------------------------------------------
// Single instance and the WebView2-missing fallback
// ---------------------------------------------------------------------------

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW         = user32.NewProc("FindWindowW")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

// alreadyRunning claims a named mutex for the life of the process. Windows
// reports ERROR_ALREADY_EXISTS when another instance already holds it, even
// though the returned handle is otherwise valid; that is the documented way
// to detect this without a helper class.
func alreadyRunning() bool {
	name, err := windows.UTF16PtrFromString(mutexName)
	if err != nil {
		return false
	}
	_, err = windows.CreateMutex(nil, false, name)
	return errors.Is(err, windows.ERROR_ALREADY_EXISTS)
}

func bringExistingToFront() {
	title, err := windows.UTF16PtrFromString(windowTitle)
	if err != nil {
		return
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		return
	}
	procSetForegroundWindow.Call(hwnd)
}

const (
	mbOK          = 0x00000000
	mbIconWarning = 0x00000030
)

// runWithoutWindow covers the very unlikely case of a managed machine
// missing the WebView2 runtime: collect once, open the result in the
// default browser, and say why there is no app window.
func runWithoutWindow(a *app) {
	if _, err := a.rebuild(true, nil); err != nil {
		log.Println("fallback collection failed:", err)
	}
	openInBrowser(a.htmlPath)
	showWebView2MissingMessage()
}

func openInBrowser(path string) {
	_ = exec.Command("cmd", "/c", "start", "", path).Start()
}

func showWebView2MissingMessage() {
	text, _ := windows.UTF16PtrFromString(
		"Claude Cost could not open its window because the WebView2 runtime is missing.\n" +
			"The dashboard was opened in your default browser instead. Installing the\n" +
			"WebView2 runtime (part of Microsoft Edge) will let the app window open normally.")
	caption, _ := windows.UTF16PtrFromString(windowTitle)
	_, _ = windows.MessageBox(0, text, caption, mbOK|mbIconWarning)
}

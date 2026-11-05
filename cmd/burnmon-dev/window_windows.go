//go:build windows

package main

import (
	"log"
	"os"
	"path/filepath"

	webview2 "github.com/jchv/go-webview2"

	"burnmon/internal/winbind"
)

var _ winbind.Window = webview2.WebView(nil)

// windowHooks carries what the Win32 side keeps of the window: the handle and
// the fullscreen state, so the fullscreen bindings and the toast click need
// no webview2 type.
type windowHooks struct {
	hwnd uintptr
	fs   *fullscreenState
}

func (h windowHooks) toggleFullscreen() { toggleFullscreen(h.hwnd, h.fs) }
func (h windowHooks) exitFullscreen()   { exitFullscreen(h.hwnd, h.fs) }

// openDevWindow creates the WebView2 window with its saved state, the window
// state saver and the F11 hotkey. It returns false after logging when the
// WebView2 runtime is missing.
func openDevWindow(setDir string) (winbind.Window, windowHooks, bool) {
	wv2Dir := filepath.Join(setDir, "wv2-dev")
	_ = os.MkdirAll(wv2Dir, 0o755)

	// Window, section 10: first launch is a normal 1280x860 window,
	// centered; every launch after that remembers size, position, maximized
	// state and monitor, restoring them only if that monitor still exists
	// (windowstate.go). WindowOptions has no X/Y field, so a restored
	// window is created at this same default size/center and then
	// immediately repositioned via applyWindowState below; a very brief
	// visible jump to the default rect is an accepted tradeoff of
	// go-webview2's WebView interface having no "create hidden, then show"
	// hook to avoid it.
	savedState, hasSavedState := loadWindowState(setDir)
	initialWidth, initialHeight := uint(1280), uint(860)
	if hasSavedState {
		initialWidth, initialHeight = uint(savedState.Width), uint(savedState.Height)
	}
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
			Width:  initialWidth,
			Height: initialHeight,
			Center: true,
			IconId: 1, // matches the "#1" icon group winres/burnmon-dev.json embeds
		},
	})
	if w == nil {
		log.Println("could not create the WebView2 window; BurnMon Dev needs the WebView2 runtime")
		return nil, windowHooks{}, false
	}
	hwnd := uintptr(w.Window())
	if hasSavedState {
		applyWindowState(hwnd, savedState)
	}

	// F11 toggles fullscreen, Esc also leaves it (section 10). fs is this
	// window's own fullscreen state (windowstate.go), declared before
	// startWindowStateSaver so that goroutine can skip saving while
	// fullscreen (its own doc comment explains why). Esc reaches
	// bdevExitFullscreen through the page's own keydown handler, same as any
	// other binding; F11 does not, since WebView2/Chromium reserves it as a
	// default browser accelerator key and never lets the DOM see it at all
	// (found empirically this session) - installF11Hotkey works around that
	// with a global hotkey plus a thread-local message hook instead (see its
	// own doc comment, windowstate.go), so bdevToggleFullscreen below is
	// only reachable from the uicheck dev eval channel, not from a real F11
	// keypress.
	fs := &fullscreenState{}
	startWindowStateSaver(hwnd, setDir, fs)

	installF11Hotkey(hwnd, fs)
	return w, windowHooks{hwnd: hwnd, fs: fs}, true
}

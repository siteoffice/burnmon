//go:build darwin && cgo

package main

import (
	webview "github.com/webview/webview_go"

	"burnmon/internal/winbind"
)

var _ winbind.Window = webview.WebView(nil)

// openDevWindow opens the native WKWebView window. No window state restore on
// macOS.
func openDevWindow(setDir string) (winbind.Window, windowHooks, bool) {
	w := webview.New(false)
	if w == nil {
		return nil, windowHooks{}, false
	}
	w.SetTitle(windowTitle)
	w.SetSize(1280, 860, webview.HintNone)
	return w, windowHooks{}, true
}

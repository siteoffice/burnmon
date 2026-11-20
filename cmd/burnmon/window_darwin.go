//go:build darwin && cgo

package main

import (
	webview "github.com/webview/webview_go"

	"burnmon/internal/winbind"
)

var _ winbind.Window = webview.WebView(nil)

// newAppWindow opens the native WKWebView window. dataDir is unused on macOS.
func newAppWindow(dataDir string) winbind.Window {
	w := webview.New(false)
	if w == nil {
		return nil
	}
	w.SetTitle(windowTitle)
	// The first window is limited to the visible area of the main screen.
	width, height := winbind.FirstWindowSize(1280, 860)
	w.SetSize(width, height, webview.HintNone)
	return w
}

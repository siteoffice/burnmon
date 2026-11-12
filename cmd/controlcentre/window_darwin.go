//go:build darwin && cgo

package main

import (
	webview "github.com/webview/webview_go"

	"burnmon/internal/winbind"
)

var _ winbind.Window = webview.WebView(nil)

// openWindow opens the native WKWebView window, 1536 by 864. WKWebView keeps
// its own data, so dataDir is not used here.
func openWindow(dataDir string) (winbind.Window, bool) {
	w := webview.New(false)
	if w == nil {
		return nil, false
	}
	w.SetTitle(windowTitle)
	w.SetSize(1536, 864, webview.HintNone)
	return w, true
}

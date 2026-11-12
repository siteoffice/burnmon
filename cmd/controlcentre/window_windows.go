//go:build windows

package main

import (
	"os"
	"path/filepath"

	webview2 "github.com/jchv/go-webview2"

	"burnmon/internal/winbind"
)

var _ winbind.Window = webview2.WebView(nil)

// openWindow creates the WebView2 window, 1536 by 864, centered, with its
// user-data folder in <dataDir>\wv2-cc. It returns false when the WebView2
// runtime is missing.
func openWindow(dataDir string) (winbind.Window, bool) {
	wv2Dir := filepath.Join(dataDir, "wv2-cc")
	_ = os.MkdirAll(wv2Dir, 0o755)
	// WEBVIEW2_USER_DATA_FOLDER: the same go-webview2 DataPath lifetime
	// workaround as cmd\burnmon-dev\window_windows.go; the loader reads this
	// variable from the process environment, so the folder is always wv2Dir.
	_ = os.Setenv("WEBVIEW2_USER_DATA_FOLDER", wv2Dir)
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath: wv2Dir,
		WindowOptions: webview2.WindowOptions{
			Title:  windowTitle,
			Width:  1536,
			Height: 864,
			Center: true,
			IconId: 1, // matches the "#1" icon group winres/controlcentre.json embeds
		},
	})
	if w == nil {
		return nil, false
	}
	return w, true
}

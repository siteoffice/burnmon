package winbind

// FirstWindowMargin is what the first window leaves free on each axis of the
// visible screen area, in pixels: room for the title bar and a gap to the
// screen edge.
const FirstWindowMargin = 64

// FitFirstWindow returns the size of the first window: the wanted size,
// limited to the visible area of the screen minus FirstWindowMargin on each
// axis. A visible size that is zero, or too small to hold the margin, means
// the screen could not be read, and the wanted size is returned unchanged.
func FitFirstWindow(wantW, wantH, visW, visH int) (int, int) {
	w, h := wantW, wantH
	if maxW := visW - FirstWindowMargin; maxW > 0 && w > maxW {
		w = maxW
	}
	if maxH := visH - FirstWindowMargin; maxH > 0 && h > maxH {
		h = maxH
	}
	return w, h
}

// FirstWindowSize is FitFirstWindow against the visible area of the main
// screen of this computer (see VisibleSize in the per-system files).
func FirstWindowSize(wantW, wantH int) (int, int) {
	visW, visH := VisibleSize()
	return FitFirstWindow(wantW, wantH, visW, visH)
}

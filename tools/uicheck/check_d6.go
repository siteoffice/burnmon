package main

// d6: 2560x1440, one of the five viewports section 11 calls out to check,
// and the one section 11's own "in fullscreen at 1152x2048 and 2560x1440
// everything fits without vertical scroll" line names by size (d7 covers
// the fullscreen case itself).
func caseD6(hwnd uintptr, args []string) error {
	return checkViewportFits(hwnd, "d6", 2560, 1440)
}

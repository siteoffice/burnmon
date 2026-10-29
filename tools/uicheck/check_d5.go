package main

// d5: 1920x1080, one of the five viewports section 11 calls out to check.
// Wide enough to cross the 1600px two-column breakpoint (UI review patch
// section 10).
func caseD5(hwnd uintptr, args []string) error {
	return checkViewportFits(hwnd, "d5", 1920, 1080)
}

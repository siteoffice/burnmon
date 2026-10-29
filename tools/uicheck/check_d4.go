package main

// d4: 1280x860 (UI review patch section 10's first-launch default size,
// also one of the five viewports section 11 calls out to check).
func caseD4(hwnd uintptr, args []string) error {
	return checkViewportFits(hwnd, "d4", 1280, 860)
}

package main

// d8: 1024x768, the fifth of the five viewports the no-scroll patch
// (02_roadmap\2026-09-25_ws2_burn_chart_no_scroll_patch.md) calls out,
// distinct from d2/d3's own two-viewport design sizes (1152x2048,
// 1024x1152).
func caseD8(hwnd uintptr, args []string) error {
	return checkViewportFits(hwnd, "d8", 1024, 768)
}

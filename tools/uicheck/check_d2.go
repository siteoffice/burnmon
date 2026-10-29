package main

import (
	"fmt"
	"time"
)

// d2: the primary viewport (screen 2, portrait, 1152x2048 CSS px, design
// doc "Viewports") fits with no scroll. body has overflow:hidden (so there
// is never literally a scrollbar to check), so this instead compares
// document.body.scrollHeight against window.innerHeight: content taller
// than the viewport is silently clipped rather than shown with a
// scrollbar, and that clipping is exactly what "fits with no scrolling"
// rules out.
func caseD2(hwnd uintptr, args []string) error {
	return checkViewportFits(hwnd, "d2", 1152, 2048)
}

// checkViewportFits sets the window to width x height (outer, physical
// window pixels, what ensureWindowSizeWH/SetWindowPos take) and then reads
// back the page's own CSS-pixel window.innerWidth/innerHeight, which is
// smaller than width/height by the title bar, borders and any DPI scaling,
// and can be smaller still if the OS clamps the window to fit the current
// screen (this check's own caller may be running on a display shorter than
// the target viewport, e.g. a single 1600x1000 remote session screen
// against the design doc's own 2048px-tall primary viewport). It logs and
// asserts against that real inner size, not the requested outer one, and
// flags plainly when the two disagree by more than a small margin, rather
// than silently reporting "fits" against whatever smaller size the OS
// actually granted (found by review, 2026-09-24).
func checkViewportFits(hwnd uintptr, name string, width, height int32) error {
	if err := ensureWindowSizeWH(hwnd, width, height); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond) // let layout/reflow settle after the resize

	var dims struct {
		ScrollHeight float64 `json:"scrollHeight"`
		InnerHeight  float64 `json:"innerHeight"`
		InnerWidth   float64 `json:"innerWidth"`
	}
	if err := evalInto(`({scrollHeight: document.body.scrollHeight, innerHeight: window.innerHeight, innerWidth: window.innerWidth})`, &dims); err != nil {
		return fmt.Errorf("read layout dimensions: %w", err)
	}
	fmt.Printf("uicheck: %s viewport target %dx%d, real CSS px innerWidth=%.0f innerHeight=%.0f, body.scrollHeight=%.0f\n",
		name, width, height, dims.InnerWidth, dims.InnerHeight, dims.ScrollHeight)

	// A wide gap between the requested outer size and the real inner size
	// (beyond ordinary title-bar/border/DPI overhead) means this run's own
	// screen could not actually grant the target viewport; the pass/fail
	// below still holds for whatever size was granted, but that is not the
	// same claim as "fits at 1152x2048 CSS px on the intended hardware".
	const sizeMismatchTolerance = 150
	if float64(width)-dims.InnerWidth > sizeMismatchTolerance || float64(height)-dims.InnerHeight > sizeMismatchTolerance {
		fmt.Printf("uicheck: %s NOTE: the real viewport (%.0fx%.0f) is well short of the %dx%d target; this run's screen likely could not fit the intended size, so this only confirms no clipping at the smaller size actually granted\n",
			name, dims.InnerWidth, dims.InnerHeight, width, height)
	}

	if _, err := screenshot(hwnd, name+"-viewport"); err != nil {
		return err
	}

	// A few px of tolerance for border/scrollbar-gutter rounding.
	const tolerance = 4
	if dims.ScrollHeight > dims.InnerHeight+tolerance {
		return fmt.Errorf("%s: page content (%.0fpx) is taller than the real viewport (%.0fpx); something is being clipped instead of fitting",
			name, dims.ScrollHeight, dims.InnerHeight)
	}
	return nil
}

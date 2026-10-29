package main

import (
	"fmt"
	"time"
)

// d3: the secondary viewport (screen 1, half-width column, 1024x1152 CSS
// px at 125%, design doc "Viewports"). The design note (section 2.2) flags
// this viewport as "not wired yet": phases 1-2 shipped one CSS layout
// shared by both viewports rather than the tabbed system zone the sketch
// describes, so the system zone is known to scroll here today, a
// pre-existing gap with no phase assigned yet, not something phase 4
// introduced. This check measures and reports the real
// scrollHeight/innerHeight numbers and always takes its screenshot, but
// (unlike d2) does not fail the run over that already-documented gap,
// per house culture: flag it, don't silently assert it away, and don't
// block the phase's own checks on an unrelated known issue either.
func caseD3(hwnd uintptr, args []string) error {
	if err := ensureWindowSizeWH(hwnd, 1024, 1152); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)

	var dims struct {
		ScrollHeight float64 `json:"scrollHeight"`
		InnerHeight  float64 `json:"innerHeight"`
	}
	if err := evalInto(`({scrollHeight: document.body.scrollHeight, innerHeight: window.innerHeight})`, &dims); err != nil {
		return fmt.Errorf("read layout dimensions: %w", err)
	}
	fmt.Printf("uicheck: d3 viewport 1024x1152: body.scrollHeight=%.0f window.innerHeight=%.0f\n", dims.ScrollHeight, dims.InnerHeight)
	if dims.ScrollHeight > dims.InnerHeight+4 {
		fmt.Println("uicheck: d3 NOTE: content overflows the secondary viewport (known gap, design doc section 2.2, \"not wired yet\"; not failing this check over it)")
	}

	if _, err := screenshot(hwnd, "d3-viewport"); err != nil {
		return err
	}
	return nil
}

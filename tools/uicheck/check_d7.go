package main

import (
	"fmt"
	"time"
)

// d7: fullscreen (F11). Section 11's own wording ("in fullscreen at
// 1152x2048 and 2560x1440 everything fits without vertical scroll") reads
// as if two different fullscreen sizes can be requested, but F11 always
// covers whatever the real monitor's own resolution is (windowstate.go's
// enterFullscreen sizes to MonitorFromWindow's rcMonitor, not to whatever
// size the window happened to be beforehand): this laptop has no monitor
// of either named resolution to actually test against. What this check
// verifies instead, honestly: the window resizes to each of those two
// sizes first (a real regression case: does enter/exit-fullscreen still
// work correctly starting from a small window versus a large one), F11
// then genuinely covers the real monitor (checked against screen.width/
// height, not just "some size bigger than before"; found by review,
// 2026-09-25, that an earlier draft never actually confirmed fullscreen
// engaged at all), fits with no vertical scroll, and F11 again always
// restores the window (deferred, so a failed assertion mid-check still
// leaves the window in a normal state for whatever check runs next in the
// same `uicheck-dev.ps1 d7 d2 d3 ...` sequence, rather than stuck
// fullscreen).
func caseD7(hwnd uintptr, args []string) error {
	for _, size := range [][2]int32{{1152, 2048}, {2560, 1440}} {
		if err := d7once(hwnd, size[0], size[1]); err != nil {
			return err
		}
	}
	return nil
}

type dims struct {
	ScrollHeight float64 `json:"scrollHeight"`
	InnerHeight  float64 `json:"innerHeight"`
	InnerWidth   float64 `json:"innerWidth"`
}

func d7once(hwnd uintptr, fromWidth, fromHeight int32) (rerr error) {
	name := fmt.Sprintf("d7-fullscreen-from-%dx%d", fromWidth, fromHeight)
	if err := ensureWindowSizeWH(hwnd, fromWidth, fromHeight); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)

	var screen struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	}
	if err := evalInto(`({width: window.screen.width, height: window.screen.height})`, &screen); err != nil {
		return fmt.Errorf("read screen size: %w", err)
	}

	entered := false
	// Always try to leave fullscreen before returning, success or failure,
	// so one bad assertion below does not strand the window fullscreen for
	// whatever check runs next.
	defer func() {
		if !entered {
			return
		}
		time.Sleep(200 * time.Millisecond)
		if err := pressKey(vkF11); err != nil && rerr == nil {
			rerr = fmt.Errorf("press F11 (exit): %w", err)
		}
	}()

	// Retried up to 3 times: F11 delivery here depends on this window
	// genuinely holding OS foreground status (windowstate.go's own F11 hook
	// gates on GetForegroundWindow, for real F11 safety against hijacking
	// every other app's F11 - see its own comment), and in this sandboxed,
	// heavily-scripted session that status was observed to be flaky run to
	// run for reasons unrelated to the fullscreen mechanism itself (found
	// by review, 2026-09-25: the exact same bringToFront+pressKey(F11)
	// sequence, run standalone right after launch, worked immediately and
	// repeatably; run through this check after uicheck-dev.ps1's own
	// backfill wait, it sometimes did not). A bounded retry is the pragmatic
	// answer to that class of flakiness; it would not paper over the
	// mechanism actually being broken, since every attempt re-does the real
	// keypress and re-checks the real result.
	var d dims
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		bringToFront(hwnd)
		// A genuine synthetic click (SendInput) is Windows' own most
		// reliable way to grant real foreground/input-focus status, more so
		// than the SetForegroundWindow/AttachThreadInput dance in
		// bringToFront alone: clicking the logo (always present, does
		// nothing) establishes real user-driven focus before the keypress
		// that depends on it (found by review, 2026-09-25: bringToFront
		// alone was not reliably enough in this sandboxed session).
		if err := clickSelectorAt(hwnd, ".logo", 0.5, 0.5); err != nil {
			fmt.Println("uicheck: d7: could not click .logo to establish focus:", err)
		}
		if err := pressKey(vkF11); err != nil {
			return fmt.Errorf("press F11: %w", err)
		}
		entered = true
		time.Sleep(500 * time.Millisecond)

		if err := evalInto(`({scrollHeight: document.body.scrollHeight, innerHeight: window.innerHeight, innerWidth: window.innerWidth})`, &d); err != nil {
			return fmt.Errorf("read layout dimensions: %w", err)
		}
		const tolerance = 4
		if screen.Width-d.InnerWidth <= tolerance*4 && screen.Height-d.InnerHeight <= tolerance*4 {
			lastErr = nil
			break
		}
		lastErr = fmt.Errorf("%s: viewport (%.0fx%.0f) is well short of the screen (%.0fx%.0f); fullscreen likely never engaged (attempt %d/3)",
			name, d.InnerWidth, d.InnerHeight, screen.Width, screen.Height, attempt)
		fmt.Println("uicheck:", lastErr)
		// Didn't engage: press F11 again to retry from a known "not
		// fullscreen" state, then bringToFront once more before the next
		// attempt.
		pressKey(vkF11)
		entered = false
		time.Sleep(300 * time.Millisecond)
	}
	if lastErr != nil {
		return lastErr
	}

	fmt.Printf("uicheck: %s: screen=%.0fx%.0f, CSS px innerWidth=%.0f innerHeight=%.0f, body.scrollHeight=%.0f\n",
		name, screen.Width, screen.Height, d.InnerWidth, d.InnerHeight, d.ScrollHeight)
	if _, err := screenshot(hwnd, name); err != nil {
		return err
	}

	const tolerance = 4
	if d.ScrollHeight > d.InnerHeight+tolerance {
		return fmt.Errorf("%s: page content (%.0fpx) is taller than the fullscreen viewport (%.0fpx)",
			name, d.ScrollHeight, d.InnerHeight)
	}
	return nil
}

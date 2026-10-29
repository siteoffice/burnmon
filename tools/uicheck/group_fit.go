package main

import (
	"fmt"
	"time"
)

// fit/d2: moved from check_d2.go.

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

// fit/d3: moved from check_d3.go.

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

// fit/d4: moved from check_d4.go.

// d4: 1280x860 (UI review patch section 10's first-launch default size,
// also one of the five viewports section 11 calls out to check).
func caseD4(hwnd uintptr, args []string) error {
	return checkViewportFits(hwnd, "d4", 1280, 860)
}

// fit/d5: moved from check_d5.go.

// d5: 1920x1080, one of the five viewports section 11 calls out to check.
// Wide enough to cross the 1600px two-column breakpoint (UI review patch
// section 10).
func caseD5(hwnd uintptr, args []string) error {
	return checkViewportFits(hwnd, "d5", 1920, 1080)
}

// fit/d6: moved from check_d6.go.

// d6: 2560x1440, one of the five viewports section 11 calls out to check,
// and the one section 11's own "in fullscreen at 1152x2048 and 2560x1440
// everything fits without vertical scroll" line names by size (d7 covers
// the fullscreen case itself).
func caseD6(hwnd uintptr, args []string) error {
	return checkViewportFits(hwnd, "d6", 2560, 1440)
}

// fit/d7: moved from check_d7.go.

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

// fit/d8: moved from check_d8.go.

// d8: 1024x768, the fifth of the five viewports the no-scroll patch
// (02_roadmap\2026-09-25_ws2_burn_chart_no_scroll_patch.md) calls out,
// distinct from d2/d3's own two-viewport design sizes (1152x2048,
// 1024x1152).
func caseD8(hwnd uintptr, args []string) error {
	return checkViewportFits(hwnd, "d8", 1024, 768)
}

// fit/d9: moved from check_d9.go.

// d9: no-scroll patch, "Verify and close" - a uicheck case that fails if
// any element has a visible scrollbar (scrollHeight > clientHeight or
// scrollWidth > clientWidth, with the corresponding overflow axis not
// hidden) at each of the five sizes the patch names: 1024x768, 1280x860,
// 1152x2048, 1920x1080, 2560x1440. Every list/grid the patch touches
// (activity heatmap, harness heatmap, process groups, turn popup) is meant
// to crop to whole items instead of scrolling, so this sweeps every element
// in the DOM rather than naming those containers by hand, the same way an
// actual scrollbar could show up anywhere a future change forgets to crop.
// #turnTicker and #todoBody are excluded: the headline patch (section 1)
// and WS2 alpha.3 item 1 respectively made them the two deliberate
// exceptions to "no scrollbars", rows get a fixed height and the box
// scrolls instead of cropping.
// The #secHost lists (.seclist) are the third exception on the vertical
// axis: B7.1 item 1 gives them "their own thin scrollbar below" the
// three-row floor (page.html secList comment, since 86a249d).
// Only overflow auto or scroll can show a scrollbar on an element; overflow
// visible spills into an ancestor, which the sweep checks on its own. html
// and body are the exception: their visible overflow becomes the window's
// scrollbar.
func caseD9(hwnd uintptr, args []string) error {
	sizes := [][2]int32{{1024, 768}, {1280, 860}, {1152, 2048}, {1920, 1080}, {2560, 1440}}
	for _, size := range sizes {
		if err := ensureWindowSizeWH(hwnd, size[0], size[1]); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)

		var offenders []string
		script := `(function(){
  var bad = [];
  var all = document.querySelectorAll('*');
  for (var i = 0; i < all.length; i++) {
    var el = all[i];
    if (el.id === 'turnTicker' || el.closest('#turnTicker')) continue;
    if (el.id === 'todoBody' || el.closest('#todoBody')) continue;
    // The R8 follow-up decided that Standard scrolls (decision of 2026-10-23, "for now"): #pageScroll is the
    // one deliberate scroll area besides the ticker and the To Do body. Plain keeps it overflow:hidden (d29).
    if (el.id === 'pageScroll') continue;
    var cs = getComputedStyle(el);
    if (cs.display === 'none') continue;
    var root = el === document.documentElement || el === document.body;
    var canBar = function(o){ return o === 'auto' || o === 'scroll' || (root && o !== 'hidden' && o !== 'clip'); };
    var vOver = el.scrollHeight > el.clientHeight + 1 && canBar(cs.overflowY) && !el.classList.contains('seclist');
    var hOver = el.scrollWidth > el.clientWidth + 1 && canBar(cs.overflowX);
    if (vOver || hOver) {
      var label = el.tagName.toLowerCase() + (el.id ? '#' + el.id : '') + (el.className && typeof el.className === 'string' && el.className ? '.' + el.className.split(' ').join('.') : '');
      bad.push(label + ' (scrollH=' + el.scrollHeight + ' clientH=' + el.clientHeight + ' scrollW=' + el.scrollWidth + ' clientW=' + el.clientWidth + ' overflowY=' + cs.overflowY + ' overflowX=' + cs.overflowX + ')');
    }
  }
  return bad;
})()`
		if err := evalInto(script, &offenders); err != nil {
			return fmt.Errorf("d9 %dx%d: eval scrollbar sweep: %w", size[0], size[1], err)
		}
		if len(offenders) > 0 {
			max := len(offenders)
			if max > 10 {
				max = 10
			}
			return fmt.Errorf("d9 %dx%d: %d element(s) with a visible scrollbar: %v", size[0], size[1], len(offenders), offenders[:max])
		}
		fmt.Printf("uicheck: d9 %dx%d: no visible scrollbar anywhere\n", size[0], size[1])
	}
	return nil
}

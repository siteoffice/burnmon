package main

import (
	"fmt"
	"strings"
	"time"
)

// d0: the UI review patch's own header (section 1) is present and quiet -
// a plain "PRESSURE nn" text, a headline total, no boxed chips (the old
// #cmd search input and .chip elements are gone) - and the deleted "Today's
// Read" panel (section 8) really is gone from the DOM, not just hidden.
// Replaces this check's own earlier version, which asserted the advisor
// panel (#advisorFindings) populated; that panel and its bdevAdvisorNow
// binding were deleted by this same patch's section 8.
func caseD0(hwnd uintptr, args []string) error {
	if err := ensureWindowSizeWH(hwnd, 1152, 2048); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)

	if exists, err := evalBool(`!!document.getElementById('advisorFindings')`); err != nil {
		return fmt.Errorf("check advisorFindings absent: %w", err)
	} else if exists {
		return fmt.Errorf("#advisorFindings still exists; section 8 was supposed to delete the Today's Read panel")
	}
	if exists, err := evalBool(`!!document.getElementById('cmd')`); err != nil {
		return fmt.Errorf("check #cmd absent: %w", err)
	} else if exists {
		return fmt.Errorf("#cmd (the search input) still exists; section 1 was supposed to remove it")
	}
	if n, err := evalRaw(`document.querySelectorAll('.chip').length`); err != nil {
		return fmt.Errorf("check .chip count: %w", err)
	} else if string(n) != "0" {
		return fmt.Errorf("found %s .chip element(s); section 1 was supposed to remove every boxed chip", n)
	}

	pressureText, err := evalString(`document.getElementById('pressureChip').textContent`)
	if err != nil {
		return fmt.Errorf("read pressure text: %w", err)
	}
	if !strings.HasPrefix(pressureText, "PRESSURE") {
		return fmt.Errorf("pressure text %q does not start with \"PRESSURE\"", pressureText)
	}

	// Give the first poll cycle a moment to land, then require the
	// headline total to hold a real (non-placeholder) value.
	var headline string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		headline, err = evalString(`document.getElementById('hlToday').textContent`)
		if err != nil {
			return fmt.Errorf("read headline total: %w", err)
		}
		if headline != "" && headline != "-" {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if headline == "" || headline == "-" {
		return fmt.Errorf("headline total never rendered a real value")
	}
	fmt.Println("uicheck: header OK, pressure =", pressureText, "headline =", headline)

	exportText, err := evalString(`document.getElementById('exportBtn').textContent`)
	if err != nil {
		return fmt.Errorf("read export button text: %w", err)
	}
	if exportText != "EXPORT" {
		return fmt.Errorf("export button text = %q, want \"EXPORT\"", exportText)
	}

	if _, err := screenshot(hwnd, "d0-header"); err != nil {
		return err
	}
	return nil
}

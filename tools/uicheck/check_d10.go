package main

import "fmt"

// d10: no-scroll patch section 1, "nothing in this chart except the
// stacked bars, the y-scale if any, and the HH:mm axis" - a uicheck case
// that finds no marker or line elements in the burn chart. The turn-marker
// overlay SVG (#turnMarkers) is gone from the DOM outright, so this checks
// both that it stays gone and that #burnBars never grows an <svg>/<line>
// some future change might reintroduce.
func caseD10(hwnd uintptr, args []string) error {
	if err := ensureWindowSizeWH(hwnd, 1280, 860); err != nil {
		return err
	}
	if exists, err := evalBool(`!!document.getElementById('turnMarkers')`); err != nil {
		return fmt.Errorf("check #turnMarkers absent: %w", err)
	} else if exists {
		return fmt.Errorf("#turnMarkers still exists in the DOM; section 1 was supposed to remove the turn-marker overlay entirely")
	}
	if n, err := evalRaw(`document.querySelectorAll('#burnBars line, #burnBars svg').length`); err != nil {
		return fmt.Errorf("check #burnBars for marker/line elements: %w", err)
	} else if string(n) != "0" {
		return fmt.Errorf("found %s marker/line element(s) inside #burnBars; section 1 wants bars only", n)
	}
	fmt.Println("uicheck: d10: burn chart has no marker/line elements")
	if _, err := screenshot(hwnd, "d10-burn-chart-no-markers"); err != nil {
		return err
	}
	return nil
}

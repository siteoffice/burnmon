package main

import (
	"fmt"
	"time"
)

// d13: WS2 alpha.3 item 2, "all 20 core bars the same length" - a uicheck
// case that measures every .corebar's own clientWidth at a real window size
// and fails unless every one of the 20 bars is exactly the same width. Before
// the fixed left/right label slots (.corelabel-l/.corelabel-r, page.html),
// each bar's track length depended on its own value text's width ("0%" vs
// "100%"), so min and max differed; this proves they no longer do.
func caseD13(hwnd uintptr, args []string) error {
	if err := ensureWindowSizeWH(hwnd, 1280, 860); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)

	var widths []float64
	script := `(function(){
  var bars = document.querySelectorAll('.corebar');
  var out = [];
  for (var i = 0; i < bars.length; i++) out.push(bars[i].clientWidth);
  return out;
})()`
	if err := evalInto(script, &widths); err != nil {
		return fmt.Errorf("d13: eval .corebar widths: %w", err)
	}
	if len(widths) == 0 {
		return fmt.Errorf("d13: no .corebar elements found")
	}
	min, max := widths[0], widths[0]
	for _, w := range widths {
		if w < min {
			min = w
		}
		if w > max {
			max = w
		}
	}
	fmt.Printf("uicheck: d13: %d core bars, width min=%.0fpx max=%.0fpx\n", len(widths), min, max)
	if min != max {
		return fmt.Errorf("d13: core bar widths are not equal (min=%.0fpx max=%.0fpx, want equal across all %d bars)", min, max, len(widths))
	}
	return nil
}

package main

import (
	"fmt"
	"time"
)

// d12: headline patch section 1 - a uicheck case that fails when any turn
// ticker row is shorter than its own text line height, the exact regression
// 11_ticker_broken.png showed (cropToFit's own scrollHeight<=clientHeight
// check never trips because flexbox just shrinks each .trow to fit instead
// of letting the ticker's own box overflow, so every row silently squeezes
// to a sliver - only the amber tag's top border and a few dashes survive).
// Sweeps the same five sizes d9 already uses, since #turnTicker's own fixed
// row height and scroll behaviour (section 1) do not depend on window size.
func caseD12(hwnd uintptr, args []string) error {
	sizes := [][2]int32{{1024, 768}, {1280, 860}, {1152, 2048}, {1920, 1080}, {2560, 1440}}
	for _, size := range sizes {
		if err := ensureWindowSizeWH(hwnd, size[0], size[1]); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)

		var result struct {
			RowCount   int      `json:"rowCount"`
			ContainerH float64  `json:"containerH"`
			Bad        []string `json:"bad"`
		}
		script := `(function(){
  var el = document.getElementById('turnTicker');
  var rows = el ? el.querySelectorAll('.trow') : [];
  var bad = [];
  for (var i = 0; i < rows.length; i++) {
    var r = rows[i];
    var h = r.getBoundingClientRect().height;
    var lh = parseFloat(getComputedStyle(r).lineHeight);
    if (isNaN(lh)) continue;
    if (h < lh - 1) {
      bad.push('row ' + i + ': height=' + h.toFixed(1) + ' lineHeight=' + lh.toFixed(1));
    }
  }
  return { rowCount: rows.length, containerH: el ? el.clientHeight : 0, bad: bad };
})()`
		if err := evalInto(script, &result); err != nil {
			return fmt.Errorf("d12 %dx%d: eval ticker row heights: %w", size[0], size[1], err)
		}
		// A zero (or near-zero) row count means this run's own store had
		// no turns in the last 30 minutes to check against - the check
		// would otherwise silently pass having verified nothing (found
		// by review, 2026-09-26).
		if result.RowCount == 0 {
			return fmt.Errorf("d12 %dx%d: no ticker rows found to check (no turns in the last 30 minutes?)", size[0], size[1])
		}
		// A collapsed #turnTicker box (0 or near-0 clientHeight) would
		// show nothing regardless of each row's own individually-correct
		// height, since overflow-y:auto's own scrollable content still
		// reports full row heights even while none of it is visible
		// (found by review, 2026-09-26).
		if result.ContainerH < 20 {
			return fmt.Errorf("d12 %dx%d: #turnTicker itself is only %.1fpx tall (less than one row) despite %d row(s)", size[0], size[1], result.ContainerH, result.RowCount)
		}
		if len(result.Bad) > 0 {
			max := len(result.Bad)
			if max > 10 {
				max = 10
			}
			return fmt.Errorf("d12 %dx%d: %d ticker row(s) shorter than their own text line height: %v", size[0], size[1], len(result.Bad), result.Bad[:max])
		}
		fmt.Printf("uicheck: d12 %dx%d: %d ticker row(s) (container %.0fpx), none shorter than their own text line height\n", size[0], size[1], result.RowCount, result.ContainerH)
	}
	return nil
}

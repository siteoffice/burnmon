package main

import (
	"fmt"
	"time"
)

// d14: WS2 follow-up (2026-09-28), month labels - a uicheck case that fails
// if any rendered .heatmonth label is clipped by its own box (scrollWidth
// greater than clientWidth) at 1920x1080, the one size wide enough to
// render most of the heatmap's columns. alpha.3's own fix made check_d9
// pass by clipping full month names ("Mar" -> "Ma") with overflow:hidden on
// a fixed 9px box; this proves the real bug (a label wider than its box) is
// gone, not just hidden from d9's scrollbar sweep.
func caseD14(hwnd uintptr, args []string) error {
	if err := ensureWindowSizeWH(hwnd, 1920, 1080); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)

	var labels []struct {
		Text        string  `json:"text"`
		ScrollWidth float64 `json:"scrollWidth"`
		ClientWidth float64 `json:"clientWidth"`
	}
	script := `(function(){
  var els = document.querySelectorAll('.heatmonth');
  var out = [];
  for (var i = 0; i < els.length; i++) {
    var el = els[i];
    if (!el.textContent) continue;
    out.push({ text: el.textContent, scrollWidth: el.scrollWidth, clientWidth: el.clientWidth });
  }
  return out;
})()`
	if err := evalInto(script, &labels); err != nil {
		return fmt.Errorf("d14: eval .heatmonth labels: %w", err)
	}
	if len(labels) == 0 {
		return fmt.Errorf("d14: no rendered .heatmonth labels found")
	}
	var offenders []string
	for _, l := range labels {
		if l.ScrollWidth > l.ClientWidth+1 {
			offenders = append(offenders, fmt.Sprintf("%q (scrollWidth=%.0f clientWidth=%.0f)", l.Text, l.ScrollWidth, l.ClientWidth))
		}
	}
	if len(offenders) > 0 {
		return fmt.Errorf("d14: %d of %d label(s) clipped: %v", len(offenders), len(labels), offenders)
	}
	fmt.Printf("uicheck: d14: %d rendered month label(s), none clipped\n", len(labels))
	return nil
}

package main

import (
	"fmt"
	"time"
)

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
func init() {
	checks["d9"] = func(hwnd uintptr, args []string) error {
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
    var vOver = el.scrollHeight > el.clientHeight + 1 && cs.overflowY !== 'hidden';
    var hOver = el.scrollWidth > el.clientWidth + 1 && cs.overflowX !== 'hidden';
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
}

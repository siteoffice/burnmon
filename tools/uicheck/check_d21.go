package main

import (
	"fmt"
	"time"
)

// d21: WS2 alpha.7, never skip forever (02_roadmap\2026-09-29_ws2_smooth
// _tick_and_system_layout.md item 1.3, plus the lost-answer freeze the
// 90-minute after run found on 2026-09-29): one bdevSnapshotNow call is made
// to never answer, the way a resolve lost in go-webview2's dispatch queue
// never answers, and the page must give up on it and paint again within
// IN_FLIGHT_GIVE_UP ticks plus one, instead of waiting on it forever. Real
// ticks, not fake mode: the wrapper only swallows the first call it sees,
// every later call goes to the real binding.
func caseD21(hwnd uintptr, args []string) error {
	setup := `(function(){
  if(!window.__bdevRealSnapshotNow) window.__bdevRealSnapshotNow = window.bdevSnapshotNow;
  var swallowed = false;
  window.__bdevSwallowAt = 0;
  window.bdevSnapshotNow = function(c){
    if(!swallowed){ swallowed = true; window.__bdevSwallowAt = performance.now(); return new Promise(function(){}); }
    return window.__bdevRealSnapshotNow(c);
  };
  return true;
})()`
	if _, err := evalRaw(setup); err != nil {
		return fmt.Errorf("d21: install wrapper: %w", err)
	}
	defer evalRaw(`(function(){ if(window.__bdevRealSnapshotNow) window.bdevSnapshotNow = window.__bdevRealSnapshotNow; })()`)
	time.Sleep(6 * time.Second)

	var out struct {
		SwallowAt  float64 `json:"swallowAt"`
		FirstAfter float64 `json:"firstAfter"`
		Refresh    float64 `json:"refresh"`
		Count      int     `json:"count"`
	}
	read := `(function(){
  var at = window.__bdevSwallowAt, first = 0, n = 0;
  (window.__bdevPaintLog || []).forEach(function(e){
    if(e.name === 'tickEnd' && e.t > at){ n++; if(!first) first = e.t; }
  });
  return { swallowAt: at, firstAfter: first, refresh: window.__bdevRefreshMs || 1000, count: n };
})()`
	if err := evalInto(read, &out); err != nil {
		return fmt.Errorf("d21: read paint log: %w", err)
	}
	if out.SwallowAt == 0 {
		return fmt.Errorf("d21: the wrapper never saw a bdevSnapshotNow call in 6s (is the tick running?)")
	}
	if out.FirstAfter == 0 {
		return fmt.Errorf("d21: no paint at all in the 6s after one unanswered call: the page still waits on it forever")
	}
	gap := out.FirstAfter - out.SwallowAt
	fmt.Printf("uicheck: d21: first paint %.0fms after the unanswered call (tick %.0fms), %d paints in the rest of the 6s\n", gap, out.Refresh, out.Count)
	// Give-up after 2 ticks, fresh call on the 3rd, paint in its frame.
	if gap > out.Refresh*3+500 {
		return fmt.Errorf("d21: first paint came %.0fms after the unanswered call, want under %.0fms", gap, out.Refresh*3+500)
	}
	fmt.Println("uicheck: d21: an unanswered snapshot call no longer freezes the page")
	return nil
}

package main

// macDOMOnlyCases are the burnmon-dev cases that only use evalRaw, evalInto,
// evalBool and fixtures (no Win32 screenshot, click, key press, drag or
// window sizing helper), so they can run on macOS. scripts/maccheck.sh holds
// the same list in DOM_CASES; a test keeps the two equal.
var macDOMOnlyCases = []string{"charts/d11", "charts/d13", "charts/d14", "charts/d21", "waits/d45"}

package main

// lampBindingNames is every function the lamp window registers for lamp.html
// (T5 binds them). TestLampBindingNamesEqualPageCalls checks it equals the
// names the page calls. The page never calls a BurnMon Dev binding.
//
//   - lampState() returns {state, theme}: the lamp.State and "dark" or "light".
//   - lampGlim(mood, bricks, theme) returns the SVG of lamp.Glim, so the lamp
//     never draws the character twice.
//   - lampNextLine() returns the next line for the same mood.
//   - lampOpenDev() brings BurnMon Dev to the front, or starts it.
//   - lampSize(w, h) asks the window for a size in CSS pixels.
//   - lampArea(json) reports the rectangles the window must keep, in CSS pixels:
//     the ticker, Glim's box and, while an act needs room, one more box.
var lampBindingNames = []string{
	"lampArea",
	"lampGlim",
	"lampNextLine",
	"lampOpenDev",
	"lampSize",
	"lampState",
}

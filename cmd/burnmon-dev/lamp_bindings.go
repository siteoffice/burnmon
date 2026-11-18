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
//     the ticker (measured from the element), Glim's box, while an act needs
//     room one more box, and the close menu while it is open.
//   - lampDragStart() starts the drag: the lamp process follows the pointer
//     until the button is up, then snaps and saves lamp_monitor and lamp_position.
//   - lampClose() closes the lamp from its menu.
//   - lampSwing(maxDeg, changes) returns {line, hard} for a grab and swing: the
//     line to show and whether Glim was pulled too hard.
//
// The page also defines window.lampPointer(x, y), which the lamp process
// calls with the pointer in CSS pixels; it is not a binding.
var lampBindingNames = []string{
	"lampArea",
	"lampClose",
	"lampDragStart",
	"lampGlim",
	"lampNextLine",
	"lampOpenDev",
	"lampSize",
	"lampState",
	"lampSwing",
}

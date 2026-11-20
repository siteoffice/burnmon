//go:build darwin && cgo

package winbind

/*
#cgo CFLAGS: -x objective-c
#cgo darwin LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// burnmonVisibleSize writes the size of the visible frame of the main screen
// (the screen minus the menu bar and the Dock), in points. It returns 0 when
// there is no main screen.
static int burnmonVisibleSize(double *w, double *h) {
	NSScreen *s = [NSScreen mainScreen];
	if (s == nil) {
		return 0;
	}
	NSRect f = [s visibleFrame];
	*w = f.size.width;
	*h = f.size.height;
	return 1;
}
*/
import "C"

// VisibleSize is the size of the visible frame of the main screen, or 0, 0
// when macOS gives no main screen.
func VisibleSize() (int, int) {
	var w, h C.double
	if C.burnmonVisibleSize(&w, &h) == 0 {
		return 0, 0
	}
	return int(w), int(h)
}

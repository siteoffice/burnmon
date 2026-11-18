//go:build windows

package main

import "testing"

// The first-show focus try: on Windows the lamp child starts hidden so the
// window shows first through SetWindowPos without activation.
func TestLampChildAttrHidesOnWindows(t *testing.T) {
	a := lampChildAttr()
	if a == nil || !a.HideWindow {
		t.Errorf("lampChildAttr = %+v, want HideWindow on Windows", a)
	}
}

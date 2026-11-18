//go:build darwin && cgo

package main

import "testing"

// The Mac has no first-show focus try: the child attributes stay nil.
func TestLampChildAttrNilOnDarwin(t *testing.T) {
	if a := lampChildAttr(); a != nil {
		t.Errorf("lampChildAttr = %+v, want nil on the Mac", a)
	}
}

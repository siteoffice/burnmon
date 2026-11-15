//go:build windows

package main

import "testing"

// toPhysical maps a CSS point into screen pixels with the measured ratio of
// physical client width to CSS viewport width (2026-11-15: at 200% on the
// laptop panel the old 1:1 mapping clicked at half the distance).
func TestToPhysical(t *testing.T) {
	o := point{X: 100, Y: 50}
	cases := []struct {
		name       string
		physW      int32
		cssW       float64
		cssX, cssY float64
		wantX      int32
		wantY      int32
	}{
		{"100 percent", 1280, 1280, 640, 20, 740, 70},
		{"200 percent", 2330, 1165, 1100, 22, 2300, 94},
		{"150 percent", 1920, 1280, 10.5, 10.5, 116, 66},
		{"no viewport width falls back to 1:1", 1280, 0, 640, 20, 740, 70},
		{"no client width falls back to 1:1", 0, 1280, 640, 20, 740, 70},
	}
	for _, c := range cases {
		x, y := toPhysical(o, c.physW, c.cssW, c.cssX, c.cssY)
		if x != c.wantX || y != c.wantY {
			t.Errorf("%s: (%d,%d), want (%d,%d)", c.name, x, y, c.wantX, c.wantY)
		}
	}
}

//go:build windows || darwin

package main

import (
	"math"
	"regexp"
	"strconv"
	"testing"
)

// hexLum is the WCAG 2 relative luminance of a #rrggbb colour (the method
// of cmd\burnmon-dev\contrast_test.go).
func hexLum(t *testing.T, hex string) float64 {
	t.Helper()
	if len(hex) != 7 || hex[0] != '#' {
		t.Fatalf("not a #rrggbb colour: %q", hex)
	}
	ch := func(s string) float64 {
		v, err := strconv.ParseUint(s, 16, 8)
		if err != nil {
			t.Fatalf("bad hex %q: %v", hex, err)
		}
		c := float64(v) / 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(hex[1:3]) + 0.7152*ch(hex[3:5]) + 0.0722*ch(hex[5:7])
}

// hexRGB returns the 0 to 255 channels of a #rrggbb colour.
func hexRGB(t *testing.T, hex string) [3]float64 {
	t.Helper()
	if len(hex) != 7 || hex[0] != '#' {
		t.Fatalf("not a #rrggbb colour: %q", hex)
	}
	var out [3]float64
	for i := range out {
		v, err := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		if err != nil {
			t.Fatalf("bad hex %q: %v", hex, err)
		}
		out[i] = float64(v)
	}
	return out
}

// rgbLum is hexLum for 0 to 255 channels that need not be whole numbers
// (a blend of a translucent fill over a surface).
func rgbLum(c [3]float64) float64 {
	ch := func(v float64) float64 {
		v /= 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c[0]) + 0.7152*ch(c[1]) + 0.0722*ch(c[2])
}

func contrast(t *testing.T, a, b string) float64 {
	la, lb := hexLum(t, a), hexLum(t, b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// TestPageContrast: text, muted, warning and accent tokens hold at least
// 4.5:1 on the page and both surfaces, light and dark, and --accent holds it
// on --accent-fill laid over both surfaces; the system-dark block equals the
// explicit dark block.
func TestPageContrast(t *testing.T) {
	blocks := map[string]string{
		"light": ruleFor(t, ":root"),
		"dark":  ruleFor(t, `:root[data-theme="dark"]`),
	}
	media := ruleFor(t, `:root:not([data-theme="light"])`)
	if compact(media) != compact(blocks["dark"]) {
		t.Errorf("the prefers-color-scheme dark block differs from the data-theme dark block\nmedia: %s\n dark: %s", compact(media), compact(blocks["dark"]))
	}
	tok := func(block, name string) string {
		m := regexp.MustCompile(`--` + name + `:\s*(#[0-9a-fA-F]{6})`).FindStringSubmatch(block)
		if m == nil {
			t.Fatalf("token --%s not found in the block", name)
		}
		return m[1]
	}
	for theme, block := range blocks {
		// Selected states (menu item, pressed theme button, selected row or
		// block) draw --accent on --accent-fill laid over a surface.
		m := regexp.MustCompile(`--accent-fill:\s*rgba\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*([0-9.]+)\s*\)`).FindStringSubmatch(block)
		if m == nil {
			t.Fatalf("%s: --accent-fill is not an rgba() value", theme)
		}
		var fill [3]float64
		for i := range fill {
			v, _ := strconv.ParseFloat(m[i+1], 64)
			fill[i] = v
		}
		alpha, _ := strconv.ParseFloat(m[4], 64)
		acc := hexRGB(t, tok(block, "accent"))
		if fill != acc {
			t.Errorf("%s: --accent-fill rgb %v is not the --accent hue %v", theme, fill, acc)
		}
		for _, bg := range []string{"surface", "surface2"} {
			under := hexRGB(t, tok(block, bg))
			var mix [3]float64
			for i := range mix {
				mix[i] = alpha*fill[i] + (1-alpha)*under[i]
			}
			la, lb := rgbLum(acc), rgbLum(mix)
			if la < lb {
				la, lb = lb, la
			}
			r := (la + 0.05) / (lb + 0.05)
			t.Logf("%s: --accent on --accent-fill over --%s = %.2f:1", theme, bg, r)
			if r < 4.5 {
				t.Errorf("%s: --accent on --accent-fill over --%s is %.2f:1, want at least 4.5:1", theme, bg, r)
			}
		}
		for _, fg := range []string{"text", "muted", "amber", "accent"} {
			for _, bg := range []string{"bg", "surface", "surface2"} {
				r := contrast(t, tok(block, fg), tok(block, bg))
				t.Logf("%s: --%s %s on --%s %s = %.2f:1", theme, fg, tok(block, fg), bg, tok(block, bg), r)
				if r < 4.5 {
					t.Errorf("%s: --%s on --%s is %.2f:1, want at least 4.5:1", theme, fg, bg, r)
				}
			}
		}
	}
}

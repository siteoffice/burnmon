//go:build windows

package main

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// hexLum is the WCAG 2 relative luminance of a #rrggbb colour.
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

func contrast(t *testing.T, a, b string) float64 {
	la, lb := hexLum(t, a), hexLum(t, b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// TestBd2Contrast: the map's borders and lines hold at least 3:1 (WCAG 2
// graphics) against the page, panel and frame backgrounds, dark and light.
func TestBd2Contrast(t *testing.T) {
	darkStart := strings.Index(pageHTML, ":root{")
	lightStart := strings.Index(pageHTML, `:root[data-theme="light"]{`)
	if darkStart < 0 || lightStart < 0 || lightStart < darkStart {
		t.Fatal("page.html lacks the dark and light :root blocks")
	}
	blocks := map[string]string{
		"dark":  pageHTML[darkStart:lightStart],
		"light": pageHTML[lightStart : lightStart+strings.Index(pageHTML[lightStart:], "}")],
	}
	tok := func(block, name string) string {
		m := regexp.MustCompile(`--` + name + `:(#[0-9a-fA-F]{6})`).FindStringSubmatch(block)
		if m == nil {
			t.Fatalf("token --%s not found in the block", name)
		}
		return m[1]
	}
	for theme, block := range blocks {
		bd2 := tok(block, "bd2")
		for _, bg := range []string{"bg", "panel", "panel3"} {
			r := contrast(t, bd2, tok(block, bg))
			t.Logf("%s: --bd2 %s on --%s %s = %.2f:1", theme, bd2, bg, tok(block, bg), r)
			if r < 3.0 {
				t.Errorf("%s: --bd2 on --%s is %.2f:1, want at least 3:1", theme, bg, r)
			}
		}
	}
}

func TestPageMapUsesBd2(t *testing.T) {
	i := strings.Index(pageHTML, ".mapbox{")
	if i < 0 {
		t.Fatal("page.html has no .mapbox rule")
	}
	rule := pageHTML[i : i+strings.Index(pageHTML[i:], "}")]
	if !strings.Contains(rule, "border:1px solid var(--bd2)") {
		t.Errorf(".mapbox rule does not use var(--bd2): %q", rule)
	}
	if !strings.Contains(pageHTML, ".mapline.dotted{") {
		t.Error("page.html lacks .mapline.dotted")
	}
	s := strings.Index(pageHTML, "function mapRender(){")
	e := strings.Index(pageHTML, "function mapIdleHTML(")
	if s < 0 || e < s {
		t.Fatal("cannot find mapRender")
	}
	body := pageHTML[s:e]
	for _, want := range []string{"column_notes", ".depth", "b.tag", "ln.style"} {
		if !strings.Contains(body, want) {
			t.Errorf("mapRender does not read %q", want)
		}
	}
}

func TestPageMapHeaderCount(t *testing.T) {
	s := strings.Index(pageHTML, "function mapRender(){")
	e := strings.Index(pageHTML, "function mapIdleHTML(")
	body := pageHTML[s:e]
	if !strings.Contains(body, "c.waiting") || !strings.Contains(body, "c.your_turn") {
		t.Error("the #mapCount text is not built from counts.waiting and counts.your_turn")
	}
}

//go:build windows

package main

import (
	"regexp"
	"strings"
	"testing"
)

// These tests read lamp.html as text: nothing here is seen in a window.

// ruleBody returns the declarations of the first CSS rule that starts with sel.
func ruleBody(t *testing.T, sel string) string {
	t.Helper()
	i := strings.Index(lampHTML, sel+"{")
	if i < 0 {
		t.Fatalf("lamp.html has no %s rule", sel)
	}
	j := strings.Index(lampHTML[i:], "}")
	return lampHTML[i+len(sel)+1 : i+j]
}

func TestLampPageNoEmDashNoOutsideResource(t *testing.T) {
	if strings.ContainsRune(lampHTML, 0x2014) {
		t.Error("lamp.html holds an em dash")
	}
	for _, bad := range []string{"http://", "https://", "//", "@import", "url(", "src=", "href=", "fetch(", "XMLHttpRequest", "<link", "@font-face", "<img", "<script src"} {
		if strings.Contains(lampHTML, bad) {
			t.Errorf("lamp.html holds %q, which can pull in an outside resource", bad)
		}
	}
}

func TestLampPageReducedMotionAndKeyframes(t *testing.T) {
	i := strings.Index(lampHTML, "@media (prefers-reduced-motion:reduce)")
	if i < 0 {
		t.Fatal("lamp.html has no prefers-reduced-motion block")
	}
	block := lampHTML[i:]
	if j := strings.Index(block, "}}"); j > 0 {
		block = block[:j]
	}
	if !strings.Contains(block, "animation:none") {
		t.Error("the reduced-motion block does not stop the animation")
	}
	for _, k := range []string{"blink", "glow", "pulse", "drop"} {
		if !strings.Contains(lampHTML, "@keyframes "+k) {
			t.Errorf("lamp.html lacks @keyframes %s", k)
		}
	}
	// The animations name the classes Glim carries.
	for _, c := range []string{".glim-eye", ".glim-ray", ".glim-brick", ".glim-dome"} {
		if !strings.Contains(lampHTML, c) {
			t.Errorf("lamp.html does not style %s", c)
		}
	}
	// Nothing animates in the sliver.
	if !strings.Contains(ruleBody(t, ".sliver"), "animation:none") {
		t.Error(".sliver rule does not say animation:none")
	}
}

func TestLampPageNoScrollbar(t *testing.T) {
	re := regexp.MustCompile(`overflow(-x|-y)?\s*:\s*(auto|scroll)`)
	if m := re.FindString(lampHTML); m != "" {
		t.Errorf("lamp.html sets %q: no scrollbar is allowed (UI standard 11)", m)
	}
	if !strings.Contains(ruleBody(t, "html,body"), "overflow:hidden") {
		t.Error("html,body must set overflow:hidden")
	}
}

func TestLampPageSizes(t *testing.T) {
	for sel, want := range map[string][]string{
		".sliver": {"width:120px", "height:6px"},
		".pill":   {"width:320px", "height:40px"},
		".opened": {"width:440px"},
		".lrow":   {"height:28px"},
	} {
		body := ruleBody(t, sel)
		for _, w := range want {
			if !strings.Contains(body, w) {
				t.Errorf("%s rule lacks %s: %q", sel, w, body)
			}
		}
	}
	// opened pill: 40 px plus 28 px per row.
	if !strings.Contains(lampHTML, "40+28*") {
		t.Error("the opened pill height is not 40+28*rows")
	}
}

func TestLampPageModesAndInteraction(t *testing.T) {
	m := regexp.MustCompile(`var SLIVER=\{([^}]*)\}`).FindStringSubmatch(lampHTML)
	if m == nil {
		t.Fatal("lamp.html has no SLIVER table")
	}
	for _, mood := range []string{"asleep", "working", "off"} {
		if !strings.Contains(m[1], mood) {
			t.Errorf("SLIVER lacks %s", mood)
		}
	}
	for _, mood := range []string{"done", "your_turn", "blocked", "bored", "sweating"} {
		if strings.Contains(m[1], mood) {
			t.Errorf("SLIVER holds %s, which shows the pill", mood)
		}
	}
	for _, want := range []string{
		"HOVER_LINGER_MS=2000", "setInterval(", ",1000)", "mouseenter", "mouseleave",
		"lampNextLine(", "dblclick", "lampOpenDev(", "lampState(", "lampGlim(", "lampSize(",
		"window.lampRender=function", "window.lampFreeze",
		"w=120;h=6", "w=320;h=40", "w=440;h=40+28*rows.length", "classList.add('blink')",
		"e.target.closest('#gl')||e.target.closest('.lrow')", "html!==lastHTML",
	} {
		if !strings.Contains(lampHTML, want) {
			t.Errorf("lamp.html lacks %q", want)
		}
	}
	if !strings.Contains(lampHTML, `class="demo">demo<`) {
		t.Error("the pill does not show the word demo for a demo state")
	}
}

// The lamp never takes the focus (amendment A5).
func TestLampPageNeverFocuses(t *testing.T) {
	for _, bad := range []string{".focus(", "autofocus", "tabindex", "window.focus", "requestFullscreen"} {
		if strings.Contains(lampHTML, bad) {
			t.Errorf("lamp.html holds %q", bad)
		}
	}
}

// A hover title carries the full text of every cut text and the exact wait.
func TestLampPageHoverTitles(t *testing.T) {
	for _, want := range []string{"title_full", "wait_exact", `class="head" title="`, `class="sub" title="`,
		`class="ltitle" title="`, `class="lwait" title="`} {
		if !strings.Contains(lampHTML, want) {
			t.Errorf("lamp.html lacks %q", want)
		}
	}
	if !strings.Contains(ruleBody(t, ".ltitle"), "text-overflow:ellipsis") {
		t.Error(".ltitle must cut with an ellipsis")
	}
}

// The opened pill shows tokens a minute first, then the 5 minute cost with its label.
func TestLampPageTokensBeforeCost(t *testing.T) {
	s := strings.Index(lampHTML, "function openedHTML(")
	if s < 0 {
		t.Fatal("lamp.html has no openedHTML")
	}
	body := lampHTML[s:]
	tok := strings.Index(body, "burn_rate")
	cost := strings.Index(body, "cost_5min")
	if tok < 0 || cost < 0 || tok > cost {
		t.Errorf("tokens a minute (%d) must come before the cost (%d)", tok, cost)
	}
	if !strings.Contains(body, "cost_5min.label") {
		t.Error("the cost is shown without its label")
	}
	if !strings.Contains(body, "tokens a minute") {
		t.Error("the tokens a minute label is missing")
	}
}

// Text on its surface is at least 4.5:1 in both themes (method of contrast_test.go).
func TestLampPageContrast(t *testing.T) {
	dark := strings.Index(lampHTML, ":root{")
	light := strings.Index(lampHTML, `:root[data-theme="light"]{`)
	if dark < 0 || light < dark {
		t.Fatal("lamp.html lacks the dark and light :root blocks")
	}
	blocks := map[string]string{
		"dark":  lampHTML[dark:light],
		"light": lampHTML[light : light+strings.Index(lampHTML[light:], "}")],
	}
	tok := func(block, name string) string {
		m := regexp.MustCompile(`--` + name + `:(#[0-9a-fA-F]{6})`).FindStringSubmatch(block)
		if m == nil {
			t.Fatalf("token --%s not found", name)
		}
		return m[1]
	}
	for theme, block := range blocks {
		for _, pair := range [][2]string{{"fg", "bg"}, {"fg", "panel"}, {"muted", "bg"}, {"muted", "panel"}, {"accent", "bg"}, {"accent", "panel"}, {"demo", "bg"}, {"demo", "panel"}} {
			r := contrast(t, tok(block, pair[0]), tok(block, pair[1]))
			if r < 4.5 {
				t.Errorf("%s: --%s on --%s is %.2f:1, want at least 4.5:1", theme, pair[0], pair[1], r)
			}
		}
	}
	if !strings.Contains(lampHTML, `data-theme`) {
		t.Error("the page does not set data-theme from the state")
	}
}

// Dark is amber on black.
func TestLampPageDarkIsAmberOnBlack(t *testing.T) {
	i, j := strings.Index(lampHTML, ":root{"), strings.Index(lampHTML, `:root[data-theme="light"]{`)
	if i < 0 || j < i {
		t.Fatal("lamp.html lacks the dark and light :root blocks")
	}
	dark := lampHTML[i:j]
	if !strings.Contains(dark, "--bg:#000000") {
		t.Error("dark --bg is not black")
	}
	if !strings.Contains(dark, "--accent:#ffc766") {
		t.Error("dark --accent is not the amber of Glim")
	}
}

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

// Every act and every state that lasts has its keyframes, and so has the idle life.
func TestLampPageKeyframesForEveryActAndState(t *testing.T) {
	for _, k := range []string{
		"alarm", "knock", "stamp", "welcome", "wake", "slip", "coffee", "peek",
		"wall", "sweat", "sleep", "dots",
		"sway", "breathe", "blink",
	} {
		if !strings.Contains(lampHTML, "@keyframes "+k+"{") {
			t.Errorf("lamp.html lacks @keyframes %s", k)
		}
	}
	// The act and hold classes the script sets are styled: a-<act> and h-<hold>.
	for _, c := range []string{
		".a-alarm", ".a-knock", ".a-stamp", ".a-welcome", ".a-wake", ".a-slip", ".a-coffee", ".a-peek",
		".h-wall", ".h-sweat", ".h-sleep", ".h-dots",
	} {
		if !strings.Contains(lampHTML, c+" ") && !strings.Contains(lampHTML, c+".") && !strings.Contains(lampHTML, c+",") {
			t.Errorf("lamp.html styles no %s", c)
		}
	}
	for _, want := range []string{"'a-'+", "'h-'+", "act_seq", "holds", "dot_seconds"} {
		if !strings.Contains(lampHTML, want) {
			t.Errorf("lamp.html lacks %q", want)
		}
	}
	// The rig is animated by the class names of lamp.Glim.
	for _, c := range []string{".swing", ".body", ".lid", ".pup", ".arm", ".rays", ".drop", ".zz", ".plank", ".brick", ".mug", ".dot", ".stampi", ".rip", ".eyes"} {
		if !strings.Contains(lampHTML, c) {
			t.Errorf("lamp.html does not style %s", c)
		}
	}
}

// Fix lane T2, read as text, nothing seen. The window area comes from the
// ticker element's own rectangle (border included), a pointer down on the
// ticker that moves more than 6 px starts the drag in the lamp process, a
// right-click shows the one item menu that closes the lamp, and the eyes take
// the pointer the lamp process feeds them.
func TestLampPageFixLaneWindowWork(t *testing.T) {
	for _, want := range []string{
		"getBoundingClientRect",            // the area is measured, not computed
		"el('tick').getBoundingClientRect", // from the ticker itself
		"DRAG_PX=6",
		"lampDragStart(",
		"'contextmenu'",
		"Close the lamp",
		"lampClose(",
		"window.lampPointer=",
		"held",
	} {
		if !strings.Contains(lampHTML, want) {
			t.Errorf("lamp.html lacks %q", want)
		}
	}
	// The ticker area is no longer worked out from rows times the row height.
	if strings.Contains(lampHTML, "ticker:{x:pad,y:0,w:TICKER_W,h:tH}") {
		t.Error("the ticker area is still computed, not measured")
	}
	// A drag never takes the keyboard focus, and neither does the menu.
	for _, bad := range []string{".focus(", "tabindex", "<input", "<button", "<select"} {
		if strings.Contains(lampHTML, bad) {
			t.Errorf("lamp.html holds %q, which can take the keyboard focus", bad)
		}
	}
	// The menu closes when the pointer leaves it, and is inside the window area.
	for _, want := range []string{"id=\"cmenu\"", "mouseleave"} {
		if !strings.Contains(lampHTML, want) {
			t.Errorf("lamp.html lacks %q", want)
		}
	}
	if !strings.Contains(lampHTML, "menu:") {
		t.Error("the area does not carry the open menu")
	}
}

func TestLampPageMotionAndReducedMotion(t *testing.T) {
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
	// lamp_motion: lively plays everything, calm only the blink, off still poses.
	for _, want := range []string{"look.motion", "'lively'", ".m-calm", ".m-off"} {
		if !strings.Contains(lampHTML, want) {
			t.Errorf("lamp.html lacks %q", want)
		}
	}
	// An act is applied only in lively.
	if !strings.Contains(lampHTML, "motion==='lively'") {
		t.Error("the script does not limit acts to lamp_motion lively")
	}
	// Nothing animates in the sliver.
	if !strings.Contains(ruleBody(t, ".sliver"), "animation:none") {
		t.Error(".sliver rule does not say animation:none")
	}
	// Calm keeps the blink and drops the rest; off drops all.
	if !strings.Contains(lampHTML, ".m-off *{animation:none !important}") {
		t.Error("m-off does not stop every animation")
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

// The ticker is 300 px wide with 26 px rows and rounded bottom corners; the sliver is 120 by 6.
func TestLampPageTickerSizes(t *testing.T) {
	for sel, want := range map[string][]string{
		".sliver": {"width:120px", "height:6px"},
		".ticker": {"width:300px", "border-radius:0 0 8px 8px"},
		".trow":   {"height:26px"},
	} {
		body := ruleBody(t, sel)
		for _, w := range want {
			if !strings.Contains(body, w) {
				t.Errorf("%s rule lacks %s: %q", sel, w, body)
			}
		}
	}
	for _, want := range []string{
		"TICKER_W=300", "ROW_H=26", "OPEN_ITEMS=6",
		"small:1.2", "medium:1.5", "large:1.9",
		"w=120;h=6", "ticker_rows", "ROW_H*",
	} {
		if !strings.Contains(lampHTML, want) {
			t.Errorf("lamp.html lacks %q", want)
		}
	}
	// 30+ in the ticker when the plank is full.
	if !strings.Contains(lampHTML, "wall_over") || !strings.Contains(lampHTML, "30+") {
		t.Error("the ticker does not show 30+ for a full plank")
	}
}

func TestLampPageModesAndInteraction(t *testing.T) {
	for _, want := range []string{
		"HOVER_LINGER_MS=2000", "setInterval(", ",1000)", "mouseenter", "mouseleave",
		"lampNextLine(", "dblclick", "lampOpenDev(", "lampState(", "lampGlim(", "lampSize(", "lampArea(",
		"window.lampRender=function", "window.lampFreeze",
		"p.quiet", "look.quiet", "look.size", "look.theme", "look.backdrop",
		"e.target.closest('#hang')||e.target.closest('.trow')",
	} {
		if !strings.Contains(lampHTML, want) {
			t.Errorf("lamp.html lacks %q", want)
		}
	}
	if !strings.Contains(lampHTML, `class="demo">demo<`) {
		t.Error("the ticker does not show the word demo for a demo state")
	}
	// A quiet lamp that hides still lets peek and coffee bring Glim down.
	for _, want := range []string{"'peek'", "'coffee'"} {
		if !strings.Contains(lampHTML, want) {
			t.Errorf("lamp.html does not bring Glim down for %s", want)
		}
	}
}

// The page works out no line, no group and no count: lines come from the payload.
func TestLampPageTakesLinesFromThePayload(t *testing.T) {
	if !strings.Contains(lampHTML, "act_lines") {
		t.Error("the page does not read act_lines")
	}
	for _, bad := range []string{"Whoa", "Stamped", "Welcome back", "Coffee", "Somebody started", "testing the rail"} {
		if strings.Contains(lampHTML, bad) {
			t.Errorf("lamp.html holds the line text %q: lines come from act_lines", bad)
		}
	}
}

// Glim's eyes follow the pointer while it is inside the lamp window.
func TestLampPageEyesFollowThePointer(t *testing.T) {
	for _, want := range []string{"mousemove", "--px", "--py", "getBoundingClientRect"} {
		if !strings.Contains(lampHTML, want) {
			t.Errorf("lamp.html lacks %q", want)
		}
	}
	// Calm and off keep the pupils still; hover opens only over the ticker, Glim or the sliver.
	if !strings.Contains(lampHTML, "motionNow!=='lively'") {
		t.Error("the pupils follow the pointer in calm and off")
	}
	if !strings.Contains(lampHTML, "['tick','hang','sliver'].forEach") || strings.Contains(lampHTML, "root.addEventListener('mouseenter'") {
		t.Error("hover must open from the ticker, Glim and the sliver only")
	}
	if !strings.Contains(lampHTML, ".pup{") {
		t.Error("the pupils do not read --px and --py")
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

// The opened ticker shows tokens a minute first, then the 5 minute cost with its label.
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

// The backdrop: clear shows only the ticker, Glim and what an act adds; card puts a panel behind him.
func TestLampPageBackdrop(t *testing.T) {
	if !strings.Contains(lampHTML, `data-backdrop`) {
		t.Fatal("the page does not set data-backdrop from the look")
	}
	if !strings.Contains(lampHTML, `html[data-backdrop="clear"]`) || !strings.Contains(lampHTML, "background:transparent") {
		t.Error("a clear backdrop does not make the page transparent")
	}
	if !strings.Contains(lampHTML, ".cardbg") {
		t.Error("the card backdrop has no panel")
	}
}

// themeBlock returns the declarations of the first rule that starts with sel.
func themeBlock(t *testing.T, sel string) string {
	t.Helper()
	i := strings.Index(lampHTML, sel+"{")
	if i < 0 {
		t.Fatalf("lamp.html lacks the rule %s", sel)
	}
	return lampHTML[i : i+strings.Index(lampHTML[i:], "}")]
}

// The theme follows lamp_theme: system follows prefers-color-scheme, light and dark force it.
// Text on its surface is at least 4.5:1 in both palettes (method of contrast_test.go).
func TestLampPageThemeAndContrast(t *testing.T) {
	light := themeBlock(t, ":root")
	dark := themeBlock(t, `:root[data-theme="dark"]`)
	if !strings.Contains(lampHTML, "@media (prefers-color-scheme:dark){:root[data-theme=\"system\"]{") {
		t.Fatal("the system theme does not follow prefers-color-scheme")
	}
	sys := themeBlock(t, `:root[data-theme="system"]`)
	tok := func(block, name string) string {
		m := regexp.MustCompile(`--` + name + `:(#[0-9a-fA-F]{6})`).FindStringSubmatch(block)
		if m == nil {
			t.Fatalf("token --%s not found", name)
		}
		return m[1]
	}
	for _, name := range []string{"panel", "fg", "muted", "accent", "demo"} {
		if tok(sys, name) != tok(dark, name) {
			t.Errorf("system dark --%s differs from the dark palette", name)
		}
	}
	for theme, block := range map[string]string{"light": light, "dark": dark} {
		for _, pair := range [][2]string{{"fg", "panel"}, {"muted", "panel"}, {"accent", "panel"}, {"demo", "panel"}} {
			if r := contrast(t, tok(block, pair[0]), tok(block, pair[1])); r < 4.5 {
				t.Errorf("%s: --%s on --%s is %.2f:1, want at least 4.5:1", theme, pair[0], pair[1], r)
			}
		}
	}
	for _, want := range []string{"data-theme", "matchMedia('(prefers-color-scheme:dark)')"} {
		if !strings.Contains(lampHTML, want) {
			t.Errorf("lamp.html lacks %q", want)
		}
	}
	// lamp_theme replaces the theme BurnMon Dev saves.
	if strings.Contains(lampHTML, "cur.theme") {
		t.Error("the page still reads the theme of BurnMon Dev")
	}
}

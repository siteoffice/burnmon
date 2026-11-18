//go:build windows

package main

import (
	"regexp"
	"strings"
	"testing"
)

// The size is a slider from 100 to 200 in steps of 5 with the words min, middle
// and max under it and the number beside it; the group names the monitor in use
// and has one button that sends it back to the primary screen.
func TestPageLampGearSizeAndMonitor(t *testing.T) {
	for _, want := range []string{
		`<input type="range" id="secLampSize" class="setin" min="100" max="200" step="5"`,
		`id="secLampSizeNum"`,
		`<span>min</span><span>middle</span><span>max</span>`,
		`id="secLampMonitor"`,
		`id="secLampMonitorBack" class="setbtn" type="button">Back to the primary screen</button>`,
		"window.bdevSetLamp('lamp_monitor', '')",
		"t.id === 'secLampSize'",
	} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page.html lacks %q", want)
		}
	}
	// The gear pane stops every click from bubbling, so the button listens on
	// itself; a poll must not move the slider while it is held; the fallback
	// lamp object has a size and a monitor.
	for _, want := range []string{
		"document.getElementById('secLampMonitorBack').addEventListener('click'",
		"id !== 'secLampSize' || document.activeElement !== el",
		"foreman_items:true, ticker_rows:1, size:150, monitor:'' }",
	} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page.html lacks %q", want)
		}
	}
	if strings.Contains(pageHTML, "e.target.id !== 'secLampMonitorBack'") {
		t.Error("the back button listens on document, where the gear pane stops the click")
	}
	for _, old := range []string{`<option value="small">`, `<option value="medium">`, `<option value="large">`} {
		if strings.Contains(pageHTML, old) {
			t.Errorf("page.html still has the size word %q", old)
		}
	}
}

// Every control of the Lamp group carries the classes the other gear controls
// use: setin for a select, the slider and the inputs, setbtn for a button.
func TestPageLampGearControlClasses(t *testing.T) {
	i := strings.Index(pageHTML, `<div id="secLamp">`)
	j := strings.Index(pageHTML, `<div class="setrow setstation">`)
	if i < 0 || j < i {
		t.Fatal("the Lamp group is not found in page.html")
	}
	group := pageHTML[i:j]
	for _, tag := range regexp.MustCompile(`<(select|button|input type="range")[^>]*>`).FindAllString(group, -1) {
		want := "setin"
		if strings.HasPrefix(tag, "<button") {
			want = "setbtn"
		}
		if !strings.Contains(tag, `class="`+want+`"`) {
			t.Errorf("%s lacks class %q", tag, want)
		}
	}
	// The other gear inputs carry setin too, and the rule gives it the look
	// and a colour scheme that follows the theme, so an open list is dark.
	for _, want := range []string{
		`#secSettings .setin{`,
		`color-scheme:`,
		`<input type="number" class="setin"`,
	} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page.html lacks %q", want)
		}
	}
}

// The gear carries one group "Lamp" with its controls, saves each through
// bdevSetLamp and ticks them from the payload's lamp object.
func TestPageLampGearGroup(t *testing.T) {
	for _, want := range []string{
		`<h4>Lamp</h4>`,
		`id="secLampEnabled"> Show the lamp</label>`,
		`id="secLampPosition"`,
		`<option value="top-center">`, `<option value="top-left">`, `<option value="top-right">`,
		`id="secLampLanguage"`,
		`<option value="en">`, `<option value="nl">`,
		`id="secLampLines"> One-liners</label>`,
		`id="secLampForeman"> Include Foreman items</label>`,
		"window.bdevSetLamp(",
		"lamp:p.lamp ||",
		"secRenderLamp()",
	} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page.html lacks %q", want)
		}
	}
	for _, want := range []string{
		`id="secLampRows"`, `<option value="1">`, `<option value="4">`,
		`id="secLampSize"`,
		`id="secLampTheme"`, `<option value="system">`, `<option value="light">`, `<option value="dark">`,
		`id="secLampMotion"`, `<option value="lively">`, `<option value="calm">`, `<option value="off">`,
		`id="secLampQuiet"`, `<option value="hide">`, `<option value="stay">`,
		`id="secLampBackdrop"`, `<option value="clear">`, `<option value="card">`,
		"Number(t.value)",
		"ticker_rows:1, size:150, monitor:'', theme:'system', motion:'lively', quiet:'hide', backdrop:'clear'",
	} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page.html lacks %q", want)
		}
	}
	for _, key := range []string{"lamp_enabled", "lamp_position", "lamp_language", "lamp_lines", "lamp_foreman_items",
		"lamp_ticker_rows", "lamp_size", "lamp_theme", "lamp_motion", "lamp_quiet", "lamp_backdrop", "lamp_monitor"} {
		if !strings.Contains(pageHTML, "'"+key+"'") {
			t.Errorf("page.html never sends the key %s", key)
		}
	}
	// The group sits inside the gear pane, before the History block.
	g, h := strings.Index(pageHTML, `<h4>Lamp</h4>`), strings.Index(pageHTML, `<div id="histAdmin">`)
	if g < 0 || h < 0 || g > h {
		t.Error("the Lamp group is not in the gear pane before History")
	}
	// The first render, before any payload, has lamp values to draw.
	if !strings.Contains(pageHTML, "toast:true, lamp:{ enabled:false") {
		t.Error("the initial secState has no lamp object")
	}
	// A refused or failed save puts the control back on the saved value.
	if !strings.Contains(pageHTML, "console.log('bdevSetLamp:', e);") {
		t.Error("page.html does not log a failed bdevSetLamp")
	}
}

// The sweat floor is a slider right under the size slider and built like it:
// min, middle and max under it, the number beside it in short form, one line
// of small text under it, the save on release, and a poll does not move it
// while it is held.
func TestPageLampGearSweatSlider(t *testing.T) {
	for _, want := range []string{
		`Glim sweats from`,
		`<input type="range" id="secLampSweat" class="setin" min="100000" max="1000000" step="50000" value="300000"`,
		`id="secLampSweatNum"`,
		`Tokens a minute, averaged over the last 5 minutes. Cache reads count.`,
		`secLampSweat:['lamp_burn_floor_tpm','burn_floor']`,
		"t.id === 'secLampRows' || t.id === 'secLampSize' || t.id === 'secLampSweat' ? Number(t.value)",
		"id !== 'secLampSweat' || document.activeElement !== el",
		"function sweatShort(",
		"burn_floor:300000",
	} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page.html lacks %q", want)
		}
	}
	// The slider sits in the group Lamp, between the size slider and the theme.
	size, sweat, theme := strings.Index(pageHTML, `id="secLampSize"`), strings.Index(pageHTML, `id="secLampSweat"`), strings.Index(pageHTML, `id="secLampTheme"`)
	if size < 0 || sweat < size || theme < sweat {
		t.Fatalf("the sweat slider is not between the size slider and the theme: %d %d %d", size, sweat, theme)
	}
	// It is a row of the same kind as the size slider, with the three words.
	i := strings.Index(pageHTML, `id="secLampSweat"`)
	row := pageHTML[strings.LastIndex(pageHTML[:i], `<div class="setrow setslider">`):]
	row = row[:strings.Index(row, `<div class="setrow">Theme`)]
	if !strings.Contains(row, `<div class="slwords"><span>min</span><span>middle</span><span>max</span></div>`) {
		t.Error("the sweat slider row lacks the words min, middle and max")
	}
	if !strings.Contains(row, `class="sldim"`) {
		t.Error("the sweat slider row lacks the dim line")
	}
	// The line is dim, small and as wide as the group.
	for _, want := range []string{`#secLamp .setslider .sldim{`} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page.html lacks the rule %q", want)
		}
	}
}

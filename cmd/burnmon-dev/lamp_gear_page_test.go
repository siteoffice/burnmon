//go:build windows

package main

import (
	"strings"
	"testing"
)

// The gear carries one group "Lamp" with the five controls, saves each through
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
	for _, key := range []string{"lamp_enabled", "lamp_position", "lamp_language", "lamp_lines", "lamp_foreman_items"} {
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

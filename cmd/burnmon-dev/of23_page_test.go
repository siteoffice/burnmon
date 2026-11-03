package main

import (
	"strings"
	"testing"
)

// OF23: the gear hint names Foreman only inside an element that secControlsSync
// hides without foreman_root. The hint sits inside #secControls, which
// secControlsSync walks.
func TestPageHintNamesForemanOnlyWithRoot(t *testing.T) {
	const sentence = "Approve a gate and Send a gate back need python on PATH and foreman_root."
	i := strings.Index(pageHTML, sentence)
	if i < 0 {
		t.Fatal("the gear hint no longer holds its Foreman sentence")
	}
	open := strings.LastIndex(pageHTML[:i], "<")
	if tag := pageHTML[open:i]; !strings.HasPrefix(tag, `<span data-ctl-needs="foreman">`) {
		t.Errorf("the Foreman sentence is not inside a data-ctl-needs=\"foreman\" element, nearest tag %q", tag)
	}
	if after := pageHTML[i+len(sentence):]; !strings.HasPrefix(strings.TrimLeft(after, " "), "</span>") {
		t.Error("the Foreman span does not close right after the sentence, so it would hide more than the Foreman clause")
	}
	if c := strings.Index(pageHTML, `<div id="secControls">`); c < 0 || c > i {
		t.Error("the hint is not inside #secControls, so secControlsSync would not reach it")
	}
	if body := pageFuncBody(t, "secControlsSync"); !strings.Contains(body, "[data-ctl-needs]") {
		t.Error("secControlsSync no longer toggles [data-ctl-needs] elements")
	}
}

//go:build windows

package main

import (
	"strings"
	"testing"
)

// The page draws Go's agent view line first in Waits on you, escaped, and the
// same in Standard and Plain (B8).
func TestPageDrawsAgentViewLine(t *testing.T) {
	if !strings.Contains(idlePage(), "var secAgentViewLine = '';") {
		t.Error("page.html does not declare secAgentViewLine")
	}
	if body := idleFn(t, "secApplyPayload"); !strings.Contains(body, "secAgentViewLine = p.agent_view_line || '';") {
		t.Error("secApplyPayload does not keep p.agent_view_line")
	}
	body := idleFn(t, "secBodyWaits")
	const draw = `if(secAgentViewLine) html = '<div class="dim secagentview">' + esc(secAgentViewLine) + '</div>' + html;`
	i := strings.Index(body, draw)
	if i < 0 {
		t.Fatalf("secBodyWaits lacks %q", draw)
	}
	// First: no other part of the body has been added yet.
	if j := strings.Index(body, "html +="); j >= 0 && j < i {
		t.Error("secBodyWaits adds to html before the agent view line")
	}
	// Both modes: the line is never behind a Plain switch.
	line := body[strings.LastIndex(body[:i], "\n")+1:]
	line = line[:strings.Index(line, "\n")]
	if strings.Contains(line, "secState.plain") {
		t.Errorf("the agent view line depends on Plain: %q", line)
	}
}

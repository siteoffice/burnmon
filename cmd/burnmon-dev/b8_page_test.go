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

// The page asks for the snapshot stamp every second and runs the full read
// only when the stamp changed; the 5 s read stays as the fallback (B8).
func TestPageStampPollReadsNewSnapshot(t *testing.T) {
	page := idlePage()
	for _, want := range []string{
		"var secLastStamp = null;",
		"setInterval(secStampPoll, 1000);",
		"setInterval(secPoll, 5000);",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page.html lacks %q", want)
		}
	}
	body := idleFn(t, "secStampPoll")
	// Hidden or frozen (uicheck feeds the page through __bdevSections.apply):
	// no call to Go at all.
	const skip = "if(typeof window.bdevSnapshotStamp !== 'function' || document.hidden || secFrozen) return;"
	ask := "window.bdevSnapshotStamp()"
	i, j := strings.Index(body, skip), strings.Index(body, ask)
	if i < 0 {
		t.Errorf("secStampPoll lacks %q", skip)
	}
	if j < 0 {
		t.Errorf("secStampPoll lacks %q", ask)
	}
	if i >= 0 && j >= 0 && j < i {
		t.Error("secStampPoll asks Go before it checks hidden and frozen")
	}
	// The first stamp is only recorded; a changed one runs the full read.
	for _, want := range []string{
		"var was = secLastStamp;",
		"secLastStamp = s;",
		"if(was !== null && s !== was) secPoll();",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("secStampPoll lacks %q", want)
		}
	}
	if n := strings.Count(body, "secPoll()"); n != 1 {
		t.Errorf("secStampPoll calls secPoll %d times, want 1 (only on a changed stamp)", n)
	}
}

// The Done group starts collapsed at every start and is never saved; a poll
// (secApplyPayload) neither opens nor closes it (B8 spec item 3). uicheck
// waits/d44 proves the DOM.
func TestPageDoneGroupStartsCollapsed(t *testing.T) {
	page := idlePage()
	if !strings.Contains(page, "var secDoneOpen = false;") {
		t.Error("page.html does not declare secDoneOpen false")
	}
	if strings.Contains(idleFn(t, "secApplyPayload"), "secDoneOpen") {
		t.Error("secApplyPayload touches secDoneOpen, so a poll would reset the Done group")
	}
	for _, line := range strings.Split(page, "\n") {
		if strings.Contains(line, "secDoneOpen") && (strings.Contains(line, "secSend") || strings.Contains(line, "bdevSet")) {
			t.Errorf("secDoneOpen is sent to Go, so it would be saved: %q", strings.TrimSpace(line))
		}
	}
	body := idleFn(t, "secBodyWaits")
	for _, want := range []string{`role="button" tabindex="0" aria-expanded="`, "secDoneOpen"} {
		if !strings.Contains(body, want) {
			t.Errorf("secBodyWaits lacks %q", want)
		}
	}
}

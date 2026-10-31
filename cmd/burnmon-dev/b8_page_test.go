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

// The render keeps the Waits list's scroll position, so a toggle scrolls the
// new Done header into view after secRender when it sits outside the list's
// visible part. A poll's render keeps preventScroll. A held key toggles once
// (B8 Task 8 fix round 1).
func TestPageDoneToggleKeepsHeaderInView(t *testing.T) {
	body := idleFn(t, "secDoneToggle")
	const scroll = "scrollIntoView({ block:'nearest' })"
	r, s := strings.Index(body, "secRender();"), strings.Index(body, scroll)
	if r < 0 || s < 0 || s < r {
		t.Errorf("secDoneToggle does not call %s on the header after secRender", scroll)
	}
	render := idleFn(t, "secRender")
	if strings.Contains(render, "scrollIntoView") || !strings.Contains(render, "focus({ preventScroll:true })") {
		t.Error("secRender (the poll path) must keep focus with preventScroll and never scroll")
	}
	page := idlePage()
	const handler = "if((e.key === 'Enter' || e.key === ' ') && e.target.closest && e.target.closest('.sectier.done'))"
	k := strings.Index(page, handler)
	if k < 0 {
		t.Fatalf("page.html lacks the Done header key handler %q", handler)
	}
	// A repeat still gets preventDefault, so a held Space never scrolls the page.
	line := page[k:]
	line = line[:strings.Index(line, "\n")]
	if !strings.Contains(line, "e.preventDefault(); if(!e.repeat) secDoneToggle();") {
		t.Errorf("the Done header key handler does not skip a held key (e.repeat) after preventDefault: %q", line)
	}
}

// A poll or a tick never moves a list the user scrolled (B8 final review fix
// 1): secRender writes new lists, so it takes each list's scrollTop before the
// write and puts it back after the fit pass; the fit pass measures the host
// lists from their top (fitListFloor), so it does the same around its work.
// uicheck waits/d45 proves the DOM.
func TestPageListsKeepScroll(t *testing.T) {
	order := func(fn string, steps ...string) {
		t.Helper()
		body, at := idleFn(t, fn), 0
		for _, s := range steps {
			i := strings.Index(body[at:], s)
			if i < 0 {
				t.Errorf("%s lacks %q after the step before it", fn, s)
				return
			}
			at += i + len(s)
		}
	}
	order("secRender", "var tops = secListTops();", "document.getElementById('secPanels').innerHTML =", "secFit();", "secListRestore(tops);")
	// Taken before fitUndo (its un-shrink of a list can already clamp the
	// position) and put back after both branches, once every height is set.
	order("applyPanelFit", "var tops = secListTops();", "fitUndo();", "fitCollectHost()", "fitPlaceHandles(cols, keys, hostItems", "fitLast = null;", "secListRestore(tops);", "window.__bdevFit = fit;")
	// Keyed by the part, so a render's new list finds the old one's position.
	body := idleFn(t, "secListTops")
	for _, want := range []string{"#secPanels .seclist", "closest('[data-sec]')", "scrollTop"} {
		if !strings.Contains(body, want) {
			t.Errorf("secListTops lacks %q", want)
		}
	}
	if !strings.Contains(idleFn(t, "secListRestore"), "scrollTop =") {
		t.Error("secListRestore does not set scrollTop")
	}
}

// A sections read that was out when uicheck froze the page
// (__bdevSections.apply) must not overwrite the payload uicheck applied: the
// answer checks secFrozen again before it renders.
func TestPagePollSkipsAnswerWhenFrozen(t *testing.T) {
	body := idleFn(t, "secPoll")
	then := strings.Index(body, ".then(function(p){")
	if then < 0 {
		t.Fatal("secPoll has no answer handler .then(function(p){")
	}
	rest := body[then:]
	skip, apply := strings.Index(rest, "if(secFrozen) return;"), strings.Index(rest, "secApplyPayload(p);")
	if skip < 0 || apply < 0 || skip > apply {
		t.Error("secPoll's answer does not return on secFrozen before secApplyPayload(p)")
	}
}

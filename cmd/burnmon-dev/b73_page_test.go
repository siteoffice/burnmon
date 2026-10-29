//go:build windows

package main

import (
	"strings"
	"testing"
)

// The page draws Go's snapshot line and composes none of its own (B7.3).
func TestPageDrawsGoSnapshotLine(t *testing.T) {
	page := idlePage()
	for _, want := range []string{
		"var secLine = { text:'Starting up: writing the first snapshot.', warn:false };",
		"secLine = p.snapshot_line || null;",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page.html lacks %q", want)
		}
	}
	head := idleFn(t, "secHeadline")
	for _, want := range []string{
		"if(secLine) return { text:secLine.text, down:!!secLine.warn };",
		"'No process has written the snapshot for ' + age + '.'",
		"'No snapshot file yet.'",
	} {
		if !strings.Contains(head, want) {
			t.Errorf("secHeadline lacks %q", want)
		}
	}
	if strings.Contains(strings.ToLower(page), "not running") {
		t.Error("page.html still says \"not running\"")
	}
}

// The box shows whenever the reading is not ok; only a warning adds the
// "No current numbers" sentence and turns amber.
func TestSecNoticeFollowsReading(t *testing.T) {
	render := idleFn(t, "secRender")
	for _, want := range []string{
		"notice.innerHTML = r.status !== 'ok' ?",
		"(head.down ? ' warn' : '')",
		`(head.down ? (/\.$/.test(head.text) ? '' : '.') + ' No current numbers are shown.' : '')`,
	} {
		if !strings.Contains(render, want) {
			t.Errorf("secRender lacks %q", want)
		}
	}
	page := idlePage()
	if n := strings.Count(page, "secStub(secState.plain ? 'No current numbers yet.' : 'No current snapshot.')"); n != 2 {
		t.Errorf("page.html has %d Plain/Standard stubs with the new words, want 2", n)
	}
	if strings.Contains(page, "Start BurnMon to see them") {
		t.Error("page.html still tells the reader to start BurnMon")
	}
}

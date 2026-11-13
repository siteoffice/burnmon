//go:build windows || darwin

package main

import (
	"strings"
	"testing"
)

// TestOpenSelIsTheOneOpen: detail keeps the selected thing's open target,
// openSel runs it, and the one dblclick path lives in wireOpen next to the
// Enter (select and open) and Space (select) paths.
func TestOpenSelIsTheOneOpen(t *testing.T) {
	mustContain(t, "detail", "openNow = o.open || null")
	mustContain(t, "openSel", "if (openNow) openNow()")
	if n := strings.Count(pageHTML, "'dblclick'"); n != 1 {
		t.Fatalf("page.html has %d 'dblclick' listeners, want 1", n)
	}
	if fn := enclosingFunc(strings.Index(pageHTML, "'dblclick'")); fn != "wireOpen" {
		t.Errorf("'dblclick' lies in %q, want wireOpen", fn)
	}
	mustContain(t, "wireOpen", "'dblclick'", "ev.key === 'Enter'", "ev.key === ' '", "pick();")
	if n := strings.Count(pageFunc(t, "wireOpen"), "openSel();"); n != 2 {
		t.Errorf("wireOpen calls openSel(); %d times, want 2 (dblclick and Enter)", n)
	}
}

// TestEveryOpenTargetWired: every kind of thing with an open target gets
// wireOpen: boxes, "+N more" rows, parts, files, flow and sprint rows, and
// the Waiting on you rows.
func TestEveryOpenTargetWired(t *testing.T) {
	mustContain(t, "box", "wireOpen(g, onPick)")
	if strings.Contains(pageFunc(t, "box"), "ev.key === 'Enter' || ev.key === ' '") {
		t.Error("box keeps its own Enter or Space handler; wireOpen owns the keys")
	}
	mustContain(t, "pageOverview", "wireOpen(r,")
	if n := strings.Count(pageFunc(t, "pageUnit"), "wireOpen(b,"); n != 3 {
		t.Errorf("pageUnit has %d wireOpen(b, calls, want 3 (parts, files, flow and sprint rows)", n)
	}
	mustContain(t, "pageWaiting", "wireOpen(b,")
}

// TestMoreBoxSelectOnly: the "+N more" map box has no open target and its
// select re-renders, so a double-click's second click would land on a new
// row. It is not wired with wireOpen; Enter and Space only select.
func TestMoreBoxSelectOnly(t *testing.T) {
	mustContain(t, "box", "if (keys) keys(g, onPick);", "else wireOpen(g, onPick);")
	mustContain(t, "drawColumn", "}, selectKeys);")
	if strings.Contains(pageFunc(t, "drawColumn"), "wireOpen(") {
		t.Error("drawColumn wires the \"+N more\" box with wireOpen")
	}
	sk := pageFunc(t, "selectKeys")
	mustContain(t, "selectKeys", "'keydown'", "ev.key === 'Enter' || ev.key === ' '", "pick();")
	for _, bad := range []string{"openSel", "'dblclick'", "openNow"} {
		if strings.Contains(sk, bad) {
			t.Errorf("selectKeys contains %s; it must only select", bad)
		}
	}
}

// TestOpenButtonsUseOpenSel: every Open button in the detail pane runs the
// same openSel, and each detail passes its open target as open:.
func TestOpenButtonsUseOpenSel(t *testing.T) {
	for _, fn := range []string{"boxDetail", "rowDetail", "waitDetail", "sprintWaitDetail"} {
		mustContain(t, fn, "openSel", "open:")
	}
	if n := strings.Count(pageFunc(t, "pageUnit"), "openSel"); n != 2 {
		t.Errorf("pageUnit names openSel %d times, want 2 (part and file Open buttons)", n)
	}
}

// TestOpenOnlyThroughOpenSel: no event handler opens a board, a file or a
// box's unit directly; those go() calls live only in the open closures.
func TestOpenOnlyThroughOpenSel(t *testing.T) {
	allowed := map[string]bool{"rowDetail": true, "waitDetail": true, "sprintWaitDetail": true, "crumbs": true}
	checkAt := func(needle string, ok map[string]bool) {
		t.Helper()
		for off := 0; ; {
			j := strings.Index(pageHTML[off:], needle)
			if j < 0 {
				return
			}
			off += j
			if fn := enclosingFunc(off); !ok[fn] {
				t.Errorf("%s in %q, want it only in an open closure", needle, fn)
			}
			off += len(needle)
		}
	}
	checkAt("go('board'", allowed)
	checkAt("go('file'", map[string]bool{"pageUnit": true})
	checkAt("go('unit', b.unit_id)", map[string]bool{"boxDetail": true})
	if n := strings.Count(pageHTML, "go('file'"); n != 1 {
		t.Errorf("page.html has %d go('file' calls, want 1 (the file open closure)", n)
	}
	for _, fn := range []string{"pagePlanning", "pageFile", "pageAgents"} {
		if strings.Contains(pageFunc(t, fn), "wireOpen(") {
			t.Errorf("%s wires open, but its things have no open target", fn)
		}
	}
}

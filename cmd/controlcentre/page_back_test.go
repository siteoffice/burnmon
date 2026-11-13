//go:build windows || darwin

package main

import (
	"strconv"
	"strings"
	"testing"

	"burnmon/internal/centre"
)

// TestBackButtonFirst: the Back button is the first control of the top
// bar, before the breadcrumb, with an icon and the word Back, and it is
// disabled (never hidden) with nothing before the current view.
func TestBackButtonFirst(t *testing.T) {
	top := strings.Index(pageHTML, `<header class="top">`)
	end := strings.Index(pageHTML, `</header>`)
	back := strings.Index(pageHTML, `id="back"`)
	cr := strings.Index(pageHTML, `id="crumb"`)
	if back <= 0 || back >= cr {
		t.Fatalf("id=\"back\" at %d, id=\"crumb\" at %d; Back must come first", back, cr)
	}
	if top < 0 || back < top || cr > end || end < top {
		t.Errorf("Back (%d) and crumb (%d) are not inside <header class=\"top\"> (%d to %d)", back, cr, top, end)
	}
	if first := strings.TrimSpace(pageHTML[top+len(`<header class="top">`) : back]); first != `<button type="button" class="back"` {
		t.Errorf("first child of the top bar starts %q, want the Back button", first)
	}
	mustContain(t, "backButton", "icon('left')", "'Back'")
	mustContain(t, "remember", "backb.disabled = hist.length < 2")
	if strings.Contains(pageFunc(t, "remember"), "hidden") {
		t.Error("remember hides the Back button; it must only disable it")
	}
	mustContain(t, "start", "backButton()")
}

// handlerAround returns the page text from the addEventListener call
// before offset i up to the end of that listener's line block.
func handlerAround(t *testing.T, needle string) string {
	t.Helper()
	if n := strings.Count(pageHTML, needle); n != 1 {
		t.Fatalf("page.html holds %d of %q, want 1", n, needle)
	}
	i := strings.Index(pageHTML, needle)
	s := strings.LastIndex(pageHTML[:i], "addEventListener(")
	e := strings.Index(pageHTML[i:], "});")
	if s < 0 || e < 0 {
		t.Fatalf("%q is not inside a listener", needle)
	}
	return pageHTML[s : i+e]
}

// TestBackHandlers: the mouse back button (button 3) and Alt+Left run the
// same back() as the Back button and stop the default action.
func TestBackHandlers(t *testing.T) {
	m := handlerAround(t, "ev.button === 3")
	for _, w := range []string{"ev.preventDefault()", "back()"} {
		if !strings.Contains(m, w) {
			t.Errorf("the button 3 handler lacks %s: %q", w, m)
		}
	}
	k := handlerAround(t, "ev.altKey && ev.key === 'ArrowLeft'")
	for _, w := range []string{"ev.preventDefault()", "back()"} {
		if !strings.Contains(k, w) {
			t.Errorf("the Alt+Left handler lacks %s: %q", w, k)
		}
	}
	if !strings.Contains(pageFunc(t, "backButton"), "addEventListener('click', back)") {
		t.Error("the Back button's click does not call back")
	}
	mustContain(t, "back", "histBack(hist, !pop.hidden)", "closePop()", "restoreView(")
}

// TestHistoryMirror: the page's history functions mirror internal\centre:
// the same cap, the same compared fields, the same branch order.
func TestHistoryMirror(t *testing.T) {
	if want := "var HIST_MAX = " + strconv.Itoa(centre.HistoryMax) + ";"; !strings.Contains(pageHTML, want) {
		t.Errorf("page.html lacks %s", want)
	}
	if !strings.Contains(pageHTML, "var hist = [];") {
		t.Error("page.html lacks var hist = [];")
	}
	va := pageFunc(t, "viewAdds")
	for _, f := range []string{".page", ".unit", ".tab", ".file", ".board"} {
		if !strings.Contains(va, f) {
			t.Errorf("viewAdds does not compare %s", f)
		}
	}
	for _, f := range []string{".sel", ".list_page", ".overview"} {
		if strings.Contains(va, f) {
			t.Errorf("viewAdds compares %s; a change of it alone adds nothing", f)
		}
	}
	hb := pageFunc(t, "histBack")
	p, l := strings.Index(hb, "popupOpen"), strings.Index(hb, "length < 2")
	if p < 0 || l < 0 || strings.Index(hb[p+1:], "popupOpen") < 0 {
		t.Fatalf("histBack lacks the popup check or the length < 2 check: %q", hb)
	}
	if q := p + 1 + strings.Index(hb[p+1:], "popupOpen"); q > l {
		t.Error("histBack checks length < 2 before the popup")
	}
	mustContain(t, "histAdd", "HIST_MAX", "viewAdds(")
	mustContain(t, "curView", "ovView", "sub.tab", "sub.page", "sub.sel", "view.arg.row.board")
	mustContain(t, "restoreView", "ovView", "list_page", "render()")
}

// TestDetailRemembers: every selection and every render reaches detail,
// which records the view.
func TestDetailRemembers(t *testing.T) {
	mustContain(t, "detail", "remember()", "openNow = o.open || null")
}

// TestSelectionRestored: unit rows and file blocks record their selection
// and pick it again after drawing, so Back shows the selection too.
func TestSelectionRestored(t *testing.T) {
	mustContain(t, "pageUnit", "sub.sel = 'part:' + p.title", "sub.sel = 'file:' + path", "sub.sel = r.key", "picked.click()")
	mustContain(t, "pageFile", "sub.sel = 'block:' + (i + 1)", "picked.click()")
}

// TestNoForward: there is no Forward in this run.
func TestNoForward(t *testing.T) {
	if strings.Contains(pageHTML, "'Forward'") {
		t.Error("page.html has a Forward control")
	}
}

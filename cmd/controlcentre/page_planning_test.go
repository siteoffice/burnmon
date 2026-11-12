//go:build windows || darwin

package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestPagePlanningReadsGrid(t *testing.T) {
	pp := pageFunc(t, "pagePlanning")
	for _, want := range []string{"state.planning.rows", ".today", ".weekend", ".fixed", ".notes", "state.next_fixed",
		"'No plan file at '", "plan_path", "'Next fixed date:", "'No fixed date ahead.'", "'Week'"} {
		if !strings.Contains(pp, want) {
			t.Errorf("pagePlanning lacks %s", want)
		}
	}
	for _, cls := range []string{"'plan'", "' today'", "' we'", "' soft'"} {
		if !strings.Contains(pp, cls) {
			t.Errorf("pagePlanning does not set class %s", cls)
		}
	}
	// The grid comes from the state: no Date arithmetic for the days.
	if strings.Contains(pp, "setDate(") || strings.Contains(pp, "getDay(") {
		t.Error("pagePlanning computes days itself; read them from state.planning")
	}
}

// TestPlanningDayMore: a day with more items than fit hides the rest behind
// a "+N more" line whose hover title lists them, in the state's order.
func TestPlanningDayMore(t *testing.T) {
	if !strings.Contains(pageFunc(t, "pagePlanning"), "fitDay(") {
		t.Error("pagePlanning does not call fitDay(")
	}
	fd := pageFunc(t, "fitDay")
	for _, want := range []string{"scrollHeight > ", "clientHeight", "'+' + ", " more'", ".hidden = true", ".title = ", "'data-title'"} {
		if !strings.Contains(fd, want) {
			t.Errorf("fitDay lacks %s", want)
		}
	}
	for _, bad := range []string{".sort(", ".reverse("} {
		if strings.Contains(fd, bad) {
			t.Errorf("fitDay reorders items with %s", bad)
		}
	}
	if !strings.Contains(compact(ruleFor(t, ".plan .day .more")), "flex:none") {
		t.Error(".plan .day .more is not flex:none")
	}
}

// TestHoverKeepsPageTitle: a title the page set itself (data-title) is
// never replaced or removed by hoverTitle.
func TestHoverKeepsPageTitle(t *testing.T) {
	ht := pageFunc(t, "hoverTitle")
	keep := strings.Index(ht, "hasAttribute('data-title')")
	set := strings.Index(ht, "el.title = el.textContent")
	if keep < 0 || set < 0 || keep > set {
		t.Error("hoverTitle must return on data-title before it sets or removes a title")
	}
	if !strings.Contains(pageFunc(t, "pagePlanning"), "setAttribute('data-title'") {
		t.Error("pagePlanning does not mark the item title with data-title")
	}
}

// TestDropOnlyInDropZone: a file dropped anywhere else never replaces the
// app page in the window.
func TestDropOnlyInDropZone(t *testing.T) {
	p := pageFunc(t, "popup")
	for _, want := range []string{"document.addEventListener('dragover'", "document.addEventListener('drop'"} {
		i := strings.Index(p, want)
		if i < 0 {
			t.Errorf("popup lacks %s", want)
			continue
		}
		if !strings.Contains(p[i:i+200], "e.preventDefault()") {
			t.Errorf("%s does not call preventDefault", want)
		}
	}
	if !strings.Contains(p, "pop.hidden") {
		t.Error("the drop zone accepts files while the popup is closed")
	}
}

func TestPlanningFixedUsesWarning(t *testing.T) {
	it := compact(ruleFor(t, ".it"))
	if !strings.Contains(it, "var(--amber)") {
		t.Error(".it does not use var(--amber)")
	}
	if strings.Contains(compact(ruleFor(t, ".it.soft")), "--amber") {
		t.Error(".it.soft uses the warning colour")
	}
	// Day cells and items fit their cell: no scroll on the grid.
	for _, sel := range []string{".plan", ".plan .day"} {
		if !strings.Contains(compact(ruleFor(t, sel)), "overflow:hidden") {
			t.Errorf("%s is not overflow:hidden", sel)
		}
	}
}

func TestPageSettingsFour(t *testing.T) {
	ps := pageFunc(t, "pageSettings")
	calls := regexp.MustCompile(`saveSetting\('(\w+)'`).FindAllStringSubmatch(ps, -1)
	got := map[string]bool{}
	for _, c := range calls {
		got[c[1]] = true
	}
	for _, k := range []string{"width", "menu", "weeks", "theme"} {
		if !got[k] {
			t.Errorf("pageSettings does not call saveSetting('%s'", k)
		}
	}
	if len(got) != 4 || strings.Count(ps, "saveSetting(") != len(calls) {
		t.Errorf("pageSettings calls saveSetting with keys %v; want only width, menu, weeks, theme as literals", got)
	}
	for _, label := range []string{"'Screen use'", "'Menu'", "'Planning horizon'", "'Theme'",
		"'60%'", "'80%'", "'100%'", "'Icons and text'", "'Icons only'", "'2 weeks'", "'4 weeks'",
		"'System'", "'Light'", "'Dark'", "'Settings. Every setting of the app lives on this page.'"} {
		if !strings.Contains(ps, label) {
			t.Errorf("pageSettings lacks %s", label)
		}
	}
	// saveSetting applies the saved value and reads the state again, so a
	// new horizon reaches the Planning grid.
	ss := pageFunc(t, "saveSetting")
	if !strings.Contains(ss, "applySettings(saved)") || !strings.Contains(ss, "loadState()") {
		t.Error("saveSetting does not apply the saved settings and reload the state")
	}
}

// appSpan returns the offsets of the <div ... id="app"> element.
func appSpan(t *testing.T) (int, int) {
	t.Helper()
	s := strings.Index(pageHTML, `<div class="app" id="app">`)
	if s < 0 {
		t.Fatal(`page.html has no <div class="app" id="app">`)
	}
	tag := regexp.MustCompile(`<div\b|</div>`)
	depth := 0
	for _, m := range tag.FindAllStringIndex(pageHTML[s:], -1) {
		if pageHTML[s+m[0]:s+m[1]] == "</div>" {
			depth--
		} else {
			depth++
		}
		if depth == 0 {
			return s, s + m[1]
		}
	}
	t.Fatal("#app is not closed")
	return 0, 0
}

func TestCommentPopup(t *testing.T) {
	s, e := appSpan(t)
	p := strings.Index(pageHTML, `id="pop"`)
	if p < s || p > e {
		t.Error(`id="pop" does not lie inside the #app element`)
	}
	for _, id := range []string{`id="cmt"`, `id="pickf"`, `id="files"`, `id="drop"`, `id="atts"`, `id="send"`, `id="cancel"`} {
		if i := strings.Index(pageHTML, id); i < s || i > e {
			t.Errorf("%s is missing or outside #app", id)
		}
	}
	for _, want := range []string{"readAsDataURL", "'paste'", "'drop'", "'dragover'"} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page.html lacks %s", want)
		}
	}
	sc := pageFunc(t, "sendComment")
	for _, want := range []string{"api.comment(", "target", "attachments", "res.appendChild(", "res.textContent = ''"} {
		if !strings.Contains(sc, want) {
			t.Errorf("sendComment lacks %s", want)
		}
	}
	// The result goes in the foot (#res), never below the fold of #det.
	if strings.Contains(sc, "det.") {
		t.Error("sendComment writes to det; the result belongs in res in the .cm foot")
	}
	// The limits match internal\centre: 5 attachments, 10 MB each.
	if !strings.Contains(pageHTML, "var MAX_ATT = 5;") || !strings.Contains(pageHTML, "var MAX_ATT_BYTES = 10 * 1024 * 1024;") {
		t.Error("the page limits are not MAX_ATT = 5 and MAX_ATT_BYTES = 10 * 1024 * 1024")
	}
	// The base64 prefix is stripped before sending.
	if !strings.Contains(pageHTML, "indexOf(',')") {
		t.Error("the data URL prefix is not stripped")
	}
	// The popup opens only for the current target.
	if op := pageFunc(t, "openPop"); !strings.Contains(op, "target.kind") || !strings.Contains(op, "target.label") {
		t.Error("openPop does not read the current target")
	}
	// The textarea and the attachment list never scroll on their own.
	for _, sel := range []string{".popbox textarea", ".atts", ".pop", ".popbox"} {
		b := ruleFor(t, sel)
		if overflowY.MatchString(b) || overflowShorthand.MatchString(b) || overflowX.MatchString(b) {
			t.Errorf("%s scrolls", sel)
		}
	}
}

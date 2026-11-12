//go:build windows || darwin

package main

import (
	"regexp"
	"strings"
	"testing"
)

// TestPageCallsEachBindingOnce: winbind.PageCalls de-duplicates, so this
// counts raw calls: exactly one per binding name, inside the api table.
func TestPageCallsEachBindingOnce(t *testing.T) {
	s, e := apiTable(t)
	for _, n := range ccBindingNames {
		locs := regexp.MustCompile(`\b` + n + `\s*\(`).FindAllStringIndex(pageHTML, -1)
		if len(locs) != 1 {
			t.Errorf("%s( is called %d times, want exactly 1", n, len(locs))
			continue
		}
		if locs[0][0] < s || locs[0][1] > e {
			t.Errorf("%s( is called outside the api table", n)
		}
	}
}

func TestPageNoEmDash(t *testing.T) {
	if strings.ContainsRune(pageHTML, rune(0x2014)) {
		t.Error("page.html contains an em dash (U+2014)")
	}
}

func TestPageNoNetwork(t *testing.T) {
	if strings.Contains(pageHTML, "https://") {
		t.Error("page.html contains https://")
	}
	rest := strings.ReplaceAll(pageHTML, `'http://www.w3.org/2000/svg'`, "")
	if strings.Contains(rest, "http://") {
		t.Error("page.html contains http:// other than the SVG namespace literal")
	}
	res := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(src|href)\s*=\s*["']?\s*(//|[a-z][a-z0-9+.-]*:)`),
		regexp.MustCompile(`(?i)\burl\(\s*["']?\s*(//|[a-z][a-z0-9+.-]*:)`),
	}
	for _, re := range res {
		if m := re.FindString(pageHTML); m != "" {
			t.Errorf("page.html loads a resource: %q", m)
		}
	}
	for _, bad := range []string{"<link", "@import", "fetch(", "XMLHttpRequest", "WebSocket"} {
		if strings.Contains(pageHTML, bad) {
			t.Errorf("page.html contains %q", bad)
		}
	}
}

func TestPageThemeSwitch(t *testing.T) {
	s := strings.Index(pageHTML, `<header class="top">`)
	e := strings.Index(pageHTML, `</header>`)
	if s < 0 || e < s || !strings.Contains(pageHTML[s:e], `id="theme"`) {
		t.Error(`the top bar has no id="theme" element`)
	}
	if !strings.Contains(pageHTML, "var THEMES = ['system','light','dark'];") {
		t.Error("THEMES is not exactly ['system','light','dark']")
	}
	if !strings.Contains(pageFunc(t, "themeSwitch"), "saveSetting('theme'") {
		t.Error("themeSwitch does not call saveSetting('theme'")
	}
}

func TestPageOneSettingsPage(t *testing.T) {
	s, e := apiTable(t)
	for _, loc := range regexp.MustCompile(`ccSettingsSet`).FindAllStringIndex(pageHTML, -1) {
		if loc[0] < s || loc[1] > e {
			t.Errorf("ccSettingsSet appears outside the api table at %d", loc[0])
		}
	}
	all := strings.Count(pageHTML, "api.settingsSet(")
	in := strings.Count(pageFunc(t, "saveSetting"), "api.settingsSet(")
	if in == 0 || all != in {
		t.Errorf("api.settingsSet( appears %d times, %d of them in saveSetting; want all in saveSetting", all, in)
	}
	allowed := map[string]bool{"pageSettings": true, "themeSwitch": true, "menu": true}
	for _, loc := range regexp.MustCompile(`\bsaveSetting\(`).FindAllStringIndex(pageHTML, -1) {
		if strings.HasSuffix(pageHTML[:loc[0]], "function ") {
			continue
		}
		if f := enclosingFunc(loc[0]); !allowed[f] {
			t.Errorf("saveSetting( is called from %q, want only pageSettings, themeSwitch or menu", f)
		}
	}
}

func TestPageMenuToggle(t *testing.T) {
	if !strings.Contains(pageHTML, `id="menuToggle"`) {
		t.Error(`page.html has no id="menuToggle"`)
	}
	body := pageFunc(t, "menu")
	for _, want := range []string{"'Icons only'", "'Icons and text'", "saveSetting('menu'"} {
		if !strings.Contains(body, want) {
			t.Errorf("menu does not contain %s", want)
		}
	}
	if !strings.Contains(body, "counts.waiting") {
		t.Error("the menu badge does not read counts.waiting")
	}
}

var overflowShorthand = regexp.MustCompile(`(^|[;{\s])overflow\s*:[^;]*\b(auto|scroll)\b`)
var overflowX = regexp.MustCompile(`overflow-x\s*:[^;]*\b(auto|scroll)\b`)
var overflowY = regexp.MustCompile(`overflow-y\s*:[^;]*\b(auto|scroll)\b`)

func TestPageNoHorizontalScroll(t *testing.T) {
	for _, r := range cssRules(t) {
		if overflowX.MatchString(r.Body) {
			t.Errorf("%v sets overflow-x to auto or scroll", r.Selectors)
		}
		if overflowShorthand.MatchString(r.Body) {
			t.Errorf("%v sets overflow to auto or scroll", r.Selectors)
		}
	}
}

func TestPageVerticalScrollOnlyTwoPlaces(t *testing.T) {
	allowed := map[string]bool{".fileview": true, ".boardframe": true}
	found := map[string]bool{}
	for _, r := range cssRules(t) {
		if !overflowY.MatchString(r.Body) {
			continue
		}
		for _, s := range r.Selectors {
			if !allowed[s] {
				t.Errorf("%q sets overflow-y to auto or scroll; only .fileview and .boardframe may", s)
			}
			found[s] = true
		}
	}
	for s := range allowed {
		if !found[s] {
			t.Errorf("%s has no overflow-y:auto rule", s)
		}
	}
}

func TestPageEllipsisHasHover(t *testing.T) {
	m := regexp.MustCompile(`var HOVER_SEL = '([^']*)';`).FindStringSubmatch(pageHTML)
	if m == nil {
		t.Fatal("page.html has no var HOVER_SEL = '...';")
	}
	covered := map[string]bool{}
	for _, p := range strings.Split(m[1], ",") {
		covered[strings.Join(strings.Fields(p), " ")] = true
	}
	n := 0
	for _, r := range cssRules(t) {
		if !regexp.MustCompile(`text-overflow\s*:\s*ellipsis`).MatchString(r.Body) {
			continue
		}
		n++
		for _, s := range r.Selectors {
			if !covered[s] {
				t.Errorf("%q has text-overflow:ellipsis but is not in HOVER_SEL", s)
			}
		}
	}
	if n == 0 {
		t.Error("no rule with text-overflow:ellipsis found; the check would be empty")
	}
	body := pageFunc(t, "hoverTitle")
	for _, want := range []string{"closest(HOVER_SEL)", "scrollWidth > el.clientWidth", "el.title = el.textContent"} {
		if !strings.Contains(body, want) {
			t.Errorf("hoverTitle does not contain %q", want)
		}
	}
	if !strings.Contains(pageHTML, "addEventListener('mouseover', hoverTitle)") {
		t.Error("hoverTitle is not the mouseover handler")
	}
}

func TestPageWidthSetting(t *testing.T) {
	if !strings.Contains(pageFunc(t, "applySettings"), "s.width + '%'") {
		t.Error("applySettings does not set the width from s.width + '%'")
	}
	px := regexp.MustCompile(`max-width\s*:\s*[0-9.]+px`)
	for _, r := range cssRules(t) {
		for _, s := range r.Selectors {
			if strings.HasSuffix(s, ".app") && px.MatchString(r.Body) {
				t.Errorf("%q has a max-width in px", s)
			}
		}
	}
}

func TestPageLayoutFixed(t *testing.T) {
	if !strings.Contains(compact(ruleFor(t, ".body")), "grid-template-columns:176pxminmax(0,1fr)256px") {
		t.Error(".body does not have grid-template-columns:176px minmax(0,1fr) 256px")
	}
	if !strings.Contains(compact(ruleFor(t, ".app.icons .body")), "grid-template-columns:46pxminmax(0,1fr)256px") {
		t.Error(".app.icons .body does not have grid-template-columns:46px minmax(0,1fr) 256px")
	}
	for _, label := range []string{"Overview", "Waiting on you", "Agents", "Planning", "Settings"} {
		if !strings.Contains(pageHTML, "'"+label+"'") {
			t.Errorf("menu label %q is missing", label)
		}
	}
}

// TestPageShellFunctions: the shell functions later tasks build on exist
// at two-space indent and do what the shell promises.
func TestPageShellFunctions(t *testing.T) {
	for _, f := range []string{"h", "btn", "icon", "render", "go", "detail", "pager", "rowsFit", "loadState",
		"applySettings", "saveSetting", "themeSwitch", "menu", "pageOverview", "pageUnit", "pageFile",
		"pageBoard", "pageWaiting", "pagePlanning", "pageSettings", "pageAgents"} {
		pageFunc(t, f)
	}
	if b := pageFunc(t, "rowsFit"); !strings.Contains(b, "/ ROW_H") || !strings.Contains(b, "Math.max(1,") {
		t.Error("rowsFit does not divide by ROW_H with a floor of 1")
	}
	if !strings.Contains(pageHTML, "var ROW_H = 28;") {
		t.Error("ROW_H is not 28")
	}
	if b := pageFunc(t, "loadState"); !strings.Contains(b, "api.state({rows: rowsFit()})") || !strings.Contains(b, "render()") {
		t.Error("loadState does not call api.state({rows: rowsFit()}) and render()")
	}
	if b := pageFunc(t, "pager"); !strings.Contains(b, "spans.length < 2") || !strings.Contains(b, "' of '") {
		t.Error("pager does not skip one page or draw 'N of M'")
	}
	if b := pageFunc(t, "detail"); !strings.Contains(b, "target = {kind:") {
		t.Error("detail does not set the comment target")
	}
	if b := pageFunc(t, "start"); !strings.Contains(b, "'resize'") || !strings.Contains(b, "200") {
		t.Error("start does not reload the state on resize, debounced 200 ms")
	}
	for _, id := range []string{`id="crumb"`, `id="sidem"`, `id="lead"`, `id="pane"`, `id="det"`, `id="openc"`, `class="tgt"`} {
		if !strings.Contains(pageHTML, id) {
			t.Errorf("page.html lacks %s", id)
		}
	}
	for _, sel := range []string{".pane", ".det"} {
		if !strings.Contains(compact(ruleFor(t, sel)), "overflow:hidden") {
			t.Errorf("%s is not overflow:hidden", sel)
		}
	}
}

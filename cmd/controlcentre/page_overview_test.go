//go:build windows || darwin

package main

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// mustContain fails for every want missing from the body of page function fn.
func mustContain(t *testing.T, fn string, wants ...string) {
	t.Helper()
	body := pageFunc(t, fn)
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("%s does not contain %s", fn, w)
		}
	}
}

func TestOverviewReadsState(t *testing.T) {
	mustContain(t, "drawPlan", "state.badges", "plan.links", ".step", "'hold'")
	mustContain(t, "pageOverview", "state.overview.default", "'data-view'", "'plan'", "'units'")
	mustContain(t, "drawUnits", "state.overview.unit_columns", "drawColumn(")
	mustContain(t, "drawPlan", "state.overview.plan_columns", "drawColumn(")
	if !strings.Contains(pageHTML, "document.createElementNS(NS,") {
		t.Error("SVG elements are not made with document.createElementNS(NS, ...)")
	}
}

func TestColumnShrinkAndMore(t *testing.T) {
	if !strings.Contains(pageHTML, "var MIN_BOX = 22;") {
		t.Error("page.html has no var MIN_BOX = 22;")
	}
	if !strings.Contains(pageHTML, "var MAX_BOX = 40;") {
		t.Error("page.html has no var MAX_BOX = 40;")
	}
	mustContain(t, "drawColumn", "MIN_BOX", "MAX_BOX", "'+' + ", "' more'", "col.title")
	// The "+N more" rows are paged by the column's own spans.
	mustContain(t, "pageOverview", "col.pages", "pager(")
}

func TestDetailForBox(t *testing.T) {
	mustContain(t, "boxDetail", "detail({", "k: b.kind", "t: b.title", "d: b.note", "list: b.conns", "src: srcPath(b.path)", "b.unit_id ?", "go('unit', b.unit_id)")
	mustContain(t, "srcPath", "state.root")
	// Every box selects through pickBox, which fills the detail through
	// boxDetail; every "+N more" row calls boxDetail directly.
	for _, fn := range []string{"drawPlan", "drawUnits"} {
		mustContain(t, fn, "pickBox(")
	}
	mustContain(t, "pickBox", "boxDetail(")
	mustContain(t, "pageOverview", "boxDetail(")
	// Choice 19: a comment on a plan box names the plan node id, exactly.
	mustContain(t, "boxDetail", "label: b.target === 'plan' ? b.id : b.title")
	mustContain(t, "detail", "o.label != null ? o.label : o.t")
}

// pageFuncNames lists every page function declared at two-space indent.
func pageFuncNames() []string {
	var out []string
	for _, m := range regexp.MustCompile(`\n  function (\w+)\(`).FindAllStringSubmatch(pageHTML, -1) {
		out = append(out, m[1])
	}
	return out
}

// TestViewsKeepStateOrder: rows come in the state's order (the Go side
// sorts); no view, nor any page function it reaches, sorts or reverses.
func TestViewsKeepStateOrder(t *testing.T) {
	names := pageFuncNames()
	seen := map[string]bool{}
	queue := []string{"pageUnit", "pageOverview"}
	for len(queue) > 0 {
		fn := queue[0]
		queue = queue[1:]
		if seen[fn] {
			continue
		}
		seen[fn] = true
		body := pageFunc(t, fn)
		for _, bad := range []string{".sort(", ".reverse("} {
			if strings.Contains(body, bad) {
				t.Errorf("%s (reached from the views) contains %s", fn, bad)
			}
		}
		for _, n := range names {
			if !seen[n] && regexp.MustCompile(`\b`+n+`\(`).MatchString(body) {
				queue = append(queue, n)
			}
		}
	}
	for _, want := range []string{"drawPlan", "drawUnits", "drawColumn", "rowDetail", "boxDetail", "pager"} {
		if !seen[want] {
			t.Errorf("the walk did not reach %s", want)
		}
	}
}

func TestUnitViewSwitch(t *testing.T) {
	mustContain(t, "pageUnit", "'Structure'", "'Flows'", "'Sprints'", "u.file_pages", "u.flows.pages", "u.sprints.pages",
		"pager(", "u.item", "u.children", "'Project'", "'Flow'", "'Sprint'", "'Stage'", "'Phase'", "'Waits'", "' warn'")
}

func TestFileViewBlocks(t *testing.T) {
	mustContain(t, "pageFile", "api.file(", "'fileview'", ".blocks", ".heading", "block: i + 1", "kind: 'block'", "unit_id:")
}

func TestRowsAreDense(t *testing.T) {
	if !strings.Contains(compact(ruleFor(t, ".rw")), "min-height:28px") {
		t.Error(".rw does not have min-height:28px")
	}
}

// TestCrumbsRootUnitFile: the breadcrumb runs root, unit, file.
func TestCrumbsRootUnitFile(t *testing.T) {
	mustContain(t, "crumbs", "rootName()", "view.page === 'unit'", "view.page === 'file'", "go('unit',")
}

// TestRootNameFromState: the root crumb is the root unit's display name,
// else state.root as is; the screen builds no name of its own.
func TestRootNameFromState(t *testing.T) {
	body := pageFunc(t, "rootName")
	if !strings.Contains(body, "display_name") || !strings.Contains(body, "return String(state.root)") {
		t.Error("rootName does not return the root unit's display_name, else state.root")
	}
	for _, bad := range []string{"split(", "slice(", "replace(", "substr"} {
		if strings.Contains(body, bad) {
			t.Errorf("rootName builds a name with %s", bad)
		}
	}
}

// TestPageASCII: arrows and other signs are written as \u escapes, so the
// editor tools cannot slip a literal one (or an em dash) into the page.
func TestPageASCII(t *testing.T) {
	for i, r := range pageHTML {
		if r > 0x7e || (r < 0x20 && r != '\n' && r != '\r' && r != '\t') {
			t.Fatalf("page.html has character %U at byte %d; write it as a \\u escape", r, i)
		}
	}
}

// TestPageNoScriptOverflow: overflow and text-overflow set from script or
// inline styles would escape the CSS rule checks.
func TestPageNoScriptOverflow(t *testing.T) {
	bad := regexp.MustCompile(`(?i)overflow[^;"']*?["':=\s]+[^;]*?\b(auto|scroll)\b`)
	sites := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bstyle\s*=\s*("[^"]*"|'[^']*')`),
		regexp.MustCompile(`\.style\b[^;\n]*`),
		regexp.MustCompile(`setAttribute\(\s*['"]style['"][^)]*\)`),
		regexp.MustCompile(`setProperty\([^)]*\)`),
	}
	for _, re := range sites {
		for _, m := range re.FindAllString(pageHTML, -1) {
			if bad.MatchString(m) {
				t.Errorf("overflow auto or scroll set outside the CSS rules: %q", m)
			}
			if strings.Contains(strings.ToLower(m), "text-overflow") || strings.Contains(m, "textOverflow") {
				t.Errorf("text-overflow set outside the CSS rules: %q", m)
			}
		}
	}
	// Over the whole script: object-literal keys (SVG presentation
	// attributes applied in a loop) and a style element made in script.
	s := strings.Index(pageHTML, "<script>")
	e := strings.Index(pageHTML, "</script>")
	if s < 0 || e < s {
		t.Fatal("page.html has no <script> element")
	}
	script := pageHTML[s:e]
	litKey := regexp.MustCompile(`(?i)['"]?\boverflow(-[xy])?['"]?\s*:\s*['"][^'"]*\b(auto|scroll)\b`)
	makeStyle := regexp.MustCompile(`(?i)createElement(NS)?\s*\([^)]*['"]style['"]`)
	if m := litKey.FindString(script); m != "" {
		t.Errorf("the script sets overflow auto or scroll through an object key: %q", m)
	}
	if m := makeStyle.FindString(script); m != "" {
		t.Errorf("the script makes a style element: %q", m)
	}
	for _, probe := range []string{`sv('svg', {overflow:'auto'})`, `{'overflow-x': 'scroll'}`, `{"overflow-y":"auto"}`} {
		if !litKey.MatchString(probe) {
			t.Errorf("the object-key scan misses %q", probe)
		}
	}
	for _, probe := range []string{`document.createElement('style')`, `document.createElementNS(NS, "style")`} {
		if !makeStyle.MatchString(probe) {
			t.Errorf("the style-element scan misses %q", probe)
		}
	}
	if strings.Contains(pageHTML, "textOverflow") {
		t.Error("page.html sets textOverflow from script; ellipsis belongs in CSS classes listed in HOVER_SEL")
	}
	// The scan itself must catch what it is meant to catch.
	for _, probe := range []string{`el.style.overflowX = 'auto'`, `<div style="overflow:scroll">`, `el.style.cssText = 'overflow-y: auto'`} {
		hit := false
		for _, re := range sites {
			for _, m := range re.FindAllString(probe, -1) {
				hit = hit || bad.MatchString(m)
			}
		}
		if !hit {
			t.Errorf("the scan misses %q", probe)
		}
	}
}

// pageVar reads an integer constant declared as "var NAME = N;" in page.html.
func pageVar(t *testing.T, name string) float64 {
	t.Helper()
	m := regexp.MustCompile(`var ` + name + ` = (\d+);`).FindStringSubmatch(pageHTML)
	if m == nil {
		t.Fatalf("page.html has no var %s", name)
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatalf("var %s: %v", name, err)
	}
	return v
}

// TestMapColumnsFit: every Overview column, its box and the badge radius
// lie inside the map, at most the viewBox width minus 2.
func TestMapColumnsFit(t *testing.T) {
	gap, badge, pad := pageVar(t, "COL_GAP"), pageVar(t, "BADGE_R"), pageVar(t, "MAP_PAD")
	line := "boxW = cols.length ? (w - MAP_PAD - BADGE_R - COL_GAP * (cols.length - 1)) / cols.length : w - MAP_PAD - BADGE_R;"
	if !strings.Contains(pageFunc(t, "mapFrame"), line) {
		t.Errorf("mapFrame does not set %q", line)
	}
	if !strings.Contains(pageFunc(t, "drawPlan"), "r: BADGE_R") {
		t.Error("drawPlan badge circle does not use r: BADGE_R")
	}
	for _, w := range []float64{200, 360, 640, 1000, 1536, 1920} {
		for n := 1; n <= 4; n++ {
			boxW := (w - pad - badge - gap*float64(n-1)) / float64(n)
			for i := 0; i < n; i++ {
				right := float64(i)*(boxW+gap) + boxW + badge
				if right > w-2+1e-9 {
					t.Errorf("w=%v n=%d column %d: right edge %v > %v", w, n, i, right, w-2)
				}
			}
		}
	}
}

// TestNodeNoOutline: a selected or focused box shows only the accent
// border, never an outline on g.node.
func TestNodeNoOutline(t *testing.T) {
	focusNone, ringAccent := false, false
	outlineDecl := regexp.MustCompile(`outline[\w-]*:[^;]*`)
	for _, r := range cssRules(t) {
		sels := strings.Join(r.Selectors, ",")
		if !strings.Contains(sels, "g.node") {
			continue
		}
		body := compact(r.Body)
		if strings.Contains(body, "outline") {
			for _, d := range outlineDecl.FindAllString(body, -1) {
				if d != "outline:none" {
					t.Errorf("rule %q sets %q, want only outline:none", sels, d)
				}
			}
		}
		for _, s := range r.Selectors {
			if (s == "g.node:focus" || s == "g.node:focus-visible") && strings.Contains(body, "outline:none") {
				focusNone = true
			}
			if s == "g.node:focus-visible rect" && strings.Contains(body, "stroke:var(--accent)") {
				ringAccent = true
			}
		}
	}
	if !focusNone {
		t.Error("no rule for g.node:focus or g.node:focus-visible sets outline:none")
	}
	if !ringAccent {
		t.Error("no rule g.node:focus-visible rect sets stroke:var(--accent)")
	}
}

// TestUnpickedOpacity: boxes not selected and not related keep an opacity
// of at least 0.6.
func TestUnpickedOpacity(t *testing.T) {
	body := compact(ruleFor(t, ".map.picked g.node:not(.sel):not(.rel)"))
	m := regexp.MustCompile(`opacity:([0-9.]+)`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("unpicked rule has no opacity: %q", body)
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatalf("opacity %q: %v", m[1], err)
	}
	if v < 0.6 {
		t.Errorf("unpicked opacity %v, want >= 0.6", v)
	}
}

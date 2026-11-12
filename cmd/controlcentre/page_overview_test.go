//go:build windows || darwin

package main

import (
	"regexp"
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

//go:build windows

package main

import (
	"encoding/json"
	"math"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"burnmon/internal/sections"
)

// pctPageJS is the page's pure size code (pctPlan through pctCheck) with a
// stand-in secState, so node can run it next to the Go rules.
func pctPageJS(t *testing.T) string {
	t.Helper()
	page := idlePage()
	s := strings.Index(page, "  var PCT_SECTIONS = ")
	e := strings.Index(page, "  function pctEqual(")
	if s < 0 || e < s {
		t.Fatal("page.html lacks the pct block")
	}
	return "var secState = {standard:{}, plainSet:{}};\n" + page[s:e]
}

type pctCase struct {
	name      string
	mode      string
	off       []string // parts switched off in mode
	scrollbar bool
	secs      map[string]int
	panes     map[string]int
}

var pctCases = []pctCase{
	{name: "fresh", mode: "standard"},
	{name: "status rest", mode: "standard", secs: map[string]int{"status": 20, "sessions_30min": 40, "processes_30min": 20, "todo": 10}},
	{name: "empty shares", mode: "standard", secs: map[string]int{"status": 40}},
	{name: "over no scroll", mode: "standard", secs: map[string]int{"status": 40, "sessions_30min": 40, "processes_30min": 20, "todo": 12}},
	{name: "over scroll", mode: "standard", scrollbar: true, secs: map[string]int{"status": 40, "sessions_30min": 40, "processes_30min": 20, "todo": 12}},
	{name: "starved", mode: "standard", secs: map[string]int{"status": 50, "sessions_30min": 30, "processes_30min": 20}},
	{name: "hidden ignored", mode: "plain", secs: map[string]int{"status": 50, "sessions_30min": 90}},
	{name: "status hidden", mode: "standard", off: []string{"waits", "outcomes", "burn", "git", "agents_map", "sessions"}, secs: map[string]int{"status": 50, "sessions_30min": 30, "processes_30min": 20, "todo": 10}},
	{name: "panes over", mode: "standard", panes: map[string]int{"burn_30min": 60, "vendor_strip": 50}},
	{name: "panes starved", mode: "standard", panes: map[string]int{"system": 70, "process_groups": 30}},
	{name: "panes rest", mode: "standard", panes: map[string]int{"burn_30min": 30, "vendor_strip": 20, "activity_heatmap": 20, "turn_ticker": 20}},
	{name: "hidden pane", mode: "standard", off: []string{"vendor_strip"}, panes: map[string]int{"burn_30min": 60, "vendor_strip": 60}},
	{name: "one row pane", mode: "standard", off: []string{"activity"}, panes: map[string]int{"activity_heatmap": 50, "turn_ticker": 60}},
}

func (c pctCase) state() sections.State {
	s := sections.Default().WithScrollbar(c.scrollbar)
	for _, id := range c.off {
		s = s.With(c.mode, id, false)
	}
	return s
}

// The page and Go give the same split per section and pane and the same
// sentence for every rule (2a to 2f).
func TestPctPageMatchesGo(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	type res struct {
		Err   string                        `json:"err"`
		Secs  map[string]float64            `json:"secs"`
		Panes map[string]map[string]float64 `json:"panes"`
	}
	for _, c := range pctCases {
		s := c.state()
		parts, _ := json.Marshal(map[string]any{"standard": s.Standard, "plain": s.PlainSet})
		secs, _ := json.Marshal(nonNil(c.secs))
		panes, _ := json.Marshal(nonNil(c.panes))
		js := pctPageJS(t) + "\nvar P = " + string(parts) + "; secState.standard = P.standard; secState.plainSet = P.plain;" +
			"\nvar m = '" + c.mode + "', S = " + string(secs) + ", Q = " + string(panes) + ";" +
			"\nvar out = {err: pctCheck(m, S, Q, " + boolJS(c.scrollbar) + "), secs: pctPlan(pctShownSections(m), S, 'status').pct, panes: {}};" +
			"\npctShownSections(m).forEach(function(sec){ var sh = pctShownPanes(m, sec); out.panes[sec] = pctPlan(sh, Q, sh[0]).pct; });" +
			"\nconsole.log(JSON.stringify(out));"
		b, err := exec.Command(node, "-e", js).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: node: %v\n%s", c.name, err, b)
		}
		var got res
		if err := json.Unmarshal([]byte(strings.TrimSpace(string(b))), &got); err != nil {
			t.Fatalf("%s: bad node output %q: %v", c.name, b, err)
		}
		wantErr := ""
		if e := s.CheckPct(c.mode, c.secs, c.panes); e != nil {
			wantErr = e.Error()
		}
		if got.Err != wantErr {
			t.Errorf("%s: page says %q, Go says %q", c.name, got.Err, wantErr)
		}
		sameSplit(t, c.name+" sections", got.Secs, sections.Plan(s.ShownSections(c.mode), c.secs, sections.RestSection).Pct)
		for _, sec := range s.ShownSections(c.mode) {
			sh := s.ShownPanes(c.mode, sec)
			sameSplit(t, c.name+" panes "+sec, got.Panes[sec], sections.Plan(sh, c.panes, sh[0]).Pct)
		}
	}
}

func nonNil(m map[string]int) map[string]int {
	if m == nil {
		return map[string]int{}
	}
	return m
}

func boolJS(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func sameSplit(t *testing.T, name string, got, want map[string]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: page %v, Go %v", name, got, want)
		return
	}
	for k, w := range want {
		if math.Abs(got[k]-w) > 0.001 {
			t.Errorf("%s: page %v, Go %v", name, got, want)
			return
		}
	}
}

// The page's fallback numbers are Go's defaults (Wilco, 2026-11-15:
// prefilled numbers), and the pixel split adds up to the room exactly.
func TestPctPageDefaultsAndPixels(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	js := pctPageJS(t) + "\nconsole.log(JSON.stringify({s: PCT_DEFAULT_SECTIONS, p: PCT_DEFAULT_PANES," +
		" a: pctPixels(['a','b','c'], {a: 30, b: 50, c: 20}, 775.6), b: pctPixels(['a','b','c'], {a: 100/3, b: 100/3, c: 100/3}, 800)}));"
	b, err := exec.Command(node, "-e", js).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, b)
	}
	var got struct {
		S, P, A, B map[string]int
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(b))), &got); err != nil {
		t.Fatalf("bad node output %q: %v", b, err)
	}
	if !reflect.DeepEqual(got.S, sections.DefaultSectionsPct) || !reflect.DeepEqual(got.P, sections.DefaultPanesPct) {
		t.Errorf("page defaults %v / %v, Go %v / %v", got.S, got.P, sections.DefaultSectionsPct, sections.DefaultPanesPct)
	}
	for name, m := range map[string]map[string]int{"30/50/20 of 775": got.A, "thirds of 800": got.B} {
		sum := 0
		for _, v := range m {
			sum += v
		}
		want := 775
		if name == "thirds of 800" {
			want = 800
		}
		if sum != want {
			t.Errorf("%s: %v adds up to %d, want %d", name, m, sum, want)
		}
	}
	if got.A["a"] != 233 || got.A["b"] != 387 || got.A["c"] != 155 {
		t.Errorf("30/50/20 of 775 = %v", got.A)
	}
}

// Rule 2b and 2c in the gear: Save is off with the sentence; the boxes are
// greyed for hidden parts (2e) and a one-pane section has no pane box.
func TestPctGearSaveAndBoxes(t *testing.T) {
	page := idlePage()
	for _, want := range []string{
		`id="secSizeGrid"`, `id="secSizeSave"`, `id="secSizeWhy"`,
		"if(save) save.disabled = !!why;",
		"if(line) line.textContent = why;",
		"i.disabled = !shown;",
		"(multi ? box('panes', p.id, m, p.label) : '')",
		"placeholder=\"auto\"",
		"secSend({ pct: v.pct });",
		"Type a whole number from 1 to 100, or leave the box empty",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	// The boxes come back from the payload only while nobody typed.
	body := idleFn(t, "pctRenderSizes")
	if !strings.Contains(body, "if(!pctDirty){") {
		t.Error("pctRenderSizes overwrites typed boxes")
	}
}

// Side by side the shared row is as tall as the larger number, Status takes
// what that leaves (Wilco, 2026-11-15).
func TestPctSideBySideRowIsTheLarger(t *testing.T) {
	body := idleFn(t, "applyPanelFit")
	for _, want := range []string{
		"var big = Math.max(draw.sessions_30min, draw.processes_30min), small = Math.min(draw.sessions_30min, draw.processes_30min);",
		"var give = Math.max(0, Math.min(small, 100 - (total - small)));",
		"draw.status += give; draw.sessions_30min = draw.processes_30min = big;",
		"stationPeek(true)", "stationPeek(false)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("applyPanelFit lacks %q", want)
		}
	}
}

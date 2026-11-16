package report

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// toolsJS is renderTools with the helpers it calls, cut out of the template,
// plus a small fake DOM that records what it draws. data is the D.months value.
func toolsJS(t *testing.T, months string) string {
	t.Helper()
	s := strings.Index(tpl, "function looksLikeHex(")
	e := strings.Index(tpl, "// ---------- sessions ----------")
	if s < 0 || e < s {
		t.Fatal("template.html lacks the Tools block")
	}
	return `
var D = {months: ` + months + `};
var charts = {}, UI = {theme: 'dark'}, drawn = null;
function n(v){ return (v||0).toLocaleString('en-GB'); }
function esc(s){ return String(s==null?'':s); }
function t(k){ return k; }
function chartFallback(){}
function chartPalette(){ return {accent:'a', text:'t', grid:'g'}; }
function Chart(canvas, cfg){ drawn = cfg; this.destroy = function(){}; }
var rows = [], line = {textContent:'', style:{display:'none'}};
var box = {style:{display:'none'}};
var tbody = {innerHTML:'', appendChild:function(tr){ rows.push(tr.innerHTML); }};
var canvas = {closest:function(){ return box; }};
var document = {
  querySelector:function(sel){ return sel === '#t_tools tbody' ? tbody : null; },
  getElementById:function(id){ return id === 'ch_tools' ? canvas : id === 'tools_builtin' ? line : null; },
  createElement:function(){ return {innerHTML:''}; }
};
` + tpl[s:e] + `
renderTools();
console.log(JSON.stringify({rows: rows, tbody: tbody.innerHTML, line: line.textContent, lineShown: line.style.display !== 'none',
  labels: drawn ? drawn.data.labels : null, data: drawn ? drawn.data.datasets[0].data : null, boxShown: box.style.display !== 'none'}));
`
}

type toolsOut struct {
	Rows      []string
	Tbody     string
	Line      string
	LineShown bool
	Labels    []string
	Data      []int
	BoxShown  bool
}

func runTools(t *testing.T, months string) toolsOut {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	b, err := exec.Command(node, "-e", toolsJS(t, months)).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, b)
	}
	var out toolsOut
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(b))), &out); err != nil {
		t.Fatalf("bad node output %q: %v", b, err)
	}
	return out
}

const toolsMonths = `{
  "2026-10": {"by_tool": {"(built in)": {"calls": 900, "sessions": 40}, "github": {"calls": 30, "sessions": 5}, "notion": {"calls": 10, "sessions": 2}}},
  "2026-11": {"by_tool": {"(built in)": {"calls": 100, "sessions": 10}, "github": {"calls": 60, "sessions": 7}}}
}`

// The Tools page leaves out the "(built in)" row and bar, counts the share of
// calls over the rows it shows, and keeps the number in one line under the
// intro (Wilco, 2026-11-16).
func TestToolsPageHidesBuiltInAndSharesOverShownRows(t *testing.T) {
	out := runTools(t, toolsMonths)
	if len(out.Rows) != 2 {
		t.Fatalf("%d rows %q, want 2 (github, notion)", len(out.Rows), out.Rows)
	}
	for _, r := range out.Rows {
		if strings.Contains(r, "(built in)") {
			t.Errorf("a row still names (built in): %q", r)
		}
	}
	// github 90 of the 100 shown calls, notion 10: 90% and 10%, not 9% and 1%.
	if !strings.Contains(out.Rows[0], "github") || !strings.Contains(out.Rows[0], "90%") {
		t.Errorf("first row %q, want github at 90%%", out.Rows[0])
	}
	if !strings.Contains(out.Rows[1], "notion") || !strings.Contains(out.Rows[1], "10%") {
		t.Errorf("second row %q, want notion at 10%%", out.Rows[1])
	}
	if strings.Join(out.Labels, ",") != "github,notion" {
		t.Errorf("chart labels %v, want github and notion only", out.Labels)
	}
	if len(out.Data) != 2 || out.Data[0] != 90 || out.Data[1] != 10 {
		t.Errorf("chart data %v, want [90 10]", out.Data)
	}
}

func TestToolsPageKeepsTheBuiltInNumberInOneLine(t *testing.T) {
	out := runTools(t, toolsMonths)
	if want := "Claude's own tools: 1,000 calls in 50 sessions, not listed."; out.Line != want || !out.LineShown {
		t.Errorf("line %q (shown %v), want %q shown", out.Line, out.LineShown, want)
	}
	// The line sits under the intro, before the chart.
	lede := strings.Index(tpl, `<p class="lede">Which connectors, plugins and skills`)
	li := strings.Index(tpl, `id="tools_builtin"`)
	chart := strings.Index(tpl, `id="ch_tools"`)
	if lede < 0 || li < lede || chart < li {
		t.Errorf("tools_builtin must sit between the intro and the chart (lede %d, line %d, chart %d)", lede, li, chart)
	}
}

// Only Claude's own tools: nothing to list, the no-data row shows, and the
// line still says what was left out.
func TestToolsPageOnlyBuiltIn(t *testing.T) {
	out := runTools(t, `{"2026-11": {"by_tool": {"(built in)": {"calls": 5, "sessions": 2}}}}`)
	if len(out.Rows) != 0 || !strings.Contains(out.Tbody, "tools_no_data_html") {
		t.Errorf("rows %q, tbody %q: want the no-data row", out.Rows, out.Tbody)
	}
	if out.BoxShown {
		t.Error("the chart box shows with nothing to draw")
	}
	if out.Line != "Claude's own tools: 5 calls in 2 sessions, not listed." || !out.LineShown {
		t.Errorf("line %q (shown %v)", out.Line, out.LineShown)
	}
}

// No Claude own tools at all: the line is hidden, the shares are what they were.
func TestToolsPageWithoutBuiltIn(t *testing.T) {
	out := runTools(t, `{"2026-11": {"by_tool": {"github": {"calls": 3, "sessions": 1}, "notion": {"calls": 1, "sessions": 1}}}}`)
	if out.LineShown {
		t.Errorf("the built-in line shows (%q) with no built-in calls", out.Line)
	}
	if len(out.Rows) != 2 || !strings.Contains(out.Rows[0], "75%") || !strings.Contains(out.Rows[1], "25%") {
		t.Errorf("rows %q, want 75%% and 25%%", out.Rows)
	}
}

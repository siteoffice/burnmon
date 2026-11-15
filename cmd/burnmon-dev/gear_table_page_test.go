//go:build windows

package main

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"burnmon/internal/sections"
)

// gearNode runs the page's pct block, pctBuildTable and the given lines in
// node, with the parts of state s, and returns what the script prints.
func gearNode(t *testing.T, s sections.State, script string) string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	parts, _ := json.Marshal(map[string]any{"standard": s.Standard, "plain": s.PlainSet})
	groups, _ := json.Marshal(sections.Groups)
	js := pctPageJS(t) +
		"\nfunction esc(x){ return String(x).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/\"/g,'&quot;'); }" +
		"\n" + idleFn(t, "pctBuildTable") +
		"\nvar P = " + string(parts) + "; secState.standard = P.standard; secState.plainSet = P.plain; secState.groups = " + string(groups) + ";" +
		"\n" + script
	b, err := exec.Command(node, "-e", js).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, b)
	}
	return strings.TrimSpace(string(b))
}

// Item 1 and 2 (first look 2026-11-15): one table, the Standard and Plain
// header once at the top, each section on one row with its own ticks and
// percent boxes, and every part tick in the table; no separate list.
func TestGearOneTable(t *testing.T) {
	page := idlePage()
	if strings.Contains(page, "secSettingsCols") {
		t.Error("the separate checkbox list secSettingsCols is still on the page")
	}
	html := gearNode(t, sections.Default(), "var g = {innerHTML:''}; pctBuildTable(g); console.log(g.innerHTML);")
	if n := strings.Count(html, "<thead>"); n != 1 {
		t.Errorf("%d table heads, want 1", n)
	}
	if n := strings.Count(html, "<th>Standard</th>"); n != 1 {
		t.Errorf("Standard header %d times, want once", n)
	}
	rows := regexp.MustCompile(`<tr class="setsecrow">.*?</tr>`).FindAllString(html, -1)
	if len(rows) != 4 {
		t.Fatalf("%d section rows, want 4", len(rows))
	}
	for i, g := range sections.Groups {
		r := rows[i]
		if !strings.Contains(r, ">"+g.Label+"</td>") {
			t.Errorf("section row %d lacks %q: %s", i, g.Label, r)
		}
		for _, m := range []string{"standard", "plain"} {
			if !strings.Contains(r, `data-pct-kind="sections" data-pct-id="`+g.ID+`" data-mode="`+m+`"`) {
				t.Errorf("%s row lacks its %s percent box", g.ID, m)
			}
			sw := `data-sec-sw="` + g.ID + `" data-mode="` + m + `"`
			if len(g.Parts) == 1 {
				sw = `data-sw="` + g.Parts[0].ID + `" data-mode="` + m + `"`
			}
			if !strings.Contains(r, sw) {
				t.Errorf("%s row lacks its %s tick %s", g.ID, m, sw)
			}
		}
	}
	// Every part has exactly one tick per mode.
	for _, id := range sections.IDs {
		for _, m := range []string{"standard", "plain"} {
			if n := strings.Count(html, `data-sw="`+id+`" data-mode="`+m+`"`); n != 1 {
				t.Errorf("part %s has %d %s ticks, want 1", id, n, m)
			}
		}
	}
	// A section with one pane (To Do) has no pane box.
	if strings.Contains(html, `data-pct-kind="panes" data-pct-id="todo"`) {
		t.Error("To Do has a pane box")
	}
}

// Item 3: the section row is bold on a band, its panes indented, and the
// section row shows its pane total.
func TestGearGroupingAndPaneTotal(t *testing.T) {
	page := idlePage()
	for _, want := range []string{
		"#secSizeGrid tr.setsecrow td{font-weight:700; background:var(--panel2)",
		"#secSizeGrid .lv1{padding-left:",
		"#secSizeGrid .lv2{padding-left:",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	s := sections.Default()
	out := gearNode(t, s, `console.log(JSON.stringify([
  pctPaneTotal('standard', 'status', {status_row1:50, sessions:50}),
  pctPaneTotal('standard', 'sessions_30min', {burn_30min:60, vendor_strip:50, activity_heatmap:20, turn_ticker:20}),
  pctPaneTotal('standard', 'processes_30min', {system:50}),
  pctPaneTotal('standard', 'todo', {})
]));`)
	var got []struct {
		Text string `json:"text"`
		Over bool   `json:"over"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("bad node output %q: %v", out, err)
	}
	want := []struct {
		text string
		over bool
	}{{"panes 100 of 100", false}, {"panes 150 of 100", true}, {"panes 50 + auto of 100", false}, {"", false}}
	for i, w := range want {
		if got[i].Text != w.text || got[i].Over != w.over {
			t.Errorf("pane total %d = %+v, want %q over %v", i, got[i], w.text, w.over)
		}
	}
	// A hidden pane does not count.
	h := s.With("standard", sections.VendorStrip, false)
	out = gearNode(t, h, `console.log(pctPaneTotal('standard', 'sessions_30min', {burn_30min:40, vendor_strip:20, activity_heatmap:20, turn_ticker:20}).text);`)
	if out != "panes 80 of 100" {
		t.Errorf("with the vendor strip hidden: %q, want panes 80 of 100", out)
	}
}

// Item 4: a hidden box is greyed with the hover text, and one total line per
// mode lists only the counted sections.
func TestGearHiddenTipAndTotalLines(t *testing.T) {
	page := idlePage()
	for _, want := range []string{
		"var PCT_HIDDEN_TIP = 'hidden in this mode, not counted';",
		"i.parentNode.title = PCT_HIDDEN_TIP;",
		"i.disabled = !shown;",
		`id="secSizeTotals"`,
		"#secSizeGrid input:disabled{opacity:.4; pointer-events:none}",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	s := sections.Default().With("plain", sections.Todo, false)
	for _, p := range []string{sections.Waits, sections.Outcomes, sections.Burn, sections.Git, sections.AgentsMap, sections.Sessions,
		sections.Burn30, sections.System} {
		s = s.With("plain", p, true)
	}
	secs := `{status:30, sessions_30min:60, processes_30min:60, todo:10}`
	out := gearNode(t, s, "console.log(pctTotalLine('plain', "+secs+")); console.log(pctTotalLine('standard', {status:30}));"+
		" secState.plainSet = {}; console.log(pctTotalLine('plain', {}));")
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	want := []string{
		"Plain: Status 30 + Sessions 60 + Processes 60 = 150 (To Do hidden)",
		"Standard: Status 30 + Sessions auto + Processes auto + To Do auto = 30",
		"Plain: nothing shown (Status, Sessions, Processes, To Do hidden)",
	}
	if len(lines) != len(want) {
		t.Fatalf("lines %q", lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

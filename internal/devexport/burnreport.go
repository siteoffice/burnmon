package devexport

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html/template"
	"os"
	"sort"
	"time"
)

// BurnReportDays is the report window: 14 local calendar days counting today.
const BurnReportDays = 14

// BurnReportFooter is the fixed closing line of the report.
const BurnReportFooter = "Want an analysis of this report? Send the zip to info@zerononsense.dev. One-off analysis: EUR 49."

// BurnReportWindow returns [since, until): until is now, since is local
// midnight BurnReportDays-1 days before now's local day.
func BurnReportWindow(now time.Time) (since, until time.Time) {
	n := now.In(time.Local)
	since = time.Date(n.Year(), n.Month(), n.Day()-(BurnReportDays-1), 0, 0, 0, 0, time.Local)
	return since, now
}

// DaysWithData counts the distinct local days holding at least one turn.
func DaysWithData(b Bundle) int {
	days := map[string]bool{}
	for _, t := range b.Turns {
		days[t.At.In(time.Local).Format("2006-01-02")] = true
	}
	return len(days)
}

type brView struct {
	Since, Until string
	Days         int
	WindowDays   int
	Names        string
	Footer       string
	Turns        int
	Tokens       string
	Cost         string
	Vendors      []brTotal
	Models       []HarnessModelTotal
	Daily        []DailyRow
	Top          []brTop
	Episodes     []brEpisode
}

type brTotal struct {
	Vendor string
	Tokens string
	Cost   string
}

type brTop struct {
	At, Vendor, Harness, Model, Project, Tokens, Cost string
}

type brEpisode struct {
	Start, End string
	Peak       int
	Avg        string
}

var brTmpl = template.Must(template.New("burn").Funcs(template.FuncMap{
	"tok":  fmtTok,
	"cost": fmtCost,
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>BurnMon burn report</title>
<style>
body{font-family:Segoe UI,Arial,sans-serif;margin:0;color:#1a1a1a;background:#fff}
main{width:80vw;max-width:100%;margin:0 auto;padding:1rem 0}
@media (max-width:900px){main{width:100%;padding:1rem}}
table{border-collapse:collapse;width:100%;margin:.5rem 0 1.5rem}
th,td{text-align:left;padding:.3rem .5rem;border-bottom:1px solid #ddd}
td.n,th.n{text-align:right}
footer{margin-top:2rem;padding-top:1rem;border-top:1px solid #ddd}
@media (prefers-color-scheme:dark){body{background:#111;color:#eee}th,td,footer{border-color:#444}}
</style></head><body><main>
<h1>BurnMon burn report</h1>
<p>Period: {{.Since}} to {{.Until}} (local time). Data on {{.Days}} of {{.WindowDays}} days.</p>
<p>Names: {{.Names}}.</p>
<p>{{.Turns}} turns, {{.Tokens}} tokens, {{.Cost}} (allocated estimate, not an invoice).</p>
<h2>By vendor</h2>
<table><tr><th>Vendor</th><th class="n">Tokens</th><th class="n">Cost</th></tr>
{{range .Vendors}}<tr><td>{{.Vendor}}</td><td class="n">{{.Tokens}}</td><td class="n">{{.Cost}}</td></tr>
{{else}}<tr><td colspan="3">No turns in this period.</td></tr>{{end}}</table>
<h2>By harness and model</h2>
<table><tr><th>Harness</th><th>Model</th><th class="n">Fresh</th><th class="n">Cache write</th><th class="n">Cache read</th><th class="n">Output</th><th class="n">Cost</th></tr>
{{range .Models}}<tr><td>{{.Harness}}</td><td>{{.Model}}</td><td class="n">{{tok .Fresh}}</td><td class="n">{{tok .CacheWrite}}</td><td class="n">{{tok .CacheRead}}</td><td class="n">{{tok .Output}}</td><td class="n">{{cost .CostUSD}}</td></tr>
{{else}}<tr><td colspan="7">None.</td></tr>{{end}}</table>
<h2>Per day</h2>
<table><tr><th>Date</th><th>Harness</th><th>Model</th><th class="n">Fresh</th><th class="n">Cache write</th><th class="n">Cache read</th><th class="n">Output</th><th class="n">Cost</th></tr>
{{range .Daily}}<tr><td>{{.Date}}</td><td>{{.Harness}}</td><td>{{.Model}}</td><td class="n">{{tok .Fresh}}</td><td class="n">{{tok .CacheWrite}}</td><td class="n">{{tok .CacheRead}}</td><td class="n">{{tok .Output}}</td><td class="n">{{cost .CostUSD}}</td></tr>
{{else}}<tr><td colspan="8">None.</td></tr>{{end}}</table>
<h2>Top turns</h2>
<table><tr><th>When</th><th>Vendor</th><th>Harness</th><th>Model</th><th>Project</th><th class="n">Tokens</th><th class="n">Cost</th></tr>
{{range .Top}}<tr><td>{{.At}}</td><td>{{.Vendor}}</td><td>{{.Harness}}</td><td>{{.Model}}</td><td>{{.Project}}</td><td class="n">{{.Tokens}}</td><td class="n">{{.Cost}}</td></tr>
{{else}}<tr><td colspan="7">None.</td></tr>{{end}}</table>
<h2>Pressure episodes</h2>
<table><tr><th>Start</th><th>End</th><th class="n">Peak</th><th class="n">Average</th></tr>
{{range .Episodes}}<tr><td>{{.Start}}</td><td>{{.End}}</td><td class="n">{{.Peak}}</td><td class="n">{{.Avg}}</td></tr>
{{else}}<tr><td colspan="4">None.</td></tr>{{end}}</table>
<footer>{{.Footer}}</footer>
</main></body></html>
`))

// BuildBurnReportHTML renders b as one self-contained, offline HTML page.
func BuildBurnReportHTML(b Bundle) (string, error) {
	const tf = "2006-01-02 15:04"
	v := brView{
		Since: b.Since.In(time.Local).Format(tf), Until: b.Until.In(time.Local).Format(tf),
		Days: DaysWithData(b), WindowDays: BurnReportDays,
		Names: "names shown", Footer: BurnReportFooter, Turns: len(b.Turns),
		Models: b.HarnessModelTotals, Daily: b.Daily,
	}
	if b.Redacted {
		v.Names = "redacted (project, owner, client and session ids are hashed)"
	}
	var total int64
	var cost float64
	tokens := map[string]int64{}
	costs := map[string]float64{}
	var order []string
	for _, t := range b.Turns {
		if _, ok := tokens[t.Vendor]; !ok {
			order = append(order, t.Vendor)
		}
		tokens[t.Vendor] += t.tokens()
		costs[t.Vendor] += t.CostUSD
		total += t.tokens()
		cost += t.CostUSD
	}
	sort.Strings(order)
	for _, name := range order {
		v.Vendors = append(v.Vendors, brTotal{Vendor: name, Tokens: fmtTok(tokens[name]), Cost: fmtCost(costs[name])})
	}
	v.Tokens, v.Cost = fmtTok(total), fmtCost(cost)
	for _, t := range b.TopTurns {
		v.Top = append(v.Top, brTop{
			At: t.At.In(time.Local).Format(tf), Vendor: t.Vendor, Harness: t.Harness, Model: t.Model,
			Project: t.Project, Tokens: fmtTok(t.tokens()), Cost: fmtCost(t.CostUSD),
		})
	}
	for _, e := range b.PressureEpisodes {
		v.Episodes = append(v.Episodes, brEpisode{
			Start: e.Start.In(time.Local).Format(tf), End: e.End.In(time.Local).Format(tf),
			Peak: e.PeakScore, Avg: fmt.Sprintf("%.0f", e.AvgScore),
		})
	}
	var buf bytes.Buffer
	if err := brTmpl.Execute(&buf, v); err != nil {
		return "", fmt.Errorf("devexport: burn report html: %w", err)
	}
	return buf.String(), nil
}

// WriteBurnReportZip writes burn-report.html and bundle.json to path,
// through path+".tmp" and a rename so a second write replaces the first.
func WriteBurnReportZip(path string, b Bundle) error {
	page, err := BuildBurnReportHTML(b)
	if err != nil {
		return err
	}
	data, err := BuildDataJSON(b)
	if err != nil {
		return fmt.Errorf("devexport: burn report bundle.json: %w", err)
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("devexport: create %s: %w", tmp, err)
	}
	zw := zip.NewWriter(f)
	werr := func() error {
		for _, e := range []struct {
			name string
			body []byte
		}{{"burn-report.html", []byte(page)}, {"bundle.json", data}} {
			w, err := zw.Create(e.name)
			if err != nil {
				return err
			}
			if _, err := w.Write(e.body); err != nil {
				return err
			}
		}
		return zw.Close()
	}()
	cerr := f.Close()
	if werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(tmp)
		return fmt.Errorf("devexport: write %s: %w", tmp, werr)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("devexport: rename to %s: %w", path, err)
	}
	return nil
}

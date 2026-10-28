package graph

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParkedLines(t *testing.T) {
	fence := "```"
	for name, c := range map[string]struct {
		text string
		want int
	}{
		"none":                        {"# R\n\n## Close-out\n- none\n", 0},
		"no block":                    {"# R\n- ASK parked: not in a block\n", 0},
		"two lines":                   {"## Close-out\n- ASK parked: which one?\n- HAND parked: run it\n\n" + fence + "\ncmd\n" + fence + "\n- DO done: x\n", 2},
		"continuation does not count": {"## Close-out\n- ASK parked: q\n  - HAND parked: not an item\n", 1},
		"fenced is ignored":           {"## Close-out\n- DO done: x\n" + fence + "\n- ASK parked: in code\n" + fence + "\n", 0},
		"block ends at next heading":  {"## Close-out\n- DO done: x\n## Other\n- ASK parked: outside\n", 0},
		"invalid state is not parked": {"## Close-out\n- DO parked: bad\n- DROP parked: bad\n- ASK deferred: later\n", 0},
		"class is exact":              {"## Close-out\n- ask parked: lower\n- ASK PARKED: upper state is fine\n", 1},
		"crlf":                        {"## Close-out\r\n- ASK parked: q\r\n- HAND parked: h\r\n", 2},
		"fenced heading is no block":  {fence + "\n## Close-out\n- ASK parked: x\n" + fence + "\n", 0},
	} {
		if got := ParkedLines(c.text); got != c.want {
			t.Errorf("%s: ParkedLines = %d, want %d", name, got, c.want)
		}
	}
}

func TestReadForemanReportFields(t *testing.T) {
	root := t.TempDir()
	mk := func(slug, status string) string {
		dir := filepath.Join(root, "sprints", "p", slug)
		write(t, filepath.Join(dir, "sprint.json"), `{"sprint_id":"p__`+slug+`__1","slug":"`+slug+`","status":"`+status+`","phase":"report","updated_at":"2026-10-23T10:00:00Z"}`)
		return dir
	}
	report := "## Close-out\n- ASK parked: one\n- HAND parked: two\n\n```\nx\n```\n"
	write(t, filepath.Join(mk("withreport", "waiting-on-you"), "report.md"), report)
	mk("noreport", "waiting-on-you")
	write(t, filepath.Join(mk("running", "running"), "report.md"), report)
	write(t, filepath.Join(mk("clean", "waiting-on-you"), "report.md"), "## Close-out\n- none\n")
	big := strings.Repeat("x", maxReportBytes+10)
	write(t, filepath.Join(mk("big", "waiting-on-you"), "report.md"), big+"\n## Close-out\n- ASK parked: past the cap\n")

	sprints, _, err := ReadForeman(root)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Sprint{}
	for _, s := range sprints {
		by[s.Slug] = s
	}
	if s := by["withreport"]; s.ReportPath != filepath.Join(root, "sprints", "p", "withreport", "report.md") || s.ParkedLines != 2 {
		t.Errorf("withreport = %q, %d", s.ReportPath, s.ParkedLines)
	}
	if s := by["noreport"]; s.ReportPath != "" || s.ParkedLines != 0 {
		t.Errorf("noreport = %q, %d", s.ReportPath, s.ParkedLines)
	}
	if s := by["running"]; s.ReportPath != "" || s.ParkedLines != 0 {
		t.Errorf("a sprint that is not waiting is not read: %q, %d", s.ReportPath, s.ParkedLines)
	}
	if s := by["clean"]; s.ReportPath == "" || s.ParkedLines != 0 {
		t.Errorf("clean = %q, %d", s.ReportPath, s.ParkedLines)
	}
	if s := by["big"]; s.ReportPath == "" || s.ParkedLines != 0 {
		t.Errorf("a report past the size cap is read only up to the cap: %q, %d", s.ReportPath, s.ParkedLines)
	}
}

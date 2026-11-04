package devexport

import (
	"archive/zip"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"burnmon/internal/schema"
	"burnmon/internal/sysmon"
)

const (
	brSalt      = "salt-of15"
	brProject   = `C:\clients\Acme\billing`
	brOwner     = "ownerzed"
	brClient    = "ClientQ"
	brTitle     = "secret prompt text"
	brSessionID = "3f2b8c1e-9d4a-4e7b-8a61-5c0d2e9f7b44"
)

func brNow() time.Time { return time.Date(2026, 11, 3, 15, 0, 0, 0, time.Local) }

// brEvents returns one turn at 12:00 local on each of the `days` days ending today.
func brEvents(days int) []schema.Event {
	var evs []schema.Event
	for i := 0; i < days; i++ {
		at := time.Date(2026, 11, 3-i, 12, 0, 0, 0, time.Local)
		e := testEvent(brSessionID, "claude-sonnet-5", at, 1000, ptr(0), ptr(0), 100)
		e.Project, e.Owner, e.Client, e.Title = brProject, brOwner, brClient, brTitle
		evs = append(evs, e)
	}
	return evs
}

func brBundle(events []schema.Event) Bundle {
	since, until := BurnReportWindow(brNow())
	return Assemble(events, testConfig(), nil, nil, sysmon.MachineProfile{}, since, until, brNow(), nil)
}

func brHTML(t *testing.T, b Bundle) string {
	t.Helper()
	h, err := BuildBurnReportHTML(b)
	if err != nil {
		t.Fatalf("BuildBurnReportHTML: %v", err)
	}
	return h
}

func TestBurnReportWindow_Is14LocalDaysIncludingToday(t *testing.T) {
	since, until := BurnReportWindow(brNow())
	want := time.Date(2026, 10, 21, 0, 0, 0, 0, time.Local)
	if !since.Equal(want) {
		t.Fatalf("since = %v, want %v", since, want)
	}
	if !until.Equal(brNow()) {
		t.Fatalf("until = %v, want now", until)
	}
}

func TestBurnReport_20DaysOfRowsGivesLast14(t *testing.T) {
	b := brBundle(brEvents(20))
	if n := DaysWithData(b); n != 14 {
		t.Fatalf("DaysWithData = %d, want 14", n)
	}
	if h := brHTML(t, b); !strings.Contains(h, "14 of 14 days") {
		t.Fatalf("html lacks %q", "14 of 14 days")
	}
}

func TestBurnReport_5DaysSays5Of14(t *testing.T) {
	if h := brHTML(t, brBundle(brEvents(5))); !strings.Contains(h, "5 of 14 days") {
		t.Fatalf("html lacks %q", "5 of 14 days")
	}
}

func TestBurnReport_ZeroTurns(t *testing.T) {
	if h := brHTML(t, brBundle(nil)); !strings.Contains(h, "0 of 14 days") {
		t.Fatalf("html lacks %q", "0 of 14 days")
	}
}

func TestBurnReport_WindowEdges(t *testing.T) {
	since, until := BurnReportWindow(brNow())
	evs := []schema.Event{
		testEvent("s1", "claude-sonnet-5", since, 1000, ptr(0), ptr(0), 100),
		testEvent("s2", "claude-sonnet-5", until, 1000, ptr(0), ptr(0), 100),
	}
	b := brBundle(evs)
	if len(b.Turns) != 1 || !b.Turns[0].At.Equal(since) {
		t.Fatalf("want only the turn at since, got %d turns", len(b.Turns))
	}
}

var attrURL = regexp.MustCompile(`(?i)(src|href)\s*=\s*["']?https?://`)

func TestBurnReportHTML_OfflineAndFooter(t *testing.T) {
	h := brHTML(t, brBundle(brEvents(3)))
	if attrURL.MatchString(h) {
		t.Fatalf("html carries a network src or href")
	}
	if strings.Contains(strings.ToLower(h), "<script") {
		t.Fatalf("html carries a script tag")
	}
	if !strings.Contains(h, BurnReportFooter) {
		t.Fatalf("html lacks the exact footer")
	}
}

func TestBurnReportHTML_SaysRedactedOrNamesShown(t *testing.T) {
	b := brBundle(brEvents(3))
	if h := brHTML(t, Redact(b, brSalt)); !strings.Contains(h, "redacted") {
		t.Fatalf("redacted html lacks 'redacted'")
	}
	if h := brHTML(t, b); !strings.Contains(h, "names shown") {
		t.Fatalf("plain html lacks 'names shown'")
	}
}

func TestBurnReportHTML_EscapesNames(t *testing.T) {
	evs := brEvents(2)
	evs[0].Project = "<script>x</script>&"
	h := brHTML(t, brBundle(evs))
	if strings.Contains(h, "<script>x</script>") {
		t.Fatalf("project name appears unescaped")
	}
}

func readZip(t *testing.T, path string) map[string]string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open entry: %v", err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read entry: %v", err)
		}
		out[f.Name] = string(data)
	}
	return out
}

func brJSONEscaped(s string) string { return strings.ReplaceAll(s, `\`, `\\`) }

func brSecrets() []string {
	return []string{
		brProject, brJSONEscaped(brProject), "Acme", "billing", `C:\clients`,
		brOwner, brClient, brTitle, brSessionID,
	}
}

func brLeaks(files map[string]string) []string {
	var hits []string
	for name, body := range files {
		for _, s := range brSecrets() {
			if strings.Contains(body, s) {
				hits = append(hits, name+":"+s)
			}
		}
	}
	return hits
}

func TestBurnReportZip_RedactionByteSearch(t *testing.T) {
	dir := t.TempDir()
	b := brBundle(brEvents(4))

	red := filepath.Join(dir, "red.zip")
	if err := WriteBurnReportZip(red, Redact(b, brSalt)); err != nil {
		t.Fatalf("write redacted: %v", err)
	}
	files := readZip(t, red)
	var names []string
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "bundle.json,burn-report.html" {
		t.Fatalf("entries = %v", names)
	}
	if hits := brLeaks(files); len(hits) > 0 {
		t.Fatalf("redacted zip leaks: %v", hits)
	}

	plain := filepath.Join(dir, "plain.zip")
	if err := WriteBurnReportZip(plain, b); err != nil {
		t.Fatalf("write plain: %v", err)
	}
	pf := readZip(t, plain)
	all := pf["bundle.json"] + pf["burn-report.html"]
	for _, s := range []string{"Acme", brOwner, brClient} {
		if !strings.Contains(all, s) {
			t.Fatalf("plain zip lacks %q", s)
		}
	}

	// Guard: the search itself must fire when Redact is skipped.
	if len(brLeaks(pf)) == 0 {
		t.Fatalf("redaction search found nothing on an un-redacted bundle")
	}
}

func TestBurnReportZip_OverwritesSecondWrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.zip")
	b := brBundle(brEvents(2))
	for i := 0; i < 2; i++ {
		if err := WriteBurnReportZip(p, b); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
}

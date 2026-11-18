package lamp

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixedWeek is the week the goldens show: 4 h 12 min of waiting (25 bricks),
// a longest wait of 38 min, the lamp on for 1 d 7 h (shown as 1 d).
var fixedWeek = Week{Year: 2026, Number: 10, WaitSeconds: 15120, LongestSeconds: 2280, OnSeconds: 111600}

func cardGolden(t *testing.T, name, got string) {
	t.Helper()
	path := "testdata/" + name
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/lamp -update once)", err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != got {
		t.Errorf("the card differs from %s (run with -update after a change you mean)", path)
	}
}

func TestCardGoldens(t *testing.T) {
	cardGolden(t, "card_story.golden.svg", CardStory(fixedWeek, "en"))
	cardGolden(t, "card_wide.golden.svg", CardWide(fixedWeek, "en"))
}

func TestWeekOfSumsTheISOWeekMondayToSunday(t *testing.T) {
	days := map[string]DayRec{
		"2026-03-01": {AgentWaitSeconds: 1000, LongestWaitSeconds: 9000, LampOnSeconds: 1000}, // the Sunday before
		"2026-03-02": {AgentWaitSeconds: 60, LongestWaitSeconds: 30, LampOnSeconds: 100},
		"2026-03-04": {AgentWaitSeconds: 40, LongestWaitSeconds: 90, LampOnSeconds: 200},
		"2026-03-08": {AgentWaitSeconds: 5, LongestWaitSeconds: 10, LampOnSeconds: 7},
		"2026-03-09": {AgentWaitSeconds: 1000, LongestWaitSeconds: 9000, LampOnSeconds: 1000}, // the Monday after
	}
	w := WeekOf(days, time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC))
	if w.Year != 2026 || w.Number != 10 || w.WaitSeconds != 105 || w.LongestSeconds != 90 || w.OnSeconds != 307 {
		t.Fatalf("week %+v", w)
	}
	if s := WeekOf(days, time.Date(2026, 3, 8, 23, 59, 0, 0, time.UTC)); s.Number != 10 || s.WaitSeconds != 105 {
		t.Fatalf("a Sunday belongs to its own week: %+v", s)
	}
}

func TestCardFilesAreNamedByISOWeek(t *testing.T) {
	story, wide := CardFileNames(Week{Year: 2026, Number: 3})
	if story != "glim-week-2026-W03-story.svg" || wide != "glim-week-2026-W03-wide.svg" {
		t.Fatalf("%q %q", story, wide)
	}
}

func TestCardSizesAndContent(t *testing.T) {
	for name, c := range map[string]struct {
		svg  string
		w, h string
	}{
		"story": {CardStory(fixedWeek, "en"), `width="1080" height="1920"`, ""},
		"wide":  {CardWide(fixedWeek, "en"), `width="1200" height="630"`, ""},
	} {
		if !strings.Contains(c.svg, c.w) {
			t.Errorf("%s: no %s", name, c.w)
		}
		for _, want := range []string{
			"My agents waited on me.", ">4 h<", ">38 min<", ">1 d<",
			"<title>4 h 12 min</title>", "<title>38 min</title>", "<title>1 d 7 h</title>",
			"BurnMon Dev, measured while the lamp was on", "github.com/siteoffice/burnmon",
		} {
			if !strings.Contains(c.svg, want) {
				t.Errorf("%s: missing %q", name, want)
			}
		}
		if strings.Contains(c.svg, "demo") {
			t.Errorf("%s: a real card says demo", name)
		}
		if strings.Count(c.svg, `class="card-brick"`) != 25 {
			t.Errorf("%s: %d bricks, want 25", name, strings.Count(c.svg, `class="card-brick"`))
		}
		checkCardSVG(t, name, c.svg)
	}
}

func checkCardSVG(t *testing.T, name, svg string) {
	t.Helper()
	for _, bad := range []string{"<script", "<image", "href", string(rune(0x2014)), "<foreignObject", "url("} {
		if strings.Contains(svg, bad) {
			t.Errorf("%s holds %q", name, bad)
		}
	}
	if len(svg) > 100*1024 {
		t.Errorf("%s is %d bytes", name, len(svg))
	}
	d := xml.NewDecoder(strings.NewReader(svg))
	for {
		if _, err := d.Token(); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("%s is not XML: %v", name, err)
		}
	}
}

func TestCardBricksOneTenMinutesAtMostSixty(t *testing.T) {
	for wait, want := range map[int64]int{0: 0, 599: 0, 600: 1, 3540: 5, 36000: 60, 99999: 60} {
		w := fixedWeek
		w.WaitSeconds = wait
		if got := strings.Count(CardWide(w, "en"), `class="card-brick"`); got != want {
			t.Errorf("%d s: %d bricks, want %d", wait, got, want)
		}
	}
}

func TestCardDemoSaysDemoAndDutchUsesTheDutchSentence(t *testing.T) {
	w := DemoWeek(time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC))
	if !w.Demo || w.Number != 10 || w.WaitSeconds <= 0 {
		t.Fatalf("demo week %+v", w)
	}
	for name, svg := range map[string]string{"story": CardStory(w, "en"), "wide": CardWide(w, "en")} {
		if !strings.Contains(svg, ">demo<") {
			t.Errorf("%s: no demo mark", name)
		}
		checkCardSVG(t, name, svg)
	}
	nl := CardWide(fixedWeek, "nl")
	if !strings.Contains(nl, "Mijn agents wachtten op mij.") || strings.Contains(nl, "My agents waited") {
		t.Error("the Dutch card does not use the Dutch sentence")
	}
}

func TestCardFromALedgerThatSawMarkerStringsHoldsNone(t *testing.T) {
	l := LoadLedger("")
	t0 := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	l.Observe(t0, ledgerSnap(t0), DefaultSettings())
	t1 := t0.Add(2 * time.Second)
	l.Observe(t1, ledgerSnap(t1), DefaultSettings())
	w := WeekOf(l.Days(), t1)
	if w.WaitSeconds != 4 {
		t.Fatalf("week %+v", w)
	}
	for _, svg := range []string{CardStory(w, "en"), CardWide(w, "en")} {
		if strings.Contains(svg, "MARK_") {
			t.Fatal("a marker string reached a card")
		}
	}
}

func TestWriteCardsWritesTwoFilesAndRefusesAMissingFolder(t *testing.T) {
	dir := t.TempDir()
	names, err := WriteCards(dir, fixedWeek, "en")
	if err != nil || len(names) != 2 {
		t.Fatalf("%v %v", names, err)
	}
	for _, n := range names {
		if b, err := os.ReadFile(filepath.Join(dir, n)); err != nil || !strings.HasPrefix(string(b), "<svg") {
			t.Errorf("%s: %v", n, err)
		}
	}
	if _, err := WriteCards(filepath.Join(dir, "missing"), fixedWeek, "en"); err == nil {
		t.Fatal("a missing folder was accepted")
	}
}

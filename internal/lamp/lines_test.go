package lamp

import (
	"hash/fnv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"burnmon/internal/waits"
)

var allMoods = []string{MoodOff, MoodAsleep, MoodWorking, MoodDone, MoodYourTurn, MoodBlocked, MoodBored, MoodSweating}

func TestLinesShape(t *testing.T) {
	perMood := map[string]int{}
	for _, l := range Lines() {
		perMood[l.Mood]++
		for lang, s := range map[string]string{"en": l.EN, "nl": l.NL} {
			if s == "" {
				t.Errorf("%s line of %s is empty", lang, l.Mood)
			}
			if n := utf8.RuneCountInString(s); n > 60 {
				t.Errorf("%q has %d runes, more than 60", s, n)
			}
			if strings.ContainsRune(s, '—') {
				t.Errorf("%q has an em dash", s)
			}
			if strings.Contains(s, "!") {
				t.Errorf("%q has an exclamation mark", s)
			}
			rest := s
			for _, p := range placeholders {
				rest = strings.ReplaceAll(rest, p, "")
			}
			if strings.ContainsAny(rest, "{}") {
				t.Errorf("%q has a placeholder that is not one of the four", s)
			}
		}
	}
	want := map[string]int{MoodOff: 1, MoodAsleep: 4, MoodWorking: 4, MoodDone: 3, MoodYourTurn: 4, MoodBlocked: 4, MoodBored: 4, MoodSweating: 4}
	for _, m := range allMoods {
		if perMood[m] != want[m] {
			t.Errorf("mood %s has %d lines, want %d", m, perMood[m], want[m])
		}
	}
}

// pick is the documented formula, written out apart from the code.
func pick(topID, mood string, at time.Time, count int) int {
	h := fnv.New32a()
	h.Write([]byte(topID + mood))
	return int((uint64(h.Sum32()) + uint64(at.Unix()/60/5)) % uint64(count))
}

func lineState(mood string, rows ...Row) State {
	return State{Mood: mood, Rows: rows, Agents: 2, Counts: waits.Counts{Blocked: 3}, LongestWaitMin: 14, BurnRate: 42000}
}

func TestLinePickFormulaOnThreeInputs(t *testing.T) {
	cases := []struct {
		mood string
		rows []Row
		at   time.Time
		lang string
	}{
		{MoodAsleep, nil, time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC), "en"},
		{MoodYourTurn, []Row{{ID: "session:codex:s2", Kind: waits.KindYourTurn}}, time.Date(2026, 11, 17, 12, 7, 0, 0, time.UTC), "nl"},
		{MoodWorking, nil, time.Date(2026, 11, 18, 3, 33, 0, 0, time.UTC), "en"},
	}
	for _, c := range cases {
		st := lineState(c.mood, c.rows...)
		top := ""
		if len(c.rows) > 0 {
			top = c.rows[0].ID
		}
		var cands []LineDef
		for _, l := range Lines() {
			if l.Mood == c.mood {
				cands = append(cands, l)
			}
		}
		want := cands[pick(top, c.mood, c.at, len(cands))]
		s := DefaultSettings()
		s.Language = c.lang
		got := Line(st, s, c.at)
		exp := want.EN
		if c.lang == "nl" {
			exp = want.NL
		}
		exp = strings.NewReplacer("{who}", whoFor(top, c.lang), "{count}", "2").Replace(exp)
		if got != exp {
			t.Errorf("%s at %s: got %q want %q", c.mood, c.at.Format(time.RFC3339), got, exp)
		}
	}
}

func TestLineChangesEveryFiveMinutes(t *testing.T) {
	st := lineState(MoodAsleep)
	a := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	same := Line(st, DefaultSettings(), a.Add(4*time.Minute+59*time.Second))
	if Line(st, DefaultSettings(), a) != same {
		t.Error("the line changed inside one 5 minute slot")
	}
	if Line(st, DefaultSettings(), a.Add(5*time.Minute)) == same {
		t.Error("the line did not change at the next slot")
	}
}

func TestLineSkipsUnfillablePlaceholder(t *testing.T) {
	// no rows: {who} cannot be filled, so no blocked line with {who} may come out
	st := lineState(MoodBlocked)
	st.Counts.Blocked = 3
	for i := 0; i < 40; i++ {
		got := Line(st, DefaultSettings(), t0.Add(time.Duration(i)*5*time.Minute))
		if strings.Contains(got, "{") || strings.Contains(got, "waits on") || strings.Contains(got, "is stuck") {
			t.Errorf("unfillable line used: %q", got)
		}
		if got == "" {
			t.Error("a fillable line exists and none was picked")
		}
	}
	// no sweating rate: the {rate} line is skipped
	sw := lineState(MoodSweating)
	sw.BurnRate = 0
	for i := 0; i < 40; i++ {
		if got := Line(sw, DefaultSettings(), t0.Add(time.Duration(i)*5*time.Minute)); strings.Contains(got, "Bold") || strings.Contains(got, "Gedurfd") {
			t.Errorf("rate line used with no rate: %q", got)
		}
	}
}

func TestGateLineOnlyForGateTopRow(t *testing.T) {
	gate := lineState(MoodBlocked, Row{ID: "gate:f:g1", Kind: waits.KindGate})
	for i := 0; i < 40; i++ {
		if got := Line(gate, DefaultSettings(), t0.Add(time.Duration(i)*5*time.Minute)); got != "A gate waits. Gates do not open themselves." {
			t.Errorf("gate top row must get the gate line only, got %q", got)
		}
	}
	q := lineState(MoodBlocked, Row{ID: "session:claude:s1", Kind: waits.KindQuestion})
	for i := 0; i < 40; i++ {
		if got := Line(q, DefaultSettings(), t0.Add(time.Duration(i)*5*time.Minute)); strings.Contains(got, "gate") {
			t.Errorf("gate line used for a question row: %q", got)
		}
	}
}

func TestLinesOffGivesNoLineAndNlWorks(t *testing.T) {
	s := DefaultSettings()
	s.Lines = false
	for _, m := range allMoods {
		if got := Line(lineState(m, Row{ID: "x"}), s, t0); got != "" {
			t.Errorf("lamp_lines false but %s has line %q", m, got)
		}
	}
	nl := DefaultSettings()
	nl.Language = "nl"
	if got := Line(State{Mood: MoodOff}, nl, t0); got != "BurnMon draait niet." {
		t.Errorf("nl off line: %q", got)
	}
	if got := Line(State{Mood: MoodOff}, DefaultSettings(), t0); got != "BurnMon is not running." {
		t.Errorf("en off line: %q", got)
	}
}

func TestLineWhoNamesTheVendor(t *testing.T) {
	st := lineState(MoodYourTurn, Row{ID: "session:codex:s2", Kind: waits.KindYourTurn})
	seen := false
	for i := 0; i < 40; i++ {
		if strings.Contains(Line(st, DefaultSettings(), t0.Add(time.Duration(i)*5*time.Minute)), "Codex") {
			seen = true
		}
	}
	if !seen {
		t.Error("the vendor name never reached a line")
	}
	if whoFor("gate:x", "nl") != "Een agent" || whoFor("gate:x", "en") != "An agent" {
		t.Error("who for a non-session item")
	}
}

func TestComputeFillsLine(t *testing.T) {
	st := compute(snap(nil, nil, 0), DefaultSettings(), nil)
	if st.Line == "" {
		t.Error("Compute left the line empty")
	}
	off := DefaultSettings()
	off.Lines = false
	if compute(snap(nil, nil, 0), off, nil).Line != "" {
		t.Error("lamp_lines false must give no line")
	}
	if got := Compute(Input{Now: t0, Settings: DefaultSettings()}).Line; got != "BurnMon is not running." {
		t.Errorf("no data line: %q", got)
	}
}

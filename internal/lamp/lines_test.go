package lamp

import (
	"hash/fnv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"burnmon/internal/waits"
)

// emDash is built from its code point, never typed (house rule).
var emDash = string(rune(0x2014))

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
			if strings.Contains(s, emDash) {
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

func TestSingularRule(t *testing.T) {
	for i := 0; i < 60; i++ {
		at := t0.Add(time.Duration(i) * 5 * time.Minute)
		for _, lang := range []string{"en", "nl"} {
			s := DefaultSettings()
			s.Language = lang
			w1 := lineState(MoodWorking)
			w1.Agents = 1
			b1 := lineState(MoodBlocked, Row{ID: "session:claude:s1", Kind: waits.KindQuestion})
			b1.Counts.Blocked = 1
			for _, st := range []State{w1, b1} {
				got := Line(st, s, at)
				if strings.Contains(got, "1 agents") || strings.Contains(got, "1 things") || strings.Contains(got, "1 dingen") || strings.Contains(got, "{") {
					t.Errorf("%s count 1 gave a plural or open line: %q", lang, got)
				}
			}
		}
	}
	// Count 2 or more never gets a singular line.
	w2 := lineState(MoodWorking)
	for i := 0; i < 60; i++ {
		got := Line(w2, DefaultSettings(), t0.Add(time.Duration(i)*5*time.Minute))
		if strings.HasPrefix(got, "1 agent") {
			t.Errorf("count 2 gave the singular line %q", got)
		}
	}
	// The singular lines are reachable at count 1, in both languages. The working one
	// ("1 agent at work") is an echo line of the headline now and is never picked.
	seen := map[string]bool{}
	w1 := lineState(MoodWorking)
	w1.Agents = 1
	b1 := lineState(MoodBlocked, Row{ID: "session:claude:s1", Kind: waits.KindQuestion})
	b1.Counts.Blocked = 1
	for i := 0; i < 200; i++ {
		at := t0.Add(time.Duration(i) * 5 * time.Minute)
		for _, lang := range []string{"en", "nl"} {
			s := DefaultSettings()
			s.Language = lang
			seen[Line(w1, s, at)] = true
			seen[Line(b1, s, at)] = true
		}
	}
	for _, want := range []string{
		"One thing is blocked. Guess whose turn.", "Een ding zit vast. Raad eens wie er aan zet is.",
	} {
		if !seen[want] {
			t.Errorf("singular line %q never used", want)
		}
	}
}

func TestWallLineOnlyUpToThirtyMinutes(t *testing.T) {
	for _, c := range []struct {
		minutes int
		allowed bool
	}{{10, true}, {30, true}, {31, false}, {90, false}} {
		st := lineState(MoodBored, Row{ID: "session:claude:s1", Kind: waits.KindQuestion})
		st.LongestWaitMin = c.minutes
		seen := false
		for i := 0; i < 200; i++ {
			got := Line(st, DefaultSettings(), t0.Add(time.Duration(i)*5*time.Minute))
			if strings.Contains(got, "The wall has") {
				seen = true
			}
		}
		if seen != c.allowed {
			t.Errorf("%d minutes: wall line used %v, want %v", c.minutes, seen, c.allowed)
		}
	}
}

func TestActLinesAreInBothLanguagesAndShort(t *testing.T) {
	type call struct {
		act, part string
		count     int
		hour      int
		en, nl    string
	}
	for _, c := range []call{
		{"slip", "start", 0, 0, "I am not bored. I am testing the rail.", "Ik verveel me niet. Ik test de rail."},
		{"slip", "air", 0, 0, "Whoa.", "Ho."},
		{"slip", "back", 0, 0, "You saw nothing.", "Je hebt niets gezien."},
		{"stamp", "a", 0, 0, "Stamped. Back to work.", "Gestempeld. Weer aan het werk."},
		{"stamp", "b", 0, 0, "Answered. That was all it took.", "Beantwoord. Meer was het niet."},
		{"wake", "", 0, 0, "Somebody started something.", "Iemand is iets begonnen."},
		{"welcome", "", 2, 0, "Welcome back. 2 things waited.", "Welkom terug. Er wachtten 2 dingen."},
		{"welcome", "", 7, 0, "Welcome back. 7 things waited.", "Welkom terug. Er wachtten 7 dingen."},
		{"welcome", "", 1, 0, "Welcome back. One thing waited.", "Welkom terug. Er wachtte een ding."},
		{"coffee", "", 0, 10, "Ten o'clock. Coffee.", "Tien uur. Koffie."},
		{"coffee", "", 0, 15, "Three o'clock. Coffee. The agents get none.", "Drie uur. Koffie. De agents krijgen niets."},
	} {
		for lang, want := range map[string]string{"en": c.en, "nl": c.nl} {
			got := ActLine(c.act, c.part, lang, c.count, c.hour)
			if got != want {
				t.Errorf("ActLine(%s,%s,%s,%d,%d) = %q, want %q", c.act, c.part, lang, c.count, c.hour, got, want)
			}
			if n := utf8.RuneCountInString(got); n > 60 {
				t.Errorf("%q has %d runes, more than 60", got, n)
			}
			if strings.Contains(got, emDash) || strings.Contains(got, "!") {
				t.Errorf("%q has an em dash or an exclamation mark", got)
			}
		}
	}
	// No line for what has none.
	for _, c := range []call{{"coffee", "", 0, 12, "", ""}, {"welcome", "", 0, 0, "", ""}, {"alarm", "", 0, 0, "", ""}, {"slip", "side", 0, 0, "", ""}} {
		if got := ActLine(c.act, c.part, "en", c.count, c.hour); got != "" {
			t.Errorf("ActLine(%s,%s,en,%d,%d) = %q, want empty", c.act, c.part, c.count, c.hour, got)
		}
	}
}

// An echo line says what the headline already says: its text starts with
// {count}, {rate} or "1 agent". It is never picked; a mood with only echo lines
// left gives no line.
func TestNoEchoLineIsPicked(t *testing.T) {
	isEcho := func(s string) bool {
		return strings.HasPrefix(s, "{count}") || strings.HasPrefix(s, "{rate}") || strings.HasPrefix(s, "1 agent")
	}
	echoOut := func(got string) bool {
		// a picked line has its placeholders filled, so test the shape of the filled text too
		for _, p := range []string{" agents at work", " agents aan het werk", " things are blocked", " dingen zitten vast", " tokens a minute", " tokens per minuut", "1 agent "} {
			if strings.Contains(got, p) {
				return true
			}
		}
		return false
	}
	for _, mood := range allMoods {
		for _, lang := range []string{"en", "nl"} {
			for _, n := range []int{1, 2, 5} {
				st := lineState(mood, Row{ID: "session:claude:s1", Kind: waits.KindQuestion})
				st.Agents, st.Counts.Blocked, st.BurnRate, st.LongestWaitMin = n, n, 12345, 20
				s := DefaultSettings()
				s.Language = lang
				for i := 0; i < 60; i++ {
					got := Line(st, s, t0.Add(time.Duration(i)*5*time.Minute))
					if isEcho(got) || echoOut(got) {
						t.Fatalf("%s %s count %d: picked the echo line %q", mood, lang, n, got)
					}
				}
			}
		}
	}
	// Mood working with one agent had its two lines left minus the echo ones: the
	// other three still show. A mood whose lines are all echo gives none: test the rule on a table.
	only := []LineDef{{MoodSweating, "{rate} tokens a minute. Bold.", "{rate} tokens per minuut. Gedurfd."}}
	if got := pickFrom(only, lineState(MoodSweating), DefaultSettings(), t0); got != "" {
		t.Errorf("a mood with only echo lines gave %q, want none", got)
	}
	for _, l := range lineTable {
		if l.Mood == MoodSweating && !isEcho(l.EN) {
			return
		}
	}
	t.Error("mood sweating has no line left that is not an echo line")
}

// The two carry lines and the three swing lines exist in both languages, at most 60 runes.
func TestCarryAndSwingLines(t *testing.T) {
	for _, c := range []struct{ act, part, en, nl string }{
		{"carry", "drag", "Put me down.", "Zet me neer."},
		{"carry", "view", "Nice view.", "Mooi uitzicht."},
		{"swing", "tickle", "That tickles.", "Dat kietelt."},
		{"swing", "protest1", "Don't do that.", "Niet doen."},
		{"swing", "protest2", "I said don't.", "Ik zei: niet doen."},
	} {
		for lang, want := range map[string]string{"en": c.en, "nl": c.nl} {
			got := ActLine(c.act, c.part, lang, 0, 0)
			if got != want {
				t.Errorf("ActLine(%s,%s,%s) = %q, want %q", c.act, c.part, lang, got, want)
			}
			if utf8.RuneCountInString(got) > 60 || strings.Contains(got, emDash) {
				t.Errorf("%q is over 60 runes or holds an em dash", got)
			}
		}
	}
}

// Too hard is the pull to the 60 degree stop, or 3 direction changes in 1.5 s. A
// second too hard under 60 s after the last gets the second line.
func TestProtest(t *testing.T) {
	now := t0
	for _, c := range []struct {
		deg     float64
		changes int
		want    bool
	}{{59, 2, false}, {60, 2, true}, {59, 3, true}, {0, 2, false}, {10, 3, true}, {61, 0, true}} {
		if got := ProtestTooHard(c.deg, c.changes); got != c.want {
			t.Errorf("ProtestTooHard(%v,%d) = %v, want %v", c.deg, c.changes, got, c.want)
		}
	}
	var none time.Time
	if got := Protest(59, 2, now, none, true); got != "tickle" {
		t.Errorf("gentle swing gave %q, want tickle", got)
	}
	if got := Protest(59, 2, now, none, false); got != "" {
		t.Errorf("no swing gave %q, want none", got)
	}
	if got := Protest(60, 0, now, none, true); got != "protest1" {
		t.Errorf("first too hard gave %q, want protest1", got)
	}
	if got := Protest(10, 3, now, now.Add(-59*time.Second), true); got != "protest2" {
		t.Errorf("too hard 59 s after the last gave %q, want protest2", got)
	}
	if got := Protest(10, 3, now, now.Add(-61*time.Second), true); got != "protest1" {
		t.Errorf("too hard 61 s after the last gave %q, want protest1", got)
	}
	if got := Protest(10, 2, now, now.Add(-5*time.Second), true); got != "tickle" {
		t.Errorf("a gentle swing right after a protest gave %q, want tickle", got)
	}
}

// Glim is awake for the coffee: the pose of the act coffee is working; every other act keeps the mood.
func TestActMood(t *testing.T) {
	for _, c := range []struct{ act, mood, want string }{
		{"coffee", MoodAsleep, MoodWorking}, {"coffee", MoodOff, MoodWorking}, {"coffee", MoodBored, MoodWorking},
		{"", MoodAsleep, MoodAsleep}, {"alarm", MoodBlocked, MoodBlocked}, {"peek", MoodAsleep, MoodAsleep},
	} {
		if got := ActMood(c.act, c.mood); got != c.want {
			t.Errorf("ActMood(%q,%q) = %q, want %q", c.act, c.mood, got, c.want)
		}
	}
}

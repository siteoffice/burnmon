package lamp

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burnmon/internal/snapshot"
	"burnmon/internal/waits"
)

var update = flag.Bool("update", false, "rewrite testdata/state_full.golden.json")

var t0 = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

// snap builds a fresh snapshot written at t0 with a 5 second tick. Counts
// come from the one count function, as the snapshot writer does.
func snap(items []waits.Item, sessions []snapshot.Session, tpm float64) *snapshot.Snapshot {
	return &snapshot.Snapshot{
		SchemaVersion: 1,
		WrittenAt:     t0.Format(time.RFC3339),
		TickSeconds:   5,
		Sessions:      sessions,
		Burn:          snapshot.Burn{TokensPerMinute: tpm, Cost: snapshot.Cost{USD: 1.25, EUR: 1.15, Label: "API price equivalent"}},
		WaitItems:     items,
		WaitCounts:    waits.CountOf(items),
	}
}

func item(kind, group, id string, age int64) waits.Item {
	return waits.Item{Kind: kind, Group: group, ID: id, Title: "title " + id, AgeSeconds: age}
}

func blocked(id string, age int64) waits.Item {
	return item(waits.KindQuestion, waits.GroupBlocked, id, age)
}

func turn(id string, age int64) waits.Item {
	return item(waits.KindYourTurn, waits.GroupYourTurn, id, age)
}

func done(id string, age int64) waits.Item {
	return item(waits.KindFinished, waits.GroupDone, id, age)
}

func session(id, state, parent string) snapshot.Session {
	return snapshot.Session{Vendor: "claude", ID: id, ParentID: parent, State: state}
}

func samples(n int, v float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func compute(s *snapshot.Snapshot, st Settings, burn []float64) State {
	return Compute(Input{Snap: s, Now: t0, Settings: st, BurnSamples: burn})
}

func TestMoodOrder(t *testing.T) {
	working := []snapshot.Session{session("a", "coding", "")}
	cases := []struct {
		name string
		snap *snapshot.Snapshot
		burn []float64
		want string
	}{
		{"no data", nil, nil, MoodOff},
		{"bored beats blocked", snap([]waits.Item{blocked("b", 600), done("d", 10)}, working, 0), nil, MoodBored},
		{"blocked", snap([]waits.Item{blocked("b", 599), turn("t", 5)}, working, 0), nil, MoodBlocked},
		{"blocked beats your turn and done", snap([]waits.Item{turn("t", 5), blocked("b", 5), done("d", 5)}, nil, 0), nil, MoodBlocked},
		{"your turn", snap([]waits.Item{turn("t", 5), done("d", 5)}, working, 0), nil, MoodYourTurn},
		{"your turn beats sweating", snap([]waits.Item{turn("t", 5)}, working, 90000), samples(120, 30000), MoodYourTurn},
		{"sweating beats done", snap([]waits.Item{done("d", 5)}, working, 90000), samples(120, 30000), MoodSweating},
		{"done", snap([]waits.Item{done("d", 599)}, working, 0), nil, MoodDone},
		{"done too old", snap([]waits.Item{done("d", 600)}, working, 0), nil, MoodWorking},
		{"working", snap(nil, working, 0), nil, MoodWorking},
		{"asleep", snap(nil, nil, 0), nil, MoodAsleep},
	}
	for _, c := range cases {
		if got := compute(c.snap, DefaultSettings(), c.burn).Mood; got != c.want {
			t.Errorf("%s: mood %q, want %q", c.name, got, c.want)
		}
	}
}

func TestBoredEdgeMinutes(t *testing.T) {
	def := DefaultSettings()
	if got := compute(snap([]waits.Item{turn("t", 599)}, nil, 0), def, nil); got.Mood != MoodYourTurn || got.LongestWaitMin != 9 {
		t.Errorf("599 s: mood %q longest %d, want your_turn 9", got.Mood, got.LongestWaitMin)
	}
	if got := compute(snap([]waits.Item{turn("t", 600)}, nil, 0), def, nil); got.Mood != MoodBored || got.LongestWaitMin != 10 {
		t.Errorf("600 s: mood %q longest %d, want bored 10", got.Mood, got.LongestWaitMin)
	}
	if got := compute(snap([]waits.Item{turn("t", 659)}, nil, 0), def, nil); got.LongestWaitMin != 10 {
		t.Errorf("659 s: longest %d, want 10 (whole minutes, rounded down)", got.LongestWaitMin)
	}
}

func TestWallEdges(t *testing.T) {
	def := DefaultSettings()
	for _, c := range []struct {
		min    int64
		bricks int
		over   bool
		bored  bool
	}{{9, 0, false, false}, {10, 10, false, true}, {29, 29, false, true}, {30, 30, true, true}, {31, 30, true, true}, {120, 30, true, true}} {
		got := compute(snap([]waits.Item{blocked("b", c.min*60)}, nil, 0), def, nil)
		if got.WallBricks != c.bricks || got.WallOver != c.over {
			t.Errorf("%d min: bricks %d over %v, want %d %v", c.min, got.WallBricks, got.WallOver, c.bricks, c.over)
		}
		if (got.Mood == MoodBored) != c.bored {
			t.Errorf("%d min: mood %q", c.min, got.Mood)
		}
	}
}

func TestFreshEdge(t *testing.T) {
	s := snap(nil, []snapshot.Session{session("a", "coding", "")}, 0)
	// tick 5 s: fresh at exactly 15 s, no data at 16 s.
	if got := Compute(Input{Snap: s, Now: t0.Add(15 * time.Second), Settings: DefaultSettings()}); got.Mood != MoodWorking {
		t.Errorf("15 s old: mood %q, want working", got.Mood)
	}
	if got := Compute(Input{Snap: s, Now: t0.Add(16 * time.Second), Settings: DefaultSettings()}); got.Mood != MoodOff {
		t.Errorf("16 s old: mood %q, want off", got.Mood)
	}
}

func TestSweatingEdges(t *testing.T) {
	working := []snapshot.Session{session("a", "coding", "")}
	const usual = 10000.0
	cases := []struct {
		name  string
		tpm   float64
		n     int
		floor int
		want  string
	}{
		{"119 samples", 90000, 119, 20000, MoodWorking},
		{"120 samples", 90000, 120, 20000, MoodSweating},
		{"2.9 times", 29000, 120, 20000, MoodWorking},
		{"3 times", 30000, 120, 20000, MoodSweating},
		{"below the floor", 30000, 120, 30001, MoodWorking},
		{"at the floor", 30000, 120, 30000, MoodSweating},
	}
	for _, c := range cases {
		st := DefaultSettings()
		st.BurnFloorTPM = c.floor
		if got := compute(snap(nil, working, c.tpm), st, samples(c.n, usual)); got.Mood != c.want {
			t.Errorf("%s: mood %q, want %q", c.name, got.Mood, c.want)
		}
	}
	// The usual burn is a median: one wild sample does not move it.
	mixed := append(samples(120, usual), 1e9)
	if got := compute(snap(nil, working, 29000), DefaultSettings(), mixed); got.Mood != MoodWorking {
		t.Errorf("median: mood %q, want working", got.Mood)
	}
}

func TestMedian(t *testing.T) {
	if m := median([]float64{5, 1, 3}); m != 3 {
		t.Errorf("odd median %v", m)
	}
	if m := median([]float64{4, 1, 3, 2}); m != 2.5 {
		t.Errorf("even median %v", m)
	}
}

func TestAgentsAtWork(t *testing.T) {
	ss := []snapshot.Session{
		session("a", "coding", ""), session("b", "thinking", ""),
		session("c", "waiting", ""), session("d", "dormant", ""),
		session("e", "coding", "a"), // a subagent, not counted
	}
	got := compute(snap(nil, ss, 0), DefaultSettings(), nil)
	if got.Agents != 2 || got.Headline != "2 agents at work" {
		t.Errorf("agents %d headline %q", got.Agents, got.Headline)
	}
	if one := compute(snap(nil, ss[:1], 0), DefaultSettings(), nil); one.Headline != "1 agent at work" {
		t.Errorf("headline %q", one.Headline)
	}
}

func TestHeadlines(t *testing.T) {
	working := []snapshot.Session{session("a", "coding", "")}
	cases := []struct {
		name string
		snap *snapshot.Snapshot
		burn []float64
		want string
	}{
		{"off", nil, nil, "BurnMon is not running"},
		{"asleep", snap(nil, nil, 0), nil, "No agents running"},
		{"blocked", snap([]waits.Item{blocked("a", 5), blocked("b", 5)}, nil, 0), nil, "2 blocked"},
		{"blocked and turn", snap([]waits.Item{blocked("a", 5), turn("b", 5), turn("c", 5)}, nil, 0), nil, "1 blocked, 2 your turn"},
		{"your turn", snap([]waits.Item{turn("b", 5)}, nil, 0), nil, "1 your turn"},
		{"bored", snap([]waits.Item{blocked("a", 700), turn("b", 5)}, nil, 0), nil, "1 blocked, 1 your turn"},
		{"bored without blocked", snap([]waits.Item{turn("b", 700)}, nil, 0), nil, "1 your turn"},
		{"done", snap([]waits.Item{done("a", 5), done("b", 5), done("c", 5)}, nil, 0), nil, "3 ready to close"},
		{"sweating rate in thousands", snap(nil, working, 61234), samples(120, 1000), "61k tokens a minute"},
		{"sweating rate below 1000", snap(nil, working, 999.4), samples(120, 100), "999 tokens a minute"},
		{"working", snap(nil, working, 0), nil, "1 agent at work"},
	}
	st := DefaultSettings()
	st.BurnFloorTPM = 0
	for _, c := range cases {
		if got := compute(c.snap, st, c.burn).Headline; got != c.want {
			t.Errorf("%s: headline %q, want %q", c.name, got, c.want)
		}
	}
}

func TestForemanItemsOff(t *testing.T) {
	items := []waits.Item{
		item(waits.KindGate, waits.GroupBlocked, "gate:f:g", 900),
		item(waits.KindSprint, waits.GroupBlocked, "sprint:s", 900),
		item(waits.KindFailedCheck, waits.GroupBlocked, "check:c", 900),
		turn("t", 5),
	}
	on := compute(snap(items, nil, 0), DefaultSettings(), nil)
	// Foreman items count and show, but they are not live items: they cannot bore.
	if on.Mood != MoodBlocked || on.Counts.Blocked != 3 || len(on.Rows) != 4 {
		t.Fatalf("on: %+v", on)
	}
	st := DefaultSettings()
	st.ForemanItems = false
	off := compute(snap(items, nil, 0), st, nil)
	if off.Mood != MoodYourTurn || off.Counts.Blocked != 0 || off.Counts.YourTurn != 1 || len(off.Rows) != 1 || off.Headline != "1 your turn" {
		t.Fatalf("off: %+v", off)
	}
}

func TestUnknownKindCountsByGroupButIsNotLive(t *testing.T) {
	it := item("mystery", waits.GroupBlocked, "x", 3000)
	got := compute(snap([]waits.Item{it}, nil, 0), DefaultSettings(), nil)
	if got.Mood != MoodBlocked {
		t.Errorf("mood %q, want blocked (the group counts, the kind is not live so it cannot bore)", got.Mood)
	}
	if got.Counts.Blocked != 1 || len(got.Rows) != 1 || got.LongestWaitMin != 0 {
		t.Fatalf("counts %+v rows %d longest %d", got.Counts, len(got.Rows), got.LongestWaitMin)
	}
	if got.Rows[0].Word != "Blocked" {
		t.Errorf("word %q", got.Rows[0].Word)
	}
}

func TestRowOrderAndCap(t *testing.T) {
	items := []waits.Item{turn("t-old", 400), turn("t-new", 10), blocked("b-b", 50), blocked("b-a", 50), blocked("b-big", 200)}
	got := compute(snap(items, nil, 0), DefaultSettings(), nil)
	var ids []string
	for _, r := range got.Rows {
		ids = append(ids, r.ID)
	}
	if want := "b-big b-a b-b t-old t-new"; strings.Join(ids, " ") != want {
		t.Fatalf("order %v, want %s", ids, want)
	}
	// Seven counted items: five rows and "and 2 more".
	seven := append(append([]waits.Item{}, items...), turn("t-x", 1), turn("t-y", 2))
	got = compute(snap(seven, nil, 0), DefaultSettings(), nil)
	if len(got.Rows) != 6 {
		t.Fatalf("rows %d, want 6", len(got.Rows))
	}
	if last := got.Rows[5]; last.Title != "and 2 more" || last.Kind != "more" || last.ID != "" || last.Wait != "" {
		t.Errorf("last row %+v", last)
	}
	// Exactly six: all six shown, no "more" row.
	got = compute(snap(seven[:6], nil, 0), DefaultSettings(), nil)
	if len(got.Rows) != 6 || got.Rows[5].Kind == "more" {
		t.Errorf("six items: rows %d last %+v", len(got.Rows), got.Rows[5])
	}
}

func TestRowTitleAndWord(t *testing.T) {
	long := strings.Repeat("x", 29)
	exact := strings.Repeat("y", 28)
	items := []waits.Item{
		{Kind: waits.KindQuestion, Group: waits.GroupBlocked, ID: "q1", Title: long, AgeSeconds: 3},
		{Kind: waits.KindQuestion, Group: waits.GroupBlocked, ID: "q2", Title: exact, AgeSeconds: 2},
		{Kind: waits.KindQuestion, Group: waits.GroupBlocked, ID: "q3", Title: "a\nb\tc", AgeSeconds: 1},
	}
	got := compute(snap(items, nil, 0), DefaultSettings(), nil)
	byID := map[string]Row{}
	for _, r := range got.Rows {
		byID[r.ID] = r
	}
	if r := byID["q1"]; len([]rune(r.Title)) != 28 || !strings.HasSuffix(r.Title, "...") || r.TitleFull != long {
		t.Errorf("29 runes: title %q full %q", r.Title, r.TitleFull)
	}
	if r := byID["q2"]; r.Title != exact || r.TitleFull != exact {
		t.Errorf("28 runes: title %q", r.Title)
	}
	if r := byID["q3"]; r.Title != "a b c" {
		t.Errorf("line breaks: title %q", r.Title)
	}
	if r := byID["q1"]; r.Word != "Question" {
		t.Errorf("word %q", r.Word)
	}
}

func TestRowWaitFormats(t *testing.T) {
	for _, c := range []struct {
		age  int64
		want string
	}{{59, "59 s"}, {60, "1 min"}, {59 * 60, "59 min"}, {60 * 60, "1 h"}, {23 * 3600, "23 h"}, {24 * 3600, "1 d"}} {
		got := compute(snap([]waits.Item{blocked("b", c.age)}, nil, 0), DefaultSettings(), nil)
		if got.Rows[0].Wait != c.want {
			t.Errorf("age %d: wait %q, want %q", c.age, got.Rows[0].Wait, c.want)
		}
	}
	got := compute(snap([]waits.Item{blocked("b", 3852)}, nil, 0), DefaultSettings(), nil)
	if got.Rows[0].WaitExact != "1 h 4 min 12 s" {
		t.Errorf("exact %q", got.Rows[0].WaitExact)
	}
}

func TestStateNeverCarriesDetailOrPaths(t *testing.T) {
	const m = "MARKER-7731"
	it := waits.Item{
		Kind: waits.KindGate, Group: waits.GroupBlocked, ID: "gate:f:g", Title: "ok",
		Project: m + "-project", Detail: m + "-detail", SpecPath: m + "-spec", ResultPath: m + "-result",
		ReportPath: m + "-report", CloseReason: m + "-reason", SprintID: m + "-sprint",
		Parked: []waits.Parked{{N: 1, Title: m + "-parked", Reason: m + "-why"}},
	}
	s := snap([]waits.Item{it}, nil, 0)
	s.Sessions = []snapshot.Session{{Vendor: "claude", ID: "s", State: "coding", Project: m + "-sessproject"}}
	b, err := json.Marshal(compute(s, DefaultSettings(), nil))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), m) {
		t.Fatalf("state holds a marker: %s", b)
	}
}

func TestCostAndRate(t *testing.T) {
	got := compute(snap(nil, nil, 1234.5), DefaultSettings(), nil)
	if got.BurnRate != 1234.5 || got.Cost5.Label != "API price equivalent" || got.Cost5.USD != 1.25 || got.Cost5.EUR != 1.15 {
		t.Errorf("burn %v cost %+v", got.BurnRate, got.Cost5)
	}
	if off := compute(nil, DefaultSettings(), nil); off.BurnRate != 0 || off.Cost5.Label != "API price equivalent" {
		t.Errorf("off: %+v", off)
	}
}

func TestDemoFlag(t *testing.T) {
	got := Compute(Input{Snap: snap(nil, nil, 0), Now: t0, Settings: DefaultSettings(), Demo: true})
	if !got.Demo {
		t.Error("demo not carried")
	}
	if compute(snap(nil, nil, 0), DefaultSettings(), nil).Demo {
		t.Error("demo set without the flag")
	}
}

func TestLoadReader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if Load(path, t0) != nil {
		t.Error("missing file must be no data")
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if Load(path, t0) != nil {
		t.Error("unparsable file must be no data")
	}
	b, _ := json.Marshal(snap(nil, nil, 0))
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if Load(path, t0) == nil {
		t.Error("fresh file must load")
	}
	if Load(path, t0.Add(16*time.Second)) != nil {
		t.Error("stale file must be no data")
	}
}

func TestGoldenFullState(t *testing.T) {
	items := []waits.Item{
		{Kind: waits.KindGate, Group: waits.GroupBlocked, ID: "gate:flow-1:g2", Title: "Approve the plan", AgeSeconds: 1900, Detail: "hidden", SpecPath: "hidden"},
		{Kind: waits.KindQuestion, Group: waits.GroupBlocked, ID: "session:claude:s1", Title: "Which table?", AgeSeconds: 700},
		turn("session:codex:s2", 30),
		done("close:1", 100),
	}
	ss := []snapshot.Session{session("a", "coding", ""), session("b", "waiting", "")}
	got := compute(snap(items, ss, 4200), DefaultSettings(), nil)
	b, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, '\n')
	const path = "testdata/state_full.golden.json"
	if *update {
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/lamp -update once)", err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != string(b) {
		t.Errorf("state differs from golden.\ngot:\n%s", b)
	}
}

func TestTickerRowsFormula(t *testing.T) {
	working := []snapshot.Session{session("a", "coding", "")}
	mk := func(n int) []waits.Item {
		var it []waits.Item
		for i := 0; i < n; i++ {
			it = append(it, blocked("b"+string(rune('a'+i)), int64(10+i)))
		}
		return it
	}
	for _, c := range []struct{ rows, items, want int }{
		{1, 0, 1}, {1, 1, 1}, {1, 5, 1},
		{4, 0, 1}, {4, 1, 2}, {4, 5, 4},
	} {
		s := DefaultSettings()
		s.TickerRows = c.rows
		got := compute(snap(mk(c.items), working, 0), s, nil)
		if got.TickerRows != c.want || got.Counted != c.items {
			t.Errorf("rows %d items %d: ticker rows %d counted %d, want %d and %d", c.rows, c.items, got.TickerRows, got.Counted, c.want, c.items)
		}
		if len(got.CountedIDs) != c.items {
			t.Errorf("rows %d items %d: %d counted ids", c.rows, c.items, len(got.CountedIDs))
		}
	}
	// A quiet lamp and an off lamp show one row, whatever the setting.
	s := DefaultSettings()
	s.TickerRows = 4
	if got := compute(snap(nil, working, 0), s, nil); got.TickerRows != 1 || got.Counted != 0 {
		t.Errorf("quiet: ticker rows %d counted %d", got.TickerRows, got.Counted)
	}
	if got := compute(nil, s, nil); got.TickerRows != 1 {
		t.Errorf("off: ticker rows %d", got.TickerRows)
	}
	// A your_turn item counts too; a done item does not.
	if got := compute(snap([]waits.Item{turn("t", 5), done("d", 5)}, working, 0), s, nil); got.Counted != 1 || got.TickerRows != 2 {
		t.Errorf("mixed: counted %d ticker rows %d", got.Counted, got.TickerRows)
	}
}

func TestCountedIDsAreNotSentToThePage(t *testing.T) {
	st := compute(snap([]waits.Item{blocked("MARKER-ID-1", 5)}, nil, 0), DefaultSettings(), nil)
	if len(st.CountedIDs) != 1 || st.CountedIDs[0] != "MARKER-ID-1" {
		t.Fatalf("counted ids %v", st.CountedIDs)
	}
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "counted_ids") || strings.Contains(string(b), "CountedIDs") {
		t.Errorf("counted ids reached the JSON: %s", b)
	}
	if !strings.Contains(string(b), `"ticker_rows"`) || !strings.Contains(string(b), `"counted"`) {
		t.Errorf("ticker_rows or counted missing from the JSON: %s", b)
	}
}

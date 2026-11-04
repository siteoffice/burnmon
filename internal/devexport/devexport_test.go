package devexport

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"burnmon/internal/advisor"
	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/sysmon"
)

func testConfig() *pricing.Config {
	cfg := pricing.Defaults()
	cfg.ContextWindows = map[string]int64{"claude-sonnet-5": 200_000}
	return &cfg
}

func ptr(v int64) *int64 { return &v }

// secretPromptText and secretResponseText stand in for real prompt/response
// content: a fixture carrying them proves the "never export prompt or
// response text" rule holds, since neither is ever read by Assemble or any
// Build* function (only schema.Event.Title carries prompt-derived text in
// this codebase, and this package never touches that field).
const (
	secretPromptText   = "PROMPT-SECRET-do-not-export-this-user-message"
	secretResponseText = "RESPONSE-SECRET-do-not-export-this-model-reply"
)

func testEvent(sessionID, model string, at time.Time, fresh int64, cw, cr *int64, out int64) schema.Event {
	return schema.Event{
		Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
		SessionID: sessionID, RequestID: sessionID + "-r", Model: model,
		At: at, Input: fresh, CacheWrite: cw, CacheRead: cr, Output: out,
		Project: "C:\\clients\\acme\\repo", Owner: "acme-owner", Client: "acme-client",
		// Title is exactly the kind of prompt-derived text the export must
		// never carry: set here so a regression that starts reading it
		// anywhere in this package would be caught by
		// TestAssemble_NeverExportsPromptOrResponseText below.
		Title: secretPromptText + " " + secretResponseText,
	}
}

func TestAssemble_FiltersToWindowAndRealTurns(t *testing.T) {
	base := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	events := []schema.Event{
		testEvent("s1", "claude-sonnet-5", base.Add(-time.Hour), 1000, ptr(0), ptr(0), 100), // before window
		testEvent("s1", "claude-sonnet-5", base, 1000, ptr(0), ptr(0), 100),                 // in window
		testEvent("s1", "claude-sonnet-5", base.Add(time.Hour), 1000, ptr(0), ptr(0), 100),  // at/after until, excluded
		{Vendor: "anthropic", SessionID: "s1", At: base.Add(30 * time.Minute)},              // synthetic tool-only, not a turn
	}
	b := Assemble(events, testConfig(), nil, nil, sysmon.MachineProfile{}, base, base.Add(time.Hour), base, nil)
	if len(b.Turns) != 1 {
		t.Fatalf("expected 1 turn inside [since, until), got %d", len(b.Turns))
	}
	if !b.Turns[0].At.Equal(base) {
		t.Fatalf("expected the in-window turn, got %v", b.Turns[0].At)
	}
}

func TestAssemble_NeverExportsPromptOrResponseText(t *testing.T) {
	base := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	events := []schema.Event{
		testEvent("s1", "claude-sonnet-5", base, 1000, ptr(0), ptr(0), 100),
	}
	findings := []advisor.Finding{{RuleID: "context-window", At: base, Message: "fine", Suggestion: "fine"}}
	b := Assemble(events, testConfig(), nil, nil, sysmon.MachineProfile{}, base, base.Add(time.Hour), base, findings)

	summary := BuildSummaryMD(b)
	dataJSON, err := BuildDataJSON(b)
	if err != nil {
		t.Fatalf("BuildDataJSON: %v", err)
	}
	daily := BuildDailyCSV(b)

	for _, secret := range []string{secretPromptText, secretResponseText} {
		if strings.Contains(summary, secret) {
			t.Fatalf("summary.md contains %q", secret)
		}
		if strings.Contains(string(dataJSON), secret) {
			t.Fatalf("data.json contains %q", secret)
		}
		if strings.Contains(daily, secret) {
			t.Fatalf("daily.csv contains %q", secret)
		}
	}
}

func TestRedact_ReplacesProjectOwnerClient_Consistently(t *testing.T) {
	base := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	events := []schema.Event{
		testEvent("s1", "claude-sonnet-5", base, 1000, ptr(0), ptr(0), 100),
		testEvent("s1", "claude-sonnet-5", base.Add(time.Minute), 1000, ptr(0), ptr(0), 100),
	}
	b := Assemble(events, testConfig(), nil, nil, sysmon.MachineProfile{}, base, base.Add(time.Hour), base, nil)
	r := Redact(b, "test-salt-1")

	if !r.Redacted {
		t.Fatal("expected Redacted to be true")
	}
	if len(r.Turns) != 2 {
		t.Fatalf("expected 2 redacted turns, got %d", len(r.Turns))
	}
	if r.Turns[0].Project == b.Turns[0].Project {
		t.Fatal("expected Project to change under redaction")
	}
	if r.Turns[0].Project != r.Turns[1].Project {
		t.Fatal("expected the same source project to hash to the same value both times (stable hash)")
	}
	if strings.Contains(r.Turns[0].Project, "acme") {
		t.Fatalf("redacted project still contains the real name: %q", r.Turns[0].Project)
	}

	dataJSON, err := BuildDataJSON(r)
	if err != nil {
		t.Fatalf("BuildDataJSON: %v", err)
	}
	if strings.Contains(string(dataJSON), "acme") {
		t.Fatal("redacted data.json still contains the real owner/client/project name")
	}
}

// TestRedact_DifferentSaltsProduceDifferentHashes guards the review fix,
// 2026-09-24: Redact must actually use its salt argument, not silently
// ignore it (an unsalted, 8-hex-character truncated hash of a short,
// guessable name like a client folder is practically reversible by brute
// force).
func TestRedact_DifferentSaltsProduceDifferentHashes(t *testing.T) {
	base := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	events := []schema.Event{testEvent("s1", "claude-sonnet-5", base, 1000, ptr(0), ptr(0), 100)}
	b := Assemble(events, testConfig(), nil, nil, sysmon.MachineProfile{}, base, base.Add(time.Hour), base, nil)

	a := Redact(b, "salt-a")
	c := Redact(b, "salt-b")
	if a.Turns[0].Project == c.Turns[0].Project {
		t.Fatalf("expected different salts to produce different hashes, both gave %q", a.Turns[0].Project)
	}
}

func TestBuildDataJSON_RoundTripsSchema(t *testing.T) {
	base := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	events := []schema.Event{testEvent("s1", "claude-sonnet-5", base, 1000, ptr(0), ptr(500), 100)}
	sys := []sysmon.Sample{{Ts: base, CPUPct: 50, Cores: []float64{50}, MemUsedMB: 4000, MemTotalMB: 16000}}
	b := Assemble(events, testConfig(), sys, nil, sysmon.MachineProfile{CPUModel: "Test CPU"}, base, base.Add(time.Hour), base, nil)

	raw, err := BuildDataJSON(b)
	if err != nil {
		t.Fatalf("BuildDataJSON: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("data.json did not parse: %v", err)
	}
	if int(doc["schema"].(float64)) != Schema {
		t.Fatalf("expected schema %d, got %v", Schema, doc["schema"])
	}
	turns, ok := doc["turns"].([]any)
	if !ok || len(turns) != 1 {
		t.Fatalf("expected 1 turn in data.json, got %v", doc["turns"])
	}
}

func TestBuildDailyCSV_HasHeaderAndOneRowPerDay(t *testing.T) {
	base := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	events := []schema.Event{
		testEvent("s1", "claude-sonnet-5", base, 1000, ptr(0), ptr(0), 100),
		testEvent("s1", "claude-sonnet-5", base.Add(time.Minute), 1000, ptr(0), ptr(0), 100),
		testEvent("s1", "claude-sonnet-5", base.Add(25*time.Hour), 1000, ptr(0), ptr(0), 100),
	}
	b := Assemble(events, testConfig(), nil, nil, sysmon.MachineProfile{}, base, base.Add(48*time.Hour), base, nil)
	csvOut := BuildDailyCSV(b)
	lines := strings.Split(strings.TrimRight(csvOut, "\n"), "\n")
	if lines[0] != "date,harness,model,fresh,cache_write,cache_read,output,cost_usd" {
		t.Fatalf("unexpected header: %q", lines[0])
	}
	// Two distinct local days (base's day, and base+25h's day, both well
	// clear of a local-midnight boundary so this holds regardless of the
	// test machine's own UTC offset): one row each.
	if len(lines) != 3 {
		t.Fatalf("expected header + 2 rows, got %d lines: %v", len(lines), lines)
	}
}

// TestDailyRows_BucketsByLocalDayNotUTC guards a regression a fresh review
// caught (2026-09-26): dailyRows keyed by t.At.Format("2006-01-02") where
// t.At is deliberately kept UTC (a portable instant, matching WS3's own
// convention), but WS2 item 5 made --since/--until (and every other
// calendar-day boundary in this app) local - a turn just after local
// midnight, still before UTC midnight, used to bucket into the previous
// local day. Skips on a machine whose own time.Local is UTC (nothing to
// distinguish there), same reasoning other DST/local-time tests in this
// codebase use, but does not need Europe/Amsterdam specifically: any
// nonzero offset reproduces the bug this guards against.
func TestDailyRows_BucketsByLocalDayNotUTC(t *testing.T) {
	if _, offset := time.Now().Zone(); offset == 0 {
		t.Skip("this machine's time.Local is UTC; nothing to distinguish from a local day")
	}
	localMidnight := time.Date(2026, 9, 26, 0, 30, 0, 0, time.Local)
	events := []schema.Event{
		testEvent("s1", "claude-sonnet-5", localMidnight, 1000, ptr(0), ptr(0), 100),
	}
	b := Assemble(events, testConfig(), nil, nil, sysmon.MachineProfile{},
		localMidnight.Add(-time.Hour), localMidnight.Add(time.Hour), localMidnight, nil)
	if len(b.Daily) != 1 {
		t.Fatalf("expected 1 daily row, got %d: %+v", len(b.Daily), b.Daily)
	}
	if want := "2026-09-26"; b.Daily[0].Date != want {
		t.Fatalf("Daily[0].Date = %q, want %q (the local day, not the UTC one)", b.Daily[0].Date, want)
	}
}

func TestTopTurns_CapsAtTenAndOrdersByTokensDescending(t *testing.T) {
	base := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	var events []schema.Event
	for i := 0; i < 15; i++ {
		events = append(events, testEvent("s1", "claude-sonnet-5", base.Add(time.Duration(i)*time.Minute), int64(1000*(i+1)), ptr(0), ptr(0), 100))
	}
	b := Assemble(events, testConfig(), nil, nil, sysmon.MachineProfile{}, base, base.Add(time.Hour), base, nil)
	if len(b.TopTurns) != topTurnsCount {
		t.Fatalf("expected %d top turns, got %d", topTurnsCount, len(b.TopTurns))
	}
	for i := 1; i < len(b.TopTurns); i++ {
		if b.TopTurns[i].tokens() > b.TopTurns[i-1].tokens() {
			t.Fatal("top turns not sorted by tokens descending")
		}
	}
	// The largest turn (i=14, 15000 fresh) must be first.
	if b.TopTurns[0].Fresh != 15000 {
		t.Fatalf("expected the largest turn first, got Fresh=%d", b.TopTurns[0].Fresh)
	}
}

func TestPressureEpisodes_GroupsConsecutiveHighSamples(t *testing.T) {
	base := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	high := func(at time.Time) sysmon.Sample {
		return sysmon.Sample{Ts: at, CPUPct: 95, Cores: []float64{95}, MemUsedMB: 15500, MemTotalMB: 16000, CPUQueue: 5, DiskLatMs: 25, GPUPct: 90}
	}
	low := func(at time.Time) sysmon.Sample {
		return sysmon.Sample{Ts: at, CPUPct: 5, Cores: []float64{5}, MemUsedMB: 2000, MemTotalMB: 16000}
	}
	samples := []sysmon.Sample{
		low(base), high(base.Add(10 * time.Second)), high(base.Add(20 * time.Second)), low(base.Add(30 * time.Second)),
		high(base.Add(40 * time.Second)),
	}
	episodes := pressureEpisodes(samples)
	if len(episodes) != 2 {
		t.Fatalf("expected 2 pressure episodes, got %d: %+v", len(episodes), episodes)
	}
	if !episodes[0].Start.Equal(base.Add(10 * time.Second)) {
		t.Fatalf("expected first episode to start at +10s, got %v", episodes[0].Start)
	}
}

func TestBuildSummaryMD_StaysUnderSizeBudget(t *testing.T) {
	base := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	var events []schema.Event
	var findings []advisor.Finding
	// A busy 24h window: one turn every 2 minutes across 3 models, plus a
	// finding per turn, to stress the size budget with realistic volume
	// rather than a handful of rows.
	models := []string{"claude-sonnet-5", "claude-opus-5", "gpt-5"}
	for i := 0; i < 720; i++ {
		at := base.Add(time.Duration(i) * 2 * time.Minute)
		m := models[i%len(models)]
		events = append(events, testEvent("s1", m, at, 5000, ptr(0), ptr(1000), 500))
		findings = append(findings, advisor.Finding{
			RuleID: "context-window", At: at, Message: "some finding message with a bit of detail in it",
			Suggestion: "do something concrete about it",
		})
	}
	sys := []sysmon.Sample{{Ts: base, CPUPct: 50, Cores: []float64{50}, MemUsedMB: 4000, MemTotalMB: 16000}}
	b := Assemble(events, testConfig(), sys, nil, sysmon.MachineProfile{CPUModel: "Test CPU"}, base, base.Add(24*time.Hour), base, findings)

	summary := BuildSummaryMD(b)
	const budget = 30 * 1024
	if len(summary) > budget {
		t.Fatalf("summary.md is %d bytes, over the ~30KB budget", len(summary))
	}
}

func TestWrite_CreatesThreeFilesWithReportedSizes(t *testing.T) {
	base := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	events := []schema.Event{testEvent("s1", "claude-sonnet-5", base, 1000, ptr(0), ptr(0), 100)}
	b := Assemble(events, testConfig(), nil, nil, sysmon.MachineProfile{}, base, base.Add(time.Hour), base, nil)

	dir := t.TempDir()
	res, err := Write(dir, b)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	for name, size := range map[string]int{res.SummaryPath: res.SummaryBytes, res.DataJSONPath: res.DataJSONBytes, res.DailyCSVPath: res.DailyCSVBytes} {
		if size <= 0 {
			t.Fatalf("expected a positive size for %s, got %d", name, size)
		}
	}
}

func TestRedact_HashesSessionID(t *testing.T) {
	b := Bundle{Turns: []TurnRow{{SessionID: "sess-raw-1"}}}
	if got := Redact(b, "s").Turns[0].SessionID; got == "sess-raw-1" || got == "" {
		t.Fatalf("SessionID not hashed: %q", got)
	}
}

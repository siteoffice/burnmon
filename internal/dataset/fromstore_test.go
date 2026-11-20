package dataset

import (
	"testing"
	"time"

	"burnmon/internal/pricing"
	"burnmon/internal/schema"
)

func ptr[T any](v T) *T { return &v }

func TestSessionsFromEventsAggregatesOneSession(t *testing.T) {
	cfg := pricing.Defaults()
	events := []schema.Event{
		{
			Vendor: "anthropic", SessionID: "sess-a", RequestID: "r1",
			Model: cfg.Families()[0], Title: "hello", Surface: "cli",
			At: time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC),
			Input: 100, Output: 10,
		},
		{
			Vendor: "anthropic", SessionID: "sess-a", RequestID: "r2",
			Model: cfg.Families()[0], Title: "", Surface: "cli",
			At: time.Date(2026, 9, 11, 9, 0, 0, 0, time.UTC),
			Input: 50, Output: 5, CacheRead: ptr(int64(20)),
			Tools: map[string]int64{"Read": 2},
		},
	}
	sessions := SessionsFromEvents(events, &cfg)
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	s := sessions[0]
	if s.SessionID != "sess-a" {
		t.Fatalf("SessionID = %q, want sess-a", s.SessionID)
	}
	if s.Title != "hello" {
		t.Fatalf("Title = %q, want the first non-empty Title seen", s.Title)
	}
	if s.Calls != 2 || s.Fresh != 150 || s.CacheR != 20 || s.Out != 15 {
		t.Fatalf("totals wrong: calls=%d fresh=%d cacheR=%d out=%d", s.Calls, s.Fresh, s.CacheR, s.Out)
	}
	if len(s.Daily) != 2 {
		t.Fatalf("got %d daily buckets, want 2 (2026-09-10 and 2026-09-11)", len(s.Daily))
	}
	if s.Tools["Read"] != 2 {
		t.Fatalf("Tools[Read] = %d, want 2", s.Tools["Read"])
	}
	if s.Start != "2026-09-10T10:00:00.000Z" {
		t.Fatalf("Start = %q, want 2026-09-10T10:00:00.000Z", s.Start)
	}
	if s.End != "2026-09-11T09:00:00.000Z" {
		t.Fatalf("End = %q, want 2026-09-11T09:00:00.000Z", s.End)
	}
}

// TestSessionsFromEventsDailyBucketsLocalDST is extra item 5's spec'd check
// (Wilco, 2026-09-26: local time everywhere): scan.Session.Daily (feeds
// internal/agg's Days/Weeks/Months view, the Now page's own aggregation)
// must key by local calendar day, not whatever Location e.At happens to
// carry. Before this fix buildSession computed it via a bare
// e.At.Format(...) with no explicit .In(time.Local), which silently
// rendered UTC (every event loaded from the store carries a UTC Location),
// so an early-morning local turn landed one day short of Wilco's own
// calendar; the same bug class internal/history's own DST test guards, see
// that test's doc comment for why only early-morning cases distinguish it.
// Skips itself if this machine's time.Local does not actually agree with
// Europe/Amsterdam at the tested instants.
func TestSessionsFromEventsDailyBucketsLocalDST(t *testing.T) {
	ams, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Skip("Europe/Amsterdam tzdata not available:", err)
	}
	cases := []struct {
		name      string
		localWall time.Time
		wantDay   string
	}{
		{"spring forward", time.Date(2026, 3, 29, 0, 30, 0, 0, ams), "2026-03-29"},
		{"fall back", time.Date(2026, 10, 25, 0, 30, 0, 0, ams), "2026-10-25"},
	}
	for _, c := range cases {
		_, offAms := c.localWall.Zone()
		_, offLocal := c.localWall.In(time.Local).Zone()
		if offAms != offLocal {
			t.Skipf("this machine's time.Local does not match Europe/Amsterdam at %s; skipping a DST test that assumes it does", c.localWall)
		}
	}

	cfg := pricing.Defaults()
	for _, c := range cases {
		// .UTC() here matters: a real event's At carries a UTC Location by
		// the time SessionsFromEvents ever sees it (AllEvents/EventsSince
		// parse it back from the store's UTC storage column), not whatever
		// Location the original wall-clock construction used. Passing
		// c.localWall directly would carry the ams Location straight
		// through to buildSession's own e.At.Format(...) call and hide
		// exactly the bug this test exists to catch.
		events := []schema.Event{
			{Vendor: "anthropic", SessionID: "s-" + c.name, RequestID: "r1",
				Model: cfg.Families()[0], At: c.localWall.UTC(), Input: 100, Output: 10},
		}
		sessions := SessionsFromEvents(events, &cfg)
		if len(sessions) != 1 {
			t.Fatalf("%s: got %d sessions, want 1", c.name, len(sessions))
		}
		if _, ok := sessions[0].Daily[c.wantDay]; !ok {
			var keys []string
			for k := range sessions[0].Daily {
				keys = append(keys, k)
			}
			t.Errorf("%s: Daily has no bucket for %s, got keys %v", c.name, c.wantDay, keys)
		}
	}
}

// TestSessionsFromEventsPricesOpenAIEvents prices a Codex turn at the OpenAI
// price book's list rate, the book History reads too. The model is one the
// book carries (gpt-5.6-terra is not in it, so it is "no price" here as in
// History).
func TestSessionsFromEventsPricesOpenAIEvents(t *testing.T) {
	cfg := pricing.Defaults()
	events := []schema.Event{
		{
			Vendor: "openai", Agent: "codex", SessionID: "codex-sess", RequestID: "codex-sess:1",
			Model: "gpt-5.6-sol", Surface: "cli",
			At:        time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC),
			Input:     1000, // fresh, already input-minus-cached per the codex adapter
			CacheRead: ptr(int64(500)),
			Output:    100,
		},
	}
	sessions := SessionsFromEvents(events, &cfg)
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	s := sessions[0]
	want := (1000*4.00 + 500*0.40 + 100*20.00) / 1e6
	if diff := s.Cost - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("Cost = %v, want %v (gpt-5.6-sol list price)", s.Cost, want)
	}
	if s.CostSub != s.Cost {
		t.Fatalf("CostSub = %v, want equal to Cost (no OpenAI subscription calibration in v0.1)", s.CostSub)
	}
	if s.Unpriced != 0 {
		t.Fatalf("Unpriced = %d, want 0 for a priced model", s.Unpriced)
	}
	if _, ok := s.Models["GPT-5.6 Sol"]; !ok {
		t.Fatalf("Models keys = %v, want a GPT-5.6 Sol entry", s.Models)
	}
}

func TestSessionsFromEventsCountsUnpricedOpenAIModel(t *testing.T) {
	cfg := pricing.Defaults()
	events := []schema.Event{
		{
			Vendor: "openai", Agent: "codex", SessionID: "codex-sess2", RequestID: "codex-sess2:1",
			Model: "codex-auto-review", Surface: "cli",
			At:     time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC),
			Input:  1000,
			Output: 100,
		},
	}
	sessions := SessionsFromEvents(events, &cfg)
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	s := sessions[0]
	if s.Cost != 0 {
		t.Fatalf("Cost = %v, want 0 for an unpriced model", s.Cost)
	}
	if s.Unpriced != 1100 {
		t.Fatalf("Unpriced = %d, want 1100 (all tokens on the one unpriced call)", s.Unpriced)
	}
}

// TestBuildSessionActiveMinutesSumsGapsUnderCutoff guards K2's active-time
// formula: gaps at or under active_idle_minutes count in full, a gap
// strictly above it counts 0.
func TestBuildSessionActiveMinutesSumsGapsUnderCutoff(t *testing.T) {
	cfg := pricing.Defaults()
	base := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	events := []schema.Event{
		{Vendor: "anthropic", SessionID: "s1", RequestID: "r1", Model: cfg.Families()[0],
			At: base, Input: 1, Output: 1},
		{Vendor: "anthropic", SessionID: "s1", RequestID: "r2", Model: cfg.Families()[0],
			At: base.Add(5 * time.Minute), Input: 1, Output: 1}, // gap 5m, counts
		{Vendor: "anthropic", SessionID: "s1", RequestID: "r3", Model: cfg.Families()[0],
			At: base.Add(5*time.Minute + 10*time.Minute), Input: 1, Output: 1}, // gap 10m, exactly at cutoff, counts
		{Vendor: "anthropic", SessionID: "s1", RequestID: "r4", Model: cfg.Families()[0],
			At: base.Add(5*time.Minute + 10*time.Minute + 11*time.Minute), Input: 1, Output: 1}, // gap 11m, above cutoff, counts 0
	}
	sessions := SessionsFromEvents(events, &cfg)
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	s := sessions[0]
	want := 15.0 // 5 + 10 + 0
	if s.ActiveMinutes != want {
		t.Fatalf("ActiveMinutes = %v, want %v (5m + 10m-at-cutoff counted, 11m-over-cutoff zeroed)", s.ActiveMinutes, want)
	}
}

// TestBuildSessionActiveMinutesZeroForSingleEvent guards the degenerate
// case: no gaps means no active time, not NaN or negative.
func TestBuildSessionActiveMinutesZeroForSingleEvent(t *testing.T) {
	cfg := pricing.Defaults()
	events := []schema.Event{
		{Vendor: "anthropic", SessionID: "solo", RequestID: "r1", Model: cfg.Families()[0],
			At: time.Now().UTC(), Input: 1, Output: 1},
	}
	sessions := SessionsFromEvents(events, &cfg)
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	if sessions[0].ActiveMinutes != 0 {
		t.Fatalf("ActiveMinutes = %v, want 0 for a single-event session", sessions[0].ActiveMinutes)
	}
}

// TestBuildSessionActiveMinutesZeroWhenAllEventsSameInstant guards the
// degenerate case of several events sharing one timestamp (a burst, or a
// synthetic zero-gap replay): every gap is 0, so ActiveMinutes is 0, not
// negative or NaN.
func TestBuildSessionActiveMinutesZeroWhenAllEventsSameInstant(t *testing.T) {
	cfg := pricing.Defaults()
	at := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	events := []schema.Event{
		{Vendor: "anthropic", SessionID: "burst", RequestID: "r1", Model: cfg.Families()[0], At: at, Input: 1, Output: 1},
		{Vendor: "anthropic", SessionID: "burst", RequestID: "r2", Model: cfg.Families()[0], At: at, Input: 1, Output: 1},
		{Vendor: "anthropic", SessionID: "burst", RequestID: "r3", Model: cfg.Families()[0], At: at, Input: 1, Output: 1},
	}
	sessions := SessionsFromEvents(events, &cfg)
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	if sessions[0].ActiveMinutes != 0 {
		t.Fatalf("ActiveMinutes = %v, want 0 for same-instant events", sessions[0].ActiveMinutes)
	}
}

// TestBuildSessionCarriesClient guards K1's client propagation into
// scan.Session, mirroring how Owner is already carried.
func TestBuildSessionCarriesClient(t *testing.T) {
	cfg := pricing.Defaults()
	events := []schema.Event{
		{Vendor: "anthropic", SessionID: "s1", RequestID: "r1", Model: cfg.Families()[0],
			At: time.Now().UTC(), Input: 1, Output: 1, Client: "ClientB"},
	}
	sessions := SessionsFromEvents(events, &cfg)
	if sessions[0].Client != "ClientB" {
		t.Fatalf("Client = %q, want ClientB", sessions[0].Client)
	}
}

// TestBuildSessionCarriesVendorAndAgent guards Task 2's fix: scan.Session
// now carries Vendor and Agent straight from its events, so the Sessions tab
// can label a session by its real harness instead of Surface's Claude-only
// vocabulary (Surface "cli" is shared by Claude Code, Codex and Copilot CLI
// alike).
func TestBuildSessionCarriesVendorAndAgent(t *testing.T) {
	cfg := pricing.Defaults()
	events := []schema.Event{
		{Vendor: "openai", Agent: "codex", SessionID: "s1", RequestID: "r1", Model: "gpt-5.6-terra",
			Surface: "cli", At: time.Now().UTC(), Input: 1, Output: 1},
	}
	sessions := SessionsFromEvents(events, &cfg)
	if sessions[0].Vendor != "openai" {
		t.Fatalf("Vendor = %q, want openai", sessions[0].Vendor)
	}
	if sessions[0].Agent != "codex" {
		t.Fatalf("Agent = %q, want codex", sessions[0].Agent)
	}
}

// TestActiveMinutesByClientSumsAcrossSessions guards K2's per-client
// rollup: the sum of its sessions' ActiveMinutes.
func TestActiveMinutesByClientSumsAcrossSessions(t *testing.T) {
	cfg := pricing.Defaults()
	base := time.Date(2026, 9, 22, 9, 0, 0, 0, time.UTC)
	events := []schema.Event{
		{Vendor: "anthropic", SessionID: "s1", RequestID: "r1", Model: cfg.Families()[0], At: base, Input: 1, Output: 1, Client: "ClientB"},
		{Vendor: "anthropic", SessionID: "s1", RequestID: "r2", Model: cfg.Families()[0], At: base.Add(4 * time.Minute), Input: 1, Output: 1, Client: "ClientB"},
		{Vendor: "anthropic", SessionID: "s2", RequestID: "r1", Model: cfg.Families()[0], At: base, Input: 1, Output: 1, Client: "ClientB"},
		{Vendor: "anthropic", SessionID: "s2", RequestID: "r2", Model: cfg.Families()[0], At: base.Add(6 * time.Minute), Input: 1, Output: 1, Client: "ClientB"},
	}
	sessions := SessionsFromEvents(events, &cfg)
	byClient := ActiveMinutesByClient(sessions)
	if byClient["ClientB"] != 10 {
		t.Fatalf("ActiveMinutesByClient[ClientB] = %v, want 10 (4 + 6)", byClient["ClientB"])
	}
}

// TestSessionsFromEventsPricesGitHubEventsThroughCopilotBook guards Task 2's
// pricing fix: a github-vendor (Copilot) event used to fall through to
// Claude's family-generic price table (cfg.ModelFamily's FallbackFamily,
// "sonnet") whenever its model didn't substring-match "opus"/"sonnet"/
// "haiku"/"fable"; it must now price through CopilotCredits' own book.
func TestSessionsFromEventsPricesGitHubEventsThroughCopilotBook(t *testing.T) {
	cfg := pricing.Defaults()
	events := []schema.Event{
		{
			Vendor: "github", Agent: "copilot-cli", SessionID: "copilot-sess", RequestID: "copilot-sess:1",
			Model: "claude-sonnet-5", Surface: "cli",
			At:     time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC),
			Input:  1000,
			Output: 100,
		},
	}
	sessions := SessionsFromEvents(events, &cfg)
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	s := sessions[0]
	want := (1000*2.00 + 100*10.00) / 1e6 // CopilotCredits book's claude-sonnet-5 rate, not Claude's family price
	if diff := s.Cost - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("Cost = %v, want %v (CopilotCredits book rate)", s.Cost, want)
	}
	if s.Unpriced != 0 {
		t.Fatalf("Unpriced = %d, want 0 for a model the Copilot book covers", s.Unpriced)
	}
}

// TestSessionsFromEventsNoPriceForUnbookedVendor guards Task 2's "no price
// exists, show 'no price' instead of a Claude price" rule: a vendor with no
// book at all (Hermes/"nous") must price at 0 and count its tokens as
// unpriced, never fall back to a guessed Claude figure.
func TestSessionsFromEventsNoPriceForUnbookedVendor(t *testing.T) {
	cfg := pricing.Defaults()
	events := []schema.Event{
		{
			Vendor: "nous", Agent: "hermes", SessionID: "hermes-sess", RequestID: "hermes-sess:1",
			Model: "some-local-model", Surface: "unknown",
			At:     time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC),
			Input:  1000,
			Output: 100,
		},
	}
	sessions := SessionsFromEvents(events, &cfg)
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	s := sessions[0]
	if s.Cost != 0 {
		t.Fatalf("Cost = %v, want 0 for a vendor with no price book", s.Cost)
	}
	if s.Unpriced != 1100 {
		t.Fatalf("Unpriced = %d, want 1100 (all tokens on the one unpriced call)", s.Unpriced)
	}
}

func TestSessionsFromEventsGroupsByVendorAndSessionID(t *testing.T) {
	cfg := pricing.Defaults()
	events := []schema.Event{
		{Vendor: "anthropic", SessionID: "s1", RequestID: "r1", Model: cfg.Families()[0],
			At: time.Now().UTC(), Input: 1, Output: 1},
		{Vendor: "anthropic", SessionID: "s2", RequestID: "r2", Model: cfg.Families()[0],
			At: time.Now().UTC(), Input: 1, Output: 1},
	}
	sessions := SessionsFromEvents(events, &cfg)
	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2", len(sessions))
	}
}

// Package devexport builds BurnMon Dev's AI-ready export bundle (design doc
// "Export (AI-ready bundle)", phase 4): summary.md, data.json and
// daily.csv, meant to be handed to a model for concrete performance and
// token-usage improvements. Assemble and the three Build* functions are
// pure (data in, string/bytes out, no file I/O); only Write (write.go)
// touches disk, and only after a caller has already fetched every input
// from the store and sysmon. Never reads schema.Event.Title (a session's
// cleaned first-user-message text): the plan's "never export prompt or
// response text" rule holds by construction, not by filtering it back out.
package devexport

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"burnmon/internal/advisor"
	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/sysmon"
)

// Schema is the export bundle's own format version, independent of
// internal/export's K4 Schema (a different file, a different audience).
const Schema = 1

// TurnRow is one real turn inside the export window. Deliberately carries
// no prompt or response text: Title (schema.Event's own cleaned
// first-user-message field) is never read anywhere in this package.
type TurnRow struct {
	At            time.Time `json:"at"`
	Vendor        string    `json:"vendor"`
	Harness       string    `json:"harness"` // schema.Event.Agent
	SessionID     string    `json:"session_id"`
	Model         string    `json:"model"`
	Fresh         int64     `json:"fresh"`
	CacheWrite    int64     `json:"cache_write"`
	CacheRead     int64     `json:"cache_read"`
	Output        int64     `json:"output"`
	CostUSD       float64   `json:"cost_usd"`
	CacheHitRatio float64   `json:"cache_hit_ratio"`
	// Project, Owner and Client are the fields --redact replaces with
	// stable short hashes (Redact below); "" when the store carries none.
	Project string `json:"project,omitempty"`
	Owner   string `json:"owner,omitempty"`
	Client  string `json:"client,omitempty"`
}

func (t TurnRow) tokens() int64 { return t.Fresh + t.CacheWrite + t.CacheRead + t.Output }

// HarnessModelTotal is one (harness, model) pair's totals over the whole
// export window.
type HarnessModelTotal struct {
	Harness    string  `json:"harness"`
	Model      string  `json:"model"`
	Fresh      int64   `json:"fresh"`
	CacheWrite int64   `json:"cache_write"`
	CacheRead  int64   `json:"cache_read"`
	Output     int64   `json:"output"`
	CostUSD    float64 `json:"cost_usd"`
}

// DailyRow is daily.csv's own row shape: date, harness, model, token kinds,
// cost, per the plan's fixed column list.
type DailyRow struct {
	Date       string
	Harness    string
	Model      string
	Fresh      int64
	CacheWrite int64
	CacheRead  int64
	Output     int64
	CostUSD    float64
}

// PressureEpisode is one run of consecutive system samples at or above
// pressureEpisodeThreshold.
type PressureEpisode struct {
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	PeakScore int       `json:"peak_score"`
	AvgScore  float64   `json:"avg_score"`
}

// pressureEpisodeThreshold matches advisor.DefaultThresholds.HeavyTurnPressure:
// both call a score "high" at the same point, so a reader is not asked to
// reconcile two different cutoffs for what looks like the same idea.
const pressureEpisodeThreshold = 70

// topTurnsCount is "top 10 expensive turns" (plan, Export section).
const topTurnsCount = 10

// findingsCap bounds how many advisor findings summary.md renders, so a
// pathological window (many heavy-turn-pressure hits) cannot blow past the
// ~30KB size budget (design doc phase 4 item D) on its own.
const findingsCap = 25

// nearestStateMaxStaleness is wider than advisor's own rule staleness
// (60s): here it is just "closest available reading for a report", not a
// rule's pass/fail gate.
const nearestStateMaxStaleness = 5 * time.Minute

// Bundle is everything the three Build* functions read. A caller (the
// bdevExport binding, or burnmon-dev.exe export) builds one Bundle via
// Assemble, optionally runs it through Redact, then calls all three
// builders against the same Bundle so every file in one export agrees with
// the others.
type Bundle struct {
	Schema      int       `json:"schema"`
	GeneratedAt time.Time `json:"generated_at"`
	Since       time.Time `json:"since"`
	Until       time.Time `json:"until"`
	Redacted    bool      `json:"redacted"`

	Machine sysmon.MachineProfile `json:"machine"`

	// SystemTimeline and ProcessGroupTimeline are passed straight through
	// from the sysmon store, already scoped to [Since, Until) by the
	// caller, at their native 10s resolution.
	SystemTimeline       []sysmon.Sample             `json:"system_timeline"`
	ProcessGroupTimeline []sysmon.ProcessGroupSample `json:"process_group_timeline"`

	Turns    []TurnRow         `json:"turns"`
	Findings []advisor.Finding `json:"findings,omitempty"`

	// Derived below by Assemble, so every Build* function is pure
	// formatting rather than re-deriving the same aggregates three times.
	HarnessModelTotals []HarnessModelTotal `json:"harness_model_totals"`
	Daily              []DailyRow          `json:"-"`
	TopTurns           []TurnRow           `json:"-"`
	PressureEpisodes   []PressureEpisode   `json:"pressure_episodes"`
}

// isTurn mirrors every other package's own copy (insight, live, history,
// export): a real API-call turn, not the claude adapter's synthetic
// tool-only event.
func isTurn(e schema.Event) bool {
	return !(e.Model == "" && e.Input == 0 && e.Output == 0)
}

// turnCostUSD mirrors internal/live's own unexported turnCost: openai
// vendor uses its own calibrated call cost, everything else uses the
// subscription-share cost for its model family. Kept as a small
// duplication rather than an export from live, the same tradeoff
// pressure_windows.go's own doc comment makes for its PDH counters.
func turnCostUSD(e schema.Event, cfg *pricing.Config) float64 {
	if cfg == nil {
		return 0
	}
	if e.Vendor == "openai" {
		var cr int64
		if e.CacheRead != nil {
			cr = *e.CacheRead
		}
		c, _ := cfg.OpenAICallCostUSD(e.Model, e.Input, cr, e.Output)
		return c
	}
	fam := cfg.ModelFamily(e.Model)
	return cfg.CallCostSubUSD(fam, e.Input, e.Output)
}

// Assemble builds one Bundle from already-fetched inputs: events (any
// window, filtered here to [since, until) and to real turns only),
// system/process-group samples already scoped to the same window, the
// machine profile, and findings the caller already computed (typically
// advisor.Analyze over the same window). No I/O happens here.
func Assemble(
	events []schema.Event,
	cfg *pricing.Config,
	systemTimeline []sysmon.Sample,
	processGroupTimeline []sysmon.ProcessGroupSample,
	machine sysmon.MachineProfile,
	since, until, generatedAt time.Time,
	findings []advisor.Finding,
) Bundle {
	var turns []TurnRow
	for _, e := range events {
		if !isTurn(e) || e.At.Before(since) || !e.At.Before(until) {
			continue
		}
		var cw, cr int64
		if e.CacheWrite != nil {
			cw = *e.CacheWrite
		}
		if e.CacheRead != nil {
			cr = *e.CacheRead
		}
		var hitRatio float64
		if e.Input+cr > 0 {
			hitRatio = float64(cr) / float64(e.Input+cr)
		}
		turns = append(turns, TurnRow{
			At: e.At.UTC(), Vendor: e.Vendor, Harness: e.Agent, SessionID: e.SessionID, Model: e.Model,
			Fresh: e.Input, CacheWrite: cw, CacheRead: cr, Output: e.Output,
			CostUSD:       turnCostUSD(e, cfg),
			CacheHitRatio: hitRatio,
			Project:       e.Project, Owner: e.Owner, Client: e.Client,
		})
	}
	sort.SliceStable(turns, func(i, j int) bool { return turns[i].At.Before(turns[j].At) })

	b := Bundle{
		Schema: Schema, GeneratedAt: generatedAt.UTC(), Since: since.UTC(), Until: until.UTC(),
		Machine: machine, SystemTimeline: systemTimeline, ProcessGroupTimeline: processGroupTimeline,
		Turns: turns, Findings: findings,
	}
	b.HarnessModelTotals = harnessModelTotals(turns)
	b.Daily = dailyRows(turns)
	b.TopTurns = topTurns(turns, topTurnsCount)
	b.PressureEpisodes = pressureEpisodes(systemTimeline)
	return b
}

func harnessModelTotals(turns []TurnRow) []HarnessModelTotal {
	type key struct{ harness, model string }
	byKey := map[key]*HarnessModelTotal{}
	var order []key
	for _, t := range turns {
		k := key{t.Harness, t.Model}
		row := byKey[k]
		if row == nil {
			row = &HarnessModelTotal{Harness: t.Harness, Model: t.Model}
			byKey[k] = row
			order = append(order, k)
		}
		row.Fresh += t.Fresh
		row.CacheWrite += t.CacheWrite
		row.CacheRead += t.CacheRead
		row.Output += t.Output
		row.CostUSD += t.CostUSD
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].harness != order[j].harness {
			return order[i].harness < order[j].harness
		}
		return order[i].model < order[j].model
	})
	out := make([]HarnessModelTotal, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
}

func dailyRows(turns []TurnRow) []DailyRow {
	type key struct{ date, harness, model string }
	byKey := map[key]*DailyRow{}
	var order []key
	for _, t := range turns {
		// t.At is a portable UTC instant (kept that way deliberately: see
		// Assemble's own doc comment), but the day it buckets into must be
		// the local calendar day the rest of the app groups by (--since/
		// --until, now local, WS2 item 5); a bare UTC .Format here bucketed
		// early-local-morning turns into the previous local day (found by
		// review, 2026-09-26 - the exact class of bug WS3 already fixed
		// elsewhere: a calendar-day key built from a different Location
		// than the boundary that selected the data).
		date := t.At.In(time.Local).Format("2006-01-02")
		k := key{date, t.Harness, t.Model}
		row := byKey[k]
		if row == nil {
			row = &DailyRow{Date: date, Harness: t.Harness, Model: t.Model}
			byKey[k] = row
			order = append(order, k)
		}
		row.Fresh += t.Fresh
		row.CacheWrite += t.CacheWrite
		row.CacheRead += t.CacheRead
		row.Output += t.Output
		row.CostUSD += t.CostUSD
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if a.date != b.date {
			return a.date < b.date
		}
		if a.harness != b.harness {
			return a.harness < b.harness
		}
		return a.model < b.model
	})
	out := make([]DailyRow, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
}

// topTurns returns the n turns with the largest whole token count,
// descending, ties broken by earliest time first for determinism.
func topTurns(turns []TurnRow, n int) []TurnRow {
	sorted := append([]TurnRow(nil), turns...)
	sort.SliceStable(sorted, func(i, j int) bool {
		ti, tj := sorted[i].tokens(), sorted[j].tokens()
		if ti != tj {
			return ti > tj
		}
		return sorted[i].At.Before(sorted[j].At)
	})
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	return sorted
}

// pressureEpisodes groups history (assumed oldest-first, as sysmon.Store's
// RecentSamples returns it) into runs of consecutive samples whose
// PressureScore is at or above pressureEpisodeThreshold.
func pressureEpisodes(history []sysmon.Sample) []PressureEpisode {
	var out []PressureEpisode
	var cur *PressureEpisode
	var sum float64
	var n int
	flush := func() {
		if cur != nil {
			cur.AvgScore = sum / float64(n)
			out = append(out, *cur)
			cur = nil
			sum, n = 0, 0
		}
	}
	for _, s := range history {
		score := sysmon.PressureScore(s)
		if score < pressureEpisodeThreshold {
			flush()
			continue
		}
		if cur == nil {
			cur = &PressureEpisode{Start: s.Ts, End: s.Ts, PeakScore: score}
		}
		cur.End = s.Ts
		if score > cur.PeakScore {
			cur.PeakScore = score
		}
		sum += float64(score)
		n++
	}
	flush()
	return out
}

// nearestSystemState returns the sysmon.Sample closest to at, or ok=false
// when history is empty or nothing lands within nearestStateMaxStaleness.
func nearestSystemState(history []sysmon.Sample, at time.Time) (sysmon.Sample, bool) {
	var best sysmon.Sample
	bestDiff := time.Duration(1<<63 - 1)
	found := false
	for _, s := range history {
		diff := at.Sub(s.Ts)
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			bestDiff = diff
			best = s
			found = true
		}
	}
	if !found || bestDiff > nearestStateMaxStaleness {
		return sysmon.Sample{}, false
	}
	return best, true
}

// Redact returns a copy of b with every turn's Project, Owner and Client
// replaced by a stable short hash (sha256 of salt+kind+value, truncated),
// so the same name always redacts to the same token across separate
// exports on the same machine, letting a developer correlate them without
// the underlying name ever appearing. salt should be a per-install random
// value the caller loads or creates once (cmd\burnmon-dev's
// loadOrCreateRedactSalt): project, owner and client names are typically
// short and drawn from a small, guessable space (a client folder name, an
// owner slug), so an unsalted hash truncated to 8 hex characters is
// practically reversible by anyone with a list of candidates to try
// (found by review, 2026-09-24). A caller with no salt available may pass
// "", which still hashes (never panics) but loses that protection; every
// production caller in this codebase supplies a real one.
func Redact(b Bundle, salt string) Bundle {
	out := b
	cache := map[string]string{}
	hash := func(kind, v string) string {
		if v == "" {
			return v
		}
		ck := kind + "|" + v
		if h, ok := cache[ck]; ok {
			return h
		}
		sum := sha256.Sum256([]byte(salt + "|" + kind + "|" + v))
		h := kind + "-" + hex.EncodeToString(sum[:])[:8]
		cache[ck] = h
		return h
	}

	out.Turns = make([]TurnRow, len(b.Turns))
	for i, t := range b.Turns {
		t.Project = hash("project", t.Project)
		t.Owner = hash("owner", t.Owner)
		t.Client = hash("client", t.Client)
		t.SessionID = hash("session", t.SessionID)
		out.Turns[i] = t
	}
	out.TopTurns = make([]TurnRow, len(b.TopTurns))
	for i, t := range b.TopTurns {
		t.Project = hash("project", t.Project)
		t.Owner = hash("owner", t.Owner)
		t.Client = hash("client", t.Client)
		t.SessionID = hash("session", t.SessionID)
		out.TopTurns[i] = t
	}
	out.Redacted = true
	return out
}

// ---------------------------------------------------------------------------
// summary.md
// ---------------------------------------------------------------------------

var knownLimits = []string{
	"Token and cost figures use BurnMon's own cost model (subscription-share or list-price, depending on vendor); they are an allocated estimate, not an invoice.",
	"System samples are 10-second snapshots, not a continuous trace; a spike shorter than 10 seconds can be missed entirely.",
	"Process-to-harness classification is best-effort (command line, then process name, then parent chain); an unusual launch wrapper can misclassify a process.",
	"Advisor findings are fixed-threshold rules calibrated on one laptop's own usage; treat them as a first pass, not a certified diagnosis.",
	"This export never includes prompt or response text, by design; \"expensive turn\" and \"context window\" findings are about token counts, not content.",
}

const readyPrompt = `You are looking at one developer's BurnMon Dev export: what AI coding
agents burned in tokens and cost, next to what running them did to this
machine, over one time window. Read the sections below and answer:

1. Where is token spend going, and which sessions or harnesses are the
   worst outliers (see "Top expensive turns")?
2. Does anything here point at a fixable inefficiency: a dropping cache hit
   rate, a session running near its context window, a harness burning CPU
   with no turns recorded against it?
3. Given the advisor findings and pressure episodes below, what is the one
   change most likely to reduce cost or system load this week?

Cite the actual numbers from the sections below rather than general advice.`

func fmtTok(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.0fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func fmtCost(c float64) string { return fmt.Sprintf("$%.2f", c) }

// BuildSummaryMD renders b as the export's model-ready summary: the ready
// prompt first, then machine profile, window, totals, top turns, pressure
// episodes, advisor findings and known limits. Pure formatting: every
// number here already exists on b (Assemble computed the derived fields).
func BuildSummaryMD(b Bundle) string {
	var s strings.Builder

	s.WriteString("# BurnMon Dev export\n\n")
	s.WriteString("## Ready prompt\n\n")
	s.WriteString(readyPrompt)
	s.WriteString("\n\n")

	s.WriteString("## Machine profile\n\n")
	fmt.Fprintf(&s, "- CPU: %s (%d physical / %d logical cores)\n", nonEmpty(b.Machine.CPUModel), b.Machine.CPUPhysical, b.Machine.CPULogical)
	fmt.Fprintf(&s, "- RAM: %.1f GB\n", b.Machine.RAMTotalMB/1024)
	fmt.Fprintf(&s, "- GPU: %s\n", nonEmpty(b.Machine.GPU))
	fmt.Fprintf(&s, "- OS: %s\n", nonEmpty(b.Machine.OS))
	fmt.Fprintf(&s, "- Power: %s, on %s\n\n", nonEmpty(b.Machine.PowerPlan), nonEmpty(b.Machine.PowerSource))

	fmt.Fprintf(&s, "## Window\n\n%s to %s (UTC)%s\n\n",
		b.Since.Format(time.RFC3339), b.Until.Format(time.RFC3339), redactedNote(b.Redacted))

	s.WriteString("## Totals per harness and model\n\n")
	if len(b.HarnessModelTotals) == 0 {
		s.WriteString("No turns in this window.\n\n")
	} else {
		s.WriteString("| Harness | Model | Fresh | Cache write | Cache read | Output | Cost |\n")
		s.WriteString("|---|---|---|---|---|---|---|\n")
		for _, r := range b.HarnessModelTotals {
			fmt.Fprintf(&s, "| %s | %s | %s | %s | %s | %s | %s |\n",
				r.Harness, r.Model, fmtTok(r.Fresh), fmtTok(r.CacheWrite), fmtTok(r.CacheRead), fmtTok(r.Output), fmtCost(r.CostUSD))
		}
		s.WriteString("\n")
	}

	fmt.Fprintf(&s, "## Top %d expensive turns\n\n", topTurnsCount)
	if len(b.TopTurns) == 0 {
		s.WriteString("No turns in this window.\n\n")
	} else {
		s.WriteString("| Time | Harness | Model | Tokens | Cost | System state at that moment |\n")
		s.WriteString("|---|---|---|---|---|---|\n")
		for _, t := range b.TopTurns {
			state := "unknown"
			if sample, ok := nearestSystemState(b.SystemTimeline, t.At); ok {
				state = fmt.Sprintf("CPU %.0f%%, pressure %d", sample.CPUPct, sysmon.PressureScore(sample))
			}
			// Local, not t.At's own stored UTC (found by review,
			// 2026-09-26): every other time this app shows a human is
			// local (the header clock, the turn ticker, the heatmap), and
			// summary.md is read by a human, not machine-parsed like
			// data.json's own At field - a bare UTC stamp with no zone
			// marker here reads as local and is silently 1-2 hours off in
			// Europe/Amsterdam.
			fmt.Fprintf(&s, "| %s | %s | %s | %s | %s | %s |\n",
				t.At.In(time.Local).Format("2006-01-02 15:04:05"), t.Harness, t.Model, fmtTok(t.tokens()), fmtCost(t.CostUSD), state)
		}
		s.WriteString("\n")
	}

	s.WriteString("## Pressure episodes\n\n")
	if len(b.PressureEpisodes) == 0 {
		fmt.Fprintf(&s, "No episode reached pressure %d or above in this window.\n\n", pressureEpisodeThreshold)
	} else {
		s.WriteString("| Start | End | Peak | Avg |\n")
		s.WriteString("|---|---|---|---|\n")
		for _, p := range b.PressureEpisodes {
			fmt.Fprintf(&s, "| %s | %s | %d | %.0f |\n",
				p.Start.Format("15:04:05"), p.End.Format("15:04:05"), p.PeakScore, p.AvgScore)
		}
		s.WriteString("\n")
	}

	s.WriteString("## Advisor findings\n\n")
	findings := b.Findings
	truncated := 0
	if len(findings) > findingsCap {
		truncated = len(findings) - findingsCap
		findings = findings[:findingsCap]
	}
	if len(findings) == 0 {
		s.WriteString("No advisor findings in this window.\n\n")
	} else {
		for _, f := range findings {
			fmt.Fprintf(&s, "- [%s] **%s**: %s - %s\n", f.At.Format("15:04:05"), f.RuleID, f.Message, f.Suggestion)
		}
		if truncated > 0 {
			fmt.Fprintf(&s, "\n_%d more finding(s) omitted; see data.json for the full list._\n", truncated)
		}
		s.WriteString("\n")
	}

	s.WriteString("## Known limits\n\n")
	for _, l := range knownLimits {
		fmt.Fprintf(&s, "- %s\n", l)
	}

	return s.String()
}

func nonEmpty(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

func redactedNote(redacted bool) string {
	if redacted {
		return " (redacted: project, owner and client names replaced with stable hashes)"
	}
	return ""
}

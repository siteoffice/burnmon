// Package insight explains a session's burn instead of just counting it
// (v0.2 I1): given one session's canonical events, ordered by turn, plus its
// model's context window from the price book, it reports Findings such as a
// re-prefill or a compaction. No HTML, no store writes: every Finding is
// computed fresh from the same events the Now snapshot (internal/live) and
// the Sessions tab already load, so there is no cache to invalidate.
package insight

import (
	"fmt"
	"math"
	"sort"
	"time"

	"burnmon/internal/pricing"
	"burnmon/internal/schema"
)

// Kind names one insight rule.
type Kind string

const (
	KindReprefill     Kind = "re-prefill"
	KindCompaction    Kind = "compaction"
	KindContextRunway Kind = "context-runway"
	KindExpensiveTurn Kind = "expensive-turn"
)

// Finding is one thing insight noticed about one turn of one session.
type Finding struct {
	Kind       Kind
	Turn       int // 1-based, over this session's real API-call turns only
	At         time.Time
	Evidence   map[string]float64
	Cause      string
	Confidence float64
}

// compactionDropRatio is I2's compaction threshold: a context drop greater
// than this between consecutive turns, while the session continues. Not
// configurable in v0.2, unlike the re-prefill threshold, per the spec.
const compactionDropRatio = 0.30

// CopilotVSCodeAgent and copilotCompactionFloor scope the compaction rule for
// Copilot in VS Code. Its OTel spans are one row per request and interleave
// the main conversation (a growing context, 60k to 700k tokens in the real
// store) with a helper model's small requests (titles, summaries; 253 to 3,826
// tokens, an independent prompt each time). The helper is no running context,
// so a drop to or between helper requests is not a compaction.
const (
	CopilotVSCodeAgent     = "copilot-vscode"
	copilotCompactionFloor = 10_000
)

// compactionBaseline is the context turn i is compared against, and whether a
// comparison is meaningful at all. Every agent but Copilot in VS Code compares
// with the previous turn. Copilot compares with the previous turn of the same
// model, and only when that context is at least copilotCompactionFloor.
func compactionBaseline(turns []schema.Event, i int) (before int64, ok bool) {
	if turns[i].Agent != CopilotVSCodeAgent {
		return contextOf(turns[i-1]), true
	}
	for j := i - 1; j >= 0; j-- {
		if turns[j].Model == turns[i].Model {
			before = contextOf(turns[j])
			return before, before >= copilotCompactionFloor
		}
	}
	return 0, false
}

// ContextTurn is the turn whose context stands for the session now. Every agent
// but Copilot in VS Code: the newest turn. Copilot: the newest turn at or above
// copilotCompactionFloor, since its newest request is often a helper's. A
// session with only helper-sized requests keeps the newest turn.
func ContextTurn(turns []schema.Event) schema.Event {
	last := turns[len(turns)-1]
	if last.Agent != CopilotVSCodeAgent {
		return last
	}
	for i := len(turns) - 1; i >= 0; i-- {
		if contextOf(turns[i]) >= copilotCompactionFloor {
			return turns[i]
		}
	}
	return last
}

// runwayFitTurns is how many of the session's most recent turns
// context-runway fits its line over, per the spec ("a linear fit over the
// last 10 turns of context size").
const runwayFitTurns = 10

// expensiveTurnPercentile is expensive-turn's threshold: a turn whose total
// tokens land strictly above this percentile of the session's own turns.
const expensiveTurnPercentile = 0.95

// isTurn reports whether e is a real API-call turn rather than the claude
// adapter's synthetic tool-only event. Mirrors internal/live's own isTurn
// and internal/dataset/fromstore.go's buildSession condition, the
// established duplicate rather than a shared export across those packages.
func isTurn(e schema.Event) bool {
	return !(e.Model == "" && e.Input == 0 && e.Output == 0)
}

// contextOf is one turn's total context sent: fresh input plus whatever was
// served from cache and whatever cache was written.
func contextOf(e schema.Event) int64 {
	c := e.Input
	if e.CacheRead != nil {
		c += *e.CacheRead
	}
	if e.CacheWrite != nil {
		c += *e.CacheWrite
	}
	return c
}

func cacheWriteOf(e schema.Event) int64 {
	if e.CacheWrite != nil {
		return *e.CacheWrite
	}
	return 0
}

// Analyze runs every v0.2 I2 rule over events, one session's worth, in any
// order (Analyze sorts by At itself), and returns every Finding in turn
// order, re-prefill and compaction interleaved. events need not be
// pre-filtered: synthetic tool-only events are dropped first, the same way
// internal/live and internal/dataset/fromstore.go already do.
func Analyze(events []schema.Event, cfg *pricing.Config) []Finding {
	turns := make([]schema.Event, 0, len(events))
	for _, e := range events {
		if isTurn(e) {
			turns = append(turns, e)
		}
	}
	sort.SliceStable(turns, func(i, j int) bool { return turns[i].At.Before(turns[j].At) })
	if len(turns) == 0 {
		return nil
	}

	var findings []Finding

	// compactionAt maps a 1-based turn number to whether I2's compaction
	// rule fired there, so the re-prefill rule below can check "compaction
	// just happened" first, per the cause order the spec fixes.
	compactionAt := map[int]bool{}
	for i := 1; i < len(turns); i++ {
		before, ok := compactionBaseline(turns, i)
		after := contextOf(turns[i])
		if !ok || before <= 0 || after >= before {
			continue
		}
		drop := float64(before-after) / float64(before)
		if drop <= compactionDropRatio {
			continue
		}
		turn := i + 1
		compactionAt[turn] = true
		findings = append(findings, Finding{
			Kind: KindCompaction,
			Turn: turn,
			At:   turns[i].At,
			Evidence: map[string]float64{
				"context_before": float64(before),
				"context_after":  float64(after),
				"drop_ratio":     drop,
			},
			Cause:      "context dropped more than 30% since the previous turn",
			Confidence: 1.0,
		})
	}

	threshold := cfg.ReprefillThreshold()
	ttl := cfg.ClaudeCodeCacheTTL()
	for i, e := range turns {
		cw := cacheWriteOf(e)
		if cw <= threshold {
			continue
		}
		turn := i + 1
		cause, confidence, evidence := reprefillCause(turns, i, turn, compactionAt, ttl)
		evidence["cache_write"] = float64(cw)
		evidence["threshold"] = float64(threshold)
		findings = append(findings, Finding{
			Kind:       KindReprefill,
			Turn:       turn,
			At:         e.At,
			Evidence:   evidence,
			Cause:      cause,
			Confidence: confidence,
		})
	}

	if f := contextRunway(turns, cfg); f != nil {
		findings = append(findings, *f)
	}
	findings = append(findings, expensiveTurns(turns)...)

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Turn != findings[j].Turn {
			return findings[i].Turn < findings[j].Turn
		}
		return findings[i].Kind < findings[j].Kind
	})
	return findings
}

// contextRunway implements I2's context-runway rule: a linear fit over the
// session's last runwayFitTurns turns' context size, reporting turns to 80%
// and 90% of the model's context window. Returns nil, not a Finding, when
// the window is unknown, fewer than two turns exist to fit a line over, or
// the fit's slope is zero or negative (a shrinking or flat session has no
// runway to report); RunwayText renders that absence as "runway unknown" for
// the Now card.
func contextRunway(turns []schema.Event, cfg *pricing.Config) *Finding {
	last := turns[len(turns)-1]
	window, ok := cfg.ContextWindow(last.Model)
	if !ok || window <= 0 {
		return nil
	}
	from := len(turns) - runwayFitTurns
	if from < 0 {
		from = 0
	}
	sample := turns[from:]
	if len(sample) < 2 {
		return nil
	}
	y := make([]float64, len(sample))
	for i, e := range sample {
		y[i] = float64(contextOf(e))
	}
	slope, intercept := linearFit(y)
	if slope <= 0 {
		return nil
	}
	lastX := float64(len(y) - 1)
	turnsTo80 := turnsToTarget(0.8*float64(window), slope, intercept, lastX)
	turnsTo90 := turnsToTarget(0.9*float64(window), slope, intercept, lastX)
	return &Finding{
		Kind: KindContextRunway,
		Turn: len(turns),
		At:   last.At,
		Evidence: map[string]float64{
			"slope":       slope,
			"window":      float64(window),
			"turns_to_80": turnsTo80,
			"turns_to_90": turnsTo90,
		},
		Cause:      fmt.Sprintf("about %d turns to 80%% of the window", int64(turnsTo80)),
		Confidence: 0.6,
	}
}

// linearFit is ordinary least squares over y against its own index (0, 1,
// 2, ...), returning the fitted slope and intercept.
func linearFit(y []float64) (slope, intercept float64) {
	n := float64(len(y))
	var sumX, sumY, sumXY, sumXX float64
	for i, v := range y {
		x := float64(i)
		sumX += x
		sumY += v
		sumXY += x * v
		sumXX += x * x
	}
	denom := n*sumXX - sumX*sumX
	if denom == 0 {
		return 0, sumY / n
	}
	slope = (n*sumXY - sumX*sumY) / denom
	intercept = (sumY - slope*sumX) / n
	return slope, intercept
}

// turnsToTarget returns how many turns past lastX the fit line (slope,
// intercept) needs to reach target, floored at zero when the fit already
// meets or exceeds it.
func turnsToTarget(target, slope, intercept, lastX float64) float64 {
	current := intercept + slope*lastX
	if current >= target {
		return 0
	}
	// The small epsilon absorbs float error: on arm64 Go fuses multiply-add
	// (FMA), so an exact fit can land a hair above a whole number and Ceil
	// would report one turn too many.
	turns := math.Ceil((target-current)/slope - 1e-9)
	if turns < 0 {
		turns = 0
	}
	return turns
}

// RunwayText renders the Now card's one-line context-runway gauge: the
// context-runway finding's Cause when Analyze found one for this session, or
// "runway unknown" when it did not (window unknown, fewer than two turns, or
// a zero-or-negative fit slope).
func RunwayText(findings []Finding) string {
	for _, f := range findings {
		if f.Kind == KindContextRunway {
			return f.Cause
		}
	}
	return "runway unknown"
}

// expensiveTurns implements I2's expensive-turn rule: a turn whose total
// tokens (fresh + cache write + cache read + output) land strictly above the
// expensiveTurnPercentile (nearest-rank) of the session's own turns, with
// the dominant token class flagged in Evidence. Needs at least two turns; a
// session too small to have a meaningful percentile reports nothing.
func expensiveTurns(turns []schema.Event) []Finding {
	if len(turns) < 2 {
		return nil
	}
	totals := make([]float64, len(turns))
	for i, e := range turns {
		totals[i] = float64(turnTotal(e))
	}
	sorted := append([]float64(nil), totals...)
	sort.Float64s(sorted)
	rank := int(math.Ceil(expensiveTurnPercentile * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	p95 := sorted[rank-1]

	var findings []Finding
	for i, e := range turns {
		if totals[i] <= p95 {
			continue
		}
		fresh := float64(e.Input)
		cw := float64(cacheWriteOf(e))
		cr := float64(cacheReadOf(e))
		out := float64(e.Output)
		class, amount := "fresh", fresh
		for name, v := range map[string]float64{"cache_write": cw, "cache_read": cr, "output": out} {
			if v > amount {
				class, amount = name, v
			}
		}
		evidence := map[string]float64{
			"fresh":             fresh,
			"cache_write":       cw,
			"cache_read":        cr,
			"output":            out,
			"total":             totals[i],
			"p95":               p95,
			"dominant_" + class: 1,
		}
		findings = append(findings, Finding{
			Kind:     KindExpensiveTurn,
			Turn:     i + 1,
			At:       e.At,
			Evidence: evidence,
			Cause: fmt.Sprintf("turn total %d tokens exceeds session p95 %d, dominated by %s",
				int64(totals[i]), int64(p95), class),
			Confidence: 1.0,
		})
	}
	return findings
}

// turnTotal is one turn's whole token count across every class.
func turnTotal(e schema.Event) int64 {
	return e.Input + cacheWriteOf(e) + cacheReadOf(e) + e.Output
}

func cacheReadOf(e schema.Event) int64 {
	if e.CacheRead != nil {
		return *e.CacheRead
	}
	return 0
}

// reprefillCause implements I2's fixed inference order: compaction just
// happened, model changed since the previous turn, gap since the previous
// turn exceeded the cache TTL, first turn after resume, otherwise unknown.
// The first three all need a previous turn to compare against; turn is
// 1-based, i is turns' 0-based index of the same event.
func reprefillCause(turns []schema.Event, i, turn int, compactionAt map[int]bool, ttl time.Duration) (cause string, confidence float64, evidence map[string]float64) {
	evidence = map[string]float64{}
	if i == 0 {
		// No previous turn in this session's events to compare against.
		// insight has no explicit "this session resumed" signal from the
		// store, so a large cache write on the very first turn we see is
		// read as a resumed session picking back up, per the spec's fixed
		// cause list; a genuinely new session's first turn also always
		// writes its whole prompt to cache, so this cause is a
		// low-confidence best guess, not a hard fact.
		return "first turn after resume", 0.4, evidence
	}
	if compactionAt[turn] || compactionAt[turn-1] {
		return "compaction just happened", 0.9, evidence
	}
	prev, cur := turns[i-1], turns[i]
	if prev.Model != "" && cur.Model != "" && prev.Model != cur.Model {
		evidence["prev_model_changed"] = 1
		return "model changed since the previous turn", 0.8, evidence
	}
	if !prev.At.IsZero() && !cur.At.IsZero() {
		gap := cur.At.Sub(prev.At)
		evidence["gap_seconds"] = gap.Seconds()
		evidence["ttl_seconds"] = ttl.Seconds()
		if gap > ttl {
			return fmt.Sprintf("gap %d min, exceeds %d min cache TTL", int(gap.Minutes()), int(ttl.Minutes())), 0.7, evidence
		}
	}
	return "unknown", 0.3, evidence
}

// Package dataset builds the usage payload shared by the burnmon CLI and
// the burnmon window: source resolution, JSONL parsing with an
// mtime/size cache, aggregation, and the JSON schema injected into the
// dashboard template.
package dataset

import (
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"burnmon/internal/adapter"
	"burnmon/internal/adapter/claude"
	"burnmon/internal/adapter/codex"
	"burnmon/internal/agg"
	"burnmon/internal/pricing"
	"burnmon/internal/scan"
	"burnmon/internal/store"
)

// Distinct errors Collect can return. Callers map each to their own message
// and exit code.
var (
	ErrNoSources  = errors.New("no source folders found")
	ErrNoFiles    = errors.New("no transcript files found")
	ErrNoSessions = errors.New("no sessions inside the reporting window")
)

// SeatError reports an unknown seat tier, with the valid options attached so
// callers can print them without recomputing the list.
type SeatError struct {
	Seat  string
	Valid []string
}

func (e *SeatError) Error() string {
	return fmt.Sprintf("unknown seat tier %q, valid: %s", e.Seat, strings.Join(e.Valid, ", "))
}

func validateSeat(cfg *pricing.Config, seat string) error {
	if _, ok := cfg.Subscription.SeatPriceUSD[seat]; ok {
		return nil
	}
	var valid []string
	for k := range cfg.Subscription.SeatPriceUSD {
		valid = append(valid, k)
	}
	sort.Strings(valid)
	return &SeatError{Seat: seat, Valid: valid}
}

// ---------------------------------------------------------------------------
// Payload, schema 1 of claude_usage_extract.py
// ---------------------------------------------------------------------------

type priceOut struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheWrite float64 `json:"cache_write"`
	CacheRead  float64 `json:"cache_read"`
}

type subOut struct {
	MonthlySubscriptionUSD float64            `json:"monthly_subscription_usd"`
	MonthlySubscriptionEUR float64            `json:"monthly_subscription_eur"`
	SeatsPurchased         int                `json:"seats_purchased"`
	UsageCreditsBalanceEUR float64            `json:"usage_credits_balance_eur"`
	CompanyConsumptionUSD  float64            `json:"company_consumption_usd"`
	Seats                  map[string]int     `json:"seats"`
	SeatPriceUSD           map[string]float64 `json:"seat_price_usd"`
	OutputCostFactor       float64            `json:"output_cost_factor"`
	CalibratedOn           string             `json:"calibrated_on"`
	Window                 string             `json:"window"`
	YourSeat               string             `json:"your_seat"`
	YourSeatPriceUSD       float64            `json:"your_seat_price_usd"`
}

type totalsOut struct {
	Sessions       int     `json:"sessions"`
	Calls          int64   `json:"calls"`
	Tokens         int64   `json:"tokens"`
	Cost           float64 `json:"cost"`
	CostSub        float64 `json:"cost_sub"`
	UnpricedTokens int64   `json:"unpriced_tokens,omitempty"`
}

// Payload is the full dataset injected into the dashboard template.
type Payload struct {
	Schema           int                    `json:"schema"`
	GeneratedAt      string                 `json:"generated_at"`
	WindowFrom       string                 `json:"window_from"`
	WindowTo         string                 `json:"window_to"`
	Prices           map[string]priceOut    `json:"prices_usd_per_mtok"`
	RefreshTaskID    string                 `json:"refresh_task_id"`
	Subscription     subOut                 `json:"subscription"`
	SurfaceLabels    map[string]string      `json:"surface_labels"`
	AgentLabels      map[string]string      `json:"agent_labels"`
	LongSessionCalls int                    `json:"long_session_calls"`
	Totals           totalsOut              `json:"totals"`
	Months           map[string]*agg.Bucket `json:"months"`
	Days             map[string]*agg.Bucket `json:"days"`
	Sessions         []*scan.Session        `json:"sessions"`
}

func round4(x float64) float64 { return math.Round(x*1e4) / 1e4 }

// monthStart is a LOCAL calendar boundary (Wilco's decision, 2026-09-26:
// local time everywhere): the 1st of the month back months before t's own
// local month, local midnight. Determines which months' sessions stay in
// Payload.Months/Days/Sessions (agg.Build's own cutoff), so this must match
// Wilco's real calendar, not UTC's.
func monthStart(t time.Time, back int) time.Time {
	t = t.In(time.Local)
	y, m := t.Year(), int(t.Month())-back
	for m <= 0 {
		m += 12
		y--
	}
	return time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.Local)
}

// BuildPayload assembles the schema-1 payload from already-aggregated data.
// weeks is accepted (agg.Build returns it alongside months and days) but not
// put in Payload: no v0.2 tab reads D.weeks (checked against every D.*
// reference in internal/report/template.html), so it would only inflate the
// embedded JSON and dashboard.html for nothing. See SESSION_LOG.md, v0.2.1
// hang patch.
func BuildPayload(cfg *pricing.Config, seat string, cutoff, today time.Time,
	months, weeks, days map[string]*agg.Bucket, kept []*scan.Session) Payload {

	prices := map[string]priceOut{}
	for _, fam := range cfg.Families() {
		p := cfg.Prices[fam]
		prices[p.Label] = priceOut{
			Input:      p.In,
			Output:     p.Out,
			CacheWrite: round4(p.In * cfg.CacheWriteMult),
			CacheRead:  round4(p.In * cfg.CacheReadMult),
		}
	}

	t := totalsOut{Sessions: len(kept)}
	for _, m := range months {
		t.Calls += m.Calls
		t.Tokens += m.Tokens
		t.Cost += m.Cost
		t.CostSub += m.CostSub
	}
	t.Cost = round4(t.Cost)
	t.CostSub = round4(t.CostSub)
	for _, s := range kept {
		t.UnpricedTokens += s.Unpriced
	}

	sub := cfg.Subscription
	return Payload{
		Schema:      2, // v0.5.0: added by_tool to agg.Bucket
		GeneratedAt: time.Now().UTC().Format("2006-01-02T15:04:05") + "+00:00",
		WindowFrom:  cutoff.Format("2006-01-02"),
		WindowTo:    today.Format("2006-01-02"),
		Prices:      prices,
		Subscription: subOut{
			MonthlySubscriptionUSD: sub.MonthlySubscriptionUSD,
			MonthlySubscriptionEUR: sub.MonthlySubscriptionEUR,
			SeatsPurchased:         sub.SeatsPurchased,
			UsageCreditsBalanceEUR: sub.UsageCreditsBalanceEUR,
			CompanyConsumptionUSD:  sub.CompanyConsumptionUSD,
			Seats:                  sub.Seats,
			SeatPriceUSD:           sub.SeatPriceUSD,
			OutputCostFactor:       sub.OutputCostFactor,
			CalibratedOn:           sub.CalibratedOn,
			Window:                 sub.Window,
			YourSeat:               seat,
			YourSeatPriceUSD:       sub.SeatPriceUSD[seat],
		},
		SurfaceLabels:    scan.SurfaceLabel,
		AgentLabels:      scan.AgentLabel,
		LongSessionCalls: scan.LongSessionCalls,
		Totals:           t,
		Months:           months,
		Days:             days,
		Sessions:         kept,
	}
}

// ---------------------------------------------------------------------------
// Cache: mtime/size-aware parsing shared by the CLI and the app window
// ---------------------------------------------------------------------------

// Cache resolves sources, ingests new transcript bytes into the store, and
// builds the dashboard Payload from what the store now holds. The name is
// kept from v0.0.1's in-memory/gob cache for minimal call-site churn in
// cmd/burnmon and cmd/burnmon-cli; what it actually caches now is durable,
// in Store, not in this struct.
type Cache struct {
	Store *store.Store

	// Sources, Files, SlowFiles, SlowScannedAt, DroppedDuplicates: same
	// meaning as v0.0.1, read by cmd/burnmon for its stamp/log lines.
	Sources           []string
	Files             []string
	SlowFiles         []string
	SlowScannedAt     time.Time
	DroppedDuplicates int

	// RootsByAdapter is resolveSources's per-adapter root list from the last
	// full pass, cached across fast-tier passes exactly like SlowFiles, so
	// ingest can classify a reused WSL file's adapter without re-probing WSL.
	// Written by Collect and SeedNativeRoots, read by ingest (via IngestFile,
	// the live watcher's path) and RootsSnapshot; rootsMu guards it because,
	// since v0.1.1 F2, the live watcher's IngestFile calls run concurrently
	// with a full Collect backfill rather than being serialised behind it
	// (see cmd/burnmon's app.rebuild and startLiveWatch, SESSION_LOG.md).
	RootsByAdapter map[string][]string
	rootsMu        sync.RWMutex

	// onAdvance, set through SetOnAdvance, is called with every file whose
	// cursor an ingest pass moved forward.
	onAdvance atomic.Pointer[func(path string)]
}

// SetOnAdvance registers fn to be called with every file whose cursor an
// ingest pass moves forward. cmd wires it to watch.Watcher.Track, so a file
// that just grew is tail-polled (WS2 follow-up, 2026-09-29, item 2). Atomic
// because the live watcher's IngestFile calls may already be running when
// it is set.
func (c *Cache) SetOnAdvance(fn func(path string)) {
	c.onAdvance.Store(&fn)
}

// SeedNativeRoots sets RootsByAdapter's native (non-WSL) entries before any
// Collect has run, so IngestFile classifies a live Claude or Codex file
// correctly from the moment the app's live watcher starts, rather than
// falling back to the claude adapter (adapterForPath's zero-roots default)
// for every Codex file until the first full backfill finishes. A later
// Collect's own RootsByAdapter write (native plus any WSL roots) simply
// replaces this.
func (c *Cache) SeedNativeRoots(claudeRoots, codexRoots []string) {
	c.rootsMu.Lock()
	defer c.rootsMu.Unlock()
	c.RootsByAdapter = map[string][]string{"claude": claudeRoots, "codex": codexRoots}
}

// RootsSnapshot returns a copy of the current RootsByAdapter, safe to read
// while a Collect may be writing it concurrently.
func (c *Cache) RootsSnapshot() map[string][]string {
	c.rootsMu.RLock()
	defer c.rootsMu.RUnlock()
	out := make(map[string][]string, len(c.RootsByAdapter))
	for k, v := range c.RootsByAdapter {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// dedupSessions removes the same conversation when it was recorded under
// two session IDs. The key is six fields already on scan.Session: vendor,
// start, end, call count, output tokens and cache-read tokens. Vendor guards
// against two different vendors' sessions colliding on the other five
// fields by coincidence (V3.1 cleanup: before Vendor existed on Session,
// nothing ruled this out); two genuinely different same-vendor conversations
// would still need identical start and end timestamps to the millisecond
// plus identical token counts to collide, so this is safe without any
// requestId bookkeeping across files. On a collision the session whose
// SessionID sorts first is kept, so the choice is deterministic across runs
// and independent of file mtime.
func dedupSessions(sessions []*scan.Session) ([]*scan.Session, int) {
	best := map[string]*scan.Session{}
	for _, s := range sessions {
		key := s.Vendor + "|" + s.Start + "|" + s.End + "|" +
			strconv.FormatInt(s.Calls, 10) + "|" +
			strconv.FormatInt(s.Out, 10) + "|" +
			strconv.FormatInt(s.CacheR, 10)
		if prev, ok := best[key]; !ok || s.SessionID < prev.SessionID {
			best[key] = s
		}
	}
	kept := make([]*scan.Session, 0, len(best))
	for _, s := range best {
		kept = append(kept, s)
	}
	return kept, len(sessions) - len(kept)
}

// ---------------------------------------------------------------------------
// Source resolution: auto-detect vs. explicit, WSL's slow tier kept separate
// ---------------------------------------------------------------------------

// CollectOpts groups Collect's per-call parameters. Sources and SlowSources
// are what the caller already knows, not what Collect should go find:
// resolving WSL sources can touch the filesystem over the 9P file server and
// even cold-start a stopped distro, which must never happen off the
// caller's own decided cadence (see RefreshSlow, and "Two refresh cadences"
// in docs/2026-08-17_wsl-source-detection-design.md).
type CollectOpts struct {
	Seat    string
	MonthsN int

	// Sources is the full resolved source list. Empty means auto-detect:
	// Collect calls scan.DefaultSourcesWithOptions itself, gated by
	// RefreshSlow so a fast-tier pass never touches WSL.
	Sources []string

	// SlowSources is the subset of Sources that is slow to walk (WSL over
	// the 9P file server). May be empty. Only consulted when Sources is
	// non-empty; the auto-detect path (Sources empty) computes its own
	// split instead.
	SlowSources []string

	// RefreshSlow requests a full pass: resolve (when auto-detecting) and
	// walk the slow sources this time, rather than reusing the file list
	// from the last pass that did.
	RefreshSlow bool

	// ForceFull re-reads every file from offset 0 even if its stored cursor
	// says it is fully read, the store-backed equivalent of v0.0.1's
	// -no-cache.
	ForceFull bool
}

// resolveSources implements the source-list half of "Two refresh cadences".
// On a full pass it is free to auto-detect, including WSL. On a fast-tier
// pass, an auto-detect must never call anything that can touch a WSL
// distro, so it resolves native sources only and leaves the slow tier to
// whatever Cache.SlowFiles already holds from the last full pass.
// cfg.ExtraSources is appended to the fast tier in every case, then the
// caller dedupes.
// roots is, per adapter name, every root folder resolved this pass (native
// plus WSL when scanned); ingest uses it to classify which adapter parses a
// given file. It is nil on the explicit -source path, which has only ever
// meant Claude (Codex has no CLI flag of its own in v0.1), and on a
// fast-tier pass, where the caller is expected to reuse the roots recorded
// by the last full pass, exactly as it reuses c.SlowFiles.
func resolveSources(cfg *pricing.Config, opts CollectOpts) (fast, slow []string, roots map[string][]string) {
	switch {
	case len(opts.Sources) > 0:
		slow = opts.SlowSources
		fast = subtractCaseInsensitive(opts.Sources, slow)
		roots = map[string][]string{"claude": append([]string{}, opts.Sources...)}
	case opts.RefreshSlow:
		nativeClaude := scan.DefaultSourcesWithOptions(false)
		fullClaude := scan.DefaultSourcesWithOptions(cfg.WSLScan != "off")
		nativeCodex := codex.NativeSources()
		fullCodex := codex.DefaultSourcesWithOptions(cfg.WSLScan != "off")
		fast = append(append([]string{}, nativeClaude...), nativeCodex...)
		slow = append(subtractCaseInsensitive(fullClaude, nativeClaude),
			subtractCaseInsensitive(fullCodex, nativeCodex)...)
		roots = map[string][]string{"claude": fullClaude, "codex": fullCodex}
	default:
		fast = append(scan.DefaultSourcesWithOptions(false), codex.NativeSources()...)
	}
	fast = append(fast, cfg.ExtraSources...)
	return fast, slow, roots
}

// adapterForPath classifies which adapter owns path by matching it against
// each registered adapter's resolved roots. Falls back to the claude
// adapter when roots is empty (no full pass has run yet on this Cache),
// matching v0.1 Step 1 behaviour exactly, since Claude was the only adapter
// before this step.
func adapterForPath(path string, roots map[string][]string) adapter.Adapter {
	lp := strings.ToLower(path)
	for _, a := range adapters {
		for _, r := range roots[a.Name()] {
			lr := strings.ToLower(r)
			if lp == lr || strings.HasPrefix(lp, lr+string(os.PathSeparator)) || strings.HasPrefix(lp, lr+"/") {
				return a
			}
		}
	}
	if len(roots) == 0 {
		return adapterFor("claude")
	}
	return nil
}

// subtractCaseInsensitive returns the entries of all that are not present in
// remove, comparing case-insensitively (Windows paths), preserving all's
// order.
func subtractCaseInsensitive(all, remove []string) []string {
	if len(remove) == 0 {
		return append([]string(nil), all...)
	}
	skip := make(map[string]bool, len(remove))
	for _, r := range remove {
		skip[strings.ToLower(r)] = true
	}
	var out []string
	for _, a := range all {
		if !skip[strings.ToLower(a)] {
			out = append(out, a)
		}
	}
	return out
}

// dedupeCaseInsensitive removes case-insensitive duplicate paths, keeping
// the first occurrence and otherwise preserving order, so a source and an
// extra_sources entry pointing at the same folder with different casing
// (both valid on Windows) are not walked twice.
func dedupeCaseInsensitive(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		k := strings.ToLower(s)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
	}
	return out
}

// filesUnderAny reports which of files sit under one of the given source
// roots, used to split the slow (WSL) tier's file list out of a full
// FindJSONL pass so it can be reused, unwalked, on the next fast-tier pass.
func filesUnderAny(files, roots []string) []string {
	if len(roots) == 0 {
		return nil
	}
	var out []string
	for _, f := range files {
		lf := strings.ToLower(f)
		for _, r := range roots {
			lr := strings.ToLower(r)
			if lf == lr || strings.HasPrefix(lf, lr+string(os.PathSeparator)) || strings.HasPrefix(lf, lr+"/") {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// toolCallsBackfillMetaKey gates the one-time full re-read Collect does the
// first time it runs against a store built before S2's tool_calls table
// existed. See Collect's own comment.
const toolCallsBackfillMetaKey = "tool_calls_backfilled_v1"


// Registered adapters. v0.1 Step 1 wired only Claude; Step 2 adds Codex
// alongside it without touching this loop's shape.
var adapters = []adapter.Adapter{claude.Adapter{}, codex.Adapter{}}

func adapterFor(name string) adapter.Adapter {
	for _, a := range adapters {
		if a.Name() == name {
			return a
		}
	}
	return nil
}

// ingest reads every new byte of every file in files through the claude
// adapter, upserting Events and advancing that file's cursor in c.Store. A
// file whose stored cursor already covers its current size and mtime is
// skipped without opening it. trustSlow files skip the stat entirely (an
// os.Stat over the WSL 9P file server is exactly the round trip the slow
// cadence exists to avoid), trusting the store's cursor outright the same
// way v0.0.1 trusted its in-memory cache entry outright for those files.
func (c *Cache) ingest(cfg *pricing.Config, files []string, trustSlow map[string]bool, forceFull bool, progress func(done, total int)) error {
	ingestStart := time.Now()
	var bytesRead int64
	var filesParsed int
	total := len(files)
	if progress != nil {
		progress(0, total)
	}
	// Snapshot once, not per file: RootsByAdapter does not change mid-pass,
	// and files can number in the thousands.
	roots := c.RootsSnapshot()
	// clientCache memoizes ClientFor per project path across this whole
	// ingest pass: ClientFor's remote match reads .git\config from disk
	// (gitRemoteURL), and every event from the same file (often thousands,
	// v0.1 scale) shares the same project path, so recomputing it per event
	// would mean the same file read over and over in a hot loop.
	clientCache := map[string]string{}
	clientFor := func(project string) string {
		if v, ok := clientCache[project]; ok {
			return v
		}
		v := cfg.ClientFor(project)
		clientCache[project] = v
		return v
	}
	for i, f := range files {
		a := adapterForPath(f, roots)
		if a == nil {
			if progress != nil {
				progress(i+1, total)
			}
			continue
		}
		offset, mtime, size, ok, err := c.Store.Cursor(f)
		if err != nil {
			return fmt.Errorf("dataset: cursor for %s: %w", f, err)
		}
		if forceFull {
			ok = false
		}
		if ok && trustSlow[f] {
			if progress != nil {
				progress(i+1, total)
			}
			continue
		}
		fi, statErr := os.Stat(f)
		if statErr != nil {
			if progress != nil {
				progress(i+1, total)
			}
			continue // listed a moment ago, gone now; same as v0.0.1's parse-miss-and-drop
		}
		if ok && !forceFull && mtime.Equal(fi.ModTime()) && size == fi.Size() && offset == fi.Size() {
			if progress != nil {
				progress(i+1, total)
			}
			continue // fully read already, nothing new
		}
		startOffset := offset
		if !ok || forceFull || (!mtime.Equal(fi.ModTime()) && fi.Size() < size) || offset > fi.Size() {
			// File shrank or its mtime moved backward relative to what the
			// cursor recorded: it was rewritten, not appended to. Or the
			// stored offset is already past the real file size (a cursor
			// corrupted by the pre-fix trailing-partial-line bug). Either
			// way, re-read from the start rather than trust a now-meaningless
			// offset.
			startOffset = 0
		}
		parseStart := time.Now()
		events, toolCalls, newOffset, err := a.Parse(f, startOffset)
		if err != nil {
			if progress != nil {
				progress(i+1, total)
			}
			continue
		}
		filesParsed++
		readBytes := newOffset - startOffset
		bytesRead += readBytes
		if readBytes > 10*1024*1024 {
			log.Printf("ingest: parsed %s, %d bytes in %v", f, readBytes, time.Since(parseStart))
		}
		// P6: the owner split, applied at ingest to each event's project
		// path so it lands in the store rather than being recomputed on
		// every read.
		for j := range events {
			events[j].Owner = cfg.OwnerFor(events[j].Project)
			events[j].Client = clientFor(events[j].Project)
		}
		// F1: commit in batches of 1,000 rather than one transaction for the
		// whole file, so a first pass over a large existing rollout (a
		// Codex session can grow past a gigabyte) doesn't hold one huge
		// transaction (and its rollback log) open for the whole read.
		const upsertBatch = 1000
		for start := 0; start < len(events); start += upsertBatch {
			end := start + upsertBatch
			if end > len(events) {
				end = len(events)
			}
			if err := c.Store.UpsertEvents(events[start:end]); err != nil {
				return fmt.Errorf("dataset: upsert events for %s: %w", f, err)
			}
		}
		for start := 0; start < len(toolCalls); start += upsertBatch {
			end := start + upsertBatch
			if end > len(toolCalls) {
				end = len(toolCalls)
			}
			if err := c.Store.UpsertToolCalls(toolCalls[start:end]); err != nil {
				return fmt.Errorf("dataset: upsert tool calls for %s: %w", f, err)
			}
		}
		if err := c.Store.SetCursor(f, newOffset, fi.ModTime(), fi.Size()); err != nil {
			return fmt.Errorf("dataset: set cursor for %s: %w", f, err)
		}
		// startOffset > 0: only a file that grew past a cursor counts as
		// live. A first read or a forced full re-read moves every file from
		// 0 at once and would crowd the tail's 50-file cap with history
		// (found by review, 2026-09-29).
		if fn := c.onAdvance.Load(); fn != nil && startOffset > 0 && newOffset > startOffset {
			(*fn)(f)
		}
		if progress != nil {
			progress(i+1, total)
		}
	}
	log.Printf("stage ingest: %v, %d/%d files parsed, %d bytes read", time.Since(ingestStart), filesParsed, total, bytesRead)
	return nil
}

// IngestFile ingests any new bytes of path (one trail file) into the store
// and advances its cursor, without touching any other file or rebuilding the
// payload. For internal/watch's live watcher: a file-change notification
// names one path, and re-running the whole Collect pipeline for it would
// mean re-listing every source on every keystroke of every open session.
// Requires RootsByAdapter to already be populated, by a prior full Collect
// or by SeedNativeRoots (cmd/burnmon calls the latter before the first
// Collect so the live watcher classifies Codex paths correctly from
// startup, v0.1.1 F2), so path classifies to the right adapter; falls back
// to the claude adapter otherwise, same as ingest does for any other path
// before either has run. Safe to call concurrently with a Collect on the
// same Cache: both go through the store's single-connection pool, and
// RootsByAdapter reads/writes are guarded by rootsMu.
func (c *Cache) IngestFile(cfg *pricing.Config, path string) error {
	return c.ingest(cfg, []string{path}, nil, false, nil)
}

// Collect resolves sources (auto-detecting when opts.Sources is empty, else
// using opts.Sources/opts.SlowSources as given), lists transcript files,
// ingests any bytes that are new or changed since the last call on this
// Cache's store, rebuilds sessions from the store's Events, aggregates the
// result and returns the dashboard payload for opts.Seat and opts.MonthsN.
//
// progress, if non-nil, is called once with (0, total) before ingest starts
// and again after every file with (doneSoFar, total).
func (c *Cache) Collect(cfg *pricing.Config, opts CollectOpts, progress func(done, total int)) (Payload, error) {
	if err := validateSeat(cfg, opts.Seat); err != nil {
		return Payload{}, err
	}

	// S2's tool_calls table did not exist before this build: every existing
	// session's transcripts must be re-read once from offset 0 so their tool
	// calls backfill into the store, gated by this meta flag so it only ever
	// happens on the first Collect after upgrading, not on every run.
	backfillDone, _, err := c.Store.Meta(toolCallsBackfillMetaKey)
	if err != nil {
		return Payload{}, fmt.Errorf("dataset: read %s: %w", toolCallsBackfillMetaKey, err)
	}
	backfilling := backfillDone != "1"
	if backfilling {
		opts.ForceFull = true
	}

	// WS2 follow-up (2026-09-29, item 0): undo any Codex ":0" collapse, so
	// its rollouts are re-read from byte 0 under the fixed key below. Run on
	// every Collect, not once behind a meta flag: an older binary sharing
	// this store (burnmon.exe built before the fix) would otherwise collapse
	// new turns again with nothing left to clean them. The query is one
	// indexed pass and a no-op once the store is clean.
	if nEvents, nCursors, err := c.Store.ResetCollapsedCodexSessions(); err != nil {
		return Payload{}, fmt.Errorf("dataset: reset collapsed codex sessions: %w", err)
	} else if nEvents > 0 || nCursors > 0 {
		log.Printf("codex collapse reset: removed %d collapsed event(s), %d cursor(s) reset for a full re-read", nEvents, nCursors)
	}

	// 2026-10-12: drop the rows the live watcher created by reading a local
	// Cowork session's audit.jsonl as a session of its own (key "audit",
	// "#udit" in BurnMon Dev). Every Collect, for the same reason as the
	// Codex reset above; a no-op once the store is clean.
	if nEvents, nCalls, nCursors, err := c.Store.PurgeAuditSessions(); err != nil {
		return Payload{}, fmt.Errorf("dataset: purge audit sessions: %w", err)
	} else if nEvents > 0 || nCalls > 0 || nCursors > 0 {
		log.Printf("audit purge: removed %d event(s), %d tool call(s), %d cursor(s) read from audit.jsonl", nEvents, nCalls, nCursors)
	}

	sourceListStart := time.Now()
	fast, slow, roots := resolveSources(cfg, opts)
	fast = dedupeCaseInsensitive(fast)
	slow = dedupeCaseInsensitive(slow)
	all := dedupeCaseInsensitive(append(append([]string{}, fast...), slow...))
	c.Sources = all
	if len(all) == 0 {
		return Payload{}, ErrNoSources
	}
	if roots != nil {
		c.rootsMu.Lock()
		c.RootsByAdapter = roots
		c.rootsMu.Unlock()
	}
	log.Printf("stage source listing: %v, %d source(s)", time.Since(sourceListStart), len(all))

	walkStart := time.Now()
	var files []string
	if opts.RefreshSlow {
		files = scan.FindJSONL(all)
		c.SlowFiles = filesUnderAny(files, slow)
		c.SlowScannedAt = time.Now()
		log.Printf("stage WSL walk (full FindJSONL): %v, %d files", time.Since(walkStart), len(files))
	} else {
		files = scan.FindJSONL(fast)
		files = append(files, c.SlowFiles...)
		log.Printf("stage source walk (fast tier): %v, %d files", time.Since(walkStart), len(files))
	}
	c.Files = files
	if len(files) == 0 {
		return Payload{}, ErrNoFiles
	}

	var trustSlow map[string]bool
	if !opts.RefreshSlow && len(c.SlowFiles) > 0 {
		trustSlow = make(map[string]bool, len(c.SlowFiles))
		for _, f := range c.SlowFiles {
			trustSlow[f] = true
		}
	}
	if err := c.ingest(cfg, files, trustSlow, opts.ForceFull, progress); err != nil {
		return Payload{}, err
	}
	if backfilling {
		if err := c.Store.SetMeta(toolCallsBackfillMetaKey, "1"); err != nil {
			return Payload{}, fmt.Errorf("dataset: set %s: %w", toolCallsBackfillMetaKey, err)
		}
	}
	// A transcript that no longer exists on disk (deleted, renamed, moved):
	// with keep_history (the default) its events stay and only its cursor is
	// forgotten, so a restored file is re-read from byte 0 and the upsert keys
	// keep its totals unchanged. Without it the events go too, so the session
	// stops contributing to every future report, as v0.0.1's gob cache pruned
	// on every save.
	deleteStart := time.Now()
	if cfg.KeepHistory {
		if _, err := c.Store.ForgetCursorsForOtherPaths(files); err != nil {
			return Payload{}, err
		}
		log.Printf("stage ForgetCursorsForOtherPaths: %v", time.Since(deleteStart))
	} else {
		if err := c.Store.DeleteEventsForOtherPaths(files); err != nil {
			return Payload{}, err
		}
		log.Printf("stage DeleteEventsForOtherPaths: %v", time.Since(deleteStart))
	}

	allEventsStart := time.Now()
	events, err := c.Store.AllEvents()
	if err != nil {
		return Payload{}, err
	}
	log.Printf("stage AllEvents: %v, %d rows", time.Since(allEventsStart), len(events))

	sessionsStart := time.Now()
	sessions := SessionsFromEvents(events, cfg)
	sessions, c.DroppedDuplicates = dedupSessions(sessions)
	log.Printf("stage SessionsFromEvents: %v, %d sessions (%d duplicate(s) dropped)",
		time.Since(sessionsStart), len(sessions), c.DroppedDuplicates)

	today := time.Now()
	cutoff := monthStart(today, max(0, opts.MonthsN-1))
	aggStart := time.Now()
	months, weeks, days, kept := agg.Build(sessions, cutoff)
	log.Printf("stage agg.Build: %v, %d months, %d weeks, %d days, %d sessions kept",
		time.Since(aggStart), len(months), len(weeks), len(days), len(kept))
	if len(kept) == 0 {
		return Payload{}, ErrNoSessions
	}
	buildPayloadStart := time.Now()
	payload := BuildPayload(cfg, opts.Seat, cutoff, today, months, weeks, days, kept)
	log.Printf("stage BuildPayload: %v", time.Since(buildPayloadStart))
	return payload, nil
}

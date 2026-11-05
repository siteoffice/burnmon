//go:build windows || darwin

// headline.go: the ticker/headline patch's own section 4 said the headline
// total should "visibly roll" every tick; in practice it only moved on
// vendorStrip's own 60s cache refresh (startSlowRefreshers, app.go), so
// most ticks painted the same number (2026-09-26 phase 5 fix). This adds
// whatever the shared tick's own already-fetched live.ChartWindow events
// carry past the cache's own GeneratedAt moment, so bdevSnapshotNow's
// headline field reflects real ingestion every tick without a second,
// full-store query.
package main

import (
	"time"

	"burnmon/internal/schema"
	"burnmon/internal/vendorstrip"
)

// eventTokens is the same token count internal/store.VendorStripTotals sums
// in SQL: fresh input plus cache write, cache read and output.
func eventTokens(e schema.Event) int64 {
	t := e.Input + e.Output
	if e.CacheWrite != nil {
		t += *e.CacheWrite
	}
	if e.CacheRead != nil {
		t += *e.CacheRead
	}
	return t
}

// headlineDayStart mirrors vendorstrip's own unexported dayStart (local
// midnight of now, WS3's "local time everywhere" - this used to be a UTC
// copy, left behind when vendorstrip switched, WS2 item 5): the same
// calendar boundary VendorStripTotals sums "today" against, so this stays
// in lockstep with vendorStrip.Total.Today.
func headlineDayStart(now time.Time) time.Time {
	now = now.In(time.Local)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
}

// headlineTodayTokens returns today's running total: vendorStrip's own
// cached Today figure, plus tokens from any event in events (the shared
// tick's own live.ChartWindow fetch, comfortably wider than the cache's 60s
// refresh cadence) that landed after the cache's GeneratedAt moment - so a
// turn ingested since the last cache refresh shows up immediately instead
// of waiting up to a minute for the next one. Filtering strictly on
// e.At.After(cacheTime) means an event the cache already counted (at or
// before that moment) is never added twice, regardless of how stale the
// cache is.
//
// A cacheGeneratedAt older than today's own start means the cache was
// built before the day rolled over (the app sat idle across midnight): the
// cached Today figure describes a day that has already ended, so it is
// dropped rather than carried forward, and today starts from whatever the
// fetched window alone can see. That is a real reset, not the "jump back"
// this function must otherwise avoid: normally, the returned total should
// never be smaller than the previous tick's, since each tick's cache base
// only grows and its delta only adds events after that same, unmoving
// cache moment.
//
// One gap this function alone cannot close (found by review, 2026-09-26):
// if vendor_strip's own 60s refresh (startSlowRefreshers, app.go) stalls
// for longer than live.ChartWindow - a sustained store error, not just a
// sleep/wake, which refreshes within one tick - cacheGeneratedAt falls
// behind events' own window start. Every fetched event is then "after
// cacheTime" and gets added on top of the stale base, but tokens from the
// gap between cacheTime and the window start are in neither, silently
// missing. Worse, as those already-counted events age out of the sliding
// window on later ticks, the sum they contributed shrinks, so the total
// would visibly dip. headlineTodayMonotonic below is the guard: call that,
// not this function directly, from bdevSnapshotNow.
func headlineTodayTokens(vendorTotal vendorstrip.Row, cacheGeneratedAt string, events []schema.Event, now time.Time) int64 {
	day := headlineDayStart(now)
	cacheTime, err := time.Parse(time.RFC3339, cacheGeneratedAt)
	cacheValid := err == nil && !cacheTime.Before(day)

	var base int64
	if cacheValid {
		base = vendorTotal.Today
	}
	var added int64
	for _, e := range events {
		if e.At.Before(day) {
			continue
		}
		if cacheValid && !e.At.After(cacheTime) {
			continue
		}
		added += eventTokens(e)
	}
	return base + added
}

// headlineTodayMonotonic wraps headlineTodayTokens with a per-day floor
// (a.headlineDay/a.headlineFloor, guarded by a.mu): the candidate value
// only ever holds level or rises within one calendar day, never dips, even
// during headlineTodayTokens' own documented gap (a vendor_strip refresh
// stalled past live.ChartWindow). A day boundary always resets the floor
// rather than clamping the new day's low starting total against the old
// day's high one - that reset is intentional, not the jump back this
// guards against.
func (a *app) headlineTodayMonotonic(vendorTotal vendorstrip.Row, cacheGeneratedAt string, events []schema.Event, now time.Time) int64 {
	candidate := headlineTodayTokens(vendorTotal, cacheGeneratedAt, events, now)
	day := headlineDayStart(now)

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.headlineDay.IsZero() || !a.headlineDay.Equal(day) {
		a.headlineDay = day
		a.headlineFloor = candidate
		return candidate
	}
	if candidate < a.headlineFloor {
		return a.headlineFloor
	}
	a.headlineFloor = candidate
	return candidate
}

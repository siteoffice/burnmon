package main

import (
	"log"
	"runtime"
	"sync/atomic"
	"time"

	"burnmon/internal/forecast"
	"burnmon/internal/history"
	"burnmon/internal/insight"
	"burnmon/internal/live"
	"burnmon/internal/pricing"
	"burnmon/internal/store"
	"burnmon/internal/vendorstrip"
)

// The page's live functions (bmLive, bmTurn, bmSessionInsight, bmHistory,
// bmVendorStrip, bmForecast), shared by both front ends: main.go binds them
// into the WebView2 window on Windows, browser.go serves them over a
// localhost-only HTTP bridge on macOS and Linux. Each takes its own config
// snapshot under a.mu, the same as the original inline bindings did.

func (a *app) cfgSnapshot() pricing.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

var bmLivePolls atomic.Uint64

// liveSnapshot is bmLive. F1: bmLive is polled every 2 seconds by the Now
// page. It used to call st.AllEvents() and re-group the whole table on
// every tick (measured at 1,886 MB, memory thrashing); EventsSince bounds
// the read to live.ChartWindow, and ApplySessionTotals fills in each running
// session's true lifetime totals from a small SQL aggregate instead of the
// full history. bucketSeconds (U5, v0.3): the monitor view's braille chart
// asks for finer columns; live.BuildSnapshot defaults to 60 when omitted.
func (a *app) liveSnapshot(st *store.Store, bucketSeconds ...int) (live.Snapshot, error) {
	callStart := time.Now()
	cfg := a.cfgSnapshot()
	now := time.Now()
	storeStart := time.Now()
	events, err := st.EventsSince(now.Add(-live.ChartWindow))
	storeWait := time.Since(storeStart)
	if err != nil {
		log.Printf("bound bmLive: %v total, %v waiting for the store (EventsSince error: %v)", time.Since(callStart), storeWait, err)
		return live.Snapshot{}, err
	}
	snap := live.BuildSnapshot(events, &cfg, now, bucketSeconds...)
	totalsStart := time.Now()
	if err := live.ApplySessionTotals(snap.Sessions, st, &cfg); err != nil {
		log.Println("bmLive: session totals:", err)
	}
	storeWait += time.Since(totalsStart)
	if n := bmLivePolls.Add(1); n%30 == 0 {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		log.Printf("debug: bmLive poll %d, HeapAlloc=%d bytes", n, mem.HeapAlloc)
	}
	if elapsed := time.Since(callStart); elapsed > 50*time.Millisecond {
		log.Printf("bound bmLive: %v total, %v waiting for the store", elapsed, storeWait)
	}
	return snap, nil
}

// turnDetail is bmTurn (I3): the Now page's "explain this spike" drawer.
// Re-reads sessionID's whole event history rather than bmLive's windowed
// one, so a drawer opened on a turn that has since scrolled out of the
// 30-minute chart still resolves. Errors are logged; the zero detail is
// still returned so the drawer can close its loading state.
func (a *app) turnDetail(st *store.Store, sessionID string, turn int) live.TurnDetail {
	cfg := a.cfgSnapshot()
	start := time.Now()
	detail, err := live.BuildTurnDetail(st, &cfg, sessionID, turn)
	if err != nil {
		log.Println("bmTurn:", sessionID, turn, ":", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		log.Printf("bound bmTurn: %v waiting for the store", elapsed)
	}
	return detail
}

// sessionInsight is bmSessionInsight (I3, Sessions tab findings). An error
// resolves as no findings rather than leaving the row pending.
func (a *app) sessionInsight(st *store.Store, sessionID string) []insight.Finding {
	cfg := a.cfgSnapshot()
	start := time.Now()
	findings, err := live.BuildSessionInsight(st, &cfg, sessionID)
	if err != nil {
		log.Println("bmSessionInsight:", sessionID, ":", err)
		findings = nil
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		log.Printf("bound bmSessionInsight: %v waiting for the store", elapsed)
	}
	return findings
}

// historyPayload is bmHistory (P2): queried on demand from the History tab's
// filter controls, so it re-reads the whole store on every call.
func (a *app) historyPayload(st *store.Store, f history.Filter) (history.Payload, error) {
	cfg := a.cfgSnapshot()
	start := time.Now()
	events, err := st.AllEvents()
	if err != nil {
		log.Println("bmHistory: AllEvents:", err)
		return history.Payload{}, err
	}
	payload := history.Build(events, &cfg, f)
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		log.Printf("bound bmHistory: %v waiting for the store, %d rows", elapsed, len(events))
	}
	return payload, nil
}

// vendorStrip is bmVendorStrip (P3), called from the Now page's own
// 1-minute timer.
func (a *app) vendorStrip(st *store.Store) (vendorstrip.Payload, error) {
	cfg := a.cfgSnapshot()
	start := time.Now()
	payload, err := vendorstrip.Build(st, &cfg, time.Now())
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		log.Printf("bound bmVendorStrip: %v total, %v waiting for the store", elapsed, elapsed)
	}
	return payload, err
}

// forecastPayload is bmForecast (F1), called from the Now page's own
// 1-minute timer; it also carries the scoring bookkeeping (EnsureScored).
func (a *app) forecastPayload(st *store.Store) (forecast.Payload, error) {
	cfg := a.cfgSnapshot()
	start := time.Now()
	payload, err := forecast.Build(st, &cfg, time.Now())
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		log.Printf("bound bmForecast: %v total, %v waiting for the store", elapsed, elapsed)
	}
	return payload, err
}

//go:build windows || darwin

// perf.go is WS2 alpha.7's measurement layer (spec
// 02_roadmap\2026-09-29_ws2_smooth_tick_and_system_layout.md, items 1 and 2):
// once a minute it logs how long bdevSnapshotNow took to build, how big its
// JSON was and what the page itself saw (round trip, dropped paints), and it
// logs every slow sample-loop pass and every gap over histGapThreshold in
// sysHistBuf as it happens. Everything here only ever writes to the app log;
// nothing reads it back.
package main

import (
	"encoding/json"
	"log"
	"sort"
	"sync"
	"time"
)

// snapshotPerf collects bdevSnapshotNow's own per-call timings for the
// current minute. measureSize is set by the minute logger and consumed by
// the next bdevSnapshotNow call, which then marshals its payload once to
// count bytes (outside the timed build), so the size costs one extra
// marshal per minute, not per tick.
type snapshotPerf struct {
	mu          sync.Mutex
	build       []time.Duration
	events      []time.Duration
	burn        []time.Duration
	totals      []time.Duration
	measureSize bool
	jsonBytes   int
	sysRows     int
	groupRows   int
}

func (p *snapshotPerf) record(build, events, burn, totals time.Duration, sysRows, groupRows int) (measure bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.build = append(p.build, build)
	p.events = append(p.events, events)
	p.burn = append(p.burn, burn)
	p.totals = append(p.totals, totals)
	p.sysRows, p.groupRows = sysRows, groupRows
	measure = p.measureSize
	p.measureSize = false
	return measure
}

func (p *snapshotPerf) setSize(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	p.mu.Lock()
	p.jsonBytes = len(b)
	p.mu.Unlock()
}

func pctl(ds []time.Duration, q float64) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	s := append([]time.Duration(nil), ds...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	i := int(q * float64(len(s)-1))
	return s[i]
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// startPerfLog logs one "perf snapshot:" line a minute and asks the next
// snapshot to measure its own JSON size.
func (p *snapshotPerf) startPerfLog() {
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for range t.C {
			p.mu.Lock()
			b, e, bu, to := p.build, p.events, p.burn, p.totals
			p.build, p.events, p.burn, p.totals = nil, nil, nil, nil
			size, sysRows, groupRows := p.jsonBytes, p.sysRows, p.groupRows
			p.measureSize = true
			p.mu.Unlock()
			log.Printf("perf snapshot: n=%d build p50=%.1fms p95=%.1fms max=%.1fms | events p50=%.1f p95=%.1f | burn p50=%.1f p95=%.1f | totals p50=%.1f p95=%.1f | json_bytes=%d sys_rows=%d group_rows=%d",
				len(b), ms(pctl(b, .5)), ms(pctl(b, .95)), ms(pctl(b, 1)),
				ms(pctl(e, .5)), ms(pctl(e, .95)), ms(pctl(bu, .5)), ms(pctl(bu, .95)),
				ms(pctl(to, .5)), ms(pctl(to, .95)), size, sysRows, groupRows)
		}
	}()
}

// pagePerfReport is what page.html sends once a minute through
// bdevPerfReport: its own view of the round trip and of paints it dropped.
type pagePerfReport struct {
	N         int     `json:"n"`
	P50       float64 `json:"p50"`
	P95       float64 `json:"p95"`
	Max       float64 `json:"max"`
	Late      int     `json:"late"`
	InFlight  int     `json:"in_flight"`
	Abandoned int     `json:"abandoned"`
	Paints    int     `json:"paints"`
	MaxStreak int     `json:"max_streak"`
	MaxGapMs  float64 `json:"max_paint_gap_ms"`
	HeapMB    float64 `json:"heap_mb"`
	SysRows   int     `json:"sys_rows"`
	GroupRows int     `json:"group_rows"`
	Hidden    bool    `json:"hidden"`
}

func logPagePerf(r pagePerfReport) {
	log.Printf("perf page: n=%d rt p50=%.0fms p95=%.0fms max=%.0fms | late=%d in_flight=%d abandoned=%d paints=%d max_streak=%d max_paint_gap=%.0fms | js_heap=%.1fMB page_sys_rows=%d page_group_rows=%d hidden=%v",
		r.N, r.P50, r.P95, r.Max, r.Late, r.InFlight, r.Abandoned, r.Paints, r.MaxStreak, r.MaxGapMs, r.HeapMB, r.SysRows, r.GroupRows, r.Hidden)
}

type clockPair struct {
	tick     time.Duration // GetTickCount64, includes sleep
	unbiased time.Duration // QueryUnbiasedInterruptTime, excludes sleep
}

func sleptBetween(a, b clockPair) time.Duration {
	d := (b.tick - a.tick) - (b.unbiased - a.unbiased)
	if d < 0 {
		return 0
	}
	return d
}

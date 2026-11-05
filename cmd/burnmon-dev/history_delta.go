//go:build windows || darwin

// history_delta.go is WS2 alpha.7 item 1 (spec
// 02_roadmap\2026-09-29_ws2_smooth_tick_and_system_layout.md): the render
// tick used to ship the whole of sysHistBuf (30 min) and groupsHistBuf (60
// min, one row per harness per second, about 25,000 rows at the end of the
// hour) on every 1s tick, so the payload, and the page's own JSON parse,
// grew for the first hour until the round trip no longer fit in 1.5 ticks
// and the page stopped painting. Now the page sends the newest sample time
// it already holds for each history and gets back only what came after it;
// it keeps its own ring and prunes it the same way these buffers are pruned.
package main

import (
	"time"

	"burnmon/internal/sysmon"
)

// snapshotCursor is bdevSnapshotNow's one argument: the newest Ts
// (UnixMilli) page.html already holds for each history, 0 when it holds
// none (first call, a reload, or just after leaving uicheck fake mode).
type snapshotCursor struct {
	SysSince    int64 `json:"sys_since"`
	GroupsSince int64 `json:"groups_since"`
	// Stages is true while the Station is open: only then does the tick
	// read the latest tool calls and fill each session's stage.
	Stages bool `json:"stages"`
}

// sysHistSince returns the part of buf the page still needs. full is true
// when the page must replace its ring instead of appending: no cursor, a
// cursor older than windowStart (everything it holds is out of the window
// anyway), or a cursor newer than the newest sample (this process restarted
// under a page that kept its state, or the clock went back). A full answer
// is buf trimmed to windowStart, exactly what the pre-alpha.7 snapshot sent
// every tick. Times compare at millisecond precision because that is all
// the page's own cursor carries (JS Date), so a sample is never sent twice.
func sysHistSince(buf []sysmon.Sample, cursorMs int64, windowStart time.Time) (out []sysmon.Sample, full bool) {
	if cursorMs <= 0 || len(buf) == 0 || cursorMs < windowStart.UnixMilli() || cursorMs > buf[len(buf)-1].Ts.UnixMilli() {
		out = make([]sysmon.Sample, 0, len(buf))
		for _, s := range buf {
			if !s.Ts.Before(windowStart) {
				out = append(out, s)
			}
		}
		return out, true
	}
	i := len(buf)
	for i > 0 && buf[i-1].Ts.UnixMilli() > cursorMs {
		i--
	}
	return append([]sysmon.Sample(nil), buf[i:]...), false
}

// groupsHistSince is sysHistSince for groupsHistBuf, served whole on a full
// answer (its own sysHistWindow is already what the page wants). A cursor
// older than the buffer's first row is full too: rows between the two were
// pruned here and the page must drop them as well.
func groupsHistSince(buf []sysmon.ProcessGroupSample, cursorMs int64) (out []sysmon.ProcessGroupSample, full bool) {
	if cursorMs <= 0 || len(buf) == 0 || cursorMs < buf[0].Ts.UnixMilli() || cursorMs > buf[len(buf)-1].Ts.UnixMilli() {
		return append([]sysmon.ProcessGroupSample(nil), buf...), true
	}
	i := len(buf)
	for i > 0 && buf[i-1].Ts.UnixMilli() > cursorMs {
		i--
	}
	return append([]sysmon.ProcessGroupSample(nil), buf[i:]...), false
}

// pruneFront drops buf's first n entries. A reslice alone keeps the dropped
// prefix alive in the backing array until append happens to grow it (spec
// item 1.5), so once the entries dropped since the last copy outnumber the
// ones still kept, the rest moves to a fresh slice and the old array can be
// collected. *dropped carries that count between calls.
func pruneFront[T any](buf []T, n int, dropped *int) []T {
	if n <= 0 {
		return buf
	}
	buf = buf[n:]
	*dropped += n
	if *dropped > len(buf) {
		buf = append(make([]T, 0, len(buf)+len(buf)/4+1), buf...)
		*dropped = 0
	}
	return buf
}

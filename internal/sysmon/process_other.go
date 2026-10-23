//go:build !windows

package sysmon

import (
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// This file is the pre-WS2-item-1 implementation, kept verbatim for the
// untested darwin/linux browser-mode build (B1, out of WS2's scope: that
// build never runs burnmon-dev.exe, only cmd/burnmon, which does not use
// ProcessSampler at all today, so this file exists only to keep the
// internal/sysmon package building on those platforms - see
// process_windows.go for the optimized Windows path this session added).
type trackedProc struct {
	p         *process.Process
	lastRead  uint64
	lastWrite uint64
	hasIO     bool
	// isNew is true only for the tick a process was first registered:
	// Percent(0) was only just primed a few lines above in that same Tick
	// call, so a read this same tick is over a near-zero interval and
	// gopsutil can report a wildly inflated percentage for it (found by
	// review, 2026-09-24). Cleared after being skipped once.
	isNew bool
}

// ProcessSampler holds per-process delta state (CPU% and IO rates both
// need two points) across ticks, matching perfadvisor's own registry
// pattern (internal/tui/sample.go's procReg). Not safe for concurrent use;
// call Tick from one goroutine.
type ProcessSampler struct {
	reg    map[int32]*trackedProc
	lastAt time.Time
}

func NewProcessSampler() *ProcessSampler {
	return &ProcessSampler{reg: map[int32]*trackedProc{}}
}

// Tick samples every running process, classifies each into a harness
// (Proc's own precedence: cmdline, then exact name, then its parent
// chain), and returns one ProcessGroupSample per harness with the CPU/RAM/
// IO summed across every process in it. HarnessOther is dropped: the
// process groups panel only ever shows named harnesses (design doc
// section 4). The first call after NewProcessSampler has zero IO rates,
// since a rate needs two points.
// Procs is Windows only for now (the R6 relation graph); here it is empty.
func (ps *ProcessSampler) Procs() ([]ProcRow, time.Time) { return nil, time.Time{} }

func (ps *ProcessSampler) Tick(copilotVSCodeConfigured bool) ([]ProcessGroupSample, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}

	seen := make(map[int32]bool, len(procs))
	infoByPID := make(map[int32]Proc, len(procs))
	for _, p := range procs {
		seen[p.Pid] = true
		name, err := p.Name()
		if err != nil || name == "" {
			continue
		}
		cmdline, _ := p.Cmdline()
		ppid, _ := p.Ppid()
		infoByPID[p.Pid] = Proc{PID: p.Pid, PPID: ppid, Name: name, Cmdline: cmdline}

		if _, ok := ps.reg[p.Pid]; !ok {
			tp := &trackedProc{p: p, isNew: true}
			_, _ = p.Percent(0) // prime the CPU delta
			if ioc, err := p.IOCounters(); err == nil && ioc != nil {
				tp.lastRead, tp.lastWrite, tp.hasIO = ioc.ReadBytes, ioc.WriteBytes, true
			}
			ps.reg[p.Pid] = tp
		}
	}
	for pid := range ps.reg {
		if !seen[pid] {
			delete(ps.reg, pid)
		}
	}

	harnessByPID := resolveHarnesses(infoByPID, copilotVSCodeConfigured)

	now := time.Now()
	elapsed := now.Sub(ps.lastAt).Seconds()
	if ps.lastAt.IsZero() || elapsed <= 0 {
		elapsed = 0
	}
	ps.lastAt = now

	sums := map[Harness]*ProcessGroupSample{}
	for pid, tp := range ps.reg {
		h := harnessByPID[pid]
		if h == "" || h == HarnessOther {
			continue
		}
		pct, err := tp.p.Percent(0)
		if err != nil {
			continue
		}
		if tp.isNew {
			pct = 0
			tp.isNew = false
		}
		var memMB float64
		if mi, err := tp.p.MemoryInfo(); err == nil && mi != nil {
			memMB = float64(mi.RSS) / (1 << 20)
		}
		var ioBps float64
		if tp.hasIO && elapsed > 0 {
			if ioc, err := tp.p.IOCounters(); err == nil && ioc != nil {
				ioBps = float64(deltaU64(ioc.ReadBytes, tp.lastRead)+deltaU64(ioc.WriteBytes, tp.lastWrite)) / elapsed
				tp.lastRead, tp.lastWrite = ioc.ReadBytes, ioc.WriteBytes
			}
		}

		s := sums[h]
		if s == nil {
			s = &ProcessGroupSample{Ts: now, Harness: h}
			sums[h] = s
		}
		s.CPUPct += pct
		s.MemMB += memMB
		s.IOBps += ioBps
	}

	out := make([]ProcessGroupSample, 0, len(sums))
	for _, s := range sums {
		out = append(out, *s)
	}
	return out, nil
}

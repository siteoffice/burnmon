package sysmon

import (
	"sort"
	"time"
)

// resolveHarnesses classifies every process in infoByPID via Classify,
// resolving parent-chain inheritance (Classify's own last resort) with
// memoized recursion so a long chain is only ever walked once per process.
// depth is capped so a cycle (a process somehow its own ancestor, or two
// processes pointing at each other) bottoms out at HarnessOther instead of
// looping forever; a real process tree never gets close to that depth.
//
// OS-specific in this package: NewProcessSampler/ProcessSampler/Tick live in
// process_windows.go (a single NtQuerySystemInformation snapshot per tick,
// WS2 item 1) and process_other.go (gopsutil per-process queries, unchanged,
// for the untested darwin/linux browser-mode build - this function is the
// only part shared by both).
func resolveHarnesses(infoByPID map[int32]Proc, copilotVSCodeConfigured bool) map[int32]Harness {
	const maxDepth = 32
	resolved := make(map[int32]Harness, len(infoByPID))

	var resolve func(pid int32, depth int) Harness
	resolve = func(pid int32, depth int) Harness {
		if h, ok := resolved[pid]; ok {
			return h
		}
		if depth > maxDepth {
			return HarnessOther
		}
		info, ok := infoByPID[pid]
		if !ok {
			return HarnessOther
		}
		h := Classify(info, copilotVSCodeConfigured, func(ppid int32) (Harness, bool) {
			if _, ok := infoByPID[ppid]; !ok {
				return "", false
			}
			// A parent that itself resolved to HarnessOther is not a real
			// ancestor classification to inherit: report "not found" so
			// Classify falls through to its own name-based fallback (e.g.
			// a plain node.exe under an unclassified shell still lands in
			// HarnessNodeUnclassified) instead of silently freezing at
			// Other before ever reaching that fallback (found by review,
			// 2026-09-24: nearly every process has *some* live parent, so
			// without this, unclassified processes almost never reached
			// their own bucket).
			if parentH := resolve(ppid, depth+1); parentH != HarnessOther {
				return parentH, true
			}
			return "", false
		})
		resolved[pid] = h
		return h
	}

	out := make(map[int32]Harness, len(infoByPID))
	for pid := range infoByPID {
		out[pid] = resolve(pid, 0)
	}
	return out
}

// procReading is one process's CPU, memory and IO rate for one tick, already
// measured by the platform sampler; sumProcessGroups only adds them up.
type procReading struct {
	PID                  int32
	CPUPct, MemMB, IOBps float64
}

// sumProcessGroups sums readings per harness, dropping processes that map to
// no harness or to HarnessOther. Output is sorted by Harness so the order is
// stable between ticks.
func sumProcessGroups(rs []procReading, harnessByPID map[int32]Harness, ts time.Time) []ProcessGroupSample {
	sums := map[Harness]*ProcessGroupSample{}
	for _, r := range rs {
		h := harnessByPID[r.PID]
		if h == "" || h == HarnessOther {
			continue
		}
		s := sums[h]
		if s == nil {
			s = &ProcessGroupSample{Ts: ts, Harness: h}
			sums[h] = s
		}
		s.CPUPct += r.CPUPct
		s.MemMB += r.MemMB
		s.IOBps += r.IOBps
	}
	out := make([]ProcessGroupSample, 0, len(sums))
	for _, s := range sums {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Harness < out[j].Harness })
	return out
}

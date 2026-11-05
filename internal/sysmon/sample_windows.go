//go:build windows

package sysmon

import (
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
)

// slowRefreshEvery ticks: readWifi (netsh) and the per-drive breakdown
// (disk.Partitions plus disk.Usage per volume, which can block for seconds
// against an unreachable mapped network drive) are both too costly, and in
// the per-drive case too risky, to run on every 2s sample (perfadvisor's own
// live TUI refreshes wifi every 4th tick of its own ~1.5s cadence, about 6s;
// this app samples every 2s, so every 5th tick is the same order of
// magnitude, about 10s). Tick 0 (the very first sample) always skips both,
// so the app's own "first system sample" startup mark is never delayed
// waiting on a shell-out or a stalled drive (found by review, 2026-09-25).
const slowRefreshEvery = 5

// Sampler holds the delta state one tick needs against the previous one:
// disk and network counters are cumulative since boot, so only their rate
// over the tick window is useful. Not safe for concurrent use; call Tick
// from one goroutine, matching perfadvisor's own sample() (internal/tui/
// sample.go), the source this file's approach is copied from (perfadvisor
// main 2ed8046). pdh holds one PDH query for the process's lifetime
// (pressure_windows.go), opened lazily on the first Tick.
type Sampler struct {
	prevIO  map[string]disk.IOCountersStat
	prevNet *gnet.IOCountersStat
	prevAt  time.Time
	pdh     pdhState

	// last is the phase timing of the most recent Tick (LastTiming).
	last TickTiming
	tick int

	// slow is the per-drive breakdown and wifi reading (section 6's
	// "disks"/"network" boxes), refreshed by its own goroutine
	// (runSlowRefresh), never inside Tick: WS2 alpha.7 item 2 found the
	// sample loop stalled for up to 3 minutes on 2026-09-29 while the rest
	// of the process kept running, and the only slow pass logged was inside
	// Tick, where disk.Usage (no timeout) and a netsh spawn used to run.
	// Tick asks for a refresh every slowRefreshEvery ticks and reads
	// whatever the last one found.
	slowReq chan struct{}
	// slowBusy counts refresh requests dropped in a row because the last
	// one is still running, so a refresh that hangs for good is logged
	// while it hangs, not only once it returns.
	slowBusy int
	slowMu  sync.Mutex
	slowOut struct {
		disks []DiskSample
		wifi  WifiSample
	}
}

func NewSampler() *Sampler { return &Sampler{} }

// runSlowRefresh serves Tick's refresh requests one at a time; a request
// that arrives while one is still running is dropped (slowReq holds one),
// so a stalled drive delays only the Disks/Wifi readout, never a sample.
// prevIO/prevAt are its own IOCounters snapshot and time, so per-drive
// rates cover its own interval.
func (s *Sampler) runSlowRefresh() {
	var prevIO map[string]disk.IOCountersStat
	var prevAt time.Time
	for range s.slowReq {
		start := time.Now()
		io, ioErr := disk.IOCounters()
		elapsed := 0.0
		if !prevAt.IsZero() {
			elapsed = start.Sub(prevAt).Seconds()
		}
		disks := collectDiskSamples(io, ioErr, prevIO, elapsed)
		if ioErr == nil {
			prevIO = io
		}
		prevAt = start
		drivesDur := time.Since(start)
		wifi := readWifi()
		wifiDur := time.Since(start) - drivesDur
		if total := time.Since(start); total > 2*time.Second {
			log.Printf("sysmon: slow drive/wifi refresh: %v (drives=%v wifi=%v)",
				total.Round(time.Millisecond), drivesDur.Round(time.Millisecond), wifiDur.Round(time.Millisecond))
		}
		s.slowMu.Lock()
		s.slowOut.disks, s.slowOut.wifi = disks, wifi
		s.slowMu.Unlock()
	}
}

// LastTiming is how long each phase of the most recent Tick took, for
// burnmon-dev's slow-pass log (WS2 alpha.7 item 2).
func (s *Sampler) LastTiming() TickTiming { return s.last }

// Tick takes one system-wide sample. The first call after NewSampler has
// zero disk/network rates, since a rate needs two points; every later call
// has a real one.
func (s *Sampler) Tick() (Sample, error) {
	now := time.Now()
	elapsed := now.Sub(s.prevAt).Seconds()
	if s.prevAt.IsZero() {
		elapsed = 0
	}

	var sm Sample
	sm.Ts = now
	var tm TickTiming
	mark := time.Now()
	lap := func(d *time.Duration) {
		t := time.Now()
		*d = t.Sub(mark)
		mark = t
	}

	if v, err := cpu.Percent(0, false); err == nil && len(v) > 0 {
		sm.CPUPct = v[0]
	}
	if v, err := cpu.Percent(0, true); err == nil {
		sm.Cores = v
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		sm.MemUsedMB = float64(vm.Used) / (1 << 20)
		sm.MemTotalMB = float64(vm.Total) / (1 << 20)
	}
	if sw, err := mem.SwapMemory(); err == nil {
		sm.SwapUsedMB = float64(sw.Used) / (1 << 20)
		sm.SwapTotalMB = float64(sw.Total) / (1 << 20)
	}
	lap(&tm.CPUMem)

	io, ioErr := disk.IOCounters()
	if ioErr == nil {
		if elapsed > 0 && s.prevIO != nil {
			var readB, writeB uint64
			for name, cur := range io {
				if old, ok := s.prevIO[name]; ok {
					readB += deltaU64(cur.ReadBytes, old.ReadBytes)
					writeB += deltaU64(cur.WriteBytes, old.WriteBytes)
				}
			}
			sm.DiskReadBps = float64(readB) / elapsed
			sm.DiskWriteBps = float64(writeB) / elapsed
		}
	}

	lap(&tm.DiskIO)

	// Per-drive breakdown and wifi: requested every slowRefreshEvery ticks
	// (see that constant's own comment), never on tick 0, and run by
	// runSlowRefresh, not here. Between refreshes, and while one is still
	// running, the samples carry whatever the last refresh found.
	if s.tick > 0 && s.tick%slowRefreshEvery == 0 {
		if s.slowReq == nil {
			s.slowReq = make(chan struct{}, 1)
			go s.runSlowRefresh()
		}
		select {
		case s.slowReq <- struct{}{}:
			s.slowBusy = 0
		default:
			s.slowBusy++
			if s.slowBusy%12 == 0 {
				log.Printf("sysmon: drive/wifi refresh still running, %d requests skipped", s.slowBusy)
			}
		}
	}
	s.slowMu.Lock()
	sm.Disks, sm.Wifi = s.slowOut.disks, s.slowOut.wifi
	s.slowMu.Unlock()
	s.tick++
	if ioErr == nil {
		s.prevIO = io
	}

	if nics, err := gnet.IOCounters(false); err == nil && len(nics) > 0 {
		cur := nics[0]
		if elapsed > 0 && s.prevNet != nil {
			sm.NetDownBps = float64(deltaU64(cur.BytesRecv, s.prevNet.BytesRecv)) / elapsed
			sm.NetUpBps = float64(deltaU64(cur.BytesSent, s.prevNet.BytesSent)) / elapsed
		}
		s.prevNet = &cur
	}

	lap(&tm.Net)
	pt := s.pdh.readTick()
	lap(&tm.PDH)
	if pt.ok {
		sm.CPUQueue = pt.cpuQueue
		sm.CPUPerfPct = pt.cpuPerfPct
		sm.DiskQueueLen = pt.diskQueue
		sm.DiskLatMs = pt.diskLatMs
		sm.HardFaults = pt.hardFaults
		sm.GPUPct = pt.gpuPct
	} else {
		sm.CPUQueue, sm.CPUPerfPct, sm.DiskQueueLen, sm.DiskLatMs, sm.HardFaults, sm.GPUPct = -1, -1, -1, -1, -1, -1
	}

	s.prevAt = now
	s.last = tm
	return sm, nil
}

// collectDiskSamples is the per-drive breakdown (design doc UI review patch,
// section 6's "disks" box: one bar per drive, free space, read and write
// rates), ported from perfadvisor's own sample() disk loop (internal/tui/
// sample.go). Only ever called from the slow-refresh branch of Tick above
// (disk.Partitions/disk.Usage can block for seconds against an unreachable
// mapped network drive, found by review, 2026-09-25), so io/ioErr and
// prevIO/elapsed here are that branch's own slower-cadence snapshot and
// interval, not the 2s tick's.
func collectDiskSamples(io map[string]disk.IOCountersStat, ioErr error, prevIO map[string]disk.IOCountersStat, elapsed float64) []DiskSample {
	var out []DiskSample
	sysDrive := strings.ToUpper(os.Getenv("SystemDrive"))
	parts, err := disk.Partitions(false)
	if err != nil {
		return out
	}
	for _, part := range parts {
		u, err := disk.Usage(part.Mountpoint)
		if err != nil || u.Total == 0 {
			continue
		}
		key := strings.ToUpper(strings.TrimRight(part.Mountpoint, `\/`))
		d := DiskSample{
			Mount: key, System: key == sysDrive,
			FreeGB: float64(u.Free) / (1 << 30), TotalGB: float64(u.Total) / (1 << 30),
			UsedPercent: u.UsedPercent,
		}
		if elapsed > 0 && prevIO != nil && ioErr == nil {
			for name, cur := range io {
				if !strings.EqualFold(strings.TrimRight(name, `\/`), key) {
					continue
				}
				if old, ok := prevIO[name]; ok {
					d.ReadBps = float64(deltaU64(cur.ReadBytes, old.ReadBytes)) / elapsed
					d.WriteBps = float64(deltaU64(cur.WriteBytes, old.WriteBytes)) / elapsed
				}
			}
		}
		out = append(out, d)
	}
	return out
}

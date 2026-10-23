package sysmon

import "time"

// Sample is one system-wide tick, persisted at 10s resolution in
// burnmon-dev.db's sysmon_samples table (design doc section 6). Live
// sampling itself runs every 2s in memory; only every 5th tick is stored.
type Sample struct {
	Ts           time.Time
	CPUPct       float64
	Cores        []float64 // per-core percent, in core order
	MemUsedMB    float64
	MemTotalMB   float64
	DiskReadBps  float64
	DiskWriteBps float64
	NetDownBps   float64
	NetUpBps     float64
	GPUPct       float64 // busiest GPU engine, Task Manager style

	// Pressure signals (phase 3), the same PDH counters perfadvisor's live
	// TUI reads (internal/tui/sysmetrics_windows.go), English counter names
	// so the tool works on any Windows display language. -1 means the
	// counter could not be read this tick, not that it was zero.
	CPUQueue     float64 // runnable threads waiting, System\Processor Queue Length
	CPUPerfPct   float64 // percent of base clock; <100 throttled, >100 turbo
	DiskQueueLen float64 // PhysicalDisk(_Total)\Avg. Disk Queue Length
	DiskLatMs    float64 // PhysicalDisk(_Total)\Avg. Disk sec/Transfer, in ms
	HardFaults   float64 // Memory\Pages Input/sec

	// UI review patch (2026-09-25), section 6: the perfadvisor-style system
	// panel's bottom row (memory/disks/network boxes) needs swap, a
	// per-drive breakdown and wifi, none of which the phase 1-3 Sample
	// carried (only the aggregate CPU/RAM/disk/net totals above). Ported
	// from perfadvisor's own live TUI (internal/tui/sample.go's swap and
	// per-disk loop, internal/tui/sysmetrics_windows.go's readWifi),
	// source commit perfadvisor main 2ed8046 (same commit design doc
	// section 7 already cites for the rest of this package).
	SwapUsedMB  float64
	SwapTotalMB float64
	Disks       []DiskSample
	Wifi        WifiSample
}

// DiskSample is one drive's usage and IO rate for one tick, the system
// panel's "disks" box (one bar per drive, free space, read and write
// rates).
type DiskSample struct {
	Mount       string
	System      bool // this machine's %SystemDrive%
	FreeGB      float64
	TotalGB     float64
	UsedPercent float64
	ReadBps     float64
	WriteBps    float64
}

// WifiSample is the system panel's network box (wifi name and signal bar).
// OK is false when this machine has no wifi adapter or netsh could not be
// read; Connected can be false with OK true (adapter present, not
// associated).
type WifiSample struct {
	OK        bool
	Connected bool
	SSID      string
	SignalPct int
}

// ProcessGroupSample is one harness's summed load at one tick, persisted in
// burnmon-dev.db's process_group_samples table so the per-harness
// sparklines and the heatmap (phase 3) read from stored history instead of
// a fresh process scan per chart render.
type ProcessGroupSample struct {
	Ts      time.Time
	Harness Harness
	CPUPct  float64
	MemMB   float64
	IOBps   float64
}

// ProcRow is one process from the last ProcessSampler.Tick, for the R6
// relation graph. Cmdline is for matching only; it can hold secrets and must
// not be written anywhere.
type ProcRow struct {
	PID, PPID  int32
	Name       string
	Cmdline    string
	CreateTime int64 // FILETIME, the process identity together with PID
	Harness    Harness
}

// TickTiming is how long each phase of one Sampler.Tick took (WS2 alpha.7
// item 2's slow-pass log). The drive and wifi refresh runs on its own
// goroutine (runSlowRefresh), so it has no phase here.
type TickTiming struct {
	CPUMem, DiskIO, Net, PDH time.Duration
}

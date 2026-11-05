package sysmon

import (
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

// gopsReading is one raw gopsutil snapshot. The json tags let tests load
// fixtures; the platform sampler (sample_darwin.go) fills it from live calls.
type gopsReading struct {
	At         time.Time                      `json:"at"`
	CPUTotal   []float64                      `json:"cpu_total"`
	CPUPerCore []float64                      `json:"cpu_per_core"`
	VMem       mem.VirtualMemoryStat          `json:"vmem"`
	Swap       mem.SwapMemoryStat             `json:"swap"`
	Usage      []disk.UsageStat               `json:"usage"`
	DiskIO     map[string]disk.IOCountersStat `json:"disk_io"`
	NetIO      []net.IOCountersStat           `json:"net_io"`
}

// keepMount is true for the root volume and external drives under /Volumes.
// APFS also lists /System/Volumes/*, /private/var/vm and the like, which are
// the same container or not user storage.
func keepMount(path string) bool {
	if path == "/" {
		return true
	}
	return strings.HasPrefix(path, "/Volumes/") && len(path) > len("/Volumes/")
}

// sampleFromGops turns two gopsutil readings into a Sample. Rates need prev
// and a positive time step, else they are 0; a counter that went backwards
// counts as 0 (deltaU64). Fields macOS has no documented source for are -1.
func sampleFromGops(prev *gopsReading, cur gopsReading) Sample {
	const mb, gb = 1 << 20, 1 << 30
	s := Sample{
		Ts: cur.At, Cores: cur.CPUPerCore,
		MemUsedMB: float64(cur.VMem.Used) / mb, MemTotalMB: float64(cur.VMem.Total) / mb,
		SwapUsedMB: float64(cur.Swap.Used) / mb, SwapTotalMB: float64(cur.Swap.Total) / mb,
		GPUPct: -1, CPUQueue: -1, CPUPerfPct: -1, DiskQueueLen: -1, DiskLatMs: -1, HardFaults: -1,
	}
	if len(cur.CPUTotal) > 0 {
		s.CPUPct = cur.CPUTotal[0]
	}
	for _, u := range cur.Usage {
		if !keepMount(u.Path) {
			continue
		}
		s.Disks = append(s.Disks, DiskSample{
			Mount: u.Path, System: u.Path == "/",
			FreeGB: float64(u.Free) / gb, TotalGB: float64(u.Total) / gb, UsedPercent: u.UsedPercent,
		})
	}
	if prev == nil {
		return s
	}
	dt := cur.At.Sub(prev.At).Seconds()
	if dt <= 0 {
		return s
	}
	var rd, wr uint64
	for name, c := range cur.DiskIO {
		if p, ok := prev.DiskIO[name]; ok {
			rd += deltaU64(c.ReadBytes, p.ReadBytes)
			wr += deltaU64(c.WriteBytes, p.WriteBytes)
		}
	}
	s.DiskReadBps, s.DiskWriteBps = float64(rd)/dt, float64(wr)/dt
	var down, up uint64
	for _, c := range cur.NetIO {
		for _, p := range prev.NetIO {
			if p.Name == c.Name {
				down += deltaU64(c.BytesRecv, p.BytesRecv)
				up += deltaU64(c.BytesSent, p.BytesSent)
			}
		}
	}
	s.NetDownBps, s.NetUpBps = float64(down)/dt, float64(up)/dt
	return s
}

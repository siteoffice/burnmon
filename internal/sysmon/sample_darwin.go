//go:build darwin

package sysmon

import (
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

// Sampler reads the macOS System zone through gopsutil only. prev holds the
// last reading for rate maths; only Tick touches it, from one goroutine.
type Sampler struct {
	prev *gopsReading
	last TickTiming
}

func NewSampler() *Sampler { return &Sampler{} }

// Tick takes one reading. A failing call leaves its part empty; only a
// failing CPU call returns an error.
func (s *Sampler) Tick() (Sample, error) {
	cur := gopsReading{At: time.Now()}
	var t TickTiming

	start := time.Now()
	total, err := cpu.Percent(0, false)
	if err != nil {
		return Sample{}, err
	}
	cur.CPUTotal = total
	cur.CPUPerCore, _ = cpu.Percent(0, true)
	if v, err := mem.VirtualMemory(); err == nil && v != nil {
		cur.VMem = *v
	}
	if sw, err := mem.SwapMemory(); err == nil && sw != nil {
		cur.Swap = *sw
	}
	t.CPUMem = time.Since(start)

	start = time.Now()
	if parts, err := disk.Partitions(false); err == nil {
		for _, p := range parts {
			if !keepMount(p.Mountpoint) {
				continue
			}
			if u, err := disk.Usage(p.Mountpoint); err == nil && u != nil {
				cur.Usage = append(cur.Usage, *u)
			}
		}
	}
	cur.DiskIO, _ = disk.IOCounters()
	t.DiskIO = time.Since(start)

	start = time.Now()
	cur.NetIO, _ = net.IOCounters(false)
	t.Net = time.Since(start)

	out := sampleFromGops(s.prev, cur)
	s.prev, s.last = &cur, t
	return out, nil
}

func (s *Sampler) LastTiming() TickTiming { return s.last }

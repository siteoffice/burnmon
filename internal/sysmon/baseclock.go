package sysmon

import (
	"sync"

	"github.com/shirou/gopsutil/v4/cpu"
)

// baseClockOnce/baseClockGHz cache cpu.Info()'s rated clock speed: a static
// property of the CPU, not a per-tick reading, read once (perfadvisor's own
// tui.go does the same, in its model's init rather than every sample()).
var (
	baseClockOnce sync.Once
	baseClockGHz  float64
)

// BaseClockGHz is the CPU box's "base 2.6 GHz" label (design doc UI review
// patch section 6). 0 when cpu.Info() could not read it.
func BaseClockGHz() float64 {
	baseClockOnce.Do(func() {
		if infos, err := cpu.Info(); err == nil && len(infos) > 0 {
			baseClockGHz = infos[0].Mhz / 1000
		}
	})
	return baseClockGHz
}

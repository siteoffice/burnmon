//go:build windows

package main

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Sleep and power readings for the sample-gap log (spec item 2). GetTickCount64
// counts time asleep, QueryUnbiasedInterruptTime does not, so the difference
// between their deltas across a gap is how long the machine slept in it.
var (
	kernel32                       = windows.NewLazySystemDLL("kernel32.dll")
	procGetTickCount64             = kernel32.NewProc("GetTickCount64")
	procQueryUnbiasedInterruptTime = kernel32.NewProc("QueryUnbiasedInterruptTime")
	procGetSystemPowerStatus       = kernel32.NewProc("GetSystemPowerStatus")
)

func readClocks() clockPair {
	t, _, _ := procGetTickCount64.Call()
	var u uint64
	procQueryUnbiasedInterruptTime.Call(uintptr(unsafe.Pointer(&u)))
	return clockPair{tick: time.Duration(t) * time.Millisecond, unbiased: time.Duration(u) * 100}
}

type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

// powerState is a short "ac=on saver=off" readout, "unknown" on failure.
func powerState() string {
	var s systemPowerStatus
	if r, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&s))); r == 0 {
		return "unknown"
	}
	ac := map[byte]string{0: "off", 1: "on"}[s.ACLineStatus]
	if ac == "" {
		ac = "unknown"
	}
	saver := "off"
	if s.SystemStatusFlag == 1 {
		saver = "on"
	}
	return "ac=" + ac + " saver=" + saver
}

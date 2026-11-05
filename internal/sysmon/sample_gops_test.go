package sysmon

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func loadGops(t *testing.T, name string) gopsReading {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var r gopsReading
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSampleFromGopsRates(t *testing.T) {
	t0, t1 := loadGops(t, "darwin_gops_t0.json"), loadGops(t, "darwin_gops_t1.json")
	s := sampleFromGops(&t0, t1)
	if !s.Ts.Equal(t1.At) {
		t.Errorf("Ts = %v, want %v", s.Ts, t1.At)
	}
	if s.CPUPct != 37.5 {
		t.Errorf("CPUPct = %v", s.CPUPct)
	}
	if !reflect.DeepEqual(s.Cores, []float64{50, 25, 40, 35}) {
		t.Errorf("Cores = %v", s.Cores)
	}
	checks := []struct {
		name      string
		got, want float64
	}{
		{"MemUsedMB", s.MemUsedMB, 8192}, {"MemTotalMB", s.MemTotalMB, 16384},
		{"SwapUsedMB", s.SwapUsedMB, 512}, {"SwapTotalMB", s.SwapTotalMB, 2048},
		{"DiskReadBps", s.DiskReadBps, 2000000}, {"DiskWriteBps", s.DiskWriteBps, 500000},
		{"NetDownBps", s.NetDownBps, 1500000}, {"NetUpBps", s.NetUpBps, 300000},
		{"GPUPct", s.GPUPct, -1}, {"CPUQueue", s.CPUQueue, -1}, {"CPUPerfPct", s.CPUPerfPct, -1},
		{"DiskQueueLen", s.DiskQueueLen, -1}, {"DiskLatMs", s.DiskLatMs, -1}, {"HardFaults", s.HardFaults, -1},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if s.Wifi != (WifiSample{}) {
		t.Errorf("Wifi = %+v, want zero", s.Wifi)
	}
}

func TestSampleFromGopsDisks(t *testing.T) {
	t0 := loadGops(t, "darwin_gops_t0.json")
	s := sampleFromGops(nil, t0)
	want := []DiskSample{
		{Mount: "/", System: true, FreeGB: 200, TotalGB: 500, UsedPercent: 60},
		{Mount: "/Volumes/External", FreeGB: 800, TotalGB: 1000, UsedPercent: 20},
	}
	if !reflect.DeepEqual(s.Disks, want) {
		t.Errorf("Disks = %+v, want %+v", s.Disks, want)
	}
}

func TestKeepMount(t *testing.T) {
	for p, want := range map[string]bool{
		"/": true, "/Volumes/External": true, "/System/Volumes/Data": false,
		"/System/Volumes/Preboot": false, "/private/var/vm": false, "/dev": false, "/Volumes/": false,
	} {
		if got := keepMount(p); got != want {
			t.Errorf("keepMount(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestSampleFromGopsFirstTick(t *testing.T) {
	t0 := loadGops(t, "darwin_gops_t0.json")
	s := sampleFromGops(nil, t0)
	if s.DiskReadBps != 0 || s.DiskWriteBps != 0 || s.NetDownBps != 0 || s.NetUpBps != 0 {
		t.Errorf("first tick rates not zero: %+v", s)
	}
}

func TestSampleFromGopsCounterReset(t *testing.T) {
	t1, t2 := loadGops(t, "darwin_gops_t1.json"), loadGops(t, "darwin_gops_t2_reset.json")
	s := sampleFromGops(&t1, t2)
	if s.DiskReadBps != 0 || s.DiskWriteBps != 0 {
		t.Errorf("reset gave read %v write %v, want 0", s.DiskReadBps, s.DiskWriteBps)
	}
}

func TestSampleFromGopsSameInstant(t *testing.T) {
	t1 := loadGops(t, "darwin_gops_t1.json")
	s := sampleFromGops(&t1, t1)
	for name, v := range map[string]float64{
		"DiskReadBps": s.DiskReadBps, "DiskWriteBps": s.DiskWriteBps,
		"NetDownBps": s.NetDownBps, "NetUpBps": s.NetUpBps,
	} {
		if v != 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("%s = %v, want 0", name, v)
		}
	}
}

func TestSampleFromGopsEmptyCPU(t *testing.T) {
	t0 := loadGops(t, "darwin_gops_t0.json")
	t0.CPUTotal = nil
	if s := sampleFromGops(nil, t0); s.CPUPct != 0 {
		t.Errorf("CPUPct = %v, want 0", s.CPUPct)
	}
}

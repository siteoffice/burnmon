package sysmon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSumProcessGroupsFixture(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "darwin_procs.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		PID     int32   `json:"pid"`
		PPID    int32   `json:"ppid"`
		Name    string  `json:"name"`
		Cmdline string  `json:"cmdline"`
		CPUPct  float64 `json:"cpu_pct"`
		MemMB   float64 `json:"mem_mb"`
		IOBps   float64 `json:"io_bps"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	info := map[int32]Proc{}
	var rs []procReading
	for _, r := range rows {
		info[r.PID] = Proc{PID: r.PID, PPID: r.PPID, Name: r.Name, Cmdline: r.Cmdline}
		rs = append(rs, procReading{PID: r.PID, CPUPct: r.CPUPct, MemMB: r.MemMB, IOBps: r.IOBps})
	}
	ts := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	got := sumProcessGroups(rs, resolveHarnesses(info, false), ts)

	want := map[Harness][3]float64{
		HarnessClaude: {16.0, 260, 1500},
		HarnessCodex:  {4.0, 30, 0},
		HarnessSelf:   {1.0, 80, 0},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d groups, want %d: %+v", len(got), len(want), got)
	}
	for _, g := range got {
		w, ok := want[g.Harness]
		if !ok {
			t.Errorf("unexpected group %q", g.Harness)
			continue
		}
		if g.CPUPct != w[0] || g.MemMB != w[1] || g.IOBps != w[2] {
			t.Errorf("%s = %v/%v/%v, want %v", g.Harness, g.CPUPct, g.MemMB, g.IOBps, w)
		}
		if !g.Ts.Equal(ts) {
			t.Errorf("%s Ts = %v", g.Harness, g.Ts)
		}
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Harness > got[i].Harness {
			t.Errorf("not sorted by Harness: %+v", got)
		}
	}
}

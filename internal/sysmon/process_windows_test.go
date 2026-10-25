//go:build windows

package sysmon

import (
	"os"
	"testing"
	"time"
)

// TestQuerySystemProcesses_FindsSelfWithSanePPID is this file's own
// correctness gate for systemProcessInfoT's hand-derived field layout: a
// wrong offset anywhere before InheritedFromUniqueProcessId would read
// garbage there, and garbage matching the real parent PID by chance is not
// a realistic risk (PIDs are small integers reused across reboots, but not
// within one test run). os.Getppid() is the independent ground truth this
// checks against, not another read of the same snapshot.
func TestQuerySystemProcesses_FindsSelfWithSanePPID(t *testing.T) {
	snaps, _, err := querySystemProcesses(nil)
	if err != nil {
		t.Fatalf("querySystemProcesses: %v", err)
	}
	pid := int32(os.Getpid())
	var found *procSnapshot
	for i := range snaps {
		if snaps[i].PID == pid {
			found = &snaps[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("own pid %d not found among %d processes", pid, len(snaps))
	}
	if wantPPID := int32(os.Getppid()); found.PPID != wantPPID {
		t.Fatalf("own PPID = %d, want %d (os.Getppid); systemProcessInfoT is likely misaligned", found.PPID, wantPPID)
	}
	if found.WorkingSetSize == 0 || found.WorkingSetSize > 8<<30 {
		t.Fatalf("own WorkingSetSize = %d bytes, not plausible", found.WorkingSetSize)
	}
	if found.ImageName == "" {
		t.Fatalf("own ImageName is empty")
	}
	// go test's own process is go.exe/go.test.exe depending on how it was
	// invoked, never blank and never containing a NUL or control character
	// that a byte-length/char-count mixup in decodeUnicodeString would
	// otherwise leave behind.
	for _, r := range found.ImageName {
		if r < 0x20 {
			t.Fatalf("own ImageName %q contains a control character; decodeUnicodeString likely double-counted UTF-16 code units as bytes", found.ImageName)
		}
	}
}

// TestProcessSampler_Tick_TwoTicksProduceRates confirms the exported Tick
// contract survived the rewrite: two Tick calls, a real sleep apart, run
// without error, and the registry keeps this process's own pid across both
// (isNew is cleared again within the very same Tick call that set it - the
// same "skip the first-ever percent, since a rate needs two points" gate
// the old gopsutil-based Tick also had - so it is not something a caller
// observes as still true from a second, later Tick call; this checks what
// a caller actually can observe instead: the cached cmdline survives).
func TestProcessSampler_Tick_TwoTicksProduceRates(t *testing.T) {
	ps := NewProcessSampler()
	if _, err := ps.Tick(false); err != nil {
		t.Fatalf("first Tick: %v", err)
	}
	pid := int32(os.Getpid())
	entry, ok := ps.reg[pid]
	if !ok {
		t.Fatalf("own pid %d not registered after first Tick", pid)
	}
	if entry.isNew {
		t.Fatalf("own entry.isNew = true after Tick returned, want false (cleared within the same Tick call)")
	}
	if entry.cmdline == "" {
		t.Fatalf("own entry.cmdline is empty; cmdlineOf should have cached it on the first Tick")
	}
	cachedCmdline := entry.cmdline

	time.Sleep(50 * time.Millisecond)
	if _, err := ps.Tick(false); err != nil {
		t.Fatalf("second Tick: %v", err)
	}
	entry, ok = ps.reg[pid]
	if !ok {
		t.Fatalf("own pid %d dropped from the registry after a second Tick", pid)
	}
	if entry.cmdline != cachedCmdline {
		t.Fatalf("own entry.cmdline changed between ticks (%q -> %q); should stay cached, not re-read", cachedCmdline, entry.cmdline)
	}
}

// TestProcessSampler_Tick_ReusesSnapshotBuffer is WS2 alpha.3 item 4's own
// correctness gate: querySystemProcesses used to allocate a fresh buffer
// (starting at 2MB) on every single call; now the sampler grows it once (if
// ever) and reuses the same backing array from then on, since Tick now runs
// once a second instead of once every three (item 3). Checks the actual
// backing array pointer survives two Tick calls, not just that ps.buf is
// non-nil (which a fresh allocation each time would also satisfy).
func TestProcessSampler_Tick_ReusesSnapshotBuffer(t *testing.T) {
	ps := NewProcessSampler()
	if _, err := ps.Tick(false); err != nil {
		t.Fatalf("first Tick: %v", err)
	}
	if len(ps.buf) == 0 {
		t.Fatalf("ps.buf is empty after first Tick; querySystemProcesses should have allocated it")
	}
	firstBufPtr := &ps.buf[0]

	if _, err := ps.Tick(false); err != nil {
		t.Fatalf("second Tick: %v", err)
	}
	if len(ps.buf) == 0 || &ps.buf[0] != firstBufPtr {
		t.Fatalf("ps.buf's backing array changed between ticks; item 4 expects the same buffer reused, not a fresh allocation each Tick (a real process-count change forcing growth mid-test would also trip this, but is not expected in this short a window)")
	}
}

// TestProcessSampler_Tick_DetectsPIDReuseViaCreateTime guards a regression
// a fresh review caught (2026-09-26): the registry is keyed only by pid,
// with no way to notice a pid disappearing and a different process
// reappearing at the same number within one 3s walk (Windows reuses pids
// aggressively) - the old gopsutil-based sampler re-read name/cmdline
// every tick, so this self-corrected within one tick; the new cached
// registry does not, unless it treats a changed CreateTime as a new
// process. Uses the real test process's own real pid and CreateTime (from
// a real Tick), then corrupts the cached entry to look like a different,
// already-exited process at the same pid, and confirms the next Tick
// re-detects and re-registers the real one. Confirmed to fail without the
// fix (`ps.reg`'s reuse check disabled): the corrupted CreateTime,
// name and cmdline all survived a second Tick untouched.
func TestProcessSampler_Tick_DetectsPIDReuseViaCreateTime(t *testing.T) {
	ps := NewProcessSampler()
	if _, err := ps.Tick(false); err != nil {
		t.Fatalf("first Tick: %v", err)
	}
	pid := int32(os.Getpid())
	entry, ok := ps.reg[pid]
	if !ok {
		t.Fatalf("own pid %d not registered after first Tick", pid)
	}
	realCreateTime := entry.createTime

	entry.createTime = realCreateTime - 1
	entry.name = "some-other-process.exe"
	entry.cmdline = "stale cmdline from a different, already-exited process"
	entry.isNew = false

	if _, err := ps.Tick(false); err != nil {
		t.Fatalf("second Tick: %v", err)
	}
	entry, ok = ps.reg[pid]
	if !ok {
		t.Fatalf("own pid %d dropped from the registry", pid)
	}
	if entry.createTime != realCreateTime {
		t.Fatalf("entry.createTime = %d, want %d (the real process's own, re-detected)", entry.createTime, realCreateTime)
	}
	if entry.name == "some-other-process.exe" {
		t.Fatalf("entry.name is still the stale value; pid-reuse re-registration did not replace it")
	}
	if entry.cmdline == "stale cmdline from a different, already-exited process" {
		t.Fatalf("entry.cmdline is still the stale value; pid-reuse re-registration did not re-fetch it")
	}
}

// R6: after a Tick, Procs lists every process with its parent, start time
// and harness, so the relation graph can build process trees.
func TestProcessSamplerProcsListsSelf(t *testing.T) {
	ps := NewProcessSampler()
	if _, err := ps.Tick(false); err != nil {
		t.Fatal(err)
	}
	rows, at := ps.Procs()
	if at.IsZero() {
		t.Fatal("Procs has no sample time after a Tick")
	}
	pid := int32(os.Getpid())
	for _, r := range rows {
		if r.PID == pid {
			if r.PPID != int32(os.Getppid()) || r.CreateTime == 0 || r.Name == "" {
				t.Fatalf("own row = %+v", r)
			}
			return
		}
	}
	t.Fatalf("own pid %d not in %d rows", pid, len(rows))
}

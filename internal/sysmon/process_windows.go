//go:build windows

// process_windows.go is WS2 item 1's "cheaper process sampler": the old
// implementation (process_other.go, kept for darwin/linux) called
// process.Processes() for the PID list and then, for every single one of
// those processes, several more gopsutil calls each tick (Name, Cmdline,
// Ppid, Percent, MemoryInfo, IOCounters) - each of those opens its own
// process handle, so a machine with several hundred processes paid for
// several hundred OpenProcess-and-query round trips every 3s walk. This
// file instead takes one NtQuerySystemInformation(SystemProcessInformation)
// snapshot per tick: one syscall returns CPU time, working set and IO
// counters for every process on the machine at once. A process's command
// line is not in that snapshot (only its bare image name is), so Classify
// still needs one gopsutil Cmdline() call per process - but only the first
// tick a pid is ever seen, cached in the registry until that pid exits
// (also item 1), not every tick for every already-known process.
package sysmon

import (
	"fmt"
	"time"
	"unsafe"

	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/windows"
)

var (
	ntdllProcSampler             = windows.NewLazySystemDLL("ntdll.dll")
	procNtQuerySystemInformation = ntdllProcSampler.NewProc("NtQuerySystemInformation")
)

const (
	systemProcessInformationClass = 5
	statusInfoLengthMismatch      = 0xC0000004
)

// unicodeStringW mirrors Win32's UNICODE_STRING: Length/MaximumLength count
// bytes, not UTF-16 code units, and Buffer is not NUL-terminated (Length is
// authoritative). Go's own struct layout inserts the same 4-byte pad before
// the 8-byte-aligned pointer on amd64 a C compiler would, so no explicit
// padding field is needed here.
type unicodeStringW struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

// systemProcessInfoT mirrors the well-known (if technically undocumented)
// SYSTEM_PROCESS_INFORMATION layout NtQuerySystemInformation's
// SystemProcessInformation class returns, one variable-length entry per
// process (a trailing SYSTEM_THREAD_INFORMATION array this file never
// reads follows each entry; NextEntryOffset is what lets a caller skip
// over it without knowing its exact size). Only the fields this sampler
// actually uses are named; every other field before/after them below still
// needs to be present so their neighbors land at the right offset, since
// Go computes each field's own alignment from its position in this struct
// exactly as a C compiler would from the equivalent order.
type systemProcessInfoT struct {
	NextEntryOffset              uint32
	NumberOfThreads              uint32
	WorkingSetPrivateSize        int64
	HardFaultCount               uint32
	NumberOfThreadsHighWatermark uint32
	CycleTime                    uint64
	CreateTime                   int64
	UserTime                     int64
	KernelTime                   int64
	ImageName                    unicodeStringW
	BasePriority                 int32
	UniqueProcessId              uintptr
	InheritedFromUniqueProcessId uintptr
	HandleCount                  uint32
	SessionId                    uint32
	UniqueProcessKey             uintptr
	PeakVirtualSize              uintptr
	VirtualSize                  uintptr
	PageFaultCount               uint32
	PeakWorkingSetSize           uintptr
	WorkingSetSize               uintptr
	QuotaPeakPagedPoolUsage      uintptr
	QuotaPagedPoolUsage          uintptr
	QuotaPeakNonPagedPoolUsage   uintptr
	QuotaNonPagedPoolUsage       uintptr
	PagefileUsage                uintptr
	PeakPagefileUsage            uintptr
	PrivatePageCount             uintptr
	ReadOperationCount           int64
	WriteOperationCount          int64
	OtherOperationCount          int64
	ReadTransferCount            int64
	WriteTransferCount           int64
	OtherTransferCount           int64
}

// procSnapshot is one process's plain-data fields, decoded out of the raw
// NtQuerySystemInformation buffer immediately (ImageName copied into a real
// Go string) rather than kept as pointers into it, so the buffer itself can
// be discarded the moment parsing finishes.
type procSnapshot struct {
	PID, PPID            int32
	ImageName            string
	CreateTime           int64
	KernelTime, UserTime int64
	WorkingSetSize       uint64
	ReadTransferCount    uint64
	WriteTransferCount   uint64
}

// querySystemProcesses takes one system-wide process snapshot via
// NtQuerySystemInformation into buf's own backing array (WS2 alpha.3 item 4:
// reused across ticks by the caller, not a fresh allocation every walk - this
// tick now runs once a second instead of once every three, so the old
// fresh-2MB-per-call pattern would have tripled its own churn), growing it
// (doubling, or to the kernel's own reported required size if larger) until
// the call succeeds. Returns the (possibly grown) buffer alongside the
// parsed snapshots so the caller keeps the grown capacity for its next call.
// A machine under this sampler's own real-world load (several hundred
// processes) has been seen needing a few hundred KB; buf starts well above
// that so the common case is one call, not two.
func querySystemProcesses(buf []byte) ([]procSnapshot, []byte, error) {
	if len(buf) == 0 {
		buf = make([]byte, 2<<20)
	}
	for attempt := 0; attempt < 8; attempt++ {
		var retLen uint32
		r1, _, _ := procNtQuerySystemInformation.Call(
			uintptr(systemProcessInformationClass),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
			uintptr(unsafe.Pointer(&retLen)),
		)
		status := uint32(r1)
		if status == statusInfoLengthMismatch {
			newSize := retLen
			if newSize <= uint32(len(buf)) {
				newSize = uint32(len(buf)) * 2
			}
			buf = make([]byte, newSize)
			continue
		}
		if status != 0 {
			return nil, buf, fmt.Errorf("NtQuerySystemInformation(SystemProcessInformation): status=0x%08x", status)
		}
		return parseProcessBuffer(buf), buf, nil
	}
	return nil, buf, fmt.Errorf("NtQuerySystemInformation(SystemProcessInformation): buffer still too small after %d growths", 8)
}

func parseProcessBuffer(buf []byte) []procSnapshot {
	var out []procSnapshot
	offset := uint32(0)
	headerSize := uint32(unsafe.Sizeof(systemProcessInfoT{}))
	for {
		if uint64(offset)+uint64(headerSize) > uint64(len(buf)) {
			break
		}
		entry := (*systemProcessInfoT)(unsafe.Pointer(&buf[offset]))
		pid := int32(entry.UniqueProcessId)
		if pid != 0 { // skip the Idle process; never a real, classifiable one
			out = append(out, procSnapshot{
				PID:                pid,
				PPID:               int32(entry.InheritedFromUniqueProcessId),
				ImageName:          decodeUnicodeString(buf, entry.ImageName),
				CreateTime:         entry.CreateTime,
				KernelTime:         entry.KernelTime,
				UserTime:           entry.UserTime,
				WorkingSetSize:     uint64(entry.WorkingSetSize),
				ReadTransferCount:  uint64(entry.ReadTransferCount),
				WriteTransferCount: uint64(entry.WriteTransferCount),
			})
		}
		if entry.NextEntryOffset == 0 {
			break
		}
		offset += entry.NextEntryOffset
	}
	return out
}

// decodeUnicodeString reads a UNICODE_STRING's UTF-16 buffer into a Go
// string. buf is passed in (rather than trusting s.Buffer alone) only so
// the bounds check below has something to check against; s.Buffer itself
// already points inside buf's own backing array (NtQuerySystemInformation
// wrote both the fixed-size entries and every entry's ImageName text into
// the one buffer the caller supplied).
func decodeUnicodeString(buf []byte, s unicodeStringW) string {
	if s.Buffer == nil || s.Length == 0 {
		return ""
	}
	n := int(s.Length / 2)
	u16 := unsafe.Slice(s.Buffer, n)
	return windows.UTF16ToString(u16)
}

// trackedProc holds one process's state across ticks: its cached
// classification inputs (cmdline is read once, the first tick this pid is
// seen, and never again - item 1) plus the previous tick's cumulative
// counters, needed to turn CPU time and IO bytes (both cumulative since
// process start) into a rate. createTime is the pid-reuse guard (found by
// review, 2026-09-26): a registry keyed only by pid, with no way to notice
// a pid disappearing and a different process reappearing at the same
// number within one 3s walk (Windows reuses pids aggressively), would
// otherwise keep serving a short-lived process's own cached name/cmdline
// to whatever unrelated process reused its pid for the rest of that new
// process's life. NtQuerySystemInformation's own CreateTime is a real
// process identity the old gopsutil-based sampler never needed (it
// re-read name/cmdline every tick, so a pid reuse self-corrected within
// one tick); Tick below re-caches when it changes.
type trackedProc struct {
	name, cmdline                 string
	ppid                          int32
	createTime                    int64
	lastKernelTime, lastUserTime  int64
	lastReadBytes, lastWriteBytes uint64
	isNew                         bool
}

// ProcessSampler holds per-process delta state (CPU% and IO rates both
// need two points) across ticks. Not safe for concurrent use; call Tick
// from one goroutine. buf is WS2 alpha.3 item 4's own reused snapshot
// buffer, grown (never shrunk) by querySystemProcesses as needed.
type ProcessSampler struct {
	reg    map[int32]*trackedProc
	lastAt time.Time
	buf    []byte
	rows   []ProcRow
}

// Procs returns every process from the last Tick and when it was sampled.
func (ps *ProcessSampler) Procs() ([]ProcRow, time.Time) { return ps.rows, ps.lastAt }

func NewProcessSampler() *ProcessSampler {
	return &ProcessSampler{reg: map[int32]*trackedProc{}}
}

// cmdlineOf is the one remaining per-process call: NtQuerySystemInformation's
// snapshot carries a process's bare image name but not its full command
// line, so Classify's cmdline-substring rules (e.g. "@anthropic-ai/claude
// -code") still need this - called only from Tick's own first-seen-pid
// branch below, never on every tick for a pid already in the registry.
func cmdlineOf(pid int32) string {
	p, err := process.NewProcess(pid)
	if err != nil {
		return ""
	}
	cmd, _ := p.Cmdline()
	return cmd
}

// Tick classifies every running process into a harness (Proc's own
// precedence: cmdline, then exact name, then its parent chain) from one
// system-wide snapshot, and returns one ProcessGroupSample per harness with
// the CPU/RAM/IO summed across every process in it. HarnessOther is
// dropped: the process groups panel only ever shows named harnesses. The
// first call after NewProcessSampler has zero CPU/IO rates, since a rate
// needs two points.
func (ps *ProcessSampler) Tick(copilotVSCodeConfigured bool) ([]ProcessGroupSample, error) {
	snaps, buf, err := querySystemProcesses(ps.buf)
	ps.buf = buf
	if err != nil {
		return nil, err
	}

	now := time.Now()
	elapsed := now.Sub(ps.lastAt).Seconds()
	if ps.lastAt.IsZero() || elapsed <= 0 {
		elapsed = 0
	}

	seen := make(map[int32]bool, len(snaps))
	infoByPID := make(map[int32]Proc, len(snaps))
	ramByPID := make(map[int32]float64, len(snaps))
	cpuPctByPID := make(map[int32]float64, len(snaps))
	ioBpsByPID := make(map[int32]float64, len(snaps))

	for _, s := range snaps {
		seen[s.PID] = true
		entry, ok := ps.reg[s.PID]
		// A pid already in the registry whose CreateTime has changed is not
		// the same process any more - Windows reused this pid for a new
		// one, possibly within the same 3s walk that never saw it absent
		// (found by review, 2026-09-26) - so this re-registers exactly as
		// the not-ok branch below does, rather than serving the old
		// process's cached name/cmdline to whatever now runs at this pid.
		if ok && entry.createTime != s.CreateTime {
			ok = false
		}
		if !ok {
			entry = &trackedProc{
				name:       s.ImageName,
				cmdline:    cmdlineOf(s.PID),
				createTime: s.CreateTime,
				isNew:      true,
			}
			ps.reg[s.PID] = entry
		}
		entry.ppid = s.PPID
		infoByPID[s.PID] = Proc{PID: s.PID, PPID: entry.ppid, Name: entry.name, Cmdline: entry.cmdline}

		ramByPID[s.PID] = float64(s.WorkingSetSize) / (1 << 20)
		if !entry.isNew && elapsed > 0 {
			cpuNs := deltaI64(s.KernelTime, entry.lastKernelTime) + deltaI64(s.UserTime, entry.lastUserTime)
			cpuPctByPID[s.PID] = float64(cpuNs) * 100e-9 / elapsed * 100
			ioBpsByPID[s.PID] = float64(deltaU64(s.ReadTransferCount, entry.lastReadBytes)+deltaU64(s.WriteTransferCount, entry.lastWriteBytes)) / elapsed
		}
		entry.lastKernelTime, entry.lastUserTime = s.KernelTime, s.UserTime
		entry.lastReadBytes, entry.lastWriteBytes = s.ReadTransferCount, s.WriteTransferCount
		entry.isNew = false
	}
	for pid := range ps.reg {
		if !seen[pid] {
			delete(ps.reg, pid)
		}
	}
	ps.lastAt = now

	harnessByPID := resolveHarnesses(infoByPID, copilotVSCodeConfigured)

	ps.rows = make([]ProcRow, 0, len(infoByPID))
	for pid, p := range infoByPID {
		ps.rows = append(ps.rows, ProcRow{PID: pid, PPID: p.PPID, Name: p.Name, Cmdline: p.Cmdline,
			CreateTime: ps.reg[pid].createTime, Harness: harnessByPID[pid]})
	}

	sums := map[Harness]*ProcessGroupSample{}
	for pid, h := range harnessByPID {
		if h == "" || h == HarnessOther {
			continue
		}
		s := sums[h]
		if s == nil {
			s = &ProcessGroupSample{Ts: now, Harness: h}
			sums[h] = s
		}
		s.CPUPct += cpuPctByPID[pid]
		s.MemMB += ramByPID[pid]
		s.IOBps += ioBpsByPID[pid]
	}

	out := make([]ProcessGroupSample, 0, len(sums))
	for _, s := range sums {
		out = append(out, *s)
	}
	return out, nil
}

// deltaI64 is deltaU64's signed counterpart, for the two cumulative
// CPU-time counters (KernelTime/UserTime): a process's own counters only
// ever increase, but a sampler restart or a counter this sampler has never
// seen before must still read as zero rather than a huge negative delta.
func deltaI64(cur, prev int64) int64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}

//go:build windows

package controls

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const maxCapture = 64 * 1024

// limitBuf keeps the first maxCapture bytes and drops the rest.
type limitBuf struct{ b bytes.Buffer }

func (l *limitBuf) Write(p []byte) (int, error) {
	if room := maxCapture - l.b.Len(); room > 0 {
		if len(p) < room {
			room = len(p)
		}
		l.b.Write(p[:room])
	}
	return len(p), nil
}

// ExecRunner runs the stored argv directly: no shell, hidden window, bounded
// output. A timeout ends only the child this runner started.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, argv []string, dir string) (int, string, error) {
	if len(argv) == 0 || argv[0] == "" {
		return -1, "", errors.New("empty command")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.WaitDelay = 2 * time.Second
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	var out, errb limitBuf
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	line := firstLine(out.b.String(), errb.b.String())
	if ctx.Err() != nil {
		return -1, line, ctx.Err()
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), line, nil
	}
	if err != nil {
		return -1, line, err
	}
	return 0, line, nil
}

// Start opens argv in its own visible console, does not wait, and releases it.
func (ExecRunner) Start(argv []string) error {
	if len(argv) == 0 || argv[0] == "" {
		return errors.New("empty command")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// Elevated reports whether this process runs with an elevated token.
func Elevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

func creationTime(pid uint32) (time.Time, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return time.Time{}, false
	}
	defer windows.CloseHandle(h)
	var c, e, k, u windows.Filetime
	if windows.GetProcessTimes(h, &c, &e, &k, &u) != nil {
		return time.Time{}, false
	}
	return time.Unix(0, c.Nanoseconds()), true
}

func parentMap() (map[uint32]uint32, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	m := map[uint32]uint32{}
	if err := windows.Process32First(snap, &pe); err != nil {
		return m, err
	}
	for {
		m[pe.ProcessID] = pe.ParentProcessID
		if windows.Process32Next(snap, &pe) != nil {
			break
		}
	}
	return m, nil
}

// CurrentSelf walks the ancestor chain. A parent counts only when it was
// created before its child, so a reused pid is not taken for a parent.
func CurrentSelf() (Self, error) {
	dir, err := exeDir()
	if err != nil {
		return Self{}, err
	}
	s := Self{PID: os.Getpid(), ExeDir: dir}
	parents, err := parentMap()
	if err != nil {
		return s, err
	}
	cur := uint32(s.PID)
	curTime, ok := creationTime(cur)
	if !ok {
		return s, nil
	}
	seen := map[uint32]bool{cur: true}
	for i := 0; i < 64; i++ {
		pp, has := parents[cur]
		if !has || pp == 0 || seen[pp] {
			break
		}
		pt, ok := creationTime(pp)
		if !ok || !pt.Before(curTime) {
			break
		}
		s.Ancestors = append(s.Ancestors, int(pp))
		seen[pp] = true
		cur, curTime = pp, pt
	}
	return s, nil
}

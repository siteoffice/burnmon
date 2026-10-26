//go:build windows

package controls

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
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
// output. A timeout ends only the child this runner started (there is no Job
// object, so a grandchild can survive; WaitDelay bounds the wait).
type ExecRunner struct{}

// validateArgv keeps "exact argv" independent of PATH, PATHEXT and shims:
// argv[0] must be an absolute path to an .exe.
func validateArgv(argv []string) error {
	if len(argv) == 0 || argv[0] == "" {
		return errors.New("empty command")
	}
	if !filepath.IsAbs(argv[0]) {
		return errors.New("the command must be an absolute path")
	}
	if !strings.HasSuffix(strings.ToLower(argv[0]), ".exe") {
		return errors.New("the command must be an .exe")
	}
	return nil
}

// classifyRun turns the outcome of cmd.Run into Run's result. killed is true
// only when this runner's cancel function really killed the process. A
// process that exited by itself keeps its real exit code even when the
// context expired a moment later.
func classifyRun(err error, killed bool, ctxErr error) (int, error) {
	if killed {
		if ctxErr == nil {
			ctxErr = context.Canceled
		}
		return -1, ctxErr
	}
	if err == nil || errors.Is(err, exec.ErrWaitDelay) {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return -1, err
}

func (ExecRunner) Run(ctx context.Context, argv []string, dir string) (int, string, error) {
	if err := validateArgv(argv); err != nil {
		return -1, "", err
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.WaitDelay = 2 * time.Second
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	var killed atomic.Bool
	cmd.Cancel = func() error {
		err := cmd.Process.Kill()
		if err == nil {
			killed.Store(true)
		}
		return err
	}
	var out, errb limitBuf
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	line := firstLine(out.b.String(), errb.b.String())
	code, cerr := classifyRun(err, killed.Load(), ctx.Err())
	return code, line, cerr
}

// Start opens argv in its own new console and does not wait. It calls
// CreateProcess directly: with CREATE_NEW_CONSOLE, no STARTF_USESTDHANDLES and
// handle inheritance off, the child gets real console stdio (os/exec would
// hand it NUL, leaving `claude attach` without a keyboard). The command line
// is the stored argv through CommandLine, no shell. The process and thread
// handles are closed at once.
func (ExecRunner) Start(argv []string) error {
	if err := validateArgv(argv); err != nil {
		return err
	}
	app, err := windows.UTF16PtrFromString(argv[0])
	if err != nil {
		return err
	}
	cl, err := windows.UTF16PtrFromString(CommandLine(argv))
	if err != nil {
		return err
	}
	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi windows.ProcessInformation
	if err := windows.CreateProcess(app, cl, nil, nil, false, windows.CREATE_NEW_CONSOLE, nil, nil, &si, &pi); err != nil {
		return err
	}
	_ = windows.CloseHandle(pi.Thread)
	_ = windows.CloseHandle(pi.Process)
	return nil
}

// Elevated reports whether this process runs with an elevated token. It
// fails closed: when the token cannot be queried the answer is true, so the
// controls stay off.
func Elevated() bool { return elevatedWith(queryElevation) }

func elevatedWith(query func() (bool, error)) bool {
	elevated, err := query()
	return err != nil || elevated
}

func queryElevation() (bool, error) {
	var elevation, ret uint32
	err := windows.GetTokenInformation(windows.GetCurrentProcessToken(), windows.TokenElevation,
		(*byte)(unsafe.Pointer(&elevation)), uint32(unsafe.Sizeof(elevation)), &ret)
	if err != nil {
		return false, err
	}
	if ret != uint32(unsafe.Sizeof(elevation)) {
		return false, errors.New("unexpected token elevation size")
	}
	return elevation != 0, nil
}

// resolveReal resolves links, junctions, subst and mapped drives, and expands
// 8.3 short names, for a path that exists. filepath.EvalSymlinks does not
// follow mount points (junctions) since Go 1.23, so the final path comes from
// an open handle: CreateFile with no access rights, then
// GetFinalPathNameByHandle. A mapped network drive comes back as a UNC path.
// Any failure returns false and the caller must fail closed.
func resolveReal(p string) (string, bool) {
	up, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return "", false
	}
	h, err := windows.CreateFile(up, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(h)
	// FILE_NAME_NORMALIZED (0x0) | VOLUME_NAME_DOS (0x0): the Win32 values,
	// which x/sys/windows does not export.
	const flags uint32 = 0x0 | 0x0
	n, err := windows.GetFinalPathNameByHandle(h, nil, 0, flags)
	if err != nil || n == 0 {
		return "", false
	}
	buf := make([]uint16, n+1)
	n2, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), flags)
	if err != nil || n2 == 0 || int(n2) >= len(buf) {
		return "", false
	}
	r := windows.UTF16ToString(buf[:n2])
	switch {
	case hasPrefixFold(r, `\\?\UNC\`):
		r = `\\` + r[8:]
	case hasPrefixFold(r, `\\?\`):
		r = r[4:]
	}
	if up, err := windows.UTF16PtrFromString(r); err == nil {
		if n, err := windows.GetLongPathName(up, nil, 0); err == nil && n > 0 {
			buf := make([]uint16, n)
			if n2, err := windows.GetLongPathName(up, &buf[0], n); err == nil && n2 > 0 && n2 <= n {
				r = windows.UTF16ToString(buf[:n2])
			}
		}
	}
	return r, true
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

// procInfo is one row of the toolhelp process table.
type procInfo struct {
	Parent uint32
	Exe    string
}

func parentMap() (map[uint32]procInfo, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	m := map[uint32]procInfo{}
	if err := windows.Process32First(snap, &pe); err != nil {
		return nil, err
	}
	for {
		m[pe.ProcessID] = procInfo{pe.ParentProcessID, windows.UTF16ToString(pe.ExeFile[:])}
		if err := windows.Process32Next(snap, &pe); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				return m, nil
			}
			return nil, err
		}
	}
}

const maxChain = 64

// rootProcs are the login and system processes whose own parent has always
// exited. Reaching one ends the walk normally.
var rootProcs = []string{"explorer.exe", "services.exe", "wininit.exe", "winlogon.exe", "smss.exe", "csrss.exe", "system", "system idle process"}

func isRootProc(exe string) bool {
	for _, r := range rootProcs {
		if strings.EqualFold(exe, r) {
			return true
		}
	}
	return false
}

// walkChain follows parent links from pid through the process table.
//
// Decision for exited parents: a parent that has exited is not in the
// toolhelp table, so its own parent cannot be read and the walk cannot go on.
// The dead parent's pid is still recorded (a reused pid then refuses, which is
// the safe direction), and the result is ChainComplete false and unverified.
// A parent that is alive but was created after its child has a reused pid: it
// is recorded and the walk continues through the table, marked unverified. A
// parent whose start time cannot be read is recorded and marked unverified.
// The walk ends complete at parent pid 0 or at a root process (explorer,
// services, ...), and ends incomplete on a loop or after maxChain steps.
func walkChain(pid uint32, procs map[uint32]procInfo, created func(uint32) (time.Time, bool)) (anc []int, complete, verified bool) {
	curT, ok := created(pid)
	if !ok {
		return nil, false, false
	}
	verified = true
	cur := pid
	seen := map[uint32]bool{cur: true}
	for i := 0; i < maxChain; i++ {
		info, ok := procs[cur]
		if !ok {
			return anc, false, false
		}
		if info.Parent == 0 || isRootProc(info.Exe) {
			return anc, true, verified
		}
		pp := info.Parent
		if seen[pp] {
			return anc, false, false
		}
		seen[pp] = true
		anc = append(anc, int(pp))
		if _, alive := procs[pp]; !alive {
			return anc, false, false
		}
		pt, tok := created(pp)
		switch {
		case !tok:
			verified = false
		case !pt.Before(curT):
			verified = false
			curT = pt
		default:
			curT = pt
		}
		cur = pp
	}
	return anc, false, verified
}

// selfEnv holds the lookups CurrentSelf needs, so tests can inject failures.
type selfEnv struct {
	pid     int
	exeDir  func() (string, error)
	procs   func() (map[uint32]procInfo, error)
	created func(uint32) (time.Time, bool)
}

func currentSelf(env selfEnv) (Self, error) {
	dir, err := env.exeDir()
	if err != nil {
		return Self{}, err
	}
	procs, err := env.procs()
	if err != nil {
		return Self{}, fmt.Errorf("process table: %w", err)
	}
	if _, ok := procs[uint32(env.pid)]; !ok {
		return Self{}, errors.New("own process is not in the process table")
	}
	if _, ok := env.created(uint32(env.pid)); !ok {
		return Self{}, errors.New("cannot read own start time")
	}
	anc, complete, verified := walkChain(uint32(env.pid), procs, env.created)
	s := Self{PID: env.pid, Ancestors: anc, ExeDir: dir, ChainComplete: complete, ChainVerified: verified}
	if !complete {
		return s, errors.New("ancestor chain incomplete")
	}
	if !verified {
		return s, errors.New("ancestor chain unverified")
	}
	return s, nil
}

// CurrentSelf walks the ancestor chain. It fails closed: any lookup that
// fails, and any chain that is incomplete or unverified, returns a non-nil
// error. The Self returned with an error must not be used. The Task 6 service
// must treat any CurrentSelf error as "controls off".
func CurrentSelf() (Self, error) {
	return currentSelf(selfEnv{pid: os.Getpid(), exeDir: exeDir, procs: parentMap, created: creationTime})
}

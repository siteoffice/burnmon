package controls

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// controlsGOOS is the system the controls decide for. Tests set it to reach the
// other system's branch; never patch os.name or runtime.GOOS for that.
var controlsGOOS = runtime.GOOS

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

// ExecRunner runs the stored argv directly: no shell, no console window on
// Windows, bounded output. A timeout ends only the child this runner started
// (there is no Job object, so a grandchild can survive; WaitDelay bounds the
// wait). The same runner serves Windows and the Mac; only Start differs.
type ExecRunner struct{}

// validateArgv keeps "exact argv" independent of PATH and shims: argv[0] must
// be an absolute path. On Windows it must also be an .exe (PATHEXT and .cmd
// shims would otherwise run).
func validateArgv(argv []string) error {
	if len(argv) == 0 || argv[0] == "" {
		return errors.New("empty command")
	}
	if !filepath.IsAbs(argv[0]) {
		return errors.New("the command must be an absolute path")
	}
	if controlsGOOS == "windows" && !strings.HasSuffix(strings.ToLower(argv[0]), ".exe") {
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
	return execRun(ctx, argv, dir, false)
}

// RunLast is Run for the Close control: on a nonzero exit the line is the last
// stderr line (a failing Python script prints its reason last), cut to 300 runes.
func (ExecRunner) RunLast(ctx context.Context, argv []string, dir string) (int, string, error) {
	return execRun(ctx, argv, dir, true)
}

func execRun(ctx context.Context, argv []string, dir string, last bool) (int, string, error) {
	if err := validateArgv(argv); err != nil {
		return -1, "", err
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.WaitDelay = 2 * time.Second
	consoleAttr(cmd)
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
	code, cerr := classifyRun(err, killed.Load(), ctx.Err())
	line := firstLine(out.b.String(), errb.b.String())
	if code != 0 && last {
		// A failing script prints its reason last (a Python traceback ends
		// with the error line), so a nonzero exit prefers the last stderr line.
		if l := lastLine(errb.b.String(), closeReasonMax); l != "" {
			line = l
		}
	}
	return code, line, cerr
}

// rootElevated is the root check off Windows as a function of the effective
// user id. 0 is root.
func rootElevated(euid int) bool { return euid == 0 }

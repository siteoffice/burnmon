//go:build windows

package controls

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestValidateArgv(t *testing.T) {
	if err := validateArgv([]string{`C:\Tools\bin.exe`, "x"}); err != nil {
		t.Fatalf("absolute .exe refused: %v", err)
	}
	if err := validateArgv([]string{`C:\Tools\RUN.EXE`}); err != nil {
		t.Fatalf("upper-case .EXE refused: %v", err)
	}
	for _, bad := range [][]string{nil, {""}, {"claude"}, {"claude.exe"}, {`.\claude.exe`}, {`C:\tools\claude.cmd`}, {`C:\tools\claude.exe.bat`}, {`C:\tools\claude`}, {`\tools\claude.exe`}, {`C:tools\claude.exe`}} {
		if err := validateArgv(bad); err == nil {
			t.Errorf("validateArgv(%q) = nil, want refusal", bad)
		}
	}
}

func TestRunAndStartRefuseUnsafeArgv0(t *testing.T) {
	for _, argv := range [][]string{{"claude"}, {`.\claude.exe`}, {`C:\tools\claude.cmd`}} {
		if _, _, err := (ExecRunner{}).Run(context.Background(), argv, ""); err == nil {
			t.Errorf("Run(%q) must refuse", argv)
		}
		if err := (ExecRunner{}).Start(argv); err == nil {
			t.Errorf("Start(%q) must refuse", argv)
		}
	}
}

func TestClassifyRun(t *testing.T) {
	c := exec.Command(os.Args[0])
	c.Env = append(os.Environ(), "BURNMON_CONTROLS_HELPER=print7")
	hideWindow(c)
	err := c.Run()
	if err == nil {
		t.Fatal("helper should exit 7")
	}
	// The process exited by itself, then the context expired: real code.
	if code, e := classifyRun(err, false, context.DeadlineExceeded); code != 7 || e != nil {
		t.Errorf("exited before expiry: %d %v", code, e)
	}
	if code, e := classifyRun(nil, false, context.DeadlineExceeded); code != 0 || e != nil {
		t.Errorf("success before expiry: %d %v", code, e)
	}
	// We killed it: -1 and the context error.
	if code, e := classifyRun(err, true, context.DeadlineExceeded); code != -1 || !errors.Is(e, context.DeadlineExceeded) {
		t.Errorf("killed: %d %v", code, e)
	}
	if code, e := classifyRun(errors.New("start failed"), false, nil); code != -1 || e == nil {
		t.Errorf("start failure: %d %v", code, e)
	}
}

type mark struct {
	PID           int
	Args          []string
	StdinConsole  bool
	StdoutConsole bool
	ConsoleWindow bool
}

func TestStartGivesARealConsoleAndExactArgv(t *testing.T) {
	exe := os.Args[0]
	if !filepath.IsAbs(exe) {
		t.Skip("test binary path is not absolute")
	}
	markFile := filepath.Join(t.TempDir(), "mark.json")
	t.Setenv("BURNMON_CONTROLS_HELPER", "mark")
	t.Setenv("BURNMON_CONTROLS_MARKFILE", markFile)
	args := []string{`a b`, `c"d`, ``, `e\`, `f\\"g h`, `ünï`}
	if err := (ExecRunner{}).Start(append([]string{exe}, args...)); err != nil {
		t.Fatal(err)
	}
	var m mark
	deadline := time.Now().Add(30 * time.Second)
	for {
		b, err := os.ReadFile(markFile)
		if err == nil && json.Unmarshal(b, &m) == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the started helper never wrote its marker")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if m.PID == 0 || m.PID == os.Getpid() {
		t.Fatalf("bad helper pid %d", m.PID)
	}
	if !reflect.DeepEqual(m.Args, args) {
		t.Errorf("argv not exact:\n got %q\nwant %q", m.Args, args)
	}
	if !m.StdinConsole || !m.StdoutConsole {
		t.Errorf("the helper did not get console stdio: %+v", m)
	}
	t.Logf("console window handle present: %v", m.ConsoleWindow)
	gone := time.Now().Add(10 * time.Second)
	for !processGone(m.PID) && time.Now().Before(gone) {
		time.Sleep(50 * time.Millisecond)
	}
	if !processGone(m.PID) {
		t.Fatalf("helper %d still running", m.PID)
	}
}

// markHelper runs inside the started child and exits at once.
func markHelper() {
	m := mark{PID: os.Getpid(), Args: os.Args[1:]}
	var mode uint32
	m.StdinConsole = windows.GetConsoleMode(windows.Handle(os.Stdin.Fd()), &mode) == nil
	m.StdoutConsole = windows.GetConsoleMode(windows.Handle(os.Stdout.Fd()), &mode) == nil
	r, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow").Call()
	m.ConsoleWindow = r != 0
	b, _ := json.Marshal(m)
	f := os.Getenv("BURNMON_CONTROLS_MARKFILE")
	_ = os.WriteFile(f+".tmp", b, 0o600)
	_ = os.Rename(f+".tmp", f)
	os.Exit(0)
}

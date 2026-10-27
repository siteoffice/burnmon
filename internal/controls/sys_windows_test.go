//go:build windows

package controls

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// The test binary re-runs itself as a harmless helper, selected by an env var.
func TestMain(m *testing.M) {
	switch os.Getenv("BURNMON_CONTROLS_HELPER") {
	case "self":
		s, err := CurrentSelf()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		b, _ := json.Marshal(s)
		fmt.Println(string(b))
		os.Exit(0)
	case "mark":
		markHelper()
	case "sleep":
		_ = os.WriteFile(os.Getenv("BURNMON_CONTROLS_PIDFILE"), []byte(strconv.Itoa(os.Getpid())), 0o600)
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "print7":
		fmt.Println("first line")
		fmt.Println("second line")
		os.Exit(7)
	case "stderronly":
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "only on stderr")
		os.Exit(1)
	case "pytrace":
		fmt.Println("progress line")
		fmt.Fprintln(os.Stderr, "Traceback (most recent call last):")
		fmt.Fprintln(os.Stderr, "ValueError: example failure")
		fmt.Fprintln(os.Stderr, "")
		os.Exit(1)
	case "long":
		fmt.Println(strings.Repeat("é", 500))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func helperCmd(mode string) *exec.Cmd {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "BURNMON_CONTROLS_HELPER="+mode)
	hideWindow(cmd)
	return cmd
}

func TestCurrentSelfSeesParent(t *testing.T) {
	out, err := helperCmd("self").Output()
	if err != nil {
		t.Fatalf("helper: %v", err)
	}
	var s Self
	if err := json.Unmarshal(out, &s); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	found := false
	for _, a := range s.Ancestors {
		if a == os.Getpid() {
			found = true
		}
	}
	if !found {
		t.Fatalf("test pid %d not in ancestors %v", os.Getpid(), s.Ancestors)
	}
	if s.PID == os.Getpid() || s.PID == 0 {
		t.Fatalf("helper pid %d", s.PID)
	}
	exe, _ := os.Executable()
	exe, _ = filepath.EvalSymlinks(exe)
	if !strings.EqualFold(s.ExeDir, filepath.Dir(exe)) {
		t.Fatalf("ExeDir %q, want %q", s.ExeDir, filepath.Dir(exe))
	}
}

func TestCurrentSelfOwnPid(t *testing.T) {
	s, err := CurrentSelf()
	if err != nil || s.PID != os.Getpid() {
		t.Fatalf("%+v %v", s, err)
	}
	for _, a := range s.Ancestors {
		if a == s.PID {
			t.Fatalf("own pid in ancestors")
		}
	}
}

func processGone(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return true
	}
	defer windows.CloseHandle(h)
	var code uint32
	if windows.GetExitCodeProcess(h, &code) != nil {
		return false
	}
	return code != 259 // STILL_ACTIVE
}

func TestExecRunnerTimeoutStopsOwnChild(t *testing.T) {
	t.Setenv("BURNMON_CONTROLS_HELPER", "sleep")
	pidFile := filepath.Join(t.TempDir(), "pid")
	t.Setenv("BURNMON_CONTROLS_PIDFILE", pidFile)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The helper is slow to start; cancel 300 ms after it proved it runs.
	// Hard cap: 25 s, after which the context is cancelled anyway.
	hard := time.AfterFunc(25*time.Second, cancel)
	defer hard.Stop()
	go func() {
		for {
			if _, err := os.Stat(pidFile); err == nil {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	done := make(chan error, 1)
	go func() {
		_, _, err := ExecRunner{}.Run(ctx, []string{os.Args[0]}, "")
		done <- err
	}()
	var cancelledAt time.Time
	select {
	case err := <-done:
		cancelledAt = time.Now()
		if err == nil {
			t.Fatal("Run returned nil after cancel")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Run did not return")
	}
	_ = cancelledAt
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if pid == 0 || pid == os.Getpid() {
		t.Fatalf("bad helper pid %d", pid)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !processGone(pid) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !processGone(pid) {
		t.Fatalf("helper %d still running after timeout", pid)
	}
}

func TestExecRunnerReturnsWithinBoundAfterTimeout(t *testing.T) {
	t.Setenv("BURNMON_CONTROLS_HELPER", "sleep")
	t.Setenv("BURNMON_CONTROLS_PIDFILE", filepath.Join(t.TempDir(), "pid"))
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err := ExecRunner{}.Run(ctx, []string{os.Args[0]}, "")
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("Run took %v", d)
	}
}

func TestExecRunnerExitCodeAndFirstLine(t *testing.T) {
	t.Setenv("BURNMON_CONTROLS_HELPER", "print7")
	code, line, err := ExecRunner{}.Run(context.Background(), []string{os.Args[0]}, "")
	if err != nil || code != 7 || line != "first line" {
		t.Fatalf("code=%d line=%q err=%v", code, line, err)
	}
}

func TestExecRunnerStderrFallbackSkipsBlank(t *testing.T) {
	t.Setenv("BURNMON_CONTROLS_HELPER", "stderronly")
	code, line, err := ExecRunner{}.Run(context.Background(), []string{os.Args[0]}, "")
	if err != nil || code != 1 || line != "only on stderr" {
		t.Fatalf("code=%d line=%q err=%v", code, line, err)
	}
}

func TestExecRunnerNonzeroExitReturnsLastStderrLine(t *testing.T) {
	t.Setenv("BURNMON_CONTROLS_HELPER", "pytrace")
	code, line, err := ExecRunner{}.Run(context.Background(), []string{os.Args[0]}, "")
	if err != nil || code != 1 || line != "ValueError: example failure" {
		t.Fatalf("code=%d line=%q err=%v", code, line, err)
	}
}

func TestExecRunnerLineCutTo120Runes(t *testing.T) {
	t.Setenv("BURNMON_CONTROLS_HELPER", "long")
	_, line, err := ExecRunner{}.Run(context.Background(), []string{os.Args[0]}, "")
	if err != nil || len([]rune(line)) != 120 {
		t.Fatalf("runes=%d err=%v", len([]rune(line)), err)
	}
}

func TestExecRunnerRefusesEmptyArgv(t *testing.T) {
	if _, _, err := (ExecRunner{}).Run(context.Background(), nil, ""); err == nil {
		t.Fatal("empty argv must error")
	}
	if err := (ExecRunner{}).Start(nil); err == nil {
		t.Fatal("empty argv must error")
	}
}

func TestElevatedIsFalseInTests(t *testing.T) {
	if Elevated() {
		t.Skip("test shell is elevated")
	}
}

func TestElevatedFailsClosed(t *testing.T) {
	if !elevatedWith(func() (bool, error) { return false, errors.New("token query failed") }) {
		t.Error("a failed token query must count as elevated")
	}
	if !elevatedWith(func() (bool, error) { return true, nil }) {
		t.Error("an elevated token must count as elevated")
	}
	if elevatedWith(func() (bool, error) { return false, nil }) {
		t.Error("a limited token is not elevated")
	}
}

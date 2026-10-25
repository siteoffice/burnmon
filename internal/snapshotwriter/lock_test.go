package snapshotwriter_test

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"burnmon/internal/snapshotwriter"
)

const lockHelperEnv = "BURNMON_LOCK_HELPER"

// TestLockHelperProcess is the child process of the dead-owner tests: it
// takes the lock named by the env var, prints "held" and sleeps until killed.
// Run on its own it returns at once.
func TestLockHelperProcess(t *testing.T) {
	path := os.Getenv(lockHelperEnv)
	if path == "" {
		return
	}
	ok, err := snapshotwriter.NewLock(path).TryAcquire("helper")
	if err != nil || !ok {
		os.Stdout.WriteString("failed\n")
		os.Exit(1)
	}
	os.Stdout.WriteString("held\n")
	time.Sleep(60 * time.Second)
}

// startLockHelper starts a child that holds the lock at path and returns it
// once it printed "held". The caller kills it.
func startLockHelper(t *testing.T, path string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLockHelperProcess$")
	cmd.Env = append(os.Environ(), lockHelperEnv+"="+path)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	line := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(out)
		if sc.Scan() {
			line <- sc.Text()
		}
		close(line)
	}()
	select {
	case got := <-line:
		if got != "held" {
			t.Fatalf("helper said %q, want held", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("helper did not take the lock within 10s")
	}
	return cmd
}

func TestLockLiveOwnerBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.lock")
	l1 := snapshotwriter.NewLock(path)
	if ok, err := l1.TryAcquire("a"); err != nil || !ok {
		t.Fatalf("first TryAcquire = %v, %v; want true, nil", ok, err)
	}
	defer l1.Release()
	if !l1.Held() {
		t.Error("Held() = false after a successful TryAcquire")
	}
	if ok, err := snapshotwriter.NewLock(path).TryAcquire("b"); err != nil || ok {
		t.Fatalf("second TryAcquire = %v, %v; want false, nil", ok, err)
	}
	o, err := snapshotwriter.ReadOwner(path)
	if err != nil {
		t.Fatal(err)
	}
	if o.PID != os.Getpid() || o.Exe != "a" {
		t.Errorf("owner = %+v, want pid %d exe a", o, os.Getpid())
	}
}

func TestLockDeadOwnerTakenOver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.lock")
	helper := startLockHelper(t, path)
	l := snapshotwriter.NewLock(path)
	if ok, err := l.TryAcquire("parent"); err != nil || ok {
		t.Fatalf("TryAcquire against a live helper = %v, %v; want false, nil", ok, err)
	}
	helper.Process.Kill()
	helper.Wait()
	killed := time.Now()
	deadline := killed.Add(2 * time.Second)
	for {
		ok, err := l.TryAcquire("parent")
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("lock not free 2s after the owner was killed")
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Logf("lock taken over %d ms after the owner was killed", time.Since(killed).Milliseconds())
	defer l.Release()
	o, err := snapshotwriter.ReadOwner(path)
	if err != nil {
		t.Fatal(err)
	}
	if o.PID != os.Getpid() {
		t.Errorf("owner pid = %d, want %d", o.PID, os.Getpid())
	}
}

func TestLockStaleFileTakenOver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.lock")
	if err := os.WriteFile(path, []byte(`{"pid": 999999, "exe": "gone"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	l := snapshotwriter.NewLock(path)
	if ok, err := l.TryAcquire("me"); err != nil || !ok {
		t.Fatalf("TryAcquire over a stale file = %v, %v; want true, nil", ok, err)
	}
	defer l.Release()
	o, err := snapshotwriter.ReadOwner(path)
	if err != nil {
		t.Fatal(err)
	}
	if o.PID != os.Getpid() || o.Exe != "me" {
		t.Errorf("owner = %+v, want this process as me", o)
	}
}

func TestLockCreatesMissingFolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nope", ".burnmon")
	l := snapshotwriter.NewLock(filepath.Join(dir, "snapshot.lock"))
	if ok, err := l.TryAcquire("a"); err != nil || !ok {
		t.Fatalf("TryAcquire = %v, %v; want true, nil", ok, err)
	}
	defer l.Release()
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("folder %s not created: %v", dir, err)
	}
}

func TestLockOnlyOneWinsConcurrently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.lock")
	const n = 8
	locks := make([]*snapshotwriter.Lock, n)
	results := make([]bool, n)
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range locks {
		locks[i] = snapshotwriter.NewLock(path)
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = locks[i].TryAcquire("x")
		}(i)
	}
	close(start)
	wg.Wait()
	wins := 0
	for i := range locks {
		if errs[i] != nil {
			t.Fatalf("lock %d: %v", i, errs[i])
		}
		if results[i] {
			wins++
			defer locks[i].Release()
		}
	}
	if wins != 1 {
		t.Errorf("%d of %d locks won, want exactly 1", wins, n)
	}
}

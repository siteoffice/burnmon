package snapshotwriter_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/pricing"
	"burnmon/internal/snapshotwriter"
	"burnmon/internal/snapshotwriter/writertest"
)

const runTick = 100 * time.Millisecond

func runWriter(t *testing.T) (*snapshotwriter.Writer, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snapshot.json")
	w := &snapshotwriter.Writer{Store: writertest.Store(t), Path: path, Tick: runTick, Config: pricing.Defaults}
	writertest.Fake(w)
	return w, path
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// waitFor polls cond every 10ms until it holds or limit passes.
func waitFor(limit time.Duration, cond func() bool) bool {
	end := time.Now().Add(limit)
	for time.Now().Before(end) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

func TestRunWritesWhenItHoldsTheLock(t *testing.T) {
	w, path := runWriter(t)
	lockPath := filepath.Join(t.TempDir(), "snapshot.lock")
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { w.Run(snapshotwriter.NewLock(lockPath), "test.exe", stop); close(done) }()
	if !waitFor(300*time.Millisecond, func() bool { return exists(path) }) {
		t.Fatal("snapshot file did not appear within 300ms")
	}
	close(stop)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after stop closed")
	}
	l := snapshotwriter.NewLock(lockPath)
	if ok, err := l.TryAcquire("after"); err != nil || !ok {
		t.Fatalf("lock not released by Run: %v, %v", ok, err)
	}
	l.Release()
}

func TestRunStaysReadOnlyWhileOtherHoldsLock(t *testing.T) {
	w, path := runWriter(t)
	lockPath := filepath.Join(t.TempDir(), "snapshot.lock")
	other := snapshotwriter.NewLock(lockPath)
	if ok, err := other.TryAcquire("other"); err != nil || !ok {
		t.Fatalf("setup: %v, %v", ok, err)
	}
	defer other.Release()
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { w.Run(snapshotwriter.NewLock(lockPath), "test.exe", stop); close(done) }()
	time.Sleep(5 * runTick)
	close(stop)
	<-done
	if exists(path) {
		t.Error("snapshot file written while another process holds the lock")
	}
}

func TestRunTakesOverWithinTwoTicks(t *testing.T) {
	w, path := runWriter(t)
	lockPath := filepath.Join(t.TempDir(), "snapshot.lock")
	helper := startLockHelper(t, lockPath)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { w.Run(snapshotwriter.NewLock(lockPath), "test.exe", stop); close(done) }()
	defer func() { close(stop); <-done }()
	time.Sleep(3 * runTick)
	if exists(path) {
		t.Fatal("snapshot file written while the helper holds the lock")
	}
	helper.Process.Kill()
	helper.Wait()
	killed := time.Now()
	if !waitFor(2*runTick+300*time.Millisecond, func() bool { return exists(path) }) {
		t.Fatalf("no snapshot within %v of the kill", 2*runTick+300*time.Millisecond)
	}
	t.Logf("take-over wrote the snapshot %d ms after the owner was killed", time.Since(killed).Milliseconds())
}

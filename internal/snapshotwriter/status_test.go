package snapshotwriter_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/snapshotwriter"
)

// runStatus starts w.Run on lockPath with a fresh Status and returns it plus
// a stop func that waits for Run to return.
func runStatus(t *testing.T, w *snapshotwriter.Writer, lockPath string) (*snapshotwriter.Status, func()) {
	t.Helper()
	w.Status = snapshotwriter.NewStatus()
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { w.Run(snapshotwriter.NewLock(lockPath), "test.exe", stop); close(done) }()
	var once bool
	return w.Status, func() {
		if !once {
			once = true
			close(stop)
			<-done
		}
	}
}

func waitState(t *testing.T, s *snapshotwriter.Status, want string) snapshotwriter.StatusView {
	t.Helper()
	if !waitFor(2*time.Second, func() bool { return s.View().State == want }) {
		t.Fatalf("state = %+v, want %q within 2s", s.View(), want)
	}
	return s.View()
}

func TestStatusNewIsStarting(t *testing.T) {
	if got := snapshotwriter.NewStatus().View(); got != (snapshotwriter.StatusView{State: snapshotwriter.StateStarting}) {
		t.Errorf("NewStatus().View() = %+v, want state starting only", got)
	}
}

func TestStatusStartingToWriting(t *testing.T) {
	w, _ := runWriter(t)
	s, stop := runStatus(t, w, filepath.Join(t.TempDir(), "snapshot.lock"))
	defer stop()
	v := waitState(t, s, snapshotwriter.StateWriting)
	if v.Error != "" || v.OwnerExe != "" || v.OwnerPID != 0 {
		t.Errorf("writing view = %+v, want no error and no owner", v)
	}
	if _, err := time.Parse(time.RFC3339, v.LastWriteAt); err != nil {
		t.Errorf("last_write_at %q is not RFC3339: %v", v.LastWriteAt, err)
	}
}

func TestStatusStartingToOther(t *testing.T) {
	w, path := runWriter(t)
	lockPath := filepath.Join(t.TempDir(), "snapshot.lock")
	other := snapshotwriter.NewLock(lockPath)
	if ok, err := other.TryAcquire("other.exe"); err != nil || !ok {
		t.Fatalf("setup: %v, %v", ok, err)
	}
	defer other.Release()
	s, stop := runStatus(t, w, lockPath)
	defer stop()
	v := waitState(t, s, snapshotwriter.StateOther)
	if v.OwnerExe != "other.exe" || v.OwnerPID != os.Getpid() {
		t.Errorf("other view = %+v, want owner other.exe pid %d", v, os.Getpid())
	}
	if exists(path) {
		t.Error("snapshot written while another Lock holds it")
	}
}

func TestStatusStartingToLockError(t *testing.T) {
	w, _ := runWriter(t)
	// A regular file where the lock folder should be: MkdirAll fails.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s, stop := runStatus(t, w, filepath.Join(blocker, "snapshot.lock"))
	defer stop()
	if v := waitState(t, s, snapshotwriter.StateLockError); v.Error == "" {
		t.Errorf("lock_error view = %+v, want the error text", v)
	}
}

func TestStatusWritingFailedWriting(t *testing.T) {
	w, path := runWriter(t)
	s, stop := runStatus(t, w, filepath.Join(t.TempDir(), "snapshot.lock"))
	defer stop()
	first := waitState(t, s, snapshotwriter.StateWriting)
	// A folder at the snapshot path: the rename onto it fails.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	v := waitState(t, s, snapshotwriter.StateFailed)
	if v.Error == "" {
		t.Errorf("failed view = %+v, want the error text", v)
	}
	if v.LastWriteAt == "" || v.LastWriteAt < first.LastWriteAt {
		t.Errorf("failed view last_write_at = %q, want the last good write kept (%q)", v.LastWriteAt, first.LastWriteAt)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if v := waitState(t, s, snapshotwriter.StateWriting); v.Error != "" {
		t.Errorf("writing again view = %+v, want the error cleared", v)
	}
}

func TestStatusNilIsSafe(t *testing.T) {
	var s *snapshotwriter.Status
	if got := s.View(); got != (snapshotwriter.StatusView{}) {
		t.Errorf("nil View() = %+v, want the zero view", got)
	}
	s.Fail(errors.New("x"))
	// Run with no Status walks every setter it reaches without a panic.
	w, path := runWriter(t)
	lockPath := filepath.Join(t.TempDir(), "snapshot.lock")
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { w.Run(snapshotwriter.NewLock(lockPath), "test.exe", stop); close(done) }()
	if !waitFor(2*time.Second, func() bool { return exists(path) }) {
		t.Fatal("no snapshot written with a nil Status")
	}
	close(stop)
	<-done
}

func TestStatusFailIsLockError(t *testing.T) {
	s := snapshotwriter.NewStatus()
	s.Fail(errors.New("no home folder"))
	if v := s.View(); v.State != snapshotwriter.StateLockError || v.Error != "no home folder" {
		t.Errorf("after Fail view = %+v, want lock_error with the error", v)
	}
}

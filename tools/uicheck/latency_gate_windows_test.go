//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// holdOpen opens path the way a Go reader does (share read and write, not
// delete), so deleting it fails until release. The hold ends with the test
// at the latest, before its temp folder is removed.
func holdOpen(t *testing.T, path string) (release func()) {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release = func() { once.Do(func() { windows.CloseHandle(h) }) }
	t.Cleanup(release)
	return release
}

// A sprint.json that cannot be deleted stops the removal before the marker:
// the folder stays marked (a later run takes it over) and the error surfaces.
func TestRemoveProbeDirKeepsMarkerWhenHeld(t *testing.T) {
	dir := claimTemp(t)
	if err := writeProbeSprint(dir, "waiting-on-you", time.Now()); err != nil {
		t.Fatal(err)
	}
	swapVar(t, &probeRemoveRetry, 200*time.Millisecond)
	release := holdOpen(t, filepath.Join(dir, "sprint.json"))
	if err := removeProbeDir(dir); err == nil {
		t.Fatal("removeProbeDir reported success with sprint.json held open")
	}
	if !hasProbeMarker(dir) {
		t.Fatal("the marker went while sprint.json stayed")
	}
	if _, err := os.Stat(filepath.Join(dir, "sprint.json")); err != nil {
		t.Fatalf("sprint.json: %v", err)
	}
	release()
	if err := removeProbeDir(dir); err != nil {
		t.Fatalf("second removal: %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("probe folder still there: %v", err)
	}
}

// A reader that lets go within the retry window does not fail the removal.
func TestRemoveProbeDirRetriesShortHold(t *testing.T) {
	dir := claimTemp(t)
	if err := writeProbeSprint(dir, "running", time.Now()); err != nil {
		t.Fatal(err)
	}
	swapVar(t, &probeRemoveRetry, 5*time.Second)
	release := holdOpen(t, filepath.Join(dir, "sprint.json"))
	time.AfterFunc(150*time.Millisecond, release)
	if err := removeProbeDir(dir); err != nil {
		t.Fatalf("removeProbeDir: %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("probe folder still there: %v", err)
	}
}

// Once the removal has run, even a failed one, no write lands in the probe:
// a Ctrl+C cleanup is never followed by a waiting-on-you write.
func TestProbeWriteAfterRemoveRefused(t *testing.T) {
	dir := claimTemp(t)
	p := &latencyProbe{dir: dir}
	if err := p.write("running", time.Now()); err != nil {
		t.Fatal(err)
	}
	swapVar(t, &probeRemoveRetry, 200*time.Millisecond)
	release := holdOpen(t, filepath.Join(dir, "sprint.json"))
	if err := p.remove(); err == nil {
		t.Fatal("remove reported success with sprint.json held open")
	}
	if err := p.write("waiting-on-you", time.Now()); err == nil {
		t.Fatal("a write after the removal was accepted")
	}
	if got := probeStatus(t, dir); got != "running" {
		t.Fatalf("sprint.json status %q after the removal, want running", got)
	}
	release()
	if err := removeProbeDir(dir); err != nil {
		t.Fatal(err)
	}
}

// Every round passes but the probe cannot be removed: the gate fails, and
// the probe stays marked.
func TestLatencyGateFailsWhenCleanupFails(t *testing.T) {
	dir := claimTemp(t)
	if err := writeProbeSprint(dir, "running", time.Now()); err != nil {
		t.Fatal(err)
	}
	fakePage(t, dir)
	swapVar(t, &probeRemoveRetry, 200*time.Millisecond)
	release := holdOpen(t, filepath.Join(dir, "sprint.json"))
	err := latencyGateIn(dir, 1)
	if err == nil || !strings.Contains(err.Error(), "cleanup") {
		t.Fatalf("latencyGateIn err = %v, want the cleanup failure", err)
	}
	if !hasProbeMarker(dir) {
		t.Fatal("the marker went while sprint.json stayed")
	}
	release()
	if err := removeProbeDir(dir); err != nil {
		t.Fatal(err)
	}
}

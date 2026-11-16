//go:build windows

package instance

import (
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

var nameSeq atomic.Int64

// A second claim on the same mutex name in one process reports "already
// running", the same signal a second app instance gets. A different name is
// independent.
func TestAlreadyRunningSecondClaim(t *testing.T) {
	// A mutex is held until the process ends, so a repeated run (-count=3) must
	// not reuse a name; the clock alone ticks too slowly for that on Windows.
	name := fmt.Sprintf(`Local\burnmon-instance-test-%d-%d-%d`, os.Getpid(), time.Now().UnixNano(), nameSeq.Add(1))
	if AlreadyRunning(name) {
		t.Fatal("first claim reported already running")
	}
	if !AlreadyRunning(name) {
		t.Fatal("second claim did not report already running")
	}
	other := name + "-other"
	if AlreadyRunning(other) {
		t.Fatal("a different mutex name reported already running")
	}
}

// A name that cannot be a Windows string (embedded NUL) reports false, as the
// app copies did.
func TestAlreadyRunningBadName(t *testing.T) {
	if AlreadyRunning("bad\x00name") {
		t.Fatal("a name with a NUL reported already running")
	}
}

// With no window of that title, BringToFront does nothing and does not panic.
func TestBringToFrontMissingWindow(t *testing.T) {
	BringToFront(fmt.Sprintf("burnmon-no-such-window-%d", time.Now().UnixNano()))
	BringToFront("bad\x00title")
}

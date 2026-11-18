//go:build windows || (darwin && cgo)

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"burnmon/internal/lamp"
)

// fakeLampProc is a child the test ends by hand.
type fakeLampProc struct {
	done    chan struct{}
	once    sync.Once
	stopped bool
}

func (p *fakeLampProc) Stop()                 { p.stopped = true; p.end() }
func (p *fakeLampProc) Done() <-chan struct{} { return p.done }
func (p *fakeLampProc) end()                  { p.once.Do(func() { close(p.done) }) }

// lampRig is a lampChild with a fake starter and a clock the test moves.
type lampRig struct {
	child *lampChild
	procs []*fakeLampProc
	argvs [][]string
	now   time.Time
}

func newLampRig(t *testing.T) *lampRig {
	t.Helper()
	r := &lampRig{now: time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)}
	r.child = newLampChild(`C:\x\burnmon-dev.exe`, func(exe string, args []string) (lampProc, error) {
		r.argvs = append(r.argvs, append([]string{exe}, args...))
		p := &fakeLampProc{done: make(chan struct{})}
		r.procs = append(r.procs, p)
		return p, nil
	}, func() time.Time { return r.now }, func(string, ...any) {})
	r.child.supported = true
	return r
}

func TestLampChildOffStartsNothing(t *testing.T) {
	r := newLampRig(t)
	r.child.Sync(false)
	if len(r.procs) != 0 {
		t.Fatalf("lamp_enabled false started %d children", len(r.procs))
	}
}

func TestLampChildOnStartsOneAndOffStopsIt(t *testing.T) {
	r := newLampRig(t)
	r.child.Sync(true)
	r.child.Sync(true)
	if len(r.procs) != 1 {
		t.Fatalf("on twice started %d children, want 1", len(r.procs))
	}
	r.child.Sync(false)
	if !r.procs[0].stopped {
		t.Fatal("switching off did not stop the child")
	}
	r.child.Sync(true)
	if len(r.procs) != 2 {
		t.Fatalf("on again started %d children in total, want 2", len(r.procs))
	}
	r.child.Stop()
	if !r.procs[1].stopped {
		t.Fatal("Stop (BurnMon Dev exit) did not stop the child")
	}
}

// The child is the own exe and the word lamp, nothing else.
func TestLampChildArgvIsOwnExeAndLamp(t *testing.T) {
	r := newLampRig(t)
	r.child.Sync(true)
	want := []string{`C:\x\burnmon-dev.exe`, "lamp"}
	if len(r.argvs) != 1 || !reflect.DeepEqual(r.argvs[0], want) {
		t.Fatalf("argv = %v, want %v", r.argvs, want)
	}
}

// A lamp that dies starts again at most 3 times in 10 minutes; the fourth
// restart is refused. After 10 minutes the count is clear again.
func TestLampChildRestartLimit(t *testing.T) {
	r := newLampRig(t)
	r.child.Sync(true)
	for i := 1; i <= 3; i++ {
		r.child.died(r.procs[len(r.procs)-1])
		if len(r.procs) != 1+i {
			t.Fatalf("restart %d: %d children in total, want %d", i, len(r.procs), 1+i)
		}
		r.now = r.now.Add(time.Minute)
	}
	r.child.died(r.procs[len(r.procs)-1])
	if len(r.procs) != 4 {
		t.Fatalf("the fourth restart in 10 minutes ran: %d children in total, want 4", len(r.procs))
	}
	r.child.Sync(true)
	if len(r.procs) != 4 {
		t.Fatal("a Sync(true) after the refusal started a child past the limit")
	}
	// Off and on again is the person's own choice and starts clean.
	r.child.Sync(false)
	r.child.Sync(true)
	if len(r.procs) != 5 {
		t.Fatalf("off then on started %d children in total, want 5", len(r.procs))
	}
}

func TestLampChildRestartWindowSlides(t *testing.T) {
	r := newLampRig(t)
	r.child.Sync(true)
	for i := 0; i < 3; i++ {
		r.child.died(r.procs[len(r.procs)-1])
	}
	r.now = r.now.Add(10*time.Minute + time.Second)
	r.child.died(r.procs[len(r.procs)-1])
	if len(r.procs) != 5 {
		t.Fatalf("after 10 minutes a death was not restarted: %d children in total, want 5", len(r.procs))
	}
}

// A child that was stopped on purpose is not restarted.
func TestLampChildStoppedOnPurposeStaysStopped(t *testing.T) {
	r := newLampRig(t)
	r.child.Sync(true)
	old := r.procs[0]
	r.child.Sync(false)
	r.child.died(old)
	if len(r.procs) != 1 {
		t.Fatalf("a stopped child was restarted: %d children in total", len(r.procs))
	}
}

// Fix lane T2, close: a lamp that closed itself from its menu saved
// lamp_enabled false before it ended. The supervisor reads the setting before
// it restarts a dead child, so that child is not started again. While the file
// still says on, a dead child is restarted as before.
func TestLampChildNotRestartedWhenTheFileSaysOff(t *testing.T) {
	r := newLampRig(t)
	on := true
	r.child.enabled = func() bool { return on }
	r.child.Sync(true)
	r.child.died(r.procs[0])
	if len(r.procs) != 2 {
		t.Fatalf("a death with lamp_enabled true: %d children in total, want 2", len(r.procs))
	}
	on = false
	r.child.died(r.procs[1])
	if len(r.procs) != 2 {
		t.Fatalf("a death with lamp_enabled false was restarted: %d children in total", len(r.procs))
	}
	// Switching on again from the gear starts a child.
	on = true
	r.child.Sync(true)
	if len(r.procs) != 3 {
		t.Fatalf("on again: %d children in total, want 3", len(r.procs))
	}
}

// A bindDev env without a child (the binding test) must not panic when the
// binding runs.
func TestLampChildNilIsSafe(t *testing.T) {
	var c *lampChild
	c.Sync(true)
	c.Stop()
	if err := setLampKey(t.TempDir(), "lamp_lines", false, nil); err != nil {
		t.Fatal(err)
	}
}

func TestLampChildUnsupportedPlatformStartsNothing(t *testing.T) {
	r := newLampRig(t)
	r.child.supported = false
	r.child.Sync(true)
	if len(r.procs) != 0 {
		t.Fatal("an unsupported platform started a child")
	}
}

// bdevSetLamp takes only the five gear keys, with the right kind of value,
// saves through saveDevConfigKey and tells the child.
func TestSetLampKey(t *testing.T) {
	dir := t.TempDir()
	r := newLampRig(t)

	for _, bad := range []struct {
		key string
		val any
	}{
		{"lamp_burn_floor_tpm", -1.0},
		{"lamp_burn_floor_tpm", 250000.5},
		{"lamp_burn_floor_tpm", "300000"},
		{"lamp_burn_floor_tpm", true},
		{"lamp_burn_floor_tpm", nil},
		{"lamp_retention_days", 5.0}, // a key that is not a gear key
		{"toast_enabled", true},
		{"lamp_enabled", "yes"},
		{"lamp_position", "middle"},
		{"lamp_position", true},
		{"lamp_language", "de"},
		{"lamp_lines", "true"},
		{"lamp_foreman_items", nil},
		{"", true},
		{"lamp_ticker_rows", 0.0},
		{"lamp_ticker_rows", 5.0},
		{"lamp_ticker_rows", 2.5},
		{"lamp_ticker_rows", "2"},
		{"lamp_size", "huge"},
		{"lamp_size", 1.5},
		{"lamp_size", 95.0},
		{"lamp_size", 102.0},
		{"lamp_size", 205.0},
		{"lamp_size", "medium"},
		{"lamp_monitor", "DISPLAY2"},
		{"lamp_monitor", true},
		{"lamp_theme", "sepia"},
		{"lamp_motion", "wild"},
		{"lamp_quiet", true},
		{"lamp_backdrop", "glass"},
	} {
		if err := setLampKey(dir, bad.key, bad.val, r.child); err == nil {
			t.Errorf("setLampKey(%q, %v) accepted", bad.key, bad.val)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "burnmon-dev.json")); err == nil {
		t.Fatal("a refused key still wrote burnmon-dev.json")
	}
	if len(r.procs) != 0 {
		t.Fatal("a refused key started a child")
	}

	for _, ok := range []struct {
		key string
		val any
	}{
		{"lamp_position", "top-left"},
		{"lamp_language", "nl"},
		{"lamp_lines", false},
		{"lamp_foreman_items", false},
		{"lamp_ticker_rows", 3.0},
		{"lamp_size", 100.0},
		{"lamp_size", 105.0},
		{"lamp_size", 200.0},
		{"lamp_size", 190.0},
		{"lamp_monitor", ""},
		{"lamp_theme", "dark"},
		{"lamp_motion", "calm"},
		{"lamp_quiet", "stay"},
		{"lamp_backdrop", "card"},
		{"lamp_burn_floor_tpm", 0.0},
		{"lamp_burn_floor_tpm", 1000000.0},
		{"lamp_burn_floor_tpm", 350000.0},
		{"lamp_enabled", true},
	} {
		if err := setLampKey(dir, ok.key, ok.val, r.child); err != nil {
			t.Fatalf("setLampKey(%q, %v): %v", ok.key, ok.val, err)
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "burnmon-dev.json"))
	if err != nil {
		t.Fatal(err)
	}
	got := lamp.ParseSettings(b)
	want := lamp.DefaultSettings()
	want.Position, want.Language, want.Lines, want.ForemanItems, want.Enabled = "top-left", "nl", false, false, true
	want.TickerRows, want.SizePct, want.Theme, want.Motion, want.Quiet, want.Backdrop = 3, 190, "dark", "calm", "stay", "card"
	want.BurnFloorTPM = 350000
	if got != want {
		t.Fatalf("saved settings %+v, want %+v", got, want)
	}
	g := lampGear(dir)
	if g.TickerRows != 3 || g.Size != 190 || g.Monitor != "" || g.Theme != "dark" || g.Motion != "calm" || g.Quiet != "stay" || g.Backdrop != "card" || g.BurnFloor != 350000 {
		t.Fatalf("lampGear does not carry the six new settings: %+v", g)
	}
	if len(r.procs) != 1 {
		t.Fatalf("lamp_enabled true started %d children, want 1", len(r.procs))
	}
	if err := setLampKey(dir, "lamp_enabled", false, r.child); err != nil {
		t.Fatal(err)
	}
	if !r.procs[0].stopped {
		t.Fatal("lamp_enabled false did not stop the child")
	}
}

// A failed save leaves the child as it was.
func TestSetLampKeySaveFailureLeavesChild(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "burnmon-dev.json"), []byte(`{"lamp_enabled": `), 0o600); err != nil {
		t.Fatal(err)
	}
	r := newLampRig(t)
	err := setLampKey(dir, "lamp_enabled", true, r.child)
	if err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("err = %v, want the not valid JSON refusal", err)
	}
	if len(r.procs) != 0 {
		t.Fatal("a failed save started a child")
	}
}

// Close fix T1: a lamp that is still running while the saved lamp_enabled is
// false is stopped by the next tick, and not started again by itself.
func TestLampChildTickStopsAChildTheFileSaysOff(t *testing.T) {
	r := newLampRig(t)
	on := true
	r.child.enabled = func() bool { return on }
	r.child.Sync(true)
	r.child.Tick()
	if r.procs[0].stopped {
		t.Fatal("a tick stopped a child while the file says on")
	}
	on = false
	r.child.Tick()
	if !r.procs[0].stopped {
		t.Fatal("a tick left a child running while the file says lamp_enabled false")
	}
	time.Sleep(20 * time.Millisecond) // let died() see the end
	if len(r.procs) != 1 {
		t.Fatalf("the child was started again after the tick stopped it: %d starts", len(r.procs))
	}
	r.child.Tick() // nothing runs: nothing happens
	on = true
	r.child.Sync(true)
	if len(r.procs) != 2 {
		t.Fatalf("on again started %d children in total, want 2", len(r.procs))
	}
}

// An unreadable file (savedOff false) never stops a running child.
func TestLampChildTickIgnoresAnUnreadableFile(t *testing.T) {
	r := newLampRig(t)
	r.child.enabled = func() bool { return false } // what a failed read gives
	r.child.savedOff = func() bool { return false }
	r.child.Sync(true)
	r.child.Tick()
	if r.procs[0].stopped {
		t.Fatal("a tick stopped the child although the file was not read as off")
	}
}

// Without a file reader (nil) and on a nil child a tick does nothing.
func TestLampChildTickWithoutReaderIsSafe(t *testing.T) {
	r := newLampRig(t)
	r.child.Sync(true)
	r.child.Tick()
	if r.procs[0].stopped {
		t.Fatal("a tick without a file reader stopped the child")
	}
	var c *lampChild
	c.Tick()
}

// Close fix T1: the lamp ends on the window's own thread. Terminate of
// go-webview2 posts the quit message to the calling thread, so it must run
// inside the dispatch, never straight from the caller's goroutine. When the
// process still lives after the grace time, it exits.
func TestLampEndRunsTerminateOnTheWindowThread(t *testing.T) {
	var queued []func()
	terminated := 0
	exited := make(chan int, 1)
	lampEnd(func(f func()) { queued = append(queued, f) }, func() { terminated++ },
		func(code int) { exited <- code }, 30*time.Millisecond)
	if terminated != 0 {
		t.Fatal("Terminate ran on the caller's thread, not inside the dispatch")
	}
	if len(queued) != 1 {
		t.Fatalf("lampEnd dispatched %d functions, want 1", len(queued))
	}
	queued[0]()
	if terminated != 1 {
		t.Fatalf("Terminate ran %d times when the dispatch ran, want 1", terminated)
	}
	select {
	case code := <-exited:
		if code != 0 {
			t.Fatalf("the second line exited with %d, want 0", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the process was not made to exit after the grace time")
	}
}

// BurnMon Dev starts the child from the saved file at its own start.
func TestSyncLampFromFile(t *testing.T) {
	dir := t.TempDir()
	r := newLampRig(t)
	syncLampFromFile(dir, r.child)
	if len(r.procs) != 0 {
		t.Fatal("no file started a child")
	}
	if err := os.WriteFile(filepath.Join(dir, "burnmon-dev.json"), []byte(`{"lamp_enabled": true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	syncLampFromFile(dir, r.child)
	if len(r.procs) != 1 {
		t.Fatalf("lamp_enabled true in the file started %d children, want 1", len(r.procs))
	}
}

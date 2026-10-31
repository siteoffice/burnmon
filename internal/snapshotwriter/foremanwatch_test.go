package snapshotwriter

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"burnmon/internal/graph"
	"burnmon/internal/pricing"
	"burnmon/internal/snapshot"
	"burnmon/internal/store"
	"burnmon/internal/sysmon"
)

func TestForemanFile(t *testing.T) {
	dir := filepath.Join("f", "sprints", "p", "s")
	for _, name := range []string{"sprint.json", "action-log.ndjson", "report.md", "flow.json", "foreman.json", "SPRINT.JSON", "Flow.Json"} {
		if !ForemanFile(filepath.Join(dir, name)) {
			t.Errorf("ForemanFile(%s) = false, want true", name)
		}
	}
	for _, name := range []string{"index.json", "index.html", "dashboard.html", "plan.md", "x.jsonl", "sprint.json.tmp", "s"} {
		if ForemanFile(filepath.Join(dir, name)) {
			t.Errorf("ForemanFile(%s) = true, want false", name)
		}
	}
}

// The window is 100 ms here, not the production 200 ms, so five triggers
// fit in it with room to spare on a loaded machine (Ruling 9). A run whose
// triggers still took a whole window is skipped: a late trigger opens a
// second window there, so one fire cannot be shown on it.
func TestCoalescerOneFire(t *testing.T) {
	const d = 100 * time.Millisecond
	fires := make(chan time.Time, 10)
	c := newCoalescer(d, func() { fires <- time.Now() })
	defer c.Stop()
	first := time.Now()
	var last time.Time
	for i := 0; i < 5; i++ {
		if i > 0 {
			time.Sleep(2 * time.Millisecond)
		}
		c.Trigger()
		last = time.Now()
	}
	if span := last.Sub(first); span >= d {
		t.Skipf("the five triggers took %v, not under the %v window, so one fire cannot be tested on this run", span, d)
	}
	select {
	case at := <-fires:
		if lag := at.Sub(first); lag < d || lag > 3*d {
			t.Fatalf("fired %v after the first trigger, want about %v", lag, d)
		}
	case <-time.After(10 * d):
		t.Fatal("five triggers gave no fire")
	}
	select {
	case <-fires:
		t.Fatal("five triggers in one window gave a second fire")
	case <-time.After(2 * d):
	}
	c.Trigger()
	select {
	case <-fires:
	case <-time.After(10 * d):
		t.Fatal("a trigger after the fire armed no new one")
	}
}

// Ruling 1: the window starts on the first trigger and later triggers do not
// move it, so a sprint that writes all the time still gets read.
func TestCoalescerStreamStillFires(t *testing.T) {
	const d = 20 * time.Millisecond
	var fires atomic.Int32
	c := newCoalescer(d, func() { fires.Add(1) })
	defer c.Stop()
	for end := time.Now().Add(10 * d); time.Now().Before(end); {
		c.Trigger()
		time.Sleep(2 * time.Millisecond)
	}
	if n := fires.Load(); n < 2 {
		t.Fatalf("a steady stream of triggers over %v fired %d times, want at least 2", 10*d, n)
	}
}

// Same guard as TestCoalescerOneFire: a stall between Trigger and Stop as
// long as the window lets the fire through before Stop runs.
func TestCoalescerStop(t *testing.T) {
	const d = 100 * time.Millisecond
	var fires atomic.Int32
	c := newCoalescer(d, func() { fires.Add(1) })
	start := time.Now()
	c.Trigger()
	c.Stop()
	if span := time.Since(start); span >= d {
		t.Skipf("Trigger to Stop took %v, not under the %v window, so a dropped fire cannot be tested on this run", span, d)
	}
	c.Trigger()
	time.Sleep(2 * d)
	if n := fires.Load(); n != 0 {
		t.Fatalf("fired %d times after Stop, want 0", n)
	}
}

// foremanWriter is a Writer over an empty store whose only graph source is
// the Foreman root; writes counts the writes (each one calls Procs once).
func foremanWriter(t *testing.T, root string, tick time.Duration) (*Writer, *atomic.Int32) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	gr := NewGraphReaders(t.TempDir(), root)
	gr.Listen = func() ([]graph.Listen, error) { return nil, nil }
	writes := &atomic.Int32{}
	w := &Writer{Store: st, Path: filepath.Join(t.TempDir(), "snapshot.json"), Tick: tick, Config: pricing.Defaults, Graph: gr,
		Procs: func() ([]sysmon.ProcessGroupSample, error) { writes.Add(1); return nil, nil }}
	return w, writes
}

// startRun runs w with its own lock file and returns a stop that waits for
// Run to return.
func startRun(t *testing.T, w *Writer, lockPath string) func() {
	t.Helper()
	stop, done := make(chan struct{}), make(chan struct{})
	go func() { w.Run(NewLock(lockPath), "test.exe", stop); close(done) }()
	return func() {
		close(stop)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Run did not return after stop closed")
		}
	}
}

// eventually polls cond every 10 ms until it holds or limit passes.
func eventually(limit time.Duration, cond func() bool) bool {
	for end := time.Now().Add(limit); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if cond() {
			return true
		}
	}
	return cond()
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// sprintBody is a sprint.json; a waiting one has a parked task, which makes
// waits.Build list it as Blocked (sprint:p__s__1).
func sprintBody(status string) string {
	tasks := ""
	if status == "waiting-on-you" {
		tasks = `,"tasks":[{"n":1,"title":"one","state":"parked"}]`
	}
	return `{"sprint_id":"p__s__1","slug":"s","project":"example","status":"` + status + `","updated_at":"` +
		time.Now().UTC().Format(time.RFC3339) + `"` + tasks + `}`
}

// foremanRoot is a Foreman root with one running sprint and an empty flows
// folder; it returns the sprint.json path.
func foremanRoot(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	sprint := filepath.Join(root, "sprints", "p", "s", "sprint.json")
	writeFile(t, sprint, sprintBody("running"))
	if err := os.MkdirAll(filepath.Join(root, "flows"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root, sprint
}

func sprintWaits(path string) bool {
	s, err := snapshot.Read(path)
	if err != nil {
		return false
	}
	for _, it := range s.WaitItems {
		if it.ID == "sprint:p__s__1" {
			return true
		}
	}
	return false
}

// With a one-hour tick and the 7.5 s Foreman read not yet due, only the
// watch can bring the new state into the file this fast.
func TestRunKickWritesAtOnce(t *testing.T) {
	root, sprint := foremanRoot(t)
	w, _ := foremanWriter(t, root, time.Hour)
	stop := startRun(t, w, filepath.Join(t.TempDir(), "snapshot.lock"))
	defer stop()
	if !eventually(2*time.Second, func() bool { _, err := os.Stat(w.Path); return err == nil }) {
		t.Fatal("no first write")
	}
	if sprintWaits(w.Path) {
		t.Fatal("the running sprint already waits in the first write")
	}
	changed := time.Now()
	writeFile(t, sprint, sprintBody("waiting-on-you"))
	if !eventually(1500*time.Millisecond, func() bool { return sprintWaits(w.Path) }) {
		t.Fatal("the waiting sprint did not reach the snapshot within 1.5 s")
	}
	t.Logf("the waiting sprint reached the snapshot %d ms after the write", time.Since(changed).Milliseconds())
}

// Review focus 2: index.json and index.html at the sprints root and the
// board files never cause a read; one sprint_step.py burst is one read.
func TestRunKickBurstIsOneRead(t *testing.T) {
	root, sprint := foremanRoot(t)
	w, writes := foremanWriter(t, root, time.Hour)
	stop := startRun(t, w, filepath.Join(t.TempDir(), "snapshot.lock"))
	defer stop()
	if !eventually(2*time.Second, func() bool { return writes.Load() == 1 }) {
		t.Fatal("no first write")
	}
	settle := foremanDebounce + 500*time.Millisecond
	board := []string{filepath.Join(root, "sprints", "index.json"), filepath.Join(root, "sprints", "index.html"),
		filepath.Join(root, "sprints", "p", "_board", "sprints", "index.json"), filepath.Join(root, "sprints", "p", "_board", "sprints", "index.html")}
	for _, p := range board {
		writeFile(t, p, "{}")
	}
	time.Sleep(settle)
	if n := writes.Load(); n != 1 {
		t.Fatalf("index and board files gave %d extra writes, want 0", n-1)
	}
	writeFile(t, filepath.Join(filepath.Dir(sprint), "action-log.ndjson"), `{"ts":"2026-10-30T10:00:00Z","action":"task_parked","task":1}`+"\n")
	writeFile(t, sprint, sprintBody("waiting-on-you"))
	for _, p := range board {
		writeFile(t, p, "{\"n\":2}")
	}
	if !eventually(1500*time.Millisecond, func() bool { return writes.Load() >= 2 }) {
		t.Fatal("the burst gave no write")
	}
	time.Sleep(settle)
	if n := writes.Load(); n != 2 {
		t.Fatalf("one burst gave %d writes, want 1", n-1)
	}
}

// A kick never writes without the lock. Once the lock is free a kick takes
// it and writes, which shows the first kick did reach Run.
func TestRunKickWithoutLock(t *testing.T) {
	root, sprint := foremanRoot(t)
	w, writes := foremanWriter(t, root, time.Hour)
	w.Status = NewStatus()
	lockPath := filepath.Join(t.TempDir(), "snapshot.lock")
	other := NewLock(lockPath)
	if ok, err := other.TryAcquire("other"); err != nil || !ok {
		t.Fatalf("setup: %v, %v", ok, err)
	}
	stop := startRun(t, w, lockPath)
	defer stop()
	if !eventually(2*time.Second, func() bool { return w.Status.View().State == StateOther }) {
		t.Fatalf("writer state %+v, want other", w.Status.View())
	}
	writeFile(t, sprint, sprintBody("waiting-on-you"))
	w.Kick()
	time.Sleep(foremanDebounce + 500*time.Millisecond)
	if n := writes.Load(); n != 0 {
		t.Fatalf("%d writes while another process holds the lock", n)
	}
	other.Release()
	w.Kick()
	if !eventually(1500*time.Millisecond, func() bool { return sprintWaits(w.Path) }) {
		t.Fatal("a kick with the lock free wrote nothing")
	}
}

// Review focus 3: without a flows folder, or without the root, Run keeps
// writing; only a root with nothing to watch logs the watch as off, once.
func TestRunForemanRootMissing(t *testing.T) {
	for _, c := range []struct {
		name      string
		sprints   bool
		offLines  int
		foremanOK bool
	}{
		{"no flows", true, 0, true},
		{"missing", false, 1, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "foreman")
			if c.sprints {
				writeFile(t, filepath.Join(root, "sprints", "p", "s", "sprint.json"), sprintBody("running"))
			}
			var buf bytes.Buffer
			log.SetOutput(&buf)
			defer log.SetOutput(os.Stderr)
			w, writes := foremanWriter(t, root, 50*time.Millisecond)
			stop := startRun(t, w, filepath.Join(t.TempDir(), "snapshot.lock"))
			ok := eventually(2*time.Second, func() bool { return writes.Load() >= 3 })
			stop()
			log.SetOutput(os.Stderr)
			if !ok {
				t.Fatalf("%d writes in 2 s with a 50 ms tick", writes.Load())
			}
			if n := strings.Count(buf.String(), "Foreman watch off"); n != c.offLines {
				t.Fatalf("watch-off line logged %d times, want %d; log:\n%s", n, c.offLines, buf.String())
			}
			s, err := snapshot.Read(w.Path)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, src := range s.Sources {
				if src.Name == graph.SourceForeman {
					found = true
					if src.OK != c.foremanOK {
						t.Fatalf("foreman source %+v, want ok %v", src, c.foremanOK)
					}
				}
			}
			if !found {
				t.Fatalf("no foreman source in %+v", s.Sources)
			}
		})
	}
}

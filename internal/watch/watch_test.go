package watch

import (
	"os"
	"runtime"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

// changeCollector is a thread-safe OnChange sink for tests.
type changeCollector struct {
	mu    sync.Mutex
	paths []string
}

func (c *changeCollector) onChange(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paths = append(c.paths, path)
}

func (c *changeCollector) waitFor(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		for _, p := range c.paths {
			if p == path {
				c.mu.Unlock()
				return
			}
		}
		c.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for a change notification on %s", path)
}

func TestWatcher_NativeRootSeesNewFile(t *testing.T) {
	dir := t.TempDir()
	c := &changeCollector{}
	w, err := New([]string{dir}, nil, c.onChange)
	if err != nil {
		t.Fatal(err)
	}
	w.Start()
	defer w.Stop()

	path := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(path, []byte(`{"a":1}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.waitFor(t, path, 3*time.Second)
}

func TestWatcher_NativeRootSeesNewSubdirectory(t *testing.T) {
	dir := t.TempDir()
	c := &changeCollector{}
	w, err := New([]string{dir}, nil, c.onChange)
	if err != nil {
		t.Fatal(err)
	}
	w.Start()
	defer w.Stop()

	sub := filepath.Join(dir, "project-a")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// Give the watcher a moment to notice the new directory and start
	// watching it before a file lands inside it, mirroring how a fresh
	// Claude Code project folder is created just before its first session
	// file.
	time.Sleep(200 * time.Millisecond)

	path := filepath.Join(sub, "session.jsonl")
	if err := os.WriteFile(path, []byte(`{"a":1}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.waitFor(t, path, 3*time.Second)
}

// TestWatcher_NewNestedDayFolderRace reproduces F7 hypothesis 3
// (SESSION_LOG.md, v0.1.2): Codex creates its whole YYYY/MM/DD path with one
// MkdirAll, then writes the rollout file immediately after, with no pause
// between mkdir and file create the way TestWatcher_NativeRootSeesNewSubdirectory
// gets (200ms, modelled on Claude Code's own folder-then-file timing). If the
// fsnotify watch on the brand-new leaf directory is not yet registered when
// the file's own Create event fires, that Create is never delivered at all
// (Windows ReadDirectoryChanges is per-directory, not recursive), and the
// session is invisible until the next full rescan.
func TestWatcher_NewNestedDayFolderRace(t *testing.T) {
	dir := t.TempDir()
	c := &changeCollector{}
	w, err := New([]string{dir}, nil, c.onChange)
	if err != nil {
		t.Fatal(err)
	}
	w.Start()
	defer w.Stop()

	day := filepath.Join(dir, "2026", "09", "22")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(day, "rollout-2026-09-22T15-00-00-test.jsonl")
	if err := os.WriteFile(path, []byte(`{"a":1}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.waitFor(t, path, 2*time.Second)
}

// TestWatcher_WatchCountStaysOneAcrossManySubdirectories is WS3 hypothesis 1
// (02_roadmap\2026-09-26_ws3_shared_ingest_performance.md): the old design
// opened one fsnotify watch per subdirectory (13,125 of a real process's
// 13,541 handles, measured against ~12,700 Cowork session folders). A
// recursive watch on the root alone must still see a file created inside a
// brand-new, several-levels-deep subdirectory, without WatchCount ever
// growing past 1. Windows-specific by nature (watch_windows.go's one
// handle per root); on the !windows fsnotify backend (B1, untested on real
// hardware, out of WS3's scope) WatchCount grows with subdirectory count as
// it always did, so this assertion only actually runs meaningfully on the
// Windows binary go test ./... -count=1 builds and executes on this laptop.
func TestWatcher_WatchCountStaysOneAcrossManySubdirectories(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("one handle per root is the Windows watcher only; fsnotify watches each subdirectory")
	}
	dir := t.TempDir()
	c := &changeCollector{}
	w, err := New([]string{dir}, nil, c.onChange)
	if err != nil {
		t.Fatal(err)
	}
	w.Start()
	defer w.Stop()

	if got := w.WatchCount(); got != 1 {
		t.Fatalf("WatchCount right after New: want 1, got %d", got)
	}

	for i := 0; i < 50; i++ {
		sub := filepath.Join(dir, "proj", "2026", "09", "26", "session-"+strconv.Itoa(i))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(sub, "session.jsonl")
		if err := os.WriteFile(path, []byte(`{"a":1}`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		c.waitFor(t, path, 2*time.Second)
	}

	if got := w.WatchCount(); got != 1 {
		t.Fatalf("WatchCount after 50 new nested subdirectories: want 1, got %d", got)
	}
}

func TestWatcher_PollsWSLRootOnAppend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	if err := os.WriteFile(path, []byte(`{"a":1}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &changeCollector{}
	w, err := New(nil, []string{dir}, c.onChange)
	if err != nil {
		t.Fatal(err)
	}
	// pollWSLOnce, called directly rather than through the 5-second ticker,
	// so the test does not need to sleep for a real poll cycle.
	w.pollWSLOnce()
	c.waitFor(t, path, time.Second)

	// A second poll with no further change must not re-fire.
	w.pollWSLOnce()
	c.mu.Lock()
	n := len(c.paths)
	c.mu.Unlock()
	if n != 1 {
		t.Fatalf("want exactly 1 notification for an unchanged file, got %d", n)
	}

	// Append: mtime moves forward, so it must fire again.
	time.Sleep(10 * time.Millisecond) // ensure a distinct mtime on filesystems with coarse resolution
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"a":2}` + "\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	w.pollWSLOnce()
	c.mu.Lock()
	n = len(c.paths)
	c.mu.Unlock()
	if n != 2 {
		t.Fatalf("want 2 notifications after an append, got %d", n)
	}
}

// TestTail_HeldOpenFileAppends guards the WS2 follow-up's item 2: a file
// its writer keeps open (Codex holds a rollout open for the whole chat)
// must still reach onChange on every append, through the tail poll alone.
// The watcher gets no native backend root, so no directory notification
// can satisfy this test; only pollTailOnce can.
func TestTail_HeldOpenFileAppends(t *testing.T) {
	dir := t.TempDir()
	c := &changeCollector{}
	w, err := New(nil, nil, c.onChange)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	w.nativeRoots = []string{dir} // tail-only: no ReadDirectoryChangesW handle on dir

	path := filepath.Join(dir, "rollout-open.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() // held open, like Codex, for the whole test

	count := func() int {
		c.mu.Lock()
		defer c.mu.Unlock()
		return len(c.paths)
	}
	w.Track(path)
	w.Track(filepath.Join(t.TempDir(), "elsewhere.jsonl")) // outside the root, ignored

	w.pollTailOnce() // first look fires once, so the caller ingests from its cursor
	if n := count(); n != 1 {
		t.Fatalf("after first look: %d calls, want 1", n)
	}
	for turn := 1; turn <= 3; turn++ {
		if _, err := f.WriteString(`{"turn":` + strconv.Itoa(turn) + "}\n"); err != nil {
			t.Fatal(err)
		}
		w.pollTailOnce()
		if n := count(); n != 1+turn {
			t.Fatalf("after append %d: %d calls, want %d", turn, n, 1+turn)
		}
	}
	w.pollTailOnce() // no append: no call
	if n := count(); n != 4 {
		t.Fatalf("idle tick fired: %d calls, want 4", n)
	}
}

// TestTail_TickerSeesHeldOpenAppend runs the real 2-second ticker end to
// end against a held-open file.
func TestTail_TickerSeesHeldOpenAppend(t *testing.T) {
	dir := t.TempDir()
	c := &changeCollector{}
	w, err := New([]string{dir}, nil, c.onChange)
	if err != nil {
		t.Fatal(err)
	}
	w.Start()
	defer w.Stop()

	path := filepath.Join(dir, "rollout-open.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w.Track(path)
	time.Sleep(tailInterval + 500*time.Millisecond) // first look done
	c.mu.Lock()
	c.paths = nil
	c.mu.Unlock()
	if _, err := f.WriteString(`{"turn":1}` + "\n"); err != nil {
		t.Fatal(err)
	}
	c.waitFor(t, path, 2*tailInterval+time.Second)
}

func TestTail_CapKeepsMostRecent(t *testing.T) {
	dir := t.TempDir()
	c := &changeCollector{}
	w, err := New(nil, nil, c.onChange)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	w.nativeRoots = []string{dir}
	base := time.Now()
	for i := 0; i < tailCap+5; i++ {
		p := filepath.Join(dir, "f"+strconv.Itoa(i)+".jsonl")
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		w.tail[p] = &tailEntry{active: base.Add(time.Duration(i) * time.Second), size: -1}
	}
	stale := filepath.Join(dir, "stale.jsonl")
	w.tail[stale] = &tailEntry{active: base.Add(-2 * tailWindow), size: -1}
	w.pollTailOnce()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.paths) != tailCap {
		t.Fatalf("%d files polled, want the cap %d", len(c.paths), tailCap)
	}
	for _, p := range c.paths {
		if p == filepath.Join(dir, "f0.jsonl") || p == stale {
			t.Fatalf("%s polled, want it dropped (oldest past the cap, or expired)", p)
		}
	}
	if _, ok := w.tail[stale]; ok {
		t.Fatal("expired entry still tracked")
	}
}

// TestWatcher_SkipsNonTranscripts guards the 2026-10-12 double count: a
// local Cowork session's audit.jsonl, and scratch copies under outputs\ or
// uploads\, must never reach onChange or the tail poll. FindJSONL already
// skipped them; the live path did not.
func TestWatcher_SkipsNonTranscripts(t *testing.T) {
	dir := t.TempDir()
	c := &changeCollector{}
	w, err := New([]string{dir}, nil, c.onChange)
	if err != nil {
		t.Fatal(err)
	}
	w.Start()
	defer w.Stop()

	skip := []string{
		filepath.Join(dir, "audit.jsonl"),
		filepath.Join(dir, "outputs", "copy.jsonl"),
		filepath.Join(dir, "uploads", "copy.jsonl"),
	}
	for _, p := range skip {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(200 * time.Millisecond)
	for _, p := range skip {
		if err := os.WriteFile(p, []byte(`{"a":1}`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	real := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(real, []byte(`{"a":1}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.waitFor(t, real, 3*time.Second)
	time.Sleep(300 * time.Millisecond)

	c.mu.Lock()
	defer c.mu.Unlock()
	for _, got := range c.paths {
		for _, p := range skip {
			if got == p {
				t.Errorf("onChange fired for %s, want it skipped", p)
			}
		}
	}

	w.Track(skip[0])
	w.tailMu.Lock()
	_, tracked := w.tail[skip[0]]
	w.tailMu.Unlock()
	if tracked {
		t.Errorf("Track accepted %s, want it skipped", skip[0])
	}
}

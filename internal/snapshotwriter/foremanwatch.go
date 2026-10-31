package snapshotwriter

import (
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"burnmon/internal/watch"
)

// foremanDebounce gathers one burst of Foreman writes into one read: a
// sprint_step.py call writes action-log.ndjson, sprint.json and the board
// together, and Windows reports one save as several changes. The window
// starts on the first change and later ones do not move it, so a sprint
// that writes every 100 ms cannot hold a gate back (Ruling 1).
const foremanDebounce = 200 * time.Millisecond

// foremanFiles are the base names graph.ReadForeman reads.
var foremanFiles = map[string]bool{
	"sprint.json": true, "action-log.ndjson": true, "report.md": true,
	"flow.json": true, "foreman.json": true,
}

// ForemanFile reports whether path is a file graph.ReadForeman reads. It runs
// on the watcher's I/O goroutine before the read is re-armed, so it looks at
// the base name only and never touches the disk. index.json, index.html and
// the board files never match, and neither does a folder.
func ForemanFile(path string) bool {
	return foremanFiles[strings.ToLower(filepath.Base(path))]
}

// coalescer calls fire once, d after the first Trigger of a quiet period.
// Later triggers in that window join it and do not reset the timer.
type coalescer struct {
	d    time.Duration
	fire func()

	mu      sync.Mutex
	timer   *time.Timer // armed window, nil when quiet
	stopped bool
}

func newCoalescer(d time.Duration, fire func()) *coalescer {
	return &coalescer{d: d, fire: fire}
}

// Trigger arms a fire d from now unless one is already armed. It never
// blocks, so it is safe on the watcher's I/O goroutine.
func (c *coalescer) Trigger() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped || c.timer != nil {
		return
	}
	c.timer = time.AfterFunc(c.d, c.run)
}

// run ends the window before it fires, so a change that lands while fire
// runs arms a new window instead of being lost.
func (c *coalescer) run() {
	c.mu.Lock()
	c.timer = nil
	stopped := c.stopped
	c.mu.Unlock()
	if !stopped {
		c.fire()
	}
}

// Stop drops an armed fire and makes every later Trigger a no-op.
func (c *coalescer) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = true
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
}

// Kick asks Run for a write now, with a fresh Foreman read. It never blocks:
// a kick still pending covers this one. Before Run starts it does nothing.
func (w *Writer) Kick() {
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

// forceForeman makes the next Foreman read due at once. Call it on Run's
// goroutine, the one Once runs on, so foremanAt needs no lock.
func (gr *GraphReaders) forceForeman() {
	if gr != nil {
		gr.foremanAt = time.Time{}
	}
}

// watchForeman watches Foreman's sprints and flows folders for Run and kicks
// a write foremanDebounce after a change. It returns the stop func, or nil
// when there is no Foreman root or not one folder could be watched; the
// foremanEvery read stays the fallback either way.
func (w *Writer) watchForeman() func() {
	if w.Graph == nil || w.Graph.ForemanRoot == "" {
		return nil
	}
	root := w.Graph.ForemanRoot
	c := newCoalescer(foremanDebounce, w.Kick)
	stop, err := watch.WatchFiles([]string{filepath.Join(root, "sprints"), filepath.Join(root, "flows")},
		ForemanFile, func(string) { c.Trigger() })
	if err != nil {
		c.Stop()
		log.Printf("snapshot: Foreman watch off, Foreman is read every %v: %v", foremanEvery, err)
		return nil
	}
	return func() {
		stop()
		c.Stop()
	}
}

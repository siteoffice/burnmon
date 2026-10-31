//go:build windows

// toast_watch.go feeds the snapshot's Waits on you to the toast rules
// (internal/toast) and shows what they return. It runs on its own goroutine
// and reads the snapshot file only: it never touches the WebView.
package main

import (
	"time"

	"burnmon/internal/snapshot"
	"burnmon/internal/snapshotwriter"
	"burnmon/internal/toast"
	"burnmon/internal/waits"
)

var procIsIconic = user32.NewProc("IsIconic")

const swRestore = 9 // SW_RESTORE

// toastTick steps the rules on one reading (see toastReadable for which).
// With enabled false the rules still step, so turning toasts on later does
// not toast what waited.
func toastTick(g *toast.Gate, r snapshot.Reading, now time.Time, enabled bool) (string, bool) {
	if !toastReadable(r) {
		return "", false
	}
	text, ok := g.Step(now, r.Snapshot.WaitItems)
	if !enabled {
		return "", false
	}
	return text, ok
}

// toastReadable: only a current snapshot steps the rules. A missing, stale or
// unreadable file says nothing about what waits, and stepping on it would
// make every waiting item look new on the next one. The same goes for a
// snapshot written before the writer's first agent view poll, whose agent
// view items arrive one write later.
func toastReadable(r snapshot.Reading) bool {
	return r.Status == snapshot.StatusOK && r.Snapshot != nil && !agentViewPolling(r.Snapshot)
}

// agentViewPolling: the writer runs the agent view and its first poll has not
// finished yet.
func agentViewPolling(s *snapshot.Snapshot) bool {
	for _, src := range s.Sources {
		if src.Name == waits.SourceAgentView && src.Error == snapshotwriter.AgentViewFirstPoll {
			return true
		}
	}
	return false
}

// toastWatch reads the snapshot file again only when its stamp changed.
//
// Until this process's startup backfill is done, the store lacks what
// happened while the app was closed, so a snapshot then misses those
// sessions and their waits arrive later, all at once. The rules therefore
// learn without speaking until they have stepped on a snapshot written after
// the backfill: that one is the real baseline, and it is quiet too.
type toastWatch struct {
	path       string
	setDir     string          // holds burnmon-dev.json, the one source of toast_enabled
	backfilled <-chan struct{} // closed when the startup backfill is done
	show       func(string)

	gate    toast.Gate
	stamp   string
	doneAt  time.Time // when poll first saw backfilled closed
	settled bool      // the rules stepped on a snapshot written after doneAt
}

func (tw *toastWatch) poll(now time.Time) {
	if tw.doneAt.IsZero() {
		select {
		case <-tw.backfilled:
			tw.doneAt = now // at or after the real end, never before it
		default:
		}
	}
	s := snapshotStamp(tw.path)
	if s == "" || s == tw.stamp {
		return
	}
	tw.stamp = s
	r := snapshot.Load(tw.path, now)
	// The file is read on every new snapshot, so a hand edit counts at once.
	on := tw.settled && loadDevConfig(tw.setDir).ToastEnabled
	if text, ok := toastTick(&tw.gate, r, now, on); ok {
		tw.show(text)
	}
	if !tw.settled && toastReadable(r) && writtenAfter(r.Snapshot, tw.doneAt) {
		tw.settled = true
	}
}

// writtenAfter reports whether s was written after t (zero t: not yet).
// The writer takes written_at before it reads the store, so a snapshot
// written after the backfill read a caught-up store. That holds when
// burnmon.exe wrote it as well: both apps share one burnmon.db and one
// clock, so what this process backfilled is in the store burnmon.exe reads.
// written_at is cut to whole seconds, so it must be strictly after t: a
// write in the second the backfill ended may come before it.
func writtenAfter(s *snapshot.Snapshot, t time.Time) bool {
	if t.IsZero() {
		return false
	}
	at, err := time.Parse(time.RFC3339, s.WrittenAt)
	return err == nil && at.After(t)
}

// startToastWatch checks the snapshot every second, like the page does.
func startToastWatch(path, setDir string, backfilled <-chan struct{}, show func(string)) {
	tw := &toastWatch{path: path, setDir: setDir, backfilled: backfilled, show: show}
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			tw.poll(time.Now())
		}
	}()
}

// bringToFront is a toast click: restore the window when it is minimized,
// then raise it.
func bringToFront(hwnd uintptr) {
	if r, _, _ := procIsIconic.Call(hwnd); r != 0 {
		procShowWindowWS.Call(hwnd, swRestore)
	}
	procSetForegroundWindow.Call(hwnd)
}

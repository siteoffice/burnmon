//go:build windows

// toast_watch.go feeds the snapshot's Waits on you to the toast rules
// (internal/toast) and shows what they return. It runs on its own goroutine
// and reads the snapshot file only: it never touches the WebView.
package main

import (
	"sync/atomic"
	"time"

	"burnmon/internal/snapshot"
	"burnmon/internal/snapshotwriter"
	"burnmon/internal/toast"
	"burnmon/internal/waits"
)

var procIsIconic = user32.NewProc("IsIconic")

const swRestore = 9 // SW_RESTORE

// toastTick steps the rules on one reading. Only a current snapshot steps
// them: a missing, stale or unreadable file says nothing about what waits,
// and stepping on it would make every waiting item look new on the next one.
// The same goes for a snapshot written before the writer's first agent view
// poll, whose agent view items arrive one write later. With enabled false the
// rules still step, so turning toasts on later does not toast what waited.
func toastTick(g *toast.Gate, r snapshot.Reading, now time.Time, enabled bool) (string, bool) {
	if r.Status != snapshot.StatusOK || r.Snapshot == nil || agentViewPolling(r.Snapshot) {
		return "", false
	}
	text, ok := g.Step(now, r.Snapshot.WaitItems)
	if !enabled {
		return "", false
	}
	return text, ok
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
type toastWatch struct {
	path  string
	on    *atomic.Bool
	show  func(string)
	gate  toast.Gate
	stamp string
}

func (tw *toastWatch) poll(now time.Time) {
	s := snapshotStamp(tw.path)
	if s == "" || s == tw.stamp {
		return
	}
	tw.stamp = s
	if text, ok := toastTick(&tw.gate, snapshot.Load(tw.path, now), now, tw.on.Load()); ok {
		tw.show(text)
	}
}

// startToastWatch checks the snapshot every second, like the page does.
func startToastWatch(path string, on *atomic.Bool, show func(string)) {
	tw := &toastWatch{path: path, on: on, show: show}
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

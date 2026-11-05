//go:build windows || darwin

package main

import (
	"log"
	"sync"

	"burnmon/internal/toast"
)

// notifier is what the lazy start needs from internal/toast.
type notifier interface {
	Show(text string)
	Close()
}

// toastStart wraps toast.Start. It returns a nil interface, not a typed nil,
// on error. Tests swap it.
var toastStart = func(appName string, onClick func()) (notifier, error) {
	n, err := toast.Start(appName, onClick)
	if err != nil {
		return nil, err
	}
	return n, nil
}

// lazyToast starts the notifier on the first banner, so a run with toasts off
// never starts it, and a run that turns them on later still shows the next one.
type lazyToast struct {
	appName string
	onClick func()
	mu      sync.Mutex
	n       notifier
	failed  bool
}

func (l *lazyToast) Show(text string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.n == nil && !l.failed {
		n, err := toastStart(l.appName, l.onClick)
		if err != nil {
			log.Println("toasts off for this run:", err)
			l.failed = true
		} else {
			l.n = n
		}
	}
	if l.n != nil {
		l.n.Show(text)
	}
}

func (l *lazyToast) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.n != nil {
		l.n.Close()
	}
}

// startLazyToasts watches the snapshot and starts the notifier on the first
// banner. A click does nothing (onClick is nil). The returned stop closes it.
func startLazyToasts(snapPath, setDir string, backfilled <-chan struct{}, onClick func()) (stop func()) {
	l := &lazyToast{appName: "BurnMon Dev", onClick: onClick}
	startToastWatch(snapPath, setDir, backfilled, l.Show)
	return l.Close
}

//go:build darwin

package toast

import (
	"context"
	"log"
	"os/exec"
	"sync"
	"time"
)

// runOsascript runs /usr/bin/osascript with args. It is a seam for tests.
var runOsascript = func(ctx context.Context, args []string) error {
	return exec.CommandContext(ctx, osascriptPath, args...).Run()
}

// showTimeout bounds one osascript call.
var showTimeout = 5 * time.Second

// Notifier shows macOS notifications through osascript.
type Notifier struct {
	appName string

	mu      sync.Mutex
	closed  bool
	errOnce sync.Once
}

// Start returns a Notifier titled appName. The error is always nil.
// onClick is never called on macOS: osascript gives no click callback.
func Start(appName string, onClick func()) (*Notifier, error) {
	return &Notifier{appName: appName}, nil
}

// Show posts a notification without blocking the caller. Empty text and a
// closed Notifier are ignored. The first failure is logged once.
func (n *Notifier) Show(text string) {
	if text == "" {
		return
	}
	n.mu.Lock()
	closed := n.closed
	n.mu.Unlock()
	if closed {
		return
	}
	args := osascriptArgs(text, n.appName)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), showTimeout)
		defer cancel()
		if err := runOsascript(ctx, args); err != nil {
			n.errOnce.Do(func() { log.Printf("toast: osascript: %v", err) })
		}
	}()
}

// Close stops further notifications. It is safe to call more than once.
func (n *Notifier) Close() {
	n.mu.Lock()
	n.closed = true
	n.mu.Unlock()
}

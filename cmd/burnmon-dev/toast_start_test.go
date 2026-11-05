//go:build windows || darwin

package main

import (
	"errors"
	"os"
	"testing"
	"time"
)

type fakeNotifier struct {
	shown  *[]string
	closed *int
}

func (f fakeNotifier) Show(text string) { *f.shown = append(*f.shown, text) }
func (f fakeNotifier) Close()           { *f.closed++ }

// swapToastStart replaces toastStart with a recorder for the test.
func swapToastStart(t *testing.T, fn func(string, func()) (notifier, error)) {
	t.Helper()
	old := toastStart
	toastStart = fn
	t.Cleanup(func() { toastStart = old })
}

// TestLazyToastStartsOnlyWhenEnabled: with toast_enabled false at start the
// notifier is never started; switching it on later starts it for the next banner.
func TestLazyToastStartsOnlyWhenEnabled(t *testing.T) {
	calls := 0
	shown := []string{}
	closed := 0
	swapToastStart(t, func(string, func()) (notifier, error) {
		calls++
		return fakeNotifier{&shown, &closed}, nil
	})
	l := &lazyToast{appName: "BurnMon Dev"}
	done := make(chan struct{})
	close(done)
	tw, _ := newToastWatch(t, done)
	tw.show = l.Show
	a, b, c, d := waitGate("a", "A"), waitGate("b", "B"), waitGate("c", "C"), waitGate("d", "D")
	if err := os.WriteFile(devJSONPath(tw.setDir), []byte(`{"toast_enabled": false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tw.poll(toastT0) // the backfill is seen done here
	writeToastSnapshot(t, tw.path, toastT0.Add(time.Second), a)
	tw.poll(toastT0.Add(1500 * time.Millisecond))
	writeToastSnapshot(t, tw.path, toastT0.Add(3*time.Second), a, b)
	tw.poll(toastT0.Add(3500 * time.Millisecond))
	if calls != 0 {
		t.Fatalf("start calls = %d with toasts off, want 0", calls)
	}
	if err := os.WriteFile(devJSONPath(tw.setDir), []byte(`{"toast_enabled": true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	writeToastSnapshot(t, tw.path, toastT0.Add(5*time.Second), a, b, c)
	tw.poll(toastT0.Add(5500 * time.Millisecond))
	if calls != 1 || len(shown) != 1 || shown[0] != "Gate: C" {
		t.Fatalf("calls = %d, shown = %q, want 1 and [Gate: C]", calls, shown)
	}
	writeToastSnapshot(t, tw.path, toastT0.Add(7*time.Second), a, b, c, d)
	tw.poll(toastT0.Add(7500 * time.Millisecond))
	if calls != 1 || len(shown) != 2 || shown[1] != "Gate: D" {
		t.Fatalf("calls = %d, shown = %q, want 1 and [Gate: C Gate: D]", calls, shown)
	}
	l.Close()
	if closed != 1 {
		t.Fatalf("closed = %d, want 1", closed)
	}
}

func TestLazyToastStartFailureIsQuiet(t *testing.T) {
	calls := 0
	swapToastStart(t, func(string, func()) (notifier, error) {
		calls++
		return nil, errors.New("no osascript")
	})
	l := &lazyToast{appName: "BurnMon Dev"}
	l.Show("one")
	l.Show("two")
	if calls != 1 {
		t.Fatalf("start calls = %d after a failure, want 1 (no retry)", calls)
	}
	l.Close()
}

func TestLazyToastCloseBeforeStart(t *testing.T) {
	(&lazyToast{}).Close()
}

func TestStartLazyToastsPassesNilOnClick(t *testing.T) {
	var gotClick func()
	called := false
	swapToastStart(t, func(_ string, onClick func()) (notifier, error) {
		called = true
		gotClick = onClick
		return fakeNotifier{&[]string{}, new(int)}, nil
	})
	// Built as startLazyToasts builds it, with a nil onClick, without the watch.
	var onClick func()
	l := &lazyToast{appName: "BurnMon Dev", onClick: onClick}
	l.Show("x")
	if !called || gotClick != nil {
		t.Fatalf("called = %v, onClick nil = %v, want true and true", called, gotClick == nil)
	}
	l.Close()
}

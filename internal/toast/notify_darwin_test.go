package toast

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

// stubSeam swaps runOsascript for the test and restores it afterwards.
func stubSeam(t *testing.T, f func(ctx context.Context, args []string) error) {
	t.Helper()
	old := runOsascript
	runOsascript = f
	t.Cleanup(func() { runOsascript = old })
}

func TestShowCallsSeamOnceWithArgs(t *testing.T) {
	got := make(chan []string, 4)
	stubSeam(t, func(ctx context.Context, args []string) error {
		got <- args
		return nil
	})
	n, err := Start("BurnMon", func() {})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer n.Close()
	n.Show("hello")
	select {
	case args := <-got:
		if want := osascriptArgs("hello", "BurnMon"); !reflect.DeepEqual(args, want) {
			t.Fatalf("args = %q, want %q", args, want)
		}
		n := len(args)
		if n < 3 || args[n-3] != "--" || args[n-2] != "BurnMon" || args[n-1] != "hello" {
			t.Fatalf("tail must be -- title text, got %q", args)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("seam not called")
	}
	select {
	case args := <-got:
		t.Fatalf("second call: %q", args)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestShowHangingSeamIsCutOff(t *testing.T) {
	oldTimeout := showTimeout
	showTimeout = 50 * time.Millisecond
	t.Cleanup(func() { showTimeout = oldTimeout })
	done := make(chan error, 1)
	stubSeam(t, func(ctx context.Context, args []string) error {
		<-ctx.Done()
		done <- ctx.Err()
		return ctx.Err()
	})
	n, _ := Start("BurnMon", nil)
	defer n.Close()
	n.Show("hang")
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected context error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("seam was not cut off by showTimeout")
	}
}

func TestShowEmptyTextNoCall(t *testing.T) {
	var calls int32
	stubSeam(t, func(ctx context.Context, args []string) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})
	n, _ := Start("BurnMon", nil)
	defer n.Close()
	n.Show("")
	time.Sleep(100 * time.Millisecond)
	if c := atomic.LoadInt32(&calls); c != 0 {
		t.Fatalf("calls = %d, want 0", c)
	}
}

func TestShowAfterCloseNoCall(t *testing.T) {
	var calls int32
	stubSeam(t, func(ctx context.Context, args []string) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})
	n, _ := Start("BurnMon", nil)
	n.Close()
	n.Close()
	n.Show("late")
	time.Sleep(100 * time.Millisecond)
	if c := atomic.LoadInt32(&calls); c != 0 {
		t.Fatalf("calls = %d, want 0", c)
	}
}

func TestOnClickNeverCalled(t *testing.T) {
	var clicks int32
	stubSeam(t, func(ctx context.Context, args []string) error { return nil })
	n, _ := Start("BurnMon", func() { atomic.AddInt32(&clicks, 1) })
	n.Show("hello")
	time.Sleep(100 * time.Millisecond)
	n.Close()
	if c := atomic.LoadInt32(&clicks); c != 0 {
		t.Fatalf("onClick called %d times", c)
	}
}

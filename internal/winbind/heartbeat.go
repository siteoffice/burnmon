package winbind

import "time"

// StartHeartbeat dispatches an empty func to the window's UI thread once a
// second, for the life of the process (found in the 90-minute after run on
// 2026-09-29): go-webview2's Dispatch queues its func and wakes the UI thread
// with one PostThreadMessage(WM_APP), which only w.Run's own GetMessage loop
// handles. A Win32 modal loop (a window drag or resize, a menu) pulls that
// thread message off the queue and drops it, so the queued func, say a
// binding's resolve, waits until some later Dispatch gets through: 67 s on
// 2026-09-29, and forever when nothing else dispatched while the page waited
// on that one call. A no-op Dispatch every second means a lost wakeup costs
// at most a second once the modal loop has ended (inside it, the heartbeat's
// own WM_APP is dropped too), for every binding, not just the render tick.
func StartHeartbeat(w Window) {
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			w.Dispatch(func() {})
		}
	}()
}

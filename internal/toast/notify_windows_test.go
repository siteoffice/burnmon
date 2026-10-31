//go:build windows

package toast

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"

	"burnmon/internal/waits"
)

var procFindWindowW = user32.NewProc("FindWindowW")

// TestNotifyIconDataLayout: Shell_NotifyIconW reads the struct by offset, so
// one misplaced field makes every call fail. The numbers are those of
// NOTIFYICONDATAW on 64-bit Windows.
func TestNotifyIconDataLayout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("the offsets below are for 64-bit Windows")
	}
	var d notifyIconData
	for _, c := range []struct {
		name      string
		got, want uintptr
	}{
		{"size", unsafe.Sizeof(d), 976},
		{"hWnd", unsafe.Offsetof(d.HWnd), 8},
		{"uID", unsafe.Offsetof(d.UID), 16},
		{"uCallbackMessage", unsafe.Offsetof(d.UCallbackMessage), 24},
		{"hIcon", unsafe.Offsetof(d.HIcon), 32},
		{"szTip", unsafe.Offsetof(d.SzTip), 40},
		{"szInfo", unsafe.Offsetof(d.SzInfo), 304},
		{"uVersion", unsafe.Offsetof(d.UVersion), 816},
		{"szInfoTitle", unsafe.Offsetof(d.SzInfoTitle), 820},
		{"dwInfoFlags", unsafe.Offsetof(d.DwInfoFlags), 948},
		{"hBalloonIcon", unsafe.Offsetof(d.HBalloonIcon), 968},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
	if len(d.SzTip) != 128 || len(d.SzInfo) != 256 || len(d.SzInfoTitle) != 64 {
		t.Errorf("field lengths tip %d info %d title %d, want 128 256 64", len(d.SzTip), len(d.SzInfo), len(d.SzInfoTitle))
	}
}

// fieldText is the text in a NUL-terminated UTF-16 field.
func fieldText(t *testing.T, u []uint16) string {
	t.Helper()
	n := 0
	for n < len(u) && u[n] != 0 {
		n++
	}
	if n == len(u) {
		t.Fatal("the field has no terminating NUL")
	}
	return string(utf16.Decode(u[:n]))
}

// checkCut: the field holds a clean cut of s, never half a surrogate pair
// (utf16.Decode turns a lone half into U+FFFD), ending in "...".
func checkCut(t *testing.T, field []uint16, s string) {
	t.Helper()
	got := fieldText(t, field)
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Fatalf("cut split a surrogate pair: %q", got)
	}
	if !strings.HasSuffix(got, "...") || !strings.HasPrefix(s, strings.TrimSuffix(got, "...")) {
		t.Fatalf("cut text %q is not a prefix of the input plus ...", got)
	}
	if n := len(utf16.Encode([]rune(got))); n > len(field)-1 {
		t.Fatalf("cut text is %d units, the field holds %d plus the NUL", n, len(field)-1)
	}
}

// TestPutUTF16: szInfo holds 256 UTF-16 units with its NUL, and 200 runes of
// emoji need 400, so the text is cut in units, on a rune boundary.
func TestPutUTF16(t *testing.T) {
	var d notifyIconData
	putUTF16(d.SzInfo[:], "Gate: ship it")
	if got := fieldText(t, d.SzInfo[:]); got != "Gate: ship it" {
		t.Fatalf("short text = %q", got)
	}
	exact := strings.Repeat("x", len(d.SzInfo)-1)
	putUTF16(d.SzInfo[:], exact)
	if got := fieldText(t, d.SzInfo[:]); got != exact {
		t.Fatalf("a text that just fits was cut to %d runes", len([]rune(got)))
	}
	// With and without one ASCII rune in front, the cut lands once on the
	// first and once on the second half of a pair.
	for _, lead := range []string{"", "a"} {
		s := lead + strings.Repeat("\U0001F600", MaxText)
		putUTF16(d.SzInfo[:], s)
		checkCut(t, d.SzInfo[:], s)
		putUTF16(d.SzInfoTitle[:], s)
		checkCut(t, d.SzInfoTitle[:], s)
		putUTF16(d.SzTip[:], s)
		checkCut(t, d.SzTip[:], s)
	}
	// The real path: a 500-emoji title through the rules, then into szInfo.
	var g Gate
	t0 := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	g.Step(t0, nil)
	text, ok := g.Step(t0.Add(time.Second), []waits.Item{{Kind: waits.KindGate, ID: "g1", Group: waits.GroupBlocked, Title: strings.Repeat("\U0001F600", 500)}})
	if !ok {
		t.Fatal("the rules gave no toast for a new gate")
	}
	if n := len(utf16.Encode([]rune(text))); n <= len(d.SzInfo)-1 {
		t.Fatalf("rules text is %d units and fits already; the test needs one that does not", n)
	}
	putUTF16(d.SzInfo[:], text)
	checkCut(t, d.SzInfo[:], text)
}

// TestTrayEvent: with NOTIFYICON_VERSION_4 the low word of lParam is the
// event and the high word the icon ID. Only the icon of the toast on show is
// removed; a late event of a replaced toast must not remove the new one.
func TestTrayEvent(t *testing.T) {
	lp := func(ev, id uint32) uintptr { return uintptr(id)<<16 | uintptr(ev) }
	for _, c := range []struct {
		name          string
		lp            uintptr
		click, remove bool
	}{
		{"click", lp(0x405, 7), true, true},
		{"timeout", lp(0x404, 7), false, true},
		{"hide", lp(0x403, 7), false, true},
		{"balloon show", lp(0x402, 7), false, false},
		{"mouse move", lp(0x200, 7), false, false},
		{"click on a replaced toast", lp(0x405, 6), true, false},
		{"timeout of a replaced toast", lp(0x404, 6), false, false},
		{"hide of a replaced toast", lp(0x403, 6), false, false},
	} {
		click, remove := trayEvent(c.lp, 7)
		if click != c.click || remove != c.remove {
			t.Errorf("%s: click %v remove %v, want %v %v", c.name, click, remove, c.click, c.remove)
		}
	}
}

// TestIconIDAlternates: Windows keeps one NotifyIconSettings entry per (exe,
// icon ID) and never prunes it, so the IDs stay bounded: 1, 2, 1, 2. A late
// event of the replaced toast carries the other ID and removes nothing.
func TestIconIDAlternates(t *testing.T) {
	var got []uint32
	last := uint32(0)
	for i := 0; i < 4; i++ {
		last = nextIconID(last)
		got = append(got, last)
	}
	want := []uint32{1, 2, 1, 2}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("icon IDs %v, want %v", got, want)
		}
	}
	lp := func(ev, id uint32) uintptr { return uintptr(id)<<16 | uintptr(ev) }
	// Toast on show has ID 2; the replaced one had ID 1.
	for _, ev := range []uint32{ninBalloonUserClick, ninBalloonTimeout, ninBalloonHide} {
		if _, remove := trayEvent(lp(ev, 1), 2); remove {
			t.Errorf("event %#x of the replaced toast (ID 1) removes the toast on show (ID 2)", ev)
		}
		if _, remove := trayEvent(lp(ev, 2), 2); !remove {
			t.Errorf("event %#x of the toast on show (ID 2) does not remove it", ev)
		}
	}
}

// TestStartCloseLeavesNoGoroutine: Close returns quickly and the window's
// goroutine is gone after it, twice in a row (the window class is
// registered once per process). Nothing is shown: the icon only appears on
// Show.
func TestStartCloseLeavesNoGoroutine(t *testing.T) {
	before := runtime.NumGoroutine()
	for i := 0; i < 2; i++ {
		start := time.Now()
		n, err := Start("BurnMon Dev test", func() {})
		if err != nil {
			t.Fatal(err)
		}
		n.Close()
		if d := time.Since(start); d > 2*time.Second {
			t.Fatalf("Start then Close took %v, want under 2 s", d)
		}
		// Show and Close after Close do nothing and do not block.
		n.Show("Gate: after close")
		n.Close()
	}
	// The goroutine has closed its done channel when Close returns; give the
	// runtime a moment to retire it.
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if g := runtime.NumGoroutine(); g > before {
		t.Fatalf("%d goroutines after Close, %d before Start", g, before)
	}
}

// TestWindowHasNoAppTitle: BurnMon Dev finds its own window by title with
// FindWindowW (a second start brings it forward, uicheck drives it). The
// hidden window must never be the one found, so it carries no title.
func TestWindowHasNoAppTitle(t *testing.T) {
	const app = "BurnMon Dev title probe"
	n, err := Start(app, func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	title, _ := windows.UTF16PtrFromString(app)
	if got, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title))); got != 0 {
		t.Fatalf("FindWindowW(%q) found window %#x (the notifier's is %#x), want none", app, got, n.hwnd)
	}
}

// TestLiveShow puts one real notification on the screen, so it runs only
// with BURNMON_TOAST_LIVE=1. It checks that every Shell_NotifyIconW call of
// the show returned success; whether the toast was seen is for a person.
func TestLiveShow(t *testing.T) {
	if os.Getenv("BURNMON_TOAST_LIVE") != "1" {
		t.Skip("set BURNMON_TOAST_LIVE=1 to show one real notification")
	}
	got := make(chan error, 1)
	n, err := start("BurnMon Dev", func() { t.Log("the toast was clicked") }, func(err error) { got <- err })
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	n.Show("Gate: BurnMon toast live test")
	select {
	case err := <-got:
		if err != nil {
			t.Fatal(err)
		}
		t.Log("Shell_NotifyIconW add, set version and modify returned success")
	case <-time.After(5 * time.Second):
		t.Fatal("no result from the window thread within 5 s")
	}
	// Leave it up long enough to be seen before Close removes the icon.
	time.Sleep(6 * time.Second)
}

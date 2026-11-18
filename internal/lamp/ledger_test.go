package lamp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burnmon/internal/fsutil"
	"burnmon/internal/snapshot"
	"burnmon/internal/waits"
)

const (
	markID      = "MARK_ID_7f3"
	markTitle   = "MARK_TITLE_7f3"
	markProject = "MARK_PROJECT_7f3"
	markPath    = "MARK_PATH_7f3"
)

// ledgerSnap is a snapshot written at at, with a 2 second tick and two
// counted live items (a question and a prompt) that hold the marker strings.
func ledgerSnap(at time.Time, ids ...string) *snapshot.Snapshot {
	if len(ids) == 0 {
		ids = []string{markID + "a", markID + "b"}
	}
	s := &snapshot.Snapshot{SchemaVersion: snapshot.SchemaVersion, WrittenAt: at.UTC().Format(time.RFC3339), TickSeconds: 2}
	for i, id := range ids {
		kind, group, age := waits.KindQuestion, waits.GroupBlocked, int64(600)
		if i%2 == 1 {
			kind, group, age = waits.KindPrompt, waits.GroupYourTurn, 100
		}
		s.WaitItems = append(s.WaitItems, waits.Item{
			Kind: kind, ID: id, Title: markTitle, Project: markProject, Detail: markPath,
			SpecPath: markPath, ResultPath: markPath, Group: group, AgeSeconds: age,
		})
	}
	return s
}

func ledgerPath(t *testing.T) string { return filepath.Join(t.TempDir(), "lamp-waits.json") }

func TestLedgerTwoDaysAndARestart(t *testing.T) {
	path := ledgerPath(t)
	set := DefaultSettings()
	l := LoadLedger(path)
	t0 := time.Date(2026, 3, 2, 23, 59, 50, 0, time.UTC)
	l.Observe(t0, ledgerSnap(t0), set) // first read: nothing to measure from
	t1 := t0.Add(2 * time.Second)
	l.Observe(t1, ledgerSnap(t1), set) // 2 s, 2 items
	t2 := t1.Add(100 * time.Second)
	l.Observe(t2, ledgerSnap(t2), set) // a gap: counts at most 3 ticks, 6 s, 2 items; the day turned at 00:00
	l.Close(t2)

	d1 := l.Days()["2026-03-02"]
	if d1.AgentWaitSeconds != 4 || d1.LampOnSeconds != 2 || d1.LongestWaitSeconds != 600 || len(d1.Items) != 2 {
		t.Fatalf("day 1: %+v", d1)
	}
	d2 := l.Days()["2026-03-03"]
	// The two items first waited on day 1, so day 2 does not count them again (F2).
	if d2.AgentWaitSeconds != 12 || d2.LampOnSeconds != 6 || len(d2.Items) != 0 {
		t.Fatalf("day 2: %+v", d2)
	}

	r := LoadLedger(path) // a restart: the totals come back, the gap is not counted
	if got := r.Days()["2026-03-03"]; got.AgentWaitSeconds != 12 {
		t.Fatalf("after restart: %+v", got)
	}
	t3 := t2.Add(time.Hour)
	r.Observe(t3, ledgerSnap(t3), set)
	r.Close(t3)
	if got := r.Days()["2026-03-03"]; got.AgentWaitSeconds != 12 || got.LampOnSeconds != 6 {
		t.Fatalf("the first read after a restart added time: %+v", got)
	}
}

// F2: items counts distinct items first seen that day. An item that waits over
// midnight counts on its first day only; a new item on day 2 counts on day 2.
func TestLedgerItemCountsOnItsFirstDayOnly(t *testing.T) {
	l := LoadLedger(ledgerPath(t))
	set := DefaultSettings()
	t0 := time.Date(2026, 3, 2, 23, 59, 58, 0, time.UTC)
	l.Observe(t0, ledgerSnap(t0, "session:fx:old"), set)
	t1 := t0.Add(4 * time.Second) // 00:00:02 on the next day
	l.Observe(t1, ledgerSnap(t1, "session:fx:old", "session:fx:new"), set)
	days := l.Days()
	if n := len(days["2026-03-02"].Items); n != 1 {
		t.Fatalf("day 1 holds %d items, want 1", n)
	}
	if n := len(days["2026-03-03"].Items); n != 1 {
		t.Fatalf("day 2 holds %d items, want only the new one", n)
	}
}

func TestLedgerFileHoldsNoMarkerString(t *testing.T) {
	path := ledgerPath(t)
	l := LoadLedger(path)
	now := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	l.Observe(now, ledgerSnap(now), DefaultSettings())
	l.Close(now)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{markID, markTitle, markProject, markPath, "MARK_"} {
		if strings.Contains(string(b), m) {
			t.Fatalf("the ledger holds %q: %s", m, b)
		}
	}
}

func TestLedgerKeepsAtMost500HashesAndOnlyCountedLiveItems(t *testing.T) {
	l := LoadLedger(ledgerPath(t))
	now := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	ids := make([]string, 600)
	for i := range ids {
		ids[i] = fmt.Sprintf("session:fx:%d", i)
	}
	s := ledgerSnap(now, ids...)
	s.WaitItems = append(s.WaitItems,
		waits.Item{Kind: waits.KindGate, ID: "gate:1", Group: waits.GroupBlocked, AgeSeconds: 9999},
		waits.Item{Kind: waits.KindSession, ID: "done:1", Group: waits.GroupDone, AgeSeconds: 9999})
	l.Observe(now, s, DefaultSettings())
	d := l.Days()["2026-03-02"]
	if len(d.Items) != 500 {
		t.Fatalf("%d hashes, want 500", len(d.Items))
	}
	if d.LongestWaitSeconds != 600 {
		t.Fatalf("longest %d: a gate or a done item counted", d.LongestWaitSeconds)
	}
}

func TestLedgerForemanSettingOffAndNoData(t *testing.T) {
	l := LoadLedger(ledgerPath(t))
	set := DefaultSettings()
	set.ForemanItems = false
	t0 := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	s := ledgerSnap(t0, "session:fx:1")
	s.WaitItems = append(s.WaitItems, waits.Item{Kind: waits.KindGate, ID: "gate:1", Group: waits.GroupBlocked, AgeSeconds: 5000})
	l.Observe(t0, s, set)
	t1 := t0.Add(2 * time.Second)
	s2 := ledgerSnap(t1, "session:fx:1")
	s2.WaitItems = append(s2.WaitItems, s.WaitItems[1])
	l.Observe(t1, s2, set)
	d := l.Days()["2026-03-02"]
	if d.AgentWaitSeconds != 2 || d.LongestWaitSeconds != 600 {
		t.Fatalf("a Foreman item counted with the setting off: %+v", d)
	}
	// no data and a stale snapshot add lamp time, never wait time
	t2 := t1.Add(2 * time.Second)
	l.Observe(t2, nil, set)
	t3 := t2.Add(2 * time.Second)
	l.Observe(t3, ledgerSnap(t0.Add(-time.Minute)), set)
	d = l.Days()["2026-03-02"]
	if d.AgentWaitSeconds != 2 || d.LampOnSeconds != 6 {
		t.Fatalf("no data or stale: %+v", d)
	}
}

func TestLedgerWritesAtMostOncePerMinuteAndAtExit(t *testing.T) {
	path := ledgerPath(t)
	l := LoadLedger(path)
	set := DefaultSettings()
	t0 := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	l.Observe(t0, ledgerSnap(t0), set)
	on := func() int64 {
		return LoadLedger(path).Days()["2026-03-02"].LampOnSeconds
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the first change was not written: %v", err)
	}
	for s := 2; s <= 58; s += 2 {
		at := t0.Add(time.Duration(s) * time.Second)
		l.Observe(at, ledgerSnap(at), set)
	}
	if got := on(); got != 0 {
		t.Fatalf("written inside the minute: lamp on %d", got)
	}
	at := t0.Add(60 * time.Second)
	l.Observe(at, ledgerSnap(at), set)
	if got := on(); got != 60 {
		t.Fatalf("not written after a minute: lamp on %d", got)
	}
	at = at.Add(2 * time.Second)
	l.Observe(at, ledgerSnap(at), set)
	l.Close(at)
	if got := on(); got != 62 {
		t.Fatalf("not written at exit: lamp on %d", got)
	}
}

func TestLedgerKeeps35Days(t *testing.T) {
	path := ledgerPath(t)
	old := `{"version":1,"days":{"2026-01-01":{"agent_wait_seconds":9},"2026-02-01":{"agent_wait_seconds":7}}}`
	if err := fsutil.WriteFileAtomic(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	l := LoadLedger(path)
	now := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC) // 2026-02-01 is 29 days back, 2026-01-01 is 60
	l.Observe(now, ledgerSnap(now), DefaultSettings())
	l.Close(now)
	days := LoadLedger(path).Days()
	if _, ok := days["2026-01-01"]; ok {
		t.Error("a 60 day old day was kept")
	}
	if days["2026-02-01"].AgentWaitSeconds != 7 {
		t.Error("a 29 day old day was dropped")
	}
}

func TestLedgerBadFileIsEmptyAndNilIsSafe(t *testing.T) {
	path := ledgerPath(t)
	_ = os.WriteFile(path, []byte("not json"), 0o644)
	if n := len(LoadLedger(path).Days()); n != 0 {
		t.Fatalf("%d days from a bad file", n)
	}
	var l *Ledger
	l.Observe(time.Now(), nil, DefaultSettings())
	l.Close(time.Now())
}

func TestLedgerFailedWriteIsNotRetriedInsideTheMinute(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "later")
	path := filepath.Join(dir, "lamp-waits.json")
	l := LoadLedger(path)
	t0 := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	l.Observe(t0, ledgerSnap(t0), DefaultSettings()) // the folder is missing: the write fails
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	at := t0.Add(30 * time.Second)
	l.Observe(at, ledgerSnap(at), DefaultSettings())
	if _, err := os.Stat(path); err == nil {
		t.Fatal("a failed write was retried inside the minute")
	}
	at = t0.Add(61 * time.Second)
	l.Observe(at, ledgerSnap(at), DefaultSettings())
	if _, err := os.Stat(path); err != nil {
		t.Fatal("not written a minute after the failed write")
	}
}

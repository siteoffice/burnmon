package lamp

import (
	"encoding/json"
	"hash/fnv"
	"sort"
	"sync"
	"time"

	"burnmon/internal/fsutil"
	"burnmon/internal/snapshot"
	"burnmon/internal/waits"
)

// The wait ledger (lane scope X2): the one file the lamp writes. Per local
// date it keeps how long agents waited on the person, the longest wait, how
// many different items waited, and how long the lamp was on. It holds no id,
// no title, no project and no path: an item is remembered only as a 32 bit
// hash, and the hashes are there to count each item once per day.
const (
	// LedgerFile is the file name inside the data folder.
	LedgerFile       = "lamp-waits.json"
	ledgerKeepDays   = 35
	ledgerMaxItems   = 500
	ledgerWriteEvery = time.Minute
	ledgerDayLayout  = "2006-01-02"
)

// DayRec is one local date of the ledger.
type DayRec struct {
	AgentWaitSeconds   int64    `json:"agent_wait_seconds"`
	LongestWaitSeconds int64    `json:"longest_wait_seconds"`
	Items              []uint32 `json:"items"`
	LampOnSeconds      int64    `json:"lamp_on_seconds"`
}

type ledgerFile struct {
	Version int                `json:"version"`
	Days    map[string]*DayRec `json:"days"`
}

// Ledger is the in-memory ledger of one lamp process. A nil Ledger accepts
// every call and does nothing, which is what the demo uses.
type Ledger struct {
	mu        sync.Mutex
	path      string
	days      map[string]*DayRec
	seen      map[string]map[uint32]bool // the hashes of each day, as a set
	lastTick  time.Time                  // the previous Observe, for lamp time
	lastFresh time.Time                  // the previous fresh snapshot, for wait time
	lastStamp string                     // written_at of that snapshot
	lastWrite time.Time
	dirty     bool
}

// LoadLedger reads the ledger file. A missing or unreadable file is an empty
// ledger.
func LoadLedger(path string) *Ledger {
	l := &Ledger{path: path, days: map[string]*DayRec{}, seen: map[string]map[uint32]bool{}}
	b, err := fsutil.ReadFileRetry(path)
	if err != nil {
		return l
	}
	var f ledgerFile
	if json.Unmarshal(b, &f) != nil {
		return l
	}
	for k, d := range f.Days {
		if _, err := time.Parse(ledgerDayLayout, k); err != nil || d == nil {
			continue
		}
		l.days[k] = d
		l.seen[k] = map[uint32]bool{}
		for _, h := range d.Items {
			l.seen[k][h] = true
		}
	}
	return l
}

// Observe adds what happened up to now. Lamp time grows on every call, by the
// time since the previous call, at most three ticks. Wait time grows once for
// every fresh snapshot, by the time since the previous fresh one (at most
// three ticks) times the counted live items. A restart does not count the
// time it was down.
func (l *Ledger) Observe(now time.Time, snap *snapshot.Snapshot, set Settings) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	day := l.day(now)
	tick := 2
	fresh := snap != nil && !snap.IsStale(now)
	if fresh && snap.TickSeconds > 0 {
		tick = snap.TickSeconds
	}
	limit := time.Duration(snapshot.StaleAfterTicks*tick) * time.Second
	if !l.lastTick.IsZero() && now.After(l.lastTick) {
		day.LampOnSeconds += int64(min(now.Sub(l.lastTick), limit) / time.Second)
		l.dirty = true
	}
	l.lastTick = now
	if fresh {
		l.observeWaits(now, snap, set, day, limit)
	}
	l.prune(now)
	l.write(now, false)
}

func (l *Ledger) observeWaits(now time.Time, snap *snapshot.Snapshot, set Settings, day *DayRec, limit time.Duration) {
	items := snap.WaitItems
	if !set.ForemanItems {
		items = withoutForeman(items)
	}
	counted := 0
	for _, it := range items {
		if (it.Group != waits.GroupBlocked && it.Group != waits.GroupYourTurn) || !liveKind(it.Kind) {
			continue
		}
		counted++
		if it.AgeSeconds > day.LongestWaitSeconds {
			day.LongestWaitSeconds = it.AgeSeconds
			l.dirty = true
		}
		h := fnv.New32a()
		_, _ = h.Write([]byte(it.ID))
		key := now.Format(ledgerDayLayout)
		if sum := h.Sum32(); !l.seen[key][sum] && len(day.Items) < ledgerMaxItems {
			l.seen[key][sum] = true
			day.Items = append(day.Items, sum)
			l.dirty = true
		}
	}
	if snap.WrittenAt != l.lastStamp {
		if !l.lastFresh.IsZero() && now.After(l.lastFresh) {
			day.AgentWaitSeconds += int64(min(now.Sub(l.lastFresh), limit)/time.Second) * int64(counted)
			l.dirty = true
		}
		l.lastFresh, l.lastStamp = now, snap.WrittenAt
	}
}

func (l *Ledger) day(now time.Time) *DayRec {
	key := now.Format(ledgerDayLayout)
	d := l.days[key]
	if d == nil {
		d = &DayRec{Items: []uint32{}}
		l.days[key] = d
		l.seen[key] = map[uint32]bool{}
		l.dirty = true
	}
	return d
}

// prune forgets the days older than 35 days before now's date.
func (l *Ledger) prune(now time.Time) {
	cut := now.AddDate(0, 0, -ledgerKeepDays).Format(ledgerDayLayout)
	for k := range l.days {
		if k < cut {
			delete(l.days, k)
			delete(l.seen, k)
			l.dirty = true
		}
	}
}

// Close writes the ledger if anything changed since the last write.
func (l *Ledger) Close(now time.Time) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.write(now, true)
}

// write puts the ledger on disk when it changed and a minute has passed since
// the last write, or when force is set.
func (l *Ledger) write(now time.Time, force bool) {
	if !l.dirty || l.path == "" || (!force && !l.lastWrite.IsZero() && now.Sub(l.lastWrite) < ledgerWriteEvery) {
		return
	}
	b, err := json.MarshalIndent(ledgerFile{Version: 1, Days: l.days}, "", " ")
	if err != nil {
		return
	}
	// A failed write waits the minute too, so a full disk is not hit every tick.
	l.lastWrite = now
	if fsutil.WriteFileAtomic(l.path, b, 0o644) == nil {
		l.dirty = false
	}
}

// Days is a copy of the days held, by local date.
func (l *Ledger) Days() map[string]DayRec {
	out := map[string]DayRec{}
	if l == nil {
		return out
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	keys := make([]string, 0, len(l.days))
	for k := range l.days {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		d := *l.days[k]
		d.Items = append([]uint32(nil), d.Items...)
		out[k] = d
	}
	return out
}

//go:build windows

// app.go holds the app struct and the small helpers shared by main.go: data
// directory, logging, single-instance mutex, and the sysmon sampling loop.
// Windows-only: BurnMon Dev is a WebView2 window for Wilco's own two
// screens (04_assets 2026-09-24_burnmon_dev_design.md), not a portable tool
// like burnmon/burnmon-cli, so unlike those there is no browser-mode
// fallback build for darwin/linux (see main_other.go).
package main

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"burnmon/internal/adapter/codex"
	"burnmon/internal/adapter/copilotcli"
	"burnmon/internal/adapter/copilotvsc"
	"burnmon/internal/adapter/hermes"
	"burnmon/internal/dataset"
	"burnmon/internal/history"
	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/snapshotwriter"
	"burnmon/internal/store"
	"burnmon/internal/sysmon"
	"burnmon/internal/todo"
	"burnmon/internal/vendorstrip"
	"burnmon/internal/watch"
)

const (
	version     = "0.4.0-beta.1"
	windowTitle = "BurnMon Dev"
	mutexName   = `Local\burnmon-dev-app`
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// appDataDir is %LOCALAPPDATA%\burnmon, the same folder burnmon.exe and
// burnmon-cli.exe already use: burnmon-dev.exe's own log and burnmon-dev.db
// land next to burnmon.db and burnmon.json, not in a folder of their own,
// since it reads and writes the same shared store (design doc section 1).
// It is a var so a test can point it at a temp dir and never touch the real
// folder.
var appDataDir = func() string {
	if lad := os.Getenv("LOCALAPPDATA"); lad != "" {
		return filepath.Join(lad, "burnmon")
	}
	return filepath.Join(os.TempDir(), "burnmon")
}

// settingsDirEnv names the folder for this app's own per-user settings. uicheck
// sets it to a fresh temp folder, so a test run never changes the real view.
const settingsDirEnv = "BURNMON_DEV_SETTINGS_DIR"

// settingsDir is BURNMON_DEV_SETTINGS_DIR when set (created if missing), else
// appDataDir(). The split:
//   - settingsDir: burnmon-dev-view.json, burnmon-dev-window.json, writes of
//     burnmon-dev.json (the station theme), the wv2-dev WebView2 profile and
//     the header EXPORT button's exports folder.
//   - appDataDir: burnmon.db, burnmon-dev.db, the snapshot, the msgraph token,
//     burnmon.json, burnmon-dev-app.log and the CLI export's redact_salt, all
//     shared data, so uicheck still runs against the real stores.
//
// burnmon-dev.json is read from the settings folder when a copy exists there,
// else from appDataDir (devConfigReadPath). A failed MkdirAll still returns the
// variable's folder: falling back to appDataDir would write the real files.
func settingsDir() string {
	if d := os.Getenv(settingsDirEnv); d != "" {
		if err := os.MkdirAll(d, 0o755); err != nil {
			log.Println("settings folder:", err)
		}
		return d
	}
	return appDataDir()
}

func setupLog(dataDir string) {
	path := filepath.Join(dataDir, "burnmon-dev-app.log")
	if fi, err := os.Stat(path); err == nil && fi.Size() > 512*1024 {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(f)
}

// app holds both stores: burnmon.db (shared with burnmon.exe, read by the
// burn zone) and burnmon-dev.db (this app's own, system samples only). Per
// the design doc's ingest decision (section 1), this app runs its own
// ingest into the shared store rather than only reading what burnmon.exe
// happens to have written, so it works whether or not burnmon.exe is open.
type app struct {
	st    *store.Store
	cache dataset.Cache

	sys         *sysmon.Store
	sampler     *sysmon.Sampler
	procSampler *sysmon.ProcessSampler

	liveWatcher *watch.Watcher

	// hidden is WS2 item 2's "pause when nobody looks": true while the
	// window is minimized, set by bdevSetHidden (main.go), which page.html
	// calls from its own visibilitychange listener. startSampling's cheap
	// system-sample ticker (below) reads this to slow from refresh_ms down
	// to a fixed 10s while true; the page itself stops calling
	// bdevSnapshotNow at all while hidden (paintTick never runs against a
	// window nobody can see), and fires one immediate tick the instant it
	// un-hides, rather than waiting for the next scheduled one.
	hidden atomic.Bool

	mu           sync.Mutex
	cfg          pricing.Config
	latest       sysmon.Sample
	latestGroups []sysmon.ProcessGroupSample

	// headlineDay/headlineFloor back headlineTodayMonotonic (headline.go):
	// a per-day floor on the headline total, guarded by mu like everything
	// else above. See headlineTodayMonotonic's own doc comment for why.
	headlineDay   time.Time
	headlineFloor int64

	// heatmapMu guards the activity heatmap's closed-day cache (Wilco's own
	// finding, 2026-09-24: bdevActivityHeatmap ran a fresh 182-day
	// EventsSince plus history.Build on every 1-minute poll, 3.34s/52,728
	// rows measured against the real store, about 5.5 percent of one core
	// on average, alone over this workstream's 2 percent CPU target). Every
	// day except today is closed (its events never change once the day is
	// over), so that 182-day pass only needs to run once per UTC day; each
	// minute now only recomputes today's own (much smaller) row and merges
	// it onto the cached closed days. See closedDayRows below.
	heatmapMu     sync.Mutex
	heatmapClosed []history.Row
	heatmapAsOf   string // "YYYY-MM-DD" UTC: the day heatmapClosed was computed for

	// initialCollectDone is closed by startInitialCollect once its one-time
	// backfill (dataset.Cache.Collect) returns. closedDayRows must not cache
	// its result before that: the backfill is still writing past-day events
	// into the store while the page's very first heatmap poll fires (fixed
	// after review, 2026-09-24 - the first version cached whatever
	// closedDayRows saw on that first call, potentially a half-backfilled
	// store, and then never rebuilt until the next UTC midnight).
	initialCollectDone chan struct{}

	// slowMu guards every "slow data" cache the shared render tick
	// (no-scroll patch section 5, bdevSnapshotNow in main.go) reads but
	// never itself recomputes: vendor strip and the activity heatmap
	// refresh every 60s, Microsoft To Do status every 30s and its task
	// list every 300s (startSlowRefreshers, this file) - the same cadences
	// this app used to poll each of these from the page directly, just
	// moved server-side so every panel still paints from one snapshot per
	// tick instead of resolving independently. Process-groups history moved
	// to sysHistMu/groupsHistBuf below, WS2 alpha.3 item 3's own fix.
	slowMu           sync.Mutex
	vendorStripCache vendorstrip.Payload
	heatmapRowsCache []history.Row
	todoStatusCache  todoStatusPayload
	todoTasksCache   todoTasksPayload

	// sysHistMu guards sysHistBuf/groupsHistBuf: WS2 alpha.3 item 3's own
	// fix for a gap a fresh review found (2026-09-27) - bdevSnapshotNow used
	// to serve SysmonHistory/ProcessGroupsHistory from burnmon-dev.db
	// (sys.RecentSamples/RecentProcessGroups), which only gains a new row
	// every persistInterval (10s), so the main chart and the process-groups
	// sparklines/harness heatmap still moved on a 10s beat even after the
	// live sample itself moved to 1s - the item 3 doc comment and README
	// both claimed otherwise. Two small in-memory ring buffers, appended
	// every systemSampleInterval tick alongside a.latest/a.latestGroups and
	// pruned to sysHistWindow, give both a true 1s resolution without
	// changing persistInterval: write volume to the store stays exactly as
	// before, only what bdevSnapshotNow reads for live rendering changed.
	// sys.RecentSamples/RecentProcessGroups (the DB path) are unaffected
	// and still used by `burnmon-dev.exe export` (export_run.go), which
	// needs history older than this buffer's own short window.
	sysHistMu     sync.Mutex
	sysHistBuf    []sysmon.Sample
	groupsHistBuf []sysmon.ProcessGroupSample
	// sysHistDropped/groupsHistDropped: pruneFront's own counters.
	sysHistDropped    int
	groupsHistDropped int
	// todoStatusGen guards a real (if narrow) race between the 30s status
	// loop below and bdevTodoLogin's own success path (main.go): both write
	// todoStatusCache from an independent read of the outside world (a disk
	// check here, "FinishLogin just returned nil" there), so a loop pass
	// that started its own disk read moments before a fresh sign-in could
	// otherwise still land after it and overwrite Signed:true with a stale
	// Signed:false. Bumped on every write; a writer that finds this has
	// moved since it started skips its own write rather than clobbering a
	// fresher one (found by review, 2026-09-25).
	todoStatusGen int

	// perf is WS2 alpha.7's measurement log (perf.go).
	perf snapshotPerf

	// snapStatus is the snapshot writer's state for the page line
	// (snapshot_line.go); set by startSnapshotWriter, nil before. started is
	// the process start, for the writer's first-write log line.
	snapStatus *snapshotwriter.Status
	started    time.Time
}

// closedDayRows returns the per-day token/cost rows for every day strictly
// before today (local, WS2 item 5 - this used to be a UTC boundary, the
// same class of bug WS3 fixed in vendorstrip/forecast/agg: a store event
// from the early local morning could still read as "yesterday" here for as
// long as the machine's own UTC offset, silently missing from the heatmap's
// "today" row and one day early in the closed-day cache), caching the
// whole 182-day pass and recomputing it only when the local date has
// rolled over since the last computation (once a day, not once a minute) -
// but only once the app's one-time startup backfill (startInitialCollect)
// has finished; before that, every call recomputes fresh rather than
// locking in a result built from a store that is still being backfilled.
// Returns today's own date string and local midnight alongside, so the
// caller derives "today"'s own window from the exact same now() this call
// used, rather than calling time.Now() a second time and risking a
// day-rollover mismatch between the two (also fixed after review).
func (a *app) closedDayRows(st *store.Store, cfg *pricing.Config) (closed []history.Row, today string, dayStart time.Time, cacheHit bool, err error) {
	now := time.Now().In(time.Local)
	dayStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	today = dayStart.Format("2006-01-02")

	a.heatmapMu.Lock()
	if a.heatmapAsOf == today {
		closed = a.heatmapClosed
		a.heatmapMu.Unlock()
		return closed, today, dayStart, true, nil
	}
	a.heatmapMu.Unlock()

	since := dayStart.AddDate(0, 0, -181)
	events, err := st.EventsSince(since)
	if err != nil {
		return nil, "", time.Time{}, false, err
	}
	closedEvents := make([]schema.Event, 0, len(events))
	for _, e := range events {
		if e.At.Before(dayStart) {
			closedEvents = append(closedEvents, e)
		}
	}
	payload := history.Build(closedEvents, cfg, history.Filter{Period: "day"})

	backfillDone := false
	select {
	case <-a.initialCollectDone:
		backfillDone = true
	default:
	}
	if backfillDone {
		a.heatmapMu.Lock()
		a.heatmapClosed = payload.Rows
		a.heatmapAsOf = today
		a.heatmapMu.Unlock()
	}
	return payload.Rows, today, dayStart, false, nil
}

// startLiveWatch starts an fsnotify watcher on the native Claude/Codex
// roots, same recipe as cmd\burnmon\app.go's own startLiveWatch: a live
// write calls IngestFile directly. Because dataset.Cache.IngestFile reads
// and advances its per-file cursor through a.st, which is the same
// burnmon.db file burnmon.exe writes to, a cursor burnmon.exe already
// advanced is resumed from there, not re-read from byte 0 (design doc
// section 1, point 2: idempotent, not a full re-parse).
func (a *app) startLiveWatch(nativeClaudeRoots, nativeCodexRoots []string) {
	if a.liveWatcher != nil {
		return
	}
	nativeRoots := append(append([]string{}, nativeClaudeRoots...), nativeCodexRoots...)
	wt, err := watch.New(nativeRoots, nil, func(path string) {
		a.mu.Lock()
		cfg := a.cfg
		a.mu.Unlock()
		if err := a.cache.IngestFile(&cfg, path); err != nil {
			log.Println("live watch: ingest", path, ":", err)
		}
	})
	if err != nil {
		log.Println("could not start the live watcher:", err)
		return
	}
	// WS2 follow-up (2026-09-29): a file whose cursor moves is tail-polled,
	// and today's and yesterday's Codex rollouts are seeded every minute,
	// since an open Codex chat raises no change notice of its own.
	a.cache.SetOnAdvance(wt.Track)
	wt.SetTailSeed(func() []string { return codex.RecentRollouts(nativeCodexRoots, time.Now()) })
	wt.Start()
	a.liveWatcher = wt
}

// startHermesPoll, startCopilotCLIPoll and startCopilotVSCPoll: the same
// 5-second poll cmd\burnmon\app.go runs, ported verbatim (a SQLite WAL file
// or an OTel file does not fit watch.Watcher's fsnotify-plus-offset design,
// see that file's own doc comments). Each is a no-op when the adapter has
// nothing to poll (no install found, or no copilot_vscode_otel_file
// configured).
func startHermesPoll(a *app, st *store.Store) {
	dbPath := hermes.DefaultDBPath()
	if dbPath == "" {
		return
	}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			events, err := hermes.PollOnce(dbPath)
			if err != nil {
				log.Println("hermes poll:", err)
				continue
			}
			if len(events) == 0 {
				continue
			}
			a.mu.Lock()
			cfg := a.cfg
			a.mu.Unlock()
			for i := range events {
				events[i].Owner = cfg.OwnerFor(events[i].Project)
			}
			if err := st.UpsertEvents(events); err != nil {
				log.Println("hermes poll: upsert:", err)
			}
		}
	}()
}

func startCopilotCLIPoll(a *app, st *store.Store) {
	dbPath := copilotcli.DefaultDBPath()
	if dbPath == "" {
		return
	}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			events, err := copilotcli.PollOnce(dbPath)
			if err != nil {
				log.Println("copilot cli poll:", err)
				continue
			}
			if len(events) == 0 {
				continue
			}
			a.mu.Lock()
			cfg := a.cfg
			a.mu.Unlock()
			for i := range events {
				events[i].Owner = cfg.OwnerFor(events[i].Project)
			}
			if err := st.UpsertEvents(events); err != nil {
				log.Println("copilot cli poll: upsert:", err)
			}
		}
	}()
}

func startCopilotVSCPoll(a *app, st *store.Store) {
	a.mu.Lock()
	path := a.cfg.CopilotVSCodeOtelFile
	a.mu.Unlock()
	if path == "" {
		return
	}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		var tail copilotvsc.Tail // WS2 follow-up item 5: new lines only, not the whole file
		for range ticker.C {
			a.mu.Lock()
			cfg := a.cfg
			a.mu.Unlock()
			events, err := tail.Poll(cfg.CopilotVSCodeOtelFile)
			if err != nil {
				log.Println("copilot vscode poll:", err)
				continue
			}
			if len(events) == 0 {
				continue
			}
			for i := range events {
				events[i].Owner = cfg.OwnerFor(events[i].Project)
				events[i].Client = cfg.ClientFor(events[i].Project)
			}
			if err := st.UpsertEvents(events); err != nil {
				log.Println("copilot vscode poll: upsert:", err)
			}
		}
	}()
}

// loadConfig mirrors cmd\burnmon's own two-tier lookup (exe-adjacent
// burnmon.json/claudecost.json first, then dataDir), so burnmon-dev.exe
// shares the same owner/client map and price overrides burnmon.exe already
// reads rather than a config of its own.
func loadConfig(dataDir string) (pricing.Config, string) {
	var path string
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		if cand := filepath.Join(exeDir, "burnmon.json"); fileExists(cand) {
			path = cand
		} else if cand := filepath.Join(exeDir, "claudecost.json"); fileExists(cand) {
			path = cand
		}
	}
	if path == "" {
		if cand := filepath.Join(dataDir, "burnmon.json"); fileExists(cand) {
			path = cand
		} else if cand := filepath.Join(dataDir, "claudecost.json"); fileExists(cand) {
			path = cand
		}
	}
	cfg, err := pricing.Load(path)
	if err != nil {
		log.Println("config error:", err)
	}
	return cfg, path
}

// devConfig is burnmon-dev.exe's own small config file, burnmon-dev.json,
// separate from the shared burnmon.json (design doc section 6): settings
// only this app cares about. Absent or unparsable is not an error, same
// convention as an absent burnmon.json: every field keeps its default.
type devConfig struct {
	RetentionDays int `json:"retention_days"`
	// MicrosoftTodoEnabled turns on section 9's Microsoft To Do panel.
	// Off by default (the zero value): the panel makes network calls to
	// Microsoft Graph on the user's own credentials, so it must be an
	// explicit opt-in, not something a fresh install starts doing.
	MicrosoftTodoEnabled bool `json:"microsoft_todo_enabled"`
	// RefreshMs (no-scroll patch section 5) is the page's one shared
	// render tick, default 1000, floored at 1000: every panel repaints
	// from one bdevSnapshotNow call on this cadence, no faster. Phase 5b
	// measured a 10-minute refresh_ms 1000 run at 1.89 percent average
	// whole-machine CPU against refresh_ms 2000's own 2.22 percent
	// (persistence runs on its own fixed cadence regardless, see
	// persistInterval below; the System zone's own sample cadence is now
	// also independent of RefreshMs, WS2 alpha.3 item 3's own
	// systemSampleInterval, fixed at 1s), so halving the paint cadence did
	// not double the cost of sampling; under the 2 percent bar and not
	// worse than the old default, so the design doc's own flip condition
	// made 1000 the new default.
	RefreshMs int `json:"refresh_ms"`
	// StationTheme is the startup theme of the Station: "space" (default)
	// or "site". Anything else falls back to "space". It reaches the page
	// as a spliced script string, so only these two values may pass.
	StationTheme string `json:"station_theme"`
}

// devConfigReadPath is burnmon-dev.json in dir, except when dir is the
// BURNMON_DEV_SETTINGS_DIR folder and has no copy yet: then the real one in
// appDataDir, so retention and To Do keep their real values under uicheck.
func devConfigReadPath(dir string) string {
	path := filepath.Join(dir, "burnmon-dev.json")
	if env := os.Getenv(settingsDirEnv); env != "" && filepath.Clean(env) == filepath.Clean(dir) && !fileExists(path) {
		return filepath.Join(appDataDir(), "burnmon-dev.json")
	}
	return path
}

func loadDevConfig(dataDir string) devConfig {
	cfg := devConfig{RetentionDays: 7, RefreshMs: 1000, StationTheme: "space"}
	b, err := os.ReadFile(devConfigReadPath(dataDir))
	if err != nil {
		return cfg
	}
	var loaded devConfig
	if err := json.Unmarshal(b, &loaded); err != nil {
		log.Println("burnmon-dev.json: parse error, using defaults:", err)
		return cfg
	}
	if loaded.RetentionDays > 0 {
		cfg.RetentionDays = loaded.RetentionDays
	}
	cfg.MicrosoftTodoEnabled = loaded.MicrosoftTodoEnabled
	if loaded.RefreshMs > 0 {
		cfg.RefreshMs = loaded.RefreshMs
	}
	if cfg.RefreshMs < 1000 {
		cfg.RefreshMs = 1000
	}
	switch loaded.StationTheme {
	case "site", "space":
		cfg.StationTheme = loaded.StationTheme
	case "":
	default:
		log.Printf("burnmon-dev.json: unknown station_theme %q, using space", loaded.StationTheme)
	}
	return cfg
}

// systemSampleInterval and persistInterval are WS2 alpha.3 item 3's own
// cadences (replacing phase 5b's split refreshInterval/processWalkInterval
// pair): one fixed 1s ticker now drives both the cheap whole-machine system
// sample (a.sampler.Tick) and the process-group walk (a.procSampler.Tick)
// together, so the CPU total bar, the 20 core bars, the main chart, the
// process-groups rows/sparklines and the Memory/Disks/Network boxes all move
// on the same beat instead of the process walk lagging on its own 3s clock -
// perfadvisor's own model (internal/tui/sample.go), one sample per second,
// one paint per sample, no panel on its own timer. Cheap enough to run at 1s:
// the process walk is one NtQuerySystemInformation snapshot (item 1, phase
// 5b), not a per-process gopsutil call each tick. persistInterval (writes to
// burnmon-dev.db) stays fixed and independent of the sample cadence, same as
// before, so a faster sample cadence never multiplies write volume to the
// store.
const (
	systemSampleInterval = 1 * time.Second
	persistInterval      = 10 * time.Second
	// hiddenSampleInterval is WS2 item 2's own number ("slow system
	// sampling to 10s") while the window is minimized, item 3's own "drops
	// everything, process walk included, to 10s when hidden": both
	// a.sampler.Tick and a.procSampler.Tick now share the one guard below,
	// so hiding the window slows the whole merged tick together, not just
	// the system sample half of it.
	hiddenSampleInterval = 10 * time.Second
	// histGapThreshold (v0.4.0-alpha.5, found by review 2026-09-28: the
	// System chart and the process-groups sparklines broke their line on
	// any gap over a page.html-side hard-coded 5s, so every minimized
	// stretch - hidden sampling runs at hiddenSampleInterval, 10s - drew as
	// no line at all, as if the data were simply missing. 2.5x
	// hiddenSampleInterval tolerates the ordinary hidden cadence while
	// still breaking on a real gap (sleep, closed app). Sent to page.html
	// as snapshotPayload.HistGapMs (main.go) instead of a second hard-coded
	// number there, so the two stay in sync by construction.
	histGapThreshold = hiddenSampleInterval * 5 / 2
	// sysHistWindow is how far back sysHistBuf/groupsHistBuf keep in
	// memory: the wider of the two live consumers' own windows -
	// live.ChartWindow (30 minutes, the main chart) and the process-groups
	// sparklines/harness heatmap's own 60-minute window (formerly the
	// DB-backed processGroupsHistCache's own since-window in
	// startSlowRefreshers, now this buffer's instead). main.go's
	// bdevSnapshotNow trims sysHistBuf down further to live.ChartWindow at
	// read time; groupsHistBuf is served as-is.
	sysHistWindow = 60 * time.Minute
)

// startSampling runs two independent tickers (WS2 alpha.3 item 3 merges the
// former three into two): systemSampleInterval samples both the
// whole-machine system reading and the process-group walk together, every
// 1s, so bdevSnapshotNow's readers (the system panel and the process-groups
// panel alike) always see numbers from the same instant; persistInterval
// writes whatever that most recently produced to burnmon-dev.db. The single
// sampling goroutine only ever touches its own two samplers in turn (both
// Sampler and ProcessSampler require single-goroutine use), and exchanges
// state with the persist goroutine through a.latest/a.latestGroups under
// a.mu.
func (a *app) startSampling() {
	firstSample := true
	go func() {
		ticker := time.NewTicker(systemSampleInterval)
		defer ticker.Stop()
		// lastSampleAt gates the hidden-window slowdown below: item 2/3
		// only want the actual sample itself to slow to 10s while nobody's
		// looking, not this ticker's own 1s tick rate (re-checking the
		// clock every tick while hidden is cheap - a lock and a time
		// comparison, not a real sample - so that part is not worth
		// avoiding on its own).
		var lastSampleAt time.Time
		// prevClocks/prevSampleTs back the alpha.7 gap log (perf.go).
		var prevClocks clockPair
		var prevSampleTs time.Time
		for range ticker.C {
			if a.hidden.Load() && time.Since(lastSampleAt) < hiddenSampleInterval {
				continue
			}
			passStart := time.Now()
			sm, err := a.sampler.Tick()
			sysDur := time.Since(passStart)
			if err != nil {
				log.Println("sysmon sample:", err)
				continue
			}
			lockStart := time.Now()
			a.mu.Lock()
			cfg := a.cfg
			a.mu.Unlock()
			lockWait := time.Since(lockStart)
			procStart := time.Now()
			groups, gerr := a.procSampler.Tick(cfg.CopilotVSCodeOtelFile != "")
			procDur := time.Since(procStart)
			if gerr != nil {
				log.Println("process sample:", gerr)
			}
			if pass := time.Since(passStart); pass > 2*time.Second {
				tt := a.sampler.LastTiming()
				log.Printf("sample loop slow: pass=%v sampler=%v (cpu_mem=%v disk_io=%v net=%v pdh=%v) proc=%v cfg_lock_wait=%v hidden=%v",
					pass.Round(time.Millisecond), sysDur.Round(time.Millisecond),
					tt.CPUMem.Round(time.Millisecond), tt.DiskIO.Round(time.Millisecond),
					tt.Net.Round(time.Millisecond), tt.PDH.Round(time.Millisecond),
					procDur.Round(time.Millisecond), lockWait.Round(time.Millisecond), a.hidden.Load())
			}
			clocks := readClocks()
			if !prevSampleTs.IsZero() {
				if gap := sm.Ts.Sub(prevSampleTs); gap > histGapThreshold {
					log.Printf("sample gap: %v from %s to %s slept=%v hidden=%v %s last_pass=%v",
						gap.Round(time.Millisecond), prevSampleTs.Format("15:04:05"), sm.Ts.Format("15:04:05"),
						sleptBetween(prevClocks, clocks).Round(time.Millisecond), a.hidden.Load(), powerState(),
						time.Since(passStart).Round(time.Millisecond))
				}
			}
			prevClocks, prevSampleTs = clocks, sm.Ts
			lastSampleAt = time.Now()
			if firstSample {
				firstSample = false
				startupMark("first system sample")
			}
			a.mu.Lock()
			a.latest = sm
			if gerr == nil {
				a.latestGroups = groups
			}
			a.mu.Unlock()

			// sysHistBuf/groupsHistBuf: WS2 alpha.3 item 3's own in-memory
			// history, a true 1s resolution for the main chart and the
			// process-groups sparklines/harness heatmap (see sysHistWindow's
			// own doc comment above). Pruned from the first entry at or after
			// the cutoff by pruneFront (history_delta.go, alpha.7 item 1.5):
			// a reslice, plus a copy to a fresh slice once the dropped prefix
			// would otherwise outgrow what is kept.
			histLockStart := time.Now()
			a.sysHistMu.Lock()
			if wait := time.Since(histLockStart); wait > 500*time.Millisecond {
				log.Printf("sample loop: sysHistMu wait %v", wait.Round(time.Millisecond))
			}
			a.sysHistBuf = append(a.sysHistBuf, sm)
			cutoff := sm.Ts.Add(-sysHistWindow)
			dropSys := 0
			for dropSys < len(a.sysHistBuf) && a.sysHistBuf[dropSys].Ts.Before(cutoff) {
				dropSys++
			}
			a.sysHistBuf = pruneFront(a.sysHistBuf, dropSys, &a.sysHistDropped)
			if gerr == nil {
				a.groupsHistBuf = append(a.groupsHistBuf, groups...)
				dropGroups := 0
				for dropGroups < len(a.groupsHistBuf) && a.groupsHistBuf[dropGroups].Ts.Before(cutoff) {
					dropGroups++
				}
				a.groupsHistBuf = pruneFront(a.groupsHistBuf, dropGroups, &a.groupsHistDropped)
			}
			a.sysHistMu.Unlock()
		}
	}()

	go func() {
		ticker := time.NewTicker(persistInterval)
		defer ticker.Stop()
		for range ticker.C {
			a.mu.Lock()
			sm := a.latest
			groups := a.latestGroups
			a.mu.Unlock()
			if err := a.sys.InsertSample(sm); err != nil {
				log.Println("sysmon insert:", err)
			}
			if len(groups) > 0 {
				if err := a.sys.InsertProcessGroupSamples(groups); err != nil {
					log.Println("process group insert:", err)
				}
			}
		}
	}()
}

// startInitialCollect runs one full backfill pass at startup: the same
// dataset.Cache.Collect call cmd\burnmon's own main() makes on its first
// run (main.go's startup goroutine, "the live watcher is registered before
// Collect is called"), so this app's vendor strip, heatmap and headline
// totals carry real history from the very first launch rather than only
// whatever the live watcher happens to see from here on. Without this,
// standalone mode (no burnmon.exe running) never backfills anything the
// watcher did not itself see fire, which contradicts the design doc's own
// ingest decision (section 1: "works whether or not burnmon.exe is open").
// Seat mirrors cmd\burnmon's own effectiveSeat precedence (an explicit
// config wins, else the "Standard" default); burnmon-dev has no -seat flag
// of its own to override it with. MonthsN matches burnmon.exe's own
// default (2); RefreshSlow true matches its first pass, which also
// resolves and walks WSL roots rather than waiting for a later refresh.
// progress, when non-nil, is section 11's own loading-screen callback
// ("Reading sessions: N of M files"): dataset.Cache.Collect already reports
// (done, total) across its own file scan, previously passed nil and never
// surfaced anywhere.
func (a *app) startInitialCollect(progress func(done, total int)) {
	go func() {
		defer close(a.initialCollectDone)
		a.mu.Lock()
		cfg := a.cfg
		a.mu.Unlock()
		seat := cfg.Subscription.YourSeat
		if seat != "Standard" && seat != "Premium" {
			seat = "Standard"
		}
		opts := dataset.CollectOpts{Seat: seat, MonthsN: 2, RefreshSlow: true}
		if _, err := a.cache.Collect(&cfg, opts, progress); err != nil {
			log.Println("initial collection failed:", err)
		}
	}()
}

// startRetentionPrune runs Prune once at startup and once a day after that,
// against burnmon-dev.json's retention_days (default 7; not read from
// config yet in phase 1, so the default is hard-coded here until phase 3
// wires burnmon-dev.json).
func (a *app) startRetentionPrune(retentionDays int) {
	prune := func() {
		cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
		if err := a.sys.Prune(cutoff); err != nil {
			log.Println("sysmon prune:", err)
		}
	}
	prune()
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			prune()
		}
	}()
}

// startSlowRefreshers runs the background loop behind every "slow data"
// cache bdevSnapshotNow reads (no-scroll patch section 5, "may be fetched
// less often in the background, but is only painted on the shared tick"):
// vendor strip and the activity heatmap refresh every 60s, and Microsoft
// To Do status every 30s / its task list every 300s - the same cadences
// this app used to poll each of these from the page directly, just moved
// server-side so every panel still paints from one snapshot per render
// tick instead of resolving independently. Process-groups history moved to
// startSampling's own sysHistBuf/groupsHistBuf (WS2 alpha.3 item 3), no
// longer one of this function's own loops. Each refresher runs once
// synchronously here before its own ticker starts, so the very first
// render tick already has real data rather than an empty cache for up to a
// whole cadence; the status loop is registered (and so runs its own first
// synchronous pass) before the task loop, so that first task refresh
// already sees a populated todoStatusCache rather than the zero value.
func (a *app) startSlowRefreshers(st *store.Store, devCfg devConfig) {
	loop := func(interval time.Duration, fn func()) {
		fn()
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for range ticker.C {
				fn()
			}
		}()
	}

	loop(60*time.Second, func() {
		a.mu.Lock()
		cfg := a.cfg
		a.mu.Unlock()
		payload, err := vendorstrip.Build(st, &cfg, time.Now())
		if err != nil {
			log.Println("slow refresh: vendor strip:", err)
			return
		}
		a.slowMu.Lock()
		a.vendorStripCache = payload
		a.slowMu.Unlock()
	})

	loop(60*time.Second, func() {
		a.mu.Lock()
		cfg := a.cfg
		a.mu.Unlock()
		rows, err := a.activityHeatmapRows(st, &cfg)
		if err != nil {
			log.Println("slow refresh: activity heatmap:", err)
			return
		}
		a.slowMu.Lock()
		a.heatmapRowsCache = rows
		a.slowMu.Unlock()
	})

	if !devCfg.MicrosoftTodoEnabled {
		return
	}
	loop(30*time.Second, func() {
		a.slowMu.Lock()
		genBefore := a.todoStatusGen
		wasSignedIn := a.todoStatusCache.SignedIn
		a.slowMu.Unlock()

		signedIn := todo.SignedIn() // a disk read, deliberately outside the lock

		a.slowMu.Lock()
		if a.todoStatusGen == genBefore {
			// Nothing else (bdevTodoLogin) wrote in the meantime; this
			// read is still the freshest information available.
			a.todoStatusCache = todoStatusPayload{Enabled: true, SignedIn: signedIn}
			a.todoStatusGen++
		} else {
			// A fresher write landed while signedIn was being read (most
			// likely bdevTodoLogin's own success path); trust that one
			// instead of overwriting it with this now-stale read.
			signedIn = a.todoStatusCache.SignedIn
		}
		a.slowMu.Unlock()
		if signedIn && !wasSignedIn {
			a.refreshTodoTasks() // a fresh sign-in shows tasks immediately, not up to 300s later
		}
	})
	loop(300*time.Second, func() {
		a.slowMu.Lock()
		signedIn := a.todoStatusCache.SignedIn
		a.slowMu.Unlock()
		if signedIn {
			a.refreshTodoTasks()
		}
	})
}

// activityHeatmapRows is bdevActivityHeatmap's own former body (UI review
// patch phase 2b): closedDayRows' own cached 182-day pass plus a fresh
// query for today alone, merged. Now run by startSlowRefreshers'
// background ticker instead of once per page poll.
func (a *app) activityHeatmapRows(st *store.Store, cfg *pricing.Config) ([]history.Row, error) {
	closed, today, dayStart, _, err := a.closedDayRows(st, cfg)
	if err != nil {
		return nil, err
	}
	todayEvents, err := st.EventsSince(dayStart)
	if err != nil {
		return nil, err
	}
	todayPayload := history.Build(todayEvents, cfg, history.Filter{Period: "day"})

	rows := make([]history.Row, 0, len(closed)+1)
	rows = append(rows, closed...)
	foundToday := false
	for _, r := range todayPayload.Rows {
		if r.Key == today {
			rows = append(rows, r)
			foundToday = true
		}
	}
	if !foundToday {
		rows = append(rows, history.Row{Key: today})
	}
	return rows, nil
}

// refreshTodoTasks fetches today's Microsoft To Do tasks and caches them
// for the shared render tick to paint. Unconditional (callers decide
// whether signed in first): called by startSlowRefreshers' own 300s loop
// and its post-sign-in transition above, and directly by bdevTodoLogin
// (main.go) right after a successful interactive sign-in, so the page
// shows real tasks on its very next tick instead of waiting up to 300s.
func (a *app) refreshTodoTasks() todoTasksPayload {
	items, err := todo.TodayTasks()
	payload := todoTasksPayload{Items: items}
	if err != nil {
		payload.Error = err.Error()
	}
	a.slowMu.Lock()
	a.todoTasksCache = payload
	a.slowMu.Unlock()
	return payload
}

// ---------------------------------------------------------------------------
// Single instance, same recipe as cmd\burnmon\main.go's own alreadyRunning/
// bringExistingToFront, a distinct mutex and window title so the two apps
// never mistake each other for a second instance of themselves.
// ---------------------------------------------------------------------------

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW         = user32.NewProc("FindWindowW")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

func alreadyRunning() bool {
	name, err := windows.UTF16PtrFromString(mutexName)
	if err != nil {
		return false
	}
	_, err = windows.CreateMutex(nil, false, name)
	return errors.Is(err, windows.ERROR_ALREADY_EXISTS)
}

func bringExistingToFront() {
	title, err := windows.UTF16PtrFromString(windowTitle)
	if err != nil {
		return
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		return
	}
	procSetForegroundWindow.Call(hwnd)
}

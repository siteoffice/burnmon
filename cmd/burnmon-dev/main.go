//go:build windows

// Command burnmon-dev is BurnMon Dev: a second WebView2 window, alongside
// burnmon.exe, that shows what running AI agents does to the token/cost
// side (burn) and the machine (system) on one screen, one time axis.
// 04_assets\2026-09-24_burnmon_dev_design.md is the design this follows.
// Phase 1 wired the system-side sampler and both viewports' empty panels;
// phase 2 starts the same live watch and adapter polls cmd\burnmon\app.go
// runs, into the same shared burnmon.db, and wires the burn zone (chart,
// session cards, vendor strip, turn ticker) against it. Phase 2b (this
// pass) adds the token-monitor items: headline running total, tok/min,
// the activity heatmap and cache-hit breakdown, design doc section 9.
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	webview2 "github.com/jchv/go-webview2"

	"burnmon/internal/adapter/codex"
	"burnmon/internal/controls"
	"burnmon/internal/devexport"
	"burnmon/internal/history"
	"burnmon/internal/historyadmin"
	"burnmon/internal/live"
	"burnmon/internal/scan"
	"burnmon/internal/snapshot"
	"burnmon/internal/store"
	"burnmon/internal/sysmon"
	"burnmon/internal/todo"
	"burnmon/internal/vendorstrip"
)

//go:embed page.html
var pageHTML string

// The Station (02_roadmap\2026-10-01_station_secret_screen.md): the two
// atlases (space and construction site), the startup theme and the screen,
// spliced into page.html at stationMarker by assemblePage before SetHtml.
//
//go:embed station/station_atlas.js
var stationAtlasJS string

//go:embed station/station_atlas_site.js
var stationAtlasSiteJS string

//go:embed station/station.js
var stationJS string

const stationMarker = "<!--BM_STATION-->"

// assemblePage returns page.html with the Station scripts at its marker:
// both atlases, window.BM_STATION_THEME, then station.js. The theme lands
// in a script string, so only the two known names pass and anything else
// becomes "space". The page keeps working without the scripts
// (window.BMStation is checked before use), so a missing marker only costs
// the secret screen.
func assemblePage(theme, uiTheme string) string {
	if theme != "site" {
		theme = "space"
	}
	page := pageHTML
	if uiTheme == "light" {
		// Light is set in the markup, so it is there before the first paint.
		page = strings.Replace(page, `<html lang="en">`, `<html lang="en" data-theme="light">`, 1)
	}
	return strings.Replace(page, stationMarker,
		"<script>\n"+stationAtlasJS+"\n</script>\n<script>\n"+stationAtlasSiteJS+"\n</script>\n"+
			`<script>window.BM_STATION_THEME = "`+theme+`";</script>`+"\n"+
			"<script>\n"+stationJS+"\n</script>", 1)
}

// Microsoft To Do panel payloads (section 9). Kept small and purpose-built
// per call rather than reusing internal/todo's own structs directly: the
// unexported deviceCode/interval/expiresIn fields on todo.DeviceCodeInfo
// never need to reach the page at all (main.go keeps the whole struct
// server-side to hand to FinishLogin).
type todoStatusPayload struct {
	Enabled  bool `json:"enabled"`
	SignedIn bool `json:"signed_in"`
}

type todoLoginStartPayload struct {
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	Message         string `json:"message"`
}

type todoLoginDonePayload struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type todoTasksPayload struct {
	Items []todo.Item `json:"items"`
	Error string      `json:"error,omitempty"`
}

// progStart marks process entry, so every startupMark call below logs an
// elapsed time from the same zero point (section 11 step 1: "measure
// first" before changing any startup ordering). Read only from main's own
// goroutine before the window is up; the sampling goroutine's one
// first-sample mark (startSampling) reads it too, but only ever after
// main() has already set it, so no lock is needed.
var progStart = time.Now()

func startupMark(step string) {
	log.Printf("startup: %s at %v", step, time.Since(progStart).Round(time.Millisecond))
}

// heatmapPayload is bdevActivityHeatmap's return shape: renderHeatmap
// (page.html) only ever reads payload.rows, so this carries just that
// field rather than the rest of history.Payload (Totals, Vendors, Owners),
// which the merged closed+today rows below don't populate meaningfully.
type heatmapPayload struct {
	Rows []history.Row `json:"rows"`
}

func init() {
	// go-webview2 pumps a Win32 message loop tied to the thread that
	// created the window; keep that on one OS thread for the life of the
	// process, same as cmd\burnmon\main.go.
	runtime.LockOSThread()

	// Same fix as cmd\burnmon\app.go's own init() (v0.2.1 hang patch,
	// SESSION_LOG.md): measured live against this build, WorkingSet passed
	// 1 GB within a couple of minutes of 2s polling (bdevBurnNow's
	// live.BuildSnapshot/ApplySessionTotals, bdevSysmonHistory) before the
	// default GC pacer (GOGC 100, no soft limit) ever collected. A 400 MB
	// soft memory limit makes the runtime collect proactively as usage
	// approaches it, matching this workstream's own under-250-MB target,
	// rather than waiting for the heap to double from whatever the last
	// collection's live size happened to be.
	debug.SetMemoryLimit(400 << 20)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "export" {
		os.Exit(runExport(os.Args[2:]))
		return
	}

	dataDir := appDataDir()
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return
	}
	setupLog(dataDir)
	log.Printf("burnmon-dev %s starting", version)

	if alreadyRunning() {
		log.Println("another instance is already running; bringing it to the front")
		bringExistingToFront()
		return
	}

	sysPath, err := sysmon.DefaultPath()
	if err != nil {
		log.Println("could not resolve the sysmon store path:", err)
		return
	}
	sys, err := sysmon.Open(sysPath)
	if err != nil {
		log.Println("could not open the sysmon store at", sysPath, ":", err)
		return
	}
	defer sys.Close()

	// The shared store: same file burnmon.exe reads and writes (design doc
	// section 1). Opening it here, with its own writer connection, is safe
	// whether or not burnmon.exe is running at the same time (WAL, 5s busy
	// timeout, idempotent upsert; see store.Open's own doc comment and the
	// design doc's ingest decision).
	storePath, err := store.DefaultPath()
	if err != nil {
		log.Println("could not resolve the burnmon store path:", err)
		return
	}
	st, err := store.Open(storePath)
	if err != nil {
		log.Println("could not open the burnmon store at", storePath, ":", err)
		return
	}
	defer st.Close()
	startupMark("store open and migrations done")

	cfg, cfgFile := loadConfig(dataDir)
	if cfgFile != "" {
		log.Println("using config overrides from", cfgFile)
	}

	devCfg := loadDevConfig(dataDir)
	devCfg.MicrosoftTodoEnabled = todoEnabledForRun(devCfg.MicrosoftTodoEnabled, uicheckActive())

	a := &app{
		sys: sys, sampler: sysmon.NewSampler(), procSampler: sysmon.NewProcessSampler(),
		st: st, cfg: cfg, initialCollectDone: make(chan struct{}), started: progStart,
	}
	a.cache.Store = st
	a.perf.startPerfLog()
	a.startSampling()
	a.startSnapshotWriter(st)
	a.startRetentionPrune(devCfg.RetentionDays)
	// Backgrounded (found by review, 2026-09-25): startSlowRefreshers' own
	// first pass per cache runs synchronously before that cache's own
	// ticker starts (app.go's own doc comment), which is exactly right for
	// ordering between the To Do status and task loops, but wrong called
	// straight from main() - it ran a full 182-day activityHeatmapRows scan
	// (3.34s against a cold cache, closedDayRows' own comment) plus a
	// vendor-strip build, a process-groups query and up to two Microsoft
	// Graph calls before the window even existed, silently reintroducing
	// the same "blocks window creation" problem section 11 moved the
	// native-root scan and startup backfill off of. bdevSnapshotNow below
	// tolerates empty caches fine (the loading screen covers the gap, the
	// same as any other startBackgroundIngest step), so there is nothing
	// here the window needs to wait for.
	go a.startSlowRefreshers(st, devCfg)

	// Section 11 steps 2-3: the native-root scan (measured this session at
	// ~13.7s of this app's own ~17.4s "watcher and pollers started" mark,
	// almost all of the gap before the pre-patch window even appeared) and
	// the startup backfill both move into startBackgroundIngest below,
	// launched only once the window and its loading screen already exist,
	// instead of blocking window creation itself. bdevSnapshotNow below only
	// ever needs a.st/a.cache/a.sys/a.slowMu's own caches, all already set
	// up by the time it is bound, so none of the bindings below need to
	// wait for this (startSlowRefreshers above is likewise backgrounded, for
	// the same reason).

	wv2Dir := filepath.Join(dataDir, "wv2-dev")
	_ = os.MkdirAll(wv2Dir, 0o755)

	// Window, section 10: first launch is a normal 1280x860 window,
	// centered; every launch after that remembers size, position, maximized
	// state and monitor, restoring them only if that monitor still exists
	// (windowstate.go). WindowOptions has no X/Y field, so a restored
	// window is created at this same default size/center and then
	// immediately repositioned via applyWindowState below; a very brief
	// visible jump to the default rect is an accepted tradeoff of
	// go-webview2's WebView interface having no "create hidden, then show"
	// hook to avoid it.
	savedState, hasSavedState := loadWindowState(dataDir)
	initialWidth, initialHeight := uint(1280), uint(860)
	if hasSavedState {
		initialWidth, initialHeight = uint(savedState.Width), uint(savedState.Height)
	}
	// WEBVIEW2_USER_DATA_FOLDER (alpha.7, found 2026-09-29): go-webview2
	// (pkg/edge/chromium.go) passes DataPath to WebView2Loader as an inline
	// windows.StringToUTF16Ptr that nothing keeps alive, and the loader reads
	// it after the Go buffer can already be freed and reused, so some launches
	// got a user-data folder named after whatever heap text was there (prompt
	// and session-path fragments), created relative to the working directory.
	// The loader also reads this environment variable, which lives in the
	// process environment, not in Go memory, so the folder is always wv2Dir.
	_ = os.Setenv("WEBVIEW2_USER_DATA_FOLDER", wv2Dir)
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath: wv2Dir,
		WindowOptions: webview2.WindowOptions{
			Title:  windowTitle,
			Width:  initialWidth,
			Height: initialHeight,
			Center: true,
			IconId: 1, // matches the "#1" icon group winres/burnmon-dev.json embeds
		},
	})
	if w == nil {
		log.Println("could not create the WebView2 window; BurnMon Dev needs the WebView2 runtime")
		return
	}
	hwnd := uintptr(w.Window())
	if hasSavedState {
		applyWindowState(hwnd, savedState)
	}

	// F11 toggles fullscreen, Esc also leaves it (section 10). fs is this
	// window's own fullscreen state (windowstate.go), declared before
	// startWindowStateSaver so that goroutine can skip saving while
	// fullscreen (its own doc comment explains why). Esc reaches
	// bdevExitFullscreen through the page's own keydown handler, same as any
	// other binding; F11 does not, since WebView2/Chromium reserves it as a
	// default browser accelerator key and never lets the DOM see it at all
	// (found empirically this session) - installF11Hotkey works around that
	// with a global hotkey plus a thread-local message hook instead (see its
	// own doc comment, windowstate.go), so bdevToggleFullscreen below is
	// only reachable from the uicheck dev eval channel, not from a real F11
	// keypress.
	fs := &fullscreenState{}
	startWindowStateSaver(hwnd, dataDir, fs)
	startupMark("window created")

	installF11Hotkey(hwnd, fs)
	if err := w.Bind("bdevToggleFullscreen", func() {
		toggleFullscreen(hwnd, fs)
	}); err != nil {
		log.Println("could not bind bdevToggleFullscreen:", err)
	}
	if err := w.Bind("bdevExitFullscreen", func() {
		exitFullscreen(hwnd, fs)
	}); err != nil {
		log.Println("could not bind bdevExitFullscreen:", err)
	}

	// bdevMarkFirstRender: page.html calls this once, as soon as its very
	// first shared render tick's bdevSnapshotNow call resolves (section 5;
	// the actual paint follows immediately after, in the same
	// requestAnimationFrame), so "first full render" is measured from the
	// page's own perspective rather than guessed at from the Go side.
	if err := w.Bind("bdevMarkFirstRender", func() {
		startupMark("first full render")
	}); err != nil {
		log.Println("could not bind bdevMarkFirstRender:", err)
	}

	// bdevSetHidden (WS2 item 2, "pause when nobody looks"): page.html
	// calls this from its own document.visibilitychange listener, which
	// WebView2/Chromium already fires when this app's own window is
	// minimized (the same page-visibility signal a real browser tab gets,
	// tied here to the host window rather than a background tab). a.hidden
	// is read by startSampling's merged system/process-walk ticker (app.go,
	// WS2 alpha.3 item 3) to slow it to a fixed 10s while true; the page
	// itself stops calling
	// bdevSnapshotNow at all while hidden, so there is nothing here to do
	// on the "becoming visible again" edge - the page's own listener fires
	// one immediate tick then resumes its normal cadence, both client-side.
	if err := w.Bind("bdevSetHidden", func(hidden bool) {
		a.hidden.Store(hidden)
	}); err != nil {
		log.Println("could not bind bdevSetHidden:", err)
	}

	// bdevSetUITheme: the L key's choice (light or dark), remembered in
	// burnmon-dev-view.json (viewstate.go) and applied at the next start by
	// assemblePage. Anything but the two names is ignored.
	if err := w.Bind("bdevSetUITheme", func(theme string) {
		if err := saveUITheme(dataDir, theme); err != nil {
			log.Println("bdevSetUITheme:", err)
		}
	}); err != nil {
		log.Println("could not bind bdevSetUITheme:", err)
	}

	// bdevSetStationTheme: the "Station starts as" setting, saved to
	// burnmon-dev.json (station_theme.go) and used at the next start. It does
	// not touch devCfg or the Station that is open now.
	if err := w.Bind("bdevSetStationTheme", func(theme string) error {
		err := saveStationTheme(dataDir, theme)
		if err != nil {
			log.Println("bdevSetStationTheme:", err)
		}
		return err
	}); err != nil {
		log.Println("could not bind bdevSetStationTheme:", err)
	}

	// bdevSections (B2): the part switches per mode, the overall scrollbar flag, and the
	// newest snapshot reading. bdevSetSections saves one change from the page
	// (Plain Mode, the scrollbar flag, or one part switch). Both read and write files only.
	snapPath, snapPathErr := snapshot.DefaultPath()
	if snapPathErr != nil {
		log.Println("snapshot path:", snapPathErr)
	}
	ctlSvc := newControlService(cfg, dataDir, snapPath, storePath)
	if err := w.Bind("bdevSections", func() sectionsPayload {
		return sectionsPayloadWith(dataDir, snapPath, time.Now(), ctlSvc.Availability, a.snapStatus.View)
	}); err != nil {
		log.Println("could not bind bdevSections:", err)
	}
	if err := w.Bind("bdevSetSections", func(c sectionsChange) error {
		err := applySectionsChange(dataDir, c)
		if err != nil {
			log.Println("bdevSetSections:", err)
		}
		return err
	}); err != nil {
		log.Println("could not bind bdevSetSections:", err)
	}
	// bdevControlPreview and bdevControlRun (R10): the page sends a control name
	// plus a target id, then only the preview token. Everything else (argv,
	// checks, audit) stays in internal/controls. The page input is untrusted.
	if err := w.Bind("bdevControlPreview", func(req controls.PreviewRequest) (controls.PreviewResult, error) {
		return bdevControlPreview(ctlSvc, req)
	}); err != nil {
		log.Println("could not bind bdevControlPreview:", err)
	}
	if err := w.Bind("bdevControlRun", func(token string) (controls.RunResult, error) {
		return bdevControlRun(ctlSvc, token)
	}); err != nil {
		log.Println("could not bind bdevControlRun:", err)
	}
	// bdevOpenItemFile (R9): opens a spec or result file only when the current
	// snapshot lists that exact path (item_files.go). The page input is untrusted.
	if err := w.Bind("bdevOpenItemFile", func(path string) error {
		a.mu.Lock()
		root := a.cfg.ForemanRoot
		a.mu.Unlock()
		return bdevOpenItemFile(snapPath, path, root)
	}); err != nil {
		log.Println("could not bind bdevOpenItemFile:", err)
	}
	// bdevAgentMap (R7): the full agent map as boxes and lines, live from the
	// snapshot file or for the last N hours from the store. The small overview
	// rides in bdevSections. Read-only, no port.
	if err := w.Bind("bdevAgentMap", func(req agentMapRequest) (agentMapPayload, error) {
		a.mu.Lock()
		root := a.cfg.ForemanRoot
		a.mu.Unlock()
		p, err := agentMapFor(agentMapEnv{DataDir: dataDir, SnapPath: snapPath, St: st, ForemanRoot: root, Writer: a.snapStatus.View}, req, time.Now())
		if err != nil {
			log.Println("bdevAgentMap:", err)
		}
		return p, err
	}); err != nil {
		log.Println("could not bind bdevAgentMap:", err)
	}

	// sysmonNowPayload adds the header pressure chip's score (internal/sysmon
	// .PressureScore, phase 3) and the CPU box's static base clock (UI review
	// patch section 6) alongside the raw sample: Sample is embedded
	// anonymously so every one of its fields still flattens into the same
	// JSON object the page already reads, plus these two extra keys.
	type sysmonNowPayload struct {
		sysmon.Sample
		PressureScore int     `json:"pressure_score"`
		BaseClockGHz  float64 `json:"base_clock_ghz"`
	}

	// snapshotPayload is bdevSnapshotNow's own return shape (no-scroll patch
	// section 5): one call, once per render tick, carries everything every
	// panel needs, so the page renders all of them inside a single
	// requestAnimationFrame instead of each panel resolving its own
	// independent poll. Now is the tick's own shared instant, computed once
	// here and reused for every time-axis field below (Burn, SysmonHistory),
	// so the burn chart and system history both shift left together rather
	// than each reading a slightly different time.Now(). SysmonHistory and
	// ProcessGroupsHistory (the harness heatmap's own source too) are read
	// from app.go's own in-memory sysHistBuf/groupsHistBuf, a true 1s
	// resolution (WS2 alpha.3 item 3); VendorStrip/ActivityHeatmap/Todo/
	// TodoTasks are still read from app's own slower background-refresh
	// caches (startSlowRefreshers, app.go), so those can lag by up to their
	// own refresh cadence - page.html still anchors their right edge to
	// this same Now (passed through the snapshot) so they visibly shift on
	// the same beat even between refreshes, rather than each reading a
	// slightly different time.Now().
	type snapshotPayload struct {
		Now                  int64                       `json:"now"`
		RefreshMs            int                         `json:"refresh_ms"`
		// HistGapMs (v0.4.0-alpha.5) is app.go's own histGapThreshold in
		// milliseconds - the System chart/sparkline gap-break threshold,
		// derived from hiddenSampleInterval so page.html never hard-codes a
		// second number that has to be kept in sync by hand.
		HistGapMs int64 `json:"hist_gap_ms"`
		Sysmon               sysmonNowPayload            `json:"sysmon"`
		SysmonHistory        []sysmon.Sample             `json:"sysmon_history"`
		ProcessGroupsNow     []sysmon.ProcessGroupSample `json:"process_groups_now"`
		ProcessGroupsHistory []sysmon.ProcessGroupSample `json:"process_groups_history"`

		// SysmonHistoryDelta/ProcessGroupsDelta (alpha.7 item 1): true when
		// the matching history field holds only samples after the page's own
		// cursor, to append to its ring; false means replace the ring.
		// uicheck's fake snapshots omit both, so they stay full replaces.
		SysmonHistoryDelta bool `json:"sysmon_history_delta"`
		ProcessGroupsDelta bool `json:"process_groups_history_delta"`

		Burn                 live.Snapshot               `json:"burn"`
		VendorStrip          vendorstrip.Payload         `json:"vendor_strip"`
		ActivityHeatmap      heatmapPayload              `json:"activity_heatmap"`
		Todo                 todoStatusPayload           `json:"todo"`
		TodoTasks            todoTasksPayload            `json:"todo_tasks"`
		// HeadlineToday (2026-09-26 phase 5 fix) is vendorStrip.Total.Today
		// plus tokens ingested since that cache's own GeneratedAt, from the
		// events this same tick already fetched below for Burn - see
		// headline.go's headlineTodayTokens, wrapped by
		// a.headlineTodayMonotonic so a stalled vendor_strip refresh can
		// never make this dip. The header no longer waits on vendorStrip's
		// own 60s refresh to move.
		HeadlineToday int64 `json:"headline_today"`
	}
	if err := w.Bind("bdevSnapshotNow", func(cursor snapshotCursor) (snapshotPayload, error) {
		now := time.Now()

		a.mu.Lock()
		cfg := a.cfg
		latest := a.latest
		latestGroups := a.latestGroups
		a.mu.Unlock()

		a.slowMu.Lock()
		vendorStrip := a.vendorStripCache
		heatmapRows := a.heatmapRowsCache
		todoStatus := a.todoStatusCache
		todoTasks := a.todoTasksCache
		a.slowMu.Unlock()

		// sysHistory/groupsHistory: WS2 alpha.3 item 3's own fix (found by
		// review, 2026-09-27) - these used to come from burnmon-dev.db
		// (sys.RecentSamples/RecentProcessGroups), which only gains a new
		// row every persistInterval (10s), so the main chart and the
		// process-groups sparklines/harness heatmap still moved on a 10s
		// beat even after the live sample itself moved to 1s. Now read
		// straight from app.go's own in-memory sysHistBuf/groupsHistBuf
		// (appended every systemSampleInterval tick, alongside a.latest/
		// a.latestGroups), a true 1s resolution; sysHistBuf is trimmed
		// further down to live.ChartWindow here (groupsHistBuf's own
		// window already matches what the sparklines/heatmap want, see
		// sysHistWindow's doc comment).
		// alpha.7 item 1: only what the page does not hold yet
		// (history_delta.go), so the payload stays constant instead of
		// growing with the buffers.
		a.sysHistMu.Lock()
		sysHistory, sysFull := sysHistSince(a.sysHistBuf, cursor.SysSince, now.Add(-live.ChartWindow))
		groupsHistory, groupsFull := groupsHistSince(a.groupsHistBuf, cursor.GroupsSince)
		a.sysHistMu.Unlock()

		tEvents := time.Now()
		events, err := st.EventsSince(now.Add(-live.ChartWindow))
		if err != nil {
			return snapshotPayload{}, err
		}
		tBurn := time.Now()
		burn := live.BuildSnapshot(events, &cfg, now)
		tTotals := time.Now()
		if err := live.ApplySessionTotals(burn.Sessions, st, &cfg); err != nil {
			log.Println("bdevSnapshotNow: session totals:", err)
		}
		// Only while the Station is open (the page sets cursor.Stages), so a
		// closed Station adds no store read to the tick.
		if cursor.Stages {
			if err := live.ApplyStages(burn.Sessions, st, now); err != nil {
				log.Println("bdevSnapshotNow: stages:", err)
			}
		}
		tDone := time.Now()

		payload := snapshotPayload{
			Now:       now.UnixMilli(),
			RefreshMs: devCfg.RefreshMs,
			HistGapMs: histGapThreshold.Milliseconds(),
			Sysmon: sysmonNowPayload{
				Sample: latest, PressureScore: sysmon.PressureScore(latest),
				BaseClockGHz: sysmon.BaseClockGHz(),
			},
			SysmonHistory:        sysHistory,
			SysmonHistoryDelta:   !sysFull,
			ProcessGroupsNow:     latestGroups,
			ProcessGroupsHistory: groupsHistory,
			ProcessGroupsDelta:   !groupsFull,
			Burn:                 burn,
			VendorStrip:          vendorStrip,
			ActivityHeatmap:      heatmapPayload{Rows: heatmapRows},
			Todo:                 todoStatus,
			TodoTasks:            todoTasks,
			HeadlineToday:        a.headlineTodayMonotonic(vendorStrip.Total, vendorStrip.GeneratedAt, events, now),
		}
		if a.perf.record(time.Since(now), tBurn.Sub(tEvents), tTotals.Sub(tBurn), tDone.Sub(tTotals), len(sysHistory), len(groupsHistory)) {
			a.perf.setSize(payload)
		}
		return payload, nil
	}); err != nil {
		log.Println("could not bind bdevSnapshotNow:", err)
	}

	// bdevPerfReport (WS2 alpha.7 measurement): page.html's own once-a-minute
	// round-trip and dropped-paint summary, written to the app log.
	if err := w.Bind("bdevPerfReport", func(r pagePerfReport) {
		logPagePerf(r)
	}); err != nil {
		log.Println("could not bind bdevPerfReport:", err)
	}

	// bdevTurnDetail: the turn ticker's click-through popup (UI review patch
	// section 5, "the equivalent of burnmon.exe's turn drawer"), same
	// live.BuildTurnDetail cmd\burnmon\main.go's own bmTurn binding calls,
	// off the UI thread for the same reason bmTurn's own N1 fix gives (a
	// full-session EventsForSession re-read must not block the window from
	// responding to the drawer's own Close/Esc while it runs). turnDetailPayload
	// wraps the result with an Error string (found by review, 2026-09-25: the
	// original version silently sent back a zero-value TurnDetail on error,
	// which the popup then rendered as "Turn 0" / an invalid date instead of
	// surfacing the failure) rather than TurnDetail bare.
	type turnDetailPayload struct {
		live.TurnDetail
		Error string `json:"error,omitempty"`
	}
	if err := w.Bind("bdevTurnDetail", func(sessionID string, turn int) {
		go func() {
			a.mu.Lock()
			cfg := a.cfg
			a.mu.Unlock()
			detail, err := live.BuildTurnDetail(st, &cfg, sessionID, turn)
			payload := turnDetailPayload{TurnDetail: detail}
			if err != nil {
				log.Println("bdevTurnDetail:", sessionID, turn, ":", err)
				payload.Error = err.Error()
			}
			key := fmt.Sprintf("%s:%d", sessionID, turn)
			js, err := bdevAsyncResolveJSWithKey("__bdevTurnDetailResolve", key, payload)
			if err != nil {
				log.Println("bdevTurnDetail: encode result:", err)
				return
			}
			w.Dispatch(func() { w.Eval(js) })
		}()
	}); err != nil {
		log.Println("could not bind bdevTurnDetail:", err)
	}

	// bdevVendorStrip/bdevActivityHeatmap are gone: no-scroll patch section 5
	// moves both onto startSlowRefreshers' own background cadence (app.go),
	// read from a.vendorStripCache/a.heatmapRowsCache by bdevSnapshotNow
	// above instead of recomputed per call. activityHeatmapRows (app.go) is
	// this binding's own former body, moved verbatim.

	// bdevCacheBreakdown: phase 2b's click-to-expand cache-hit breakdown,
	// scoped to vendor-strip (harness) rows, design doc section 9. Today's
	// events for that vendor through the same history.Build, returning its
	// Totals (Fresh/CacheW/CacheR/Out) directly rather than a new struct.
	// "Today" is local (WS2 item 5), matching vendorstrip.Total.Today's own
	// boundary: this is that same row's click-through detail, so it must
	// agree with it on which events count as "today", not disagree for
	// part of every day the way a UTC boundary here would have.
	if err := w.Bind("bdevCacheBreakdown", func(vendor string) (history.Totals, error) {
		a.mu.Lock()
		cfg := a.cfg
		a.mu.Unlock()
		now := time.Now().In(time.Local)
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		events, err := st.EventsSince(dayStart)
		if err != nil {
			return history.Totals{}, err
		}
		today := now.Format("2006-01-02")
		payload := history.Build(events, &cfg, history.Filter{Period: "day", From: today, To: today, Vendor: vendor})
		return payload.Totals, nil
	}); err != nil {
		log.Println("could not bind bdevCacheBreakdown:", err)
	}

	// bdevAdvisorNow (phase 4's "TODAY'S READ" panel) is gone: UI review
	// patch section 8 deletes the panel outright. The advisor rules
	// themselves (internal/advisor) stay, since buildExportBundle
	// (export_run.go) calls advisor.Analyze directly for summary.md,
	// independent of this binding.

	// bdevExport is the header EXPORT button (design doc "Export (AI-ready
	// bundle)"): the last 24 hours, not redacted (the CLI's --redact flag
	// covers the redacted case), written to dataDir\exports\. Shares
	// buildExportBundle (export_run.go) with the `export` CLI subcommand
	// so the two paths can never disagree on what an export contains.
	type exportResultPayload struct {
		OK            bool   `json:"ok"`
		Error         string `json:"error,omitempty"`
		Dir           string `json:"dir,omitempty"`
		SummaryBytes  int    `json:"summary_bytes,omitempty"`
		DataJSONBytes int    `json:"data_json_bytes,omitempty"`
		DailyCSVBytes int    `json:"daily_csv_bytes,omitempty"`
	}
	if err := w.Bind("bdevExport", func() {
		go func() {
			a.mu.Lock()
			cfg := a.cfg
			a.mu.Unlock()
			until := time.Now()
			since := until.Add(-24 * time.Hour)

			var payload exportResultPayload
			b, err := buildExportBundle(st, sys, &cfg, since, until)
			if err != nil {
				payload = exportResultPayload{OK: false, Error: err.Error()}
			} else {
				outDir := filepath.Join(dataDir, "exports")
				res, werr := devexport.Write(outDir, b)
				if werr != nil {
					payload = exportResultPayload{OK: false, Error: werr.Error()}
				} else {
					payload = exportResultPayload{
						OK: true, Dir: res.Dir,
						SummaryBytes: res.SummaryBytes, DataJSONBytes: res.DataJSONBytes, DailyCSVBytes: res.DailyCSVBytes,
					}
				}
			}
			js, jerr := bdevAsyncResolveJS("__bdevExportResolve", payload)
			if jerr != nil {
				log.Println("bdevExport: encode result:", jerr)
				return
			}
			w.Dispatch(func() { w.Eval(js) })
		}()
	}); err != nil {
		log.Println("could not bind bdevExport:", err)
	}

	// bdevDeleteHistory, bdevExportHistory and bdevHistoryInfo (B1): the gear
	// dialog's History block. Delete is quick and returns its counts (a
	// Promise); export reads the whole store, so it returns at once and
	// resolves through bdevAsyncResolveJS. The rules (confirm, absolute folder)
	// live in a.deleteHistory and a.exportHistory, not in the page.
	if err := w.Bind("bdevDeleteHistory", func(req historyadmin.DeleteRequest) (historyadmin.DeleteResult, error) {
		return a.deleteHistory(st, req)
	}); err != nil {
		log.Println("could not bind bdevDeleteHistory:", err)
	}
	if err := w.Bind("bdevExportHistory", func(req historyadmin.ExportRequest) {
		go func() {
			js, err := bdevAsyncResolveJS("__bdevExportHistoryResolve", a.historyExportOutcome(st, req))
			if err != nil {
				log.Println("bdevExportHistory: encode result:", err)
				return
			}
			w.Dispatch(func() { w.Eval(js) })
		}()
	}); err != nil {
		log.Println("could not bind bdevExportHistory:", err)
	}
	if err := w.Bind("bdevHistoryInfo", func() historyInfoPayload {
		return a.historyInfo()
	}); err != nil {
		log.Println("could not bind bdevHistoryInfo:", err)
	}

	// Microsoft To Do panel (section 9), off unless burnmon-dev.json sets
	// microsoft_todo_enabled. Read-only, own token cache under
	// %LOCALAPPDATA%\burnmon\ (internal/todo's own doc comment), never
	// touched by the export bundle or any log line - task titles are
	// personal data the design doc's "never in exports, logs or
	// screenshots" line covers by this package simply never being imported
	// from that path, not by a redaction rule. bdevTodoStatus is gone: its
	// periodic poll moves onto startSlowRefreshers' own 30s cadence
	// (app.go), read from a.todoStatusCache by bdevSnapshotNow above.

	// bdevTodoLogin: StartLogin is one fast HTTP call, so this returns
	// synchronously with the code/URL to show; the browser opens
	// immediately, and FinishLogin's own slow polling loop (the
	// interactive "type this code in your browser" wait) runs in a
	// goroutine, resolving through bdevAsyncResolveJS once the user
	// actually approves it (or it times out), the same pattern every other
	// slow bdev* binding in this file already uses. On success this also
	// updates a.todoStatusCache and fetches tasks immediately, so the very
	// next render tick already shows signed-in with real tasks rather than
	// waiting on startSlowRefreshers' own 30s status loop to notice.
	if err := w.Bind("bdevTodoLogin", func() (todoLoginStartPayload, error) {
		if !devCfg.MicrosoftTodoEnabled {
			return todoLoginStartPayload{}, fmt.Errorf("Microsoft To Do is not enabled")
		}
		info, err := todo.StartLogin()
		if err != nil {
			return todoLoginStartPayload{}, err
		}
		openBrowser(info.VerificationURI)
		go func() {
			loginErr := todo.FinishLogin(info)
			payload := todoLoginDonePayload{OK: loginErr == nil}
			if loginErr != nil {
				payload.Error = loginErr.Error()
			} else {
				a.slowMu.Lock()
				a.todoStatusCache = todoStatusPayload{Enabled: true, SignedIn: true}
				a.todoStatusGen++
				a.slowMu.Unlock()
				a.refreshTodoTasks()
			}
			js, jerr := bdevAsyncResolveJS("__bdevTodoLoginResolve", payload)
			if jerr != nil {
				log.Println("bdevTodoLogin: encode result:", jerr)
				return
			}
			w.Dispatch(func() { w.Eval(js) })
		}()
		return todoLoginStartPayload{UserCode: info.UserCode, VerificationURI: info.VerificationURI, Message: info.Message}, nil
	}); err != nil {
		log.Println("could not bind bdevTodoLogin:", err)
	}

	// bdevTodoTasks is gone: the page no longer calls it directly. On a
	// successful sign-in, bdevTodoLogin above already calls
	// a.refreshTodoTasks() itself, and every other refresh comes from
	// startSlowRefreshers' own 300s background loop (app.go); either way,
	// the page just paints whatever bdevSnapshotNow's own todo_tasks field
	// carries on its next tick.

	// Dispatch heartbeat (alpha.7, found in the 90-minute after run on
	// 2026-09-29): go-webview2's Dispatch queues its func and wakes the UI
	// thread with one PostThreadMessage(WM_APP), which only w.Run's own
	// GetMessage loop handles. A Win32 modal loop (a window drag or resize,
	// a menu) pulls that thread message off the queue and drops it, so the
	// queued func, here a bdevSnapshotNow resolve, waits until some later
	// Dispatch gets through: 67s on 2026-09-29, and forever before alpha.7,
	// when nothing else dispatched while the page waited on that one call.
	// A no-op Dispatch every second means a lost wakeup costs at most a
	// second once the modal loop has ended (inside it, the heartbeat's own
	// WM_APP is dropped too), for every binding, not just the render tick.
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			w.Dispatch(func() {})
		}
	}()

	startUICheckServer(w, func() string { return assemblePage(devCfg.StationTheme, loadUITheme(dataDir)) })

	w.SetHtml(assemblePage(devCfg.StationTheme, loadUITheme(dataDir)))
	go startBackgroundIngest(a, st, w)
	w.Run()
}

// startBackgroundIngest is section 11 steps 2-3: everything that used to run
// synchronously before the window was created (the native-root scan, the
// live watcher, the adapter pollers, the startup backfill) now runs here
// instead, after the window and its own loading screen already exist, so
// "time to window" no longer includes any of it. Progress reaches the page
// through the same w.Eval mechanism bdevAsyncResolveJS already uses
// elsewhere in this file, not a bound function: the loading screen has
// nothing to call, only something to listen for.
func startBackgroundIngest(a *app, st *store.Store, w webview2.WebView) {
	// "Opening store" already finished, synchronously, before the window
	// was created (it is fast: 24ms measured this session); shown here
	// anyway so the loading screen's own step list reads as one coherent
	// narrative from 0%, not starting partway through.
	pushLoadingStep(w, "Opening store", 5)
	pushLoadingStep(w, "Sampling system", 15)

	// The Hermes/Copilot pollers (app.go) never touch the native Claude/
	// Codex roots below; starting them first means those vendors show up
	// as soon as this goroutine runs, rather than being delayed behind the
	// ~13s native-root scan for no reason (found by review, 2026-09-25).
	startHermesPoll(a, st)
	startCopilotCLIPoll(a, st)
	startCopilotVSCPoll(a, st)

	pushLoadingStep(w, "Starting watchers", 30)
	// Cowork (Cloud): make sure the exe-adjacent .burnmon\cloud drop folder
	// exists before the roots are resolved, so the live watcher watches it
	// from the start (02_roadmap\2026-10-12_cowork_cloud_bucket_spec.md).
	if err := scan.EnsureCloudDropDir(); err != nil {
		log.Println("cloud drop folder:", err)
	}
	nativeClaudeRoots := scan.DefaultSourcesWithOptions(false)
	nativeCodexRoots := codex.NativeSources()
	a.cache.SeedNativeRoots(nativeClaudeRoots, nativeCodexRoots)
	a.startLiveWatch(nativeClaudeRoots, nativeCodexRoots)
	startupMark("watcher and pollers started")

	pushLoadingStep(w, "Reading sessions", 45)
	// Throttled to at most 4 pushes/second: dataset.Cache.Collect calls
	// this once per file, including skipped ones, so an unthrottled push
	// (two w.Dispatch/w.Eval round trips each) could reach thousands of
	// calls against a large trail, competing with the UI thread's own
	// message queue for no visible benefit once the loading screen only
	// updates a few times a second anyway (found by review, 2026-09-25).
	var lastProgressPush time.Time
	a.startInitialCollect(func(done, total int) {
		now := time.Now()
		last := done == total
		if !last && now.Sub(lastProgressPush) < 250*time.Millisecond {
			return
		}
		lastProgressPush = now
		pct := 45
		if total > 0 {
			pct = 45 + int(50*float64(done)/float64(total))
		}
		pushHeaderStatus(w, fmt.Sprintf("backfilling: %d of %d files", done, total))
		pushLoadingStep(w, fmt.Sprintf("Reading sessions: %d of %d files", done, total), pct)
	})
	<-a.initialCollectDone
	startupMark("initial collect and backfill done")
	pushLoadingStep(w, "Ready", 100)
	pushHeaderStatus(w, "")
}

// pushLoadingStep and pushHeaderStatus both just run JS in the page, the
// same w.Dispatch/w.Eval pattern every async bdev* binding in this file
// already uses to deliver a result computed off the UI thread; neither is a
// bound function since the page never calls either of these itself.
func pushLoadingStep(w webview2.WebView, text string, pct int) {
	textJSON, err := json.Marshal(text)
	if err != nil {
		return
	}
	js := fmt.Sprintf("window.__bdevLoadingStep && window.__bdevLoadingStep(%s, %d)", textJSON, pct)
	w.Dispatch(func() { w.Eval(js) })
}

func pushHeaderStatus(w webview2.WebView, text string) {
	textJSON, err := json.Marshal(text)
	if err != nil {
		return
	}
	js := fmt.Sprintf("window.__bdevHeaderStatus && window.__bdevHeaderStatus(%s)", textJSON)
	w.Dispatch(func() { w.Eval(js) })
}

// openBrowser opens url in the default browser (cmd\burnmon-cli\main.go's
// own Windows branch, ported here since burnmon-dev is Windows-only and has
// no such helper of its own yet). Only ever called with Microsoft's own
// device-code verification_uri (internal/todo.StartLogin), never anything
// page- or user-supplied, but still checked for an https scheme before
// shelling out (found by review, 2026-09-25: cheap, and rules out this
// call site ever being repurposed later with a less trusted URL).
// HideWindow: burnmon-dev.exe is built -H windowsgui, so without it `cmd`
// can still flash a console window.
func openBrowser(rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		log.Println("openBrowser: refusing a non-https URL:", rawURL)
		return
	}
	cmd := exec.Command("cmd", "/c", "start", "", rawURL)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		log.Println("openBrowser: could not start:", err)
	}
}

// bdevAsyncResolveJS mirrors cmd\burnmon\main.go's own asyncResolveJS: a
// binding that returns immediately and finishes its real work in a
// goroutine hands the result to a page-side resolver this way instead of
// through go-webview2's own per-call Promise, which only resolves with
// whatever the bound function returns synchronously. encoding/json's
// default HTML escaping rules out a literal "</script>" in the marshalled
// JSON. No reqID here (unlike bmHistory): bdevActivityHeatmap is a fixed
// periodic poll with no filter to correlate against, so the latest
// resolved call simply overwrites whatever the page is showing.
func bdevAsyncResolveJS(resolverName string, data any) (string, error) {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("window.%s && window.%s(%s)", resolverName, resolverName, dataJSON), nil
}

// bdevAsyncResolveJSWithKey mirrors cmd\burnmon\main.go's own asyncResolveJS:
// a keyed variant of bdevAsyncResolveJS for a binding more than one call to
// which can be in flight at once (bdevTurnDetail: a fast second ticker
// click before the first resolves), so the resolver can tell which call a
// result belongs to.
func bdevAsyncResolveJSWithKey(resolverName, key string, data any) (string, error) {
	keyJSON, err := json.Marshal(key)
	if err != nil {
		return "", err
	}
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("window.%s && window.%s(%s, %s)", resolverName, resolverName, keyJSON, dataJSON), nil
}

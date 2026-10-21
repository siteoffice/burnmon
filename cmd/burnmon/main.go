//go:build windows

// Command burnmon is the single-window desktop face of burnmon: a
// WebView2 window that collects your Claude usage on startup, on an
// interval, and on demand, and shows the same dashboard template as the CLI,
// live, in place. No server, no open port, no tray, no browser tab.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"burnmon/internal/adapter/codex"
	"burnmon/internal/forecast"
	"burnmon/internal/history"
	"burnmon/internal/live"
	"burnmon/internal/pricing"
	"burnmon/internal/scan"
	"burnmon/internal/store"
	"burnmon/internal/vendorstrip"
)

const (
	windowTitle = "BurnMon"
	mutexName   = `Local\burnmon-app`
)

func init() {
	// go-webview2 pumps a Win32 message loop tied to the thread that created
	// the window; keep that on one OS thread for the life of the process.
	runtime.LockOSThread()
}

func main() {
	interval := flag.Duration("interval", 15*time.Minute, "how often to re-read transcripts while the window is open")
	wslInterval := flag.Duration("wsl-interval", 4*time.Hour, "how often to re-read WSL transcripts while the window is open (native transcripts stay on -interval)")
	monthsN := flag.Int("months", 2, "how many months to include, counting the current one")
	seat := flag.String("seat", "Standard", "your own seat tier (Standard or Premium)")
	cfgPath := flag.String("config", "", "config file overriding the compiled-in prices and subscription (default: burnmon.json in the app's data folder, if present)")
	var sources multiFlag
	flag.Var(&sources, "source", "extra folder to scan; repeatable, overrides auto-detect")
	flag.Parse()

	if *interval < minInterval {
		*interval = minInterval
	}

	// -wsl-interval wins over wsl_interval_hours in config; whether it was
	// actually passed (as opposed to sitting at its flag default) is only
	// knowable via flag.Visit, since flag.Duration itself cannot tell "not
	// set" from "set to the default". Same trick for -seat against
	// your_seat: an explicit -seat always wins, otherwise a your_seat in
	// burnmon.json beats the flag's own "Standard" default, so a shared
	// config for a team on one tier (see the Groundwork Kit) never needs
	// anyone to open Settings on first run.
	wslIntervalFlagSet := false
	seatFlagSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "wsl-interval" {
			wslIntervalFlagSet = true
		}
		if f.Name == "seat" {
			seatFlagSet = true
		}
	})

	dataDir := appDataDir()
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		// No window, no console, and now nowhere to log either: nothing
		// sensible left to do.
		return
	}
	setupLog(dataDir)
	log.Printf("burnmon %s starting", version)

	// Two-tier lookup, exe-adjacent first: this makes burnmon.exe plus
	// burnmon.json droppable together into one portable folder (the
	// ZeroNonsense.dev Groundwork Kit ships exactly this pair), the same
	// "drop one file next to the exe" pattern the CLI already used and that
	// burnmon.json's own bundled _comment describes. Falls back to
	// %LOCALAPPDATA%\burnmon\burnmon.json, so a fixed install that
	// must run from a read-only share (nothing written or read next to the
	// exe in that case) keeps working exactly as before: place
	// burnmon.json in %LOCALAPPDATA%\burnmon\ and restart the app.
	// Whichever file loads is also where Settings saves land afterward
	// (cfgSavePath below), so a Groundwork Kit user editing Settings keeps
	// writing to their own portable folder, not %LOCALAPPDATA%.
	// claudecost.json is the pre-rename config name (v0.1.2 and earlier);
	// still read if present so an existing portable folder or fixed install
	// keeps working after the burnmon rename, but burnmon.json always wins
	// when both exist.
	cfgFile := *cfgPath
	if cfgFile == "" {
		if exe, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exe)
			if cand := filepath.Join(exeDir, "burnmon.json"); fileExists(cand) {
				cfgFile = cand
			} else if cand := filepath.Join(exeDir, "claudecost.json"); fileExists(cand) {
				cfgFile = cand
			}
		}
	}
	if cfgFile == "" {
		if cand := filepath.Join(dataDir, "burnmon.json"); fileExists(cand) {
			cfgFile = cand
		} else if cand := filepath.Join(dataDir, "claudecost.json"); fileExists(cand) {
			cfgFile = cand
		}
	}
	cfg, err := pricing.Load(cfgFile)
	if err != nil {
		log.Println("config error:", err)
		return
	}
	if cfgFile != "" {
		log.Println("using config overrides from", cfgFile)
	}

	// Zero or absent config value means the flag's own 4h default, whether
	// or not the flag was explicitly passed.
	effectiveWSLInterval := *wslInterval
	if !wslIntervalFlagSet && cfg.WSLIntervalHours > 0 {
		effectiveWSLInterval = time.Duration(cfg.WSLIntervalHours * float64(time.Hour))
	}
	effectiveSeat := *seat
	if !seatFlagSet && cfg.Subscription.YourSeat != "" {
		effectiveSeat = cfg.Subscription.YourSeat
	}
	if effectiveWSLInterval < minWSLInterval {
		effectiveWSLInterval = minWSLInterval
	}
	wslEnabled := cfg.WSLScan != "off"

	// Where Settings saves land: the file that was actually loaded, or the
	// default location next to the store if none existed yet, so the very
	// first save from the window creates it rather than erroring.
	cfgSavePath := cfgFile
	if cfgSavePath == "" {
		cfgSavePath = filepath.Join(dataDir, "burnmon.json")
	}

	if alreadyRunning() {
		log.Println("another instance is already running; bringing it to the front")
		bringExistingToFront()
		return
	}

	wv2Dir := filepath.Join(dataDir, "wv2")
	_ = os.MkdirAll(wv2Dir, 0o755)

	storePath, err := store.DefaultPath()
	if err != nil {
		log.Println("could not resolve the store path:", err)
		return
	}
	st, err := store.Open(storePath)
	if err != nil {
		log.Println("could not open the local store at", storePath, ":", err)
		return
	}
	defer st.Close()

	// Best-effort cleanup of the old gob-based parse cache, replaced by the
	// SQLite store above; ignore any error, missing is the expected case on
	// every run after the first.
	_ = os.Remove(filepath.Join(dataDir, "parsecache.gob"))

	a := &app{
		cfg:         cfg,
		seat:        effectiveSeat,
		monthsN:     *monthsN,
		sources:     []string(sources),
		htmlPath:    filepath.Join(dataDir, "dashboard.html"),
		interval:    *interval,
		wslInterval: effectiveWSLInterval,
		cfgPath:     cfgSavePath,
	}
	a.cache.Store = st

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
			Width:  1280,
			Height: 860,
			Center: true,
			IconId: 1, // matches the "#1" icon group winres/app.json embeds
		},
	})
	if w == nil {
		log.Println("could not create the WebView2 window; falling back to the default browser")
		runWithoutWindow(a)
		return
	}

	// A dashboard.html from a previous run is shown immediately, stale
	// "Snapshot taken" stamp and all: that stamp is already honest about
	// its own age, so no extra "refreshing" banner is needed on top of it.
	// A true first run has nothing to show yet, so it keeps the warming
	// page with the progress bar instead.
	hadDashboard := fileExists(a.htmlPath)
	if hadDashboard {
		w.Navigate(toFileURL(a.htmlPath))
	} else {
		w.SetHtml(warmingPageHTML())
	}

	// W1 (02_roadmap\2026-09-23_v0.2.3_window_check_patch.md): everything
	// from here down used to run synchronously on this goroutine, which is
	// the UI thread w.Run() below pumps messages on (runtime.LockOSThread
	// in this file's init). w.SetHtml/w.Navigate above only queue a
	// navigation; nothing paints until the message loop is actually
	// running, so scan.DefaultSourcesWithOptions and codex.NativeSources
	// (both walk the filesystem) blocked the very first paint, reading as a
	// blank, "Not Responding" window until they and the first collection
	// below finished. Moving it all into the same startup goroutine that
	// already did the first collection fixes this: w.Run() starts pumping
	// messages immediately after this call, so the warming/stale page paints
	// first and everything else happens behind it. F2 (SESSION_LOG.md,
	// v0.1.1) still holds: the live watcher is registered before Collect is
	// called below, in the same order as before, just one goroutine later.
	go func() {
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
		startProfiling(dataDir, func() int {
			if a.liveWatcher == nil {
				return -1
			}
			return a.liveWatcher.WatchCount()
		})
		startHermesPoll(a, st)
		startCopilotCLIPoll(a, st)
		startCopilotVSCPoll(a, st)

		if _, err := a.rebuild(true, progressReporter(w)); err != nil {
			log.Println("initial collection failed:", err)
			if !hadDashboard {
				msg := startupFailureMessage(err)
				blob, _ := json.Marshal(msg)
				w.Dispatch(func() { w.Eval("window.ccWarmFail && ccWarmFail(" + string(blob) + ")") })
			}
			return
		}
		// The backfill above has now resolved the WSL roots (if any); add
		// them to the watcher already running on native roots.
		a.extendLiveWatchWSL(nativeClaudeRoots, nativeCodexRoots)
		if hadDashboard {
			w.Dispatch(func() { w.Eval("window.ccReload ? ccReload() : location.reload()") })
			return
		}
		fileURL := toFileURL(a.htmlPath)
		w.Dispatch(func() { w.Navigate(fileURL) })
	}()

	// wslTicker drives the slow (WSL) cadence; nil when wsl_scan is off, so
	// that case never even starts a background timer for it. resetWSLTicker
	// is what "Refresh now and Settings-save reset it" (Two refresh
	// cadences) means in practice: without this, pressing Refresh right
	// before the four-hour mark would still trigger a second full WSL walk
	// moments later.
	var wslTicker *time.Ticker
	var tickerMu sync.Mutex
	if wslEnabled {
		wslTicker = time.NewTicker(a.wslInterval)
	}
	resetWSLTicker := func() {
		if wslTicker == nil {
			return
		}
		tickerMu.Lock()
		wslTicker.Reset(a.wslInterval)
		tickerMu.Unlock()
	}

	if err := w.Bind("ccRefresh", func() {
		go func() {
			if _, err := a.rebuild(true, progressReporter(w)); err != nil {
				log.Println("refresh failed:", err)
				w.Dispatch(func() { w.Eval("window.ccRefreshFailed && window.ccRefreshFailed()") })
				return
			}
			resetWSLTicker()
			w.Dispatch(func() { w.Eval("window.ccReload ? ccReload() : location.reload()") })
		}()
	}); err != nil {
		log.Println("could not bind ccRefresh:", err)
	}

	// The bound functions' bodies live in bindings.go, shared with the
	// browser bridge (browser.go) used on macOS and Linux.
	// bucketSeconds (U5, v0.3): monitor view's braille chart calls
	// bmLive(10) for finer columns than the Now page's own bmLive(), no
	// second binding; live.BuildSnapshot defaults to 60 when omitted.
	if err := w.Bind("bmLive", func(bucketSeconds ...int) (live.Snapshot, error) {
		return a.liveSnapshot(st, bucketSeconds...)
	}); err != nil {
		log.Println("could not bind bmLive:", err)
	}

	// bmTurn (I3): the Now page's "explain this spike" drawer, called on
	// demand when a marker or ticker line is clicked, not polled. Re-reads
	// sessionID's whole event history (EventsForSession, any vendor) rather
	// than bmLive's windowed one, so a drawer opened on a turn that has
	// since scrolled out of the 30-minute chart still resolves.
	//
	// N1 (v0.2.2, SESSION_LOG.md): this used to return live.BuildTurnDetail's
	// result directly, synchronously, on go-webview2's Bind callback, which
	// v0.2.1's own hang patch documented as running on the same UI thread
	// the window paints and processes input on. On the real store a turn
	// drawer open re-reads a session's whole event history, so while that
	// call is in flight the window (including the drawer's own Close button
	// and Esc) cannot respond, which reads as "the drawer cannot be closed"
	// rather than "the window is briefly frozen". Same fix as
	// bmSessionInsight/bmHistory below: return immediately, do the real work
	// in a goroutine, resolve through asyncResolveJS.
	if err := w.Bind("bmTurn", func(sessionID string, turn int) {
		go func() {
			detail := a.turnDetail(st, sessionID, turn)
			key := fmt.Sprintf("%s:%d", sessionID, turn)
			js, err := asyncResolveJS("__bmTurnResolve", key, detail)
			if err != nil {
				log.Println("bmTurn: encode result:", err)
				return
			}
			w.Dispatch(func() { w.Eval(js) })
		}()
	}); err != nil {
		log.Println("could not bind bmTurn:", err)
	}

	// bmSessionInsight (I3, Sessions tab): the findings column and its
	// expandable row call this on demand, one sessionID at a time, never
	// polled; the frontend caches the result in page memory so re-expanding
	// a row costs nothing further. renderSessions() (template.html) fires
	// one of these per kept session on every page load, unconditionally
	// (hundreds at once against a real history, see SESSION_LOG.md v0.2.1
	// hang patch): go-webview2's Bind runs the bound Go function
	// synchronously on the same Win32 message-pump thread the window
	// paints and processes input on, so hundreds of them in a row, even at
	// a few tens of milliseconds each, add up to the UI thread being fully
	// consumed for the whole burst. This binding therefore returns
	// immediately (case (b): "or move it to a goroutine that resolves
	// through w.Dispatch") and does the real work in a goroutine, pushing
	// the result back into the page by calling a small JS-side resolver
	// once it is ready; asyncResolveJS below is the shared plumbing both
	// this and bmHistory use for that.
	if err := w.Bind("bmSessionInsight", func(sessionID string) {
		go func() {
			js, err := asyncResolveJS("__bmSessionInsightResolve", sessionID, a.sessionInsight(st, sessionID))
			if err != nil {
				log.Println("bmSessionInsight: encode result:", err)
				return
			}
			w.Dispatch(func() { w.Eval(js) })
		}()
	}); err != nil {
		log.Println("could not bind bmSessionInsight:", err)
	}

	// bmHistory (P2): queried on demand from the History tab's filter
	// controls, not polled, so it re-reads the whole store on every call
	// rather than the windowed query bmLive uses for its 2-second poll.
	// Same UI-thread concern as bmSessionInsight above (a large store's
	// AllEvents plus history.Build's own aggregation can run past 50ms), so
	// it returns immediately and resolves asynchronously too; unlike
	// bmSessionInsight this fires once per filter change, not once per
	// session, so the win here is "never freezes on a big store" rather
	// than "no longer fires hundreds at once".
	if err := w.Bind("bmHistory", func(reqID string, f history.Filter) {
		go func() {
			payload, err := a.historyPayload(st, f)
			if err != nil {
				return
			}
			js, err := asyncResolveJS("__bmHistoryResolve", reqID, payload)
			if err != nil {
				log.Println("bmHistory: encode result:", err)
				return
			}
			w.Dispatch(func() { w.Eval(js) })
		}()
	}); err != nil {
		log.Println("could not bind bmHistory:", err)
	}

	// bmVendorStrip (P3): the Now page's vendor strip calls this from its
	// own 1-minute timer, not bmLive's 2-second poll, so it runs one SQL
	// aggregate against the whole events table rather than bmLive's
	// windowed read.
	if err := w.Bind("bmVendorStrip", func() (vendorstrip.Payload, error) {
		return a.vendorStrip(st)
	}); err != nil {
		log.Println("could not bind bmVendorStrip:", err)
	}

	// bmForecast (F1): the Now page's forecast chart calls this from its own
	// 1-minute timer, same reason as bmVendorStrip above; it also carries
	// the scoring bookkeeping (EnsureScored), so a week's plan and actual
	// get recorded even if the Now page forecast section is never scrolled
	// to, as long as the app is open at least once a minute somewhere in
	// that week.
	if err := w.Bind("bmForecast", func() (forecast.Payload, error) {
		return a.forecastPayload(st)
	}); err != nil {
		log.Println("could not bind bmForecast:", err)
	}

	startUICheckServer(w)

	if err := w.Bind("ccSaveSettings", func(p settingsPayload) error {
		if err := a.applySettings(p); err != nil {
			return err
		}
		go func() {
			if _, err := a.rebuild(true, progressReporter(w)); err != nil {
				log.Println("rebuild after settings save failed:", err)
				w.Dispatch(func() { w.Eval("window.ccSettingsRebuildFailed && window.ccSettingsRebuildFailed()") })
				return
			}
			resetWSLTicker()
			w.Dispatch(func() { w.Eval("window.ccReload ? ccReload() : location.reload()") })
		}()
		return nil
	}); err != nil {
		log.Println("could not bind ccSaveSettings:", err)
	}

	go func() {
		ticker := time.NewTicker(*interval)
		defer ticker.Stop()
		for range ticker.C {
			// The 15-minute (or whatever -interval is) tier never touches
			// WSL: RefreshSlow false, so Collect reuses the last full
			// pass's slow-tier file list instead of walking it again.
			if _, err := a.rebuild(false, progressReporter(w)); err != nil {
				log.Println("scheduled collection failed:", err)
				continue
			}
			// Reload the page so the new snapshot shows without a click.
			// ccReload (app chrome) stashes the active tab first so the
			// reload lands back on the same tab, not on Overview.
			w.Dispatch(func() { w.Eval("window.ccReload ? ccReload() : location.reload()") })
		}
	}()

	if wslTicker != nil {
		go func() {
			defer wslTicker.Stop()
			for range wslTicker.C {
				if _, err := a.rebuild(true, progressReporter(w)); err != nil {
					log.Println("scheduled WSL collection failed:", err)
					continue
				}
				w.Dispatch(func() { w.Eval("window.ccReload ? ccReload() : location.reload()") })
			}
		}()
	}

	w.Run()
}

// progressReporter turns a Collect-shaped (done,total) callback into live
// updates on the page via ccProgress(done, total, etaSeconds|null). Call it
// fresh immediately before each rebuild so its elapsed-time clock, which the
// estimate is based on, starts at zero every time.
func progressReporter(w webview2.WebView) func(done, total int) {
	start := time.Now()
	return func(done, total int) {
		if total <= 0 {
			return
		}
		eta := "null"
		if done > 0 {
			perFile := time.Since(start) / time.Duration(done)
			remaining := perFile * time.Duration(total-done)
			eta = strconv.Itoa(int(remaining.Seconds()))
		}
		d, t := done, total
		w.Dispatch(func() {
			w.Eval(fmt.Sprintf("window.ccProgress && window.ccProgress(%d,%d,%s)", d, t, eta))
		})
	}
}

// asyncResolveJS builds the JS statement that delivers a result computed off
// the UI thread back into the page: bmSessionInsight and bmHistory both
// return immediately from their bound Go function and finish their real
// work in a goroutine (see their own comments, v0.2.1 hang patch), then call
// this to hand the result to a small page-side resolver by key (a sessionID
// or a request id) instead of through go-webview2's own auto-generated
// per-call Promise, which only resolves with whatever the bound function
// returns synchronously. encoding/json's default HTML escaping makes a
// literal "</script>" impossible in either json.Marshal call below, the same
// property report.Render's own doc comment relies on.
func asyncResolveJS(resolverName, key string, data any) (string, error) {
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


// ---------------------------------------------------------------------------
// Single instance and the WebView2-missing fallback
// ---------------------------------------------------------------------------

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW         = user32.NewProc("FindWindowW")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

// alreadyRunning claims a named mutex for the life of the process. Windows
// reports ERROR_ALREADY_EXISTS when another instance already holds it, even
// though the returned handle is otherwise valid; that is the documented way
// to detect this without a helper class.
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

const (
	mbOK          = 0x00000000
	mbIconWarning = 0x00000030
)

// runWithoutWindow covers the very unlikely case of a managed machine
// missing the WebView2 runtime: collect once, open the result in the
// default browser, and say why there is no app window.
func runWithoutWindow(a *app) {
	if _, err := a.rebuild(true, nil); err != nil {
		log.Println("fallback collection failed:", err)
	}
	openInBrowser(a.htmlPath)
	showWebView2MissingMessage()
}

func openInBrowser(path string) {
	_ = exec.Command("cmd", "/c", "start", "", path).Start()
}

func showWebView2MissingMessage() {
	text, _ := windows.UTF16PtrFromString(
		"Claude Cost could not open its window because the WebView2 runtime is missing.\n" +
			"The dashboard was opened in your default browser instead. Installing the\n" +
			"WebView2 runtime (part of Microsoft Edge) will let the app window open normally.")
	caption, _ := windows.UTF16PtrFromString(windowTitle)
	_, _ = windows.MessageBox(0, text, caption, mbOK|mbIconWarning)
}

// app.go holds everything the WebView2 window (main.go, Windows only) and
// the browser-mode entry point (main_other.go, B1: darwin and linux) share:
// the app struct, the one rebuild path, live/poll wiring, Settings, and the
// HTML chrome rebuild() injects into the CLI's own template. No build tag:
// none of this touches a Windows-only API.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"burnmon/internal/adapter/codex"
	"burnmon/internal/adapter/copilotcli"
	"burnmon/internal/adapter/copilotvsc"
	"burnmon/internal/adapter/hermes"
	"burnmon/internal/dataset"
	"burnmon/internal/pricing"
	"burnmon/internal/report"
	"burnmon/internal/scan"
	"burnmon/internal/store"
	"burnmon/internal/watch"
)

const (
	version        = "0.3.2"
	minInterval    = 5 * time.Minute
	minWSLInterval = 15 * time.Minute
)

func init() {
	// v0.2.1 hang patch (SESSION_LOG.md): Refresh's own live (reachable)
	// footprint against Wilco's real store measured at well under 100 MB
	// (isolated AllEvents + a forced GC), but Go's default GC pacer (GOGC
	// 100, no soft limit) lets HeapAlloc run up to whatever the last
	// collection's live size implies before the next one, and a rebuild's
	// burst of allocation (Collect's own event/session slices, plus however
	// many bmSessionInsight/bmHistory goroutines the page fires at once)
	// measured that at 1+ GB before a GC ever ran. A 400 MB soft memory
	// limit (matching the Done-when target) makes the runtime collect
	// proactively as usage approaches it rather than waiting for the heap
	// to double; a burst that briefly needs more than 400 MB of genuinely
	// live data still gets it (this is a soft target, not a hard cap), the
	// difference is only how eagerly garbage gets reclaimed under load.
	debug.SetMemoryLimit(400 << 20)
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ", ") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// appDataDir is %LOCALAPPDATA%\burnmon on Windows (unchanged); on darwin and
// linux (B1) it is os.UserConfigDir()'s own per-OS default (~/Library/
// Application Support on darwin, $XDG_CONFIG_HOME or ~/.config on linux),
// joined with "burnmon", matching store.DefaultPath's own GOOS split so the
// app's log, dashboard.html and burnmon.json land next to the same store
// the CLI reads.
func appDataDir() string {
	if runtime.GOOS == "windows" {
		if lad := os.Getenv("LOCALAPPDATA"); lad != "" {
			return filepath.Join(lad, "burnmon")
		}
		return filepath.Join(os.TempDir(), "burnmon")
	}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "burnmon")
	}
	return filepath.Join(os.TempDir(), "burnmon")
}

// setupLog opens the app's log file in append mode, rotating it out of the
// way first if it has grown past 512KB. Truncating on every start (the
// previous behaviour) destroyed the previous run's trail on every restart,
// including the "another instance already running" case, which exits before
// logging anything else: exactly the log a "my dashboard is stuck / shows no
// data" report needs to diagnose from app.log alone.
func setupLog(dataDir string) {
	path := filepath.Join(dataDir, "burnmon-app.log")
	if fi, err := os.Stat(path); err == nil && fi.Size() > 512*1024 {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(f)
}

// ---------------------------------------------------------------------------
// app: the one rebuild path shared by startup, both tickers, Refresh now and
// Settings save
// ---------------------------------------------------------------------------

type app struct {
	cfg         pricing.Config
	seat        string
	monthsN     int
	sources     []string
	htmlPath    string
	interval    time.Duration
	wslInterval time.Duration
	cfgPath     string

	cache      dataset.Cache
	wslDistros []string
	mu         sync.Mutex
	building   atomic.Bool

	liveWatcher *watch.Watcher
}

// startLiveWatch starts the Now page's file watcher on nativeClaudeRoots and
// nativeCodexRoots: fsnotify, recursively, no WSL yet (see extendLiveWatchWSL).
// Called once, synchronously, from main() before the first backfill even
// starts (v0.1.1 F2: previously this ran only after the first full rebuild
// completed, so a live turn arriving during that backfill, which can take
// minutes over a large existing history, had no watcher to catch it at
// all). onChange calls IngestFile directly, without a.mu: dataset.Cache
// guards its own shared state (rootsMu) and the store serialises through
// its single connection, so a live ingest is never blocked behind a
// concurrently-running Collect the way it would be if this held a.mu for
// that call, as rebuild() itself no longer does either.
func (a *app) startLiveWatch(nativeClaudeRoots, nativeCodexRoots []string) {
	if a.liveWatcher != nil {
		return
	}
	nativeRoots := append(append([]string{}, nativeClaudeRoots...), nativeCodexRoots...)
	wt, err := watch.New(nativeRoots, nil, func(path string) {
		// A shallow copy, not a live pointer into a.cfg: same read-safety
		// idiom rebuild() already uses (see its own comment above), since
		// this callback runs with no lock held against a concurrent
		// Settings save.
		cfg := a.cfg
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

// extendLiveWatchWSL adds whatever WSL roots the first full backfill
// resolved (a.cache.RootsByAdapter, now populated) to the already-running
// live watcher, once, right after that backfill completes. A no-op when
// wsl_scan is off, no distro was found, or the watcher never started.
func (a *app) extendLiveWatchWSL(nativeClaudeRoots, nativeCodexRoots []string) {
	if a.liveWatcher == nil {
		return
	}
	full := a.cache.RootsSnapshot()
	fullRoots := append(append([]string{}, full["claude"]...), full["codex"]...)
	nativeRoots := append(append([]string{}, nativeClaudeRoots...), nativeCodexRoots...)
	a.liveWatcher.AddWSLRoots(diffRootsCaseInsensitive(fullRoots, nativeRoots))
}

// diffRootsCaseInsensitive returns the entries of all not present in remove,
// comparing case-insensitively (Windows paths). Local, minimal copy of
// dataset's own subtractCaseInsensitive: not worth exporting one function
// for one caller outside that package.
func diffRootsCaseInsensitive(all, remove []string) []string {
	skip := make(map[string]bool, len(remove))
	for _, r := range remove {
		skip[strings.ToLower(r)] = true
	}
	var out []string
	for _, a := range all {
		if !skip[strings.ToLower(a)] {
			out = append(out, a)
		}
	}
	return out
}

// startHermesPoll starts A1's 5-second Hermes poll, a no-op if no Hermes
// install is found (DefaultDBPath returns ""). No fsnotify watch, unlike
// startLiveWatch's Claude/Codex trails: a SQLite WAL file's own writes do
// not fit watch.Watcher's file-offset, .jsonl-only design (see
// internal/adapter/hermes's package doc). Hermes needs no cursor either:
// PollOnce returns every session's current running totals on every call,
// and UpsertEvents' own "largest output wins" upsert (RequestID fixed to
// the session id) already skips a no-op write when a session has not grown.
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

// startCopilotCLIPoll starts A2's 5-second Copilot CLI poll, a no-op if no
// Copilot CLI install is found (DefaultDBPath returns ""). Same shape as
// startHermesPoll for the same reason: session-store.db is a SQLite WAL
// file, not a .jsonl trail, so it does not fit watch.Watcher's
// fsnotify-plus-offset design; PollOnce needs no cursor either, since it
// always returns every session's current latest-row totals and the store's
// own upsert skips a no-op write when a session has not grown.
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

// startCopilotVSCPoll starts A4's 5-second Copilot-in-VS-Code poll, a no-op
// when burnmon.json carries no copilot_vscode_otel_file (the two VS Code
// settings in the README are not set up, or the config just doesn't name
// where they write to). Same shape as startHermesPoll/startCopilotCLIPoll,
// except that copilotvsc.Tail reads only the lines appended since the last
// tick (WS2 follow-up, 2026-09-29, item 5; it used to re-read the whole,
// ever-growing file every call) and the store's upsert (largest output per
// RequestID) makes the one full read after a start a no-op for stored rows.
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

var errRebuildBusy = errors.New("a collection is already running")

// rebuild collects, renders and writes dashboard.html, reporting progress
// through progress if non-nil. It is guarded so only one collection runs at
// a time; a call that lands while another is already in flight (a tick
// firing mid-refresh) is skipped, not queued, and reports errRebuildBusy.
//
// refreshSlow is threaded straight through to dataset.CollectOpts.
// RefreshSlow: true means this pass is allowed to (re)resolve and walk WSL
// sources, false means it must not touch WSL at all and instead reuses
// whatever the last true pass found. See "Two refresh cadences" in
// docs/2026-08-17_wsl-source-detection-design.md.
func (a *app) rebuild(refreshSlow bool, progress func(done, total int)) (time.Time, error) {
	if !a.building.CompareAndSwap(false, true) {
		return time.Time{}, errRebuildBusy
	}
	defer a.building.Store(false)

	// F2: copy the config and take everything else Collect needs under a
	// brief lock, then run Collect (the long part: it lists, reads and
	// ingests every source file) without holding a.mu for the whole call.
	// Before this fix, a.mu was held for rebuild's entire body, so the live
	// watcher's IngestFile calls (also gated on a.mu) queued behind
	// whichever rebuild was in flight, including the multi-minute initial
	// backfill, matching the reported "Codex card minutes late" symptom
	// exactly. dataset.Cache now guards its own shared state internally
	// (rootsMu), so this is safe; Collect gets its own cfg snapshot rather
	// than a live pointer into a.cfg, so a concurrent Settings save cannot
	// race it (that save triggers its own follow-up rebuild regardless).
	a.mu.Lock()
	cfg := a.cfg
	opts := dataset.CollectOpts{
		Seat:        a.seat,
		MonthsN:     a.monthsN,
		Sources:     a.sources,
		RefreshSlow: refreshSlow,
	}
	a.mu.Unlock()

	collectStart := time.Now()
	payload, err := a.cache.Collect(&cfg, opts, progress)
	log.Printf("stage Collect (total): %v", time.Since(collectStart))
	if err != nil {
		return time.Time{}, err
	}
	{
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		log.Printf("debug: after Collect, HeapAlloc=%d bytes, Sys=%d bytes", mem.HeapAlloc, mem.Sys)
	}
	// Reflects whatever the most recent WSL probe (if any ran this pass, or
	// the last pass that did) actually found; empty when WSL is off, not
	// installed, or found nothing.
	wslDistros := scan.WSLDistroNames()
	logSourceScan(a.cache.Sources, a.cache.Files)
	marshalStart := time.Now()
	blob, err := json.Marshal(payload)
	if err != nil {
		return time.Time{}, err
	}
	log.Printf("stage json.Marshal: %v, %d bytes", time.Since(marshalStart), len(blob))
	renderStart := time.Now()
	html, err := report.Render(blob)
	if err != nil {
		return time.Time{}, err
	}
	log.Printf("stage report.Render: %v", time.Since(renderStart))

	a.mu.Lock()
	a.wslDistros = wslDistros
	built := time.Now()
	html = a.applyAppChrome(html)
	a.mu.Unlock()

	writeStart := time.Now()
	if err := os.WriteFile(a.htmlPath, []byte(html), 0o600); err != nil {
		return time.Time{}, err
	}
	log.Printf("stage WriteFile: %v, %d bytes", time.Since(writeStart), len(html))
	return built, nil
}

// logSourceScan writes one line per scanned source folder with the number of
// transcript files found under it, plus a total. Kept deliberately terse: it
// exists so a "my dashboard stopped at last month" report can be diagnosed
// from app.log alone, by seeing which folders were scanned and which were
// empty, without touching the user's machine.
func logSourceScan(sources, files []string) {
	for _, src := range sources {
		n := 0
		prefix := src + string(os.PathSeparator)
		for _, f := range files {
			if strings.HasPrefix(f, prefix) {
				n++
			}
		}
		log.Printf("source %s: %d transcript files", src, n)
	}
	log.Printf("sources scanned: %d, transcript files total: %d", len(sources), len(files))
}

// ---------------------------------------------------------------------------
// Settings: the Subscription block of the pricing config, editable from the
// window itself instead of by hand-editing burnmon.json.
// ---------------------------------------------------------------------------

// settingsPayload is the shape ccSaveSettings receives from the settings
// modal's Save button. Field names match the JS object literal exactly;
// go-webview2 unmarshals the JS argument straight into this struct.
type settingsPayload struct {
	YourSeat                  string  `json:"yourSeat"`
	MonthlySubscriptionEUR    float64 `json:"monthlySubscriptionEUR"`
	MonthlySubscriptionUSD    float64 `json:"monthlySubscriptionUSD"`
	SeatsPurchased            int     `json:"seatsPurchased"`
	StandardSeats             int     `json:"standardSeats"`
	PremiumSeats              int     `json:"premiumSeats"`
	StandardSeatPriceUSD      float64 `json:"standardSeatPriceUSD"`
	PremiumSeatPriceUSD       float64 `json:"premiumSeatPriceUSD"`
	UsageCreditsBalanceEUR    float64 `json:"usageCreditsBalanceEUR"`
	UsageCreditsSpentEUR      float64 `json:"usageCreditsSpentEUR"`
	UsageCreditsMonthlyCapEUR float64 `json:"usageCreditsMonthlyCapEUR"`
	CompanyConsumptionUSD     float64 `json:"companyConsumptionUSD"`
	OutputCostFactor          float64 `json:"outputCostFactor"`
	CalibratedOn              string  `json:"calibratedOn"`
	Window                    string  `json:"window"`
}

// applySettings writes p to burnmon.json (preserving any other keys
// already in that file, such as an unusual Prices override or the wsl_scan
// / extra_sources fields), then updates the running config and seat in
// memory. Cost is computed fresh from the config at Collect time, so
// nothing needs invalidating here; the caller triggers the actual rebuild
// afterward.
func (a *app) applySettings(p settingsPayload) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	sub := pricing.Subscription{
		MonthlySubscriptionEUR:    p.MonthlySubscriptionEUR,
		MonthlySubscriptionUSD:    p.MonthlySubscriptionUSD,
		SeatsPurchased:            p.SeatsPurchased,
		Seats:                     map[string]int{"Standard": p.StandardSeats, "Premium": p.PremiumSeats},
		SeatPriceUSD:              map[string]float64{"Standard": p.StandardSeatPriceUSD, "Premium": p.PremiumSeatPriceUSD},
		UsageCreditsBalanceEUR:    p.UsageCreditsBalanceEUR,
		UsageCreditsSpentEUR:      p.UsageCreditsSpentEUR,
		UsageCreditsMonthlyCapEUR: p.UsageCreditsMonthlyCapEUR,
		CompanyConsumptionUSD:     p.CompanyConsumptionUSD,
		OutputCostFactor:          p.OutputCostFactor,
		CalibratedOn:              p.CalibratedOn,
		Window:                    p.Window,
		YourSeat:                  p.YourSeat,
	}
	if err := writeConfigKey(a.cfgPath, "subscription", sub); err != nil {
		return err
	}

	a.cfg.Subscription = sub
	if p.YourSeat == "Standard" || p.YourSeat == "Premium" {
		a.seat = p.YourSeat
	}
	return nil
}

// writeConfigKey merges value into the named key of the JSON file at path,
// leaving any other keys (an unusual Prices override, wsl_scan,
// extra_sources) exactly as they were. A missing or unreadable existing file
// is treated as empty, not an error: this is very likely the first time
// anyone has saved from the window. Written via a .tmp file plus
// os.Rename, so a crash mid-write never corrupts the real file. Used by the
// settings dialog (ccSaveSettings, "subscription").
func writeConfigKey(path, key string, value interface{}) error {
	raw := map[string]json.RawMessage{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &raw)
	}
	valBytes, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw[key] = valBytes

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// escapeAttr is a minimal HTML attribute escape for the couple of text
// fields (CalibratedOn, Window) that land inside a value="..." attribute.
// Not a general-purpose escaper, just enough for values the user themselves
// typed into this same form a moment ago.
func escapeAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// ---------------------------------------------------------------------------
// HTML chrome: turn the CLI's rendered page into the app window's page.
// The embedded template.html itself is never touched.
// ---------------------------------------------------------------------------

const warmingPageTemplate = `<!doctype html><title>BurnMon</title><body style="font-family:sans-serif;background:#1f2733;
color:#eee;display:grid;place-items:center;height:100vh;margin:0;overflow:hidden">
<div style="text-align:center;min-width:320px">
 <div>Reading your session transcripts&hellip;</div>
 <div style="margin:14px auto 0;width:320px;height:8px;background:#334455;border-radius:4px;overflow:hidden">
  <div id="cc_warm_fill" style="width:0%;height:100%;background:#FFDD32;transition:width .2s"></div>
 </div>
 <div id="cc_warm_text" style="margin-top:8px;font-size:13px;color:#9fb4bd">Starting&hellip;</div>
 <div style="margin-top:22px;font-size:11px;color:#5b6b74">BurnMon vAPP_VERSION</div>
</div>
<script>
function ccProgress(done,total,eta){
  if(total<=0) return;
  var pct = Math.round(done/total*100);
  var fill = document.getElementById('cc_warm_fill');
  if(fill) fill.style.width = pct+'%';
  var t = document.getElementById('cc_warm_text');
  if(t) t.textContent = done+' / '+total+' files ('+pct+'%)'+(eta!=null && eta>0 ? ', about '+eta+'s left' : '');
}
function ccWarmFail(msg){
  var t = document.getElementById('cc_warm_text');
  if(t){ t.textContent = msg; t.style.color = '#ff8a65'; }
  var f = document.getElementById('cc_warm_fill');
  if(f) f.style.background = '#ff8a65';
}
</script>
</body>`

func warmingPageHTML() string {
	return strings.Replace(warmingPageTemplate, "APP_VERSION", version, 1)
}

const cliRebuildNotice = `<b>This page is rebuilt every time you run the burnmon CLI again.</b> It reads your session transcripts live at each run, so refreshing is simply running it again: a new report is written and opened for you.`

// appRebuildNotice produces the app window's "how this stays current" text.
// With no WSL distros found, it reproduces today's single-cadence wording
// byte for byte. With WSL distros found, it names them and states both
// cadences, since the "Snapshot taken" stamp would otherwise read as more
// current than the WSL half of the numbers actually is.
func appRebuildNotice(interval, wslInterval time.Duration, wslDistros []string) string {
	if len(wslDistros) == 0 {
		return fmt.Sprintf(`<b>This window keeps itself current.</b> It opens straight to your last snapshot and quietly catches up in the background within moments. It also re-reads your session transcripts every %d minutes while open, whenever you press Refresh now (top right), and right after you save changes in Settings (the gear icon).`,
			int(interval/time.Minute))
	}
	names := strings.Join(wslDistros, ", ")
	return fmt.Sprintf(`<b>This window keeps itself current.</b> It opens straight to your last snapshot and quietly catches up in the background within moments. Windows transcripts are re-read every %d minutes while open. WSL transcripts (%s) are re-read every %s, because reading Linux files from Windows is slow. Refresh now (top right) and saving Settings re-read everything, WSL included.`,
		int(interval/time.Minute), names, formatHours(wslInterval))
}

// formatHours renders a duration the way the header text wants it: whole
// hours as "N hour(s)", anything else as minutes.
func formatHours(d time.Duration) string {
	if d > 0 && d%time.Hour == 0 {
		h := int(d / time.Hour)
		if h == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", h)
	}
	return fmt.Sprintf("%d minutes", int(d/time.Minute))
}

// stampAnchor is the exact markup template.html renders for the "Snapshot
// taken" box in the header. Its own script fills it in by id, so it stays
// intact; we just wrap it together with our button so both sit in the same
// spot, top right, instead of adding a second, duplicate timestamp of our
// own at the bottom.
const stampAnchor = `<div class="stamp" id="stamp"></div>`

func (a *app) stampAreaHTML() string {
	wslStamp := ""
	if len(a.wslDistros) > 0 && !a.cache.SlowScannedAt.IsZero() {
		wslStamp = ` <span style="color:var(--muted);font-size:11px;white-space:nowrap" title="WSL transcripts are re-read on a slower, ` +
			formatHours(a.wslInterval) +
			` cycle because reading Linux files from Windows is slow; native Windows transcripts are current as of the main snapshot time.">WSL data as of ` +
			a.cache.SlowScannedAt.Format("15:04") + `</span>`
	}
	return `<div style="display:flex;align-items:center;gap:12px">
 <span style="color:var(--sun);font-size:11px;white-space:nowrap">v` + version + `</span>` + wslStamp + `
 <span id="cc_progress" style="display:none;color:var(--muted);font-size:12px;white-space:nowrap"></span>
 <button id="cc_settings_btn" class="act ghost" type="button" title="Subscription settings" style="padding:6px 10px;white-space:nowrap">&#9881;</button>
 <button id="cc_btn" class="act" type="button" style="padding:6px 12px;white-space:nowrap">Refresh now</button>
 ` + stampAnchor + `
</div>`
}

// appChromeStyle is a small CSS override, not a template edit: the Sessions
// tab's own scrollable table (".scroll.tall.with-filters", template.html's
// class, unique to that tab) is sized for a generic browser viewport. In our
// fixed 1280x860 window that leaves the page slightly taller than the
// window, so a second, outer scrollbar appears alongside the table's own
// one. Shrinking just that box's max-height keeps everything on one screen.
// Only the Sessions tab is affected; Months/Weeks/Days are untouched.
const appChromeStyle = `
<style>
.scroll.tall.with-filters{max-height:calc(100vh - 500px) !important}
</style>`

const appChromeScript = `
<script>
(function(){
  var btn = document.getElementById('cc_btn');
  if(!btn) return;
  var progressEl = document.getElementById('cc_progress');
  var hideTimer = null;

  window.ccProgress = function(done, total, eta){
    if(!progressEl || total<=0) return;
    var pct = Math.round(done/total*100);
    progressEl.style.display = 'inline';
    progressEl.textContent = pct+'% ('+done+'/'+total+')'+(eta!=null && eta>0 ? ', ~'+eta+'s left' : '');
    if(hideTimer) clearTimeout(hideTimer);
    if(done>=total){
      hideTimer = setTimeout(function(){ progressEl.style.display='none'; }, 800);
    }
  };

  // ccReload: reload the page for a fresh snapshot, landing back on the
  // tab the user was on. Tab state is in-memory only (see template.html's
  // tab navigation notes), so it is stashed in localStorage across the
  // reload. Used by both interval tickers, Refresh now, and Settings save.
  window.ccReload = function(){
    try{ localStorage.setItem('cc_tab', (typeof currentTab === 'function') ? currentTab() : 'overview'); }catch(e){}
    location.reload();
  };

  // Restore the stashed tab after a ccReload-driven reload.
  try{
    var savedTab = localStorage.getItem('cc_tab');
    if(savedTab){
      localStorage.removeItem('cc_tab');
      if(savedTab !== 'overview' && typeof showTab === 'function') showTab(savedTab);
    }
  }catch(e){}

  window.ccRefreshFailed = function(){
    btn.disabled = false;
    btn.textContent = 'Refresh failed – try again';
    btn.style.borderColor = 'var(--warn)';
    btn.style.color = 'var(--warn)';
  };

  btn.onclick = function(){
    btn.disabled = true;
    btn.style.borderColor = '';
    btn.style.color = '';
    btn.textContent = 'Rebuilding…';
    window.ccRefresh();
  };
})();
</script>`

func (a *app) applyAppChrome(html string) string {
	if n := strings.Count(html, cliRebuildNotice); n != 1 {
		log.Printf("warning: rebuild notice found %d times in the rendered template, expected 1", n)
	}
	html = strings.Replace(html, cliRebuildNotice, appRebuildNotice(a.interval, a.wslInterval, a.wslDistros), 1)

	if n := strings.Count(html, stampAnchor); n != 1 {
		log.Printf("warning: snapshot stamp found %d times in the rendered template, expected 1; Refresh now not placed", n)
	} else {
		html = strings.Replace(html, stampAnchor, a.stampAreaHTML(), 1)
	}

	if !strings.Contains(html, "</body>") {
		log.Println("warning: no </body> in the rendered template; chrome not injected")
		return html
	}
	return strings.Replace(html, "</body>", appChromeStyle+appChromeScript+a.settingsModalHTML()+"</body>", 1)
}

// settingsModalHTML renders the Settings overlay, pre-filled with the
// currently loaded subscription numbers and seat, so opening it always
// shows what the dashboard is actually using right now, not stale form
// defaults. Saving posts to ccSaveSettings (bound in main), which writes
// burnmon.json and rebuilds in the background.
func (a *app) settingsModalHTML() string {
	sub := a.cfg.Subscription
	std := sub.Seats["Standard"]
	prem := sub.Seats["Premium"]
	stdPrice := sub.SeatPriceUSD["Standard"]
	premPrice := sub.SeatPriceUSD["Premium"]
	selected := func(seat string) string {
		if a.seat == seat {
			return " selected"
		}
		return ""
	}
	f := func(v float64) string { return fmt.Sprintf("%v", v) }
	n := func(v int) string { return fmt.Sprintf("%d", v) }

	return `
<div id="cc_settings_overlay" style="display:none;position:fixed;inset:0;background:rgba(0,0,0,.55);z-index:1000;align-items:center;justify-content:center;font-family:'Inter','Segoe UI',sans-serif">
 <div style="background:#1f2733;color:#eee;border-radius:10px;padding:22px 26px;width:480px;max-height:86vh;overflow:auto;box-shadow:0 12px 40px rgba(0,0,0,.5)">
  <h3 style="margin:0 0 4px;font-size:16px">Subscription settings</h3>
  <p style="margin:0 0 16px;color:#9fb4bd;font-size:12px">Saved to ` + escapeAttr(a.cfgPath) + `.</p>
  <div id="cc_settings_error" style="display:none;margin-bottom:12px;color:#ff8a65;font-size:12px"></div>
  <style>
   #cc_settings_overlay label{display:flex;flex-direction:column;gap:4px;font-size:12px;color:#c8d4d9}
   #cc_settings_overlay input,#cc_settings_overlay select{background:#111a24;border:1px solid var(--pine-40);border-radius:6px;color:#fff;padding:6px 8px;font-size:13px}
   #cc_settings_overlay .grid{display:grid;grid-template-columns:1fr 1fr;gap:10px 14px}
  </style>
  <div class="grid">
   <label>Your seat
    <select id="cc_s_yourSeat">
     <option value="Standard"` + selected("Standard") + `>Standard</option>
     <option value="Premium"` + selected("Premium") + `>Premium</option>
    </select></label>
   <label>Monthly subscription (EUR)<input id="cc_s_subEUR" type="number" step="0.01" min="0" value="` + f(sub.MonthlySubscriptionEUR) + `"></label>
   <label>Monthly subscription (USD)<input id="cc_s_subUSD" type="number" step="0.01" min="0" value="` + f(sub.MonthlySubscriptionUSD) + `"></label>
   <label>Seats purchased<input id="cc_s_seatsTotal" type="number" step="1" min="0" value="` + n(sub.SeatsPurchased) + `"></label>
   <div></div>
   <label>Standard seats<input id="cc_s_seatsStd" type="number" step="1" min="0" value="` + n(std) + `"></label>
   <label>Premium seats<input id="cc_s_seatsPrem" type="number" step="1" min="0" value="` + n(prem) + `"></label>
   <label>Standard seat price (USD)<input id="cc_s_priceStd" type="number" step="0.01" min="0" value="` + f(stdPrice) + `"></label>
   <label>Premium seat price (USD)<input id="cc_s_pricePrem" type="number" step="0.01" min="0" value="` + f(premPrice) + `"></label>
   <label>Usage credits balance (EUR)<input id="cc_s_creditsBal" type="number" step="0.01" min="0" value="` + f(sub.UsageCreditsBalanceEUR) + `"></label>
   <label>Usage credits spent (EUR)<input id="cc_s_creditsSpent" type="number" step="0.01" min="0" value="` + f(sub.UsageCreditsSpentEUR) + `"></label>
   <label>Usage credits monthly cap (EUR)<input id="cc_s_creditsCap" type="number" step="0.01" min="0" value="` + f(sub.UsageCreditsMonthlyCapEUR) + `"></label>
   <label>Company consumption (USD)<input id="cc_s_companyUSD" type="number" step="0.01" min="0" value="` + f(sub.CompanyConsumptionUSD) + `"></label>
  </div>
  <details style="margin-top:14px">
   <summary style="cursor:pointer;color:#9fb4bd;font-size:12px">Advanced (recalibration)</summary>
   <div class="grid" style="margin-top:10px">
    <label>Output cost factor<input id="cc_s_factor" type="number" step="0.0001" min="0" value="` + f(sub.OutputCostFactor) + `"></label>
    <div></div>
    <label>Calibrated on<input id="cc_s_calibratedOn" type="text" value="` + escapeAttr(sub.CalibratedOn) + `"></label>
    <label>Window<input id="cc_s_window" type="text" value="` + escapeAttr(sub.Window) + `"></label>
   </div>
  </details>
  <div style="display:flex;justify-content:flex-end;gap:10px;margin-top:20px">
   <button id="cc_s_cancel" style="padding:6px 14px;border:1px solid var(--pine-40);border-radius:6px;background:transparent;color:#fff;font:13px 'Inter','Segoe UI',sans-serif;cursor:pointer">Cancel</button>
   <button id="cc_s_save" style="padding:6px 14px;border:1px solid #FFDD32;border-radius:6px;background:transparent;color:#FFDD32;font:13px 'Inter','Segoe UI',sans-serif;cursor:pointer">Save and reload</button>
  </div>
 </div>
</div>
<script>
(function(){
  var btn = document.getElementById('cc_settings_btn');
  var overlay = document.getElementById('cc_settings_overlay');
  if(!btn || !overlay) return;
  var errEl = document.getElementById('cc_settings_error');
  var saveBtn = document.getElementById('cc_s_save');

  btn.onclick = function(){ if(errEl) errEl.style.display='none'; overlay.style.display='flex'; };
  var cancelBtn = document.getElementById('cc_s_cancel');
  if(cancelBtn) cancelBtn.onclick = function(){ overlay.style.display='none'; };

  window.ccSettingsRebuildFailed = function(msg){
    document.body.style.opacity = '';
    if(saveBtn){ saveBtn.disabled = false; saveBtn.textContent = 'Save and reload'; }
    if(errEl){ errEl.textContent = msg || 'Rebuild failed after saving; your numbers were kept, try Refresh now.'; errEl.style.display = 'block'; }
  };

  if(saveBtn) saveBtn.onclick = function(){
    var num = function(id){ var v = parseFloat(document.getElementById(id).value); return isNaN(v) ? 0 : v; };
    var int = function(id){ var v = parseInt(document.getElementById(id).value, 10); return isNaN(v) ? 0 : v; };
    var payload = {
      yourSeat: document.getElementById('cc_s_yourSeat').value,
      monthlySubscriptionEUR: num('cc_s_subEUR'),
      monthlySubscriptionUSD: num('cc_s_subUSD'),
      seatsPurchased: int('cc_s_seatsTotal'),
      standardSeats: int('cc_s_seatsStd'),
      premiumSeats: int('cc_s_seatsPrem'),
      standardSeatPriceUSD: num('cc_s_priceStd'),
      premiumSeatPriceUSD: num('cc_s_pricePrem'),
      usageCreditsBalanceEUR: num('cc_s_creditsBal'),
      usageCreditsSpentEUR: num('cc_s_creditsSpent'),
      usageCreditsMonthlyCapEUR: num('cc_s_creditsCap'),
      companyConsumptionUSD: num('cc_s_companyUSD'),
      outputCostFactor: num('cc_s_factor'),
      calibratedOn: document.getElementById('cc_s_calibratedOn').value,
      window: document.getElementById('cc_s_window').value
    };
    saveBtn.disabled = true;
    saveBtn.textContent = 'Saving…';
    if(errEl) errEl.style.display = 'none';
    window.ccSaveSettings(payload).then(function(){
      saveBtn.textContent = 'Rebuilding…';
      document.body.style.opacity = '.6';
    }).catch(function(err){
      saveBtn.disabled = false;
      saveBtn.textContent = 'Save and reload';
      if(errEl){ errEl.textContent = String(err); errEl.style.display = 'block'; }
    });
  };
})();
</script>`
}

func toFileURL(path string) string {
	u := &url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(path)}
	return u.String()
}

// startupFailureMessage turns a Collect error from the very first pass (no
// dashboard.html yet to fall back on) into plain text for the warming page.
// Before this, any of these three errors was logged and then silently
// swallowed, leaving the window stuck on "Reading your session
// transcripts... Starting..." forever: exactly what a chat-only user, or
// anyone whose Cowork sessions all run in Anthropic's cloud, hits, since
// neither leaves a local transcript for this machine to find.
func startupFailureMessage(err error) string {
	switch {
	case errors.Is(err, dataset.ErrNoSources):
		return "No Claude transcript folders were found on this machine. Cowork sessions that run in Anthropic's cloud leave no local transcript, so there may be nothing local to read yet."
	case errors.Is(err, dataset.ErrNoFiles):
		return "Claude's folders exist but contain no transcript files. Cloud Cowork sessions and claude.ai browser chats leave no local transcript."
	case errors.Is(err, dataset.ErrNoSessions):
		return "Transcripts were found, but none with usage inside the reporting window."
	default:
		return "Could not read usage data: " + err.Error()
	}
}

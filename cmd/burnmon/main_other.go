//go:build !windows

// Command burnmon (darwin, linux; B1, 2026-09-23_v0.3_spec.md 2.4): the same
// dashboard.html the Windows WebView2 window shows, opened in the system
// default browser instead. WebView2 itself is Windows-only (jchv/go-webview2
// wraps the Win32 runtime), so the page's bound live functions (bmLive,
// ccRefresh, ...) are served by browserBridge (browser.go), a localhost-only
// HTTP server, rather than injected into a window.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"burnmon/internal/adapter/codex"
	"burnmon/internal/pricing"
	"burnmon/internal/scan"
	"burnmon/internal/store"
)

func main() {
	port := flag.Int("port", 47615, "localhost port the dashboard is served on")
	noOpen := flag.Bool("no-open", false, "do not open the dashboard in the browser")
	interval := flag.Duration("interval", 15*time.Minute, "how often to re-read transcripts and rewrite the dashboard")
	monthsN := flag.Int("months", 2, "how many months to include, counting the current one")
	seat := flag.String("seat", "Standard", "your own seat tier (Standard or Premium)")
	cfgPath := flag.String("config", "", "config file overriding the compiled-in prices and subscription (default: burnmon.json in the app's data folder, if present)")
	var sources multiFlag
	flag.Var(&sources, "source", "extra folder to scan; repeatable, overrides auto-detect")
	flag.Parse()

	if *interval < minInterval {
		*interval = minInterval
	}

	dataDir := appDataDir()
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return
	}
	setupLog(dataDir)
	log.Printf("burnmon %s starting, browser mode (%s/%s)", version, runtime.GOOS, runtime.GOARCH)

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

	cfgSavePath := cfgFile
	if cfgSavePath == "" {
		cfgSavePath = filepath.Join(dataDir, "burnmon.json")
	}

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

	a := &app{
		cfg:      cfg,
		seat:     *seat,
		monthsN:  *monthsN,
		sources:  []string(sources),
		htmlPath: filepath.Join(dataDir, "dashboard.html"),
		interval: *interval,
		cfgPath:  cfgSavePath,
	}
	a.cache.Store = st

	// Serve the page before the first collection: the browser opens on the
	// last snapshot (or the warming page on a first run) straight away, and
	// the page reloads itself once the collection below rewrites it. A port
	// already in use most likely means another instance: open that one.
	bridge := &browserBridge{a: a, st: st, addr: fmt.Sprintf("127.0.0.1:%d", *port)}
	ln, err := net.Listen("tcp", bridge.addr)
	if err != nil {
		log.Printf("could not listen on %s (%v); assuming burnmon is already running there", bridge.addr, err)
		if !*noOpen {
			openInBrowser(bridge.url())
		}
		return
	}
	go func() {
		if err := http.Serve(ln, bridge.handler()); err != nil {
			log.Println("browser bridge stopped:", err)
		}
	}()
	log.Println("dashboard served at", bridge.url())
	fmt.Println("BurnMon dashboard:", bridge.url())
	if !*noOpen {
		openInBrowser(bridge.url())
	}

	// Cowork (Cloud): the exe-adjacent .burnmon/cloud drop folder, created
	// before the roots are resolved so the live watcher watches it.
	if err := scan.EnsureCloudDropDir(); err != nil {
		log.Println("cloud drop folder:", err)
	}
	nativeClaudeRoots := scan.DefaultSourcesWithOptions(false)
	nativeCodexRoots := codex.NativeSources()
	a.cache.SeedNativeRoots(nativeClaudeRoots, nativeCodexRoots)
	a.startLiveWatch(nativeClaudeRoots, nativeCodexRoots)
	startHermesPoll(a, st)
	startCopilotCLIPoll(a, st)
	startCopilotVSCPoll(a, st)

	if _, err := a.rebuild(true, nil); err != nil {
		log.Println("initial collection failed:", err)
		bridge.setStartErr(startupFailureMessage(err))
	} else {
		a.extendLiveWatchWSL(nativeClaudeRoots, nativeCodexRoots)
	}

	// Re-collect on -interval; the open page notices the rewritten
	// dashboard.html (bridgeScript polls /api/built) and reloads itself. No
	// WSL ticker: scan's own wsl_other.go stub already makes WSL detection a
	// no-op off Windows.
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for range ticker.C {
		if _, err := a.rebuild(false, nil); err != nil {
			log.Println("rebuild failed:", err)
		}
	}
}

// openInBrowser opens url in the platform's
// default browser: "open" on darwin, "xdg-open" on linux (both bundled with
// the OS, no extra dependency). Any other GOOS just logs the path, since
// CGO_ENABLED=0 cross-compilation is not restricted to these two.
func openInBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		log.Println("do not know how to open a browser on", runtime.GOOS, "; open manually:", url)
		return
	}
	if err := cmd.Start(); err != nil {
		log.Println("could not open the default browser:", err)
	}
}

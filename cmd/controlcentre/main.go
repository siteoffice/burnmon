//go:build windows || (darwin && cgo)

// Command controlcentre is the Control Centre: a local window (WebView2 on
// Windows, WKWebView via webview_go on macOS) over the units, flows and
// sprints below one root. No server, no port, no network call. Spec:
// C:\ZND\10_holding\05_docs\superpowers\specs\2026-11-11-burnmon-control-centre-slice-1.md
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"burnmon/internal/centre"
	"burnmon/internal/controls"
	"burnmon/internal/snapshot"
	"burnmon/internal/store"
)

const windowTitle = "Control Centre"

func init() {
	// The webview message loop is tied to the thread that created the
	// window; keep that on one OS thread, same as cmd\burnmon-dev.
	runtime.LockOSThread()
}

func main() {
	rootFlag := flag.String("root", "", "the folder to show; default the settings root, else the current folder")
	flag.Parse()

	dataDir := centre.AppDataDir(os.Getenv("LOCALAPPDATA"), os.UserConfigDir)
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	settings, settingsPath := centre.LoadSettings(exeDir, dataDir)
	cwd, _ := os.Getwd()
	root := centre.ResolveRoot(*rootFlag, settings.Root, cwd)
	snapPath, _ := snapshot.DefaultPath()
	// The audit file sits next to the shared store, the file BurnMon Dev's
	// controls write (choices item 23), derived exactly as burnmon-dev does.
	// Without a store path, decisions are refused with the reason.
	var (
		svc               *controls.Service
		auditPath, svcWhy string
	)
	if storePath, err := store.DefaultPath(); err != nil {
		svcWhy = "the BurnMon data folder cannot be found: " + err.Error()
	} else {
		auditPath = controls.AuditPath(storePath)
		cfg := loadBurnmonConfig(exeDir, dataDir)
		svc = controls.NewService(newDecisionEnv(cfg, dataDir, snapPath, auditPath, exec.LookPath))
	}

	w, ok := openWindow(dataDir)
	if !ok {
		fmt.Fprintln(os.Stderr, "could not create the window; the Control Centre needs the WebView2 runtime on Windows")
		os.Exit(1)
	}
	defer w.Destroy()

	// Dispatch heartbeat, same reason as cmd\burnmon-dev\main.go: a Win32
	// modal loop (a window drag or resize) drops the thread message that
	// wakes the UI thread for a queued Dispatch, so a binding result could
	// wait until some later Dispatch. A no-op Dispatch every second caps
	// that wait at a second.
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			w.Dispatch(func() {})
		}
	}()

	bindAll(bindEnv{
		w: w, root: root, dataDir: dataDir, settingsPath: settingsPath,
		snapPath: snapPath, auditPath: auditPath, now: time.Now,
		svc: svc, svcWhy: svcWhy,
		settings: newSettingsBox(settings),
	})
	w.SetHtml(assemblePage(settings))
	w.Run()
}

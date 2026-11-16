//go:build windows || darwin

package main

import (
	"log"
	"os"
	"path/filepath"

	"burnmon/internal/controls"
	"burnmon/internal/pricing"
	"burnmon/internal/sections"
)

// loadBurnmonConfig reads the shared BurnMon config in the order
// loadConfigAt in cmd\burnmon-dev\app.go uses: burnmon.json, then
// claudecost.json, next to the exe ("" skips that tier), then the same two in
// dataDir. No file gives the defaults.
func loadBurnmonConfig(exeDir, dataDir string) pricing.Config {
	path := pricing.FindConfigFunc(exeDir, dataDir, isFile)
	cfg, err := pricing.Load(path)
	if err != nil {
		log.Println("config error:", err)
	}
	return cfg
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// newDecisionEnv is the controls Env BurnMon Dev builds in
// newControlServiceWith (cmd\burnmon-dev\controls_view.go), with the same
// fields: the switches come from BurnMon Dev's burnmon-dev-view.json in
// dataDir and the audit file is BurnMon Dev's (choices item 23), so one gear
// governs both apps. A binary that does not resolve leaves its path empty,
// which makes the controls that need it unavailable with a reason.
func newDecisionEnv(cfg pricing.Config, dataDir, snapPath, auditPath string, look func(string) (string, error)) controls.Env {
	switches := func() map[string]bool {
		return sections.Load(filepath.Join(dataDir, "burnmon-dev-view.json")).Controls
	}
	return controls.NewEnv(cfg, snapPath, auditPath, switches, look)
}

//go:build windows || darwin

package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"burnmon/internal/controls"
	"burnmon/internal/gitrefs"
	"burnmon/internal/graph"
	"burnmon/internal/pricing"
	"burnmon/internal/sections"
	"burnmon/internal/snapshot"
	"burnmon/internal/snapshotwriter"
)

// loadBurnmonConfig reads the shared BurnMon config in the order
// loadConfigAt in cmd\burnmon-dev\app.go uses: burnmon.json, then
// claudecost.json, next to the exe ("" skips that tier), then the same two in
// dataDir. No file gives the defaults.
func loadBurnmonConfig(exeDir, dataDir string) pricing.Config {
	var path string
	if exeDir != "" {
		path = firstFile(filepath.Join(exeDir, "burnmon.json"), filepath.Join(exeDir, "claudecost.json"))
	}
	if path == "" {
		path = firstFile(filepath.Join(dataDir, "burnmon.json"), filepath.Join(dataDir, "claudecost.json"))
	}
	cfg, err := pricing.Load(path)
	if err != nil {
		log.Println("config error:", err)
	}
	return cfg
}

func firstFile(paths ...string) string {
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

// newDecisionEnv is the controls Env BurnMon Dev builds in
// newControlServiceWith (cmd\burnmon-dev\controls_view.go), with the same
// fields: the switches come from BurnMon Dev's burnmon-dev-view.json in
// dataDir and the audit file is BurnMon Dev's (choices item 23), so one gear
// governs both apps. A binary that does not resolve leaves its path empty,
// which makes the controls that need it unavailable with a reason.
func newDecisionEnv(cfg pricing.Config, dataDir, snapPath, auditPath string, look func(string) (string, error)) controls.Env {
	env := controls.Env{
		AgentViewOn: cfg.AgentView,
		ForemanRoot: cfg.ForemanRoot,
		ReadForeman: graph.ReadForeman,
		Snapshot:    func() snapshot.Reading { return snapshot.Load(snapPath, time.Now()) },
		Switches: func() map[string]bool {
			return sections.Load(filepath.Join(dataDir, "burnmon-dev-view.json")).Controls
		},
		Self:      controls.CurrentSelf,
		Elevated:  controls.Elevated,
		Runner:    controls.ExecRunner{},
		AuditPath: auditPath,
		Merged:    gitrefs.NewChecker().MergedOrGone,
	}
	if p, err := snapshotwriter.ResolveClaudeConfigured(cfg.ClaudePath, look, runtime.GOOS); err == nil {
		env.ClaudePath = p
		env.AgentView = snapshotwriter.ClaudeAgentView(p)
	}
	if p, err := controls.ResolvePython(look, runtime.GOOS); err == nil {
		env.PythonPath = p
	}
	return env
}

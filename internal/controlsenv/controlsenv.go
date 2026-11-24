// Package controlsenv holds the glue that builds the controls Env from the
// BurnMon config, without any window or cgo code. cmd\controlcentre and
// cmd\znd both use it.
package controlsenv

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"burnmon/internal/controls"
	"burnmon/internal/pricing"
	"burnmon/internal/sections"
)

// LoadBurnmonConfig reads the shared BurnMon config in the order
// loadConfigAt in cmd\burnmon-dev\app.go uses: burnmon.json, then
// claudecost.json, next to the exe ("" skips that tier), then the same two in
// dataDir. No file gives the defaults.
func LoadBurnmonConfig(exeDir, dataDir string) pricing.Config {
	path := pricing.FindConfigFunc(exeDir, dataDir, IsFile)
	cfg, err := pricing.Load(path)
	if err != nil {
		log.Println("config error:", err)
	}
	return cfg
}

// IsFile reports whether p is an existing regular file (not a folder).
func IsFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// NewDecisionEnv is the controls Env BurnMon Dev builds in
// newControlServiceWith (cmd\burnmon-dev\controls_view.go), with the same
// fields: the switches come from BurnMon Dev's burnmon-dev-view.json in
// dataDir and the audit file is BurnMon Dev's (choices item 23), so one gear
// governs both apps. A binary that does not resolve leaves its path empty,
// which makes the controls that need it unavailable with a reason.
func NewDecisionEnv(cfg pricing.Config, dataDir, snapPath, auditPath string, look func(string) (string, error)) controls.Env {
	switches := func() map[string]bool {
		return sections.Load(filepath.Join(dataDir, "burnmon-dev-view.json")).Controls
	}
	return controls.NewEnv(cfg, snapPath, auditPath, switches, look)
}

// ZndConfig is znd.json. One key for now.
type ZndConfig struct {
	// GateWithoutSnapshot is controls.GateRefuse (default) or
	// controls.GateForemanRead.
	GateWithoutSnapshot string `json:"gate_without_snapshot"`
}

// LoadZndConfig finds znd.json next to the exe first (exeDir "" skips that
// tier), then in dataDir, as for burnmon.json. No file, no key or an empty
// value gives refuse. Any other value is an error that names it.
func LoadZndConfig(exeDir, dataDir string) (ZndConfig, error) {
	z := ZndConfig{GateWithoutSnapshot: controls.GateRefuse}
	dirs := []string{dataDir}
	if exeDir != "" {
		dirs = []string{exeDir, dataDir}
	}
	for _, d := range dirs {
		p := filepath.Join(d, "znd.json")
		if !IsFile(p) {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return z, fmt.Errorf("%s: %w", p, err)
		}
		var raw ZndConfig
		if err := json.Unmarshal(b, &raw); err != nil {
			return z, fmt.Errorf("%s: %w", p, err)
		}
		switch raw.GateWithoutSnapshot {
		case "":
		case controls.GateRefuse, controls.GateForemanRead:
			z.GateWithoutSnapshot = raw.GateWithoutSnapshot
		default:
			return z, fmt.Errorf("%s: gate_without_snapshot %q is not %s or %s", p, raw.GateWithoutSnapshot, controls.GateRefuse, controls.GateForemanRead)
		}
		return z, nil
	}
	return z, nil
}

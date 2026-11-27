//go:build windows || darwin

package main

import (
	"burnmon/internal/controls"
	"burnmon/internal/controlsenv"
	"burnmon/internal/pricing"
)

// loadBurnmonConfig and newDecisionEnv live in internal\controlsenv, where
// cmd\znd uses them too. These two names stay so the callers and the tests of
// this package keep their shape.
func loadBurnmonConfig(exeDir, dataDir string) pricing.Config {
	return controlsenv.LoadBurnmonConfig(exeDir, dataDir)
}

func newDecisionEnv(cfg pricing.Config, dataDir, snapPath, auditPath string, look func(string) (string, error)) controls.Env {
	return controlsenv.NewDecisionEnv(cfg, dataDir, snapPath, auditPath, look)
}

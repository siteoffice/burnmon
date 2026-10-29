package main

import (
	"fmt"
	"os"
)

// devSettingsEnv is the folder burnmon-dev.exe keeps its per-user settings in
// (view, window, dev config, wv2-dev, exports). scripts\uicheck-dev.ps1 sets it
// to a fresh temp folder per run, so a check never touches the real files.
const devSettingsEnv = "BURNMON_DEV_SETTINGS_DIR"

// devSettingsDir returns BURNMON_DEV_SETTINGS_DIR, or an error when it is
// unset. No fallback to %LOCALAPPDATA%\burnmon: a check reading settings
// there would be reading Wilco's real files, not the run's own.
func devSettingsDir() (string, error) {
	d := os.Getenv(devSettingsEnv)
	if d == "" {
		return "", fmt.Errorf("%s not set; run through scripts\\uicheck.ps1, which points burnmon-dev.exe at a temp settings folder", devSettingsEnv)
	}
	return d, nil
}

package store

import (
	"os"
	"path/filepath"
)

// DataDirFrom picks the data folder: localAppData when set, else the per-user
// config dir (the folder the stores use on macOS), else the temp dir, each
// with "burnmon" added. BurnMon Dev and the Control Centre share this rule;
// BurnMon keeps its own Windows-only one.
func DataDirFrom(localAppData string, userConfigDir func() (string, error)) string {
	if localAppData != "" {
		return filepath.Join(localAppData, "burnmon")
	}
	if dir, err := userConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "burnmon")
	}
	return filepath.Join(os.TempDir(), "burnmon")
}

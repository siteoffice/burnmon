package centre

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

// SettingsFile is the Control Centre settings file name.
const SettingsFile = "controlcentre.json"

// Settings is controlcentre.json. Root comes only from the file or --root.
type Settings struct {
	Root  string `json:"root"`
	Width int    `json:"width"`
	Menu  string `json:"menu"`
	Weeks int    `json:"weeks"`
	Theme string `json:"theme"`
}

// DefaultSettings is width 80, menu text, weeks 2, theme system, no root.
func DefaultSettings() Settings {
	return Settings{Width: 80, Menu: "text", Weeks: 2, Theme: "system"}
}

// Normalized replaces every value outside its allowed set by its default.
// Root is kept as is.
func (s Settings) Normalized() Settings {
	d := DefaultSettings()
	if s.Width != 60 && s.Width != 80 && s.Width != 100 {
		s.Width = d.Width
	}
	if s.Menu != "text" && s.Menu != "icons" {
		s.Menu = d.Menu
	}
	if s.Weeks != 2 && s.Weeks != 4 {
		s.Weeks = d.Weeks
	}
	if s.Theme != "system" && s.Theme != "light" && s.Theme != "dark" {
		s.Theme = d.Theme
	}
	return s
}

// LoadSettings reads controlcentre.json from exeDir first ("" skips it),
// then from dataDir, the rule burnmon.json uses. It returns the normalized
// settings and the file read; with no file, the defaults and the dataDir
// path to save to. A file that does not parse gives the defaults.
func LoadSettings(exeDir, dataDir string) (Settings, string) {
	p := filepath.Join(dataDir, SettingsFile)
	if exeDir != "" {
		if cand := filepath.Join(exeDir, SettingsFile); isFile(cand) {
			p = cand
		}
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return DefaultSettings(), p
	}
	s := DefaultSettings()
	if err := json.Unmarshal(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), &s); err != nil {
		return DefaultSettings(), p
	}
	return s.Normalized(), p
}

// SaveSettings writes s, normalized, to path through a temp file and a
// rename, making the folder when needed.
func SaveSettings(path string, s Settings) error {
	body, err := json.MarshalIndent(s.Normalized(), "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(body, '\n'), 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

// AppDataDir picks the data folder by the rule of appDataDirFrom in
// cmd/burnmon-dev: localAppData when set, else the per-user config dir,
// else the temp dir, each with "burnmon" added.
func AppDataDir(localAppData string, userConfigDir func() (string, error)) string {
	if localAppData != "" {
		return filepath.Join(localAppData, "burnmon")
	}
	if dir, err := userConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "burnmon")
	}
	return filepath.Join(os.TempDir(), "burnmon")
}

// ResolveRoot is flagRoot, else settingRoot, else cwd. A relative root is
// taken from cwd. The result is absolute and cleaned.
func ResolveRoot(flagRoot, settingRoot, cwd string) string {
	r := flagRoot
	if r == "" {
		r = settingRoot
	}
	if r == "" {
		r = cwd
	} else if !filepath.IsAbs(r) {
		r = filepath.Join(cwd, r)
	}
	if abs, err := filepath.Abs(r); err == nil {
		return abs
	}
	return filepath.Clean(r)
}

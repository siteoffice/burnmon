package centre

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"burnmon/internal/store"
)

// SettingsFile is the Control Centre settings file name.
const SettingsFile = "controlcentre.json"

// Settings is controlcentre.json. Root is set on the Settings page (Save
// writes it here) or by --root, which wins and is never saved. GitPRs is the
// Git page switch that runs gh; off by default.
type Settings struct {
	Root   string `json:"root"`
	Width  int    `json:"width"`
	Menu   string `json:"menu"`
	Weeks  int    `json:"weeks"`
	Theme  string `json:"theme"`
	GitPRs bool   `json:"git_prs"`
	// Panel is the right panel's preset width: narrow, normal or wide.
	// PanelPx is the width dragged at the panel edge, in px; it wins while
	// set, 0 means the preset (choosing a preset or a double-click on the
	// edge sets it back to 0).
	Panel   string `json:"panel"`
	PanelPx int    `json:"panel_px"`
}

// PanelMinPx and PanelMaxPx bound a dragged right panel width.
const (
	PanelMinPx = 220
	PanelMaxPx = 640
)

// DefaultSettings is width 80, menu text, weeks 2, theme system, panel
// normal, no root.
func DefaultSettings() Settings {
	return Settings{Width: 80, Menu: "text", Weeks: 2, Theme: "system", Panel: "normal"}
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
	if s.Panel != "narrow" && s.Panel != "normal" && s.Panel != "wide" {
		s.Panel = d.Panel
	}
	if s.PanelPx < PanelMinPx || s.PanelPx > PanelMaxPx {
		s.PanelPx = 0
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
	return store.DataDirFrom(localAppData, userConfigDir)
}

// ResolveRoot is flagRoot, else settingRoot, else "" (no root: the Control
// Centre scans nothing and asks for a folder; the current folder is no
// fallback because a double-clicked Mac app starts in "/"). A relative root
// is taken from cwd. A root is absolute and cleaned.
func ResolveRoot(flagRoot, settingRoot, cwd string) string {
	r := flagRoot
	if r == "" {
		r = settingRoot
	}
	if r == "" {
		return ""
	}
	if !filepath.IsAbs(r) {
		r = filepath.Join(cwd, r)
	}
	if abs, err := filepath.Abs(r); err == nil {
		return abs
	}
	return filepath.Clean(r)
}

// RootCheck is what the Settings page shows for a root before Save.
type RootCheck struct {
	Path  string `json:"path"` // absolute and cleaned, "" when blank
	OK    bool   `json:"ok"`
	Units int    `json:"units"`
	Line  string `json:"line"`
}

// CheckRoot checks that p (relative to cwd when not absolute) is an existing
// folder that reads, and counts the units below it.
func CheckRoot(p, cwd string) RootCheck {
	p = strings.TrimSpace(p)
	if p == "" {
		return RootCheck{Line: "Type a folder or pick one with Browse."}
	}
	abs := ResolveRoot(p, "", cwd)
	c := RootCheck{Path: abs}
	info, err := os.Stat(abs)
	switch {
	case err != nil:
		c.Line = "No folder at " + abs + "."
		return c
	case !info.IsDir():
		c.Line = abs + " is a file, not a folder."
		return c
	}
	if _, err := os.ReadDir(abs); err != nil {
		c.Line = "The folder cannot be read: " + err.Error()
		return c
	}
	units, _ := FindUnits(abs)
	c.OK, c.Units = true, len(units)
	switch len(units) {
	case 0:
		c.Line = "No units below this folder: no siteoffice.json in it or 4 levels down."
	case 1:
		c.Line = "1 unit below this folder."
	default:
		c.Line = strconv.Itoa(len(units)) + " units below this folder."
	}
	return c
}

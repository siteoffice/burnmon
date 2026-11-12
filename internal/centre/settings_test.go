package centre

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSettings(t *testing.T) {
	want := Settings{Root: "", Width: 80, Menu: "text", Weeks: 2, Theme: "system"}
	if got := DefaultSettings(); got != want {
		t.Errorf("DefaultSettings = %+v", got)
	}
}

func TestSettingsLoadOrder(t *testing.T) {
	exeDir, dataDir := t.TempDir(), t.TempDir()
	writeFile(t, exeDir, SettingsFile, `{"width":60}`)
	writeFile(t, dataDir, SettingsFile, `{"width":100}`)
	s, p := LoadSettings(exeDir, dataDir)
	if s.Width != 60 || p != filepath.Join(exeDir, SettingsFile) {
		t.Errorf("exe dir: %+v from %q", s, p)
	}

	s, p = LoadSettings(t.TempDir(), dataDir)
	if s.Width != 100 || p != filepath.Join(dataDir, SettingsFile) {
		t.Errorf("data dir: %+v from %q", s, p)
	}
	s, p = LoadSettings("", dataDir)
	if s.Width != 100 || p != filepath.Join(dataDir, SettingsFile) {
		t.Errorf("no exe dir: %+v from %q", s, p)
	}

	empty := t.TempDir()
	s, p = LoadSettings(t.TempDir(), empty)
	if s != DefaultSettings() || p != filepath.Join(empty, SettingsFile) {
		t.Errorf("none: %+v at %q", s, p)
	}
}

func TestSettingsBadValueReplaced(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, SettingsFile, `{"width":70,"menu":"x","weeks":3,"theme":"pink","root":"C:/r"}`)
	s, _ := LoadSettings("", dir)
	want := Settings{Root: "C:/r", Width: 80, Menu: "text", Weeks: 2, Theme: "system"}
	if s != want {
		t.Errorf("got %+v, want %+v", s, want)
	}

	writeFile(t, dir, SettingsFile, `{"width":`)
	if s, _ := LoadSettings("", dir); s != DefaultSettings() {
		t.Errorf("unparsable: %+v", s)
	}
	writeFile(t, dir, SettingsFile, `{"width":"wide","weeks":4}`)
	if s, _ := LoadSettings("", dir); s != DefaultSettings() {
		t.Errorf("wrong type: %+v", s)
	}
	writeFile(t, dir, SettingsFile, "\xef\xbb\xbf"+`{"weeks":4,"theme":"dark","menu":"icons"}`)
	want = Settings{Width: 80, Menu: "icons", Weeks: 4, Theme: "dark"}
	if s, _ := LoadSettings("", dir); s != want {
		t.Errorf("BOM file: %+v, want %+v", s, want)
	}
}

func TestSaveSettingsRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new")
	p := filepath.Join(dir, SettingsFile)
	in := Settings{Root: `C:\work`, Width: 100, Menu: "icons", Weeks: 4, Theme: "light"}
	if err := SaveSettings(p, in); err != nil {
		t.Fatal(err)
	}
	got, from := LoadSettings("", dir)
	if got != in.Normalized() || from != p {
		t.Errorf("round trip %+v from %q, want %+v", got, from, in)
	}
	if err := SaveSettings(p, Settings{Width: 7}); err != nil {
		t.Fatal(err)
	}
	if got, _ := LoadSettings("", dir); got != DefaultSettings() {
		t.Errorf("bad values saved: %+v", got)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("folder holds %d entries, want only %s", len(entries), SettingsFile)
	}
}

func TestAppDataDirRule(t *testing.T) {
	fail := func() (string, error) { return "", errors.New("no config dir") }
	if got, want := AppDataDir(`C:\L`, fail), filepath.Join(`C:\L`, "burnmon"); got != want {
		t.Errorf("local app data: %q, want %q", got, want)
	}
	cfg := func() (string, error) { return "/cfg", nil }
	if got, want := AppDataDir("", cfg), filepath.Join("/cfg", "burnmon"); got != want {
		t.Errorf("config dir: %q, want %q", got, want)
	}
	if got, want := AppDataDir("", fail), filepath.Join(os.TempDir(), "burnmon"); got != want {
		t.Errorf("temp dir: %q, want %q", got, want)
	}
}

func TestResolveRoot(t *testing.T) {
	cwd := t.TempDir()
	flag := t.TempDir()
	set := t.TempDir()
	if got := ResolveRoot(flag+"/x/../", set, cwd); got != filepath.Clean(flag) {
		t.Errorf("flag: %q", got)
	}
	if got := ResolveRoot("", set, cwd); got != set {
		t.Errorf("setting: %q", got)
	}
	if got := ResolveRoot("", "", cwd); got != cwd {
		t.Errorf("cwd: %q", got)
	}
	if got := ResolveRoot("", "rel/./sub", cwd); got != filepath.Join(cwd, "rel", "sub") {
		t.Errorf("relative setting: %q", got)
	}
	if got := ResolveRoot("", "", "."); !filepath.IsAbs(got) {
		t.Errorf("relative cwd gave %q, want absolute", got)
	}
}

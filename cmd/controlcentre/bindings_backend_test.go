//go:build windows || darwin

package main

import (
	"reflect"
	"strings"
	"testing"

	"burnmon/internal/centre"
	"burnmon/internal/winbind"
)

func TestBindAllRegistersSharedList(t *testing.T) {
	rec := &winbind.Recorder{}
	bindAll(bindEnv{
		w: rec, root: t.TempDir(), dataDir: t.TempDir(),
		settings: newSettingsBox(centre.DefaultSettings()),
	})
	if !reflect.DeepEqual(rec.Names, ccBindingNames) {
		t.Fatalf("bound names differ from ccBindingNames\n got: %v\nwant: %v", rec.Names, ccBindingNames)
	}
}

func TestAssemblePageTheme(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		s := centre.DefaultSettings()
		s.Theme = theme
		want := `<html lang="en" data-theme="` + theme + `">`
		if got := assemblePage(s); !strings.Contains(got, want) {
			t.Errorf("theme %s: page lacks %s", theme, want)
		}
	}
	got := assemblePage(centre.DefaultSettings())
	if !strings.Contains(got, `<html lang="en">`) || strings.Contains(got, `<html lang="en" data-theme=`) {
		t.Error("theme system must leave <html lang=\"en\"> as is")
	}
	if got != pageHTML {
		t.Error("theme system must return the page unchanged")
	}
}

func TestSettingsSetKeepsRoot(t *testing.T) {
	dir := t.TempDir()
	cur := centre.DefaultSettings()
	cur.Root = "kept-root"
	e := bindEnv{settingsPath: dir + "/" + centre.SettingsFile, settings: newSettingsBox(cur)}
	saved, err := settingsSet(e, centre.Settings{Root: "other", Width: 60, Menu: "bogus", Weeks: 4, Theme: "dark"})
	if err != nil {
		t.Fatal(err)
	}
	want := centre.Settings{Root: "kept-root", Width: 60, Menu: "text", Weeks: 4, Theme: "dark"}
	if saved != want {
		t.Fatalf("saved %+v, want %+v", saved, want)
	}
	if got := e.settings.get(); got != want {
		t.Fatalf("box holds %+v, want %+v", got, want)
	}
	loaded, _ := centre.LoadSettings("", dir)
	if loaded != want {
		t.Fatalf("file holds %+v, want %+v", loaded, want)
	}
}

// TestAgentsWithoutSnapshotPath: with no snapshot path (Task 12 replaced the
// Task 6 stub) the view says BurnMon Dev is not running and lists nothing.
func TestAgentsWithoutSnapshotPath(t *testing.T) {
	e := bindEnv{}
	v := agentsView(e, 12)
	if v.Available || v.Line != "BurnMon Dev is not running, so there is no agent list" || v.Agents == nil || v.Pages == nil || len(v.Agents) != 0 || len(v.Pages) != 0 {
		t.Errorf("agentsView = %+v", v)
	}
}

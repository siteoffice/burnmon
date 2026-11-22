//go:build windows || darwin

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burnmon/internal/centre"
)

// noRootEnv is a bindEnv whose root is empty (no --root, no saved root), with a
// siteoffice.json in the current folder that must never be read.
func noRootEnv(t *testing.T) bindEnv {
	t.Helper()
	cwd := t.TempDir()
	writeTo(t, cwd, "siteoffice.json", `{"name":"here"}`)
	t.Chdir(cwd)
	return bindEnv{roots: newRootBox("", false), settingsPath: filepath.Join(t.TempDir(), centre.SettingsFile),
		settings: newSettingsBox(centre.DefaultSettings()), now: func() time.Time { return rigT0 },
		git: centre.NewGitScanner(centre.GitDeps{})}
}

// TestNoRootBindingsReadNothing: with no root every binding that reads below the root
// answers with a refusal or an empty view, never a read of the current folder.
func TestNoRootBindingsReadNothing(t *testing.T) {
	e := noRootEnv(t)
	if st := centre.Build(e.rootNow(), centre.Options{Now: rigT0}); st.Root != "" || len(st.Units) != 0 {
		t.Errorf("state: root %q, %d units", st.Root, len(st.Units))
	}
	if _, err := filePage(e, "siteoffice.json"); err == nil || !strings.Contains(err.Error(), "no folder") {
		t.Errorf("filePage err = %v", err)
	}
	if _, err := boardPage(e, "siteoffice.json"); err == nil || !strings.Contains(err.Error(), "no folder") {
		t.Errorf("boardPage err = %v", err)
	}
	req := centre.CommentRequest{Target: centre.Target{Kind: "unit"}, Text: "x"}
	if _, err := commentWrite(e, req); err == nil || !strings.Contains(err.Error(), "no folder") {
		t.Errorf("commentWrite err = %v", err)
	}
	if _, err := os.Stat("siteoffice.json"); err != nil {
		t.Fatal(err)
	}
	gv := gitView(e, GitRequest{Rows: 10})
	if gv.Root != "" || len(gv.Repos) != 0 || !strings.Contains(gv.Line, "no folder") {
		t.Errorf("gitView = %+v", gv)
	}
	if d := gitRepo(e, "."); d != (centre.GitDetailView{}) {
		t.Errorf("gitRepo = %+v", d)
	}
}

// TestNoRootSaveThenPagesWork: the Root row starts empty and Save points every page at the folder.
func TestNoRootSaveThenPagesWork(t *testing.T) {
	e := noRootEnv(t)
	if v := rootView(e); v.Root != "" || v.Check.OK || v.Check.Line == "" {
		t.Fatalf("view with no root: %+v", v)
	}
	b := t.TempDir()
	writeTo(t, b, "siteoffice.json", `{"name":"b"}`)
	if _, err := rootSet(e, b); err != nil {
		t.Fatal(err)
	}
	if st := centre.Build(e.rootNow(), centre.Options{Now: rigT0}); st.Root != b || st.Counts.Units != 1 {
		t.Errorf("state after Save: root %q, %d units", st.Root, st.Counts.Units)
	}
	if _, err := filePage(e, "siteoffice.json"); err != nil {
		t.Errorf("filePage after Save: %v", err)
	}
}

// TestPageNoRoot: the page asks for a folder, on Settings first, and every other page says so.
func TestPageNoRoot(t *testing.T) {
	body := pageFunc(t, "pageNoRoot")
	for _, want := range []string{"No folder is chosen yet.", "Open Settings", "go('settings')", "'muted'"} {
		if !strings.Contains(body, want) {
			t.Errorf("pageNoRoot lacks %q", want)
		}
	}
	render := pageFunc(t, "render")
	if !strings.Contains(render, "state.root === ''") || !strings.Contains(render, "pageNoRoot") {
		t.Error("render does not send a state with no root to pageNoRoot")
	}
	if !strings.Contains(pageFunc(t, "loadState"), "state.root === ''") || !strings.Contains(pageFunc(t, "loadState"), "'settings'") {
		t.Error("loadState does not open Settings first when no folder is chosen")
	}
	if !strings.Contains(pageFunc(t, "rootName"), "state.root === ''") {
		t.Error("rootName gives an empty crumb when no folder is chosen")
	}
	if !strings.Contains(pageFunc(t, "pageSettings"), "state.root === ''") {
		t.Error("pageSettings does not ask for the folder when none is chosen")
	}
}

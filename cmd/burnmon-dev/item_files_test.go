//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burnmon/internal/snapshot"
	"burnmon/internal/waits"
)

var itemNow = time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)

type itemEnv struct {
	snap, spec, report, dir, exe, missing, other string
}

// newItemEnv writes real temp files and a snapshot whose items name them.
// extra items are appended to the wait list.
func newItemEnv(t *testing.T, writtenAt time.Time, extra ...waits.Item) itemEnv {
	t.Helper()
	d := t.TempDir()
	e := itemEnv{
		snap:    filepath.Join(d, "snapshot.json"),
		spec:    filepath.Join(d, "spec.md"),
		report:  filepath.Join(d, "report.md"),
		dir:     filepath.Join(d, "dir.md"),
		exe:     filepath.Join(d, "tool.exe"),
		missing: filepath.Join(d, "gone.md"),
		other:   filepath.Join(d, "other.md"),
	}
	for _, p := range []string{e.spec, e.report, e.exe, e.other} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(e.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	wait := []waits.Item{
		{Kind: waits.KindGate, ID: "gate:a:G1", Title: "a", Since: "2026-10-23T09:00:00Z", Source: "foreman", SpecPath: e.spec},
		{Kind: waits.KindSprint, ID: "sprint:b", Title: "b", Since: "2026-10-23T09:00:00Z", Source: "foreman", SpecPath: e.dir, ResultPath: e.missing},
	}
	wait = append(wait, extra...)
	out := []waits.Item{
		{Kind: waits.KindFinished, ID: "sprint:c", Title: "c", Since: "2026-10-23T09:00:00Z", Source: "foreman", ResultPath: e.report},
		{Kind: waits.KindFinished, ID: "sprint:d", Title: "d", Since: "2026-10-23T09:00:00Z", Source: "foreman", ResultPath: e.exe},
	}
	s := snapshot.Build(snapshot.Input{Now: writtenAt, TickSeconds: 5, WaitItems: wait, OutcomeItems: out})
	if err := snapshot.Write(e.snap, s); err != nil {
		t.Fatal(err)
	}
	return e
}

func stubOpener(t *testing.T) *[]string {
	t.Helper()
	var calls []string
	old := openItemFile
	openItemFile = func(p string) error { calls = append(calls, p); return nil }
	t.Cleanup(func() { openItemFile = old })
	return &calls
}

func TestItemFileAllowedOpensListedFiles(t *testing.T) {
	e := newItemEnv(t, itemNow)
	for _, p := range []string{e.spec, e.report, strings.ToUpper(e.spec), strings.Replace(e.spec, "spec.md", "spec.MD", 1)} {
		if err := itemFileAllowed(e.snap, itemNow, p); err != nil {
			t.Errorf("%q should be allowed: %v", p, err)
		}
	}
}

func TestItemFileAllowedRefuses(t *testing.T) {
	// A relative path listed in an item, a UNC path, a device path and a
	// stream, all written into the snapshot on purpose.
	extra := []waits.Item{
		{Kind: waits.KindGate, ID: "g:rel", Title: "r", Since: "2026-10-23T09:00:00Z", Source: "foreman", SpecPath: `spec.md`},
		{Kind: waits.KindGate, ID: "g:unc", Title: "u", Since: "2026-10-23T09:00:00Z", Source: "foreman", SpecPath: `\\example-host\share\x.md`},
		{Kind: waits.KindGate, ID: "g:dev", Title: "v", Since: "2026-10-23T09:00:00Z", Source: "foreman", SpecPath: `\\.\C:\x.md`},
		{Kind: waits.KindGate, ID: "g:nul", Title: "n", Since: "2026-10-23T09:00:00Z", Source: "foreman", SpecPath: `NUL.md`},
	}
	e := newItemEnv(t, itemNow, extra...)
	calls := stubOpener(t)

	// Every listed bad extension, written as real files and listed.
	var extPaths []string
	var extItems []waits.Item
	dd := filepath.Dir(e.spec)
	for _, n := range []string{"a.exe", "a.bat", "a.cmd", "a.ps1", "a.lnk", "a.msi", "a.js", "a.vbs", "a.com", "a.scr", "a.hta", "a.reg", "a.dll", "noext"} {
		p := filepath.Join(dd, n)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		extPaths = append(extPaths, p)
		extItems = append(extItems, waits.Item{Kind: waits.KindGate, ID: "g:" + n, Title: n, Since: "2026-10-23T09:00:00Z", Source: "foreman", SpecPath: p})
	}
	e2 := newItemEnv(t, itemNow, append(extra, extItems...)...)
	_ = e2

	cases := []struct {
		name, snap, path, want string
	}{
		{"not listed", e.snap, e.other, "path is not in the snapshot"},
		{"empty", e.snap, "", "path is not in the snapshot"},
		{"relative listed", e.snap, `spec.md`, "path is not absolute"},
		{"unc listed", e.snap, `\\example-host\share\x.md`, "path is not absolute"},
		{"device listed", e.snap, `\\.\C:\x.md`, "path is not absolute"},
		{"reserved name listed", e.snap, `NUL.md`, "path is not absolute"},
		{"folder", e.snap, e.dir, "not a file"},
		{"exe", e.snap, e.exe, "file type not allowed"},
		{"missing", e.snap, e.missing, "not a file"},
		{"dot dot to listed", e.snap, filepath.Join(filepath.Dir(e.spec), "dir.md", "..", "other.md"), "path is not in the snapshot"},
	}
	for _, p := range extPaths {
		// listed in e2's snapshot, not e's
		cases = append(cases, struct{ name, snap, path, want string }{"ext " + filepath.Base(p), e2.snap, p, "file type not allowed"})
	}
	for _, c := range cases {
		err := itemFileAllowed(c.snap, itemNow, c.path)
		if err == nil || err.Error() != c.want {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
		if err := openItemFileChecked(c.snap, itemNow, c.path); err == nil {
			t.Errorf("%s: openItemFileChecked returned nil", c.name)
		}
	}

	// Stale snapshot: the listed spec is refused.
	st := newItemEnv(t, itemNow.Add(-20*time.Second))
	if err := itemFileAllowed(st.snap, itemNow, st.spec); err == nil || err.Error() != "snapshot is not current" {
		t.Errorf("stale: err = %v", err)
	}
	// Missing snapshot file.
	if err := itemFileAllowed(filepath.Join(t.TempDir(), "none.json"), itemNow, e.spec); err == nil || err.Error() != "snapshot is not current" {
		t.Errorf("missing snapshot: err = %v", err)
	}
	if err := openItemFileChecked(st.snap, itemNow, st.spec); err == nil {
		t.Error("stale snapshot opened")
	}
	if len(*calls) != 0 {
		t.Errorf("opener was called for refused paths: %v", *calls)
	}
}

func TestBindingOpensOnlyAllowed(t *testing.T) {
	e := newItemEnv(t, itemNow)
	calls := stubOpener(t)
	if err := openItemFileChecked(e.snap, itemNow, e.other); err == nil {
		t.Error("unlisted path accepted")
	}
	if len(*calls) != 0 {
		t.Fatalf("opener called for refused path: %v", *calls)
	}
	if err := openItemFileChecked(e.snap, itemNow, strings.ToUpper(e.spec)); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || !strings.EqualFold(filepath.Clean((*calls)[0]), filepath.Clean(e.spec)) {
		t.Fatalf("calls = %v", *calls)
	}
}

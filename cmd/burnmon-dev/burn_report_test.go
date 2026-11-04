//go:build windows

package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/store"
	"burnmon/internal/sysmon"
)

const (
	brtSalt    = "salt-of15"
	brtProject = `C:\clients\Acme\billing`
	brtOwner   = "ownerzed"
	brtClient  = "ClientQ"
	brtTitle   = "secret prompt text"
)

func brtNow() time.Time { return time.Date(2026, 11, 3, 15, 0, 0, 0, time.Local) }

func brtFixture(t *testing.T) (*store.Store, *sysmon.Store, *pricing.Config) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	sys, err := sysmon.Open(filepath.Join(dir, "sys.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sys.Close() })
	var evs []schema.Event
	for i := 0; i < 3; i++ {
		at := time.Date(2026, 11, 3-i, 12, 0, 0, 0, time.Local).UTC()
		evs = append(evs, schema.Event{
			Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
			SessionID: "3f2b8c1e-9d4a-4e7b-8a61-5c0d2e9f7b44", RequestID: "r" + string(rune('a'+i)),
			At: at, Model: "claude-sonnet-5", Project: brtProject, Owner: brtOwner,
			Client: brtClient, Title: brtTitle, Input: 1000, Output: 100,
		})
	}
	if err := st.UpsertEvents(evs); err != nil {
		t.Fatal(err)
	}
	cfg := pricing.Defaults()
	return st, sys, &cfg
}

func brtEntries(t *testing.T, path string) map[string]string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(b)
	}
	return out
}

func TestWriteBurnReport_Redacted(t *testing.T) {
	st, sys, cfg := brtFixture(t)
	dir := t.TempDir()
	out := writeBurnReport(st, sys, cfg, burnReportRequest{Dir: dir}, brtNow(), brtSalt)
	if out.Error != "" {
		t.Fatalf("error: %s", out.Error)
	}
	if !out.Redacted {
		t.Fatalf("Redacted = false, want true")
	}
	want := filepath.Join(dir, "burn-report-2026-11-03.zip")
	if out.Path != want {
		t.Fatalf("Path = %q, want %q", out.Path, want)
	}
	if out.DaysWithData != 3 {
		t.Fatalf("DaysWithData = %d, want 3", out.DaysWithData)
	}
	entries := brtEntries(t, want)
	if len(entries) == 0 {
		t.Fatal("zip has no entries")
	}
	escaped := strings.ReplaceAll(brtProject, `\`, `\\`)
	for name, body := range entries {
		for _, s := range []string{brtProject, escaped, "Acme", "billing", "clients", brtOwner, brtClient, brtTitle, "secret"} {
			if strings.Contains(body, s) {
				t.Errorf("%s contains %q", name, s)
			}
		}
	}
}

func TestWriteBurnReport_ShowNames(t *testing.T) {
	st, sys, cfg := brtFixture(t)
	out := writeBurnReport(st, sys, cfg, burnReportRequest{Dir: t.TempDir(), ShowNames: true}, brtNow(), brtSalt)
	if out.Error != "" {
		t.Fatalf("error: %s", out.Error)
	}
	if out.Redacted {
		t.Fatalf("Redacted = true, want false")
	}
	all := ""
	for _, body := range brtEntries(t, out.Path) {
		all += body
	}
	for _, s := range []string{brtOwner, brtClient} {
		if !strings.Contains(all, s) {
			t.Errorf("names shown but %q absent", s)
		}
	}
	if !strings.Contains(all, "Acme") {
		t.Errorf("names shown but project absent")
	}
}

func TestWriteBurnReport_RelativeOrEmptyDirIsRefused(t *testing.T) {
	st, sys, cfg := brtFixture(t)
	for _, d := range []string{"", "reports"} {
		out := writeBurnReport(st, sys, cfg, burnReportRequest{Dir: d}, brtNow(), brtSalt)
		if !strings.Contains(out.Error, "absolute path") {
			t.Errorf("Dir %q: Error = %q, want mention of absolute path", d, out.Error)
		}
		if out.Path != "" {
			t.Errorf("Dir %q: Path = %q, want none", d, out.Path)
		}
	}
	if _, err := os.Stat("reports"); err == nil {
		t.Errorf("relative dir was created")
	}
}

func TestWriteBurnReport_SecondPressOverwrites(t *testing.T) {
	st, sys, cfg := brtFixture(t)
	dir := t.TempDir()
	for i := 0; i < 2; i++ {
		out := writeBurnReport(st, sys, cfg, burnReportRequest{Dir: dir}, brtNow(), brtSalt)
		if out.Error != "" {
			t.Fatalf("press %d: %s", i+1, out.Error)
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*"))
	if len(files) != 1 {
		t.Fatalf("want one file, got %v", files)
	}
}

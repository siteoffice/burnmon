//go:build windows

package main

import (
	"errors"
	"os"
	"strings"
	"testing"

	"burnmon/internal/export"
	"burnmon/internal/export/exporttest"
	"burnmon/internal/history"
	"burnmon/internal/historyadmin"
	"burnmon/internal/pricing"
)

func TestDevExportEqualsCLIExport(t *testing.T) {
	st := exporttest.Store(t)
	a := &app{cfg: pricing.Defaults()}
	res, err := a.exportHistory(st, historyadmin.ExportRequest{Dir: t.TempDir(), Owners: []string{"wilco"}}, exporttest.FixedNow)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	want := exporttest.Golden(t, st, &a.cfg, export.Filter{Owners: []string{"wilco"}}, "dev-1")
	if string(got) != string(want) {
		t.Fatalf("dev export differs from the CLI golden:\n got %s\nwant %s", got, want)
	}
}

func TestDevExportRefusesRelativeFolder(t *testing.T) {
	st := exporttest.Store(t)
	a := &app{cfg: pricing.Defaults()}
	for _, dir := range []string{"", ".", "exports", `..\exports`} {
		_, err := a.exportHistory(st, historyadmin.ExportRequest{Dir: dir}, exporttest.FixedNow)
		if err == nil || !strings.Contains(err.Error(), "absolute") {
			t.Fatalf("dir %q: want an error naming an absolute folder, got %v", dir, err)
		}
	}
}

func TestDevDeleteClearsHeatmapCache(t *testing.T) {
	st := exporttest.Store(t)
	a := &app{cfg: pricing.Defaults()}
	a.heatmapAsOf = "2026-10-25"
	a.heatmapClosed = []history.Row{{}}
	res, err := a.deleteHistory(st, historyadmin.DeleteRequest{Scope: historyadmin.ScopeAll, Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Events != 4 {
		t.Fatalf("deleted %d events, want 4", res.Events)
	}
	a.heatmapMu.Lock()
	defer a.heatmapMu.Unlock()
	if a.heatmapClosed != nil || a.heatmapAsOf != "" {
		t.Fatalf("heatmap cache not cleared: %v %q", a.heatmapClosed, a.heatmapAsOf)
	}
}

func TestDevDeleteNeedsConfirm(t *testing.T) {
	st := exporttest.Store(t)
	a := &app{cfg: pricing.Defaults()}
	a.heatmapAsOf = "2026-10-25"
	a.heatmapClosed = []history.Row{{}}
	_, err := a.deleteHistory(st, historyadmin.DeleteRequest{Scope: historyadmin.ScopeAll})
	if !errors.Is(err, historyadmin.ErrNotConfirmed) {
		t.Fatalf("err = %v, want ErrNotConfirmed", err)
	}
	if evs, _ := st.AllEvents(); len(evs) != 4 {
		t.Fatalf("store changed without confirm: %d events", len(evs))
	}
	if a.heatmapAsOf == "" || a.heatmapClosed == nil {
		t.Fatal("heatmap cache cleared although nothing was deleted")
	}
}

func TestPageHasHistoryBlockOutsideSectionColumns(t *testing.T) {
	if !strings.Contains(pageHTML, `id="histAdmin"`) {
		t.Fatal(`page lacks id="histAdmin"`)
	}
	start := strings.Index(pageHTML, `id="secSettingsCols"`)
	if start < 0 {
		t.Fatal("page lacks secSettingsCols")
	}
	end := strings.Index(pageHTML[start:], "</div>")
	if end < 0 {
		t.Fatal("secSettingsCols is not closed")
	}
	if strings.Contains(pageHTML[start:start+end], "histAdmin") {
		t.Fatal("histAdmin sits inside secSettingsCols")
	}
	for _, want := range []string{"hist_delete_older", "hist_delete_session", "hist_delete_all", "hist_export", "Number.isInteger", "bdevDeleteHistory", "bdevExportHistory"} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page lacks %s", want)
		}
	}
}

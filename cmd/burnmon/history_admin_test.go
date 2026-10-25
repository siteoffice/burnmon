package main

import (
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"burnmon/internal/export"
	"burnmon/internal/export/exporttest"
	"burnmon/internal/historyadmin"
	"burnmon/internal/pricing"
)

// noRebuildAfterDelete keeps a test from scanning the machine's real
// transcript folders: it swaps the post-delete rebuild for a counter.
func noRebuildAfterDelete(t *testing.T) *atomic.Int32 {
	t.Helper()
	var n atomic.Int32
	old := rebuildAfterDelete
	rebuildAfterDelete = func(*app) { n.Add(1) }
	t.Cleanup(func() { rebuildAfterDelete = old })
	return &n
}

func TestAppExportEqualsCLIExport(t *testing.T) {
	st := exporttest.Store(t)
	a := &app{cfg: pricing.Defaults()}
	dir := t.TempDir()
	res, err := a.exportHistory(st, historyadmin.ExportRequest{Dir: dir, Owners: []string{"wilco"}}, exporttest.FixedNow)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	want := exporttest.Golden(t, st, &a.cfg, export.Filter{Owners: []string{"wilco"}}, "dev-1")
	if string(got) != string(want) {
		t.Fatalf("app export differs from the CLI golden:\n got %s\nwant %s", got, want)
	}
}

func TestAppExportRefusesRelativeFolder(t *testing.T) {
	st := exporttest.Store(t)
	a := &app{cfg: pricing.Defaults()}
	for _, dir := range []string{"", ".", "exports", `..\exports`} {
		_, err := a.exportHistory(st, historyadmin.ExportRequest{Dir: dir}, exporttest.FixedNow)
		if err == nil || !strings.Contains(err.Error(), "absolute") {
			t.Fatalf("dir %q: want an error naming an absolute folder, got %v", dir, err)
		}
	}
}

func TestAppDeleteHistoryNeedsConfirm(t *testing.T) {
	rebuilds := noRebuildAfterDelete(t)
	st := exporttest.Store(t)
	a := &app{cfg: pricing.Defaults()}
	_, err := a.deleteHistory(st, historyadmin.DeleteRequest{Scope: historyadmin.ScopeAll})
	if !errors.Is(err, historyadmin.ErrNotConfirmed) {
		t.Fatalf("err = %v, want ErrNotConfirmed", err)
	}
	if evs, _ := st.AllEvents(); len(evs) != 4 {
		t.Fatalf("store changed without confirm: %d events", len(evs))
	}
	if rebuilds.Load() != 0 {
		t.Fatal("rebuild started although nothing was deleted")
	}
}

func TestAppDeleteHistoryAll(t *testing.T) {
	rebuilds := noRebuildAfterDelete(t)
	st := exporttest.Store(t)
	a := &app{cfg: pricing.Defaults()}
	res, err := a.deleteHistory(st, historyadmin.DeleteRequest{Scope: historyadmin.ScopeAll, Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Events != 4 {
		t.Fatalf("deleted %d events, want 4", res.Events)
	}
	if evs, _ := st.AllEvents(); len(evs) != 0 {
		t.Fatalf("store holds %d events after delete all", len(evs))
	}
	// the rebuild runs in a goroutine; wait briefly for it
	for i := 0; i < 200 && rebuilds.Load() == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if rebuilds.Load() != 1 {
		t.Fatalf("rebuild count = %d, want 1", rebuilds.Load())
	}
}

func TestSettingsModalHasHistoryBlock(t *testing.T) {
	a := &app{cfg: pricing.Defaults()}
	html := a.settingsModalHTML()
	for _, want := range []string{
		`id="cc_h_delete_older"`, `id="cc_h_delete_session"`, `id="cc_h_delete_all"`,
		`id="cc_h_export"`, "keep_history",
		`'Confirm: ' + cur.label`, `'delete older than ' + days + ' days'`,
		`'delete session ' + id`, `'delete all history'`, `Number.isInteger(days)`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("settings modal lacks %s", want)
		}
	}
}

func TestSettingsModalOwnerCheckboxesDeduplicated(t *testing.T) {
	cfg := pricing.Defaults()
	cfg.Owners = []pricing.OwnerRule{{Match: `C:\a\*`, Owner: "wilco"}, {Match: `C:\b\*`, Owner: "wilco"}, {Match: `C:\c\*`, Owner: "other"}}
	a := &app{cfg: cfg}
	if n := strings.Count(a.settingsModalHTML(), `class="cc_h_owner"`); n != 2 {
		t.Fatalf("owner checkboxes = %d, want 2", n)
	}
}

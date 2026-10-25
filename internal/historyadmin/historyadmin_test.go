package historyadmin

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/export"
	"burnmon/internal/export/exporttest"
	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/store"
)

var now = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func event(session, req string, at time.Time) schema.Event {
	return schema.Event{Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
		SessionID: session, RequestID: req, At: at, Model: "claude-sonnet-5", Input: 10, Output: 1}
}

func count(t *testing.T, st *store.Store) int {
	t.Helper()
	events, err := st.AllEvents()
	if err != nil {
		t.Fatal(err)
	}
	return len(events)
}

func seed(t *testing.T, st *store.Store, events ...schema.Event) {
	t.Helper()
	if err := st.UpsertEvents(events); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteRefusesWithoutConfirm(t *testing.T) {
	st := openStore(t)
	seed(t, st, event("s1", "r1", now.Add(-24*time.Hour)))
	for _, scope := range []string{ScopeAll, ScopeSession, ScopeOlderThan} {
		_, err := Delete(st, DeleteRequest{Scope: scope, Days: 1, SessionID: "s1"}, now)
		if !errors.Is(err, ErrNotConfirmed) {
			t.Fatalf("scope %s: err = %v, want ErrNotConfirmed", scope, err)
		}
	}
	if n := count(t, st); n != 1 {
		t.Fatalf("events = %d, want 1 (nothing deleted)", n)
	}
}

func TestDeleteOlderThanDays(t *testing.T) {
	st := openStore(t)
	seed(t, st,
		event("s1", "r1", now.Add(-1*24*time.Hour)),
		event("s1", "r2", now.Add(-5*24*time.Hour)),
		event("s1", "r3", now.Add(-10*24*time.Hour)),
	)
	res, err := Delete(st, DeleteRequest{Scope: ScopeOlderThan, Days: 7, Confirm: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Events != 1 || count(t, st) != 2 {
		t.Fatalf("deleted %d, left %d; want 1 and 2", res.Events, count(t, st))
	}
	if _, err := Delete(st, DeleteRequest{Scope: ScopeOlderThan, Days: 0, Confirm: true}, now); err == nil {
		t.Fatal("Days 0 must be an error")
	}
	if n := count(t, st); n != 2 {
		t.Fatalf("events = %d after refused delete, want 2", n)
	}
}

func TestDeleteOneSession(t *testing.T) {
	st := openStore(t)
	seed(t, st, event("s1", "r1", now), event("s2", "r2", now))
	res, err := Delete(st, DeleteRequest{Scope: ScopeSession, SessionID: "s1", Confirm: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Events != 1 || count(t, st) != 1 {
		t.Fatalf("deleted %d, left %d; want 1 and 1", res.Events, count(t, st))
	}
	if _, err := Delete(st, DeleteRequest{Scope: ScopeSession, Confirm: true}, now); err == nil {
		t.Fatal("empty session id must be an error")
	}
	if n := count(t, st); n != 1 {
		t.Fatalf("events = %d after refused delete, want 1", n)
	}
}

func TestDeleteAll(t *testing.T) {
	st := openStore(t)
	seed(t, st, event("s1", "r1", now), event("s2", "r2", now))
	res, err := Delete(st, DeleteRequest{Scope: ScopeAll, Confirm: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Events != 2 || count(t, st) != 0 {
		t.Fatalf("deleted %d, left %d; want 2 and 0", res.Events, count(t, st))
	}
}

func TestDeleteUnknownScope(t *testing.T) {
	st := openStore(t)
	seed(t, st, event("s1", "r1", now))
	if _, err := Delete(st, DeleteRequest{Scope: "bogus", Confirm: true}, now); err == nil {
		t.Fatal("unknown scope must be an error")
	}
	if n := count(t, st); n != 1 {
		t.Fatalf("events = %d, want 1", n)
	}
}

func testConfig(t *testing.T) pricing.Config {
	t.Helper()
	cfg, err := pricing.Load("")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestExportEqualsCLIGolden(t *testing.T) {
	st := exporttest.Store(t)
	cfg := testConfig(t)
	dir := t.TempDir()
	res, err := Export(st, &cfg, ExportRequest{Dir: dir, Owners: []string{"wilco"}}, exporttest.FixedNow)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	want := exporttest.Golden(t, st, &cfg, export.Filter{Owners: []string{"wilco"}}, "dev-1")
	if string(got) != string(want) {
		t.Fatalf("export bytes differ from golden:\n%s\nwant:\n%s", got, want)
	}
	events, _ := st.AllEvents()
	doc := export.Build(export.Select(events, export.Filter{Owners: []string{"wilco"}}), &cfg, "dev-1", exporttest.FixedNow)
	if res.Rows != len(doc.Rows) || res.Rows == 0 {
		t.Fatalf("Rows = %d, want %d (non-zero)", res.Rows, len(doc.Rows))
	}
}

func TestExportRefusesWithoutOwnerWhenRulesExist(t *testing.T) {
	st := exporttest.Store(t)
	cfg := testConfig(t)
	cfg.Owners = []pricing.OwnerRule{{Match: `C:\dev\Work\*`, Owner: "work"}}
	dir := t.TempDir()
	_, err := Export(st, &cfg, ExportRequest{Dir: dir}, exporttest.FixedNow)
	if !errors.Is(err, export.ErrOwnerRequired) {
		t.Fatalf("err = %v, want ErrOwnerRequired", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("%d files written, want none", len(entries))
	}
}

func TestExportRefusesMissingDir(t *testing.T) {
	st := exporttest.Store(t)
	cfg := testConfig(t)
	missing := filepath.Join(t.TempDir(), "nope")
	if _, err := Export(st, &cfg, ExportRequest{Dir: missing}, exporttest.FixedNow); err == nil {
		t.Fatal("missing folder must be an error")
	}
}

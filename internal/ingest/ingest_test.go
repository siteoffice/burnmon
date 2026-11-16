package ingest

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func testCfg() pricing.Config {
	cfg := pricing.Defaults()
	cfg.Owners = []pricing.OwnerRule{{Match: `C:\work\*`, Owner: "acme", Client: "AcmeCo"}}
	return cfg
}

func testEvent() schema.Event {
	return schema.Event{
		Vendor: "nous", Agent: "hermes", Surface: "cli",
		SessionID: "s1", RequestID: "r1", At: time.Now().UTC(),
		Project: `C:\work\proj`, Output: 5,
	}
}

func stored(t *testing.T, st *store.Store) []schema.Event {
	t.Helper()
	got, err := st.EventsForSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestApplyPolled_OwnerOnly(t *testing.T) {
	st := testStore(t)
	applyPolled("hermes poll", []schema.Event{testEvent()}, nil, st, testCfg, false)
	got := stored(t, st)
	if len(got) != 1 || got[0].Owner != "acme" {
		t.Fatalf("want one event owned by acme, got %+v", got)
	}
	if got[0].Client == "AcmeCo" {
		t.Fatalf("the Hermes and Copilot CLI polls do not set Client, got %q", got[0].Client)
	}
}

func TestApplyPolled_OwnerAndClient(t *testing.T) {
	st := testStore(t)
	applyPolled("copilot vscode poll", []schema.Event{testEvent()}, nil, st, testCfg, true)
	got := stored(t, st)
	if len(got) != 1 || got[0].Owner != "acme" || got[0].Client != "AcmeCo" {
		t.Fatalf("want owner acme and client AcmeCo, got %+v", got)
	}
}

func TestApplyPolled_ErrorStoresNothing(t *testing.T) {
	st := testStore(t)
	applyPolled("hermes poll", []schema.Event{testEvent()}, errors.New("boom"), st, testCfg, false)
	if got := stored(t, st); len(got) != 0 {
		t.Fatalf("a poll error must store nothing, got %+v", got)
	}
}

func TestApplyPolled_EmptyDoesNotReadConfig(t *testing.T) {
	st := testStore(t)
	called := false
	applyPolled("hermes poll", nil, nil, st, func() pricing.Config { called = true; return pricing.Config{} }, false)
	if called {
		t.Fatal("an empty poll must not take the config copy (the apps' lock)")
	}
}

func TestStartPollsNoopWithoutSource(t *testing.T) {
	// No copilot_vscode_otel_file: StartCopilotVSCPoll must return without a goroutine.
	StartCopilotVSCPoll(nil, func() pricing.Config { return pricing.Config{} })
}

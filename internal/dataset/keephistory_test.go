package dataset

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/store"
)

// keepHistoryEnv is a store plus a source folder holding the basic fixture
// as two transcripts, sess-a and sess-b.
type keepHistoryEnv struct {
	st     *store.Store
	c      Cache
	srcDir string
	fix    []byte
}

func newKeepHistoryEnv(t *testing.T) *keepHistoryEnv {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srcDir := filepath.Join(dir, "projects")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fix, err := os.ReadFile(filepath.Join("..", "adapter", "claude", "testdata", "basic.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	env := &keepHistoryEnv{st: st, c: Cache{Store: st}, srcDir: srcDir, fix: fix}
	env.write(t, "sess-a")
	env.write(t, "sess-b")
	return env
}

func (e *keepHistoryEnv) path(id string) string { return filepath.Join(e.srcDir, id+".jsonl") }

func (e *keepHistoryEnv) write(t *testing.T, id string) {
	t.Helper()
	// sess-b is the fixture a day later: two byte-identical transcripts would
	// collapse into one session in dedupSessions, hiding what these tests count.
	body := e.fix
	if id == "sess-b" {
		body = bytes.ReplaceAll(body, []byte("2026-09-10"), []byte("2026-09-11"))
	}
	if err := os.WriteFile(e.path(id), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func (e *keepHistoryEnv) collect(t *testing.T, cfg *pricing.Config) (Payload, error) {
	t.Helper()
	return e.c.Collect(cfg, CollectOpts{Seat: "Standard", MonthsN: 24, Sources: []string{e.srcDir}, RefreshSlow: true}, nil)
}

func sumEvents(evs []schema.Event) (rows int, tokens int64) {
	for _, ev := range evs {
		tokens += ev.Input + ev.Output
		for _, p := range []*int64{ev.CacheWrite, ev.CacheRead, ev.Reasoning} {
			if p != nil {
				tokens += *p
			}
		}
	}
	return len(evs), tokens
}

func TestKeepHistoryTrueKeepsVanishedTranscript(t *testing.T) {
	env := newKeepHistoryEnv(t)
	cfg := pricing.Defaults()
	if _, err := env.collect(t, &cfg); err != nil {
		t.Fatal(err)
	}
	before, err := env.st.EventsForSession("sess-b")
	if err != nil || len(before) == 0 {
		t.Fatalf("sess-b events before = %d, err %v", len(before), err)
	}
	if err := os.Remove(env.path("sess-b")); err != nil {
		t.Fatal(err)
	}
	payload, err := env.collect(t, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	after, err := env.st.EventsForSession("sess-b")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("sess-b events after = %d, want %d", len(after), len(before))
	}
	if _, _, _, ok, err := env.st.Cursor(env.path("sess-b")); err != nil || ok {
		t.Fatalf("sess-b cursor ok = %v, err %v, want forgotten", ok, err)
	}
	if len(payload.Sessions) != 2 {
		t.Fatalf("payload lists %d sessions, want 2", len(payload.Sessions))
	}
}

func TestKeepHistoryFalseDropsVanishedTranscript(t *testing.T) {
	env := newKeepHistoryEnv(t)
	cfg := pricing.Defaults()
	cfg.KeepHistory = false
	if _, err := env.collect(t, &cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(env.path("sess-b")); err != nil {
		t.Fatal(err)
	}
	payload, err := env.collect(t, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	after, err := env.st.EventsForSession("sess-b")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("sess-b events after = %d, want 0", len(after))
	}
	if len(payload.Sessions) != 1 {
		t.Fatalf("payload lists %d sessions, want 1", len(payload.Sessions))
	}
}

// A transcript that vanishes and comes back loses its cursor, so it is read
// again from byte 0; the upsert keys must keep its totals unchanged.
func TestVanishedTranscriptReappearsWithoutDoubling(t *testing.T) {
	env := newKeepHistoryEnv(t)
	cfg := pricing.Defaults()
	if _, err := env.collect(t, &cfg); err != nil {
		t.Fatal(err)
	}
	evs, err := env.st.EventsForSession("sess-b")
	if err != nil {
		t.Fatal(err)
	}
	rows, tokens := sumEvents(evs)
	if rows == 0 {
		t.Fatal("sess-b has no events")
	}
	if err := os.Remove(env.path("sess-b")); err != nil {
		t.Fatal(err)
	}
	if _, err := env.collect(t, &cfg); err != nil {
		t.Fatal(err)
	}
	env.write(t, "sess-b")
	if _, err := env.collect(t, &cfg); err != nil {
		t.Fatal(err)
	}
	evs, err = env.st.EventsForSession("sess-b")
	if err != nil {
		t.Fatal(err)
	}
	gotRows, gotTokens := sumEvents(evs)
	if gotRows != rows || gotTokens != tokens {
		t.Fatalf("after reappearing: %d rows, %d tokens; want %d rows, %d tokens", gotRows, gotTokens, rows, tokens)
	}
}

// Deleting every event keeps the cursors, so the next scan does not read the
// unchanged transcripts back in.
func TestCollectAfterDeleteAllDoesNotReingest(t *testing.T) {
	env := newKeepHistoryEnv(t)
	cfg := pricing.Defaults()
	if _, err := env.collect(t, &cfg); err != nil {
		t.Fatal(err)
	}
	if _, _, err := env.st.DeleteAllEvents(); err != nil {
		t.Fatal(err)
	}
	_, err := env.collect(t, &cfg)
	if !errors.Is(err, ErrNoSessions) {
		t.Fatalf("Collect error = %v, want ErrNoSessions", err)
	}
	all, err := env.st.AllEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("AllEvents = %d rows, want 0", len(all))
	}
}

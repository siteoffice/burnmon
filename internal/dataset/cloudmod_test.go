package dataset

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/store"
)

// TestCloudModAndPullCountOnce: the same cloud request stored from a mod file
// and from a pulled transcript counts once, in either order (hypothesis H2 of
// the cloud mod route plan). The mod row carries the transcript's request id,
// so the store's pulled-row rule keeps the pulled copy only.
func TestCloudModAndPullCountOnce(t *testing.T) {
	at := time.Date(2026, 11, 24, 10, 0, 1, 0, time.UTC)
	pulled := schema.Event{Vendor: "anthropic", Agent: "cowork-cloud", Surface: "cloud",
		SessionID: "session_01AB", RequestID: "req_M1", At: at, Model: "claude-opus-5-5", Input: 2, Output: 120}
	modBody := `{"source":"burnmon-cloud-mod","ts":"2026-11-24T10:00:01Z","session_id":"6f1c-cli","request_id":"req_M1","message_id":"msg_M1","model":"claude-opus-5-5","agent":"main","input_tokens":2,"output_tokens":120,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}` + "\n"

	for _, pullFirst := range []bool{false, true} {
		dir := t.TempDir()
		st, err := store.Open(filepath.Join(dir, "burnmon.db"))
		if err != nil {
			t.Fatal(err)
		}
		drop := filepath.Join(dir, ".burnmon", "cloud")
		if err := os.MkdirAll(drop, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(drop, "mod-6f1c-cli.jsonl")
		if err := os.WriteFile(path, []byte(modBody), 0o644); err != nil {
			t.Fatal(err)
		}
		c := Cache{Store: st, RootsByAdapter: map[string][]string{"claude": {drop}}}
		cfg := pricing.Defaults()
		ingestMod := func() {
			if err := c.IngestFile(&cfg, path); err != nil {
				t.Fatal(err)
			}
		}
		pull := func() {
			if err := st.UpsertEvents([]schema.Event{pulled}); err != nil {
				t.Fatal(err)
			}
		}
		if pullFirst {
			pull()
			ingestMod()
		} else {
			ingestMod()
			pull()
		}
		events, err := st.AllEvents()
		if err != nil {
			t.Fatal(err)
		}
		st.Close()
		if len(events) != 1 || events[0].SessionID != "session_01AB" || events[0].Output != 120 {
			t.Fatalf("pullFirst=%v: events = %+v, want only the pulled copy", pullFirst, events)
		}
	}
}

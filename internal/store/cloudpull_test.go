package store

import (
	"path/filepath"
	"sort"
	"testing"
	"time"

	"burnmon/internal/schema"
)

func rowKeys(t *testing.T, st *Store) []string {
	t.Helper()
	got, err := st.AllEvents()
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, e := range got {
		keys = append(keys, e.Agent+"|"+e.SessionID+"|"+e.RequestID)
	}
	sort.Strings(keys)
	return keys
}

func openTemp(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// A pulled row (cowork-cloud, session id "session_...") is the one copy of
// its request: it removes a teleport copy stored as Claude Code and an old
// drop row with the same request id, and a later copy of either is skipped.
func TestUpsertPulledRowIsTheOneCopy(t *testing.T) {
	st := openTemp(t)
	at := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	teleport := schema.Event{Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
		SessionID: "360e8015-local", RequestID: "req_1", At: at, Input: 5, Output: 7}
	drop := schema.Event{Vendor: "anthropic", Agent: "cowork-cloud", Surface: "cloud",
		SessionID: "a85e8e2c-container", RequestID: "req_1", At: at, Input: 5, Output: 7}
	unrelated := schema.Event{Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
		SessionID: "other", RequestID: "req_2", At: at, Input: 1, Output: 1}
	otherVendor := schema.Event{Vendor: "openai", Agent: "codex", Surface: "cli",
		SessionID: "x", RequestID: "req_1", At: at, Input: 1, Output: 1}
	if err := st.UpsertEvents([]schema.Event{teleport, drop, unrelated, otherVendor}); err != nil {
		t.Fatal(err)
	}
	pulled := schema.Event{Vendor: "anthropic", Agent: "cowork-cloud", Surface: "cloud",
		SessionID: "session_0131", RequestID: "req_1", At: at, Input: 5, Output: 7}
	if err := st.UpsertEvents([]schema.Event{pulled}); err != nil {
		t.Fatal(err)
	}
	// Re-reading the drop file and the teleport copy must not bring them back.
	if err := st.UpsertEvents([]schema.Event{drop, teleport}); err != nil {
		t.Fatal(err)
	}
	// A second pull of the same cloud session is the same key: still one row.
	if err := st.UpsertEvents([]schema.Event{pulled}); err != nil {
		t.Fatal(err)
	}
	got := rowKeys(t, st)
	want := []string{"claude-code|other|req_2", "codex|x|req_1", "cowork-cloud|session_0131|req_1"}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows = %v, want %v", got, want)
		}
	}
}

func TestRequestIDIndexExists(t *testing.T) {
	st := openTemp(t)
	var n int
	if err := st.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_events_request'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("index idx_events_request missing")
	}
}

// PurgePullCopies removes what a pull file left as anything but cowork-cloud,
// and its cursor, so the file is read again; nothing outside the pull
// folder's project folder is touched, and a clean store is a no-op.
func TestPurgePullCopies(t *testing.T) {
	st := openTemp(t)
	at := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	pullDir := `C:\Users\u\.claude\projects\C--Users-u--burnmon-cloud-pull`
	pullFile := pullDir + `\8adec148.jsonl`
	otherFile := `C:\Users\u\.claude\projects\C--work\keep1.jsonl`
	evs := []schema.Event{
		{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "8adec148", RequestID: "r1", At: at, Output: 1},
		{Vendor: "anthropic", Agent: "cowork-cloud", Surface: "cloud", SessionID: "session_X", RequestID: "r9", At: at, Output: 1},
		{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "keep1", RequestID: "r2", At: at, Output: 1},
	}
	if err := st.UpsertEvents(evs); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertToolCalls([]schema.ToolCall{{Vendor: "anthropic", Agent: "claude-code", SessionID: "8adec148", CallID: "c1", Turn: "r1", Tool: "Read", At: at}}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{pullFile, otherFile} {
		if err := st.SetCursor(p, 100, at, 100); err != nil {
			t.Fatal(err)
		}
	}
	ev, tc, cur, err := st.PurgePullCopies(pullDir)
	if err != nil || ev != 1 || tc != 1 || cur != 1 {
		t.Fatalf("PurgePullCopies = %d, %d, %d, %v; want 1, 1, 1", ev, tc, cur, err)
	}
	if _, _, _, ok, _ := st.Cursor(pullFile); ok {
		t.Error("pull file cursor kept")
	}
	if _, _, _, ok, _ := st.Cursor(otherFile); !ok {
		t.Error("other cursor removed")
	}
	got := rowKeys(t, st)
	if want := []string{"claude-code|keep1|r2", "cowork-cloud|session_X|r9"}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if ev, tc, cur, err := st.PurgePullCopies(pullDir); err != nil || ev+tc+cur != 0 {
		t.Fatalf("second purge = %d, %d, %d, %v; want a no-op", ev, tc, cur, err)
	}
}

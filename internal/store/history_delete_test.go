package store

import (
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/schema"
)

func openHistoryStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func histEvent(session, req, parent string, at time.Time) schema.Event {
	return schema.Event{
		Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
		SessionID: session, RequestID: req, ParentID: parent, Model: "claude-x",
		At: at, Input: 10, Output: 5,
	}
}

func histCall(session, id string, at time.Time) schema.ToolCall {
	return schema.ToolCall{
		Vendor: "anthropic", Agent: "claude-code", SessionID: session,
		CallID: id, Turn: "r", Tool: "Bash", At: at, InputBytes: 1,
	}
}

func countToolCalls(t *testing.T, st *Store) int {
	t.Helper()
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM tool_calls`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDeleteEventsBeforeUsesInstantNotText(t *testing.T) {
	st := openHistoryStore(t)
	whole := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	half := whole.Add(500 * time.Millisecond)
	later := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	if err := st.UpsertEvents([]schema.Event{
		histEvent("s", "r1", "", whole),
		histEvent("s", "r2", "", half),
		histEvent("s", "r3", "", later),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertToolCalls([]schema.ToolCall{histCall("s", "c1", whole), histCall("s", "c2", later)}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetCursor("a.jsonl", 10, whole, 10); err != nil {
		t.Fatal(err)
	}

	cutoff := whole.Add(250 * time.Millisecond)
	ev, tc, err := st.DeleteEventsBefore(cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if ev != 1 || tc != 1 {
		t.Fatalf("deleted (%d, %d), want (1, 1)", ev, tc)
	}
	got, err := st.AllEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("%d events remain, want 2", len(got))
	}
	for _, e := range got {
		if e.RequestID == "r1" {
			t.Fatalf("r1 (before cutoff) still stored")
		}
	}
	if n := countToolCalls(t, st); n != 1 {
		t.Fatalf("%d tool calls remain, want 1", n)
	}
	if _, _, _, ok, err := st.Cursor("a.jsonl"); err != nil || !ok {
		t.Fatalf("cursor gone after DeleteEventsBefore: ok=%v err=%v", ok, err)
	}
}

func TestDeleteSessionRemovesSubagents(t *testing.T) {
	st := openHistoryStore(t)
	at := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	if err := st.UpsertEvents([]schema.Event{
		histEvent("p", "r1", "", at),
		histEvent("p", "r2", "", at.Add(time.Second)),
		histEvent("c", "r3", "p", at),
		histEvent("x", "r4", "", at),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertToolCalls([]schema.ToolCall{
		histCall("p", "c1", at), histCall("c", "c2", at), histCall("x", "c3", at),
	}); err != nil {
		t.Fatal(err)
	}

	ev, tc, err := st.DeleteSession("p")
	if err != nil {
		t.Fatal(err)
	}
	if ev != 3 || tc != 2 {
		t.Fatalf("deleted (%d, %d), want (3, 2)", ev, tc)
	}
	got, err := st.AllEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SessionID != "x" {
		t.Fatalf("remaining events = %+v, want only session x", got)
	}
	if n := countToolCalls(t, st); n != 1 {
		t.Fatalf("%d tool calls remain, want 1", n)
	}
	if _, _, err := st.DeleteSession(""); err == nil {
		t.Fatal("DeleteSession(\"\") returned no error")
	}
}

func TestDeleteAllEventsKeepsCursors(t *testing.T) {
	st := openHistoryStore(t)
	at := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	if err := st.UpsertEvents([]schema.Event{histEvent("s", "r1", "", at)}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertToolCalls([]schema.ToolCall{histCall("s", "c1", at)}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetCursor("a.jsonl", 42, at, 99); err != nil {
		t.Fatal(err)
	}

	ev, tc, err := st.DeleteAllEvents()
	if err != nil {
		t.Fatal(err)
	}
	if ev != 1 || tc != 1 {
		t.Fatalf("deleted (%d, %d), want (1, 1)", ev, tc)
	}
	got, err := st.AllEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 || countToolCalls(t, st) != 0 {
		t.Fatalf("events or tool calls remain after DeleteAllEvents")
	}
	off, _, _, ok, err := st.Cursor("a.jsonl")
	if err != nil || !ok || off != 42 {
		t.Fatalf("cursor = (%d, ok=%v, err=%v), want (42, true, nil)", off, ok, err)
	}
}

func TestForgetCursorsForOtherPathsKeepsEvents(t *testing.T) {
	st := openHistoryStore(t)
	at := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	if err := st.UpsertEvents([]schema.Event{
		histEvent("a", "r1", "", at),
		histEvent("b", "r2", "", at),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetCursor("a.jsonl", 1, at, 1); err != nil {
		t.Fatal(err)
	}
	if err := st.SetCursor("b.jsonl", 1, at, 1); err != nil {
		t.Fatal(err)
	}

	n, err := st.ForgetCursorsForOtherPaths([]string{"a.jsonl"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("forgot %d cursors, want 1", n)
	}
	if _, _, _, ok, _ := st.Cursor("b.jsonl"); ok {
		t.Fatal("b.jsonl cursor still present")
	}
	if _, _, _, ok, _ := st.Cursor("a.jsonl"); !ok {
		t.Fatal("a.jsonl cursor was removed")
	}
	got, err := st.AllEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("%d events remain, want 2 (events must stay)", len(got))
	}
}

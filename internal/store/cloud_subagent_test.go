package store

import (
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/schema"
)

// TestUpsertCloudSubagentMovesFoldedRow: before 2026-10-25 the Cowork
// (Cloud) parser stored a subagent's request under its parent session.
// When the same request arrives again as a subagent row (own session,
// parent set), the folded copy under the parent goes, so the tokens count
// once. Only a cowork-cloud row with no parent of its own is removed: two
// sessions may share a request id in other harnesses (mirror sessions).
func TestUpsertCloudSubagentMovesFoldedRow(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	at := time.Date(2026, 10, 25, 13, 37, 0, 0, time.UTC)
	folded := schema.Event{Vendor: "anthropic", Agent: "cowork-cloud", Surface: "cloud",
		SessionID: "parent-1", RequestID: "req_S", At: at, Input: 2, Output: 4}
	mainRow := schema.Event{Vendor: "anthropic", Agent: "cowork-cloud", Surface: "cloud",
		SessionID: "parent-1", RequestID: "req_M", At: at, Input: 2, Output: 9}
	mirror := schema.Event{Vendor: "anthropic", Agent: "cowork", Surface: "desktop",
		SessionID: "parent-1", RequestID: "req_X", At: at, Input: 1, Output: 1}
	if err := st.UpsertEvents([]schema.Event{folded, mainRow, mirror}); err != nil {
		t.Fatal(err)
	}
	sub := folded
	sub.SessionID, sub.ParentID = "agent-ae4d83ca", "parent-1"
	other := mirror
	other.Agent, other.Surface, other.SessionID, other.ParentID = "cowork-cloud", "cloud", "agent-zz", "parent-1"
	if err := st.UpsertEvents([]schema.Event{sub, other}); err != nil {
		t.Fatal(err)
	}
	got, err := st.AllEvents()
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]string{}
	for _, e := range got {
		keys[e.SessionID+"/"+e.RequestID] = e.ParentID
	}
	want := map[string]string{
		"agent-ae4d83ca/req_S": "parent-1",
		"parent-1/req_M":       "",
		"parent-1/req_X":       "", // not cowork-cloud: never removed
		"agent-zz/req_X":       "parent-1",
	}
	if len(keys) != len(want) {
		t.Fatalf("got rows %v, want %v", keys, want)
	}
	for k, p := range want {
		if gp, ok := keys[k]; !ok || gp != p {
			t.Fatalf("got rows %v, want %v", keys, want)
		}
	}
}

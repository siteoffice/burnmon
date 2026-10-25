package claude

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseCloudDropSubagent: a drop row with an agent other than "main"
// is a subagent's request. It becomes its own session, the agent id
// ("agent-<id>", as a local subagent trail is named), with the drop row's
// session id as its parent, so the Station draws it under the parent.
// Measured 2026-10-25 in a live cloud session: the plugin writes
// "agent":"agent-ae4d83ca9bb5c66f3","is_sidechain":true for a subagent row.
func TestParseCloudDropSubagent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".burnmon", "cloud")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "latest.jsonl")
	sub := `{"v":1,"source":"burnmon-cloud","ts":"2026-10-25T13:37:51.085Z","session_id":"0fd6bf8c-6ce5","request_id":"req_S","message_id":"msg_S","model":"claude-haiku-5-5","agent":"agent-ae4d83ca","is_sidechain":true,"entrypoint":"remote_cowork","input_tokens":2,"output_tokens":4,"cache_creation_input_tokens":80608,"cache_read_input_tokens":0}`
	noAgent := `{"v":1,"source":"burnmon-cloud","ts":"2026-10-25T13:37:52.000Z","session_id":"0fd6bf8c-6ce5","request_id":"req_N","message_id":"msg_N","model":"claude-opus-5-5","input_tokens":1,"output_tokens":1}`
	if err := os.WriteFile(path, []byte(cloudRowA+"\n"+sub+"\n"+noAgent+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	events, _, _, err := Adapter{}.Parse(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	main, s, old := events[0], events[1], events[2]
	if main.SessionID != "551731f0-5334" || main.ParentID != "" {
		t.Fatalf("main row session/parent = %q/%q, want 551731f0-5334/empty", main.SessionID, main.ParentID)
	}
	if s.SessionID != "agent-ae4d83ca" || s.ParentID != "0fd6bf8c-6ce5" {
		t.Fatalf("subagent row session/parent = %q/%q, want agent-ae4d83ca/0fd6bf8c-6ce5", s.SessionID, s.ParentID)
	}
	if s.Agent != "cowork-cloud" || s.Surface != "cloud" || s.RequestID != "req_S" || s.Model != "claude-haiku-5-5" {
		t.Fatalf("subagent row agent/surface/request/model = %s/%s/%s/%s", s.Agent, s.Surface, s.RequestID, s.Model)
	}
	// A row from an older plugin without the agent field stays on its session.
	if old.SessionID != "0fd6bf8c-6ce5" || old.ParentID != "" {
		t.Fatalf("row without agent session/parent = %q/%q, want 0fd6bf8c-6ce5/empty", old.SessionID, old.ParentID)
	}
}

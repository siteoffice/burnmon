package claude

import (
	"os"
	"path/filepath"
	"testing"
)

const cloudRowA = `{"v":1,"source":"burnmon-cloud","ts":"2026-10-12T15:32:10.310Z","session_id":"551731f0-5334","request_id":"req_A","message_id":"msg_A","model":"claude-sonnet-5-5","agent":"main","is_sidechain":false,"entrypoint":"remote_cowork","service_tier":"standard","speed":"standard","cache_5m":0,"cache_1h":30793,"thinking_tokens":12,"input_tokens":2,"output_tokens":192,"cache_creation_input_tokens":30793,"cache_read_input_tokens":103204}`
const cloudRowB = `{"v":1,"source":"burnmon-cloud","ts":"2026-10-12T15:32:13.376Z","session_id":"551731f0-5334","request_id":"req_B","message_id":"msg_B","model":"claude-sonnet-5-5","agent":"main","is_sidechain":false,"entrypoint":"remote_cowork","service_tier":"standard","speed":"standard","cache_5m":0,"cache_1h":345,"thinking_tokens":0,"input_tokens":2,"output_tokens":157,"cache_creation_input_tokens":345,"cache_read_input_tokens":133997}`
const cloudRowOther = `{"v":1,"source":"burnmon-cloud-probe","ts":"2026-10-12T15:31:55.371Z","session_id":"1afdc570-0ca2","request_id":"req_C","message_id":"msg_C","model":"claude-opus-5-5","agent":"agent-a1","is_sidechain":true,"entrypoint":"remote_cowork","input_tokens":2,"output_tokens":87,"cache_creation_input_tokens":24504,"cache_read_input_tokens":103857}`

// TestParseCloudDrop: a drop file from the BurnMon cloud plugin becomes one
// Cowork (Cloud) event per line, keyed by the line's own session and request
// ids. The plugin rewrites the file whole every turn (and latest.jsonl can
// switch to another session's rows), so Parse reads it from the top whatever
// offset it is given.
func TestParseCloudDrop(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".burnmon", "cloud")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "latest.jsonl")
	if err := os.WriteFile(path, []byte(cloudRowA+"\n"+cloudRowB+"\n"+"not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	events, calls, offset, err := Adapter{}.Parse(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(path)
	if offset != fi.Size() {
		t.Fatalf("offset = %d, want file size %d", offset, fi.Size())
	}
	if len(calls) != 0 {
		t.Fatalf("got %d tool calls, want 0 (the drop carries none)", len(calls))
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	e := events[0]
	if e.Vendor != "anthropic" || e.Agent != "cowork-cloud" || e.Surface != "cloud" {
		t.Fatalf("vendor/agent/surface = %s/%s/%s, want anthropic/cowork-cloud/cloud", e.Vendor, e.Agent, e.Surface)
	}
	if e.SessionID != "551731f0-5334" || e.RequestID != "req_A" || e.Model != "claude-sonnet-5-5" {
		t.Fatalf("session/request/model = %s/%s/%s", e.SessionID, e.RequestID, e.Model)
	}
	if e.Input != 2 || e.Output != 192 || e.CacheWrite == nil || *e.CacheWrite != 30793 || e.CacheRead == nil || *e.CacheRead != 103204 {
		t.Fatalf("tokens in=%d out=%d cw=%v cr=%v", e.Input, e.Output, e.CacheWrite, e.CacheRead)
	}
	if e.At.Format("2006-01-02T15:04:05.000Z") != "2026-10-12T15:32:10.310Z" {
		t.Fatalf("At = %v", e.At)
	}
	if e.Project != "" || e.Title != "" {
		t.Fatalf("Project/Title = %q/%q, want empty (the drop carries no paths or text)", e.Project, e.Title)
	}

	// Rewritten with another session's rows, longer than before: Parse
	// must start at byte 0, not at the old offset.
	if err := os.WriteFile(path, []byte(cloudRowOther+"\n"+cloudRowA+"\n"+cloudRowB+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	events, _, _, err = Adapter{}.Parse(path, offset)
	if err != nil {
		t.Fatal(err)
	}
	// req_C is a subagent row (agent-a1): its own session under its parent.
	if len(events) != 3 || events[0].SessionID != "agent-a1" || events[0].ParentID != "1afdc570-0ca2" || events[0].RequestID != "req_C" {
		t.Fatalf("after rewrite got %d events, first %+v; want 3 starting with req_C as agent-a1 under 1afdc570-0ca2", len(events), events[0])
	}
}

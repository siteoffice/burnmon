package claude

import (
	"os"
	"path/filepath"
	"testing"
)

// Rows the burnmon-cloud 2.0.0 mod writes: request and message ids looked up
// in the session transcript (plan section 4a), or, when the lookup has not
// found the row yet, only a step key made of the turn id and the step index.
const modRowResolved = `{"source":"burnmon-cloud-mod","ts":"2026-11-24T10:00:01.000Z","session_id":"6f1c-cli","request_id":"req_M1","message_id":"msg_M1","model":"claude-opus-5-5","agent":"main","input_tokens":2,"output_tokens":120,"cache_creation_input_tokens":500,"cache_read_input_tokens":9000}`
const modRowStepKey = `{"source":"burnmon-cloud-mod","ts":"2026-11-24T10:00:05.000Z","session_id":"6f1c-cli","request_id":"","message_id":"","step_key":"turn-t9-3","model":"claude-opus-5-5","agent":"main","input_tokens":2,"output_tokens":115,"cache_creation_input_tokens":0,"cache_read_input_tokens":9500}`
const modRowNoKey = `{"source":"burnmon-cloud-mod","ts":"2026-11-24T10:00:06.000Z","session_id":"6f1c-cli","model":"claude-opus-5-5","agent":"main","input_tokens":1,"output_tokens":1}`

// TestParseCloudModFile: a mod-<session>.jsonl in .burnmon\cloud is read by the
// same drop parser. A resolved row keys on its request id; a row the mod could
// not resolve keys on its step key so its usage still counts; a row with no key
// at all is skipped.
func TestParseCloudModFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".burnmon", "cloud")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "mod-6f1c-cli.jsonl")
	body := modRowResolved + "\n" + modRowStepKey + "\n" + modRowNoKey + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	events, _, _, err := Adapter{}.Parse(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2 (the keyless row skipped)", len(events))
	}
	if e := events[0]; e.RequestID != "req_M1" || e.Agent != "cowork-cloud" || e.SessionID != "6f1c-cli" || e.Output != 120 {
		t.Fatalf("resolved row = %+v", e)
	}
	if e := events[1]; e.RequestID != "turn-t9-3" || e.Output != 115 {
		t.Fatalf("step-key row = %+v, want request id turn-t9-3", e)
	}
}

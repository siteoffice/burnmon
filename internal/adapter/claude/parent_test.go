package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A Claude Code subagent writes its own trail at
// <parent session>\subagents\agent-<id>.jsonl. Every line carries the
// parent's sessionId and isSidechain true. R6: the event must carry that
// parent as ParentID, or the subagent shows as a session of its own.
func writeSubagentTrail(t *testing.T) (path string, firstLineLen int64) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "proj", "parent-1", "subagents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"parentUuid":null,"isSidechain":true,"agentId":"abc","sessionId":"parent-1","cwd":"C:\\proj\\demo","type":"user","message":{"role":"user","content":"look around"},"timestamp":"2026-10-23T09:00:00Z"}`,
		`{"isSidechain":true,"agentId":"abc","sessionId":"parent-1","cwd":"C:\\proj\\demo","type":"assistant","requestId":"req-a","message":{"model":"claude-sonnet-5-5","usage":{"input_tokens":10,"output_tokens":5},"content":[]},"timestamp":"2026-10-23T09:00:01Z"}`,
		`{"isSidechain":true,"agentId":"abc","sessionId":"parent-1","cwd":"C:\\proj\\demo","type":"assistant","requestId":"req-b","message":{"model":"claude-sonnet-5-5","usage":{"input_tokens":12,"output_tokens":7},"content":[]},"timestamp":"2026-10-23T09:00:02Z"}`,
	}
	path = filepath.Join(dir, "agent-abc.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, int64(len(lines[0]) + 1)
}

func TestParseSubagentSetsParentID(t *testing.T) {
	path, _ := writeSubagentTrail(t)
	events, calls, _, err := Adapter{}.Parse(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	for _, e := range events {
		if e.SessionID != "agent-abc" {
			t.Fatalf("SessionID = %q, want agent-abc (the file name)", e.SessionID)
		}
		if e.ParentID != "parent-1" {
			t.Fatalf("ParentID = %q, want parent-1 (the sessionId on the subagent's lines)", e.ParentID)
		}
	}
	_ = calls
}

// An incremental read that starts after the first line still finds the
// parent: every line carries it, not only the first.
func TestParseSubagentIncrementalKeepsParentID(t *testing.T) {
	path, first := writeSubagentTrail(t)
	events, _, _, err := Adapter{}.Parse(path, first)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	for _, e := range events {
		if e.ParentID != "parent-1" {
			t.Fatalf("ParentID = %q, want parent-1 on an incremental read", e.ParentID)
		}
	}
}

// A main session's own lines carry its own sessionId and no isSidechain, so
// it gets no parent.
func TestParseMainSessionHasNoParentID(t *testing.T) {
	events, _, _, err := Adapter{}.Parse(filepath.Join("testdata", "basic.jsonl"), 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.ParentID != "" {
			t.Fatalf("ParentID = %q on a main session, want empty", e.ParentID)
		}
	}
}

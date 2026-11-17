package claude

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/cloudpull"
)

const pullLines = `{"type":"user","cwd":"/ccs-homes/h/1/tmp/cc-tenant-x","entrypoint":"sdk-cli","message":{"content":"hello"},"timestamp":"2026-11-17T07:46:00Z"}
{"type":"assistant","cwd":"/ccs-homes/h/1/tmp/cc-tenant-x","entrypoint":"sdk-cli","requestId":"req_A","timestamp":"2026-11-17T07:46:14Z","message":{"model":"claude-opus-5-5","usage":{"input_tokens":10,"output_tokens":3},"content":[{"type":"tool_use","id":"tu1","name":"Read","input":{"file_path":"/x"}}]}}
{"type":"assistant","cwd":"C:\\Users\\u\\.burnmon\\cloud-pull","entrypoint":"sdk-cli","requestId":"req_A","timestamp":"2026-11-17T07:46:15Z","message":{"model":"claude-opus-5-5","usage":{"input_tokens":10,"output_tokens":9}}}
`

func writePull(t *testing.T, name string) string {
	t.Helper()
	pd := cloudpull.ProjectDir()
	if err := os.MkdirAll(pd, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(pd, name)
	if err := os.WriteFile(p, []byte(pullLines), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func setPullHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
}

// A pull file in the state file is Cowork (Cloud) under the cloud session id,
// with no project, whatever its cwd and entrypoint say.
func TestParsePullFileMapped(t *testing.T) {
	setPullHome(t)
	st := &cloudpull.State{Sessions: map[string]*cloudpull.Entry{
		"session_0131": {SessionID: "local-1", Path: "x", PulledAt: time.Now()},
	}}
	if err := st.Save(cloudpull.StatePath()); err != nil {
		t.Fatal(err)
	}
	p := writePull(t, "local-1.jsonl")
	events, calls, off, err := Adapter{}.Parse(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(p)
	if off != fi.Size() {
		t.Fatalf("offset %d, want %d", off, fi.Size())
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	e := events[0]
	if e.Agent != "cowork-cloud" || e.Surface != "cloud" || e.SessionID != "session_0131" ||
		e.Project != "" || e.ParentID != "" || e.RequestID != "req_A" || e.Output != 9 {
		t.Fatalf("event = %+v", e)
	}
	if len(calls) != 1 || calls[0].Agent != "cowork-cloud" || calls[0].SessionID != "session_0131" {
		t.Fatalf("tool calls = %+v", calls)
	}
}

// A pull file the runner has not recorded yet gives no rows and offset 0, so
// it is read again later; no row is ever stored under a local pull id.
func TestParsePullFileNotYetMapped(t *testing.T) {
	setPullHome(t)
	p := writePull(t, "local-2.jsonl")
	events, calls, off, err := Adapter{}.Parse(p, 0)
	if err != nil || len(events) != 0 || len(calls) != 0 || off != 0 {
		t.Fatalf("Parse = %d events, %d calls, offset %d, %v; want nothing at offset 0", len(events), len(calls), off, err)
	}
}

// The same file outside the pull folder stays Claude Code.
func TestParseNonPullFileUnchanged(t *testing.T) {
	setPullHome(t)
	p := filepath.Join(t.TempDir(), "local-3.jsonl")
	if err := os.WriteFile(p, []byte(pullLines), 0o644); err != nil {
		t.Fatal(err)
	}
	events, _, _, err := Adapter{}.Parse(p, 0)
	if err != nil || len(events) != 1 || events[0].Agent != "claude-code" || events[0].SessionID != "local-3" {
		t.Fatalf("Parse = %+v, %v", events, err)
	}
}

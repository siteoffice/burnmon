package controls

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditLineContent(t *testing.T) {
	var sb strings.Builder
	l := AuditLine{Time: "2026-10-26T10:00:00Z", Control: "stop", Target: "ab12", Command: `C:\x\claude.exe stop ab12`, ExitCode: 0}
	if err := WriteAudit(&sb, l); err != nil {
		t.Fatal(err)
	}
	want := `{"time":"2026-10-26T10:00:00Z","control":"stop","target":"ab12","command":"C:\\x\\claude.exe stop ab12","exit_code":0}` + "\n"
	if sb.String() != want {
		t.Fatalf("got  %q\nwant %q", sb.String(), want)
	}

	sb.Reset()
	l.ExitCode = -1
	l.Refused = ErrExpired.Error()
	if err := WriteAudit(&sb, l); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), `"exit_code":-1,"refused":"preview expired, `) {
		t.Fatalf("refused line %q", sb.String())
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(sb.String()), &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"time", "control", "target", "command", "exit_code", "refused"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing key %s", k)
		}
	}
	if len(m) != 6 {
		t.Fatalf("keys %v", m)
	}
}

func TestAuditLineHasNoToken(t *testing.T) {
	var sb strings.Builder
	if err := WriteAudit(&sb, AuditLine{Time: "t", Control: "stop", Target: "ab12", Command: "c", ExitCode: 0}); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(sb.String()), &m); err != nil {
		t.Fatal(err)
	}
	for k := range m {
		if strings.Contains(strings.ToLower(k), "token") {
			t.Fatalf("token key %s", k)
		}
	}
}

func TestAuditAppendsAndNeverTruncates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controls-audit.ndjson")
	l := AuditLine{Time: "2026-10-26T10:00:00Z", Control: "stop", Target: "ab12", Command: "c", ExitCode: 0}
	for i := 0; i < 2; i++ {
		f, err := OpenAudit(path)
		if err != nil {
			t.Fatal(err)
		}
		l.Target = []string{"ab12", "cd34"}[i]
		if err := WriteAudit(f, l); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"ab12"`) || !strings.Contains(lines[1], `"cd34"`) {
		t.Fatalf("lines %q", lines)
	}
}

func TestOpenAuditFailsWhenFolderIsAFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if f, err := OpenAudit(filepath.Join(file, "controls-audit.ndjson")); err == nil {
		f.Close()
		t.Fatal("expected error")
	}
}

func TestAuditPath(t *testing.T) {
	got := AuditPath(filepath.Join("C:\\x", "burnmon", "burnmon.db"))
	want := filepath.Join("C:\\x", "burnmon", "controls-audit.ndjson")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

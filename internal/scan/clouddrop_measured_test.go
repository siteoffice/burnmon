package scan

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCloudLastMeasured: the newest row time over every drop file, so the
// vendor strip can tell "no usage" from "usage no longer measured".
func TestCloudLastMeasured(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	write := func(dir, name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(a, "latest.jsonl", `{"source":"burnmon-cloud","ts":"2026-11-11T09:00:00Z","session_id":"s"}`+"\n"+
		`not json`+"\n"+
		`{"source":"burnmon-cloud","ts":"2026-11-12T16:43:00Z","session_id":"s"}`+"\n"+
		`{"source":"other","ts":"2026-11-30T00:00:00Z","session_id":"s"}`+"\n")
	write(b, "old.jsonl", `{"source":"burnmon-cloud","ts":"2026-11-01T00:00:00Z","session_id":"x"}`+"\n")
	write(b, "note.txt", `{"source":"burnmon-cloud","ts":"2026-12-01T00:00:00Z","session_id":"x"}`+"\n")

	got, ok := CloudLastMeasured([]string{a, b, filepath.Join(a, "missing")})
	want := time.Date(2026, 11, 12, 16, 43, 0, 0, time.UTC)
	if !ok || !got.Equal(want) {
		t.Fatalf("CloudLastMeasured = %v, %v; want %v, true", got, ok, want)
	}
}

func TestCloudLastMeasuredNoRows(t *testing.T) {
	empty := t.TempDir()
	if _, ok := CloudLastMeasured([]string{empty, filepath.Join(empty, "missing")}); ok {
		t.Error("CloudLastMeasured reported a time with no drop rows")
	}
	if _, ok := CloudLastMeasured(nil); ok {
		t.Error("CloudLastMeasured reported a time with no folders")
	}
}

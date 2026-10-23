package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The fixture is lane A's frozen R4 example, not a hand-made copy.
const examplePath = "../../testdata/snapshot_example.json"

var exampleWritten = time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)

func TestLoadFreshExample(t *testing.T) {
	r := Load(examplePath, exampleWritten.Add(10*time.Second))
	if r.Status != StatusOK {
		t.Fatalf("status %q, want ok (error %q)", r.Status, r.Error)
	}
	if r.AgeSeconds != 10 {
		t.Errorf("age %d, want 10", r.AgeSeconds)
	}
	if r.Snapshot == nil || len(r.Snapshot.Sessions) != 2 || len(r.Snapshot.WaitsOnYou) != 1 {
		t.Fatalf("snapshot not carried through: %+v", r.Snapshot)
	}
}

// Three ticks of 5 seconds is the limit: 15 s is fresh, 16 s is stale, and a
// stale file must not hand its numbers to the page as current.
func TestLoadStaleExample(t *testing.T) {
	if r := Load(examplePath, exampleWritten.Add(15*time.Second)); r.Status != StatusOK {
		t.Errorf("15 s old: %q, want ok", r.Status)
	}
	r := Load(examplePath, exampleWritten.Add(16*time.Second))
	if r.Status != StatusStale {
		t.Fatalf("16 s old: %q, want stale", r.Status)
	}
	if r.AgeSeconds != 16 {
		t.Errorf("age %d, want 16", r.AgeSeconds)
	}
	if r.Snapshot != nil {
		t.Error("a stale reading must not carry the old snapshot")
	}
}

func TestLoadMissingUnreadableAndNewVersion(t *testing.T) {
	dir := t.TempDir()
	now := exampleWritten

	if r := Load(filepath.Join(dir, "none.json"), now); r.Status != StatusMissing || r.Snapshot != nil {
		t.Errorf("missing file: %+v", r)
	}

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"schema_version": `), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := Load(bad, now); r.Status != StatusUnreadable || r.Error == "" {
		t.Errorf("half a file: %+v", r)
	}

	b, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatal(err)
	}
	v2 := filepath.Join(dir, "v2.json")
	changed := []byte(strings.Replace(string(b), `"schema_version": 1`, `"schema_version": 2`, 1))
	if err := os.WriteFile(v2, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	if r := Load(v2, now); r.Status != StatusUnsupported {
		t.Errorf("schema 2: %+v", r)
	}
}

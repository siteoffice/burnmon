package scan

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCloudModNewest: the newest delivery of the burnmon-cloud mod is the
// newest modification time of a mod-*.jsonl in any drop folder. Other drop
// files (latest.jsonl of the retired plugin) do not count, and no mod file
// means no delivery yet.
func TestCloudModNewest(t *testing.T) {
	a := filepath.Join(t.TempDir(), ".burnmon", "cloud")
	b := filepath.Join(t.TempDir(), ".burnmon", "cloud")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := CloudModNewest([]string{a, b}); ok {
		t.Fatal("empty folders report a delivery")
	}
	put := func(dir, name string, mt time.Time) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	put(a, "latest.jsonl", base.Add(time.Hour)) // the old plugin's file: ignored
	if _, ok := CloudModNewest([]string{a, b}); ok {
		t.Fatal("latest.jsonl counted as a mod delivery")
	}
	put(a, "mod-s1.jsonl", base)
	put(b, "mod-s2.jsonl", base.Add(10*time.Minute))
	got, ok := CloudModNewest([]string{a, b})
	if !ok || !got.Equal(base.Add(10*time.Minute)) {
		t.Fatalf("CloudModNewest = %v %v, want %v", got, ok, base.Add(10*time.Minute))
	}
}

package pricing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteConfigKeyKeepsOtherKeys(t *testing.T) {
	p := filepath.Join(t.TempDir(), "burnmon.json")
	if err := os.WriteFile(p, []byte(`{"view":"full","agent_view":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfigKey(p, "cloud_pull", true); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	b, _ := os.ReadFile(p)
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["view"] != "full" || m["agent_view"] != true || m["cloud_pull"] != true {
		t.Fatalf("file = %v", m)
	}
	if !ReadCloudPull(p) {
		t.Error("ReadCloudPull = false after writing true")
	}
	cfg, err := Load(p)
	if err != nil || !cfg.CloudPull {
		t.Fatalf("Load: CloudPull %v, %v", cfg.CloudPull, err)
	}
	if ReadCloudPull(filepath.Join(t.TempDir(), "missing.json")) {
		t.Error("missing file reads as on")
	}
}

// A missing file is created: the first save from a window.
func TestWriteConfigKeyNewFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "burnmon.json")
	if err := WriteConfigKey(p, "cloud_pull", false); err != nil {
		t.Fatal(err)
	}
	if ReadCloudPull(p) {
		t.Error("ReadCloudPull = true after writing false")
	}
}

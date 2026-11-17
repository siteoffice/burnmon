package pricing

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// WriteConfigKey merges value into the named key of the JSON file at path,
// leaving every other key exactly as it was. A missing or unreadable file is
// treated as empty: very likely the first save from a window. Written via a
// .tmp file plus rename, so a crash mid-write never corrupts the real file.
// Both apps use it, so a setting saved in one is the same bytes as in the other.
func WriteConfigKey(path, key string, value interface{}) error {
	raw := map[string]json.RawMessage{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &raw)
	}
	valBytes, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw[key] = valBytes
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadCloudPull reads only the cloud_pull switch from the file at path, so an
// app sees a switch the other app flipped without a restart. Missing or
// unreadable means off.
func ReadCloudPull(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var v struct {
		CloudPull bool `json:"cloud_pull"`
	}
	if json.Unmarshal(b, &v) != nil {
		return false
	}
	return v.CloudPull
}

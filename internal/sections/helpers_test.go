package sections

import (
	"encoding/json"
	"os"
	"testing"
)

func writeRaw(path, body string) error { return os.WriteFile(path, []byte(body), 0o600) }

func readRaw(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

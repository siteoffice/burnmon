package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicWritesBytesAndLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.json")
	for _, content := range []string{"first", "second, longer than the first"} {
		if err := WriteFileAtomic(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(p)
		if err != nil || string(got) != content {
			t.Fatalf("read %q, %v; want %q", got, err, content)
		}
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 {
		t.Errorf("folder holds %d entries, want only the target", len(ents))
	}
}

func TestWriteFileAtomicMissingFolderFailsWithoutCreatingIt(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nope", "a.json")
	if err := WriteFileAtomic(p, []byte("x"), 0o644); err == nil {
		t.Fatal("want an error for a missing folder, as os.WriteFile gives")
	}
	if _, err := os.Stat(filepath.Dir(p)); err == nil {
		t.Error("the folder was created")
	}
}

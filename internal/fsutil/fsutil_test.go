package fsutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadFileRetryReadsAFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.json")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFileRetry(p)
	if err != nil || string(got) != "hello" {
		t.Fatalf("read %q, %v; want hello", got, err)
	}
}

// A missing file is an answer, not a race: it returns at once with ErrNotExist.
func TestReadFileRetryMissingFileReturnsAtOnce(t *testing.T) {
	start := time.Now()
	_, err := ReadFileRetry(filepath.Join(t.TempDir(), "nope.json"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v, want ErrNotExist", err)
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Errorf("a missing file took %s, want no retry", d)
	}
}

// A reader that meets the instant of an atomic rename must never see an error.
func TestReadFileRetryNeverFailsAgainstAConcurrentAtomicWrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.json")
	if err := WriteFileAtomic(p, []byte("0"), 0o644); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				_ = WriteFileAtomic(p, []byte("12345"), 0o644)
			}
		}
	}()
	defer func() { close(stop); <-done }()
	for i := 0; i < 300; i++ {
		if b, err := ReadFileRetry(p); err != nil || len(b) == 0 {
			t.Fatalf("read %d: %q, %v", i, b, err)
		}
	}
}

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

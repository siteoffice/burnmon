//go:build windows

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"burnmon/internal/pricing"
	"burnmon/internal/snapshotwriter"
	"burnmon/internal/snapshotwriter/writertest"
)

// The dev app's writer, wired as the app wires it, writes the same bytes as
// burnmon.exe's on the shared fixture.
func TestDevSnapshotBytesEqualGolden(t *testing.T) {
	st := writertest.Store(t)
	a := &app{cfg: pricing.Defaults()}
	path := filepath.Join(t.TempDir(), "snapshot.json")
	w := a.newSnapshotWriter(st, path)
	writertest.Fake(w)
	if err := w.Once(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := writertest.Golden(t); !bytes.Equal(got, want) {
		t.Fatalf("snapshot bytes differ from golden:\n%s\n%s", got, want)
	}
	if w.Tick != snapshotwriter.Tick {
		t.Fatalf("Tick = %v, want %v", w.Tick, snapshotwriter.Tick)
	}
}
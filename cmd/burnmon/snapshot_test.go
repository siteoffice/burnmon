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

// The app's writer records the config file the app loaded at start, and
// nothing when it loaded none.
func TestBurnmonSnapshotWriterRecordsLoadedConfig(t *testing.T) {
	st := writertest.Store(t)
	for _, loaded := range []string{`C:\portable\burnmon.json`, ""} {
		a := &app{cfg: pricing.Defaults(), loadedCfg: loaded}
		w := a.newSnapshotWriter(st, filepath.Join(t.TempDir(), "snapshot.json"))
		if w.Graph == nil || w.Graph.ConfigPath != loaded {
			t.Errorf("loaded %q: graph %+v", loaded, w.Graph)
		}
	}
}

// The app's writer, wired as the app wires it, writes the same bytes as the
// shared writer on the shared fixture.
func TestBurnmonSnapshotBytesEqualGolden(t *testing.T) {
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

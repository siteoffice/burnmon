package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

// d42Snapshot counts its turns back from d42TurnsBase, not the clock: two
// builds are the same bytes, and the newest turn sits 20 s before the base.
func TestD42SnapshotIsFixed(t *testing.T) {
	a, err := d42Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	b, err := d42Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("two builds differ (%d and %d bytes)", len(a), len(b))
	}
	var m struct {
		Burn struct {
			Turns []struct {
				At string `json:"at"`
			} `json:"turns"`
		} `json:"burn"`
	}
	if err := json.Unmarshal(a, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Burn.Turns) != 24 {
		t.Fatalf("burn.turns has %d turns, want 24", len(m.Burn.Turns))
	}
	if got, want := m.Burn.Turns[0].At, "2026-10-23T09:57:40Z"; got != want {
		t.Fatalf("first turn at %s, want %s", got, want)
	}
	if got, want := m.Burn.Turns[23].At, "2026-10-23T09:50:00Z"; got != want {
		t.Fatalf("last turn at %s, want %s", got, want)
	}
}

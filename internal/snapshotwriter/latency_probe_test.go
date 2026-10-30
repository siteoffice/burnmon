package snapshotwriter_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"burnmon/internal/graph"
	"burnmon/internal/snapshotwriter"
	"burnmon/internal/waits"
)

// The uicheck case latency/gate (tools\uicheck\latency_gate.go) times how
// fast its probe sprint shows on the page. uicheck is its own module, so
// testdata\latency_probe_sprint.json is the shared shape: uicheck pins the
// harness to it, and this test proves the writer turns it, written where the
// harness writes it, into one Blocked sprint item, and its "running" form
// into none. A wrong fixture fails here, not in a live run.
func TestLatencyProbeIsBlockedSprint(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "latency_probe_sprint.json"))
	if err != nil {
		t.Fatal(err)
	}
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatal(err)
	}
	id := "sprint:" + probe["sprint_id"].(string)
	for _, tc := range []struct {
		status  string
		blocked bool
	}{{"waiting-on-you", true}, {"running", false}} {
		root := t.TempDir()
		dir := filepath.Join(root, "sprints", "burnmon", "b8-gate-latency-probe")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		probe["status"] = tc.status
		b, err := json.Marshal(probe)
		if err != nil {
			t.Fatal(err)
		}
		for name, body := range map[string][]byte{"sprint.json": b, "probe-by-uicheck.txt": []byte("marker\r\n")} {
			if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		gr := snapshotwriter.NewGraphReaders(t.TempDir(), root)
		gr.Listen = func() ([]graph.Listen, error) { return nil, nil }
		got := onceRead(t, newWriter(snapshotTestStore(t), filepath.Join(t.TempDir(), "snapshot.json"), noProcesses, gr))
		var item *waits.Item
		for i := range got.WaitItems {
			if got.WaitItems[i].ID == id {
				item = &got.WaitItems[i]
			}
		}
		if !tc.blocked {
			if item != nil || got.WaitCounts.Blocked != 0 {
				t.Fatalf("%s: item %+v, blocked %d; want none", tc.status, item, got.WaitCounts.Blocked)
			}
			continue
		}
		if item == nil || item.Kind != waits.KindSprint || item.Group != waits.GroupBlocked || got.WaitCounts.Blocked != 1 {
			t.Fatalf("%s: item %+v, blocked %d; want one Blocked sprint item %s (all: %+v)", tc.status, item, got.WaitCounts.Blocked, id, got.WaitItems)
		}
	}
}

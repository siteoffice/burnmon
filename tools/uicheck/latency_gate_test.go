package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// p95 is the nearest rank: the ceil(0.95 n)-th smallest value.
func TestLatencyStats(t *testing.T) {
	ten := []int64{500, 100, 1000, 300, 200, 900, 400, 800, 700, 600}
	if p95, max := latencyStats(ten); p95 != 1000 || max != 1000 {
		t.Fatalf("100..1000: p95 %d, max %d; want 1000, 1000", p95, max)
	}
	var twenty []int64
	for v := int64(20); v >= 1; v-- {
		twenty = append(twenty, v*10)
	}
	if p95, max := latencyStats(twenty); p95 != 190 || max != 200 {
		t.Fatalf("10..200: p95 %d, max %d; want the 19th sorted 190, 200", p95, max)
	}
	if twenty[0] != 200 {
		t.Fatalf("latencyStats reordered the caller's slice: %v", twenty)
	}
	if p95, max := latencyStats(nil); p95 != 0 || max != 0 {
		t.Fatalf("no rounds: p95 %d, max %d; want 0, 0", p95, max)
	}
}

// A probe folder without the marker is someone else's: never written, never
// removed.
func TestProbeDirRefusesForeign(t *testing.T) {
	root := t.TempDir()
	dir := latencyProbeDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(dir, "sprint.json")
	if err := os.WriteFile(foreign, []byte(`{"sprint_id":"someone__else__1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := claimProbeDir(root); err == nil {
		t.Fatal("claimProbeDir took a folder without the marker")
	}
	if err := removeProbeDir(dir); err == nil {
		t.Fatal("removeProbeDir removed a folder without the marker")
	}
	if b, err := os.ReadFile(foreign); err != nil || string(b) != `{"sprint_id":"someone__else__1"}` {
		t.Fatalf("the foreign file changed: %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, latencyMarker)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a marker was written into the foreign folder: %v", err)
	}
}

// A fresh probe folder gets the marker before anything else; a folder an
// interrupted run left (marker in it) is taken again; removal takes the
// probe folder only.
func TestProbeDirClaimAndRemove(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sprints"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := claimProbeDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(root, "sprints", "burnmon", "b8-gate-latency-probe") {
		t.Fatalf("probe folder %s", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, latencyMarker)); err != nil {
		t.Fatalf("no marker: %v", err)
	}
	if err := writeProbeSprint(dir, "running", time.Now()); err != nil {
		t.Fatal(err)
	}
	if again, err := claimProbeDir(root); err != nil || again != dir {
		t.Fatalf("claim of a marked leftover: %s, %v", again, err)
	}
	if err := removeProbeDir(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("probe folder still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "sprints", "burnmon")); err != nil {
		t.Fatalf("the project folder went too: %v", err)
	}
}

// A root with no sprints folder is a wrong foreman_root: nothing is made.
func TestProbeDirNeedsSprintsFolder(t *testing.T) {
	root := t.TempDir()
	if _, err := claimProbeDir(root); err == nil {
		t.Fatal("claimProbeDir made a probe under a root without sprints")
	}
	if _, err := os.Stat(filepath.Join(root, "sprints")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sprints was created: %v", err)
	}
}

func TestReadForemanRoot(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) string {
		p := filepath.Join(dir, "burnmon.json")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	got, err := readForemanRoot(write(`{"keep_history":true,"foreman_root":"C:\\example\\foreman"}`))
	if err != nil || got != `C:\example\foreman` {
		t.Fatalf("readForemanRoot = %q, %v", got, err)
	}
	if got, err := readForemanRoot(write(`{"keep_history":true}`)); err == nil {
		t.Fatalf("no foreman_root gave %q, want an error", got)
	}
	if got, err := readForemanRoot(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatalf("a missing file gave %q, want an error", got)
	}
}

// The probe's sprint.json is testdata\latency_probe_sprint.json at its
// stamp. internal\snapshotwriter\latency_probe_test.go proves on that same
// file that the writer makes it a Blocked sprint item, so the two modules
// cannot drift apart.
func TestProbeSprintMatchesFixture(t *testing.T) {
	raw, err := readFixture("latency_probe_sprint.json")
	if err != nil {
		t.Fatal(err)
	}
	var want, got map[string]any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(probeSprintJSON("waiting-on-you", time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("probe sprint.json = %v, fixture %v", got, want)
	}
}

// The case starts like any dev case (applyStart first), from a state with the
// Waits part on, and never reaches its body when the start fails.
func TestLatencyGateStartShowsWaits(t *testing.T) {
	sentinel := errors.New("start stub")
	var got []startState
	stubApplyStart(t, func(_ uintptr, s startState) error {
		got = append(got, s)
		return sentinel
	})
	fn := checks["latency/gate"]
	if fn == nil {
		t.Fatal("latency/gate is not registered")
	}
	if err := fn(0, nil); !errors.Is(err, sentinel) || len(got) != 1 {
		t.Fatalf("err %v after %d applyStart calls; want the stub's error after one", err, len(got))
	}
	s := got[0]
	on := s.Standard["waits"]
	if s.Plain {
		on = s.PlainSet["waits"]
	}
	if !on {
		t.Fatalf("latency/gate starts with the Waits part off: %+v", s)
	}
}

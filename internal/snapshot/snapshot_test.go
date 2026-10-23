package snapshot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/live"
	"burnmon/internal/sysmon"
)

var t0 = time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)

func liveFixture() live.Snapshot {
	return live.Snapshot{
		Sessions: []*live.Session{
			{
				Vendor: "anthropic", SessionID: "s-parent", Agent: "claude-code", Model: "claude-opus",
				Project: `C:\ZND\50_projects\burnmon`, Tokens: 1200, Cost: 2.5,
				LastTurn: t0.Add(-20 * time.Second).Format(time.RFC3339),
				Stage:    "coding", StageSince: t0.Add(-30 * time.Second).Format(time.RFC3339),
				Subagents: []*live.Session{
					{
						Vendor: "anthropic", SessionID: "s-sub", Agent: "claude-code", Tokens: 300, Cost: 0.5,
						LastTurn: t0.Add(-5 * time.Second).Format(time.RFC3339),
						Stage:    "waiting", StageSince: t0.Add(-90 * time.Second).Format(time.RFC3339),
					},
				},
			},
		},
		Chart: []live.Bucket{
			{At: t0.Add(-10 * time.Minute).Format(time.RFC3339), BySession: map[string]live.ClassCounts{"x": {Fresh: 9999}}},
			{At: t0.Add(-2 * time.Minute).Format(time.RFC3339), Cost: 1,
				BySession: map[string]live.ClassCounts{"s-parent": {Fresh: 100, Output: 50, CacheRead: 400, CacheWrite: 50}}},
		},
	}
}

func buildFixture() Snapshot {
	return Build(Input{
		Now:         t0,
		TickSeconds: 5,
		FXUSDEUR:    0.5,
		Live:        liveFixture(),
		Sources:     []Source{{Name: "store", OK: true, ReadAt: t0.Format(time.RFC3339)}, {Name: "sysmon", OK: false, ReadAt: t0.Format(time.RFC3339), Error: "boom"}},
		Processes:   []sysmon.ProcessGroupSample{{Ts: t0.Add(-2 * time.Second), Harness: sysmon.HarnessClaude, CPUPct: 12.5, MemMB: 300, IOBps: 10}},
	})
}

func TestBuildHeader(t *testing.T) {
	s := buildFixture()
	if s.SchemaVersion != SchemaVersion || s.TickSeconds != 5 || s.WrittenAt != t0.Format(time.RFC3339) {
		t.Fatalf("header wrong: %+v", s)
	}
	if len(s.Sources) != 2 || s.Sources[1].Error != "boom" || s.Sources[1].OK {
		t.Fatalf("sources not carried through: %+v", s.Sources)
	}
}

func TestBuildFlattensSessionsWithParentAndAge(t *testing.T) {
	s := buildFixture()
	if len(s.Sessions) != 2 {
		t.Fatalf("want parent and subagent, got %d", len(s.Sessions))
	}
	p, sub := s.Sessions[0], s.Sessions[1]
	if p.ID != "s-parent" || p.ParentID != "" || p.State != "coding" || p.Tokens != 1200 {
		t.Fatalf("parent wrong: %+v", p)
	}
	if sub.ParentID != "s-parent" {
		t.Fatalf("subagent should name its parent: %+v", sub)
	}
	if p.LastActivity != t0.Add(-20*time.Second).Format(time.RFC3339) || p.AgeSeconds != 20 || p.Source != "store" {
		t.Fatalf("activity, age or source wrong: %+v", p)
	}
}

func TestBuildCostCarriesPlainLabel(t *testing.T) {
	p := buildFixture().Sessions[0]
	if p.Cost.Label != CostLabel || CostLabel != "API price equivalent" {
		t.Fatalf("label wrong: %q", p.Cost.Label)
	}
	if p.Cost.USD != 2.5 || p.Cost.EUR != 1.25 {
		t.Fatalf("cost wrong: %+v", p.Cost)
	}
}

func TestBuildWaitsOnYouOnlyForWaitingSessions(t *testing.T) {
	s := buildFixture()
	if len(s.WaitsOnYou) != 1 {
		t.Fatalf("want one wait, got %+v", s.WaitsOnYou)
	}
	w := s.WaitsOnYou[0]
	if w.SessionID != "s-sub" || w.WaitingSeconds != 90 || w.Source != "store" {
		t.Fatalf("wait wrong: %+v", w)
	}
}

func TestBuildBurnCountsOnlyLastFiveMinutes(t *testing.T) {
	b := buildFixture().Burn
	if b.TokensLast5Min != 600 || b.TokensPerMinute != 120 {
		t.Fatalf("burn tokens wrong: %+v", b)
	}
	if b.Cost.Label != CostLabel || b.Cost.USD != 1 || b.Source != "store" {
		t.Fatalf("burn cost wrong: %+v", b)
	}
}

func TestBuildProcessesCarryAge(t *testing.T) {
	ps := buildFixture().Processes
	if len(ps) != 1 || ps[0].Harness != "claude" || ps[0].CPUPct != 12.5 || ps[0].AgeSeconds != 2 || ps[0].Source != "sysmon" {
		t.Fatalf("processes wrong: %+v", ps)
	}
}

func TestEmptyListsEncodeAsArraysNotNull(t *testing.T) {
	s := Build(Input{Now: t0, TickSeconds: 5})
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"sources", "sessions", "processes", "waits_on_you"} {
		if string(m[k]) != "[]" {
			t.Fatalf("%s should be [], got %s", k, m[k])
		}
	}
}

func TestWriteThenReadBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "snapshot.json")
	want := buildFixture()
	if err := Write(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(want)
	b, _ := json.Marshal(got)
	if string(a) != string(b) {
		t.Fatalf("read back differs:\n%s\n%s", a, b)
	}
}

func TestWriteLeavesNoTempFileAndReplacesAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot.json")
	for i := 0; i < 3; i++ {
		if err := Write(path, buildFixture()); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "snapshot.json" {
		t.Fatalf("expected only snapshot.json, got %v", entries)
	}
}

func TestStaleAfterThreeTicks(t *testing.T) {
	s := Build(Input{Now: t0, TickSeconds: 5})
	if s.IsStale(t0.Add(15 * time.Second)) {
		t.Fatal("exactly three ticks old is still fresh")
	}
	if !s.IsStale(t0.Add(16 * time.Second)) {
		t.Fatal("older than three ticks must be stale")
	}
}

func TestReadMissingFileIsNotExist(t *testing.T) {
	_, err := Read(filepath.Join(t.TempDir(), "nope.json"))
	if !os.IsNotExist(err) {
		t.Fatalf("want not-exist error, got %v", err)
	}
}

func TestFixtureMatchesSchema(t *testing.T) {
	got, err := Read(filepath.Join("..", "..", "testdata", "snapshot_example.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != SchemaVersion {
		t.Fatalf("fixture schema_version %d, code %d", got.SchemaVersion, SchemaVersion)
	}
	if len(got.Sessions) == 0 || len(got.Processes) == 0 || len(got.WaitsOnYou) == 0 || len(got.Sources) == 0 {
		t.Fatalf("fixture should exercise every list: %+v", got)
	}
	if got.Sessions[0].Cost.Label != CostLabel {
		t.Fatalf("fixture cost label wrong: %q", got.Sessions[0].Cost.Label)
	}
}

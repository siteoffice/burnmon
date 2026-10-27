package snapshot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burnmon/internal/graph"
	"burnmon/internal/live"
	"burnmon/internal/sysmon"
	"burnmon/internal/waits"
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

func itemsFixture() []waits.Item {
	return []waits.Item{
		{Kind: waits.KindSprint, ID: "sprint:example__s__1", Title: "sprint-example", Project: "example", Detail: "2 tasks parked",
			Since: t0.Add(-10 * time.Minute).Format(time.RFC3339), AgeSeconds: 600, SpecPath: `C:\example\specs\a.md`, Source: "foreman",
			Parked: []waits.Parked{{N: 1, Title: "one", Reason: "needs a decision"}, {N: 3, Title: "three"}}},
		{Kind: waits.KindFinished, ID: "sprint:example__d__1", Title: "done-example", Detail: "test pass, verify pass",
			Since: t0.Add(-5 * time.Hour).Format(time.RFC3339), AgeSeconds: 18000, Source: "foreman"},
	}
}

func buildFixture() Snapshot {
	return Build(Input{
		Now:          t0,
		TickSeconds:  5,
		FXUSDEUR:     0.5,
		Live:         liveFixture(),
		Sources:      []Source{{Name: "store", OK: true, ReadAt: t0.Format(time.RFC3339)}, {Name: "sysmon", OK: false, ReadAt: t0.Format(time.RFC3339), Error: "boom"}},
		Processes:    []sysmon.ProcessGroupSample{{Ts: t0.Add(-2 * time.Second), Harness: sysmon.HarnessClaude, CPUPct: 12.5, MemMB: 300, IOBps: 10}},
		WaitItems:    itemsFixture()[:1],
		OutcomeItems: itemsFixture()[1:],
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
	for _, k := range []string{"sources", "sessions", "processes", "waits_on_you", "nodes", "edges", "wait_items", "outcome_items", "agent_sessions"} {
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
	if len(got.Nodes) == 0 || len(got.Edges) == 0 {
		t.Fatalf("fixture should carry the R6 graph: %d nodes, %d edges", len(got.Nodes), len(got.Edges))
	}
	ids := map[string]bool{}
	for _, n := range got.Nodes {
		ids[n.ID] = true
	}
	for _, e := range got.Edges {
		if !ids[e.From] || !ids[e.To] || e.Basis == "" || e.Source == "" {
			t.Fatalf("fixture edge %+v points outside the graph or lacks basis or source", e)
		}
	}
	if got.Sessions[0].Cost.Label != CostLabel {
		t.Fatalf("fixture cost label wrong: %q", got.Sessions[0].Cost.Label)
	}
}

// R6: the graph goes into the snapshot as nodes[] and edges[], unchanged.
func TestBuildCarriesGraph(t *testing.T) {
	g := graph.Graph{
		Nodes: []graph.Node{{ID: "device:local", Kind: "device", Label: "This PC", Source: "sysmon"}, {ID: "harness:claude", Kind: "harness", Label: "Claude Code", Source: "sysmon"}},
		Edges: []graph.Edge{{From: "harness:claude", To: "device:local", Kind: "runs_on", Basis: "vendor", Source: "store", FirstSeen: "x", LastSeen: "y"}},
	}
	s := Build(Input{Now: t0, TickSeconds: 5, Graph: g})
	if len(s.Nodes) != 2 || len(s.Edges) != 1 || s.Edges[0].Kind != "runs_on" {
		t.Fatalf("graph not carried: %+v %+v", s.Nodes, s.Edges)
	}
	if s.SchemaVersion != 1 {
		t.Fatalf("the graph is additive; schema_version must stay 1, got %d", s.SchemaVersion)
	}
}

func TestBuildCarriesItems(t *testing.T) {
	it := itemsFixture()
	s := Build(Input{Now: t0, TickSeconds: 5, WaitItems: it[:1], OutcomeItems: it[1:]})
	if len(s.WaitItems) != 1 || len(s.OutcomeItems) != 1 {
		t.Fatalf("items not carried: %+v %+v", s.WaitItems, s.OutcomeItems)
	}
	a, _ := json.Marshal(it[0])
	b, _ := json.Marshal(s.WaitItems[0])
	if string(a) != string(b) || len(s.WaitItems[0].Parked) != 2 {
		t.Fatalf("wait item changed:\n%s\n%s", a, b)
	}
	if s.OutcomeItems[0].ID != "sprint:example__d__1" {
		t.Fatalf("outcome wrong: %+v", s.OutcomeItems[0])
	}
	if s.SchemaVersion != 1 {
		t.Fatalf("items are additive; schema_version must stay 1, got %d", s.SchemaVersion)
	}
}

func TestFixtureHasItemsOfEveryKind(t *testing.T) {
	got, err := Read(filepath.Join("..", "..", "testdata", "snapshot_example.json"))
	if err != nil {
		t.Fatal(err)
	}
	written, _ := time.Parse(time.RFC3339, got.WrittenAt)
	check := func(name string, items []waits.Item, kinds []string, newestFirst bool) {
		seen := map[string]bool{}
		var prevAge int64 = -1
		prevID := ""
		for _, it := range items {
			seen[it.Kind] = true
			if it.Kind == "" || it.ID == "" || it.Title == "" || it.Since == "" || it.Source == "" {
				t.Fatalf("%s item lacks a required field: %+v", name, it)
			}
			since, err := time.Parse(time.RFC3339, it.Since)
			if err != nil {
				t.Fatalf("%s item since: %v", name, err)
			}
			if it.AgeSeconds != int64(written.Sub(since)/time.Second) {
				t.Fatalf("%s item %s age %d does not match written_at minus since", name, it.ID, it.AgeSeconds)
			}
			if newestFirst && (it.AgeSeconds < prevAge || (it.AgeSeconds == prevAge && it.ID < prevID)) {
				t.Fatalf("%s not sorted newest first at %s", name, it.ID)
			}
			prevAge, prevID = it.AgeSeconds, it.ID
			for _, s := range []string{it.Title, it.Detail, it.SpecPath, it.ResultPath, it.ID} {
				for _, bad := range []string{"ZND", "10_holding", "Valona"} {
					if strings.Contains(s, bad) {
						t.Fatalf("%s item %s holds %q", name, it.ID, bad)
					}
				}
			}
		}
		for _, k := range kinds {
			if !seen[k] {
				t.Fatalf("%s lacks kind %s", name, k)
			}
		}
	}
	check("wait_items", got.WaitItems, []string{"gate", "sprint", "failed_check", "question", "prompt", "session", "your_turn", "close_out", "can_close"}, false)
	check("outcome_items", got.OutcomeItems, []string{"finished", "failed_check"}, true)
}

// B6: every wait item has a group and holds, wait_counts is the one count of
// the list, and the list is in ranking order (Blocked, Your turn, Done; inside
// Blocked the item that holds most first, then the oldest; ties by id).
func TestFixtureWaitItemsGroupedCountedAndRanked(t *testing.T) {
	got, err := Read(filepath.Join("..", "..", "testdata", "snapshot_example.json"))
	if err != nil {
		t.Fatal(err)
	}
	tier := map[string]int{waits.GroupBlocked: 0, waits.GroupYourTurn: 1, waits.GroupDone: 2}
	for i, it := range got.WaitItems {
		if _, ok := tier[it.Group]; !ok {
			t.Fatalf("item %s has group %q", it.ID, it.Group)
		}
		if it.Holds < 1 {
			t.Fatalf("item %s holds %d", it.ID, it.Holds)
		}
		if i == 0 {
			continue
		}
		p := got.WaitItems[i-1]
		switch {
		case tier[p.Group] != tier[it.Group]:
			if tier[p.Group] > tier[it.Group] {
				t.Fatalf("%s (%s) is listed before %s (%s)", p.ID, p.Group, it.ID, it.Group)
			}
		case it.Group == waits.GroupBlocked && p.Holds != it.Holds:
			if p.Holds < it.Holds {
				t.Fatalf("%s holds %d, listed before %s which holds %d", p.ID, p.Holds, it.ID, it.Holds)
			}
		case p.AgeSeconds != it.AgeSeconds:
			if p.AgeSeconds < it.AgeSeconds {
				t.Fatalf("%s is older-first violated against %s", p.ID, it.ID)
			}
		case p.ID > it.ID:
			t.Fatalf("tie not by id: %s before %s", p.ID, it.ID)
		}
	}
	if got.WaitCounts != waits.CountOf(got.WaitItems) {
		t.Fatalf("wait_counts %+v, list counts %+v", got.WaitCounts, waits.CountOf(got.WaitItems))
	}
	if got.WaitCounts.Blocked == 0 || got.WaitCounts.YourTurn == 0 || got.WaitCounts.Done == 0 {
		t.Fatalf("fixture should exercise all three groups: %+v", got.WaitCounts)
	}
}

func TestWaitCountsAlwaysPresent(t *testing.T) {
	b, err := json.Marshal(Build(Input{Now: t0, TickSeconds: 5}))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if string(m["wait_counts"]) != `{"blocked":0,"your_turn":0,"done":0}` {
		t.Fatalf("wait_counts = %s", m["wait_counts"])
	}
	items := []waits.Item{{ID: "a", Group: waits.GroupBlocked}, {ID: "b", Group: waits.GroupBlocked}, {ID: "c", Group: waits.GroupYourTurn}, {ID: "d", Group: waits.GroupDone}}
	if got := Build(Input{Now: t0, TickSeconds: 5, WaitItems: items}).WaitCounts; got != (waits.Counts{Blocked: 2, YourTurn: 1, Done: 1}) {
		t.Fatalf("wait_counts = %+v", got)
	}
}

func TestBuildCarriesAgentSessions(t *testing.T) {
	in := []AgentSession{{Kind: "background", ID: "ab12", PID: 1, StartedAt: 5}, {Kind: "interactive", SessionID: "example-parent", StartedAt: 6}}
	s := Build(Input{Now: t0, TickSeconds: 5, AgentSessions: in})
	if len(s.AgentSessions) != 2 || s.AgentSessions[0].ID != "ab12" || s.AgentSessions[1].SessionID != "example-parent" {
		t.Fatalf("agent sessions not carried: %+v", s.AgentSessions)
	}
	if s.SchemaVersion != 1 {
		t.Fatalf("agent_sessions is additive; schema_version must stay 1, got %d", s.SchemaVersion)
	}
}

func TestFixtureHasAgentSessions(t *testing.T) {
	got, err := Read(filepath.Join("..", "..", "testdata", "snapshot_example.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.AgentSessions) != 3 {
		t.Fatalf("agent_sessions = %+v", got.AgentSessions)
	}
	a, b, c := got.AgentSessions[0], got.AgentSessions[1], got.AgentSessions[2]
	if a.Kind != "background" || a.ID != "ab12" || a.SessionID != "" {
		t.Errorf("first entry: %+v", a)
	}
	if b.Kind != "background" || b.ID != "cd34" || b.SessionID != "example-sub" {
		t.Errorf("second entry: %+v", b)
	}
	if c.Kind != "interactive" || c.ID != "" || c.SessionID != "example-parent" {
		t.Errorf("third entry: %+v", c)
	}
	waitIDs := map[string]bool{}
	for _, it := range got.WaitItems {
		waitIDs[it.ID] = true
	}
	for _, e := range got.AgentSessions {
		if e.Kind == "background" && e.Status == "waiting" {
			id := "agentview:" + e.ID
			if e.SessionID != "" {
				id = "session:anthropic:" + e.SessionID
			}
			if !waitIDs[id] {
				t.Errorf("waiting agent session %s has no wait item %s", e.ID, id)
			}
		}
	}
	for _, g := range []string{"gate:example__flow__1:G4", "gate:example__flow__2:G2"} {
		if !waitIDs[g] {
			t.Errorf("wait_items lacks %s", g)
		}
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "snapshot_example.json"))
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(top["agent_sessions"], &entries); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if _, bad := e["cwd"]; bad {
			t.Errorf("agent_sessions entry carries cwd: %v", e)
		}
		if _, bad := e["name"]; bad {
			t.Errorf("agent_sessions entry carries name: %v", e)
		}
		for k, v := range e {
			for _, bad := range []string{"ZND", "10_holding", "Valona"} {
				if strings.Contains(string(v), bad) {
					t.Errorf("agent_sessions %s holds %q", k, bad)
				}
			}
		}
	}
}

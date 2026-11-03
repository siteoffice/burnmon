//go:build windows

package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burnmon/internal/graph"
	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/snapshot"
	"burnmon/internal/store"
)

func frameExplain(p agentMapPayload, label string) string {
	for _, f := range p.Layout.Frames {
		if f.Label == label {
			return f.Explain
		}
	}
	return "<no frame " + label + ">"
}

// OF23: with an empty foreman_root the live map view drops the Foreman clause
// from the help sentence of an unlinked folder; with a root it keeps it.
func TestAgentMapLiveNoForemanFromEmptyRoot(t *testing.T) {
	now := time.Date(2026, 10, 23, 10, 0, 7, 0, time.UTC)
	snapPath := filepath.Join(t.TempDir(), "snapshot.json")
	s := snapshot.Snapshot{SchemaVersion: 1, WrittenAt: now.Add(-7 * time.Second).UTC().Format(time.RFC3339), TickSeconds: 5,
		Nodes: []graph.Node{{ID: "session:anthropic:s1", Kind: graph.KindSession, Label: "alpha", Attrs: map[string]string{"state": "coding"}}}}
	if err := snapshot.Write(snapPath, s); err != nil {
		t.Fatal(err)
	}
	empty, err := agentMapFor(agentMapEnv{DataDir: t.TempDir(), SnapPath: snapPath}, agentMapRequest{Source: "live"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := frameExplain(empty, "alpha"); got != `Sessions in the folder "alpha".` {
		t.Errorf("live, empty root: explain %q", got)
	}
	rooted, err := agentMapFor(agentMapEnv{DataDir: t.TempDir(), SnapPath: snapPath, ForemanRoot: t.TempDir()}, agentMapRequest{Source: "live"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := frameExplain(rooted, "alpha"); !strings.Contains(got, "No Foreman sprint or flow is linked.") {
		t.Errorf("live, with a root: explain %q lost the Foreman clause", got)
	}
}

// OF23: the same for the history map view.
func TestAgentMapHistoryNoForemanFromEmptyRoot(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.UpsertEvents([]schema.Event{
		{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "S1", RequestID: "r1", Model: "m", Project: `C:\w\repo`, At: now.Add(-3 * time.Hour), Input: 1, Output: 1},
	}); err != nil {
		t.Fatal(err)
	}
	empty, err := agentMapFor(agentMapEnv{DataDir: t.TempDir(), St: st}, agentMapRequest{Source: "history", Hours: 24}, now)
	if err != nil {
		t.Fatal(err)
	}
	label := ""
	for _, f := range empty.Layout.Frames {
		label = f.Label
	}
	if got := frameExplain(empty, label); label == "" || strings.Contains(got, "Foreman") {
		t.Errorf("history, empty root: frame %q explain %q", label, got)
	}
	rooted, err := agentMapFor(agentMapEnv{DataDir: t.TempDir(), St: st, ForemanRoot: t.TempDir()}, agentMapRequest{Source: "history", Hours: 24}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := frameExplain(rooted, label); !strings.Contains(got, "No Foreman sprint or flow is linked.") {
		t.Errorf("history, with a root: explain %q lost the Foreman clause", got)
	}
}

// OF23: the small overview in the sections payload names its group of
// unlinked sessions "Sessions" with no foreman_root, and keeps "No Foreman
// work" with one.
func TestSectionsOverviewNoForemanFromAvailability(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 23, 10, 0, 7, 0, time.UTC)
	snapPath := filepath.Join(dir, "snapshot.json")
	s := snapshot.Snapshot{SchemaVersion: 1, WrittenAt: now.Add(-7 * time.Second).UTC().Format(time.RFC3339), TickSeconds: 5,
		Nodes: []graph.Node{{ID: "session:anthropic:s1", Kind: graph.KindSession, Label: "alpha", Attrs: map[string]string{"state": "coding"}}}}
	if err := snapshot.Write(snapPath, s); err != nil {
		t.Fatal(err)
	}
	pyLook := func(string) (string, error) { return `C:\example\python.exe`, nil }
	dbPath := filepath.Join(dir, "store", "burnmon.db")
	label := func(root string) string {
		svc := newControlServiceWith(pricing.Config{ForemanRoot: root}, dir, snapPath, dbPath, pyLook)
		p := sectionsPayloadWith(dir, snapPath, now, svc.Availability, nil)
		if p.AgentMap == nil || len(p.AgentMap.Groups) != 1 {
			t.Fatalf("root %q: overview %+v", root, p.AgentMap)
		}
		return p.AgentMap.Groups[0].Label
	}
	if got := label(""); got != "Sessions" {
		t.Errorf("empty root: overview group %q, want Sessions", got)
	}
	if got := label(`C:\example\foreman`); got != "No Foreman work" {
		t.Errorf("with a root: overview group %q, want No Foreman work", got)
	}
}

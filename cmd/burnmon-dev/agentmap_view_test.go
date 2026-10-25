//go:build windows

package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burnmon/internal/agentmap"
	"burnmon/internal/schema"
	"burnmon/internal/sections"
	"burnmon/internal/store"
)

func TestMapHoursClamp(t *testing.T) {
	for in, want := range map[int]int{-5: 24, 0: 24, 1: 1, 24: 24, 168: 168, 720: 720, 721: 720, 100000: 720} {
		if got := clampMapHours(in); got != want {
			t.Errorf("clampMapHours(%d) = %d, want %d", in, got, want)
		}
	}
}

func boxIn(l agentmap.Layout, id string) (agentmap.Box, bool) {
	for _, b := range l.Boxes {
		if b.ID == id {
			return b, true
		}
	}
	return agentmap.Box{}, false
}

func factIn(b agentmap.Box, label string) string {
	for _, f := range b.Facts {
		if f.Label == label {
			return f.Value
		}
	}
	return ""
}

// The live map is the snapshot file's graph, with the snapshot's own sessions
// as evidence (tokens, the labelled price, source and age). Plain Mode, kept
// in the view file, changes the words.
func TestAgentMapLiveFromFrozenExample(t *testing.T) {
	dir := t.TempDir()
	env := agentMapEnv{DataDir: dir, SnapPath: filepath.Join("..", "..", "testdata", "snapshot_example.json")}
	now := time.Date(2026, 10, 23, 10, 0, 7, 0, time.UTC)

	p, err := agentMapFor(env, agentMapRequest{Source: "live"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Source != "live" || p.Status != "ok" || p.AgeSeconds != 7 || p.Note != "" {
		t.Fatalf("header %+v", p)
	}
	s, ok := boxIn(p.Layout, "session:anthropic:example-parent")
	if !ok {
		t.Fatalf("no session box in %d boxes", len(p.Layout.Boxes))
	}
	if factIn(s, "Tokens") != "1,200" || factIn(s, "API price equivalent") != "\u20ac2.30" || factIn(s, "Model") != "claude-opus-5-5" {
		t.Errorf("evidence facts %+v", s.Facts)
	}
	if b, _ := boxIn(p.Layout, "session:anthropic:example-sub"); b.Pill != "done" {
		t.Errorf("standard pill %q: a subagent that ended its turn is done", b.Pill)
	}

	if err := saveSections(dir, sections.Default().WithPlain(true)); err != nil {
		t.Fatal(err)
	}
	q, err := agentMapFor(env, agentMapRequest{Source: "live"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := boxIn(q.Layout, "session:anthropic:example-sub"); b.Pill != "finished" {
		t.Errorf("plain pill %q", b.Pill)
	}
}

// A stale or missing snapshot draws no old boxes and says BurnMon is not running.
func TestAgentMapLiveNotRunning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join("..", "..", "testdata", "snapshot_example.json")
	now := time.Date(2026, 10, 23, 10, 0, 7, 0, time.UTC)

	stale, err := agentMapFor(agentMapEnv{DataDir: dir, SnapPath: path}, agentMapRequest{Source: "live"}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if stale.Status != "stale" || len(stale.Layout.Boxes) != 0 || !strings.Contains(stale.Note, "not running") {
		t.Errorf("stale: status %q, %d boxes, note %q", stale.Status, len(stale.Layout.Boxes), stale.Note)
	}
	missing, err := agentMapFor(agentMapEnv{DataDir: dir, SnapPath: filepath.Join(dir, "none.json")}, agentMapRequest{Source: "live"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if missing.Status != "missing" || len(missing.Layout.Boxes) != 0 || !strings.Contains(missing.Note, "not running") {
		t.Errorf("missing: status %q, note %q", missing.Status, missing.Note)
	}
}

// History comes from the store, so a subagent shows next to its parent after
// the session files are gone. Tokens are not kept there and read "unknown".
func TestAgentMapHistoryFromStore(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	at := now.Add(-3 * time.Hour)
	old := now.Add(-100 * time.Hour)
	proj := `C:\w\repo`
	if err := st.UpsertEvents([]schema.Event{
		{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "S1", RequestID: "r1", Model: "m", Project: proj, At: at, Input: 1, Output: 1},
		{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: "agent-a1", ParentID: "S1", RequestID: "r2", Model: "m", Project: proj, At: at, Input: 1, Output: 1},
		{Vendor: "openai", Agent: "codex", Surface: "cli", SessionID: "OLD", RequestID: "r3", Model: "m", Project: proj, At: old, Input: 1, Output: 1},
	}); err != nil {
		t.Fatal(err)
	}
	env := agentMapEnv{DataDir: t.TempDir(), St: st}

	p, err := agentMapFor(env, agentMapRequest{Source: "history", Hours: 24}, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Hours != 24 || p.Source != "history" || p.Status != "ok" {
		t.Errorf("payload header %+v", p)
	}
	if p.Layout.Counts.Sessions != 1 || p.Layout.Counts.Subagents != 1 {
		t.Fatalf("24 h counts %+v: the old session is outside the window", p.Layout.Counts)
	}
	var sub agentmap.Box
	for _, b := range p.Layout.Boxes {
		if b.Kind == "sub" {
			sub = b
		}
	}
	if sub.ID == "" || sub.Column != 2 {
		t.Errorf("subagent box %+v: want one in the subagent column", sub)
	}
	if factIn(sub, "Tokens") != "unknown" {
		t.Errorf("history tokens %q, want unknown", factIn(sub, "Tokens"))
	}
	if !strings.Contains(p.Note, "history") {
		t.Errorf("note %q does not say what history lacks", p.Note)
	}
	wide, err := agentMapFor(env, agentMapRequest{Source: "history", Hours: 168}, now)
	if err != nil {
		t.Fatal(err)
	}
	if wide.Layout.Counts.Sessions != 2 {
		t.Errorf("168 h counts %+v, want both sessions", wide.Layout.Counts)
	}
}

func TestAgentMapRefusesOtherSources(t *testing.T) {
	for _, src := range []string{"", "tomorrow", "LIVE"} {
		if _, err := agentMapFor(agentMapEnv{DataDir: t.TempDir()}, agentMapRequest{Source: src}, time.Now()); err == nil {
			t.Errorf("source %q: want an error", src)
		}
	}
}

// A Foreman folder that cannot be read is said plainly, and the map still
// draws from the store.
func TestAgentMapHistoryBadForemanRoot(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	env := agentMapEnv{DataDir: t.TempDir(), St: st, ForemanRoot: filepath.Join(t.TempDir(), "no-such-folder")}
	p, err := agentMapFor(env, agentMapRequest{Source: "history", Hours: 24}, time.Now())
	if err != nil {
		t.Fatalf("a missing Foreman folder must not fail the map: %v", err)
	}
	if !strings.Contains(p.Note, "Foreman") {
		t.Errorf("no note about Foreman state: %q", p.Note)
	}
}

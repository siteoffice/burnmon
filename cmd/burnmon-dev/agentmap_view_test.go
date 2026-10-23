//go:build windows

package main

import (
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/schema"
	"burnmon/internal/store"
)

func TestMapHoursClamp(t *testing.T) {
	for in, want := range map[int]int{-5: 24, 0: 24, 1: 1, 24: 24, 168: 168, 720: 720, 721: 720, 100000: 720} {
		if got := clampMapHours(in); got != want {
			t.Errorf("clampMapHours(%d) = %d, want %d", in, got, want)
		}
	}
}

// History comes from the store, so a subagent shows under its parent after
// the session files are gone.
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

	p, err := agentMapHistory(st, "", 24, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Hours != 24 || p.Source != "history" {
		t.Errorf("payload header %+v", p)
	}
	if len(p.View.Groups) != 1 || len(p.View.Groups[0].Cards) != 1 {
		t.Fatalf("groups %+v: want one top card, the old session is outside 24 h", p.View.Groups)
	}
	top := p.View.Groups[0].Cards[0]
	if len(top.Children) != 1 || !top.Children[0].Subagent {
		t.Errorf("subagent not nested: %+v", top)
	}
	wide, err := agentMapHistory(st, "", 168, now)
	if err != nil {
		t.Fatal(err)
	}
	if wide.View.Counts.Sessions != 2 {
		t.Errorf("168 h counts %+v, want both sessions", wide.View.Counts)
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
	p, err := agentMapHistory(st, filepath.Join(t.TempDir(), "no-such-folder"), 24, time.Now())
	if err != nil {
		t.Fatalf("a missing Foreman folder must not fail the map: %v", err)
	}
	if p.Note == "" {
		t.Error("no note about Foreman state")
	}
}

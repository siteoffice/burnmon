package snapshotwriter_test

import (
	"testing"
	"time"

	"burnmon/internal/schema"
	"burnmon/internal/snapshotwriter"
)

func TestTokensSinceFromEvents(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	one := int64(1)
	events := []schema.Event{
		{Vendor: "anthropic", SessionID: "s1", At: now.Add(-20 * time.Minute), Input: 60, Output: 30, CacheRead: &one, CacheWrite: &one, Reasoning: &one, Agent: "claude-code"},
		{Vendor: "anthropic", SessionID: "s1", At: now.Add(-2 * time.Minute), Input: 30, Output: 10, Agent: "claude-code"},
		{Vendor: "anthropic", SessionID: "other", At: now.Add(-2 * time.Minute), Input: 999, Agent: "claude-code"},
	}
	f := snapshotwriter.TokensSince(events, now)
	ids := []string{"session:anthropic:s1"}
	if tok, partial := f(ids, now.Add(-5*time.Minute)); tok != 40 || partial {
		t.Fatalf("since 5 min = (%d, %v), want (40, false)", tok, partial)
	}
	if tok, partial := f(ids, now.Add(-45*time.Minute)); tok != 133 || !partial {
		t.Fatalf("since 45 min = (%d, %v), want (133, true)", tok, partial)
	}
}

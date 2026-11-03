package snapshot

import (
	"encoding/json"
	"strings"
	"testing"

	"burnmon/internal/live"
)

// OF28: the Sessions table reads the snapshot, so a cloud session carries its
// data-age label there too, and a local session carries no such key.
func TestBuildCarriesCloudAge(t *testing.T) {
	const label = "cloud data 3 min old"
	in := Input{Now: t0, TickSeconds: 5, Live: live.Snapshot{Sessions: []*live.Session{
		{Vendor: "anthropic", SessionID: "c-1", Agent: "cowork-cloud", CloudAge: label, Stage: "coding"},
		{Vendor: "anthropic", SessionID: "l-1", Agent: "claude-code", Stage: "coding"},
	}}}
	s := Build(in)
	if len(s.Sessions) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(s.Sessions))
	}
	if s.Sessions[0].CloudAge != label {
		t.Errorf("cloud session CloudAge %q, want %q", s.Sessions[0].CloudAge, label)
	}
	if s.SchemaVersion != 1 {
		t.Errorf("schema_version %d, want 1", s.SchemaVersion)
	}
	b, err := json.Marshal(s.Sessions[1])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "cloud_age") {
		t.Errorf("local session JSON has a cloud_age key: %s", b)
	}
}

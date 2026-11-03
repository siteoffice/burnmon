package live

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"burnmon/internal/schema"
)

func TestCloudAgeLabel(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{0, "cloud data under a minute old"},
		{59 * time.Second, "cloud data under a minute old"},
		{60 * time.Second, "cloud data 1 min old"},
		{3*time.Minute + 10*time.Second, "cloud data 3 min old"},
		{2 * time.Hour, "cloud data 120 min old"},
		{-5 * time.Second, "cloud data under a minute old"},
	} {
		if got := CloudAgeLabel(c.d); got != c.want {
			t.Errorf("CloudAgeLabel(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

// TestCloudAgeUsesNewestRequest: the age is measured from the newest turn,
// whatever order the rows arrive in.
func TestCloudAgeUsesNewestRequest(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	var events []schema.Event
	for i, sec := range []int{400, 200, 300} {
		events = append(events, schema.Event{Vendor: "anthropic", Agent: "cowork-cloud", Surface: "cloud",
			SessionID: "cloud-sess", RequestID: string(rune('a' + i)), Model: "claude-sonnet-5",
			At: now.Add(time.Duration(-sec) * time.Second), Input: 1000, Output: 20})
	}
	snap := BuildSnapshot(events, testConfig(), now)
	if len(snap.Sessions) != 1 {
		t.Fatalf("%d sessions, want 1", len(snap.Sessions))
	}
	want := CloudAgeLabel(200 * time.Second)
	if got := snap.Sessions[0].CloudAge; got != want {
		t.Errorf("CloudAge %q, want %q", got, want)
	}
}

func TestNonCloudSessionHasNoCloudAge(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	events := []schema.Event{{Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
		SessionID: "cc-sess", RequestID: "a", Model: "claude-sonnet-5",
		At: now.Add(-10 * time.Second), Input: 1000, Output: 20}}
	snap := BuildSnapshot(events, testConfig(), now)
	if len(snap.Sessions) != 1 {
		t.Fatalf("%d sessions, want 1", len(snap.Sessions))
	}
	if got := snap.Sessions[0].CloudAge; got != "" {
		t.Errorf("CloudAge %q, want empty", got)
	}
	b, err := json.Marshal(snap.Sessions[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "cloud_age") {
		t.Errorf("JSON has cloud_age key: %s", b)
	}
}

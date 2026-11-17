package vendorstrip

import (
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/cloudpull"
	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/store"
)

func pullStripFixture(t *testing.T, cfg *pricing.Config, status cloudpull.Status) Payload {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	oldLast, oldStatus := cloudLastMeasured, cloudPullStatus
	cloudLastMeasured = func() (time.Time, bool) { return time.Date(2026, 11, 12, 16, 43, 0, 0, time.UTC), true }
	cloudPullStatus = func() cloudpull.Status { return status }
	defer func() { cloudLastMeasured, cloudPullStatus = oldLast, oldStatus }()
	ev := []schema.Event{{Vendor: "anthropic", Agent: "cowork-cloud", SessionID: "session_A", RequestID: "r1",
		At: time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC), Model: "m", Input: 700}}
	if err := st.UpsertEvents(ev); err != nil {
		t.Fatal(err)
	}
	p, err := Build(st, cfg, time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// Switch off: today's marker stays, and the payload says the switch is off.
func TestCloudPullOff(t *testing.T) {
	p := pullStripFixture(t, &pricing.Config{}, cloudpull.Status{OK: true})
	c := rowOf(p, "cowork-cloud")
	// The last drop row is 2026-11-12: today is not measured, the month partial.
	if c.TodayNote != NoteNotMeasured || c.MonthNote != NotePartial || p.CloudPull != CloudPullOff {
		t.Fatalf("notes %q/%q, cloud_pull %q; want not_measured/partial, off", c.TodayNote, c.MonthNote, p.CloudPull)
	}
	if want := "measured until " + p.CloudMeasuredUntil + ", later cloud sessions are not measured. Turn on Read Cowork (Cloud) sessions in Settings to read them."; p.CloudTip != want {
		t.Fatalf("tip = %q, want %q", p.CloudTip, want)
	}
	if q := pullStripFixture(t, nil, cloudpull.Status{OK: true}); q.CloudPull != CloudPullOff {
		t.Fatalf("nil config: cloud_pull %q, want off", q.CloudPull)
	}
}

// Switch on and the last pass good: plain numbers, no note anywhere.
func TestCloudPullOnGood(t *testing.T) {
	p := pullStripFixture(t, &pricing.Config{CloudPull: true}, cloudpull.Status{OK: true})
	for _, r := range append(append([]Row{}, p.Rows...), p.Total) {
		if r.TodayNote != "" || r.WeekNote != "" || r.MonthNote != "" {
			t.Errorf("row %q carries notes with the pull on and good: %+v", r.Agent, r)
		}
	}
	if p.CloudPull != CloudPullOK || p.CloudReason != "" || p.CloudTip != "" {
		t.Fatalf("cloud_pull %q reason %q", p.CloudPull, p.CloudReason)
	}
	if c := rowOf(p, "cowork-cloud"); c.Today != 700 {
		t.Fatalf("today = %d, want 700", c.Today)
	}
}

// Switch on and failing: not measured on every period, with the reason.
func TestCloudPullOnFailed(t *testing.T) {
	p := pullStripFixture(t, &pricing.Config{CloudPull: true}, cloudpull.Status{Reason: "Claude Code is not signed in"})
	c := rowOf(p, "cowork-cloud")
	if c.TodayNote != NoteNotMeasured || c.WeekNote != NoteNotMeasured || c.MonthNote != NoteNotMeasured {
		t.Fatalf("notes %q %q %q, want not_measured on all", c.TodayNote, c.WeekNote, c.MonthNote)
	}
	if p.CloudTip != "Cowork (Cloud) not measured: Claude Code is not signed in" {
		t.Fatalf("tip = %q", p.CloudTip)
	}
	if p.CloudPull != CloudPullFailed || p.CloudReason != "Claude Code is not signed in" {
		t.Fatalf("cloud_pull %q reason %q", p.CloudPull, p.CloudReason)
	}
}

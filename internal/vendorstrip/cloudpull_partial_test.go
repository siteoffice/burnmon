package vendorstrip

import (
	"testing"
	"time"

	"burnmon/internal/cloudpull"
	"burnmon/internal/pricing"
)

// Some cloud sessions read, others failing: numbers with the partial mark on
// every period (today's 0 included) and a tip that counts the unread ones.
func TestCloudPullPartialMarksEveryCell(t *testing.T) {
	reason := "Couldn't load the history of cloud session session_X. Check it at https://claude.ai/code/session_X?from=cli"
	p := pullStripFixture(t, &pricing.Config{CloudPull: true}, cloudpull.Status{OK: true, Unread: 31, UnreadReason: reason})
	c := rowOf(p, "cowork-cloud")
	if c.TodayNote != NotePartial || c.WeekNote != NotePartial || c.MonthNote != NotePartial {
		t.Fatalf("notes %q %q %q, want partial on all", c.TodayNote, c.WeekNote, c.MonthNote)
	}
	if p.Total.TodayNote != NotePartial || p.Total.WeekNote != NotePartial || p.Total.MonthNote != NotePartial {
		t.Fatalf("total notes %+v, want partial on all", p.Total)
	}
	if c.Today != 700 {
		t.Fatalf("today = %d, the number must stay", c.Today)
	}
	if want := "Cowork (Cloud): 31 sessions are not read. Newest failure: " + reason; p.CloudTip != want {
		t.Fatalf("tip = %q, want %q", p.CloudTip, want)
	}
	if p.CloudPull != CloudPullPartial {
		t.Fatalf("cloud_pull = %q", p.CloudPull)
	}
	one := pullStripFixture(t, &pricing.Config{CloudPull: true}, cloudpull.Status{OK: true, Unread: 1, UnreadReason: "x"})
	if want := "Cowork (Cloud): 1 session is not read. Newest failure: x"; one.CloudTip != want {
		t.Fatalf("tip = %q, want %q", one.CloudTip, want)
	}
}

// Some sessions unread and the newest good copy older than the period: that
// period holds no measured cloud usage, so it says not measured, not "0+". The
// period that holds the newest copy keeps its number with the +. The total
// stays a number with the + on every period the cloud cell is marked.
func TestCloudPullUnreadAndOldCopyIsNotMeasuredPerPeriod(t *testing.T) {
	// Now in the fixture is 2026-11-17 09:00 UTC (a Tuesday); the newest copy is from Sunday the 15th.
	p := pullStripFixture(t, &pricing.Config{CloudPull: true}, cloudpull.Status{
		OK: true, Unread: 86, UnreadReason: "x",
		LastPulledAt: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC),
	})
	c := rowOf(p, "cowork-cloud")
	if c.TodayNote != NoteNotMeasured || c.WeekNote != NoteNotMeasured || c.MonthNote != NotePartial {
		t.Fatalf("notes %q %q %q, want not_measured, not_measured, partial", c.TodayNote, c.WeekNote, c.MonthNote)
	}
	if p.Total.TodayNote != NotePartial || p.Total.WeekNote != NotePartial || p.Total.MonthNote != NotePartial {
		t.Fatalf("total notes %+v, want partial on all", p.Total)
	}
	if p.CloudPull != CloudPullPartial || p.CloudTip == "" {
		t.Fatalf("cloud_pull %q, tip %q", p.CloudPull, p.CloudTip)
	}
}

// Nothing read: the row stays "not measured" with the CLI's reason.
func TestCloudPullNothingReadStaysNotMeasured(t *testing.T) {
	p := pullStripFixture(t, &pricing.Config{CloudPull: true}, cloudpull.Status{Reason: "Couldn't load the history"})
	c := rowOf(p, "cowork-cloud")
	if c.TodayNote != NoteNotMeasured || p.CloudPull != CloudPullFailed {
		t.Fatalf("note %q, cloud_pull %q", c.TodayNote, p.CloudPull)
	}
}

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

// With the reasons counted, the tip says how many sessions fail for each, in
// plain words, and for sessions the service does not find it gives a page to
// open in the browser to compare (2026-11-23: the page loaded, the CLI said not
// found). Without reasons the old "Newest failure" tip stays.
func TestCloudPullTipCountsEachReason(t *testing.T) {
	p := pullStripFixture(t, &pricing.Config{CloudPull: true}, cloudpull.Status{
		OK: true, Unread: 88, UnreadReason: "ignored when reasons are present",
		Reasons: []cloudpull.Reason{
			{Kind: cloudpull.ReasonNotFound, N: 83, Example: "session_0131Y29YQutk7UuzNFk8CJJS"},
			{Kind: cloudpull.ReasonCheckout, N: 3, Example: "session_B"},
			{Kind: cloudpull.ReasonOther, N: 2, Example: "session_C", Text: "network down"},
		},
	})
	want := "Cowork (Cloud): 88 sessions are not read: 83 are not found by the Claude Code service for your login, " +
		"3 need a checkout of their repo, 2 failed: network down. " +
		"Open one in the browser to compare: https://claude.ai/code/session_0131Y29YQutk7UuzNFk8CJJS"
	if p.CloudTip != want {
		t.Fatalf("tip = %q\nwant  %q", p.CloudTip, want)
	}
	// No page link when the biggest reason is not "not found".
	q := pullStripFixture(t, &pricing.Config{CloudPull: true}, cloudpull.Status{
		OK: true, Unread: 1,
		Reasons: []cloudpull.Reason{{Kind: cloudpull.ReasonCheckout, N: 1, Example: "session_B"}},
	})
	if want := "Cowork (Cloud): 1 session is not read: 1 needs a checkout of their repo."; q.CloudTip != want {
		t.Fatalf("tip = %q, want %q", q.CloudTip, want)
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

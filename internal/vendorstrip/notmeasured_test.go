package vendorstrip

import (
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/schema"
	"burnmon/internal/store"
)

func cloudStripFixture(t *testing.T, last time.Time, haveLast bool) Payload {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	old := cloudLastMeasured
	cloudLastMeasured = func() (time.Time, bool) { return last, haveLast }
	defer func() { cloudLastMeasured = old }()

	// Cloud rows from before the break, plus a local row today.
	at := time.Date(2026, 11, 12, 16, 0, 0, 0, time.UTC)
	ev := []schema.Event{
		{Vendor: "anthropic", Agent: "cowork-cloud", SessionID: "c1", RequestID: "r1", At: at, Model: "m", Input: 700},
		{Vendor: "anthropic", Agent: "claude-code", SessionID: "l1", RequestID: "r2",
			At: time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC), Model: "m", Input: 50},
	}
	if err := st.UpsertEvents(ev); err != nil {
		t.Fatal(err)
	}
	p, err := Build(st, nil, time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func rowOf(p Payload, agent string) Row {
	for _, r := range p.Rows {
		if r.Agent == agent {
			return r
		}
	}
	return Row{}
}

// TestCloudNotMeasuredAfterLastRow: last row 2026-11-12 and now 2026-11-17:
// today and this week start after it (not measured), the month holds it (+).
func TestCloudNotMeasuredAfterLastRow(t *testing.T) {
	last := time.Date(2026, 11, 12, 16, 43, 0, 0, time.UTC)
	p := cloudStripFixture(t, last, true)
	c := rowOf(p, "cowork-cloud")
	if c.TodayNote != NoteNotMeasured || c.WeekNote != NoteNotMeasured || c.MonthNote != NotePartial {
		t.Errorf("cloud notes = %q %q %q, want not_measured not_measured partial", c.TodayNote, c.WeekNote, c.MonthNote)
	}
	if c.Month != 700 {
		t.Errorf("cloud month = %d, want the number kept (700)", c.Month)
	}
	if p.Total.TodayNote != "" || p.Total.WeekNote != "" || p.Total.MonthNote != NotePartial {
		t.Errorf("total notes = %q %q %q, want only the month partial", p.Total.TodayNote, p.Total.WeekNote, p.Total.MonthNote)
	}
	if l := rowOf(p, "claude-code"); l.TodayNote != "" || l.WeekNote != "" || l.MonthNote != "" {
		t.Errorf("a local vendor carries notes: %+v", l)
	}
	if p.CloudMeasuredUntil == "" {
		t.Error("CloudMeasuredUntil is empty with a last row")
	}
}

// TestCloudNoDropRowsNoNotes: no drop row at all means no marker anywhere.
func TestCloudNoDropRowsNoNotes(t *testing.T) {
	p := cloudStripFixture(t, time.Time{}, false)
	for _, r := range append(append([]Row{}, p.Rows...), p.Total) {
		if r.TodayNote != "" || r.WeekNote != "" || r.MonthNote != "" {
			t.Errorf("row %q carries notes with no drop rows: %+v", r.Agent, r)
		}
	}
	if p.CloudMeasuredUntil != "" {
		t.Errorf("CloudMeasuredUntil = %q, want empty", p.CloudMeasuredUntil)
	}
}

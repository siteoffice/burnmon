package centre

import (
	"testing"
	"time"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestMondayOf(t *testing.T) {
	want := day(2026, 11, 9)
	cases := []time.Time{
		time.Date(2026, 11, 11, 15, 30, 0, 0, time.UTC),
		time.Date(2026, 11, 15, 23, 59, 0, 0, time.UTC),
	}
	for _, c := range cases {
		if got := MondayOf(c); !got.Equal(want) {
			t.Errorf("MondayOf(%v) = %v, want %v", c, got, want)
		}
	}
	if got := MondayOf(day(2026, 11, 16)); !got.Equal(day(2026, 11, 16)) {
		t.Errorf("MondayOf(Mon 16 Nov) = %v", got)
	}
}

func TestPlanningGrid(t *testing.T) {
	plan := &Plan{
		Dates: []PlanDate{
			{Date: "2026-11-20", Time: "10:00", Title: "B review"},
			{Date: "2026-11-20", Time: "09:00", Title: "Z early"},
			{Date: "2026-11-20", Time: "10:00", Title: "A review"},
			{Date: "2026-11-25", Title: "Outside"},
		},
		WeekNotes: []WeekNote{{Week: "2026-W47", Text: "Review week."}},
	}
	g := PlanningGrid(day(2026, 11, 9), 2, day(2026, 11, 11), plan)
	if g.Weeks != 2 || g.Monday != "2026-11-09" || g.Today != "2026-11-11" {
		t.Errorf("head %d %s %s", g.Weeks, g.Monday, g.Today)
	}
	if len(g.Rows) != 2 {
		t.Fatalf("rows %d, want 2", len(g.Rows))
	}
	for i, r := range g.Rows {
		if len(r.Days) != 7 {
			t.Errorf("row %d has %d days", i, len(r.Days))
		}
	}
	first := g.Rows[0].Days[0]
	if first.Date != "2026-11-09" || first.Label != "9 Nov" || first.Weekday != "Mon" || first.Today || first.Weekend {
		t.Errorf("first day %+v", first)
	}
	if !g.Rows[0].Days[2].Today || g.Rows[0].Days[2].Date != "2026-11-11" {
		t.Errorf("today cell %+v", g.Rows[0].Days[2])
	}
	if !g.Rows[0].Days[5].Weekend || !g.Rows[0].Days[6].Weekend || g.Rows[0].Days[4].Weekend {
		t.Error("weekend flags wrong")
	}
	if g.Rows[0].Week != "2026-W46" || len(g.Rows[0].Notes) != 0 {
		t.Errorf("row 0 %s %v", g.Rows[0].Week, g.Rows[0].Notes)
	}
	r1 := g.Rows[1]
	if r1.Week != "2026-W47" || r1.Label != "W47" || len(r1.Notes) != 1 || r1.Notes[0] != "Review week." {
		t.Errorf("row 1 %s %s %v", r1.Week, r1.Label, r1.Notes)
	}
	fri := r1.Days[4]
	if fri.Date != "2026-11-20" || len(fri.Dates) != 3 {
		t.Fatalf("friday %+v", fri)
	}
	order := []string{fri.Dates[0].Title, fri.Dates[1].Title, fri.Dates[2].Title}
	if order[0] != "Z early" || order[1] != "A review" || order[2] != "B review" {
		t.Errorf("order %v", order)
	}
	if g.Rows[0].Days[0].Dates == nil {
		t.Error("empty day dates is nil, want []")
	}

	if g := PlanningGrid(day(2026, 11, 9), 4, day(2026, 11, 11), plan); len(g.Rows) != 4 || g.Weeks != 4 {
		t.Errorf("weeks 4: %d rows, weeks %d", len(g.Rows), g.Weeks)
	}
	if g := PlanningGrid(day(2026, 11, 9), 3, day(2026, 11, 11), plan); len(g.Rows) != 2 || g.Weeks != 2 {
		t.Errorf("weeks 3: %d rows, weeks %d", len(g.Rows), g.Weeks)
	}
	if g := PlanningGrid(day(2026, 11, 9), 2, day(2026, 11, 11), nil); len(g.Rows) != 2 {
		t.Errorf("no plan: %d rows", len(g.Rows))
	}
}

func TestNextFixed(t *testing.T) {
	today := time.Date(2026, 11, 11, 18, 0, 0, 0, time.UTC)
	plan := &Plan{Dates: []PlanDate{
		{Date: "2026-11-01", Title: "Past", Fixed: true},
		{Date: "2026-11-12", Title: "Loose"},
		{Date: "2026-11-20", Time: "10:00", Title: "B"},
		{Date: "2026-11-20", Time: "10:00", Title: "A", Fixed: true},
		{Date: "2026-11-20", Time: "09:00", Title: "C", Fixed: true},
	}}
	got := NextFixed(plan, today)
	if got == nil || got.Title != "C" {
		t.Errorf("next %+v, want C", got)
	}
	plan.Dates = append(plan.Dates, PlanDate{Date: "2026-11-11", Time: "08:00", Title: "Today", Fixed: true})
	if got := NextFixed(plan, today); got == nil || got.Title != "Today" {
		t.Errorf("next %+v, want Today", got)
	}
	if got := NextFixed(&Plan{Dates: []PlanDate{{Date: "2026-11-01", Fixed: true}, {Date: "2026-12-01"}}}, today); got != nil {
		t.Errorf("next %+v, want nil", got)
	}
	if got := NextFixed(nil, today); got != nil {
		t.Errorf("no plan: %+v", got)
	}
}

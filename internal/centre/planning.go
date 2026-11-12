package centre

import (
	"fmt"
	"sort"
	"time"
)

const dateLayout = "2006-01-02"

// MondayOf returns midnight of the Monday of t's week, in t's location.
func MondayOf(t time.Time) time.Time {
	back := (int(t.Weekday()) + 6) % 7
	return time.Date(t.Year(), t.Month(), t.Day()-back, 0, 0, 0, 0, t.Location())
}

// PlanningGrid lays out weeks rows of 7 days from monday. weeks other than
// 4 means 2. Each day holds the plan dates on it, by time, then title
// (a date without a time first); each week holds the notes of its ISO week.
func PlanningGrid(monday time.Time, weeks int, today time.Time, plan *Plan) Planning {
	if weeks != 4 {
		weeks = 2
	}
	todayStr := today.Format(dateLayout)
	g := Planning{Weeks: weeks, Monday: monday.Format(dateLayout), Today: todayStr, Rows: []PlanWeek{}}
	for w := 0; w < weeks; w++ {
		start := time.Date(monday.Year(), monday.Month(), monday.Day()+7*w, 0, 0, 0, 0, monday.Location())
		y, n := start.ISOWeek()
		row := PlanWeek{
			Week:  fmt.Sprintf("%04d-W%02d", y, n),
			Label: fmt.Sprintf("W%02d", n),
			Notes: []string{},
			Days:  []PlanDay{},
		}
		if plan != nil {
			for _, note := range plan.WeekNotes {
				if note.Week == row.Week {
					row.Notes = append(row.Notes, note.Text)
				}
			}
		}
		for d := 0; d < 7; d++ {
			t := time.Date(start.Year(), start.Month(), start.Day()+d, 0, 0, 0, 0, start.Location())
			date := t.Format(dateLayout)
			cell := PlanDay{
				Date:    date,
				Label:   t.Format("2 Jan"),
				Weekday: t.Format("Mon"),
				Today:   date == todayStr,
				Weekend: t.Weekday() == time.Saturday || t.Weekday() == time.Sunday,
				Dates:   []PlanDate{},
			}
			if plan != nil {
				for _, pd := range plan.Dates {
					if pd.Date == date {
						cell.Dates = append(cell.Dates, pd)
					}
				}
			}
			sort.SliceStable(cell.Dates, func(i, j int) bool {
				a, b := cell.Dates[i], cell.Dates[j]
				if a.Time != b.Time {
					return a.Time < b.Time
				}
				return a.Title < b.Title
			})
			row.Days = append(row.Days, cell)
		}
		g.Rows = append(g.Rows, row)
	}
	return g
}

// NextFixed returns the first fixed plan date on or after today's date (in
// today's location), by date, time, title; nil when there is none. A fixed
// date on today counts even when its time has passed.
func NextFixed(plan *Plan, today time.Time) *PlanDate {
	if plan == nil {
		return nil
	}
	todayStr := today.Format(dateLayout)
	var best *PlanDate
	for i := range plan.Dates {
		d := plan.Dates[i]
		if !d.Fixed || d.Date < todayStr {
			continue
		}
		if best == nil || d.Date < best.Date ||
			(d.Date == best.Date && (d.Time < best.Time || (d.Time == best.Time && d.Title < best.Title))) {
			c := d
			best = &c
		}
	}
	return best
}

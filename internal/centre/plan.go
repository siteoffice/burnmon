package centre

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

// PlanFile is the plan file name, directly in root.
const PlanFile = "siteoffice.plan.json"

var weekPattern = regexp.MustCompile(`^(\d{4})-W(\d{2})$`)

// LoadPlan reads root/siteoffice.plan.json. An absent file gives nil, nil.
// A file that does not read or parse, has a schemaVersion other than 1, a
// date that is not YYYY-MM-DD, a week that is not an ISO week such as
// 2026-W47, or an id used twice across goals, lines and items is refused:
// nil plus one problem.
func LoadPlan(root string) (*Plan, *Problem) {
	refuse := func(format string, a ...any) (*Plan, *Problem) {
		return nil, &Problem{Path: PlanFile, Message: fmt.Sprintf(format, a...)}
	}
	data, err := os.ReadFile(filepath.Join(root, PlanFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return refuse("cannot read plan file: %v", err)
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return refuse("cannot read plan file: %v", err)
	}
	if plan.SchemaVersion != 1 {
		return refuse("plan schemaVersion is %d, want 1", plan.SchemaVersion)
	}
	seen := map[string]bool{}
	for _, list := range [][]PlanNode{plan.Goals, plan.Lines, plan.Items} {
		for _, n := range list {
			if seen[n.ID] {
				return refuse("plan id %q is used twice", n.ID)
			}
			seen[n.ID] = true
		}
	}
	for _, d := range plan.Dates {
		if _, err := time.Parse("2006-01-02", d.Date); err != nil || len(d.Date) != 10 {
			return refuse("plan date %q is not a YYYY-MM-DD date", d.Date)
		}
	}
	for _, w := range plan.WeekNotes {
		if !validWeek(w.Week) {
			return refuse("plan week %q is not an ISO week such as 2026-W47", w.Week)
		}
	}
	if plan.Goals == nil {
		plan.Goals = []PlanNode{}
	}
	if plan.Lines == nil {
		plan.Lines = []PlanNode{}
	}
	if plan.Items == nil {
		plan.Items = []PlanNode{}
	}
	if plan.Links == nil {
		plan.Links = []PlanLink{}
	}
	if plan.Dates == nil {
		plan.Dates = []PlanDate{}
	}
	if plan.WeekNotes == nil {
		plan.WeekNotes = []WeekNote{}
	}
	return &plan, nil
}

// validWeek reports whether s is an ISO week that exists, such as 2026-W47.
func validWeek(s string) bool {
	m := weekPattern.FindStringSubmatch(s)
	if m == nil {
		return false
	}
	year, _ := strconv.Atoi(m[1])
	week, _ := strconv.Atoi(m[2])
	// 28 December always lies in the last ISO week of its year.
	_, last := time.Date(year, 12, 28, 0, 0, 0, 0, time.UTC).ISOWeek()
	return week >= 1 && week <= last
}

// unitByName maps each unit name to the first unit id in id order with
// that name. units must be sorted by id.
func unitByName(units []Unit) map[string]string {
	m := map[string]string{}
	for _, u := range units {
		if _, ok := m[u.Name]; !ok {
			m[u.Name] = u.ID
		}
	}
	return m
}

// Badges counts, per plan item, the waiting flow rows owned by the item's
// unit and by its parts' units, each unit counted once, no descendant
// units. Only items with a count above 0 are in the map; it is never nil.
func Badges(plan *Plan, units []Unit, flows []FlowRow) map[string]int {
	badges := map[string]int{}
	if plan == nil {
		return badges
	}
	byName := unitByName(units)
	waiting := map[string]int{}
	for _, f := range flows {
		if f.Waiting {
			waiting[f.Unit]++
		}
	}
	for _, item := range plan.Items {
		ids := map[string]bool{}
		if id, ok := byName[item.Unit]; ok && item.Unit != "" {
			ids[id] = true
		}
		for _, p := range item.Parts {
			if id, ok := byName[p.Unit]; ok && p.Unit != "" {
				ids[id] = true
			}
		}
		n := 0
		for id := range ids {
			n += waiting[id]
		}
		if n > 0 {
			badges[item.ID] = n
		}
	}
	return badges
}

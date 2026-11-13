package centre

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Build reads root and returns the centre state at opt.Now. It only reads.
// Weeks other than 4 means 2; Rows below 1 means 12.
func Build(root string, opt Options) State {
	now := opt.Now
	weeks := opt.Weeks
	if weeks != 4 {
		weeks = 2
	}
	rows := opt.Rows
	if rows < 1 {
		rows = 12
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}

	st := State{
		SchemaVersion: 1,
		Root:          root,
		Now:           now.UTC().Format(time.RFC3339),
		Today:         now.Format(dateLayout),
		RowsPerPage:   rows,
		Units:         []Unit{},
		Flows:         []FlowRow{},
		Sprints:       []SprintRow{},
		Waiting:       List{Keys: []string{}, Pages: []Span{}},
		Overview:      Overview{UnitColumns: []Column{}, Default: "units"},
		PlanPath:      PlanFile,
		Badges:        map[string]int{},
		Problems:      []Problem{},
	}
	monday := MondayOf(now)

	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		st.Problems = append(st.Problems, Problem{Path: ".", Message: "root folder not found"})
		st.Planning = PlanningGrid(monday, weeks, now, nil)
		st.Counts.Problems = len(st.Problems)
		return st
	}

	units, problems := FindUnits(root)
	st.Problems = append(st.Problems, problems...)
	for i := range units {
		dir := filepath.Join(root, filepath.FromSlash(units[i].ID))
		units[i].Files, units[i].Folders = ListFiles(dir, now)
		units[i].FilePages = Pages(len(units[i].Files), rows)
	}

	flows, sprints, problems := ReadBoards(root, units, now)
	st.Problems = append(st.Problems, problems...)
	SortFlows(flows)
	SortSprints(sprints)
	st.Flows, st.Sprints = flows, sprints

	index := map[string]int{}
	for i, u := range units {
		index[u.ID] = i
	}
	for _, f := range flows {
		if i, ok := index[f.Unit]; ok {
			units[i].Flows.Keys = append(units[i].Flows.Keys, f.Key)
		}
		if f.Waiting {
			st.Waiting.Keys = append(st.Waiting.Keys, WaitFlow+f.Key)
			if f.WaitsDays > st.Counts.LongestWaitDays {
				st.Counts.LongestWaitDays = f.WaitsDays
			}
		}
	}
	for _, s := range sprints {
		if i, ok := index[s.Unit]; ok {
			units[i].Sprints.Keys = append(units[i].Sprints.Keys, s.Key)
		}
		if s.Waiting {
			st.Waiting.Keys = append(st.Waiting.Keys, WaitSprint+s.Key)
			if s.WaitsDays > st.Counts.LongestWaitDays {
				st.Counts.LongestWaitDays = s.WaitsDays
			}
		}
	}
	st.Problems = append(st.Problems, StaleProblems(flows)...)
	for i := range units {
		units[i].Flows.Pages = Pages(len(units[i].Flows.Keys), rows)
		units[i].Sprints.Pages = Pages(len(units[i].Sprints.Keys), rows)
	}
	st.Waiting.Pages = Pages(len(st.Waiting.Keys), rows)

	plan, prob := LoadPlan(root)
	if prob != nil {
		st.Problems = append(st.Problems, *prob)
	}
	if plan != nil {
		byName := unitByName(units)
		for i := range plan.Items {
			item := &plan.Items[i]
			if item.Unit != "" {
				item.UnitID = byName[item.Unit]
			}
			for j := range item.Parts {
				if p := &item.Parts[j]; p.Unit != "" {
					p.UnitID = byName[p.Unit]
				}
			}
			if item.UnitID != "" {
				if u := &units[index[item.UnitID]]; u.Item == "" {
					u.Item = item.ID
				}
			}
		}
		st.Plan = plan
		st.Overview.Default = "plan"
		st.Overview.PlanColumns = []Column{
			column("Goals", nodeIDs(plan.Goals), rows),
			column("Plan", nodeIDs(plan.Lines), rows),
			column("Products and projects", nodeIDs(plan.Items), rows),
		}
	}
	st.Badges = Badges(plan, units, flows, sprints)

	maxLevel := -1
	for _, u := range units {
		if u.Level > maxLevel {
			maxLevel = u.Level
		}
	}
	for level := 0; level <= maxLevel; level++ {
		keys := []string{}
		for _, u := range units {
			if u.Level == level {
				keys = append(keys, u.ID)
			}
		}
		title := "Root"
		if level > 0 {
			title = fmt.Sprintf("Level %d", level)
		}
		st.Overview.UnitColumns = append(st.Overview.UnitColumns, column(title, keys, rows))
	}
	st.Units = units

	st.Planning = PlanningGrid(monday, weeks, now, plan)
	st.NextFixed = NextFixed(plan, now)

	sort.SliceStable(st.Problems, func(i, j int) bool { return st.Problems[i].Path < st.Problems[j].Path })
	st.Counts = Counts{
		Units:           len(st.Units),
		Flows:           len(st.Flows),
		Sprints:         len(st.Sprints),
		Waiting:         len(st.Waiting.Keys),
		LongestWaitDays: st.Counts.LongestWaitDays,
		Problems:        len(st.Problems),
	}
	return st
}

func column(title string, keys []string, rows int) Column {
	return Column{Title: title, List: List{Keys: keys, Pages: Pages(len(keys), rows)}}
}

func nodeIDs(nodes []PlanNode) []string {
	ids := []string{}
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	return ids
}

// Package waits turns Foreman state, the relation graph's waits_on edges and
// agent view entries into two lists for the snapshot: what waits on the
// person and what finished or failed in the last day. It reads nothing itself.
package waits

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"burnmon/internal/graph"
)

// Item kinds.
const (
	KindGate        = "gate"
	KindSprint      = "sprint"
	KindFailedCheck = "failed_check"
	KindQuestion    = "question"
	KindPrompt      = "prompt"
	KindSession     = "session"
	KindFinished    = "finished"

	SourceAgentView = "agent_view"

	OutcomeWindow = 24 * time.Hour
	DetailMax     = 120

	personID = "person:you"
)

// Parked is a task a sprint parked for the person.
type Parked struct {
	N      int    `json:"n"`
	Title  string `json:"title"`
	Reason string `json:"reason,omitempty"`
}

// Item is one row of Waits on you or Outcomes.
type Item struct {
	Kind       string   `json:"kind"`
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Project    string   `json:"project,omitempty"`
	Detail     string   `json:"detail,omitempty"`
	Since      string   `json:"since"`       // RFC3339 UTC
	AgeSeconds int64    `json:"age_seconds"` // seconds since Since at Now
	SpecPath   string   `json:"spec_path,omitempty"`
	ResultPath string   `json:"result_path,omitempty"`
	Source     string   `json:"source"`
	Parked     []Parked `json:"parked,omitempty"`
}

// Input is everything Build joins. Any field may be empty.
type Input struct {
	Now     time.Time
	Sprints []graph.Sprint
	Flows   []graph.Flow
	Graph   graph.Graph
	Agents  []AgentEntry
}

func itoa(n int) string { return strconv.Itoa(n) }

func utc(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func samePath(a, b string) bool {
	return a != "" && b != "" && strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// cut shortens s to DetailMax runes, adding "..." when it cut.
func cut(s string) string {
	r := []rune(s)
	if len(r) <= DetailMax {
		return s
	}
	return string(r[:DetailMax]) + "..."
}

// lastFolder is the last folder name of a Windows or slash path.
func lastFolder(p string) string {
	p = strings.TrimRight(strings.ReplaceAll(p, "/", `\`), `\`)
	if i := strings.LastIndex(p, `\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func sprintStopped(status string) bool {
	return status == "capped" || status == "failed" || status == "escalated"
}

// Build returns the wait and outcome items, newest first, never nil.
func Build(in Input) (wait, outcomes []Item) {
	wait, outcomes = []Item{}, []Item{}
	now := in.Now
	age := func(t time.Time) int64 {
		if a := int64(now.Sub(t).Seconds()); a > 0 {
			return a
		}
		return 0
	}
	mk := func(kind, id, title, project, detail string, since time.Time, spec, result, source string) Item {
		return Item{Kind: kind, ID: id, Title: title, Project: project, Detail: cut(detail), Since: utc(since),
			AgeSeconds: age(since), SpecPath: spec, ResultPath: result, Source: source}
	}

	// resultForSpec is the result path of the newest sprint with that spec.
	resultForSpec := func(spec string) string {
		var best *graph.Sprint
		for i := range in.Sprints {
			s := &in.Sprints[i]
			if samePath(s.SpecPath, spec) && (best == nil || s.UpdatedAt.After(best.UpdatedAt)) {
				best = s
			}
		}
		if best == nil {
			return ""
		}
		return best.ResultPath
	}

	for _, f := range in.Flows {
		if !f.GatePending {
			continue
		}
		since := f.GateOpenedAt
		if since.IsZero() {
			since = f.UpdatedAt
		}
		wait = append(wait, mk(KindGate, "gate:"+f.ID+":"+f.Gate, f.Slug, f.Project, f.Gate, since,
			f.SpecPath, resultForSpec(f.SpecPath), graph.SourceForeman))
	}

	for _, s := range in.Sprints {
		if s.Status == "waiting-on-you" {
			since := s.LastParkedAt
			if since.IsZero() {
				since = s.UpdatedAt
			}
			it := mk(KindSprint, "sprint:"+s.ID, s.Slug, s.Project, "", since, s.SpecPath, s.ResultPath, graph.SourceForeman)
			switch n := len(s.Parked); n {
			case 0:
			case 1:
				it.Detail = "1 task parked"
			default:
				it.Detail = itoa(n) + " tasks parked"
			}
			for _, p := range s.Parked {
				it.Parked = append(it.Parked, Parked{N: p.N, Title: p.Title, Reason: p.Reason})
			}
			wait = append(wait, it)
		}

		failedOutcome := false
		for _, c := range []struct {
			name string
			chk  *graph.Check
		}{{"test", s.Test}, {"verify", s.Verify}} {
			if c.chk == nil || !c.chk.Failed {
				continue
			}
			at := c.chk.At
			if at.IsZero() {
				at = s.UpdatedAt
			}
			detail := c.chk.Label
			if c.chk.Summary != "" {
				detail += ": " + c.chk.Summary
			}
			it := mk(KindFailedCheck, "check:"+s.ID+":"+c.name, s.Slug, s.Project, detail, at,
				s.SpecPath, s.ResultPath, graph.SourceForeman)
			if sprintStopped(s.Status) {
				wait = append(wait, it)
			}
			if now.Sub(at) <= OutcomeWindow {
				outcomes = append(outcomes, it)
				failedOutcome = true
			}
		}

		// A sprint that ended (done, passing, or stopped as capped, failed or
		// escalated) within the window is a finished outcome. A sprint that
		// already shows a failed_check outcome is not listed a second time:
		// one outcome row per sprint.
		stopped := sprintStopped(s.Status)
		ended := stopped || s.Phase == "done" || s.Status == "passing" || s.Status == "done"
		if ended && !failedOutcome && now.Sub(s.UpdatedAt) <= OutcomeWindow {
			detail := "test " + checkWord(s.Test) + ", verify " + checkWord(s.Verify)
			if stopped {
				detail = s.Status + ", " + detail
			}
			outcomes = append(outcomes, mk(KindFinished, "sprint:"+s.ID, s.Slug, s.Project, detail, s.UpdatedAt,
				s.SpecPath, s.ResultPath, graph.SourceForeman))
		}
	}

	// Sessions: waits_on edges from a session or subagent to the person, then
	// agent view entries, which replace an item for the same id.
	nodes := map[string]graph.Node{}
	for _, n := range in.Graph.Nodes {
		nodes[n.ID] = n
	}
	worksFor := map[string]string{}
	for _, e := range in.Graph.Edges {
		if e.Kind != graph.KindWorksFor {
			continue
		}
		if old, ok := worksFor[e.From]; !ok || (strings.HasPrefix(e.To, "sprint:") && !strings.HasPrefix(old, "sprint:")) {
			worksFor[e.From] = e.To
		}
	}
	specResult := func(nodeID string) (spec, result string) {
		to, ok := worksFor[nodeID]
		if !ok {
			return "", ""
		}
		if id, ok := strings.CutPrefix(to, "sprint:"); ok {
			for _, s := range in.Sprints {
				if s.ID == id {
					return s.SpecPath, s.ResultPath
				}
			}
		}
		if id, ok := strings.CutPrefix(to, "flow:"); ok {
			for _, f := range in.Flows {
				if f.ID == id {
					return f.SpecPath, resultForSpec(f.SpecPath)
				}
			}
		}
		return "", ""
	}

	sessions := map[string]Item{}
	var sessionOrder []string
	for _, e := range in.Graph.Edges {
		if e.Kind != graph.KindWaitsOn || e.To != personID {
			continue
		}
		n, ok := nodes[e.From]
		if !ok || (n.Kind != graph.KindSession && n.Kind != graph.KindSubagent) {
			continue
		}
		spec, result := specResult(n.ID)
		since := now.Add(-time.Duration(e.AgeSeconds) * time.Second)
		if _, dup := sessions[n.ID]; !dup {
			sessionOrder = append(sessionOrder, n.ID)
		}
		sessions[n.ID] = mk(KindSession, n.ID, n.Label, "", "", since, spec, result, e.Source)
	}
	for _, a := range in.Agents {
		if !a.Waiting() {
			continue
		}
		key := a.Key()
		since := a.Since
		if since.IsZero() {
			since = now
		}
		kind, detail := KindSession, "blocked"
		if a.Status == "waiting" && a.WaitingFor != "" {
			detail = a.WaitingFor
			switch a.WaitingFor {
			case "permission prompt":
				kind = KindPrompt
			case "input needed":
				kind = KindQuestion
			}
		}
		spec, result := specResult(key)
		if _, dup := sessions[key]; !dup {
			sessionOrder = append(sessionOrder, key)
		}
		sessions[key] = mk(kind, key, lastFolder(a.Cwd), "", detail, since, spec, result, SourceAgentView)
	}
	for _, id := range sessionOrder {
		wait = append(wait, sessions[id])
	}

	sortItems(wait)
	sortItems(outcomes)
	return wait, outcomes
}

func checkWord(c *graph.Check) string {
	switch {
	case c == nil:
		return "-"
	case c.Failed:
		return "fail"
	}
	return "pass"
}

// sortItems puts the newest first (ascending age), ties by id.
func sortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].AgeSeconds != items[j].AgeSeconds {
			return items[i].AgeSeconds < items[j].AgeSeconds
		}
		return items[i].ID < items[j].ID
	})
}

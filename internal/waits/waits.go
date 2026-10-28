// Package waits turns Foreman state, the relation graph's waits_on edges and
// agent view entries into two lists for the snapshot: what waits on the
// person, in one ranked list of three groups, and what finished or failed in
// the last day. It reads nothing itself; the cost of waiting comes from an
// injected function.
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
	KindCloseOut    = "close_out"
	KindCanClose    = "can_close"
	KindBranchGone  = "branch_gone"
	KindYourTurn    = "your_turn"

	SourceAgentView = "agent_view"

	OutcomeWindow = 24 * time.Hour
	DetailMax     = 120

	personID = "person:you"
)

// Item groups, in ranking order. Only GroupBlocked is the amber number.
const (
	GroupBlocked  = "blocked"
	GroupYourTurn = "your_turn"
	GroupDone     = "done"
)

// Parked is a task a sprint parked for the person.
type Parked struct {
	N      int    `json:"n"`
	Title  string `json:"title"`
	Reason string `json:"reason,omitempty"`
}

// Item is one row of Waits on you or Outcomes.
type Item struct {
	Kind        string   `json:"kind"`
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Project     string   `json:"project,omitempty"`
	Detail      string   `json:"detail,omitempty"`
	Since       string   `json:"since"`       // RFC3339 UTC
	AgeSeconds  int64    `json:"age_seconds"` // seconds since Since at Now
	SpecPath    string   `json:"spec_path,omitempty"`
	ResultPath  string   `json:"result_path,omitempty"`
	Source      string   `json:"source"`
	Parked      []Parked `json:"parked,omitempty"`
	Group       string   `json:"group"`                  // blocked, your_turn or done
	Holds       int      `json:"holds"`                  // sessions, sprints and flows the item holds, itself included
	CostTokens  int64    `json:"cost_tokens,omitempty"`  // tokens burned by what waits on it since Since
	CostPartial bool     `json:"cost_partial,omitempty"` // the figure misses tokens older than the live window
	SprintID    string   `json:"sprint_id,omitempty"`    // close_out only: the Close control's target
	// ParkedLines and ReportPath, close_out only: the parked lines in the
	// Close-out block of the sprint's report.md, and that file's path ("" when
	// there is none). Foreman refuses to close a sprint with parked lines or
	// without a report, so the page shows no Close chip then.
	ParkedLines int    `json:"parked_lines,omitempty"`
	ReportPath  string `json:"report_path,omitempty"`
	// CloseOK and CloseReason, close_out only: graph.CloseOutVerdict, whether
	// Foreman's close-out gate would accept the report, and why not. The page
	// shows the Close chip only when CloseOK is true.
	CloseOK     bool   `json:"close_ok,omitempty"`
	CloseReason string `json:"close_reason,omitempty"`
}

// Counts is the one count of the wait list, per group.
type Counts struct {
	Blocked  int `json:"blocked"`
	YourTurn int `json:"your_turn"`
	Done     int `json:"done"`
}

// CountOf is the one count: every place that shows a number reads it.
func CountOf(items []Item) Counts {
	var c Counts
	for _, it := range items {
		switch it.Group {
		case GroupBlocked:
			c.Blocked++
		case GroupYourTurn:
			c.YourTurn++
		case GroupDone:
			c.Done++
		}
	}
	return c
}

// Input is everything Build joins. Any field may be empty.
type Input struct {
	Now     time.Time
	Sprints []graph.Sprint
	Flows   []graph.Flow
	Graph   graph.Graph
	Agents  []AgentEntry
	// TokensSince sums the tokens the given session nodes burned since a
	// time. partial is true when it cannot see that far back. Nil: no cost.
	TokensSince func(sessionNodeIDs []string, since time.Time) (tokens int64, partial bool)
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

// sprintFinished reports a sprint that no longer holds anything. Foreman's
// status values (sprints\_lib\schema.md): running, paused and waiting-on-you
// are live and count toward a gate's holds; done, passing, failed, capped and
// escalated are finished and do not. An empty or unknown status counts as live.
func sprintFinished(status string) bool {
	switch status {
	case "done", "passing", "failed", "capped", "escalated":
		return true
	}
	return false
}

func sprintStopped(status string) bool {
	return status == "capped" || status == "failed" || status == "escalated"
}

// entry is a wait item with what the cost of waiting needs.
type entry struct {
	it    Item
	ids   []string
	since time.Time
}

func groupOf(kind string) string {
	switch kind {
	case KindYourTurn:
		return GroupYourTurn
	case KindCloseOut, KindCanClose, KindBranchGone:
		return GroupDone
	}
	return GroupBlocked
}

// Build returns the wait list, ranked, and the outcomes, newest first, never nil.
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
	var ents []entry
	add := func(it Item, since time.Time, holds int, ids []string) {
		it.Group = groupOf(it.Kind)
		it.Holds = holds
		ents = append(ents, entry{it: it, ids: ids, since: since})
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

	nodes := map[string]graph.Node{}
	for _, n := range in.Graph.Nodes {
		nodes[n.ID] = n
	}
	worksFor := map[string]string{}
	workers := map[string][]string{} // flow or sprint node id to the sessions that work for it
	children := map[string][]string{}
	for _, e := range in.Graph.Edges {
		switch e.Kind {
		case graph.KindWorksFor:
			if old, ok := worksFor[e.From]; !ok || (strings.HasPrefix(e.To, "sprint:") && !strings.HasPrefix(old, "sprint:")) {
				worksFor[e.From] = e.To
			}
			workers[e.To] = append(workers[e.To], e.From)
		case graph.KindSpawned:
			children[e.From] = append(children[e.From], e.To)
		}
	}
	// held returns the given sessions and their subagents, sorted, once each.
	held := func(sessions []string) []string {
		seen := map[string]bool{}
		var out []string
		for _, s := range sessions {
			for _, id := range append([]string{s}, children[s]...) {
				if !seen[id] {
					seen[id] = true
					out = append(out, id)
				}
			}
		}
		sort.Strings(out)
		return out
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

	closeout := map[string]bool{} // sprint node ids whose branch is in main
	gone := map[string]bool{}     // sprint node ids whose branch ref is gone and whose merge was not found
	for _, n := range in.Graph.Nodes {
		if n.Kind == graph.KindSprint && n.Attrs["closeout"] == "true" {
			closeout[n.ID] = true
		}
		if n.Kind == graph.KindSprint && n.Attrs["branch_gone"] == "true" {
			gone[n.ID] = true
		}
	}

	for _, f := range in.Flows {
		if !f.GatePending {
			continue
		}
		since := f.GateOpenedAt
		if since.IsZero() {
			since = f.UpdatedAt
		}
		holds := 1
		for _, s := range in.Sprints {
			if samePath(s.SpecPath, f.SpecPath) && !sprintFinished(s.Status) {
				holds++
			}
		}
		sess := workers["flow:"+f.ID]
		holds += len(sess)
		add(mk(KindGate, "gate:"+f.ID+":"+f.Gate, f.Slug, f.Project, f.Gate, since,
			f.SpecPath, resultForSpec(f.SpecPath), graph.SourceForeman), since, holds, held(sess))
	}

	for _, s := range in.Sprints {
		if s.Status == "waiting-on-you" && !closeout["sprint:"+s.ID] && !gone["sprint:"+s.ID] {
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
			sess := workers["sprint:"+s.ID]
			add(it, since, 1+len(sess), held(sess))
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
				add(it, at, 1, nil)
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

	// Done, needs close-out: a waiting sprint whose branch is in main. The
	// graph decided that; the sprint, when listed, gives the detail.
	for _, n := range in.Graph.Nodes {
		if !closeout[n.ID] {
			continue
		}
		id := strings.TrimPrefix(n.ID, "sprint:")
		title, project, spec, result := n.Label, "", "", ""
		report, parked, closeOK, closeReason := "", 0, false, ""
		since := now.Add(-time.Duration(n.AgeSeconds) * time.Second)
		for _, s := range in.Sprints {
			if s.ID != id {
				continue
			}
			title, project, spec, result = s.Slug, s.Project, s.SpecPath, s.ResultPath
			since = s.LastParkedAt
			if since.IsZero() {
				since = s.UpdatedAt
			}
			report, parked, closeOK, closeReason = s.ReportPath, s.ParkedLines, s.CloseOK, s.CloseReason
		}
		it := mk(KindCloseOut, "closeout:"+id, title, project, "Done, needs close-out", since, spec, result, graph.SourceForeman)
		it.SprintID = id
		it.ReportPath, it.ParkedLines = report, parked
		it.CloseOK, it.CloseReason = closeOK, closeReason
		sess := workers[n.ID]
		add(it, since, 1+len(sess), held(sess))
	}

	// Branch gone, check by hand: the branch ref was deleted and no merge
	// commit naming it was found. Grey, in the Done group, no Close target.
	for _, n := range in.Graph.Nodes {
		if !gone[n.ID] {
			continue
		}
		id := strings.TrimPrefix(n.ID, "sprint:")
		title, project, spec, result := n.Label, "", "", ""
		since := now.Add(-time.Duration(n.AgeSeconds) * time.Second)
		for _, s := range in.Sprints {
			if s.ID != id {
				continue
			}
			title, project, spec, result = s.Slug, s.Project, s.SpecPath, s.ResultPath
			since = s.LastParkedAt
			if since.IsZero() {
				since = s.UpdatedAt
			}
		}
		it := mk(KindBranchGone, "branchgone:"+id, title, project, "branch gone, check by hand", since, spec, result, graph.SourceForeman)
		sess := workers[n.ID]
		add(it, since, 1+len(sess), held(sess))
	}

	// Sessions: waits_on edges from a session or subagent to the person (only
	// Blocked ones have one), then agent view entries, which replace an item
	// for the same id. Your turn and Done sessions come from the node attrs.
	sessions := map[string]entry{}
	var sessionOrder []string
	put := func(e entry) {
		if _, dup := sessions[e.it.ID]; !dup {
			sessionOrder = append(sessionOrder, e.it.ID)
		}
		sessions[e.it.ID] = e
	}
	mkSession := func(it Item, since time.Time, id string) entry {
		it.Group = groupOf(it.Kind)
		it.Holds = 1 + len(children[id])
		return entry{it: it, ids: held([]string{id}), since: since}
	}
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
		put(mkSession(mk(KindSession, n.ID, n.Label, "", "", since, spec, result, e.Source), since, n.ID))
	}
	for _, a := range in.Agents {
		if !a.Waiting() {
			continue
		}
		key := a.Key()
		if nodes[key].Attrs["can_close"] == "true" {
			continue // the work is in main; the question is stale
		}
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
		put(mkSession(mk(kind, key, lastFolder(a.Cwd), "", detail, since, spec, result, SourceAgentView), since, key))
	}
	for _, id := range sessionOrder {
		ents = append(ents, sessions[id])
	}

	// Your turn and Done sessions. Blocked wins over both, Working hides both.
	yourTurn := map[string]entry{}
	var yourOrder []string
	putTurn := func(e entry) {
		if _, dup := yourTurn[e.it.ID]; !dup {
			yourOrder = append(yourOrder, e.it.ID)
		}
		yourTurn[e.it.ID] = e
	}
	for _, n := range in.Graph.Nodes {
		if n.Kind != graph.KindSession {
			continue
		}
		since := now.Add(-time.Duration(n.AgeSeconds) * time.Second)
		if n.Attrs["can_close"] == "true" {
			it := mk(KindCanClose, "canclose:"+n.ID, n.Label, "", "Done, can be closed", since, "", "", n.Source)
			ents = append(ents, mkSession(it, since, n.ID))
			continue
		}
		if _, isBlocked := sessions[n.ID]; n.Attrs["wait"] == graph.WaitYourTurn && !isBlocked {
			spec, result := specResult(n.ID)
			putTurn(mkSession(mk(KindYourTurn, n.ID, n.Label, "", "", since, spec, result, n.Source), since, n.ID))
		}
	}
	for _, a := range in.Agents {
		if !a.YourTurn() {
			continue
		}
		key := a.Key()
		n, known := nodes[key]
		if _, isBlocked := sessions[key]; isBlocked || (known && (n.Attrs["wait"] == graph.WaitWorking || n.Attrs["can_close"] == "true")) {
			continue
		}
		since := a.Since
		if since.IsZero() {
			since = now
		}
		spec, result := specResult(key)
		putTurn(mkSession(mk(KindYourTurn, key, lastFolder(a.Cwd), "", "", since, spec, result, SourceAgentView), since, key))
	}
	for _, id := range yourOrder {
		ents = append(ents, yourTurn[id])
	}

	// The cost of waiting: tokens the held sessions burned since the item began.
	if in.TokensSince != nil {
		for i := range ents {
			if len(ents[i].ids) == 0 {
				continue
			}
			tokens, partial := in.TokensSince(ents[i].ids, ents[i].since)
			ents[i].it.CostTokens = tokens
			ents[i].it.CostPartial = partial && tokens > 0
		}
	}
	rank(ents)
	for _, e := range ents {
		wait = append(wait, e.it)
	}
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

func tier(group string) int {
	switch group {
	case GroupBlocked:
		return 0
	case GroupYourTurn:
		return 1
	}
	return 2
}

// rank orders the wait list: Blocked, then Your turn, then Done. Inside
// Blocked, the item that holds most goes first, then the oldest wait. Inside
// the other groups the oldest goes first. Ties by id.
func rank(ents []entry) {
	sort.SliceStable(ents, func(i, j int) bool {
		a, b := ents[i].it, ents[j].it
		if ta, tb := tier(a.Group), tier(b.Group); ta != tb {
			return ta < tb
		}
		if a.Group == GroupBlocked && a.Holds != b.Holds {
			return a.Holds > b.Holds
		}
		if a.AgeSeconds != b.AgeSeconds {
			return a.AgeSeconds > b.AgeSeconds
		}
		return a.ID < b.ID
	})
}

// sortItems puts the newest first (ascending age), ties by id. Outcomes only.
func sortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].AgeSeconds != items[j].AgeSeconds {
			return items[i].AgeSeconds < items[j].AgeSeconds
		}
		return items[i].ID < items[j].ID
	})
}

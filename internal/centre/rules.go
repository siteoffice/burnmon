package centre

import (
	"sort"
	"time"
)

// statusWaiting is the flow status that, with awaiting false or absent,
// marks a stale flow status: the gate was decided, the flow did not move.
const statusWaiting = "awaiting-owner"

// sprintWaiting is the sprint status that means the row waits on you.
const sprintWaiting = "waiting-on-you"

// StaleText is the state text of a stale flow status.
const StaleText = "gate decided, flow not advanced"

// WaitFlow and WaitSprint prefix the keys in State.Waiting with their kind.
const (
	WaitFlow   = "flow:"
	WaitSprint = "sprint:"
)

// StaleProblems returns one problem per flow row with a stale flow status,
// in the given order. It is never nil.
func StaleProblems(flows []FlowRow) []Problem {
	problems := []Problem{}
	for _, f := range flows {
		if f.StateText == StaleText {
			problems = append(problems, Problem{Path: f.Index, Message: "flow " + f.Key + ": " + StaleText})
		}
	}
	return problems
}

// flowClosed reports whether a flow row with this status is closed.
func flowClosed(status string) bool {
	return status == "closed"
}

// sprintClosed reports whether a sprint row is closed: status done or
// closed, or phase done.
func sprintClosed(status, phase string) bool {
	return status == "done" || status == "closed" || phase == "done"
}

// parseTime reads an RFC 3339 time; ok is false for an empty or bad value.
func parseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// WaitsDays returns the whole days from updated to now in UTC, rounded
// down; -1 when updated is missing or does not parse, 0 when it lies in
// the future.
func WaitsDays(updated string, now time.Time) int {
	t, ok := parseTime(updated)
	if !ok {
		return -1
	}
	d := now.UTC().Sub(t.UTC())
	if d < 0 {
		return 0
	}
	return int(d / (24 * time.Hour))
}

// newerFirst orders two update times newest first, rows without a time
// after rows with one. It returns -1 when a goes first, 1 when b does,
// 0 when they tie.
func newerFirst(a, b string) int {
	ta, oka := parseTime(a)
	tb, okb := parseTime(b)
	switch {
	case oka && !okb:
		return -1
	case !oka && okb:
		return 1
	case !oka && !okb:
		return 0
	case ta.After(tb):
		return -1
	case tb.After(ta):
		return 1
	}
	return 0
}

// SortFlows orders flow rows: waiting first; then not closed before
// closed; then newest updated_at first, rows without a time last; then
// by key.
func SortFlows(rows []FlowRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Waiting != b.Waiting {
			return a.Waiting
		}
		if a.Closed != b.Closed {
			return !a.Closed
		}
		if c := newerFirst(a.UpdatedAt, b.UpdatedAt); c != 0 {
			return c < 0
		}
		return a.Key < b.Key
	})
}

// SortSprints orders sprint rows: not closed before closed; then newest
// updated_at first, rows without a time last; then by key.
func SortSprints(rows []SprintRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Closed != b.Closed {
			return !a.Closed
		}
		if c := newerFirst(a.UpdatedAt, b.UpdatedAt); c != 0 {
			return c < 0
		}
		return a.Key < b.Key
	})
}

// Pages splits n rows into half-open spans of at most capacity rows. A
// capacity below 1 means 1. No rows means no pages (an empty slice).
func Pages(n, capacity int) []Span {
	if capacity < 1 {
		capacity = 1
	}
	spans := []Span{}
	for from := 0; from < n; from += capacity {
		to := from + capacity
		if to > n {
			to = n
		}
		spans = append(spans, Span{From: from, To: to})
	}
	return spans
}

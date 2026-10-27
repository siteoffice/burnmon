package graph

import (
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/schema"
	"burnmon/internal/store"
)

func TestSprintMainRepo(t *testing.T) {
	for in, want := range map[string]string{
		`C:\w\repo\.worktrees\sprint-x`:  `C:\w\repo`,
		`C:\w\repo\.worktrees\sprint-x\`: `C:\w\repo`,
		`c:/w/repo/.worktrees/sprint-x`:  `c:/w/repo`,
		`C:\w\repo`:                      "",
		"":                               "",
	} {
		if got := SprintMainRepo(in); got != want {
			t.Errorf("SprintMainRepo(%q) = %q, want %q", in, got, want)
		}
	}
}

const (
	linkRepo = `C:\w\repo`
	linkWT   = `C:\w\repo\.worktrees\sprint-x`
)

func linkSession(id, parent, project string, first, last time.Duration) Session {
	return Session{Vendor: "anthropic", Agent: "claude-code", ID: id, ParentID: parent, Project: project, State: "coding",
		FirstActivity: t0.Add(first), LastActivity: t0.Add(last)}
}

func linkSprint(logTimes ...time.Duration) Sprint {
	sp := Sprint{ID: "p__x__1", Slug: "x", WorktreePath: linkWT, Status: "running", UpdatedAt: t0}
	for _, d := range logTimes {
		sp.LogTimes = append(sp.LogTimes, t0.Add(d))
	}
	return sp
}

func linked(g Graph, id string) (Edge, bool) {
	return edge(g, SessionID("anthropic", id), "sprint:p__x__1", KindWorksFor)
}

// A session in the sprint's main repo (an attended sprint's agents run there,
// not in the worktree) is linked to the sprint only while the sprint's own
// action log was being written: some log entry falls inside the time the
// session was active, give or take ten minutes. The link is inferred.
func TestSprintLinkGateOnActionLog(t *testing.T) {
	g := Build(Input{Now: t0, Sprints: []Sprint{linkSprint(-30*time.Minute, -20*time.Minute)}, Sessions: []Session{
		linkSession("orch", "", linkRepo, -5*time.Hour, 0),                      // open the whole time: spans the entries
		linkSession("agent-sub", "orch", linkRepo, -35*time.Minute, -25*time.Minute), // a task's subagent
		linkSession("early", "", linkRepo, -3*time.Hour, -2*time.Hour),          // same repo, hours before
		linkSession("late", "", linkRepo, -1*time.Hour, -45*time.Minute),        // same repo, ended more than ten minutes before the first entry
		linkSession("other", "", `C:\w\other`, -35*time.Minute, -25*time.Minute), // another folder
		linkSession("inwt", "", linkWT, -35*time.Minute, -25*time.Minute),       // in the worktree, in the window
		linkSession("inwt-old", "", linkWT, -6*time.Hour, -5*time.Hour),         // in the worktree, long before
	}})
	for _, id := range []string{"orch", "agent-sub", "inwt"} {
		e, ok := linked(g, id)
		if !ok {
			t.Errorf("%s is not linked to the sprint", id)
			continue
		}
		if e.Basis != BasisInferred || e.Source != SourceForeman {
			t.Errorf("%s link basis %q source %q, want inferred from foreman", id, e.Basis, e.Source)
		}
	}
	for _, id := range []string{"early", "late", "other", "inwt-old"} {
		if _, ok := linked(g, id); ok {
			t.Errorf("%s is linked to the sprint but was not active while its log was written", id)
		}
	}
	if _, ok := node(g, "sprint:p__x__1"); !ok {
		t.Error("a linked sprint is not in the graph")
	}
}

// Without a readable action log, only the worktree folder links, as in R6;
// a session in the main repo never does, since nothing says when it mattered.
func TestSprintLinkWithoutActionLog(t *testing.T) {
	g := Build(Input{Now: t0, Sprints: []Sprint{linkSprint()}, Sessions: []Session{
		linkSession("inwt", "", linkWT, -time.Hour, 0),
		linkSession("inrepo", "", linkRepo, -time.Hour, 0),
	}})
	if _, ok := linked(g, "inwt"); !ok {
		t.Error("worktree session lost its R6 link")
	}
	if _, ok := linked(g, "inrepo"); ok {
		t.Error("a main repo session was linked with no action log to say when")
	}
}

// A live session has no start time in some sources: only its last activity
// counts, and an entry within ten minutes of it links.
func TestSprintLinkLiveSessionWithoutStart(t *testing.T) {
	near := Session{Vendor: "anthropic", Agent: "claude-code", ID: "near", Project: linkRepo, State: "coding", LastActivity: t0.Add(-2 * time.Minute)}
	far := Session{Vendor: "anthropic", Agent: "claude-code", ID: "far", Project: linkRepo, State: "coding", LastActivity: t0.Add(-3 * time.Hour)}
	g := Build(Input{Now: t0, Sprints: []Sprint{linkSprint(-5 * time.Minute)}, Sessions: []Session{near, far}})
	if _, ok := linked(g, "near"); !ok {
		t.Error("a session active 3 minutes from a log entry is not linked")
	}
	if _, ok := linked(g, "far"); ok {
		t.Error("a session active hours from any log entry is linked")
	}
}

// ReadForeman reads the entries' times from the sprint's action-log.ndjson,
// skips lines it cannot read, and keeps them in order.
func TestReadForemanLogTimes(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sprints", "p", "x")
	write(t, filepath.Join(dir, "sprint.json"), `{"sprint_id":"p__x__1","slug":"x","worktree_path":"C:\\w\\repo\\.worktrees\\sprint-x","status":"running","updated_at":"2026-10-23T10:00:00Z"}`)
	write(t, filepath.Join(dir, "action-log.ndjson"),
		`{"ts":"2026-10-23T09:30:00Z","action":"task_done"}`+"\n"+"not json\n"+`{"ts":"2026-10-23T09:00:00Z","action":"task_start"}`+"\n"+`{"action":"no time"}`+"\n")
	sprints, _, err := ReadForeman(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sprints) != 1 || len(sprints[0].LogTimes) != 2 {
		t.Fatalf("sprints = %+v, want one with two log times", sprints)
	}
	if !sprints[0].LogTimes[0].Equal(time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)) || !sprints[0].LogTimes[1].After(sprints[0].LogTimes[0]) {
		t.Errorf("log times %v are not in order", sprints[0].LogTimes)
	}
	// No log at all is fine.
	write(t, filepath.Join(root, "sprints", "p", "y", "sprint.json"), `{"sprint_id":"p__y__1","slug":"y","status":"running"}`)
	sprints, _, err = ReadForeman(root)
	if err != nil || len(sprints) != 2 {
		t.Fatalf("a sprint with no action log: %v, %d sprints", err, len(sprints))
	}
}

// A subagent whose turn ended is finished, not waiting for a person: only a
// Blocked session's own wait is a wait on you.
func TestSubagentWaitingIsNotWaitsOn(t *testing.T) {
	g := Build(Input{Now: t0, Sessions: []Session{
		{Vendor: "anthropic", Agent: "claude-code", ID: "P", State: "waiting", LastActivity: t0},
		{Vendor: "anthropic", Agent: "claude-code", ID: "agent-k", ParentID: "P", State: "waiting", LastActivity: t0},
	}, Agents: []AgentState{{SessionID: "P", State: "blocked"}, {SessionID: "agent-k", State: "blocked"}}})
	if _, ok := edge(g, SessionID("anthropic", "P"), personID, KindWaitsOn); !ok {
		t.Error("a waiting session lost its wait on you")
	}
	if _, ok := edge(g, SessionID("anthropic", "agent-k"), personID, KindWaitsOn); ok {
		t.Error("a finished subagent waits on you")
	}
}

// History gives the same link: the events table gives each session's first
// and last turn, so a finished sprint's agents still link to it.
func TestHistoryLinksSprintFromActionLog(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ev := func(sid, parent, rid, project string, d time.Duration) schema.Event {
		return schema.Event{Vendor: "anthropic", Agent: "claude-code", Surface: "cli", SessionID: sid, ParentID: parent, RequestID: rid,
			Model: "m", Project: project, At: t0.Add(d), Input: 1, Output: 1}
	}
	if err := st.UpsertEvents([]schema.Event{
		ev("orch", "", "r1", linkRepo, -5*time.Hour), ev("orch", "", "r2", linkRepo, -time.Minute),
		ev("agent-sub", "orch", "r3", linkRepo, -33*time.Minute), ev("agent-sub", "orch", "r4", linkRepo, -26*time.Minute),
		ev("early", "", "r5", linkRepo, -3*time.Hour), ev("early", "", "r6", linkRepo, -2*time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	g, err := History(st, HistoryInput{From: t0.Add(-24 * time.Hour), To: t0, Now: t0, Sprints: []Sprint{linkSprint(-30*time.Minute, -20*time.Minute)}})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"orch", "agent-sub"} {
		if _, ok := linked(g, id); !ok {
			t.Errorf("history: %s is not linked to the sprint", id)
		}
	}
	if _, ok := linked(g, "early"); ok {
		t.Error("history: a session from hours before the log entries is linked")
	}
}

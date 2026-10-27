package graph

import "testing"

const (
	coMain = `C:\example\repo`
	coWT   = `C:\example\repo\.worktrees\wt`
)

type mergeCall struct{ repo, branch string }

func mergedFn(calls *[]mergeCall, merged, known bool) func(repo, branch string) (bool, bool) {
	return func(repo, branch string) (bool, bool) {
		*calls = append(*calls, mergeCall{repo, branch})
		if repo == coMain && (branch == "sprint/example-merged" || branch == "feat/example") {
			return merged, known
		}
		return false, false
	}
}

func coSprint(wt string) Sprint {
	return Sprint{ID: "p__x__1", Slug: "x", WorktreePath: wt, Branch: "sprint/example-merged", Status: "waiting-on-you", UpdatedAt: t0}
}

func TestMergedSprintNeedsCloseOut(t *testing.T) {
	var calls []mergeCall
	g := Build(Input{Now: t0, Sprints: []Sprint{coSprint(coWT)}, MergedFor: mergedFn(&calls, true, true)})
	n, ok := node(g, "sprint:p__x__1")
	if !ok || n.Attrs["closeout"] != "true" {
		t.Fatalf("sprint node = %+v, %v; want closeout true", n, ok)
	}
	if _, ok := edge(g, "sprint:p__x__1", personID, KindWaitsOn); ok {
		t.Error("merged sprint still waits on you")
	}
	if len(calls) != 1 || calls[0] != (mergeCall{coMain, "sprint/example-merged"}) {
		t.Errorf("MergedFor calls = %+v", calls)
	}
}

func TestUnmergedSprintStaysBlocked(t *testing.T) {
	var calls []mergeCall
	g := Build(Input{Now: t0, Sprints: []Sprint{coSprint(coWT)}, MergedFor: mergedFn(&calls, false, true)})
	n, _ := node(g, "sprint:p__x__1")
	if n.Attrs["closeout"] != "" {
		t.Error("unmerged sprint marked closeout")
	}
	if _, ok := edge(g, "sprint:p__x__1", personID, KindWaitsOn); !ok {
		t.Error("unmerged sprint lost its waits_on edge")
	}
}

func TestUnknownMergeStaysBlocked(t *testing.T) {
	var calls []mergeCall
	for name, in := range map[string]Input{
		"unknown":          {Now: t0, Sprints: []Sprint{coSprint(coWT)}, MergedFor: mergedFn(&calls, false, false)},
		"unknown-but-true": {Now: t0, Sprints: []Sprint{coSprint(coWT)}, MergedFor: mergedFn(&calls, true, false)},
		"nil":              {Now: t0, Sprints: []Sprint{coSprint(coWT)}},
	} {
		g := Build(in)
		n, _ := node(g, "sprint:p__x__1")
		if n.Attrs["closeout"] != "" {
			t.Errorf("%s: marked closeout", name)
		}
		if _, ok := edge(g, "sprint:p__x__1", personID, KindWaitsOn); !ok {
			t.Errorf("%s: lost its waits_on edge", name)
		}
	}
}

func TestSprintWithoutWorktreeStaysBlocked(t *testing.T) {
	var calls []mergeCall
	g := Build(Input{Now: t0, Sprints: []Sprint{coSprint("")}, MergedFor: mergedFn(&calls, true, true)})
	if len(calls) != 0 {
		t.Errorf("MergedFor called: %+v", calls)
	}
	if _, ok := edge(g, "sprint:p__x__1", personID, KindWaitsOn); !ok {
		t.Error("sprint without a worktree lost its waits_on edge")
	}
}

func coSession(state string, ask bool) Session {
	return Session{Vendor: "anthropic", Agent: "claude-code", ID: "s1", Project: coWT, State: state, AskOpen: ask, LastActivity: t0}
}

func coRepo(string) (Repo, bool) {
	return Repo{Root: coWT, Main: coMain, Branch: "feat/example"}, true
}

func TestMergedWorktreeSessionCanClose(t *testing.T) {
	var calls []mergeCall
	id := "session:anthropic:s1"
	g := Build(Input{Now: t0, Sessions: []Session{coSession("waiting", false)}, RepoFor: coRepo, MergedFor: mergedFn(&calls, true, true)})
	n, _ := node(g, id)
	if n.Attrs["can_close"] != "true" || n.Attrs["wait"] != WaitYourTurn {
		t.Errorf("attrs = %+v; want can_close true, wait your_turn", n.Attrs)
	}
	// Blocked on a stale question: can_close set, wait stays blocked, edge dropped.
	g = Build(Input{Now: t0, Sessions: []Session{coSession("waiting", true)}, RepoFor: coRepo, MergedFor: mergedFn(&calls, true, true)})
	n, _ = node(g, id)
	if n.Attrs["can_close"] != "true" || n.Attrs["wait"] != WaitBlocked {
		t.Errorf("blocked attrs = %+v; want can_close true, wait blocked", n.Attrs)
	}
	if _, ok := edge(g, id, personID, KindWaitsOn); ok {
		t.Error("merged worktree session still draws a waits_on edge")
	}
	// Unmerged: untouched.
	g = Build(Input{Now: t0, Sessions: []Session{coSession("waiting", true)}, RepoFor: coRepo, MergedFor: mergedFn(&calls, false, true)})
	n, _ = node(g, id)
	if n.Attrs["can_close"] != "" {
		t.Error("unmerged session marked can_close")
	}
	if _, ok := edge(g, id, personID, KindWaitsOn); !ok {
		t.Error("unmerged blocked session lost its edge")
	}
}

func TestCanCloseNotForSprintSession(t *testing.T) {
	var calls []mergeCall
	sp := coSprint(coWT)
	sp.Status = "running"
	g := Build(Input{Now: t0, Sessions: []Session{coSession("waiting", false)}, RepoFor: coRepo, Sprints: []Sprint{sp}, MergedFor: mergedFn(&calls, true, true)})
	if _, ok := edge(g, "session:anthropic:s1", "sprint:p__x__1", KindWorksFor); !ok {
		t.Fatal("fixture: session not linked to the sprint")
	}
	n, _ := node(g, "session:anthropic:s1")
	if n.Attrs["can_close"] != "" {
		t.Error("a sprint's session is marked can_close")
	}
}

func TestCanCloseNotForMainCheckout(t *testing.T) {
	var calls []mergeCall
	repo := func(string) (Repo, bool) { return Repo{Root: coMain, Branch: "feat/example"}, true }
	g := Build(Input{Now: t0, Sessions: []Session{coSession("waiting", false)}, RepoFor: repo, MergedFor: mergedFn(&calls, true, true)})
	n, _ := node(g, "session:anthropic:s1")
	if n.Attrs["can_close"] != "" {
		t.Error("a main checkout session is marked can_close")
	}
}

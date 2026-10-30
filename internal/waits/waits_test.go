package waits

import (
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"burnmon/internal/graph"
)

var now = time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)

func openFlow() graph.Flow {
	return graph.Flow{ID: "f1", Slug: "example-flow", Gate: "G4", GatePending: true, Project: "example",
		GateOpenedAt: now.Add(-2 * time.Hour), SpecPath: `C:\example\specs\a.md`}
}

func TestOpenGateIsAWait(t *testing.T) {
	decided := graph.Flow{ID: "f2", Slug: "done-flow", Gate: "G3", GatePending: false}
	w, _ := Build(Input{Now: now, Flows: []graph.Flow{openFlow(), decided}})
	want := []Item{{Kind: "gate", ID: "gate:f1:G4", Title: "example-flow", Project: "example", Detail: "G4",
		Since: "2026-10-23T08:00:00Z", AgeSeconds: 7200, SpecPath: `C:\example\specs\a.md`, Source: "foreman", Group: "blocked", Holds: 1}}
	if !reflect.DeepEqual(w, want) {
		t.Fatalf("got %+v", w)
	}
}

func TestGateResultFromSprintWithSameSpec(t *testing.T) {
	sp := []graph.Sprint{
		{ID: "s1", SpecPath: `c:\EXAMPLE\specs\a.md`, ResultPath: "r-old", UpdatedAt: now.Add(-5 * time.Hour)},
		{ID: "s2", SpecPath: `c:\EXAMPLE\specs\a.md`, ResultPath: "r-new", UpdatedAt: now.Add(-time.Hour)},
		{ID: "s3", SpecPath: `C:\example\specs\other.md`, ResultPath: "r-other", UpdatedAt: now},
	}
	w, _ := Build(Input{Now: now, Flows: []graph.Flow{openFlow()}, Sprints: sp})
	if len(w) != 1 || w[0].ResultPath != "r-new" {
		t.Fatalf("got %+v", w)
	}
	w, _ = Build(Input{Now: now, Flows: []graph.Flow{openFlow()}})
	if w[0].ResultPath != "" {
		t.Fatalf("got %+v", w)
	}
}

func TestWaitingSprintWithParkedTasks(t *testing.T) {
	sp := graph.Sprint{ID: "sp1", Slug: "sprint-example", Project: "example", Status: "waiting-on-you",
		SpecPath: `C:\example\specs\s.md`, ResultPath: `C:\example\r.md`,
		UpdatedAt:    now.Add(-3 * time.Hour),
		LastParkedAt: now.Add(-10 * time.Minute),
		Parked:       []graph.ParkedTask{{N: 1, Title: "one", Reason: "needs a key"}, {N: 3, Title: "three"}}}
	w, _ := Build(Input{Now: now, Sprints: []graph.Sprint{sp}})
	if len(w) != 1 {
		t.Fatalf("got %+v", w)
	}
	it := w[0]
	if it.Kind != "sprint" || it.ID != "sprint:sp1" || it.Detail != "2 tasks parked" || it.AgeSeconds != 600 ||
		it.SpecPath != sp.SpecPath || it.ResultPath != sp.ResultPath || it.Source != "foreman" || it.Project != "example" {
		t.Fatalf("item = %+v", it)
	}
	if !reflect.DeepEqual(it.Parked, []Parked{{1, "one", "needs a key"}, {3, "three", ""}}) {
		t.Fatalf("parked = %+v", it.Parked)
	}
	sp.Parked = sp.Parked[:1]
	w, _ = Build(Input{Now: now, Sprints: []graph.Sprint{sp}})
	if w[0].Detail != "1 task parked" {
		t.Fatalf("detail = %q", w[0].Detail)
	}
	sp.LastParkedAt = time.Time{}
	w, _ = Build(Input{Now: now, Sprints: []graph.Sprint{sp}})
	if w[0].AgeSeconds != 3*3600 {
		t.Fatalf("age = %d", w[0].AgeSeconds)
	}
}

func TestFailedVerifyOnStoppedSprintWaits(t *testing.T) {
	sp := graph.Sprint{ID: "sp2", Slug: "capped-example", Status: "capped", UpdatedAt: now.Add(-time.Hour),
		Verify: &graph.Check{Failed: true, Label: "verify fail", Summary: "verify broke", At: now.Add(-3 * time.Hour)}}
	w, o := Build(Input{Now: now, Sprints: []graph.Sprint{sp}})
	if len(w) != 1 || w[0].Kind != "failed_check" || w[0].ID != "check:sp2:verify" || w[0].Detail != "verify fail: verify broke" {
		t.Fatalf("wait = %+v", w)
	}
	if len(o) != 1 || o[0].ID != "check:sp2:verify" {
		t.Fatalf("outcomes = %+v", o)
	}
	sp.Status = "running"
	w, o = Build(Input{Now: now, Sprints: []graph.Sprint{sp}})
	if len(w) != 0 || len(o) != 1 {
		t.Fatalf("running: %+v %+v", w, o)
	}
	sp.Status = "failed"
	sp.Verify = nil
	sp.Test = &graph.Check{Failed: true, Label: "test exit 1", Summary: "two failed", At: now.Add(-time.Hour)}
	w, _ = Build(Input{Now: now, Sprints: []graph.Sprint{sp}})
	if len(w) != 1 || w[0].ID != "check:sp2:test" || w[0].Detail != "test exit 1: two failed" {
		t.Fatalf("test check = %+v", w)
	}
	sp.Test = &graph.Check{Failed: false, Label: "test exit 0"}
	w, o = Build(Input{Now: now, Sprints: []graph.Sprint{sp}})
	if len(w) != 0 || len(o) != 1 || o[0].Kind != "finished" || o[0].Detail != "failed, test pass, verify -" {
		t.Fatalf("passing check listed as failed: %+v %+v", w, o)
	}
}

func TestOutcomesKeepOnlyLast24Hours(t *testing.T) {
	pass := &graph.Check{Label: "test exit 0"}
	sps := []graph.Sprint{
		{ID: "a", Slug: "a", Phase: "done", Status: "passing", UpdatedAt: now.Add(-23 * time.Hour), Test: pass},
		{ID: "b", Slug: "b", Phase: "done", Status: "passing", UpdatedAt: now.Add(-25 * time.Hour), Test: pass},
		{ID: "c", Slug: "c", Status: "failed", UpdatedAt: now,
			Test: &graph.Check{Failed: true, Label: "test exit 2", At: now.Add(-25 * time.Hour)}},
	}
	_, o := Build(Input{Now: now, Sprints: sps})
	// c failed with a check older than the window, but the sprint itself ended inside it.
	if len(o) != 2 || o[0].ID != "sprint:c" || o[0].Detail != "failed, test fail, verify -" ||
		o[1].ID != "sprint:a" || o[1].Kind != "finished" || o[1].Detail != "test pass, verify -" {
		t.Fatalf("outcomes = %+v", o)
	}
	sps[0].Verify = &graph.Check{Failed: true, Label: "verify fail"}
	sps[0].Test = nil
	_, o = Build(Input{Now: now, Sprints: sps[:1]})
	// A done sprint with a failing check inside the window is one outcome row,
	// the failed_check; it is not listed a second time as finished.
	if len(o) != 1 || o[0].Kind != KindFailedCheck || o[0].ID != "check:a:verify" {
		t.Fatalf("done sprint with a failing check listed twice or not at all: %+v", o)
	}
}

func sessionGraph() graph.Graph {
	return graph.Graph{
		Nodes: []graph.Node{
			{ID: "session:anthropic:s1", Kind: graph.KindSession, Label: "proj"},
			{ID: "sprint:sp1", Kind: graph.KindSprint, Label: "sp1"},
			{ID: "person:you", Kind: graph.KindPerson, Label: "You"},
		},
		Edges: []graph.Edge{
			{From: "session:anthropic:s1", To: "person:you", Kind: graph.KindWaitsOn, Source: "claude_pids", AgeSeconds: 40},
			{From: "session:anthropic:s1", To: "sprint:sp1", Kind: graph.KindWorksFor, Source: "store"},
			{From: "sprint:sp1", To: "person:you", Kind: graph.KindWaitsOn, Source: "foreman", AgeSeconds: 5},
		},
	}
}

func sp1() graph.Sprint {
	return graph.Sprint{ID: "sp1", Slug: "sp1", Status: "running", SpecPath: `C:\example\specs\s.md`, ResultPath: `C:\example\r.md`, UpdatedAt: now}
}

func TestSessionWaitFromGraphEdge(t *testing.T) {
	w, _ := Build(Input{Now: now, Graph: sessionGraph(), Sprints: []graph.Sprint{sp1()}})
	if len(w) != 1 {
		t.Fatalf("got %+v", w)
	}
	it := w[0]
	if it.Kind != "session" || it.ID != "session:anthropic:s1" || it.Title != "proj s1" || it.AgeSeconds != 40 ||
		it.Source != "claude_pids" || it.SpecPath != `C:\example\specs\s.md` || it.ResultPath != `C:\example\r.md` ||
		it.Since != "2026-10-23T09:59:20Z" {
		t.Fatalf("item = %+v", it)
	}
}

func TestAgentEntryReplacesSessionItem(t *testing.T) {
	in := Input{Now: now, Graph: sessionGraph(), Sprints: []graph.Sprint{sp1()}}
	e := AgentEntry{SessionID: "s1", Status: "waiting", WaitingFor: "permission prompt", Cwd: `C:\example\proj`, Since: now.Add(-15 * time.Second)}
	in.Agents = []AgentEntry{e}
	w, _ := Build(in)
	if len(w) != 1 {
		t.Fatalf("got %+v", w)
	}
	it := w[0]
	if it.Kind != "prompt" || it.ID != "session:anthropic:s1" || it.Detail != "permission prompt" || it.Source != "agent_view" ||
		it.AgeSeconds != 15 || it.SpecPath != `C:\example\specs\s.md` || it.ResultPath != `C:\example\r.md` {
		t.Fatalf("item = %+v", it)
	}
	in.Agents[0].WaitingFor = "input needed"
	w, _ = Build(in)
	if w[0].Kind != "question" || w[0].Detail != "input needed" {
		t.Fatalf("question = %+v", w[0])
	}
	blocked := []AgentEntry{{ID: "ab12", State: "blocked", Cwd: `C:\example\proj`, Since: now.Add(-9 * time.Second)}}
	w, _ = Build(Input{Now: now, Agents: blocked})
	if len(w) != 1 || w[0].Kind != "session" || w[0].ID != "agentview:ab12" || w[0].Detail != "blocked" || w[0].Title != "proj ab12" {
		t.Fatalf("blocked = %+v", w)
	}
	in.Agents = []AgentEntry{{SessionID: "s1", Status: "busy", Cwd: `C:\example\proj`}}
	w, _ = Build(in)
	if len(w) != 1 || w[0].Source != "claude_pids" {
		t.Fatalf("busy entry should leave the graph item: %+v", w)
	}
	w, _ = Build(Input{Now: now, Agents: []AgentEntry{{PID: 3, Status: "busy"}}})
	if len(w) != 0 {
		t.Fatalf("busy alone: %+v", w)
	}
}

func TestWaitsOldestFirstOutcomesNewestFirst(t *testing.T) {
	mk := func(id string, age time.Duration) graph.Sprint {
		return graph.Sprint{ID: id, Slug: id, Status: "waiting-on-you", UpdatedAt: now.Add(-age)}
	}
	w, _ := Build(Input{Now: now, Sprints: []graph.Sprint{mk("c", time.Hour), mk("b", time.Minute), mk("a", time.Minute)}})
	var ids []string
	for _, it := range w {
		ids = append(ids, it.ID)
	}
	if !reflect.DeepEqual(ids, []string{"sprint:c", "sprint:a", "sprint:b"}) {
		t.Fatalf("ids = %v", ids)
	}
	fin := func(id string, age time.Duration) graph.Sprint {
		return graph.Sprint{ID: id, Slug: id, Phase: "done", UpdatedAt: now.Add(-age)}
	}
	_, o := Build(Input{Now: now, Sprints: []graph.Sprint{fin("y", 2*time.Hour), fin("x", time.Hour), fin("z", time.Hour)}})
	ids = nil
	for _, it := range o {
		ids = append(ids, it.ID)
	}
	if !reflect.DeepEqual(ids, []string{"sprint:x", "sprint:z", "sprint:y"}) {
		t.Fatalf("outcome ids = %v", ids)
	}
}

func TestDetailCutAt120Runes(t *testing.T) {
	sum := strings.Repeat("\u00e9\u4e2d", 150)
	sp := graph.Sprint{ID: "x", Slug: "x", Status: "capped", UpdatedAt: now,
		Verify: &graph.Check{Failed: true, Label: "verify fail", Summary: sum, At: now}}
	w, _ := Build(Input{Now: now, Sprints: []graph.Sprint{sp}})
	d := w[0].Detail
	if !utf8.ValidString(d) || utf8.RuneCountInString(d) > DetailMax+3 || !strings.HasSuffix(d, "...") {
		t.Fatalf("detail = %q (%d runes)", d, utf8.RuneCountInString(d))
	}
}

func TestEmptyInputGivesEmptyLists(t *testing.T) {
	w, o := Build(Input{Now: now})
	if w == nil || o == nil || len(w) != 0 || len(o) != 0 {
		t.Fatalf("got %#v %#v", w, o)
	}
}

func TestStoppedSprintsAreOutcomesWithStatusFirst(t *testing.T) {
	chk := func(failed bool) *graph.Check {
		return &graph.Check{Failed: failed, Label: "x", At: now.Add(-time.Hour)}
	}
	for _, status := range []string{"capped", "failed", "escalated"} {
		sp := graph.Sprint{ID: "s-" + status, Slug: status, Status: status, UpdatedAt: now.Add(-2 * time.Hour)}
		_, o := Build(Input{Now: now, Sprints: []graph.Sprint{sp}})
		if len(o) != 1 || o[0].Kind != "finished" || o[0].ID != "sprint:s-"+status || o[0].Detail != status+", test -, verify -" {
			t.Fatalf("%s without check: %+v", status, o)
		}
		sp.Test = chk(false)
		sp.Verify = chk(false)
		_, o = Build(Input{Now: now, Sprints: []graph.Sprint{sp}})
		if len(o) != 1 || o[0].Detail != status+", test pass, verify pass" {
			t.Fatalf("%s passing checks: %+v", status, o)
		}
		// A failing check inside the window keeps its failed_check item only.
		sp.Verify = chk(true)
		_, o = Build(Input{Now: now, Sprints: []graph.Sprint{sp}})
		if len(o) != 1 || o[0].Kind != "failed_check" || o[0].ID != "check:s-"+status+":verify" {
			t.Fatalf("%s failing check: %+v", status, o)
		}
		// A failing check older than the window gives no failed_check item, so the sprint shows as finished.
		sp.Verify = &graph.Check{Failed: true, Label: "x", At: now.Add(-30 * time.Hour)}
		_, o = Build(Input{Now: now, Sprints: []graph.Sprint{sp}})
		if len(o) != 1 || o[0].Kind != "finished" || o[0].Detail != status+", test pass, verify fail" {
			t.Fatalf("%s old failing check: %+v", status, o)
		}
	}
}

func TestPassingAndDoneStatusAreFinishedWithin24Hours(t *testing.T) {
	for _, status := range []string{"passing", "done"} {
		in := func(age time.Duration) Input {
			return Input{Now: now, Sprints: []graph.Sprint{{ID: "p", Slug: "p", Status: status, UpdatedAt: now.Add(-age)}}}
		}
		_, o := Build(in(OutcomeWindow))
		if len(o) != 1 || o[0].Kind != "finished" || o[0].Detail != "test -, verify -" {
			t.Fatalf("%s at the boundary: %+v", status, o)
		}
		_, o = Build(in(OutcomeWindow + time.Second))
		if len(o) != 0 {
			t.Fatalf("%s past the boundary: %+v", status, o)
		}
	}
	// Stopped sprint boundary.
	sp := graph.Sprint{ID: "c", Slug: "c", Status: "capped", UpdatedAt: now.Add(-OutcomeWindow)}
	_, o := Build(Input{Now: now, Sprints: []graph.Sprint{sp}})
	if len(o) != 1 {
		t.Fatalf("capped at boundary: %+v", o)
	}
	sp.UpdatedAt = now.Add(-OutcomeWindow - time.Second)
	_, o = Build(Input{Now: now, Sprints: []graph.Sprint{sp}})
	if len(o) != 0 {
		t.Fatalf("capped past boundary: %+v", o)
	}
}

// A session item never shows a raw id as its title: its folder and a short
// id, else its agent, else "session". Gate and sprint titles keep their slug,
// and the count does not move.
func TestSessionTitlesNeverShowARawID(t *testing.T) {
	const uuid = "ec3c0d3c-c753-54e3-b994-c3a39e7505fc"
	g := graph.Graph{
		Nodes: []graph.Node{
			// The laptop case: a cowork-cloud session has no project folder.
			{ID: "session:anthropic:" + uuid, Kind: graph.KindSession, Label: uuid, Source: "store",
				Attrs: map[string]string{"agent": "cowork-cloud", "state": "waiting", "wait": "your_turn"}},
			{ID: "session:anthropic:c1d2e3f4a5b6", Kind: graph.KindSession, Label: "c1d2e3f4a5b6", Source: "store",
				Attrs: map[string]string{"agent": "claude-code", "wait": "your_turn", "can_close": "true"}},
			{ID: "session:anthropic:b1b2b3b4b5b6", Kind: graph.KindSession, Label: "burnmon", Source: "store",
				Attrs: map[string]string{"agent": "claude-code", "wait": "blocked"}},
			{ID: personID, Kind: graph.KindPerson, Label: "You"},
		},
		Edges: []graph.Edge{{From: "session:anthropic:b1b2b3b4b5b6", To: personID, Kind: graph.KindWaitsOn, Source: "store"}},
	}
	in := Input{Now: now, Graph: g, Flows: []graph.Flow{openFlow()},
		Sprints: []graph.Sprint{{ID: "sp1", Slug: "sprint-example", Status: "waiting-on-you", UpdatedAt: now}},
		Agents: []AgentEntry{
			{ID: "9f8e7d6c5b4a", State: "blocked"},
			{SessionID: "a0a1a2a3a4a5", Status: "waiting", WaitingFor: "permission prompt", Cwd: `C:\example\proj`},
		}}
	w, _ := Build(in)
	titles := map[string]string{}
	for _, it := range w {
		titles[it.Kind+" "+it.ID] = it.Title
	}
	want := map[string]string{
		"your_turn session:anthropic:" + uuid:               "cowork-cloud ec3c0d3c",
		"can_close canclose:session:anthropic:c1d2e3f4a5b6": "claude-code e3f4a5b6",
		"session session:anthropic:b1b2b3b4b5b6":            "burnmon b3b4b5b6",
		"session agentview:9f8e7d6c5b4a":                    "session 7d6c5b4a",
		"prompt session:anthropic:a0a1a2a3a4a5":             "proj a2a3a4a5",
		"gate gate:f1:G4":                                   "example-flow",
		"sprint sprint:sp1":                                 "sprint-example",
	}
	if !reflect.DeepEqual(titles, want) {
		t.Fatalf("titles = %v", titles)
	}
	if c := CountOf(w); c != (Counts{Blocked: 5, YourTurn: 1, Done: 1}) {
		t.Fatalf("counts = %+v", c)
	}
}

func TestWaitingForMapping(t *testing.T) {
	cases := []struct{ waitingFor, kind string }{
		{"permission prompt", "prompt"},
		{"input needed", "question"},
		{"sandbox request", "session"},
		{"worker request", "session"},
		{"dialog open", "session"},
	}
	for _, c := range cases {
		a := AgentEntry{SessionID: "s9", Status: "waiting", WaitingFor: c.waitingFor, Cwd: `C:\example\proj`}
		w, _ := Build(Input{Now: now, Agents: []AgentEntry{a}})
		if len(w) != 1 || w[0].Kind != c.kind || w[0].Detail != c.waitingFor {
			t.Fatalf("%s: %+v", c.waitingFor, w)
		}
	}
}

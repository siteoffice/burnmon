package centre

import "testing"

// laneBase is a repo checkout that is clean, saved, shared and merged on the
// default branch with PRs on; each case changes what it tests.
func laneBase() LaneIn {
	return LaneIn{StatusRead: true, Upstream: "origin/main", OnDefault: true, PRsOn: true, GitHub: true,
		LacksD: 0, InOriginD: 1, Tags: false, SinceTag: 0}
}

type laneCase struct {
	name  string
	mod   func(*LaneIn)
	stage string
	state string
	note  string
}

// TestLaneOfTable: one row per state of every stage, written before LaneOf.
func TestLaneOfTable(t *testing.T) {
	pr := func(n int, state string) *GitPR { return &GitPR{Number: n, State: state, Checks: "pass"} }
	cases := []laneCase{
		// Changes
		{"changes conflicts", func(l *LaneIn) { l.Conflicts, l.Dirty = 2, 2 }, "Changes", "bad", "2 conflicts"},
		{"changes dirty", func(l *LaneIn) { l.Dirty = 4 }, "Changes", "active", "4 files"},
		{"changes clean", func(l *LaneIn) {}, "Changes", "done", "clean"},
		{"changes not read", func(l *LaneIn) { l.StatusRead = false }, "Changes", "off", "?"},
		{"changes detached", func(l *LaneIn) { l.Detached = true }, "Changes", "bad", "detached"},
		// Saved
		{"saved ahead of upstream", func(l *LaneIn) { l.Ahead = 3 }, "Saved", "active", "3 to share"},
		{"saved no upstream lacks D", func(l *LaneIn) { l.Upstream, l.OnDefault, l.LacksD = "", false, 2 }, "Saved", "active", "2 to share"},
		{"saved dirty nothing ahead", func(l *LaneIn) { l.Dirty = 1 }, "Saved", "todo", ""},
		{"saved all saved", func(l *LaneIn) {}, "Saved", "done", "all saved"},
		{"saved count unknown", func(l *LaneIn) { l.Upstream, l.OnDefault, l.LacksD = "", false, -1 }, "Saved", "off", "?"},
		{"saved ahead unknown", func(l *LaneIn) { l.AheadUnknown = true }, "Saved", "off", "?"},
		// Shared
		{"shared ahead unknown", func(l *LaneIn) { l.AheadUnknown = true }, "Shared", "off", "?"},
		{"shared no upstream", func(l *LaneIn) { l.Upstream = "" }, "Shared", "todo", "not online"},
		{"shared waiting", func(l *LaneIn) { l.Ahead = 2 }, "Shared", "todo", "2 waiting"},
		{"shared behind", func(l *LaneIn) { l.Behind = 5 }, "Shared", "warn", "5 new online"},
		{"shared up to date", func(l *LaneIn) {}, "Shared", "done", "up to date"},
		// Review
		{"review off", func(l *LaneIn) { l.PRsOn, l.OnDefault = false, false }, "Review", "off", "PRs off"},
		{"review on default", func(l *LaneIn) {}, "Review", "done", ""},
		{"review no github", func(l *LaneIn) { l.OnDefault, l.GitHub = false, false }, "Review", "off", "no GitHub"},
		{"review not asked", func(l *LaneIn) { l.OnDefault = false }, "Review", "todo", "not asked"},
		{"review merged", func(l *LaneIn) { l.OnDefault, l.PR = false, pr(7, "MERGED") }, "Review", "done", "#7"},
		{"review checks failed", func(l *LaneIn) {
			l.OnDefault, l.PR = false, &GitPR{Number: 8, State: "OPEN", Checks: "fail"}
		}, "Review", "bad", "#8 checks failed"},
		{"review changes asked", func(l *LaneIn) {
			l.OnDefault, l.PR = false, &GitPR{Number: 9, State: "OPEN", Review: "CHANGES_REQUESTED"}
		}, "Review", "bad", "#9 changes asked"},
		{"review checks running", func(l *LaneIn) {
			l.OnDefault, l.PR = false, &GitPR{Number: 10, State: "OPEN", Checks: "pending"}
		}, "Review", "warn", "#10 checks running"},
		{"review approved", func(l *LaneIn) {
			l.OnDefault, l.PR = false, &GitPR{Number: 11, State: "OPEN", Checks: "pass", Review: "APPROVED"}
		}, "Review", "done", "#11"},
		{"review open", func(l *LaneIn) { l.OnDefault, l.PR = false, pr(12, "OPEN") }, "Review", "warn", "#12 open"},
		{"review closed unmerged", func(l *LaneIn) { l.OnDefault, l.PR = false, pr(13, "CLOSED") }, "Review", "todo", "#13 closed"},
		// Merged
		{"merged on default", func(l *LaneIn) {}, "Merged", "done", ""},
		{"merged by pull request", func(l *LaneIn) { l.OnDefault, l.InOriginD, l.PR = false, 0, pr(7, "MERGED") }, "Merged", "done", "#7"},
		{"merged tip in origin", func(l *LaneIn) { l.OnDefault, l.InOriginD = false, 1 }, "Merged", "done", "in origin"},
		{"merged not", func(l *LaneIn) { l.OnDefault, l.InOriginD = false, 0 }, "Merged", "todo", ""},
		{"merged unknown", func(l *LaneIn) { l.OnDefault, l.InOriginD = false, -1 }, "Merged", "off", "?"},
		// Released
		{"released no tags", func(l *LaneIn) {}, "Released", "done", "no tags"},
		{"released at head", func(l *LaneIn) { l.Tags, l.Tag, l.TagAtHead = true, "v0.4.0", true }, "Released", "done", "v0.4.0"},
		{"released since", func(l *LaneIn) { l.Tags, l.Tag, l.SinceTag = true, "v0.4.0-beta.4", 2 }, "Released", "warn", "2 since v0.4.0-beta.4"},
		{"released since unknown", func(l *LaneIn) { l.Tags, l.Tag, l.SinceTag = true, "v1.0.0", -1 }, "Released", "warn", "after v1.0.0"},
		{"released not read", func(l *LaneIn) { l.TagsUnknown = true }, "Released", "off", "?"},
		{"released of a feature branch not merged", func(l *LaneIn) {
			l.OnDefault, l.InOriginD, l.Tags, l.Tag, l.SinceTag = false, 0, true, "v1", 4
		}, "Released", "todo", ""},
		{"released of a merged feature branch", func(l *LaneIn) {
			l.OnDefault, l.InOriginD, l.Tags, l.Tag, l.TagAtHead = false, 1, true, "v1", true
		}, "Released", "done", "v1"},
	}
	for _, c := range cases {
		in := laneBase()
		c.mod(&in)
		lane := LaneOf(in)
		if len(lane) != len(LaneStages) {
			t.Fatalf("%s: lane has %d stages", c.name, len(lane))
		}
		var got LaneStage
		for _, s := range lane {
			if s.Stage == c.stage {
				got = s
			}
		}
		if got.State != c.state || got.Note != c.note {
			t.Errorf("%s: %s = %s %q, want %s %q", c.name, c.stage, got.State, got.Note, c.state, c.note)
		}
	}
}

func TestLaneStagesOrder(t *testing.T) {
	want := []string{"Changes", "Saved", "Shared", "Review", "Merged", "Released"}
	for i, w := range want {
		if LaneStages[i] != w {
			t.Fatalf("LaneStages = %v", LaneStages)
		}
	}
}

// TestStopsAt: the first stage that is not done; Review "off" is skipped
// (no pull request data says nothing), Changes "off" is not.
func TestStopsAt(t *testing.T) {
	mk := func(states ...string) []LaneStage {
		out := make([]LaneStage, len(LaneStages))
		for i := range out {
			out[i] = LaneStage{Stage: LaneStages[i], State: "done"}
		}
		for i, s := range states {
			out[i].State = s
			out[i].Note = "n" + LaneStages[i]
		}
		return out
	}
	cases := []struct {
		lane      []LaneStage
		stage     string
		note      string
		published bool
	}{
		{mk(), "", "", true},
		{mk("active"), "Changes", "nChanges", false},
		{mk("done", "done", "done", "off", "done", "done"), "", "", true},
		{mk("done", "done", "done", "off", "todo", "todo"), "Merged", "nMerged", false},
		{mk("off"), "Changes", "nChanges", false},
		{mk("done", "done", "done", "done", "done", "warn"), "Released", "nReleased", false},
	}
	for i, c := range cases {
		stage, note, pub := StopsAt(c.lane)
		if stage != c.stage || note != c.note || pub != c.published {
			t.Errorf("case %d: got %q %q %v, want %q %q %v", i, stage, note, pub, c.stage, c.note, c.published)
		}
	}
}

// TestLaneSummary counts open repos per stage; a published repo counts for none.
func TestLaneSummary(t *testing.T) {
	repos := []GitRepo{
		{StopsAt: "Changes", Published: false},
		{StopsAt: "Review", Published: false},
		{StopsAt: "Review", Published: false},
		{StopsAt: "", Published: true},
	}
	s := LaneSummaryOf(repos)
	if s.Open != 3 || s.Published != 1 || s.Total != 4 || s.Stages["Review"] != 2 || s.Stages["Changes"] != 1 || s.Stages["Released"] != 0 {
		t.Errorf("summary = %+v", s)
	}
	if len(s.Stages) != len(LaneStages) {
		t.Errorf("summary has %d stage counts", len(s.Stages))
	}
}

// TestSortReposByLane: open repos first by the earliest stage they stop at,
// published repos last.
func TestSortReposByLane(t *testing.T) {
	repos := []GitRepo{
		{ID: "pub", Published: true, StopsAt: "", LastAt: "2026-11-16T10:00:00Z"},
		{ID: "rel", StopsAt: "Released", LastAt: "2026-11-16T09:00:00Z"},
		{ID: "chg", StopsAt: "Changes", LastAt: "2026-11-01T09:00:00Z"},
		{ID: "rev", StopsAt: "Review", LastAt: "2026-11-16T09:00:00Z"},
	}
	SortReposByLane(repos)
	got := repos[0].ID + repos[1].ID + repos[2].ID + repos[3].ID
	if got != "chgrevrelpub" {
		t.Errorf("order = %s", got)
	}
}

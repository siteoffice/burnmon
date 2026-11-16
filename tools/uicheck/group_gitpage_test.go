package main

import (
	"strings"
	"testing"
)

func fixtureRead() gitpageRead {
	r := gitpageRead{Big: "4 of 5 repos open"}
	for _, name := range []string{"pub", "dirty", "ahead", "behind", "feat"} {
		want := gitpageWantLanes[name]
		r.Rows = append(r.Rows, gitpageRow{Name: name, Stop: want.Stop, Dots: append([]string(nil), want.Dots...)})
	}
	r.Strip = []gitpageStrip{{N: 1}, {N: 2}, {N: 1}, {N: 0}, {N: 0}, {N: 0}}
	return r
}

// The fixture the launcher builds: five repos, and the stage each stops at.
func TestGitpageFixtureTable(t *testing.T) {
	want := map[string]string{"pub": "Published", "dirty": "Changes", "ahead": "Saved", "behind": "Shared", "feat": "Saved"}
	for name, stop := range want {
		got := gitpageWantLanes[name].Stop
		if !strings.HasPrefix(got, stop) {
			t.Errorf("%s stops at %q, want it to start with %q", name, got, stop)
		}
		if len(gitpageWantLanes[name].Dots) != 6 {
			t.Errorf("%s has %d dots, want 6", name, len(gitpageWantLanes[name].Dots))
		}
	}
	if len(gitpageWantLanes) != len(want) {
		t.Errorf("fixture table has %d repos, want %d", len(gitpageWantLanes), len(want))
	}
}

func TestGitpageStripProblems(t *testing.T) {
	r := fixtureRead()
	if p := gitpageStripProblems(r, 5); len(p) != 0 {
		t.Fatalf("a consistent read has problems: %v", p)
	}
	r = fixtureRead()
	r.Strip[1].N = 3
	if p := gitpageStripProblems(r, 5); len(p) == 0 {
		t.Error("a strip count that differs from the rows was not reported")
	}
	r = fixtureRead()
	r.Big = "3 of 5 repos open"
	if p := gitpageStripProblems(r, 5); len(p) == 0 {
		t.Error("a headline that differs from the rows was not reported")
	}
	r = fixtureRead()
	r.Rows = r.Rows[:4]
	if p := gitpageStripProblems(r, 5); len(p) == 0 {
		t.Error("a missing row was not reported")
	}
	r = fixtureRead()
	r.Strip = r.Strip[:5]
	if p := gitpageStripProblems(r, 5); len(p) == 0 {
		t.Error("a strip with five stages was not reported")
	}
}

func TestGitpageLaneProblems(t *testing.T) {
	r := fixtureRead()
	if p := gitpageLaneProblems(r); len(p) != 0 {
		t.Fatalf("a consistent read has problems: %v", p)
	}
	r = fixtureRead()
	r.Rows[1].Dots[0] = "done"
	if p := gitpageLaneProblems(r); len(p) == 0 {
		t.Error("a wrong dot was not reported")
	}
	r = fixtureRead()
	r.Rows[2].Stop = "Published"
	if p := gitpageLaneProblems(r); len(p) == 0 {
		t.Error("a wrong stop was not reported")
	}
	r = fixtureRead()
	r.Rows = append(r.Rows, gitpageRow{Name: "stranger", Stop: "Changes", Dots: make([]string, 6)})
	if p := gitpageLaneProblems(r); len(p) == 0 {
		t.Error("a row that is not in the fixture was not reported")
	}
}

func TestGitpageFilterProblems(t *testing.T) {
	r := gitpageRead{Rows: []gitpageRow{{Name: "ahead", Stop: "Saved: 1 to share"}, {Name: "feat", Stop: "Saved: 1 to share"}}}
	if p := gitpageFilterProblems(r, "Saved", []string{"ahead", "feat"}); len(p) != 0 {
		t.Fatalf("a correct filter has problems: %v", p)
	}
	if p := gitpageFilterProblems(r, "Saved", []string{"ahead"}); len(p) == 0 {
		t.Error("a missing repo was not reported")
	}
	r.Rows = append(r.Rows, gitpageRow{Name: "dirty", Stop: "Changes: 1 file"})
	if p := gitpageFilterProblems(r, "Saved", []string{"ahead", "feat"}); len(p) == 0 {
		t.Error("a repo that stops elsewhere was not reported")
	}
}

func TestGitpageWidthProblems(t *testing.T) {
	ok := gitpageWidth{PaneScrollW: 700, PaneClientW: 700, DocScrollW: 1200, InnerW: 1200}
	if p := gitpageWidthProblems(ok); len(p) != 0 {
		t.Fatalf("a fitting page has problems: %v", p)
	}
	if p := gitpageWidthProblems(gitpageWidth{PaneScrollW: 720, PaneClientW: 700, DocScrollW: 1200, InnerW: 1200}); len(p) == 0 {
		t.Error("a pane wider than itself was not reported")
	}
	if p := gitpageWidthProblems(gitpageWidth{PaneScrollW: 700, PaneClientW: 700, DocScrollW: 1300, InnerW: 1200}); len(p) == 0 {
		t.Error("a page wider than the window was not reported")
	}
	if p := gitpageWidthProblems(gitpageWidth{PaneScrollW: 700, PaneClientW: 700, DocScrollW: 1200, InnerW: 1200, Cut: []string{"g1 wider than its row"}}); len(p) == 0 {
		t.Error("a cell that pushes out of its row was not reported")
	}
}

// The centre checks target controlcentre.exe, not burnmon.exe or burnmon-dev.exe.
func TestCentreChecksAreRouted(t *testing.T) {
	for _, name := range []string{"gitpage", "gitpage/strip", "gitpage/lane", "gitpage/width", "gitpage/loading"} {
		if checks[name] == nil {
			t.Errorf("%s is not registered", name)
		}
		if !isCentreCheck(name) {
			t.Errorf("%s is not a centre check", name)
		}
		if isDevCheck(name) {
			t.Errorf("%s is counted as a burnmon-dev check", name)
		}
	}
	for _, name := range []string{"w0", "d2", "waits", "waits/d36"} {
		if isCentreCheck(name) {
			t.Errorf("%s is counted as a centre check", name)
		}
	}
}

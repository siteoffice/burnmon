package centre

import (
	"strings"
	"testing"
	"time"
)

var boardNow = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)

func readFixtureBoards(t *testing.T, root string) ([]FlowRow, []SprintRow, []Problem) {
	t.Helper()
	units, problems := FindUnits(root)
	if len(problems) != 0 {
		t.Fatalf("unit problems: %v", problems)
	}
	return ReadBoards(root, units, boardNow)
}

func flowByKey(t *testing.T, flows []FlowRow, key string) FlowRow {
	t.Helper()
	var found []FlowRow
	for _, f := range flows {
		if f.Key == key {
			found = append(found, f)
		}
	}
	if len(found) != 1 {
		t.Fatalf("flow %q found %d times", key, len(found))
	}
	return found[0]
}

func sprintByKey(t *testing.T, sprints []SprintRow, key string) SprintRow {
	t.Helper()
	for _, s := range sprints {
		if s.Key == key {
			return s
		}
	}
	t.Fatalf("no sprint %q", key)
	return SprintRow{}
}

func TestRowsAttachByProjectThenHolder(t *testing.T) {
	flows, sprints, problems := readFixtureBoards(t, fixtureTree(t))
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	want := map[string]string{
		"kettle/kettle-brew-gate": "projects/kettle",
		"nobody/orphan-flow":      ".",
		"lamp/lamp-light":         "projects/lamp",
		"acme-hub/hub-closed-one": ".",
		"stove/stove-no-time":     "projects/stove",
	}
	if len(flows) != len(want) {
		t.Fatalf("got %d flows, want %d", len(flows), len(want))
	}
	for key, unit := range want {
		if got := flowByKey(t, flows, key).Unit; got != unit {
			t.Errorf("%s unit %q, want %q", key, got, unit)
		}
	}
	if got := sprintByKey(t, sprints, "lamp/lamp-sprint-b").Unit; got != "projects/lamp" {
		t.Errorf("lamp-sprint-b unit %q", got)
	}
	f := flowByKey(t, flows, "lamp/lamp-light")
	if f.Project != "lamp" || f.Slug != "lamp-light" || f.Stage != "G3" || f.Index != "projects/kettle/_board/flows/index.json" {
		t.Errorf("lamp-light fields %+v", f)
	}
}

func TestLaterRowWins(t *testing.T) {
	flows, _, _ := readFixtureBoards(t, fixtureTree(t))
	f := flowByKey(t, flows, "kettle/kettle-brew-gate")
	if f.UpdatedAt != "2026-11-04T10:00:00Z" || f.Index != "_board/flows/index.json" || f.Status != "awaiting-owner" {
		t.Errorf("kettle-brew-gate %+v", f)
	}

	// The later row wins also when it sits in the larger holder id.
	root := fixtureTree(t)
	writeFile(t, root, "projects/kettle/_board/flows/index.json",
		`{"flows":[{"project":"kettle","slug":"kettle-brew-gate","stage":"G3","status":"execute","updatedAt":"2026-11-05T00:00:00Z"}]}`)
	flows, _, _ = readFixtureBoards(t, root)
	f = flowByKey(t, flows, "kettle/kettle-brew-gate")
	if f.Index != "projects/kettle/_board/flows/index.json" || f.Stage != "G3" {
		t.Errorf("later row in kettle lost: %+v", f)
	}

	// Equal times: the row from the smaller holder id wins.
	writeFile(t, root, "projects/kettle/_board/flows/index.json",
		`{"flows":[{"project":"kettle","slug":"kettle-brew-gate","stage":"G3","status":"execute","updatedAt":"2026-11-04T10:00:00Z"}]}`)
	flows, _, _ = readFixtureBoards(t, root)
	f = flowByKey(t, flows, "kettle/kettle-brew-gate")
	if f.Index != "_board/flows/index.json" {
		t.Errorf("equal time kept %q, want the hub row", f.Index)
	}

	// Sprints dedup on updated_at the same way.
	writeFile(t, root, "projects/lamp/_board/sprints/index.json",
		`{"sprints":[{"project":"kettle","slug":"kettle-sprint-a","status":"running","phase":"execute","updated_at":"2026-11-03T00:00:00Z"}]}`)
	_, sprints, _ := readFixtureBoards(t, root)
	s := sprintByKey(t, sprints, "kettle/kettle-sprint-a")
	if s.Index != "projects/lamp/_board/sprints/index.json" || s.Status != "running" {
		t.Errorf("later sprint row lost: %+v", s)
	}
	if len(sprints) != 2 {
		t.Errorf("got %d sprints, want 2", len(sprints))
	}
}

func TestWaitingFromStatusOnly(t *testing.T) {
	root := fixtureTree(t)
	writeFile(t, root, "projects/stove/_board/flows/index.json",
		`{"flows":[{"project":"stove","slug":"stove-flagged","stage":"G2","status":"execute","awaiting":true,"updatedAt":"2026-11-10T00:00:00Z"}]}`)
	flows, _, _ := readFixtureBoards(t, root)
	if !flowByKey(t, flows, "kettle/kettle-brew-gate").Waiting {
		t.Error("kettle-brew-gate not waiting although status is awaiting-owner")
	}
	if flowByKey(t, flows, "stove/stove-flagged").Waiting {
		t.Error("stove-flagged waiting although status is execute")
	}
	if !flowByKey(t, flows, "lamp/lamp-light").Waiting {
		t.Error("lamp-light not waiting")
	}
}

func TestWaitsDaysEdges(t *testing.T) {
	cases := map[string]int{
		"2026-11-04T10:00:00Z": 7,
		"2026-11-04T12:00:01Z": 6,
		"":                     -1,
		"garbage":              -1,
		"2026-11-12T00:00:00Z": 0,
		"2026-11-11T12:00:00Z": 0,
	}
	for in, want := range cases {
		if got := WaitsDays(in, boardNow); got != want {
			t.Errorf("WaitsDays(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestFlowRowWaits(t *testing.T) {
	flows, _, _ := readFixtureBoards(t, fixtureTree(t))
	if got := flowByKey(t, flows, "kettle/kettle-brew-gate").WaitsDays; got != 7 {
		t.Errorf("kettle-brew-gate waits %d, want 7", got)
	}
	if got := flowByKey(t, flows, "stove/stove-no-time").WaitsDays; got != -1 {
		t.Errorf("stove-no-time waits %d, want -1", got)
	}
}

func TestBoardPagePath(t *testing.T) {
	flows, sprints, _ := readFixtureBoards(t, fixtureTree(t))
	if got := flowByKey(t, flows, "kettle/kettle-brew-gate").Board; got != "_board/flows/kettle/kettle-brew-gate/index.html" {
		t.Errorf("kettle-brew-gate board %q", got)
	}
	if got := flowByKey(t, flows, "lamp/lamp-light").Board; got != "" {
		t.Errorf("lamp-light board %q, want empty", got)
	}
	if got := sprintByKey(t, sprints, "lamp/lamp-sprint-b").Board; got != "" {
		t.Errorf("lamp-sprint-b board %q, want empty", got)
	}

	// A folder named index.html is not a board page; a sprint page in its holder's _board is.
	root := fixtureTree(t)
	writeFile(t, root, "projects/kettle/_board/flows/lamp/lamp-light/index.html/x.txt", "x")
	writeFile(t, root, "_board/sprints/kettle/kettle-sprint-a/index.html", "<p>sprint</p>")
	flows, sprints, _ = readFixtureBoards(t, root)
	if got := flowByKey(t, flows, "lamp/lamp-light").Board; got != "" {
		t.Errorf("folder counted as board page: %q", got)
	}
	if got := sprintByKey(t, sprints, "kettle/kettle-sprint-a").Board; got != "_board/sprints/kettle/kettle-sprint-a/index.html" {
		t.Errorf("kettle-sprint-a board %q", got)
	}
}

func TestClosedRule(t *testing.T) {
	flows, sprints, _ := readFixtureBoards(t, fixtureTree(t))
	if !flowByKey(t, flows, "acme-hub/hub-closed-one").Closed {
		t.Error("hub-closed-one not closed")
	}
	if flowByKey(t, flows, "nobody/orphan-flow").Closed {
		t.Error("orphan-flow closed")
	}
	if !sprintByKey(t, sprints, "kettle/kettle-sprint-a").Closed {
		t.Error("kettle-sprint-a (phase done) not closed")
	}
	if sprintByKey(t, sprints, "lamp/lamp-sprint-b").Closed {
		t.Error("lamp-sprint-b closed")
	}
	if !flowClosed("closed") || flowClosed("done") || flowClosed("awaiting-owner") {
		t.Error("flowClosed rule")
	}
	sc := []struct {
		status, phase string
		want          bool
	}{
		{"running", "done", true},
		{"done", "execute", true},
		{"closed", "execute", true},
		{"passing", "execute", false},
		{"running", "", false},
	}
	for _, c := range sc {
		if got := sprintClosed(c.status, c.phase); got != c.want {
			t.Errorf("sprintClosed(%q,%q) = %v", c.status, c.phase, got)
		}
	}
}

func TestBrokenIndexIsAProblem(t *testing.T) {
	root := fixtureTree(t)
	writeFile(t, root, "projects/kettle/_board/flows/index.json", `{"flows": 3}`)
	writeFile(t, root, "projects/lamp/_board/sprints/index.json", `not json`)
	flows, sprints, problems := readFixtureBoards(t, root)
	for _, f := range flows {
		if f.Index == "projects/kettle/_board/flows/index.json" {
			t.Errorf("row from broken index: %+v", f)
		}
	}
	if len(flows) != 4 || len(sprints) != 2 {
		t.Errorf("got %d flows and %d sprints, want 4 and 2", len(flows), len(sprints))
	}
	if len(problems) != 2 {
		t.Fatalf("problems %v, want 2", problems)
	}
	paths := map[string]bool{}
	for _, p := range problems {
		paths[p.Path] = true
		if p.Message == "" {
			t.Errorf("problem without message: %+v", p)
		}
	}
	if !paths["projects/kettle/_board/flows/index.json"] || !paths["projects/lamp/_board/sprints/index.json"] {
		t.Errorf("problem paths %v", problems)
	}
}

func TestRowWithoutProjectOrSlugIsAProblem(t *testing.T) {
	root := fixtureTree(t)
	writeFile(t, root, "projects/stove/_board/flows/index.json",
		`{"flows":[{"project":"stove","slug":""},{"project":"","slug":"x"},{"project":"stove","slug":"ok-one"}]}`)
	flows, _, problems := readFixtureBoards(t, root)
	flowByKey(t, flows, "stove/ok-one")
	if len(problems) != 2 {
		t.Fatalf("problems %v, want 2", problems)
	}
	for _, p := range problems {
		if p.Path != "projects/stove/_board/flows/index.json" || !strings.Contains(p.Message, "project or slug") {
			t.Errorf("problem %+v", p)
		}
	}
}

func TestReadBoardsSortsAndArrays(t *testing.T) {
	flows, sprints, problems := ReadBoards(t.TempDir(), []Unit{{ID: ".", Name: "x"}}, boardNow)
	if flows == nil || sprints == nil || problems == nil {
		t.Error("nil slice from ReadBoards")
	}
	flows, sprints, _ = readFixtureBoards(t, fixtureTree(t))
	var keys []string
	for _, f := range flows {
		keys = append(keys, f.Key)
	}
	want := "lamp/lamp-light,kettle/kettle-brew-gate,stove/stove-no-time,nobody/orphan-flow,acme-hub/hub-closed-one"
	if got := strings.Join(keys, ","); got != want {
		t.Errorf("flow order %s\nwant       %s", got, want)
	}
	if sprints[0].Key != "lamp/lamp-sprint-b" {
		t.Errorf("sprint order %v", sprints)
	}
}

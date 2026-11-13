package centre

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite testdata/state_golden.json")

var goldenNow = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)

func TestGoldenState(t *testing.T) {
	root := fixtureTree(t)
	st := Build(root, Options{Now: goldenNow, Weeks: 2, Rows: 5})
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	esc, err := json.Marshal(st.Root)
	if err != nil {
		t.Fatal(err)
	}
	escRoot := strings.Trim(string(esc), `"`)
	if escRoot == "" {
		t.Fatal("state root is empty")
	}
	data = bytes.ReplaceAll(data, []byte(escRoot), []byte("ROOT"))
	data = append(data, '\n')
	golden := filepath.Join("testdata", "state_golden.json")
	if *update {
		if err := os.WriteFile(golden, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run with -update to make it): %v", err)
	}
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(data, want) {
		t.Errorf("state differs from %s; got:\n%s", golden, data)
	}
}

func TestBuildMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "no-such-folder")
	st := Build(root, Options{Now: goldenNow})
	if len(st.Units) != 0 {
		t.Errorf("units %d", len(st.Units))
	}
	if len(st.Problems) != 1 || st.Problems[0].Message != "root folder not found" {
		t.Errorf("problems %+v", st.Problems)
	}
	if st.Counts.Problems != 1 {
		t.Errorf("counts.problems %d", st.Counts.Problems)
	}
}

func TestBuildProblemsSortedByPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "siteoffice.json", `{"name":"hub"}`)
	writeFile(t, root, "b/siteoffice.json", "{")
	writeFile(t, root, "_board/flows/index.json", "{")
	writeFile(t, root, PlanFile, "{")
	st := Build(root, Options{Now: goldenNow})
	var paths []string
	for _, p := range st.Problems {
		paths = append(paths, p.Path)
	}
	want := []string{"_board/flows/index.json", "b/siteoffice.json", "siteoffice.plan.json"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Errorf("problem paths %v, want %v", paths, want)
	}
	if st.Plan != nil || st.Overview.Default != "units" || st.Counts.Problems != 3 {
		t.Errorf("plan %v default %q problems %d", st.Plan, st.Overview.Default, st.Counts.Problems)
	}
}

func TestBuildEmptySlicesAreArrays(t *testing.T) {
	st := Build(t.TempDir(), Options{Now: goldenNow})
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{`"units":[]`, `"flows":[]`, `"sprints":[]`, `"problems":[]`, `"plan":null`, `"next_fixed":null`, `"plan_columns":null`, `"unit_columns":[]`, `"badges":{}`, `"keys":[]`, `"pages":[]`} {
		if !strings.Contains(s, want) {
			t.Errorf("JSON lacks %s: %s", want, s)
		}
	}
	if st.RowsPerPage != 12 || st.Planning.Weeks != 2 || st.Overview.Default != "units" {
		t.Errorf("defaults: rows %d weeks %d default %q", st.RowsPerPage, st.Planning.Weeks, st.Overview.Default)
	}
}

func TestStaleProblems(t *testing.T) {
	rows := []FlowRow{
		{Key: "a/s1", Index: "_board/flows/index.json", StateText: StaleText},
		{Key: "a/s2", Index: "_board/flows/index.json"},
	}
	got := StaleProblems(rows)
	want := []Problem{{Path: "_board/flows/index.json", Message: "flow a/s1: gate decided, flow not advanced"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("StaleProblems = %+v, want %+v", got, want)
	}
	if got := StaleProblems(nil); got == nil || len(got) != 0 {
		t.Errorf("StaleProblems(nil) = %#v, want empty non-nil", got)
	}
}

func TestBuildWaitingRule(t *testing.T) {
	root := fixtureTree(t)
	writeFile(t, root, "projects/lamp/_board/sprints/index.json",
		`{"sprints":[{"project":"lamp","slug":"l-wait","status":"waiting-on-you","phase":"execute","updated_at":"2026-11-08T12:00:00Z"}]}`)
	st := Build(root, Options{Now: goldenNow, Weeks: 2, Rows: 5})
	if want := []string{"flow:lamp/lamp-light", "sprint:lamp/l-wait"}; !reflect.DeepEqual(st.Waiting.Keys, want) {
		t.Errorf("waiting keys %v, want %v", st.Waiting.Keys, want)
	}
	if st.Counts.Waiting != 2 || st.Counts.LongestWaitDays != 3 {
		t.Errorf("counts waiting %d longest %d, want 2 and 3", st.Counts.Waiting, st.Counts.LongestWaitDays)
	}
	if want := map[string]int{"i-kettle": 2}; !reflect.DeepEqual(st.Badges, want) {
		t.Errorf("badges %v, want %v", st.Badges, want)
	}
	want := []Problem{
		{Path: "_board/flows/index.json", Message: "flow kettle/kettle-brew-gate: gate decided, flow not advanced"},
		{Path: "_board/flows/index.json", Message: "flow stove/stove-no-time: gate decided, flow not advanced"},
	}
	if !reflect.DeepEqual(st.Problems, want) {
		t.Errorf("problems %+v, want %+v", st.Problems, want)
	}
}

func TestBuildWaitingKeysCarryKind(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "siteoffice.json", `{"name":"hub"}`)
	writeFile(t, root, "_board/flows/index.json",
		`{"flows":[{"project":"p","slug":"s","stage":"G1","status":"execute","awaiting":true,"updatedAt":"2026-11-10T00:00:00Z"}]}`)
	writeFile(t, root, "_board/sprints/index.json",
		`{"sprints":[{"project":"p","slug":"s","status":"waiting-on-you","phase":"execute","updated_at":"2026-11-10T00:00:00Z"}]}`)
	st := Build(root, Options{Now: goldenNow})
	if want := []string{"flow:p/s", "sprint:p/s"}; !reflect.DeepEqual(st.Waiting.Keys, want) {
		t.Errorf("waiting keys %v, want %v", st.Waiting.Keys, want)
	}
}

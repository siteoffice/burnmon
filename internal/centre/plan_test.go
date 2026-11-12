package centre

import (
	"strings"
	"testing"
	"time"
)

func TestLoadPlanGood(t *testing.T) {
	root := fixtureTree(t)
	plan, prob := LoadPlan(root)
	if prob != nil {
		t.Fatalf("problem %+v", *prob)
	}
	if plan == nil {
		t.Fatal("plan is nil")
	}
	if len(plan.Goals) != 2 {
		t.Errorf("goals %d, want 2", len(plan.Goals))
	}
	if len(plan.Items) != 2 || plan.Items[0].ID != "i-kettle" || len(plan.Items[0].Parts) != 1 {
		t.Errorf("items %+v", plan.Items)
	}
	if !plan.Items[1].Hold {
		t.Error("i-stove hold is false")
	}
}

func TestLoadPlanAbsent(t *testing.T) {
	plan, prob := LoadPlan(t.TempDir())
	if plan != nil || prob != nil {
		t.Errorf("got %v, %v; want nil, nil", plan, prob)
	}
}

func TestLoadPlanUnparsable(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, PlanFile, "{")
	plan, prob := LoadPlan(root)
	if plan != nil {
		t.Error("plan is not nil")
	}
	if prob == nil {
		t.Fatal("no problem")
	}
	if prob.Path != "siteoffice.plan.json" {
		t.Errorf("path %q", prob.Path)
	}
	if !strings.HasPrefix(prob.Message, "cannot read") {
		t.Errorf("message %q", prob.Message)
	}
}

func TestLoadPlanDoubleID(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, PlanFile, `{"schemaVersion":1,"goals":[{"id":"x","title":"A"}],"items":[{"id":"x","title":"B"}]}`)
	plan, prob := LoadPlan(root)
	if plan != nil {
		t.Error("plan is not nil")
	}
	if prob == nil {
		t.Fatal("no problem")
	}
	if prob.Path != PlanFile || !strings.Contains(prob.Message, `"x"`) {
		t.Errorf("problem %+v does not name x", *prob)
	}
}

func TestLoadPlanStrictFields(t *testing.T) {
	cases := map[string]string{
		"schema": `{"schemaVersion":2,"goals":[{"id":"g","title":"G"}]}`,
		"date":   `{"schemaVersion":1,"dates":[{"date":"2026-13-01","title":"Bad"}]}`,
		"week":   `{"schemaVersion":1,"weekNotes":[{"week":"2026-47","text":"Bad"}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, PlanFile, body)
			plan, prob := LoadPlan(root)
			if plan != nil {
				t.Error("plan is not nil")
			}
			if prob == nil || prob.Path != PlanFile || prob.Message == "" {
				t.Errorf("problem %v", prob)
			}
		})
	}
}

func TestBadges(t *testing.T) {
	root := fixtureTree(t)
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	units, _ := FindUnits(root)
	flows, _, _ := ReadBoards(root, units, now)
	plan, prob := LoadPlan(root)
	if prob != nil {
		t.Fatalf("problem %+v", *prob)
	}
	got := Badges(plan, units, flows)
	want := map[string]int{"i-kettle": 2, "i-stove": 1}
	if len(got) != len(want) {
		t.Errorf("badges %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("badge %s = %d, want %d", k, got[k], v)
		}
	}

	// A part on the item's own unit counts that unit once.
	same := &Plan{Items: []PlanNode{{ID: "i-k", Unit: "kettle", Parts: []PlanPart{{Title: "Self", Unit: "kettle"}}}, {ID: "i-none", Unit: "engine"}}}
	got = Badges(same, units, flows)
	if got["i-k"] != 1 {
		t.Errorf("i-k = %d, want 1", got["i-k"])
	}
	if _, ok := got["i-none"]; ok {
		t.Error("item without waiting rows has a badge")
	}

	if got := Badges(nil, units, flows); got == nil || len(got) != 0 {
		t.Errorf("no plan: %v, want empty map", got)
	}
}

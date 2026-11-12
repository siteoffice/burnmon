package centre

import (
	"reflect"
	"strings"
	"testing"
)

func unitIDs(units []Unit) []string {
	ids := make([]string, len(units))
	for i, u := range units {
		ids[i] = u.ID
	}
	return ids
}

func unitByID(t *testing.T, units []Unit, id string) Unit {
	t.Helper()
	for _, u := range units {
		if u.ID == id {
			return u
		}
	}
	t.Fatalf("no unit %q in %v", id, unitIDs(units))
	return Unit{}
}

func TestFindUnitsDepthAndSkips(t *testing.T) {
	units, problems := FindUnits(fixtureTree(t))
	if len(problems) != 0 {
		t.Errorf("problems: %v", problems)
	}
	want := []string{".", "deep/l2/l3/l4", "projects/kettle", "projects/kettle/engine", "projects/lamp", "projects/stove"}
	if got := unitIDs(units); !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	for _, u := range units {
		if strings.HasPrefix(u.Name, "skipped-") {
			t.Errorf("skipped folder became a unit: %s (%s)", u.ID, u.Name)
		}
		if u.Name == "deep5" {
			t.Errorf("unit below depth %d found: %s", MaxDepth, u.ID)
		}
		if u.ID == "projects/ghost" {
			t.Error("ghost became a unit through nodes")
		}
	}
	if d := unitByID(t, units, "deep/l2/l3/l4").Depth; d != 4 {
		t.Errorf("deep4 depth = %d, want 4", d)
	}
	if d := unitByID(t, units, ".").Depth; d != 0 {
		t.Errorf("root depth = %d, want 0", d)
	}
}

func TestSkip(t *testing.T) {
	for _, name := range append(append([]string{}, SkipDirs...), ".hidden", ".x") {
		if !Skip(name) {
			t.Errorf("Skip(%q) = false", name)
		}
	}
	for _, name := range []string{"projects", "kettle", "board", "log_"} {
		if Skip(name) {
			t.Errorf("Skip(%q) = true", name)
		}
	}
}

func TestUnitNamesAndDisplayNames(t *testing.T) {
	units, _ := FindUnits(fixtureTree(t))
	root := unitByID(t, units, ".")
	if root.Name != "acme-hub" || root.DisplayName != "Acme Hub" || root.Type != "hub" {
		t.Errorf("root = %q %q %q", root.Name, root.DisplayName, root.Type)
	}
	if got := unitByID(t, units, "projects/kettle").DisplayName; got != "Kettle" {
		t.Errorf("kettle display = %q", got)
	}
	if got := unitByID(t, units, "projects/kettle/engine").DisplayName; got != "engine" {
		t.Errorf("engine display = %q", got)
	}
	for _, u := range units {
		if u.Children == nil || u.Files == nil || u.Folders == nil || u.FilePages == nil ||
			u.Flows.Keys == nil || u.Flows.Pages == nil || u.Sprints.Keys == nil || u.Sprints.Pages == nil {
			t.Errorf("unit %s has a nil slice", u.ID)
		}
	}

	tmp := t.TempDir()
	writeFile(t, tmp, "siteoffice.json", `{"name":"top"}`)
	writeFile(t, tmp, "nameless/siteoffice.json", `{"type":"project"}`)
	units, problems := FindUnits(tmp)
	if len(problems) != 0 {
		t.Errorf("problems: %v", problems)
	}
	u := unitByID(t, units, "nameless")
	if u.Name != "nameless" || u.DisplayName != "nameless" || u.Type != "project" {
		t.Errorf("nameless = %q %q %q", u.Name, u.DisplayName, u.Type)
	}
}

func TestUnitParents(t *testing.T) {
	units, _ := FindUnits(fixtureTree(t))
	parents := map[string]string{
		".":                      "",
		"projects/kettle":        ".",
		"projects/kettle/engine": "projects/kettle",
		"projects/lamp":          "projects/kettle",
		"projects/stove":         ".",
	}
	for id, want := range parents {
		if got := unitByID(t, units, id).Parent; got != want {
			t.Errorf("parent of %s = %q, want %q", id, got, want)
		}
	}
	levels := map[string]int{".": 0, "projects/kettle": 1, "projects/kettle/engine": 2, "projects/lamp": 2, "projects/stove": 1}
	for id, want := range levels {
		if got := unitByID(t, units, id).Level; got != want {
			t.Errorf("level of %s = %d, want %d", id, got, want)
		}
	}
	want := []string{"projects/kettle/engine", "projects/lamp"}
	if got := unitByID(t, units, "projects/kettle").Children; !reflect.DeepEqual(got, want) {
		t.Errorf("kettle children = %v, want %v", got, want)
	}
}

func TestParentCycleFallsBackToAncestor(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "siteoffice.json", `{"name":"top"}`)
	writeFile(t, tmp, "a/siteoffice.json", `{"name":"a","parent":"b"}`)
	writeFile(t, tmp, "b/siteoffice.json", `{"name":"b","parent":"a"}`)
	units, _ := FindUnits(tmp)
	for _, id := range []string{"a", "b"} {
		u := unitByID(t, units, id)
		if u.Parent != "." || u.Level != 1 {
			t.Errorf("%s parent %q level %d, want \".\" 1", id, u.Parent, u.Level)
		}
	}
}

func TestParentNameEqualsOwnName(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "siteoffice.json", `{"name":"top"}`)
	writeFile(t, tmp, "x/siteoffice.json", `{"name":"x"}`)
	writeFile(t, tmp, "x/self/siteoffice.json", `{"name":"self","parent":"self"}`)
	units, _ := FindUnits(tmp)
	if got := unitByID(t, units, "x/self").Parent; got != "x" {
		t.Errorf("parent of x/self = %q, want x", got)
	}
}

func TestDuplicateNamesFirstIdWins(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "siteoffice.json", `{"name":"top"}`)
	writeFile(t, tmp, "b/siteoffice.json", `{"name":"docs"}`)
	writeFile(t, tmp, "a/siteoffice.json", `{"name":"docs"}`)
	writeFile(t, tmp, "c/siteoffice.json", `{"name":"c","parent":"docs"}`)
	units, _ := FindUnits(tmp)
	if got := unitByID(t, units, "c").Parent; got != "a" {
		t.Errorf("parent of c = %q, want a", got)
	}
}

func TestBrokenUnitFileStillAUnit(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "siteoffice.json", `{"name":"top"}`)
	writeFile(t, tmp, "broken/siteoffice.json", `{`)
	units, problems := FindUnits(tmp)
	u := unitByID(t, units, "broken")
	if u.Name != "broken" || u.DisplayName != "broken" || u.Parent != "." {
		t.Errorf("broken = name %q display %q parent %q", u.Name, u.DisplayName, u.Parent)
	}
	if len(problems) != 1 || problems[0].Path != "broken/siteoffice.json" || problems[0].Message == "" {
		t.Errorf("problems = %v, want one for broken/siteoffice.json", problems)
	}
}

func TestUnitFileWithBOM(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "siteoffice.json", `{"name":"top"}`)
	writeFile(t, tmp, "x/siteoffice.json", `{"name":"x"}`)
	bom := string([]byte{0xEF, 0xBB, 0xBF})
	writeFile(t, tmp, "bom/siteoffice.json", bom+`{"name":"marked","parent":"x"}`)
	units, problems := FindUnits(tmp)
	if len(problems) != 0 {
		t.Errorf("problems: %v", problems)
	}
	u := unitByID(t, units, "bom")
	if u.Name != "marked" || u.Parent != "x" {
		t.Errorf("bom = name %q parent %q, want marked x", u.Name, u.Parent)
	}
}

func TestWrongTypeUnitFileStillAUnit(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "siteoffice.json", `{"name":"top"}`)
	writeFile(t, tmp, "odd/siteoffice.json", `{"name":7}`)
	units, problems := FindUnits(tmp)
	if got := unitByID(t, units, "odd").Name; got != "odd" {
		t.Errorf("odd name = %q", got)
	}
	if len(problems) != 1 || problems[0].Path != "odd/siteoffice.json" {
		t.Errorf("problems = %v", problems)
	}
}

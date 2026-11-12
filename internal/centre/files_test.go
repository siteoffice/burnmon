package centre

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestRoleTable(t *testing.T) {
	cases := map[string]string{
		"AGENTS.md":       "Instruction file",
		"CLAUDE.md":       "Instruction file",
		"STATUS.md":       "State",
		"DEADLINES.md":    "Dates and gates",
		"README.md":       "Readme",
		"CHANGELOG.md":    "Release history",
		"SESSION_LOG.md":  "Session log",
		"GLOSSARY.md":     "Glossary",
		"siteoffice.json": "Unit file",
		"x.md":            "File",
	}
	for name, want := range cases {
		if got := Role(name); got != want {
			t.Errorf("Role(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestFileStateEdges(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	cases := []struct {
		size int64
		mod  time.Time
		want string
	}{
		{199, now, "stub"},
		{200, now.Add(-60 * day), "ok"},
		{200, now.Add(-61 * day), "stale"},
		{199, now.Add(-100 * day), "stub"},
	}
	for _, c := range cases {
		if got := FileState(c.size, c.mod, now); got != c.want {
			t.Errorf("FileState(%d, %v) = %q, want %q", c.size, c.mod, got, c.want)
		}
	}
}

func TestListFilesFixture(t *testing.T) {
	root := fixtureTree(t)
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)

	files, folders := ListFiles(root, now)
	var names []string
	byName := map[string]File{}
	for _, f := range files {
		names = append(names, f.Name)
		byName[f.Name] = f
	}
	want := []string{"AGENTS.md", "README.md", "STATUS.md", "siteoffice.json", "siteoffice.plan.json"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("root files %v, want %v", names, want)
	}
	if got := byName["STATUS.md"].State; got != "stub" {
		t.Errorf("STATUS.md state %q, want stub", got)
	}
	if got := byName["AGENTS.md"]; got.Role != "Instruction file" || got.State != "ok" || got.Modified != "2026-11-01T00:00:00Z" || got.AgeDays != 10 {
		t.Errorf("AGENTS.md %+v", got)
	}

	var folderNames []string
	for _, f := range folders {
		folderNames = append(folderNames, f.Name)
	}
	if !reflect.DeepEqual(folderNames, []string{"deep", "projects"}) {
		t.Errorf("root folders %v, want [deep projects]", folderNames)
	}
	for _, f := range folders {
		entries, err := os.ReadDir(filepath.Join(root, f.Name))
		if err != nil {
			t.Fatal(err)
		}
		if f.Entries != len(entries) || f.Entries == 0 {
			t.Errorf("folder %s entries %d, want %d", f.Name, f.Entries, len(entries))
		}
	}

	kettle, _ := ListFiles(filepath.Join(root, "projects", "kettle"), now)
	found := false
	for _, f := range kettle {
		if f.Name == "CHANGELOG.md" {
			found = true
			if f.State != "stale" || f.AgeDays != 102 {
				t.Errorf("kettle CHANGELOG.md %+v, want stale and 102 days", f)
			}
		}
	}
	if !found {
		t.Error("kettle CHANGELOG.md not listed")
	}
}

func TestListFilesCapAndSort(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i <= 40; i++ {
		writeFile(t, dir, fmt.Sprintf("f%02d.md", i), "x")
	}
	writeFile(t, dir, "a.json", "{}")
	files, _ := ListFiles(dir, time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC))
	if len(files) != MaxFiles {
		t.Fatalf("got %d files, want %d", len(files), MaxFiles)
	}
	if files[0].Name != "a.json" || files[len(files)-1].Name != "f38.md" {
		t.Errorf("first %q last %q, want a.json and f38.md", files[0].Name, files[len(files)-1].Name)
	}
}

func TestListFilesSkipsNonRegular(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "x.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	files, folders := ListFiles(dir, time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC))
	if len(files) != 0 {
		t.Errorf("files %v, want none", files)
	}
	if len(folders) != 1 || folders[0].Name != "x.md" {
		t.Errorf("folders %v, want [x.md]", folders)
	}
}

package centre

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MaxFiles is the most unit files listed for one unit.
const MaxFiles = 40

// StubBytes is the size a file must reach to not be a stub.
const StubBytes = 200

// StaleDays is the age in days a file may reach before it is stale.
const StaleDays = 60

var roles = map[string]string{
	"AGENTS.md":       "Instruction file",
	"CLAUDE.md":       "Instruction file",
	"STATUS.md":       "State",
	"DEADLINES.md":    "Dates and gates",
	"README.md":       "Readme",
	"CHANGELOG.md":    "Release history",
	"SESSION_LOG.md":  "Session log",
	"GLOSSARY.md":     "Glossary",
	"siteoffice.json": "Unit file",
}

// Role returns the role of a unit file by its name, "File" when the name
// is not in the table.
func Role(name string) string {
	if r, ok := roles[name]; ok {
		return r
	}
	return "File"
}

// FileState returns "stub" under StubBytes, else "stale" when mod is more
// than StaleDays before now, else "ok".
func FileState(size int64, mod, now time.Time) string {
	if size < StubBytes {
		return "stub"
	}
	if now.Sub(mod) > StaleDays*24*time.Hour {
		return "stale"
	}
	return "ok"
}

// ListFiles returns the unit files of dir (regular .md and .json files,
// sorted by name, at most MaxFiles) and its direct subfolders that the walk
// does not skip, each with its entry count. It does not recurse.
func ListFiles(dir string, now time.Time) ([]File, []Folder) {
	files := []File{}
	folders := []Folder{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return files, folders
	}
	var names []string
	infos := map[string]os.FileInfo{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if Skip(name) {
				continue
			}
			count := 0
			if sub, err := os.ReadDir(filepath.Join(dir, name)); err == nil {
				count = len(sub)
			}
			folders = append(folders, Folder{Name: name, Entries: count})
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".md" && ext != ".json" {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		names = append(names, name)
		infos[name] = info
	}
	sort.Strings(names)
	if len(names) > MaxFiles {
		names = names[:MaxFiles]
	}
	for _, name := range names {
		info := infos[name]
		mod := info.ModTime()
		age := int(now.Sub(mod) / (24 * time.Hour))
		if age < 0 {
			age = 0
		}
		files = append(files, File{
			Name:     name,
			Role:     Role(name),
			Size:     info.Size(),
			Modified: mod.UTC().Format(time.RFC3339),
			AgeDays:  age,
			State:    FileState(info.Size(), mod, now),
		})
	}
	sort.Slice(folders, func(i, j int) bool { return folders[i].Name < folders[j].Name })
	return files, folders
}

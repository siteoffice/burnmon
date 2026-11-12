package centre

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// MaxDepth is the deepest folder level below root that can be a unit.
const MaxDepth = 4

// SkipDirs are folder names the walk never enters.
var SkipDirs = []string{".git", ".worktrees", "node_modules", "_archive", "__pycache__", ".venv", "graphify-out", "_board", "_log"}

// unitFileName marks a folder as a unit.
const unitFileName = "siteoffice.json"

// maxUnitFile is the most bytes read from one unit file.
const maxUnitFile = 512 * 1024

// Skip reports whether the walk skips a folder with this name.
func Skip(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	for _, s := range SkipDirs {
		if name == s {
			return true
		}
	}
	return false
}

type unitFile struct {
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	DisplayName2 string `json:"displayName"`
	Parent       string `json:"parent"`
	Type         string `json:"type"`
}

// FindUnits walks root to MaxDepth and returns every unit, sorted by id
// with "." first, with names, parents, levels and children filled in.
// A unit file that does not read or parse still makes a unit (folder name)
// plus one problem.
func FindUnits(root string) ([]Unit, []Problem) {
	units := []Unit{}
	problems := []Problem{}
	parentField := map[string]string{}

	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() && p != root {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return fs.SkipDir
		}
		id := filepath.ToSlash(rel)
		depth := 0
		if id != "." {
			if Skip(d.Name()) {
				return fs.SkipDir
			}
			depth = strings.Count(id, "/") + 1
			if depth > MaxDepth {
				return fs.SkipDir
			}
		}
		filePath := filepath.Join(p, unitFileName)
		info, serr := os.Stat(filePath)
		if serr != nil || !info.Mode().IsRegular() {
			return nil
		}
		uf, perr := readUnitFile(filePath)
		if perr != nil {
			problems = append(problems, Problem{Path: relFile(id), Message: perr.Error()})
			uf = unitFile{}
		}
		name := uf.Name
		if name == "" {
			name = filepath.Base(p)
		}
		display := uf.DisplayName
		if display == "" {
			display = uf.DisplayName2
		}
		if display == "" {
			display = name
		}
		parentField[id] = uf.Parent
		units = append(units, Unit{
			ID:          id,
			Name:        name,
			DisplayName: display,
			Type:        uf.Type,
			Depth:       depth,
			Children:    []string{},
			Files:       []File{},
			Folders:     []Folder{},
			FilePages:   []Span{},
			Flows:       List{Keys: []string{}, Pages: []Span{}},
			Sprints:     List{Keys: []string{}, Pages: []Span{}},
		})
		return nil
	})

	sort.Slice(units, func(i, j int) bool { return idLess(units[i].ID, units[j].ID) })
	resolveParents(units, parentField)
	return units, problems
}

// idLess orders unit ids with "." first, then by id.
func idLess(a, b string) bool {
	if a == "." || b == "." {
		return a == "." && b != "."
	}
	return a < b
}

func relFile(id string) string {
	if id == "." {
		return unitFileName
	}
	return id + "/" + unitFileName
}

func readUnitFile(p string) (unitFile, error) {
	var uf unitFile
	f, err := os.Open(p)
	if err != nil {
		return uf, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxUnitFile+1))
	if err != nil {
		return uf, err
	}
	if len(data) > maxUnitFile {
		return uf, errors.New("unit file is over 512 KB")
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if err := json.Unmarshal(data, &uf); err != nil {
		return unitFile{}, err
	}
	return uf, nil
}

// resolveParents sets Parent, Level and Children. units must be sorted by
// id. A parent name points at the first unit in id order with that name,
// never the unit itself; otherwise, and for every unit on a parent cycle,
// the nearest ancestor folder that is a unit is the parent.
func resolveParents(units []Unit, parentField map[string]string) {
	index := map[string]int{}
	byName := map[string]string{}
	for i, u := range units {
		index[u.ID] = i
		if _, ok := byName[u.Name]; !ok {
			byName[u.Name] = u.ID
		}
	}
	ancestor := func(id string) string {
		if id == "." {
			return ""
		}
		for d := path.Dir(id); ; d = path.Dir(d) {
			if _, ok := index[d]; ok {
				return d
			}
			if d == "." {
				return ""
			}
		}
	}
	for i := range units {
		u := &units[i]
		if u.ID == "." {
			u.Parent = ""
			continue
		}
		if p, ok := byName[parentField[u.ID]]; ok && parentField[u.ID] != "" && p != u.ID {
			u.Parent = p
		} else {
			u.Parent = ancestor(u.ID)
		}
	}
	// Break cycles. Each round turns at least one name edge into an
	// ancestor edge, and ancestor edges alone cannot cycle, so this ends.
	for {
		var onCycle []int
		for i := range units {
			p := units[i].Parent
			for steps := 0; p != "" && steps < len(units); steps++ {
				if p == units[i].ID {
					onCycle = append(onCycle, i)
					break
				}
				p = units[index[p]].Parent
			}
		}
		if len(onCycle) == 0 {
			break
		}
		for _, i := range onCycle {
			units[i].Parent = ancestor(units[i].ID)
		}
	}
	for i := range units {
		level := 0
		for p := units[i].Parent; p != ""; p = units[index[p]].Parent {
			level++
		}
		units[i].Level = level
		if p := units[i].Parent; p != "" {
			parent := &units[index[p]]
			parent.Children = append(parent.Children, units[i].ID)
		}
	}
}

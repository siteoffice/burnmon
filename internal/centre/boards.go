package centre

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type flowIndex struct {
	Flows []struct {
		Project   string `json:"project"`
		Slug      string `json:"slug"`
		Stage     string `json:"stage"`
		Status    string `json:"status"`
		Awaiting  bool   `json:"awaiting"` // absent reads as false
		UpdatedAt string `json:"updatedAt"`
	} `json:"flows"`
}

type sprintIndex struct {
	Sprints []struct {
		Project   string `json:"project"`
		Slug      string `json:"slug"`
		Status    string `json:"status"`
		Phase     string `json:"phase"`
		Progress  int    `json:"progress"`
		UpdatedAt string `json:"updated_at"`
	} `json:"sprints"`
}

// ReadBoards reads <unit>/_board/flows/index.json and
// <unit>/_board/sprints/index.json for every unit (units sorted by id) and
// returns the flow and sprint rows, deduplicated by key and sorted by
// SortFlows and SortSprints. A row belongs to the first unit whose name
// equals its project, else to the unit that holds the index. Of two rows
// with one key the later update wins; on equal times the row from the
// smaller holder id stays. An index that does not read or parse gives no
// rows and one problem.
func ReadBoards(root string, units []Unit, now time.Time) ([]FlowRow, []SprintRow, []Problem) {
	problems := []Problem{}
	owner := map[string]string{}
	for _, u := range units {
		if _, ok := owner[u.Name]; !ok {
			owner[u.Name] = u.ID
		}
	}
	ownerOf := func(project, holder string) string {
		if id, ok := owner[project]; ok {
			return id
		}
		return holder
	}

	flows := []FlowRow{}
	flowAt := map[string]int{}
	sprints := []SprintRow{}
	sprintAt := map[string]int{}

	for _, u := range units {
		rel := boardRel(u.ID, "flows/index.json")
		var fi flowIndex
		if found, err := readIndex(root, rel, &fi); err != nil {
			problems = append(problems, Problem{Path: rel, Message: err.Error()})
		} else if found {
			for n, r := range fi.Flows {
				if r.Project == "" || r.Slug == "" {
					problems = append(problems, Problem{Path: rel, Message: fmt.Sprintf("flow row %d has no project or slug", n+1)})
					continue
				}
				row := FlowRow{
					Key:       r.Project + "/" + r.Slug,
					Project:   r.Project,
					Slug:      r.Slug,
					Stage:     r.Stage,
					Status:    r.Status,
					UpdatedAt: r.UpdatedAt,
					WaitsDays: WaitsDays(r.UpdatedAt, now),
					Waiting:   r.Awaiting,
					Closed:    flowClosed(r.Status),
					Unit:      ownerOf(r.Project, u.ID),
					Index:     rel,
					Board:     boardPage(root, u.ID, "flows", r.Project, r.Slug),
				}
				if r.Status == statusWaiting && !r.Awaiting {
					row.StateText = StaleText
				}
				if i, ok := flowAt[row.Key]; ok {
					if newerFirst(row.UpdatedAt, flows[i].UpdatedAt) < 0 {
						flows[i] = row
					}
					continue
				}
				flowAt[row.Key] = len(flows)
				flows = append(flows, row)
			}
		}

		rel = boardRel(u.ID, "sprints/index.json")
		var si sprintIndex
		if found, err := readIndex(root, rel, &si); err != nil {
			problems = append(problems, Problem{Path: rel, Message: err.Error()})
		} else if found {
			for n, r := range si.Sprints {
				if r.Project == "" || r.Slug == "" {
					problems = append(problems, Problem{Path: rel, Message: fmt.Sprintf("sprint row %d has no project or slug", n+1)})
					continue
				}
				row := SprintRow{
					Key:       r.Project + "/" + r.Slug,
					Project:   r.Project,
					Slug:      r.Slug,
					Status:    r.Status,
					Phase:     r.Phase,
					Progress:  r.Progress,
					UpdatedAt: r.UpdatedAt,
					Closed:    sprintClosed(r.Status, r.Phase),
					Unit:      ownerOf(r.Project, u.ID),
					Index:     rel,
					Board:     boardPage(root, u.ID, "sprints", r.Project, r.Slug),
					WaitsDays: WaitsDays(r.UpdatedAt, now),
					Waiting:   r.Status == sprintWaiting,
				}
				if i, ok := sprintAt[row.Key]; ok {
					if newerFirst(row.UpdatedAt, sprints[i].UpdatedAt) < 0 {
						sprints[i] = row
					}
					continue
				}
				sprintAt[row.Key] = len(sprints)
				sprints = append(sprints, row)
			}
		}
	}

	SortFlows(flows)
	SortSprints(sprints)
	return flows, sprints, problems
}

// boardRel returns the root-relative path of rest inside a unit's _board.
func boardRel(unitID, rest string) string {
	if unitID == "." {
		return "_board/" + rest
	}
	return unitID + "/_board/" + rest
}

// readIndex parses root/rel into v. found is false when the file is
// absent; err is set when it exists but does not read or parse.
func readIndex(root, rel string, v any) (bool, error) {
	p := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return true, fmt.Errorf("cannot read index: %v", err)
	}
	if !info.Mode().IsRegular() {
		return true, fmt.Errorf("index is not a regular file")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return true, fmt.Errorf("cannot read index: %v", err)
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if err := json.Unmarshal(data, v); err != nil {
		return true, fmt.Errorf("cannot parse index: %v", err)
	}
	return true, nil
}

// boardPage returns the root-relative path of the row's board page in the
// holder's _board, or "" when it is not a regular file or a name is not a
// plain folder name.
func boardPage(root, holder, kind, project, slug string) string {
	for _, s := range []string{project, slug} {
		if s == "." || s == ".." || strings.ContainsAny(s, `/\:`) {
			return ""
		}
	}
	rel := boardRel(holder, kind+"/"+project+"/"+slug+"/index.html")
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return rel
}

package controls

import (
	"errors"
	"path/filepath"
	"regexp"

	"burnmon/internal/gitrefs"
	"burnmon/internal/graph"
)

// CloseDetail is the one-line reason Foreman records for the close.
const CloseDetail = "branch in main, closed from BurnMon Dev"

// closeRefused is the page text for exit code 1. sprint_step.py exits 1 for
// any uncaught error, including a dashboard render failure after sprint.json
// was written, so the text says only what is known.
const closeRefused = "Foreman stopped with exit 1: the sprint may or may not be closed, check the sprint's dashboard or report"

// closeReasonMax caps the reason line appended to closeRefused, in runes.
const closeReasonMax = 300

// branchRe is stricter than gitrefs.ValidBranch: a branch that reaches a
// command line holds only letters, digits and . _ / -.
var branchRe = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

// SprintScript is Foreman's own sprint_step.py under foreman_root.
func SprintScript(root string) string {
	return filepath.Join(root, "sprints", "_lib", "sprint_step.py")
}

// CloseArgv builds the command that closes one sprint: Foreman's documented
// sprint_done move into phase done, with status done so the row leaves the
// wait list. -E -s drops PYTHONPATH, PYTHONHOME and the user site; unlike -I it
// keeps the script's own folder on sys.path, which sprint_step.py needs for
// its sibling modules. The caller passes values CheckSprint has validated.
func CloseArgv(pythonPath, root, sprintDir string) []string {
	return []string{pythonPath, "-E", "-s", SprintScript(root), sprintDir,
		"--phase", "done", "--action", "sprint_done", "--actor", "burnmon",
		"--detail", CloseDetail, "--status", "done"}
}

// CheckSprint compares the preview target with a fresh Foreman read and a
// fresh merge check. It is the whole safety test for the Close control, run at
// the first press and again at the second.
func CheckSprint(t Target, sprints []graph.Sprint, merged func(repo, branch string) (bool, bool), root string) error {
	matches := 0
	var s graph.Sprint
	for _, f := range sprints {
		if f.ID == t.SprintID {
			s = f
			matches++
		}
	}
	switch {
	case matches > 1:
		return errors.New("the sprint id is listed more than once")
	case matches == 0:
		return errors.New("the sprint is no longer listed")
	case s.Status != "waiting-on-you":
		return errors.New("the sprint is no longer waiting on you")
	case s.Phase == "done":
		return errors.New("the sprint is already done")
	case filepath.Clean(s.Dir) != s.Dir:
		return errors.New("the sprint folder is not a clean path")
	case !insideDir(filepath.Join(root, "sprints"), s.Dir):
		return errors.New("the sprint folder is outside foreman_root\\sprints")
	case s.Dir != t.SprintDir:
		return errors.New("the sprint changed since the preview, press again")
	case !branchRe.MatchString(s.Branch) || !gitrefs.ValidBranch(s.Branch):
		return errors.New("the sprint branch is not a safe branch name")
	case s.Branch != t.Branch:
		return errors.New("the sprint changed since the preview, press again")
	}
	repo := graph.SprintMainRepo(s.WorktreePath)
	if repo == "" {
		return errors.New("the sprint has no worktree to find its repo")
	}
	if merged == nil {
		return errors.New("cannot check whether the branch is in main")
	}
	m, known := merged(repo, s.Branch)
	switch {
	case !known:
		return errors.New("cannot tell whether the branch is in main")
	case !m:
		return errors.New("the branch is not in main")
	}
	return nil
}

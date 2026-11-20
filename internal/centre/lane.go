package centre

import (
	"sort"
	"strconv"
)

// The lane of the Git page: the line of the Fleet's Git tab (Changes, Saved,
// Shared, Review, Merged) plus Released. LaneOf is a pure function: the scan
// gathers LaneIn, LaneOf says what each stage reads.

// LaneStages are the six stage names, in order.
var LaneStages = []string{"Changes", "Saved", "Shared", "Review", "Merged", "Released"}

// Stage states.
const (
	LaneDone   = "done"
	LaneActive = "active"
	LaneWarn   = "warn"
	LaneBad    = "bad"
	LaneTodo   = "todo"
	LaneOff    = "off"
)

// LaneStage is one stage of a lane.
type LaneStage struct {
	Stage string `json:"stage"`
	State string `json:"state"`
	Note  string `json:"note"`
}

// LaneIn is what LaneOf reads about one branch B of a repo whose default
// branch is D. Counts that could not be read are -1 (InOriginD, LacksD,
// SinceTag) or false (StatusRead).
type LaneIn struct {
	Detached     bool
	OnDefault    bool // B is D
	StatusRead   bool // dirty and ahead/behind are known
	Dirty        int
	Conflicts    int
	Upstream     string // "" without an upstream
	Ahead        int    // commits ahead of the upstream
	Behind       int
	AheadUnknown bool // Ahead could not be read
	LacksD       int  // commits B has that D lacks, -1 unknown
	PRsOn        bool
	GitHub       bool   // the repo has a GitHub remote
	PR           *GitPR // newest pull request whose head is B, any state
	InOriginD    int    // 1 the tip of B is in origin/D, 0 not, -1 unknown
	Tags         bool   // the repo has a version tag
	TagsUnknown  bool   // the tags or their commits did not read
	Tag          string // newest version tag
	TagAtHead    bool   // Tag peels to the head of D
	SinceTag     int    // first-parent commits of D after Tag, -1 unknown
	AtTag        string // with Detached: the tag HEAD sits on exactly, "" for none
}

// atTagClean is a checkout detached exactly at a tag with nothing changed: a
// release checkout. Without a read status or with changed files it is not.
func atTagClean(in LaneIn) bool {
	return in.Detached && in.AtTag != "" && in.StatusRead && in.Dirty == 0 && in.Conflicts == 0
}

// LaneOf returns the six stages of one branch.
func LaneOf(in LaneIn) []LaneStage {
	if atTagClean(in) {
		// A tag names a saved commit, so nothing is left to do on this checkout.
		return []LaneStage{
			stage(0, LaneDone, "at tag "+in.AtTag),
			stage(1, LaneDone, "all saved"),
			stage(2, LaneDone, ""),
			stage(3, LaneDone, ""),
			stage(4, LaneDone, ""),
			stage(5, LaneDone, in.AtTag),
		}
	}
	merged := mergedStage(in)
	return []LaneStage{
		changesStage(in),
		savedStage(in),
		sharedStage(in),
		reviewStage(in),
		merged,
		releasedStage(in, merged.State == LaneDone),
	}
}

func stage(i int, state, note string) LaneStage {
	return LaneStage{Stage: LaneStages[i], State: state, Note: note}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

func changesStage(in LaneIn) LaneStage {
	switch {
	case in.Detached:
		return stage(0, LaneBad, "detached")
	case !in.StatusRead:
		return stage(0, LaneOff, "?")
	case in.Conflicts > 0:
		return stage(0, LaneBad, plural(in.Conflicts, "conflict", "conflicts"))
	case in.Dirty > 0:
		return stage(0, LaneActive, plural(in.Dirty, "file", "files"))
	}
	return stage(0, LaneDone, "clean")
}

func savedStage(in LaneIn) LaneStage {
	n := in.Ahead
	if in.AheadUnknown {
		n = -1
	} else if in.Upstream == "" {
		n = in.LacksD
		if in.OnDefault {
			n = 0 // D lacks nothing of itself
		}
	}
	switch {
	case n < 0:
		return stage(1, LaneOff, "?")
	case n > 0:
		return stage(1, LaneActive, strconv.Itoa(n)+" to share")
	case in.Dirty > 0:
		return stage(1, LaneTodo, "")
	}
	return stage(1, LaneDone, "all saved")
}

func sharedStage(in LaneIn) LaneStage {
	switch {
	case in.AheadUnknown:
		return stage(2, LaneOff, "?")
	case in.Upstream == "":
		return stage(2, LaneTodo, "not online")
	case in.Ahead > 0:
		return stage(2, LaneTodo, strconv.Itoa(in.Ahead)+" waiting")
	case in.Behind > 0:
		return stage(2, LaneWarn, strconv.Itoa(in.Behind)+" new online")
	}
	return stage(2, LaneDone, "up to date")
}

func reviewStage(in LaneIn) LaneStage {
	switch {
	case !in.PRsOn:
		return stage(3, LaneOff, "PRs off")
	case in.OnDefault:
		return stage(3, LaneDone, "")
	case !in.GitHub:
		return stage(3, LaneOff, "no GitHub")
	case in.PR == nil:
		return stage(3, LaneTodo, "not asked")
	}
	p := in.PR
	num := "#" + strconv.Itoa(p.Number)
	switch {
	case p.State == "MERGED":
		return stage(3, LaneDone, num)
	case p.State == "CLOSED":
		return stage(3, LaneTodo, num+" closed")
	case p.Checks == "fail":
		return stage(3, LaneBad, num+" checks failed")
	case p.Review == "CHANGES_REQUESTED":
		return stage(3, LaneBad, num+" changes asked")
	case p.Checks == "pending":
		return stage(3, LaneWarn, num+" checks running")
	case p.Review == "APPROVED":
		return stage(3, LaneDone, num)
	}
	return stage(3, LaneWarn, num+" open")
}

func mergedStage(in LaneIn) LaneStage {
	switch {
	case in.OnDefault:
		return stage(4, LaneDone, "")
	case in.PR != nil && in.PR.State == "MERGED":
		return stage(4, LaneDone, "#"+strconv.Itoa(in.PR.Number))
	case in.InOriginD == 1:
		return stage(4, LaneDone, "in origin")
	case in.InOriginD < 0:
		return stage(4, LaneOff, "?")
	}
	return stage(4, LaneTodo, "")
}

// releasedStage reads the repo's release state for D. A branch that is not
// merged is not there yet, so it reads todo.
func releasedStage(in LaneIn, merged bool) LaneStage {
	if !in.OnDefault && !merged {
		return stage(5, LaneTodo, "")
	}
	switch {
	case in.TagsUnknown:
		return stage(5, LaneOff, "?")
	case !in.Tags:
		return stage(5, LaneDone, "no tags")
	case in.TagAtHead:
		return stage(5, LaneDone, in.Tag)
	case in.SinceTag < 0:
		return stage(5, LaneWarn, "after "+in.Tag)
	}
	return stage(5, LaneWarn, strconv.Itoa(in.SinceTag)+" since "+in.Tag)
}

// StopsAt is the first stage that is not done, with its note. A Review stage
// that is off is skipped: no pull request data says nothing about the branch.
// published is true when no stage stops it.
func StopsAt(lane []LaneStage) (stage, note string, published bool) {
	for _, s := range lane {
		if s.State == LaneDone || (s.Stage == "Review" && s.State == LaneOff) {
			continue
		}
		return s.Stage, s.Note, false
	}
	return "", "", true
}

// LaneOpen reports whether a branch or worktree besides the checkout is
// open: its files are not saved, or it is not merged.
func LaneOpen(lane []LaneStage) bool {
	for _, s := range lane {
		if (s.Stage == "Changes" || s.Stage == "Merged") && s.State != LaneDone {
			return true
		}
	}
	return false
}

// NextOf is the one line that says what to do next.
func NextOf(lane []LaneStage) string {
	stage, note, pub := StopsAt(lane)
	if pub {
		return "Nothing open."
	}
	switch stage {
	case "Changes":
		if note == "detached" {
			return "HEAD is detached: put it on a branch."
		}
		return "Save the changes (" + note + ")."
	case "Saved":
		return "Commit and share (" + note + ")."
	case "Shared":
		if note == "not online" {
			return "Put the branch online."
		}
		return "Share the commits (" + note + ")."
	case "Review":
		return "Review: " + note + "."
	case "Merged":
		return "Merge the branch."
	case "Released":
		return "Tag a release (" + note + ")."
	}
	return ""
}

// LaneSummary is the strip on top of the Git page.
type LaneSummary struct {
	Total     int            `json:"total"`
	Open      int            `json:"open"`
	Published int            `json:"published"`
	Stages    map[string]int `json:"stages"` // open repos per stage they stop at
}

// LaneSummaryOf counts the repos.
func LaneSummaryOf(repos []GitRepo) LaneSummary {
	s := LaneSummary{Total: len(repos), Stages: map[string]int{}}
	for _, st := range LaneStages {
		s.Stages[st] = 0
	}
	for _, r := range repos {
		if r.Published {
			s.Published++
			continue
		}
		s.Open++
		if _, ok := s.Stages[r.StopsAt]; ok {
			s.Stages[r.StopsAt]++
		}
	}
	return s
}

func laneRank(r GitRepo) int {
	if r.Published {
		return len(LaneStages)
	}
	for i, st := range LaneStages {
		if st == r.StopsAt {
			return i
		}
	}
	return len(LaneStages) - 1
}

// SortReposByLane puts open repos first, by the earliest stage they stop at,
// and published repos last, inside each unit. The units and the order within
// equal ranks stay as SortRepos left them.
func SortReposByLane(repos []GitRepo) {
	unit := map[string]int{}
	for _, r := range repos {
		if _, ok := unit[r.Unit]; !ok {
			unit[r.Unit] = len(unit)
		}
	}
	sort.SliceStable(repos, func(i, j int) bool {
		a, b := repos[i], repos[j]
		if unit[a.Unit] != unit[b.Unit] {
			return unit[a.Unit] < unit[b.Unit]
		}
		return laneRank(a) < laneRank(b)
	})
}

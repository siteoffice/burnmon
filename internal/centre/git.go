package centre

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"burnmon/internal/gitrefs"
)

// The Git page (overview only, nothing on it changes a repo). Refs, HEAD,
// worktrees, stash, tags and remotes come from .git files through
// internal\gitrefs; dirty and ahead/behind come from one git command,
// GitStatusArgs, run per repo and per linked worktree. Nothing writes inside
// .git.

// GitStatusArgs is the only git command the Control Centre runs.
var GitStatusArgs = []string{"--no-optional-locks", "status", "--porcelain=v2", "--branch"}

// GitStatusTimeout is the most one git status may take.
const GitStatusTimeout = 5 * time.Second

// GhArgs is the gh command for one repo, before "-R owner/name".
var GhArgs = []string{"pr", "list", "--json", "number,title,headRefName,state,isDraft,reviewDecision,statusCheckRollup,updatedAt"}

// Attention reasons, in the order the row shows them.
const (
	AttnConflicts = "conflicts"
	AttnDirty     = "dirty"
	AttnBehind    = "behind"
	AttnDetached  = "detached"
)

// GitRepo is one row of the Git page.
type GitRepo struct {
	ID          string   `json:"id"`   // root-relative folder, "." for the root
	Name        string   `json:"name"` // folder name
	Unit        string   `json:"unit"` // nearest unit at or above the folder, "" for none
	Branch      string   `json:"branch"`
	Detached    bool     `json:"detached"`
	Upstream    string   `json:"upstream"` // "" without an upstream
	Ahead       int      `json:"ahead"`
	Behind      int      `json:"behind"`
	Dirty       int      `json:"dirty"`
	Conflicts   int      `json:"conflicts"`
	StatusError string   `json:"status_error"` // why dirty and ahead/behind are unknown
	LastSHA     string   `json:"last_sha"`     // 7 characters
	LastAt      string   `json:"last_at"`      // RFC3339 UTC, "" when unknown
	LastAge     string   `json:"last_age"`     // "3 d", "5 h", "12 min"
	LastSubject string   `json:"last_subject"`
	Stashes     int      `json:"stashes"`
	Worktrees   int      `json:"worktrees"` // linked worktrees, the main one not counted
	GitHub      string   `json:"github"`    // owner/name of a GitHub remote, "" for none
	PRs         int      `json:"prs"`       // open pull requests, -1 when not read
	Attention   []string `json:"attention"`
}

// GitWorktree is one worktree of a repo, the main one first.
type GitWorktree struct {
	Path        string   `json:"path"` // absolute
	Name        string   `json:"name"` // folder name
	Main        bool     `json:"main"`
	Branch      string   `json:"branch"`
	Detached    bool     `json:"detached"`
	Missing     bool     `json:"missing"` // registered, but the folder is gone
	Dirty       int      `json:"dirty"`
	StatusError string   `json:"status_error"`
	Sessions    []string `json:"sessions"` // agent sessions whose folder name matches
}

// GitBranch is a local branch; Merged is "trunk", "merged", "not merged" or "unknown".
type GitBranch struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
	Merged  string `json:"merged"`
}

type GitRemote struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// GitPR is one open pull request as gh reports it.
type GitPR struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Branch    string `json:"branch"`
	State     string `json:"state"`
	Draft     bool   `json:"draft"`
	Review    string `json:"review"` // gh reviewDecision, "" when none
	Checks    string `json:"checks"` // "pass", "fail", "pending" or ""
	UpdatedAt string `json:"updated_at"`
}

// GitDetail is the repo page.
type GitDetail struct {
	Repo      GitRepo       `json:"repo"`
	Path      string        `json:"path"` // absolute
	Worktrees []GitWorktree `json:"worktrees"`
	Branches  []GitBranch   `json:"branches"`
	Tags      []string      `json:"tags"` // newest version first
	Remotes   []GitRemote   `json:"remotes"`
	PRs       []GitPR       `json:"prs"`
	PRLine    string        `json:"pr_line"`
}

// RepoDir is a repo found below root.
type RepoDir struct {
	ID  string // root-relative, forward slashes
	Abs string
}

// FindRepos walks root to MaxDepth with the unit walk's skip rules and
// returns every folder holding a .git folder, or a .git file that is not a
// linked worktree. Linked worktrees belong to their repo. Sorted by id.
func FindRepos(root string) []RepoDir {
	out := []RepoDir{}
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
		if id != "." {
			if Skip(d.Name()) {
				return fs.SkipDir
			}
			if strings.Count(id, "/")+1 > MaxDepth {
				return fs.SkipDir
			}
		}
		if isRepoFolder(p) {
			out = append(out, RepoDir{ID: id, Abs: p})
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return idLess(out[i].ID, out[j].ID) })
	return out
}

func isRepoFolder(dir string) bool {
	dot := filepath.Join(dir, ".git")
	fi, err := os.Lstat(dot)
	if err != nil {
		return false
	}
	if fi.IsDir() {
		return hasHead(dot)
	}
	if !fi.Mode().IsRegular() {
		return false
	}
	gd := worktreeGitDir(dir)
	if gd == "" || !hasHead(gd) {
		return false
	}
	_, err = os.Stat(filepath.Join(gd, "commondir"))
	return err != nil // a commondir file marks a linked worktree
}

// hasHead reports whether a git dir holds a HEAD file. Git treats a folder
// without one as no repo; a stray .git\info\exclude left by a shell that ate
// backslashes is such a folder.
func hasHead(gd string) bool {
	fi, err := os.Stat(filepath.Join(gd, "HEAD"))
	return err == nil && fi.Mode().IsRegular()
}

// worktreeGitDir is the per-worktree git dir of dir: dir\.git when it is a
// folder, else the gitdir line of the .git file. "" when unreadable.
func worktreeGitDir(dir string) string {
	dot := filepath.Join(dir, ".git")
	fi, err := os.Lstat(dot)
	if err != nil {
		return ""
	}
	if fi.IsDir() {
		return dot
	}
	data, err := os.ReadFile(dot)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir:") {
		return ""
	}
	gd := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
	if gd == "" {
		return ""
	}
	if !filepath.IsAbs(gd) {
		gd = filepath.Join(dir, gd)
	}
	return filepath.Clean(gd)
}

// readHead reads HEAD in a per-worktree git dir: the branch name, or
// detached with the object name.
func readHead(wtGitDir string) (branch, sha string, detached bool, err error) {
	data, err := os.ReadFile(filepath.Join(wtGitDir, "HEAD"))
	if err != nil {
		return "", "", false, err
	}
	v := strings.TrimSpace(string(data))
	if strings.HasPrefix(v, "ref:") {
		ref := strings.TrimSpace(strings.TrimPrefix(v, "ref:"))
		return strings.TrimPrefix(ref, "refs/heads/"), "", false, nil
	}
	return "", v, true, nil
}

// RepoFiles is what the .git files say about a repo.
type RepoFiles struct {
	GitDir    string // common git dir
	Branch    string
	Detached  bool
	HeadSHA   string // "" for a branch without commits
	Last      gitrefs.Commit
	LastErr   error
	Stashes   int
	Worktrees []GitWorktree // main first, without Dirty and Sessions
	Remotes   []GitRemote
}

// ReadRepoFiles reads a repo's .git files. It never runs git.
func ReadRepoFiles(dir string) (RepoFiles, error) {
	var rf RepoFiles
	gd, err := gitrefs.GitDir(dir)
	if err != nil {
		return rf, err
	}
	rf.GitDir = gd
	wgd := worktreeGitDir(dir)
	if wgd == "" {
		return rf, errors.New("no git dir")
	}
	rf.Branch, rf.HeadSHA, rf.Detached, err = readHead(wgd)
	if err != nil {
		return rf, err
	}
	if !rf.Detached && rf.Branch != "" {
		if sha, err := gitrefs.ResolveRef(gd, "refs/heads/"+rf.Branch); err == nil {
			rf.HeadSHA = sha
		}
	}
	if rf.HeadSHA != "" {
		rf.Last, rf.LastErr = gitrefs.CommitInfo(gd, rf.HeadSHA)
	} else {
		rf.LastErr = errors.New("no commits")
	}
	rf.Stashes = countLines(filepath.Join(gd, "logs", "refs", "stash"))
	rf.Worktrees = append([]GitWorktree{{Path: dir, Name: filepath.Base(dir), Main: true, Branch: rf.Branch, Detached: rf.Detached}},
		linkedWorktrees(gd)...)
	rf.Remotes = readRemotes(filepath.Join(gd, "config"))
	return rf, nil
}

func countLines(p string) int {
	data, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	n := 0
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

// linkedWorktrees reads <gitdir>\worktrees\*: gitdir names the worktree's
// .git file, HEAD its branch. Sorted by path.
func linkedWorktrees(gd string) []GitWorktree {
	out := []GitWorktree{}
	entries, err := os.ReadDir(filepath.Join(gd, "worktrees"))
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		wd := filepath.Join(gd, "worktrees", e.Name())
		data, err := os.ReadFile(filepath.Join(wd, "gitdir"))
		if err != nil {
			continue
		}
		dotGit := strings.TrimSpace(string(data))
		if !filepath.IsAbs(dotGit) {
			dotGit = filepath.Join(wd, dotGit)
		}
		p := filepath.Dir(filepath.Clean(dotGit))
		w := GitWorktree{Path: p, Name: filepath.Base(p)}
		w.Branch, _, w.Detached, _ = readHead(wd)
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			w.Missing = true
		}
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// readRemotes reads the [remote "name"] url lines of a git config file, in
// file order.
func readRemotes(p string) []GitRemote {
	out := []GitRemote{}
	f, err := os.Open(p)
	if err != nil {
		return out
	}
	defer f.Close()
	cur := -1
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			cur = -1
			if m := remoteHead.FindStringSubmatch(line); m != nil {
				out = append(out, GitRemote{Name: m[1]})
				cur = len(out) - 1
			}
			continue
		}
		if cur < 0 {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.EqualFold(strings.TrimSpace(k), "url") && out[cur].URL == "" {
			out[cur].URL = strings.TrimSpace(v)
		}
	}
	return out
}

var remoteHead = regexp.MustCompile(`^\[remote\s+"([^"]+)"\]$`)

var githubURL = regexp.MustCompile(`^(?:https://(?:[^@/]+@)?github\.com/|git@github\.com:|ssh://git@github\.com/)([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`)

// GitHubSlug is owner/name of the first GitHub remote, origin first.
func GitHubSlug(remotes []GitRemote) string {
	ordered := append([]GitRemote{}, remotes...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Name == "origin" && ordered[j].Name != "origin" })
	for _, r := range ordered {
		if m := githubURL.FindStringSubmatch(r.URL); m != nil {
			return m[1] + "/" + m[2]
		}
	}
	return ""
}

// StatusInfo is what one git status --porcelain=v2 --branch says.
type StatusInfo struct {
	Head        string // branch.head; "(detached)" when detached
	Upstream    string
	HasAB       bool
	Ahead       int
	Behind      int
	Dirty       int // changed, unmerged and untracked entries
	Conflicts   int // unmerged entries
	Detached    bool
	Initial     bool
	OID         string
	Unparseable bool
}

// ParseStatus reads porcelain v2 output with --branch headers.
func ParseStatus(out string) StatusInfo {
	var s StatusInfo
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		switch {
		case line == "":
		case strings.HasPrefix(line, "# branch.oid "):
			s.OID = strings.TrimPrefix(line, "# branch.oid ")
			s.Initial = s.OID == "(initial)"
		case strings.HasPrefix(line, "# branch.head "):
			s.Head = strings.TrimPrefix(line, "# branch.head ")
			s.Detached = s.Head == "(detached)"
		case strings.HasPrefix(line, "# branch.upstream "):
			s.Upstream = strings.TrimPrefix(line, "# branch.upstream ")
		case strings.HasPrefix(line, "# branch.ab "):
			f := strings.Fields(strings.TrimPrefix(line, "# branch.ab "))
			if len(f) == 2 {
				a, e1 := strconv.Atoi(strings.TrimPrefix(f[0], "+"))
				b, e2 := strconv.Atoi(strings.TrimPrefix(f[1], "-"))
				if e1 == nil && e2 == nil {
					s.HasAB, s.Ahead, s.Behind = true, a, b
				}
			}
		case strings.HasPrefix(line, "# "):
		case strings.HasPrefix(line, "1 "), strings.HasPrefix(line, "2 "), strings.HasPrefix(line, "? "):
			s.Dirty++
		case strings.HasPrefix(line, "u "):
			s.Dirty++
			s.Conflicts++
		case strings.HasPrefix(line, "! "):
		default:
			s.Unparseable = true
		}
	}
	return s
}

// AttentionOf lists why a repo needs attention.
func AttentionOf(r GitRepo) []string {
	out := []string{}
	if r.Conflicts > 0 {
		out = append(out, AttnConflicts)
	}
	if r.Dirty > 0 {
		out = append(out, AttnDirty)
	}
	if r.Behind > 0 {
		out = append(out, AttnBehind)
	}
	if r.Detached {
		out = append(out, AttnDetached)
	}
	return out
}

// SortRepos puts repos that need attention first, then the newest last
// commit, then id. Then it groups by unit: the groups keep the order of
// their first repo, and a repo keeps its place inside its group.
func SortRepos(repos []GitRepo) {
	sort.SliceStable(repos, func(i, j int) bool {
		a, b := repos[i], repos[j]
		if (len(a.Attention) > 0) != (len(b.Attention) > 0) {
			return len(a.Attention) > 0
		}
		if a.LastAt != b.LastAt {
			return a.LastAt > b.LastAt // RFC3339 UTC sorts as text; "" last
		}
		return idLess(a.ID, b.ID)
	})
	rank := map[string]int{}
	for _, r := range repos {
		if _, ok := rank[r.Unit]; !ok {
			rank[r.Unit] = len(rank)
		}
	}
	sort.SliceStable(repos, func(i, j int) bool { return rank[repos[i].Unit] < rank[repos[j].Unit] })
}

// GitPages splits sorted repos into pages of at most rows lines, where a
// unit header line comes before the first repo of a unit on each page.
func GitPages(repos []GitRepo, rows int) []Span {
	if rows < 2 {
		rows = 2
	}
	out := []Span{}
	from, used := 0, 0
	for i, r := range repos {
		need := 1
		if used == 0 || repos[i-1].Unit != r.Unit {
			need = 2
		}
		if used > 0 && used+need > rows {
			out = append(out, Span{From: from, To: i})
			from, used, need = i, 0, 2
		}
		used += need
	}
	if len(repos) > 0 {
		out = append(out, Span{From: from, To: len(repos)})
	}
	return out
}

// OwningUnit is the nearest unit id at or above the repo folder id.
func OwningUnit(id string, units []Unit) string {
	has := map[string]bool{}
	for _, u := range units {
		has[u.ID] = true
	}
	for d := id; ; d = path.Dir(d) {
		if has[d] {
			return d
		}
		if d == "." || d == "/" || d == "" {
			return ""
		}
	}
}

// Age is a short age: "12 min", "5 h", "3 d"; "" for a zero time.
func Age(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < time.Hour:
		m := int(d / time.Minute)
		if m < 0 {
			m = 0
		}
		return strconv.Itoa(m) + " min"
	case d < 48*time.Hour:
		return strconv.Itoa(int(d/time.Hour)) + " h"
	}
	return strconv.Itoa(int(d/(24*time.Hour))) + " d"
}

// SortTags orders tag names newest version first: digit runs compare as
// numbers, a pre-release (a "-" part) before its release is older.
func SortTags(tags []string) {
	sort.SliceStable(tags, func(i, j int) bool { return natLess(tags[j], tags[i]) })
}

func natLess(a, b string) bool {
	ab, ap, _ := strings.Cut(a, "-")
	bb, bp, _ := strings.Cut(b, "-")
	if c := natCmp(ab, bb); c != 0 {
		return c < 0
	}
	switch {
	case ap == bp:
		return false
	case ap == "":
		return false // a release is newer than its pre-release
	case bp == "":
		return true
	}
	return natCmp(ap, bp) < 0
}

func natCmp(a, b string) int {
	for a != "" && b != "" {
		da, db := isDigit(a[0]), isDigit(b[0])
		if da && db {
			na, ra := digitRun(a)
			nb, rb := digitRun(b)
			if len(na) != len(nb) {
				if len(na) < len(nb) {
					return -1
				}
				return 1
			}
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
			a, b = ra, rb
			continue
		}
		if a[0] != b[0] {
			if a[0] < b[0] {
				return -1
			}
			return 1
		}
		a, b = a[1:], b[1:]
	}
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return -1
	}
	return 1
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// digitRun returns the leading digits without leading zeros, and the rest.
func digitRun(s string) (string, string) {
	i := 0
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	n := strings.TrimLeft(s[:i], "0")
	return n, s[i:]
}

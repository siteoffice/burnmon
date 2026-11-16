package centre

import (
	"errors"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"burnmon/internal/gitrefs"
)

// GitRescanAfter is how old a Git page reading may get before the next
// visit reads again. PRCacheTTL is how long one repo's pull requests are kept.
const (
	GitRescanAfter = 30 * time.Second
	PRCacheTTL     = 5 * time.Minute
	gitWorkers     = 4
)

// GitDeps is what the scanner runs and reads; tests pass fakes.
type GitDeps struct {
	Status    func(dir string) (string, error) // nil: StatusWhy says why
	StatusWhy string
	PRs       func(slug string) ([]GitPR, error)
	Sessions  func() map[string][]string // worktree folder name -> session labels
	Merged    func(repo, branch string) (merged, known bool)
	Now       func() time.Time
}

// GitView is what the Git page draws.
type GitView struct {
	Root    string      `json:"root"`
	Loading bool        `json:"loading"` // a reading runs; the page asks again
	ReadAt  string      `json:"read_at"` // RFC3339 UTC of the rows shown, "" before the first
	Line    string      `json:"line"`    // git missing, else ""
	PRsOn   bool        `json:"prs_on"`
	PRLine  string      `json:"pr_line"` // gh missing, signed out or failing, else ""
	Repos   []GitRepo   `json:"repos"`
	Pages   []Span      `json:"pages"`
	Summary LaneSummary `json:"summary"` // open repos per lane stage
}

// GitDetailView is what the repo page draws.
type GitDetailView struct {
	Loading bool       `json:"loading"`
	Found   bool       `json:"found"`
	Detail  *GitDetail `json:"detail"`
}

type repoScan struct {
	repo      GitRepo
	abs       string
	gitDir    string
	worktrees []GitWorktree
	remotes   []GitRemote
	prs       []GitPR                // open pull requests only
	prsAll    []GitPR                // every state, for the Review and Merged stages
	lanes     map[string][]LaneStage // lane per local branch name
}

type prEntry struct {
	at  time.Time
	prs []GitPR
	err error
}

// GitScanner reads the repos below a root in the background, so no binding
// waits on git or gh. One reading runs at a time.
type GitScanner struct {
	deps GitDeps
	wg   sync.WaitGroup

	mu         sync.Mutex
	root       string
	prsOn      bool
	gen        int
	running    bool
	read       bool
	readAt     time.Time
	repos      []repoScan
	prLine     string
	prCache    map[string]prEntry
	details    map[string]*GitDetail
	detailBusy map[string]bool

	laneMu    sync.Mutex
	laneCache map[string]laneCounts
	relCache  map[string]releaseInfo
}

// NewGitScanner returns a scanner with no reading yet.
func NewGitScanner(deps GitDeps) *GitScanner {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &GitScanner{deps: deps, prCache: map[string]prEntry{}, details: map[string]*GitDetail{}, detailBusy: map[string]bool{},
		laneCache: map[string]laneCounts{}, relCache: map[string]releaseInfo{}}
}

// Wait blocks until every reading started so far has finished.
func (s *GitScanner) Wait() { s.wg.Wait() }

// View returns the rows of the last reading of root and starts a new one
// when there is none, when root or the PR switch changed, when refresh is
// set (which also drops the PR cache) or when the last is over
// GitRescanAfter old.
func (s *GitScanner) View(root string, prsOn, refresh bool, rows int) GitView {
	return s.ViewStage(root, prsOn, refresh, rows, "")
}

// ViewStage is View for the rows that stop at one lane stage ("" for every
// repo). The pages hold the filtered rows; Summary always counts all repos.
func (s *GitScanner) ViewStage(root string, prsOn, refresh bool, rows int, stage string) GitView {
	s.mu.Lock()
	defer s.mu.Unlock()
	if root != s.root || prsOn != s.prsOn {
		s.root, s.prsOn = root, prsOn
		s.gen++
		s.read, s.repos, s.prLine = false, nil, ""
		s.details, s.detailBusy = map[string]*GitDetail{}, map[string]bool{}
	}
	now := s.deps.Now()
	if refresh {
		s.prCache = map[string]prEntry{}
	}
	if !s.running && (!s.read || refresh || now.Sub(s.readAt) > GitRescanAfter) {
		s.start()
	}
	v := GitView{Root: root, Loading: s.running, Line: s.deps.StatusWhy, PRsOn: prsOn, PRLine: s.prLine,
		Repos: []GitRepo{}, Pages: []Span{}, Summary: LaneSummaryOf(nil)}
	if s.deps.Status != nil {
		v.Line = ""
	}
	if s.read {
		v.ReadAt = s.readAt.UTC().Format(time.RFC3339)
		for _, r := range s.repos {
			v.Repos = append(v.Repos, r.repo)
		}
		v.Summary = LaneSummaryOf(v.Repos)
		if stage != "" {
			kept := []GitRepo{}
			for _, r := range v.Repos {
				if !r.Published && r.StopsAt == stage {
					kept = append(kept, r)
				}
			}
			v.Repos = kept
		}
		v.Pages = GitPages(v.Repos, rows)
	}
	return v
}

// start runs one reading; s.mu is held.
func (s *GitScanner) start() {
	s.running = true
	gen, root, prsOn := s.gen, s.root, s.prsOn
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		repos, line := s.scan(root, prsOn)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.running = false
		if gen != s.gen {
			return // root or switch changed meanwhile; the next View reads again
		}
		s.read, s.readAt, s.repos, s.prLine = true, s.deps.Now(), repos, line
		s.details, s.detailBusy = map[string]*GitDetail{}, map[string]bool{}
	}()
}

func (s *GitScanner) scan(root string, prsOn bool) ([]repoScan, string) {
	units, _ := FindUnits(root)
	dirs := FindRepos(root)
	out := make([]repoScan, len(dirs))
	errs := make([]error, len(dirs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, gitWorkers)
	for i, d := range dirs {
		wg.Add(1)
		go func(i int, d RepoDir) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i], errs[i] = s.scanRepo(root, d, units, prsOn)
		}(i, d)
	}
	wg.Wait()
	line := ""
	for _, err := range errs {
		if err == nil {
			continue
		}
		if errors.Is(err, ErrGhMissing) || errors.Is(err, ErrGhSignedOut) {
			line = err.Error()
			break
		}
		if line == "" {
			line = "Pull requests: " + err.Error()
		}
	}
	repos := make([]GitRepo, len(out))
	for i := range out {
		repos[i] = out[i].repo
	}
	SortRepos(repos)
	SortReposByLane(repos)
	byID := map[string]repoScan{}
	for _, r := range out {
		byID[r.repo.ID] = r
	}
	sorted := make([]repoScan, len(repos))
	for i, r := range repos {
		sorted[i] = byID[r.ID]
	}
	return sorted, line
}

// scanRepo reads one repo; the error is a pull request error only.
func (s *GitScanner) scanRepo(root string, d RepoDir, units []Unit, prsOn bool) (repoScan, error) {
	name := filepath.Base(d.Abs)
	rs := repoScan{abs: d.Abs, repo: GitRepo{ID: d.ID, Name: name, Unit: OwningUnit(d.ID, units), PRs: -1, Attention: []string{},
		Lane: []LaneStage{}, OpenBranches: []OpenBranch{}}}
	rf, err := ReadRepoFiles(d.Abs)
	if err != nil {
		rs.repo.StatusError = "the .git files did not read: " + err.Error()
		rs.repo.StopsAt, rs.repo.StopsNote = "Changes", "not read"
		return rs, nil
	}
	now := s.deps.Now()
	r := &rs.repo
	rs.gitDir, rs.remotes, rs.worktrees = rf.GitDir, rf.Remotes, rf.Worktrees
	r.Branch, r.Detached, r.Stashes = rf.Branch, rf.Detached, rf.Stashes
	if rf.Detached {
		r.Branch = ""
	}
	r.Worktrees = len(rf.Worktrees) - 1
	r.GitHub = GitHubSlug(rf.Remotes)
	if len(rf.HeadSHA) >= 7 {
		r.LastSHA = rf.HeadSHA[:7]
	}
	if rf.LastErr == nil && !rf.Last.When.IsZero() {
		r.LastAt = rf.Last.When.UTC().Format(time.RFC3339)
		r.LastAge = Age(rf.Last.When, now)
		r.LastSubject = rf.Last.Subject
	}
	if s.deps.Status == nil {
		r.StatusError = s.deps.StatusWhy
	} else {
		for i := range rs.worktrees {
			w := &rs.worktrees[i]
			if w.Missing {
				w.StatusError = "the folder is gone"
				continue
			}
			out, err := s.deps.Status(w.Path)
			if err != nil {
				w.StatusError = err.Error()
				continue
			}
			st := ParseStatus(out)
			w.Dirty = st.Dirty
			w.statusRead, w.conflicts, w.upstream = true, st.Conflicts, st.Upstream
			if st.HasAB {
				w.ahead, w.behind = st.Ahead, st.Behind
			}
			if w.Main {
				r.Upstream, r.Dirty, r.Conflicts = st.Upstream, st.Dirty, st.Conflicts
				if st.HasAB {
					r.Ahead, r.Behind = st.Ahead, st.Behind
				}
				if st.Detached {
					r.Detached, r.Branch = true, ""
				}
			}
		}
		if len(rs.worktrees) > 0 && rs.worktrees[0].StatusError != "" {
			r.StatusError = rs.worktrees[0].StatusError
		}
	}
	r.Attention = AttentionOf(*r)
	var prErr error
	if prsOn && r.GitHub != "" && s.deps.PRs != nil {
		rs.prsAll, prErr = s.prs(r.GitHub, now)
		if prErr == nil {
			rs.prs = []GitPR{}
			for _, p := range rs.prsAll {
				if p.State == "OPEN" {
					rs.prs = append(rs.prs, p)
				}
			}
			r.PRs = len(rs.prs)
		}
	}
	s.computeLanes(&rs, prsOn && prErr == nil && r.GitHub != "")
	return rs, prErr
}

// prs returns a repo's pull requests from the cache when under PRCacheTTL old.
func (s *GitScanner) prs(slug string, now time.Time) ([]GitPR, error) {
	s.mu.Lock()
	e, ok := s.prCache[slug]
	s.mu.Unlock()
	if ok && now.Sub(e.at) < PRCacheTTL {
		return e.prs, e.err
	}
	prs, err := s.deps.PRs(slug)
	s.mu.Lock()
	s.prCache[slug] = prEntry{at: now, prs: prs, err: err}
	s.mu.Unlock()
	return prs, err
}

// Detail returns the repo page of id from the last reading of root. The
// branch merge check reads commits, so it runs in the background: the first
// call answers Loading and the page asks again.
func (s *GitScanner) Detail(root, id string) GitDetailView {
	s.mu.Lock()
	defer s.mu.Unlock()
	if root != s.root || !s.read {
		return GitDetailView{Loading: s.running}
	}
	if d, ok := s.details[id]; ok {
		return GitDetailView{Found: true, Detail: d}
	}
	var rs *repoScan
	for i := range s.repos {
		if s.repos[i].repo.ID == id {
			rs = &s.repos[i]
		}
	}
	if rs == nil {
		return GitDetailView{}
	}
	if !s.detailBusy[id] {
		s.detailBusy[id] = true
		gen, cp, line := s.gen, *rs, s.prLine
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			d := s.buildDetail(cp, line)
			s.mu.Lock()
			defer s.mu.Unlock()
			if gen == s.gen {
				s.details[id] = d
				delete(s.detailBusy, id)
			}
		}()
	}
	return GitDetailView{Loading: true, Found: true}
}

func (s *GitScanner) buildDetail(rs repoScan, prLine string) *GitDetail {
	d := &GitDetail{Repo: rs.repo, Path: rs.abs, Worktrees: []GitWorktree{}, Branches: []GitBranch{}, Tags: []string{},
		Remotes: append([]GitRemote{}, rs.remotes...), PRs: []GitPR{}, PRLine: prLine}
	sessions := map[string][]string{}
	if s.deps.Sessions != nil {
		sessions = s.deps.Sessions()
	}
	for _, w := range rs.worktrees {
		w.Sessions = append([]string{}, sessions[w.Name]...)
		d.Worktrees = append(d.Worktrees, w)
	}
	if rs.gitDir != "" {
		heads, _ := gitrefs.ListRefs(rs.gitDir, "refs/heads/")
		for name := range heads {
			b := GitBranch{Name: name, Current: name == rs.repo.Branch, Merged: "unknown"}
			switch {
			case name == "main" || name == "master":
				b.Merged = "trunk"
			case s.deps.Merged != nil:
				if m, known := s.deps.Merged(rs.abs, name); known && m {
					b.Merged = "merged"
				} else if known {
					b.Merged = "not merged"
				}
			}
			b.Lane = rs.lanes[name]
			d.Branches = append(d.Branches, b)
		}
		sort.Slice(d.Branches, func(i, j int) bool {
			a, b := d.Branches[i], d.Branches[j]
			if (a.Merged == "trunk") != (b.Merged == "trunk") {
				return a.Merged == "trunk"
			}
			return a.Name < b.Name
		})
		tags, _ := gitrefs.ListRefs(rs.gitDir, "refs/tags/")
		for t := range tags {
			d.Tags = append(d.Tags, t)
		}
		SortTags(d.Tags)
	}
	if rs.prs != nil {
		d.PRs = rs.prs
	}
	return d
}

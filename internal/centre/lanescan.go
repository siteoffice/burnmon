package centre

import (
	"regexp"
	"sort"

	"burnmon/internal/gitrefs"
)

// The scan side of the lane: gather LaneIn for every local branch of a repo
// from the .git files (internal\gitrefs), never from a new git command. A
// repo's default branch D is refs/heads/main, else refs/heads/master.

var versionTag = regexp.MustCompile(`^v?[0-9]+\.[0-9]+`)

// laneCounts are the reachability answers for one branch tip, cached by the
// refs they were read from so a rescan with unmoved refs walks nothing.
type laneCounts struct {
	lacksD       int // -1 unknown
	inOriginD    int // 1, 0, -1
	ahead        int
	behind       int
	aheadUnknown bool
}

type releaseInfo struct {
	tags, unknown, atHead bool
	tag                   string
	since                 int
}

const laneCacheMax = 1024

func (s *GitScanner) cached(key string, read func() laneCounts) laneCounts {
	s.laneMu.Lock()
	c, ok := s.laneCache[key]
	s.laneMu.Unlock()
	if ok {
		return c
	}
	c = read()
	s.laneMu.Lock()
	if len(s.laneCache) >= laneCacheMax {
		s.laneCache = map[string]laneCounts{}
	}
	s.laneCache[key] = c
	s.laneMu.Unlock()
	return c
}

// releaseOf reads the newest version tag and where D stands against it. The
// answer is cached by the git dir, D's tip and the newest tag's object name.
func (s *GitScanner) releaseOf(gd string, reach *gitrefs.Reach, dSHA string) releaseInfo {
	tags, err := gitrefs.ListRefs(gd, "refs/tags/")
	if err != nil {
		return releaseInfo{unknown: true}
	}
	names := []string{}
	for n := range tags {
		if versionTag.MatchString(n) {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return releaseInfo{}
	}
	SortTags(names)
	key := gd + "|" + dSHA + "|" + names[0] + "|" + tags[names[0]]
	s.laneMu.Lock()
	ri, ok := s.relCache[key]
	s.laneMu.Unlock()
	if ok {
		return ri
	}
	ri = releaseInfo{tags: true, tag: names[0], since: -1}
	commit, ok := reach.Peel(tags[names[0]])
	switch {
	case !ok || dSHA == "":
		ri.unknown = true
	case commit == dSHA:
		ri.atHead = true
	default:
		if n, found, known := reach.FirstParentSteps(dSHA, commit); found && known {
			ri.since = n
		}
	}
	s.laneMu.Lock()
	if len(s.relCache) >= laneCacheMax {
		s.relCache = map[string]releaseInfo{}
	}
	s.relCache[key] = ri
	s.laneMu.Unlock()
	return ri
}

// computeLanes fills the lane of the checkout, of every linked worktree and
// every local branch, and the open branches. prsOn says the pull request data
// of this repo was read.
func (s *GitScanner) computeLanes(rs *repoScan, prsOn bool) {
	r := &rs.repo
	rs.lanes = map[string][]LaneStage{}
	if rs.gitDir == "" || len(rs.worktrees) == 0 {
		return
	}
	gd := rs.gitDir
	heads, _ := gitrefs.ListRefs(gd, "refs/heads/")
	d := ""
	for _, n := range []string{"main", "master"} {
		if _, ok := heads[n]; ok {
			d = n
			break
		}
	}
	dSHA := heads[d]
	reach := gitrefs.OpenReach(gd)
	defer reach.Close()
	originD := ""
	if d != "" {
		if sha, err := gitrefs.ResolveRef(gd, "refs/remotes/origin/"+d); err == nil {
			originD = sha
		}
	}
	rel := s.releaseOf(gd, reach, dSHA)

	prOf := map[string]*GitPR{}
	for i := range rs.prsAll {
		p := &rs.prsAll[i]
		if cur := prOf[p.Branch]; cur == nil || p.UpdatedAt > cur.UpdatedAt {
			prOf[p.Branch] = p
		}
	}
	wtOf := map[string]*GitWorktree{}
	for i := range rs.worktrees {
		w := &rs.worktrees[i]
		if !w.Detached && w.Branch != "" && !w.Missing {
			wtOf[w.Branch] = w
		}
	}

	build := func(name, tip string, w *GitWorktree, detached bool) LaneIn {
		in := LaneIn{OnDefault: !detached && name != "" && name == d, PRsOn: prsOn, GitHub: r.GitHub != "",
			PR: prOf[name], InOriginD: -1, LacksD: -1, Detached: detached,
			Tags: rel.tags, TagsUnknown: rel.unknown, Tag: rel.tag, TagAtHead: rel.atHead, SinceTag: rel.since}
		if w != nil {
			in.StatusRead, in.Dirty, in.Conflicts = w.statusRead, w.Dirty, w.conflicts
			in.Upstream, in.Ahead, in.Behind = w.upstream, w.ahead, w.behind
		}
		if detached || tip == "" {
			return in
		}
		upSHA := ""
		if w == nil {
			in.StatusRead = true // nothing is checked out, so nothing is unsaved
			if sha, err := gitrefs.ResolveRef(gd, "refs/remotes/origin/"+name); err == nil {
				upSHA = sha
				in.Upstream = "origin/" + name
			}
		}
		base := originD
		if base == "" {
			base = dSHA
		}
		key := gd + "|" + tip + "|" + dSHA + "|" + base + "|" + upSHA
		c := s.cached(key, func() laneCounts {
			c := laneCounts{lacksD: -1, inOriginD: -1}
			if base != "" {
				if in, ok := reach.Contains(base, tip); ok {
					c.inOriginD = 0
					if in {
						c.inOriginD = 1
					}
				}
			}
			if dSHA != "" {
				if n, ok := reach.NotIn(tip, dSHA); ok {
					c.lacksD = n
				}
			}
			if upSHA != "" {
				a, ok1 := reach.NotIn(tip, upSHA)
				b, ok2 := reach.NotIn(upSHA, tip)
				c.ahead, c.behind, c.aheadUnknown = a, b, !ok1 || !ok2
			}
			return c
		})
		in.InOriginD, in.LacksD = c.inOriginD, c.lacksD
		if w == nil {
			in.Ahead, in.Behind, in.AheadUnknown = c.ahead, c.behind, c.aheadUnknown
		}
		return in
	}

	for name, tip := range heads {
		rs.lanes[name] = LaneOf(build(name, tip, wtOf[name], false))
	}
	w0 := &rs.worktrees[0]
	detached := w0.Detached || r.Detached
	if detached {
		r.Lane = LaneOf(build("", "", w0, true))
	} else if l, ok := rs.lanes[w0.Branch]; ok {
		r.Lane = l
	} else { // a branch without commits has no ref yet
		r.Lane = LaneOf(build(w0.Branch, "", w0, false))
	}
	onD := !detached && w0.Branch == d
	for i := 1; i < len(rs.worktrees); i++ {
		w := &rs.worktrees[i]
		if w.Detached {
			w.Lane = LaneOf(build("", "", w, true))
		} else if l, ok := rs.lanes[w.Branch]; ok {
			w.Lane = l
		} else {
			w.Lane = LaneOf(build(w.Branch, "", w, false))
		}
	}

	open := []OpenBranch{}
	noteOf := func(lane []LaneStage) string {
		st, note, _ := StopsAt(lane)
		if note != "" {
			return st + ": " + note
		}
		return st
	}
	for i := 1; i < len(rs.worktrees); i++ {
		w := &rs.worktrees[i]
		if w.Missing {
			continue
		}
		_, _, pub := StopsAt(w.Lane)
		if LaneOpen(w.Lane) || (!onD && !w.Detached && w.Branch == d && !pub) {
			name := w.Name
			if w.Branch != "" {
				name += " (" + w.Branch + ")"
			}
			open = append(open, OpenBranch{Name: name, Kind: "worktree", Lane: w.Lane, Note: noteOf(w.Lane)})
		}
	}
	names := make([]string, 0, len(heads))
	for n := range heads {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if wtOf[n] != nil {
			continue
		}
		lane := rs.lanes[n]
		_, _, pub := StopsAt(lane)
		isD := n == d
		if (isD && !onD && !pub && r.Lane[5].State == LaneTodo) || (!isD && LaneOpen(lane)) {
			open = append(open, OpenBranch{Name: n, Kind: "branch", Lane: lane, Note: noteOf(lane)})
		}
	}
	r.OpenBranches = open

	stage, note, pub := StopsAt(r.Lane)
	next := NextOf(r.Lane)
	if pub && len(open) > 0 {
		first := open[0]
		stage, _, _ = StopsAt(first.Lane)
		_, n, _ := StopsAt(first.Lane)
		note, pub = first.Name+": "+n, false
		next = first.Name + ": " + NextOf(first.Lane)
	}
	r.StopsAt, r.StopsNote, r.Published, r.Next = stage, note, pub, next
}

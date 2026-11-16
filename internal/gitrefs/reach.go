package gitrefs

import (
	"errors"
	"strings"
	"time"
)

// ReachTimeout is the wall time one Reach may spend on all its walks.
const ReachTimeout = 3 * time.Second

// Reach answers reachability questions about the commits of one git dir for
// the Git page lane: is a tip inside another tip, how many commits a tip has
// that another lacks, what a tag points at, how far a head is past a tag. It
// reads files only, never runs git. Every answer carries a known flag that is
// false on any read problem, on the commit limit (WalkLimit per walk) or on the
// deadline; a caller shows that as "?", never as a yes or a no. One Reach is
// for one goroutine.
type Reach struct {
	st   *store
	sets map[string]map[string]bool
}

// OpenReach opens the objects of gitDir. Close it when done.
func OpenReach(gitDir string) *Reach {
	st := newStore(gitDir)
	st.deadline = time.Now().Add(ReachTimeout)
	return &Reach{st: st, sets: map[string]map[string]bool{}}
}

// Close releases the pack files.
func (r *Reach) Close() { r.st.close() }

var errReachLimit = errors.New("gitrefs: reach limit")

// set is every commit reachable from tip, tip included.
func (r *Reach) set(tip string) (map[string]bool, error) {
	if s, ok := r.sets[tip]; ok {
		return s, nil
	}
	seen := map[string]bool{tip: true}
	queue := []string{tip}
	for len(queue) > 0 {
		if len(seen) > walkLimit || time.Now().After(r.st.deadline) {
			return nil, errReachLimit
		}
		sha := queue[0]
		queue = queue[1:]
		parents, err := r.st.commitParents(sha)
		if err != nil {
			return nil, err
		}
		for _, p := range parents {
			if !seen[p] {
				seen[p] = true
				queue = append(queue, p)
			}
		}
	}
	r.sets[tip] = seen
	return seen, nil
}

// Contains reports whether tip is reachable from base (a commit contains itself).
func (r *Reach) Contains(base, tip string) (in, known bool) {
	if base == tip {
		return true, true
	}
	s, err := r.set(base)
	if err != nil {
		return false, false
	}
	return s[tip], true
}

// NotIn counts the commits reachable from tip that are not reachable from base.
func (r *Reach) NotIn(tip, base string) (n int, known bool) {
	if tip == base {
		return 0, true
	}
	bs, err := r.set(base)
	if err != nil {
		return 0, false
	}
	seen := map[string]bool{}
	queue := []string{}
	if !bs[tip] {
		seen[tip] = true
		queue = append(queue, tip)
	}
	for len(queue) > 0 {
		if len(seen) > walkLimit || time.Now().After(r.st.deadline) {
			return 0, false
		}
		sha := queue[0]
		queue = queue[1:]
		n++
		parents, err := r.st.commitParents(sha)
		if err != nil {
			return 0, false
		}
		for _, p := range parents {
			if !bs[p] && !seen[p] {
				seen[p] = true
				queue = append(queue, p)
			}
		}
	}
	return n, true
}

// Peel follows annotated tag objects to the commit they name. A name that is
// already a commit comes back unchanged.
func (r *Reach) Peel(sha string) (string, bool) {
	for i := 0; i < 5; i++ {
		r.st.alloc = 0
		typ, body, err := r.st.readObject(sha, 0)
		if err != nil {
			return "", false
		}
		if typ == typeCommit {
			return sha, true
		}
		if typ != typeTag {
			return "", false
		}
		next := ""
		for _, line := range strings.Split(string(body), "\n") {
			if line == "" {
				break
			}
			if strings.HasPrefix(line, "object ") {
				next = strings.TrimSpace(strings.TrimPrefix(line, "object "))
				break
			}
		}
		if !isHex40(next) {
			return "", false
		}
		sha = next
	}
	return "", false
}

// FirstParentSteps counts the first-parent steps from head down to target.
// found is false when target is not on head's first-parent line (known stays
// true then, if the whole line was read within the limits).
func (r *Reach) FirstParentSteps(head, target string) (n int, found, known bool) {
	seen := map[string]bool{}
	for sha := head; sha != ""; {
		if sha == target {
			return n, true, true
		}
		if seen[sha] || len(seen) >= walkLimit || time.Now().After(r.st.deadline) {
			return 0, false, false
		}
		seen[sha] = true
		parents, err := r.st.commitParents(sha)
		if err != nil {
			return 0, false, false
		}
		sha = ""
		if len(parents) > 0 {
			sha = parents[0]
		}
		n++
	}
	return 0, false, true
}

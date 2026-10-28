// Package gitrefs reads git's files, never runs git; unknown is never merged.
//
// It answers one question for BurnMon: was a branch merged into the trunk
// (refs/heads/main, else refs/heads/master). Merged
// means the branch tip is not the trunk's tip and is the second or later parent of
// a merge commit reachable from main's tip, because Foreman merges with
// --no-ff. A tip that only sits on main's first-parent history (a
// fast-forward, or a branch with no commit of its own yet) is not merged. It
// resolves refs from loose files and packed-refs, then walks commit parents
// from main's tip through loose objects and pack files, using only the
// standard library. Anything it cannot read is "unknown", and callers leave
// the item where it was.
package gitrefs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Walk limits: at most WalkLimit commits and WalkTimeout of wall time.
// MessageWalkLimit is how many first-parent commits MergedByMessage reads.
const (
	WalkLimit        = 20000
	WalkTimeout      = 2 * time.Second
	MessageWalkLimit = 500
)

// Unexported copies so tests can lower the limits.
var (
	walkLimit   = WalkLimit
	walkTimeout = WalkTimeout
)

// maxCacheEntries bounds the Checker cache; it is cleared when it passes this.
var maxCacheEntries = 1024

// ErrRefNotFound is what ResolveRef returns when the ref is in neither the
// loose files nor packed-refs. Every other error is a read problem.
var ErrRefNotFound = errors.New("gitrefs: ref not found")

// GitDir resolves a repo or worktree folder to its common git dir. It follows
// a .git file ("gitdir: <path>") and the commondir file of a linked worktree.
func GitDir(repo string) (string, error) {
	dot := filepath.Join(repo, ".git")
	fi, err := os.Lstat(dot)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		return dot, nil
	}
	if !fi.Mode().IsRegular() {
		return "", errors.New("gitrefs: .git is not a file or folder")
	}
	data, err := os.ReadFile(dot)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir:") {
		return "", errors.New("gitrefs: .git file has no gitdir line")
	}
	gd := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
	if gd == "" {
		return "", errors.New("gitrefs: empty gitdir")
	}
	if !filepath.IsAbs(gd) {
		gd = filepath.Join(repo, gd)
	}
	gd = filepath.Clean(gd)
	if cd, err := os.ReadFile(filepath.Join(gd, "commondir")); err == nil {
		c := strings.TrimSpace(string(cd))
		if c == "" {
			return "", errors.New("gitrefs: empty commondir")
		}
		if !filepath.IsAbs(c) {
			c = filepath.Join(gd, c)
		}
		gd = filepath.Clean(c)
	}
	if st, err := os.Stat(gd); err != nil || !st.IsDir() {
		return "", errors.New("gitrefs: git dir not found")
	}
	return gd, nil
}

func isHex40(s string) bool {
	if len(s) != 40 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// safeRefName guards file access: the name must stay inside the git dir.
func safeRefName(name string) bool {
	if !strings.HasPrefix(name, "refs/") || strings.Contains(name, "\\") || strings.ContainsAny(name, "\x00\r\n") {
		return false
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.Contains(seg, ":") {
			return false
		}
	}
	return true
}

// ResolveRef resolves a full ref name such as "refs/heads/x" to a 40 hex
// object name: the loose file first, then packed-refs. A symbolic loose ref
// ("ref: refs/heads/y") is followed up to five times.
func ResolveRef(gitDir, name string) (string, error) {
	for depth := 0; depth < 5; depth++ {
		if !safeRefName(name) {
			return "", errors.New("gitrefs: bad ref name")
		}
		data, err := os.ReadFile(filepath.Join(gitDir, filepath.FromSlash(name)))
		if err == nil {
			v := strings.TrimSpace(string(data))
			if strings.HasPrefix(v, "ref:") {
				name = strings.TrimSpace(strings.TrimPrefix(v, "ref:"))
				continue
			}
			if isHex40(v) {
				return v, nil
			}
			return "", errors.New("gitrefs: loose ref is not an object name")
		}
		if !os.IsNotExist(err) {
			// A directory where a file was expected also lands here.
			if _, serr := os.Stat(filepath.Join(gitDir, filepath.FromSlash(name))); serr == nil {
				return "", err
			}
		}
		return resolvePacked(gitDir, name)
	}
	return "", errors.New("gitrefs: symbolic ref loop")
}

func resolvePacked(gitDir, name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(gitDir, "packed-refs"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrRefNotFound
		}
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] == '^' {
			continue
		}
		sha, ref, ok := strings.Cut(line, " ")
		if !ok || strings.TrimSpace(ref) != name {
			continue
		}
		if isHex40(sha) {
			return sha, nil
		}
		return "", errors.New("gitrefs: packed ref is not an object name")
	}
	return "", ErrRefNotFound
}

// ValidBranch reports whether name is safe to use as a branch name in an
// argument list and as a ref path: no leading dash, no space, no "..", no
// control character, none of ~^:?*[\ and the usual git ref rules.
func ValidBranch(name string) bool {
	if name == "" || name == "@" {
		return false
	}
	if name[0] == '-' || name[0] == '/' || strings.HasSuffix(name, "/") || strings.HasSuffix(name, ".") {
		return false
	}
	if strings.HasSuffix(name, ".lock") || strings.Contains(name, "..") || strings.Contains(name, "//") || strings.Contains(name, "@{") {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c < 0x20 || c == 0x7f || c == ' ' || strings.IndexByte("~^:?*[\\", c) >= 0 {
			return false
		}
	}
	for _, seg := range strings.Split(name, "/") {
		if strings.HasPrefix(seg, ".") || strings.HasSuffix(seg, ".lock") {
			return false
		}
	}
	return true
}

type result struct{ merged, known bool }

// Checker answers "is this branch in main" and caches by git dir, main sha
// and branch sha.
type Checker struct {
	mu    sync.Mutex
	cache map[string]result
}

// NewChecker returns an empty Checker.
func NewChecker() *Checker { return &Checker{cache: map[string]result{}} }

// Merged reports whether branch was merged into main with a merge commit:
// its tip is not main's tip and is the second or later parent of a merge
// commit reachable from main's tip (Foreman merges with --no-ff). A tip that
// only sits on main's first-parent history (a fast-forward, or a branch that
// never got a commit of its own) is not merged. known is false on any read
// problem or when the walk limit or timeout is hit; merged is then false.
// The trunk is refs/heads/main when that ref exists (loose or packed), else
// refs/heads/master, else unknown. HEAD is never read: the main tree's HEAD
// may point at a work branch.
func (c *Checker) Merged(repo, branch string) (merged, known bool) {
	if !ValidBranch(branch) {
		return false, false
	}
	gd, err := GitDir(repo)
	if err != nil {
		return false, false
	}
	mainSHA, err := trunkSHA(gd)
	if err != nil {
		return false, false
	}
	brSHA, err := ResolveRef(gd, "refs/heads/"+branch)
	if err != nil {
		return false, false
	}
	key := gd + "|" + mainSHA + "|" + brSHA
	c.mu.Lock()
	if r, ok := c.cache[key]; ok {
		c.mu.Unlock()
		return r.merged, r.known
	}
	c.mu.Unlock()

	merged, known, definite := walk(gd, mainSHA, brSHA)
	if !definite {
		// The limit or the timeout stopped the walk: that says nothing about
		// the branch, so do not pin it until the trunk moves.
		return merged, known
	}

	c.mu.Lock()
	if len(c.cache) >= maxCacheEntries {
		c.cache = map[string]result{}
	}
	c.cache[key] = result{merged, known}
	c.mu.Unlock()
	return merged, known
}

// trunkSHA resolves refs/heads/main, else refs/heads/master.
func trunkSHA(gitDir string) (string, error) {
	sha, err := ResolveRef(gitDir, "refs/heads/main")
	if err == nil {
		return sha, nil
	}
	if sha, merr := ResolveRef(gitDir, "refs/heads/master"); merr == nil {
		return sha, nil
	}
	return "", err
}

// walk searches main's ancestors, breadth first, for a merge commit that has
// target as a second or later parent.
func walk(gitDir, mainSHA, target string) (merged, known, definite bool) {
	if mainSHA == target {
		return false, true, true
	}
	st := newStore(gitDir)
	defer st.close()
	deadline := time.Now().Add(walkTimeout)
	st.deadline = deadline
	seen := map[string]bool{mainSHA: true}
	queue := []string{mainSHA}
	processed := 0
	for len(queue) > 0 {
		if processed >= walkLimit || time.Now().After(deadline) {
			return false, false, false
		}
		sha := queue[0]
		queue = queue[1:]
		processed++
		parents, err := st.commitParents(sha)
		if err != nil {
			return false, false, !time.Now().After(deadline)
		}
		for _, p := range parents[min(1, len(parents)):] {
			if p == target {
				return true, true, true
			}
		}
		for _, p := range parents {
			if !seen[p] {
				seen[p] = true
				queue = append(queue, p)
			}
		}
	}
	return false, true, true
}

// BranchGone reports that branch has no ref in the repo: the trunk resolves,
// the name is safe, and the ref is in neither the loose files nor
// packed-refs. Any other problem (no trunk, an unreadable ref file, a bad
// name) is false: only a clearly missing ref reads as a deleted branch.
func (c *Checker) BranchGone(repo, branch string) bool {
	if !ValidBranch(branch) {
		return false
	}
	gd, err := GitDir(repo)
	if err != nil {
		return false
	}
	if _, err := trunkSHA(gd); err != nil {
		return false
	}
	_, err = ResolveRef(gd, "refs/heads/"+branch)
	return errors.Is(err, ErrRefNotFound)
}

// MergedByMessage is the check for a branch whose ref was deleted after the
// merge: it reads the commit messages of the trunk's last MessageWalkLimit
// first-parent commits and reports found when a merge commit (two or more
// parents) names branch as a whole token. A branch whose ref still exists, a
// bad name, no trunk or any read problem is known=false. Nothing found within
// the limit is (false, true). Same stdlib-only reader, allocation caps and
// deadline as Merged.
func (c *Checker) MergedByMessage(repo, branch string) (found, known bool) {
	if !c.BranchGone(repo, branch) {
		return false, false
	}
	gd, err := GitDir(repo)
	if err != nil {
		return false, false
	}
	mainSHA, err := trunkSHA(gd)
	if err != nil {
		return false, false
	}
	key := "msg|" + gd + "|" + mainSHA + "|" + branch
	c.mu.Lock()
	if r, ok := c.cache[key]; ok {
		c.mu.Unlock()
		return r.merged, r.known
	}
	c.mu.Unlock()

	st := newStore(gd)
	defer st.close()
	deadline := time.Now().Add(walkTimeout)
	st.deadline = deadline
	sha := mainSHA
	for n := 0; n < MessageWalkLimit && sha != ""; n++ {
		if time.Now().After(deadline) {
			return false, false
		}
		parents, msg, err := st.commitInfo(sha)
		if err != nil {
			return false, false
		}
		if len(parents) >= 2 && namesBranch(msg, branch) {
			found = true
			break
		}
		sha = ""
		if len(parents) > 0 {
			sha = parents[0]
		}
	}
	c.mu.Lock()
	if len(c.cache) >= maxCacheEntries {
		c.cache = map[string]result{}
	}
	c.cache[key] = result{found, true}
	c.mu.Unlock()
	return found, true
}

// namesBranch reports whether msg holds branch as a whole token: the name is
// preceded by the start, whitespace, a quote, a slash (an owner prefix) or
// other punctuation, and followed by the end, whitespace, a quote or other
// punctuation. Letters, digits, ".", "_", "-" and "/" glue a name to a longer
// word or branch ("sprint/a" is not in "sprint/ab" or "sprint/a/b").
func namesBranch(msg, branch string) bool {
	glue := func(c byte) bool {
		return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '.' || c == '_' || c == '-'
	}
	for from := 0; from < len(msg); {
		i := strings.Index(msg[from:], branch)
		if i < 0 {
			return false
		}
		i += from
		end := i + len(branch)
		from = i + 1
		if i > 0 && glue(msg[i-1]) {
			continue
		}
		if end < len(msg) {
			c := msg[end]
			if glue(c) && !(c == '.' && (end+1 == len(msg) || msg[end+1] == ' ' || msg[end+1] == '\n' || msg[end+1] == '\r')) || c == '/' {
				continue
			}
		}
		return true
	}
	return false
}

// MergedOrGone is Merged, and for a branch whose ref is gone it is the
// message search instead. The Close control uses it, so a sprint found
// merged by message can be closed like any merged one.
func (c *Checker) MergedOrGone(repo, branch string) (merged, known bool) {
	if c.BranchGone(repo, branch) {
		return c.MergedByMessage(repo, branch)
	}
	return c.Merged(repo, branch)
}

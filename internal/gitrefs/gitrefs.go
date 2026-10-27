// Package gitrefs reads git's files, never runs git; unknown is never merged.
//
// It answers one question for BurnMon: was a branch merged into main. Merged
// means the branch tip is not main's tip and is the second or later parent of
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
const (
	WalkLimit   = 20000
	WalkTimeout = 2 * time.Second
)

// Unexported copies so tests can lower the limits.
var (
	walkLimit   = WalkLimit
	walkTimeout = WalkTimeout
)

// maxCacheEntries bounds the Checker cache; it is cleared when it passes this.
var maxCacheEntries = 1024

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
	return "", errors.New("gitrefs: ref not found")
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
// Only refs/heads/main is the trunk.
func (c *Checker) Merged(repo, branch string) (merged, known bool) {
	if !ValidBranch(branch) {
		return false, false
	}
	gd, err := GitDir(repo)
	if err != nil {
		return false, false
	}
	mainSHA, err := ResolveRef(gd, "refs/heads/main")
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

	merged, known = walk(gd, mainSHA, brSHA)

	c.mu.Lock()
	if len(c.cache) >= maxCacheEntries {
		c.cache = map[string]result{}
	}
	c.cache[key] = result{merged, known}
	c.mu.Unlock()
	return merged, known
}

// walk searches main's ancestors, breadth first, for a merge commit that has
// target as a second or later parent.
func walk(gitDir, mainSHA, target string) (merged, known bool) {
	if mainSHA == target {
		return false, true
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
			return false, false
		}
		sha := queue[0]
		queue = queue[1:]
		processed++
		parents, err := st.commitParents(sha)
		if err != nil {
			return false, false
		}
		for _, p := range parents[min(1, len(parents)):] {
			if p == target {
				return true, true
			}
		}
		for _, p := range parents {
			if !seen[p] {
				seen[p] = true
				queue = append(queue, p)
			}
		}
	}
	return false, true
}

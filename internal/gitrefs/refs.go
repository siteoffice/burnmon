package gitrefs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Commit is what the Control Centre Git page shows of a commit.
type Commit struct {
	When    time.Time // committer time
	Subject string    // first line of the message
}

// CommitInfo reads the commit object sha from gitDir (loose or packed) and
// returns its committer time and subject. It never runs git.
func CommitInfo(gitDir, sha string) (Commit, error) {
	if !isHex40(sha) {
		return Commit{}, errors.New("gitrefs: bad object name")
	}
	s := newStore(gitDir)
	defer s.close()
	typ, body, err := s.readObject(sha, 0)
	if err != nil {
		return Commit{}, err
	}
	if typ != typeCommit {
		return Commit{}, errors.New("gitrefs: object is not a commit")
	}
	head, msg, _ := strings.Cut(string(body), "\n\n")
	var c Commit
	for _, line := range strings.Split(head, "\n") {
		if !strings.HasPrefix(line, "committer ") {
			continue
		}
		// "committer Name <mail> 1763200000 +0100": the second field from the end.
		f := strings.Fields(line)
		if len(f) >= 3 {
			if sec, err := strconv.ParseInt(f[len(f)-2], 10, 64); err == nil {
				c.When = time.Unix(sec, 0)
			}
		}
	}
	c.Subject, _, _ = strings.Cut(strings.TrimLeft(msg, "\n"), "\n")
	c.Subject = strings.TrimSpace(c.Subject)
	return c, nil
}

// ListRefs returns every ref below prefix ("refs/heads/" or "refs/tags/"),
// name without the prefix to object name, from loose files and packed-refs.
// A loose file wins over a packed line. Peeled lines ("^sha") are skipped.
func ListRefs(gitDir, prefix string) (map[string]string, error) {
	if !strings.HasPrefix(prefix, "refs/") || !strings.HasSuffix(prefix, "/") || !safeRefName(strings.TrimSuffix(prefix, "/")) {
		return nil, errors.New("gitrefs: bad ref prefix")
	}
	out := map[string]string{}
	if data, err := os.ReadFile(filepath.Join(gitDir, "packed-refs")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || line[0] == '#' || line[0] == '^' {
				continue
			}
			sha, ref, ok := strings.Cut(line, " ")
			ref = strings.TrimSpace(ref)
			if ok && isHex40(sha) && strings.HasPrefix(ref, prefix) {
				out[strings.TrimPrefix(ref, prefix)] = sha
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	base := filepath.Join(gitDir, filepath.FromSlash(prefix))
	_ = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(base, p)
		if rerr != nil {
			return nil
		}
		name := filepath.ToSlash(rel)
		if sha, err := ResolveRef(gitDir, prefix+name); err == nil {
			out[name] = sha
		}
		return nil
	})
	return out, nil
}

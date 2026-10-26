package controls

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Self is what BurnMon Dev knows about its own place in the process tree.
// No control may aim at it.
//
// ChainComplete and ChainVerified are set by CurrentSelf. The ancestor list is
// trustworthy only when both are true. CurrentSelf returns a non-nil error
// otherwise, and the Task 6 service must treat any CurrentSelf error as
// "controls off" and never use the Self that came with it.
type Self struct {
	PID           int
	Ancestors     []int
	ExeDir        string
	ChainComplete bool // the walk reached a root (pid 0 or a login/system process)
	ChainVerified bool // every parent link was checked by creation time
}

const (
	errNoPID     = "the session has no process id"
	errCwdUnsure = "the session's working folder cannot be checked"
)

// Refuses returns nil, or the reason a session with this pid and working
// folder must not get a control. It fails closed: a pid below 1, and a
// working folder that is empty, not absolute or cannot be proven different
// from BurnMon Dev's own folder, are all refused.
func (s Self) Refuses(pid int, cwd string) error {
	if pid <= 0 {
		return errors.New(errNoPID)
	}
	if pid == s.PID {
		return errors.New("this is BurnMon Dev itself")
	}
	for _, a := range s.Ancestors {
		if a == pid {
			return errors.New("this session started BurnMon Dev")
		}
	}
	dir, ok := canonical(cwd)
	if !ok {
		return errors.New(errCwdUnsure)
	}
	if s.ExeDir == "" {
		return nil
	}
	root, ok := canonical(s.ExeDir)
	if !ok {
		return errors.New(errCwdUnsure)
	}
	dir, root = adminShareToDrive(dir), adminShareToDrive(root)
	switch {
	case dir.unc != root.unc && dir.unc:
		// A UNC spelling may name the same folder through a share we
		// cannot map. It cannot be proven different, so refuse.
		return errors.New(errCwdUnsure)
	case dir.unc != root.unc && localHost(root.parts[0]):
		// The exe folder is on a loopback share that is not an admin-share
		// mapping, so it may be the same folder as a drive path. Refuse.
		return errors.New(errCwdUnsure)
	case dir.unc != root.unc:
		return nil
	}
	if within(dir, root) {
		return errors.New("this session works in BurnMon Dev's own folder")
	}
	return nil
}

// cpath is a Windows path split into components. For a drive path parts[0]
// is "C:"; for a UNC path parts[0] is the server and parts[1] the share.
type cpath struct {
	unc   bool
	parts []string
}

// within reports whether d equals r or lies under it, comparing component
// by component with Unicode simple case folding (as Windows does for NTFS,
// closely enough; no ASCII-only fold). A component boundary is required, so
// "burnmon-other" is not inside "burnmon".
func within(d, r cpath) bool {
	if len(d.parts) < len(r.parts) || len(r.parts) == 0 {
		return false
	}
	for i, c := range r.parts {
		if !strings.EqualFold(d.parts[i], c) {
			return false
		}
	}
	return true
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// trimName drops the trailing dots and spaces Windows ignores.
func trimName(c string) string { return strings.TrimRight(c, ". ") }

const badNameChars = `:*?"<>|` + "\x00"

// lexical parses an absolute Windows path without touching the disk. It
// strips the \\?\ and \\.\ prefixes (\\?\UNC\ becomes a UNC path), cleans
// "." and "..", and drops trailing dots and spaces from every component.
// Relative paths, rooted-without-drive paths, drive-relative paths and names
// with illegal characters (including alternate data stream colons) fail.
func lexical(p string) (cpath, bool) {
	p = strings.ReplaceAll(p, "/", `\`)
	switch {
	case hasPrefixFold(p, `\\?\UNC\`), hasPrefixFold(p, `\\.\UNC\`):
		p = `\\` + p[8:]
	case hasPrefixFold(p, `\\?\`), hasPrefixFold(p, `\\.\`), hasPrefixFold(p, `\??\`):
		p = p[4:]
	}
	var out cpath
	var rest []string
	switch {
	case strings.HasPrefix(p, `\\`):
		comps := strings.Split(p[2:], `\`)
		if len(comps) < 2 {
			return cpath{}, false
		}
		srv, share := trimName(comps[0]), trimName(comps[1])
		if srv == "" || share == "" || srv == "?" || srv == "." {
			return cpath{}, false
		}
		out = cpath{unc: true, parts: []string{srv, share}}
		rest = comps[2:]
	case len(p) >= 3 && p[1] == ':' && p[2] == '\\' &&
		(p[0] >= 'a' && p[0] <= 'z' || p[0] >= 'A' && p[0] <= 'Z'):
		out = cpath{parts: []string{p[:2]}}
		rest = strings.Split(p[3:], `\`)
	default:
		return cpath{}, false
	}
	floor := len(out.parts)
	if out.unc && strings.ContainsAny(out.parts[0]+out.parts[1], badNameChars) {
		return cpath{}, false
	}
	for _, c := range rest {
		switch c {
		case "", ".":
			continue
		case "..":
			if len(out.parts) > floor {
				out.parts = out.parts[:len(out.parts)-1]
			}
			continue
		}
		if strings.ContainsAny(c, badNameChars) {
			return cpath{}, false
		}
		if c = trimName(c); c != "" {
			out.parts = append(out.parts, c)
		}
	}
	return out, true
}

func (c cpath) String() string {
	if c.unc {
		return `\\` + strings.Join(c.parts, `\`)
	}
	if len(c.parts) == 1 {
		return c.parts[0] + `\`
	}
	return strings.Join(c.parts, `\`)
}

// canonical returns the lexical form, then replaces the part of the path
// that exists on disk with its resolved form (symlinks, junctions, subst and
// mapped drives, 8.3 short names expanded). The part that does not exist
// stays lexical, with the lexical checks already applied. It fails closed:
// when no prefix exists at all it returns not-ok, since only a path whose
// existing part was resolved may be judged lexically for the rest;
// when the longest existing prefix cannot be resolved (a dangling junction,
// no access, an unexpected error) it returns not-ok and never the lexical
// path, because the lexical path may hide a link into BurnMon Dev's folder.
func canonical(p string) (cpath, bool) {
	lex, ok := lexical(p)
	if !ok {
		return cpath{}, false
	}
	// Longest existing prefix.
	for n := len(lex.parts); n >= 1; n-- {
		pre := cpath{unc: lex.unc, parts: lex.parts[:n]}
		if lex.unc && n < 2 {
			break
		}
		prefix := pre.String()
		if _, err := os.Lstat(prefix); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return cpath{}, false
		}
		real, ok := resolveReal(prefix)
		if !ok {
			return cpath{}, false
		}
		rl, ok := lexical(real)
		if !ok {
			return cpath{}, false
		}
		rl.parts = append(rl.parts, lex.parts[n:]...)
		return rl, true
	}
	// No prefix exists in this process's namespace (a drive letter from subst
	// or a mapped drive in another logon session or token, or an unknown UNC
	// host). Nothing was resolved, so the path cannot be judged.
	return cpath{}, false
}

// adminShareToDrive rewrites \\localhost\C$\x as C:\x. Any other UNC path is
// returned unchanged.
func adminShareToDrive(c cpath) cpath {
	if !c.unc || len(c.parts) < 2 || len(c.parts[1]) != 2 || c.parts[1][1] != '$' {
		return c
	}
	l := c.parts[1][0]
	if !(l >= 'a' && l <= 'z' || l >= 'A' && l <= 'Z') || !localHost(c.parts[0]) {
		return c
	}
	return cpath{parts: append([]string{c.parts[1][:1] + ":"}, c.parts[2:]...)}
}

func localHost(h string) bool {
	if h == "." || strings.EqualFold(h, "localhost") || h == "127.0.0.1" || h == "::1" {
		return true
	}
	if n, err := os.Hostname(); err == nil && strings.EqualFold(h, n) {
		return true
	}
	return false
}

// exeDir is the folder of the running executable, links and junctions
// resolved. An executable path that cannot be resolved is an error, so the
// caller turns controls off.
func exeDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	r, ok := resolveReal(exe)
	if !ok {
		return "", errors.New("cannot resolve the executable path")
	}
	return filepath.Dir(r), nil
}

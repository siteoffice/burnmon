package controls

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Self is what BurnMon Dev knows about its own place in the process tree.
// No control may aim at it.
type Self struct {
	PID       int
	Ancestors []int
	ExeDir    string
}

// Refuses returns nil, or the reason a session with this pid and working
// folder must not get a control.
func (s Self) Refuses(pid int, cwd string) error {
	if pid > 0 && pid == s.PID {
		return errors.New("this is BurnMon Dev itself")
	}
	if pid > 0 {
		for _, a := range s.Ancestors {
			if a == pid {
				return errors.New("this session started BurnMon Dev")
			}
		}
	}
	if inside(cwd, s.ExeDir) {
		return errors.New("this session works in BurnMon Dev's own folder")
	}
	return nil
}

// normPath cleans a Windows or slash path and folds ASCII case only.
func normPath(p string) string {
	if strings.TrimSpace(p) == "" {
		return ""
	}
	p = path.Clean(strings.ReplaceAll(p, `\`, "/"))
	b := []byte(p)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// inside reports whether dir equals root or lies under it. Empty never matches.
func inside(dir, root string) bool {
	d, r := normPath(dir), normPath(root)
	if d == "" || r == "" {
		return false
	}
	return d == r || strings.HasPrefix(d, strings.TrimSuffix(r, "/")+"/")
}

// exeDir is the folder of the running executable, links resolved.
func exeDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return filepath.Dir(exe), nil
}

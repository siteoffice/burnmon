//go:build windows

package controls

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

const ownFolder = "this session works in BurnMon Dev's own folder"

// realTree makes <temp>\Burnmon Ünï Long Name\sub and returns the long,
// resolved folder path.
func realTree(t *testing.T) string {
	t.Helper()
	root, ok := resolveReal(t.TempDir())
	if !ok {
		t.Fatal("cannot resolve temp dir")
	}
	dir := filepath.Join(root, "Burnmon Ünï Long Name")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func mustRefuse(t *testing.T, s Self, cwd string) {
	t.Helper()
	if err := s.Refuses(40, cwd); err == nil || err.Error() != ownFolder {
		t.Errorf("Refuses(40, %q) = %v, want own-folder refusal", cwd, err)
	}
}

func TestRefusesRealFolderSpellings(t *testing.T) {
	dir := realTree(t)
	s := Self{PID: 10, ExeDir: dir}
	sub := filepath.Join(dir, "sub")
	mustRefuse(t, s, sub)
	mustRefuse(t, s, `\\?\`+sub)
	mustRefuse(t, s, `\\.\`+sub)
	mustRefuse(t, s, strings.ToUpper(sub))
	mustRefuse(t, s, strings.ToLower(sub))
	mustRefuse(t, s, dir+".")
	mustRefuse(t, s, dir+` .\sub`)
	mustRefuse(t, s, filepath.Join(sub, "not-there-yet"))
	if vol := filepath.VolumeName(dir); len(vol) == 2 {
		mustRefuse(t, s, `\\localhost\`+vol[:1]+`$`+dir[2:])
		mustRefuse(t, s, `\\?\UNC\localhost\`+vol[:1]+`$`+dir[2:])
	}
	if err := s.Refuses(40, dir+"-other"); err != nil {
		t.Errorf("sibling refused: %v", err)
	}
}

func TestRefusesShortName(t *testing.T) {
	dir := realTree(t)
	long, _ := windows.UTF16PtrFromString(dir)
	buf := make([]uint16, 1024)
	n, err := windows.GetShortPathName(long, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 {
		t.Skipf("GetShortPathName failed: %v", err)
	}
	short := windows.UTF16ToString(buf[:n])
	if strings.EqualFold(short, dir) {
		t.Skip("8.3 short names are disabled on this volume")
	}
	s := Self{PID: 10, ExeDir: dir}
	mustRefuse(t, s, filepath.Join(short, "sub"))
	mustRefuse(t, s, filepath.Join(short, "sub", "not-there-yet"))
	// And the other way round: the exe folder spelled short.
	mustRefuse(t, Self{PID: 10, ExeDir: short}, filepath.Join(dir, "sub"))
}

func TestRefusesLinkedFolder(t *testing.T) {
	dir := realTree(t)
	link := filepath.Join(filepath.Dir(dir), "link")
	if err := os.Symlink(dir, link); err != nil {
		out, err2 := exec.Command("cmd", "/c", "mklink", "/J", link, dir).CombinedOutput()
		if err2 != nil {
			t.Skipf("cannot create a symlink (%v) or a junction (%v: %s)", err, err2, out)
		}
	}
	s := Self{PID: 10, ExeDir: dir}
	mustRefuse(t, s, link)
	mustRefuse(t, s, filepath.Join(link, "sub"))
	mustRefuse(t, s, filepath.Join(link, "sub", "not-there-yet"))
	// Exe folder reached through the link, session in the real folder.
	mustRefuse(t, Self{PID: 10, ExeDir: link}, filepath.Join(dir, "sub"))
}

// --- injectable CurrentSelf lookups -------------------------------------

var chainT0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func ct(m map[uint32]int) func(uint32) (time.Time, bool) {
	return func(p uint32) (time.Time, bool) {
		s, ok := m[p]
		return chainT0.Add(time.Duration(s) * time.Second), ok
	}
}

func TestWalkChain(t *testing.T) {
	cases := []struct {
		name               string
		procs              map[uint32]procInfo
		created            map[uint32]int
		want               []int
		complete, verified bool
	}{
		{"complete to explorer", map[uint32]procInfo{100: {90, "burnmon-dev.exe"}, 90: {80, "cmd.exe"}, 80: {70, "explorer.exe"}},
			map[uint32]int{100: 30, 90: 20, 80: 10}, []int{90, 80}, true, true},
		{"explorer parent already gone is a normal root", map[uint32]procInfo{100: {80, "x.exe"}, 80: {70, "Explorer.EXE"}},
			map[uint32]int{100: 30, 80: 10}, []int{80}, true, true},
		{"parent pid zero", map[uint32]procInfo{100: {0, "x.exe"}},
			map[uint32]int{100: 30}, nil, true, true},
		{"parent exited", map[uint32]procInfo{100: {90, "x.exe"}},
			map[uint32]int{100: 30}, []int{90}, false, false},
		{"grandparent exited", map[uint32]procInfo{100: {90, "x.exe"}, 90: {80, "cmd.exe"}},
			map[uint32]int{100: 30, 90: 20}, []int{90, 80}, false, false},
		{"reused parent pid keeps walking, unverified", map[uint32]procInfo{100: {90, "x.exe"}, 90: {80, "other.exe"}, 80: {0, "explorer.exe"}},
			map[uint32]int{100: 30, 90: 40, 80: 5}, []int{90, 80}, true, false},
		{"parent start time unreadable", map[uint32]procInfo{100: {90, "x.exe"}, 90: {0, "svc.exe"}},
			map[uint32]int{100: 30}, []int{90}, true, false},
		{"loop", map[uint32]procInfo{100: {90, "x.exe"}, 90: {100, "y.exe"}},
			map[uint32]int{100: 30, 90: 40}, []int{90}, false, false},
	}
	for _, c := range cases {
		got, complete, verified := walkChain(100, c.procs, ct(c.created))
		if complete != c.complete || verified != c.verified || len(got) != len(c.want) {
			t.Errorf("%s: got %v complete=%v verified=%v", c.name, got, complete, verified)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: got %v, want %v", c.name, got, c.want)
			}
		}
	}
}

func TestWalkChainStopsAt64Steps(t *testing.T) {
	procs := map[uint32]procInfo{}
	created := map[uint32]int{}
	for i := uint32(1); i <= 200; i++ {
		procs[i] = procInfo{Parent: i + 1, Exe: "x.exe"}
		created[i] = int(1000 - i)
	}
	procs[201] = procInfo{0, "x.exe"}
	created[201] = 1
	anc, complete, _ := walkChain(1, procs, ct(created))
	if complete || len(anc) != 64 {
		t.Fatalf("len=%d complete=%v", len(anc), complete)
	}
}

func goodEnv() selfEnv {
	return selfEnv{
		pid:    100,
		exeDir: func() (string, error) { return `C:\example\burnmon`, nil },
		procs: func() (map[uint32]procInfo, error) {
			return map[uint32]procInfo{100: {90, "b.exe"}, 90: {0, "explorer.exe"}}, nil
		},
		created: ct(map[uint32]int{100: 20, 90: 10}),
	}
}

func TestCurrentSelfInjected(t *testing.T) {
	s, err := currentSelf(goodEnv())
	if err != nil || s.PID != 100 || len(s.Ancestors) != 1 || s.Ancestors[0] != 90 || !s.ChainComplete || !s.ChainVerified || s.ExeDir == "" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestCurrentSelfFailsClosed(t *testing.T) {
	boom := errors.New("boom")
	mods := map[string]func(*selfEnv){
		"exe dir":        func(e *selfEnv) { e.exeDir = func() (string, error) { return "", boom } },
		"process table":  func(e *selfEnv) { e.procs = func() (map[uint32]procInfo, error) { return nil, boom } },
		"own start time": func(e *selfEnv) { e.created = ct(map[uint32]int{90: 10}) },
		"partial table": func(e *selfEnv) {
			e.procs = func() (map[uint32]procInfo, error) { return map[uint32]procInfo{100: {90, "b.exe"}}, boom }
		},
		"own entry missing": func(e *selfEnv) {
			e.procs = func() (map[uint32]procInfo, error) { return map[uint32]procInfo{}, nil }
		},
		"parent exited": func(e *selfEnv) {
			e.procs = func() (map[uint32]procInfo, error) { return map[uint32]procInfo{100: {90, "b.exe"}}, nil }
		},
	}
	for name, mod := range mods {
		env := goodEnv()
		mod(&env)
		s, err := currentSelf(env)
		if err == nil {
			t.Errorf("%s: want an error, got %+v", name, s)
			continue
		}
		if name != "parent exited" && (s.PID != 0 || len(s.Ancestors) != 0 || s.ChainComplete) {
			t.Errorf("%s: Self must be unusable, got %+v", name, s)
		}
		if name == "parent exited" && (s.ChainComplete || s.ChainVerified || !strings.Contains(err.Error(), "ancestor chain")) {
			t.Errorf("%s: %+v %v", name, s, err)
		}
	}
}

package controls

import "testing"

const containsFolder = "this session works in a folder that contains BurnMon Dev"

func TestRefuses(t *testing.T) {
	s := Self{PID: 10, Ancestors: []int{20, 30}, ExeDir: `C:\example\burnmon`}
	cases := []struct {
		pid  int
		cwd  string
		want string
	}{
		{10, "", "this is BurnMon Dev itself"},
		{30, "", "this session started BurnMon Dev"},
		{40, `c:\EXAMPLE\burnmon\.worktrees\x`, "this session works in BurnMon Dev's own folder"},
		{40, `C:\example\burnmon`, "this session works in BurnMon Dev's own folder"},
		{40, `C:\example\burnmon\`, "this session works in BurnMon Dev's own folder"},
		{40, `C:\example\burnmon\..\burnmon\x`, "this session works in BurnMon Dev's own folder"},
		{40, `C:\example`, containsFolder},
		{40, `C:\example\`, containsFolder},
		{40, `c:\EXAMPLE`, containsFolder},
		{40, `C:\example.`, containsFolder},
		{40, `C:\example\burnmon\..`, containsFolder},
		{40, `C:\`, containsFolder},
	}
	for _, c := range cases {
		err := s.Refuses(c.pid, c.cwd)
		if err == nil || err.Error() != c.want {
			t.Errorf("Refuses(%d, %q) = %v, want %q", c.pid, c.cwd, err, c.want)
		}
	}
	for _, cwd := range []string{`C:\example\burnmon-other`, `C:\example\burnmon\..\other`, `C:\example2\burnmon`, `C:\examples`, `C:\elsewhere`} {
		if err := s.Refuses(40, cwd); err != nil {
			t.Errorf("Refuses(40, %q) = %v, want nil", cwd, err)
		}
	}
}

func TestRefusesEmptyExeDirNeverMatchesEverything(t *testing.T) {
	s := Self{PID: 10}
	if err := s.Refuses(40, `C:\anything`); err != nil {
		t.Fatalf("got %v", err)
	}
}

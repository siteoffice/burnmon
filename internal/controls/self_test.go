package controls

import "testing"

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
	}
	for _, c := range cases {
		err := s.Refuses(c.pid, c.cwd)
		if err == nil || err.Error() != c.want {
			t.Errorf("Refuses(%d, %q) = %v, want %q", c.pid, c.cwd, err, c.want)
		}
	}
	for _, cwd := range []string{`C:\example\burnmon-other`, `C:\example`, `C:\example\burnmon\..\other`, ""} {
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

func TestRefusesPidZeroNotSelf(t *testing.T) {
	if err := (Self{}).Refuses(0, ""); err != nil {
		t.Fatalf("pid 0 with empty Self must not match: %v", err)
	}
}

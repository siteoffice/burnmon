package controls

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestRefusesFailsClosedWithoutPid(t *testing.T) {
	for _, pid := range []int{0, -1} {
		err := (Self{}).Refuses(pid, `C:\elsewhere`)
		if err == nil || err.Error() != "the session has no process id" {
			t.Errorf("pid %d: got %v", pid, err)
		}
	}
}

func TestRefusesCwdForms(t *testing.T) {
	own := "this session works in BurnMon Dev's own folder"
	unsure := "the session's working folder cannot be checked"
	s := Self{PID: 10, ExeDir: `C:\example\burnmon`}
	cases := []struct{ cwd, want string }{
		{`\\?\C:\example\burnmon\x`, own},
		{`\\.\C:\example\burnmon`, own},
		{`\\?\c:\EXAMPLE\burnmon`, own},
		{`\??\C:\example\burnmon`, own},
		{`\\?\UNC\localhost\C$\example\burnmon\x`, own},
		{`\\localhost\c$\example\burnmon`, own},
		{`C:/example/burnmon/x`, own},
		{`C:\example\burnmon.\x`, own},
		{`C:\example\burnmon \x`, own},
		{`C:\example\burnmon...`, own},
		{`C:\example\burnmon. . \x`, own},
		{`C:\example\.\burnmon\.\x`, own},
		{`C:\example\other\..\burnmon`, own},
		{`\\server\share\x`, unsure},
		{`\\?\UNC\server\share\x`, unsure},
		{``, unsure},
		{`   `, unsure},
		{`example\burnmon`, unsure},
		{`.`, unsure},
		{`..\burnmon`, unsure},
		{`\example\burnmon`, unsure},
		{`C:example\burnmon`, unsure},
		{`C:\example\burnmon:stream`, unsure},
		{`\\?\GLOBALROOT\x`, unsure},
	}
	for _, c := range cases {
		err := s.Refuses(40, c.cwd)
		if err == nil || err.Error() != c.want {
			t.Errorf("Refuses(40, %q) = %v, want %q", c.cwd, err, c.want)
		}
	}
	for _, cwd := range []string{`C:\example\burnmon-other`, `C:\example\burnmon...x`, `C:\example2\burnmon`, `C:\`} {
		if err := s.Refuses(40, cwd); err != nil {
			t.Errorf("Refuses(40, %q) = %v, want nil", cwd, err)
		}
	}
}

func TestRefusesNonASCIIFolds(t *testing.T) {
	s := Self{PID: 10, ExeDir: `C:\Ünï\Burnmon`}
	if err := s.Refuses(40, `c:\üNï\BURNMON\x`); err == nil {
		t.Error("non-ASCII case difference must still match")
	}
	if err := s.Refuses(40, `C:\Ünï\Burnmon-ü`); err != nil {
		t.Errorf("sibling: %v", err)
	}
}

func TestRefusesUnusableExeDirFailsClosed(t *testing.T) {
	if err := (Self{PID: 10, ExeDir: `burnmon`}).Refuses(40, `C:\x`); err == nil {
		t.Error("a relative ExeDir cannot be compared, must refuse")
	}
}

// freeDrive returns a drive letter whose root does not exist for this process.
func freeDrive(t *testing.T) string {
	t.Helper()
	for l := 'Z'; l >= 'D'; l-- {
		root := string(l) + `:\`
		if _, err := os.Stat(root); errors.Is(err, fs.ErrNotExist) {
			return string(l)
		}
	}
	t.Skip("no unused drive letter found (Z down to D all exist)")
	return ""
}

func TestRefusesUnseenDriveAndHostAsUnsure(t *testing.T) {
	unsure := "the session's working folder cannot be checked"
	s := Self{PID: 10, ExeDir: `C:\example\burnmon`}
	d := freeDrive(t)
	for _, cwd := range []string{d + `:\x`, d + `:\`, `\burnmon-no-such-host-7f3a\share\x`} {
		if err := s.Refuses(40, cwd); err == nil || err.Error() != unsure {
			t.Errorf("Refuses(40, %q) = %v, want %q", cwd, err, unsure)
		}
	}
}

func TestRefusesLexicalTailUnderExistingPrefix(t *testing.T) {
	base := t.TempDir()
	exe := filepath.Join(base, "burnmon")
	s := Self{PID: 10, ExeDir: exe}
	if err := s.Refuses(40, filepath.Join(base, "other", "not", "there")); err != nil {
		t.Errorf("unrelated non-existing tail: %v", err)
	}
	err := s.Refuses(40, filepath.Join(base, "burnmon", "not", "there"))
	if err == nil || err.Error() != "this session works in BurnMon Dev's own folder" {
		t.Errorf("tail under exe folder: %v", err)
	}
}

func TestRefusesLoopbackUNCExeFolderAsUnsure(t *testing.T) {
	s := Self{PID: 10, ExeDir: `\localhost\someshare\x`}
	if err := s.Refuses(40, `C:\example\other`); err == nil || err.Error() != "the session's working folder cannot be checked" {
		t.Errorf("got %v", err)
	}
}

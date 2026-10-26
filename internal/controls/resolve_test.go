package controls

import (
	"errors"
	"testing"
)

func lookOf(p string, err error) func(string) (string, error) {
	return func(string) (string, error) { return p, err }
}

func TestResolvePython(t *testing.T) {
	got, err := ResolvePython(lookOf(`C:\Python311\python.exe`, nil), "windows")
	if err != nil || got != `C:\Python311\python.exe` {
		t.Fatalf("python.exe: %q %v", got, err)
	}
	for _, tc := range []struct{ path, want string }{
		{`C:\Users\u\AppData\Local\Microsoft\WindowsApps\python.exe`, "python is the Store alias"},
		{`C:\Users\u\AppData\Local\Microsoft\WINDOWSAPPS\python.exe`, "python is the Store alias"},
		{`C:\x\python.bat`, "python is not a .exe"},
	} {
		if _, err := ResolvePython(lookOf(tc.path, nil), "windows"); err == nil || err.Error() != tc.want {
			t.Errorf("%s: err %v, want %q", tc.path, err, tc.want)
		}
	}
	if _, err := ResolvePython(lookOf("", errors.New("nope")), "windows"); err == nil || err.Error() != "python not found" {
		t.Errorf("look error: %v", err)
	}
}

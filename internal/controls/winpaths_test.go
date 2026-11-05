package controls

import (
	"runtime"
	"testing"
)

// requireWindowsPaths skips a test whose fixtures are Windows paths (drive
// letters, backslashes): the cwd and exe-folder checks only parse them on Windows.
func requireWindowsPaths(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("fixtures are Windows paths; the folder checks parse them only on Windows")
	}
}

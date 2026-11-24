//go:build !windows

package controls

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// runner_other_runs: runs only on a Mac or Linux runner. On Windows the cross
// build proves it compiles.
func TestRunnerOtherRuns(t *testing.T) {
	dir := t.TempDir()
	ok := filepath.Join(dir, "ok.sh")
	if err := os.WriteFile(ok, []byte("#!/bin/sh\necho hello\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, line, err := ExecRunner{}.Run(context.Background(), []string{ok}, dir)
	if err != nil || code != 3 || line != "hello" {
		t.Errorf("Run = %d %q %v", code, line, err)
	}
	slow := filepath.Join(dir, "slow.sh")
	if err := os.WriteFile(slow, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if code, _, err := (ExecRunner{}).Run(ctx, []string{slow}, dir); err == nil || code != -1 {
		t.Errorf("timeout = %d, %v", code, err)
	}
	if err := (ExecRunner{}).Start([]string{ok}); err == nil || err.Error() != "attach runs on Windows only" {
		t.Errorf("Start = %v", err)
	}
}

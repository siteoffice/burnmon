//go:build !windows

package snapshotwriter

import "os/exec"

// hideWindow is a no-op outside Windows.
func hideWindow(cmd *exec.Cmd) {}

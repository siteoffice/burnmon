//go:build !windows

package controls

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}

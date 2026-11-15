//go:build windows

package centre

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps a console window from appearing for git or gh: the
// Control Centre is built -H windowsgui.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

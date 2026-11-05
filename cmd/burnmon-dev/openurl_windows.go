//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// startBrowser opens rawURL in the default browser through `cmd /c start`.
// HideWindow: burnmon-dev.exe is built -H windowsgui, so without it `cmd`
// can still flash a console window.
func startBrowser(rawURL string) error {
	cmd := exec.Command("cmd", "/c", "start", "", rawURL)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

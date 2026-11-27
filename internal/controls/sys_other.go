//go:build !windows

package controls

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

// CurrentSelf has no ancestor walk off Windows, so ChainComplete stays false.
// The service keeps the agent controls (Stop, Attach) off here for that reason;
// the gate and close controls do not use the process tree.
func CurrentSelf() (Self, error) {
	d, err := exeDir()
	return Self{PID: os.Getpid(), ExeDir: d}, err
}

// consoleAttr has nothing to hide off Windows.
func consoleAttr(*exec.Cmd) {}

// Start refuses off Windows: attach needs a console window.
func (ExecRunner) Start(argv []string) error { return errors.New("attach runs on Windows only") }

// Elevated is true for root, so controls stay off for a root process as they
// do for an elevated one on Windows.
func Elevated() bool { return rootElevated(os.Geteuid()) }


// resolveReal resolves links in an existing path.
func resolveReal(p string) (string, bool) {
	r, err := filepath.EvalSymlinks(p)
	return r, err == nil
}

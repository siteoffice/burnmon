//go:build !windows

package controls

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

var errUnsupported = errors.New("controls run on Windows only")

// CurrentSelf has no ancestor walk off Windows, so ChainComplete stays false
// and the service must not run controls here anyway (ExecRunner is a stub).
func CurrentSelf() (Self, error) {
	d, err := exeDir()
	return Self{PID: os.Getpid(), ExeDir: d}, err
}

// ExecRunner is a stub off Windows.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, argv []string, dir string) (int, string, error) {
	return -1, "", errUnsupported
}

func (ExecRunner) Start(argv []string) error { return errUnsupported }

// Elevated is always false off Windows.
func Elevated() bool { return false }

// resolveReal resolves links in an existing path.
func resolveReal(p string) (string, bool) {
	r, err := filepath.EvalSymlinks(p)
	return r, err == nil
}

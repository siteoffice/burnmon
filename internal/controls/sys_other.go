//go:build !windows

package controls

import (
	"context"
	"errors"
	"os"
)

var errUnsupported = errors.New("controls run on Windows only")

// CurrentSelf has no ancestor walk off Windows.
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

//go:build darwin

package instance

import (
	"errors"
	"os"
	"syscall"
)

// held keeps the locked files open for the process lifetime; closing a
// descriptor releases its flock.
var held []*os.File

// Acquire takes an exclusive, non-blocking flock on path (created 0600). It
// returns true when another process already holds the lock.
func Acquire(path string) (alreadyHeld bool, err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return true, nil
		}
		return false, err
	}
	held = append(held, f)
	return false, nil
}

//go:build !windows

package snapshotwriter

import (
	"os"
	"syscall"
)

// lockFile takes the exclusive lock without blocking. false with a nil error
// means another open file holds it.
func lockFile(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == syscall.EWOULDBLOCK {
		return false, nil
	}
	return err == nil, err
}

func unlockFile(f *os.File) {
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

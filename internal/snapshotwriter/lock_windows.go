package snapshotwriter

import (
	"os"

	"golang.org/x/sys/windows"
)

// The lock is one byte at offset 4 GiB, past the owner text, so readers can
// still read the file while it is locked.
func lockRange() *windows.Overlapped { return &windows.Overlapped{Offset: 0, OffsetHigh: 1} }

// lockFile takes the exclusive lock without blocking. false with a nil error
// means another handle holds it.
func lockFile(f *os.File) (bool, error) {
	err := windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, lockRange())
	if err == windows.ERROR_LOCK_VIOLATION {
		return false, nil
	}
	return err == nil, err
}

func unlockFile(f *os.File) {
	windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, lockRange())
}

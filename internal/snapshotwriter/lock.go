package snapshotwriter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Owner is the diagnosis text in the lock file: who writes the snapshot.
// Liveness is the OS lock, never this text.
type Owner struct {
	PID   int    `json:"pid"`
	Exe   string `json:"exe"`
	Since string `json:"since"` // RFC3339 UTC
}

// DefaultLockPath is %USERPROFILE%\.burnmon\snapshot.lock, next to
// snapshot.json.
func DefaultLockPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".burnmon", "snapshot.lock"), nil
}

// Lock is the single-writer lock: an OS file lock on path that the OS drops
// the moment the owning process dies, so a dead owner never blocks.
type Lock struct {
	path string
	f    *os.File
}

// NewLock returns a lock on path. It takes nothing until TryAcquire.
func NewLock(path string) *Lock { return &Lock{path: path} }

// Path is the lock file path, where ReadOwner finds the owner text.
func (l *Lock) Path() string { return l.path }

// Held reports whether this Lock holds the OS lock.
func (l *Lock) Held() bool { return l.f != nil }

// TryAcquire takes the lock without blocking. It returns true when this Lock
// holds it, already or now, and false (nil error) when another process or
// Lock does. It creates the folder, and on success replaces the file text
// with the Owner JSON. A failed attempt closes its file handle.
func (l *Lock) TryAcquire(exe string) (bool, error) {
	if l.f != nil {
		return true, nil
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(l.path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return false, err
	}
	ok, err := lockFile(f)
	if err != nil || !ok {
		f.Close()
		return false, err
	}
	body, err := json.Marshal(Owner{PID: os.Getpid(), Exe: exe, Since: time.Now().UTC().Format(time.RFC3339)})
	if err == nil {
		err = f.Truncate(0)
	}
	if err == nil {
		_, err = f.WriteAt(body, 0)
	}
	if err != nil {
		unlockFile(f)
		f.Close()
		return false, err
	}
	l.f = f
	return true, nil
}

// Release drops the lock. It is safe to call when not held.
func (l *Lock) Release() error {
	if l.f == nil {
		return nil
	}
	f := l.f
	l.f = nil
	unlockFile(f)
	return f.Close()
}

// ReadOwner reads the Owner text of the lock file at path.
func ReadOwner(path string) (Owner, error) {
	var o Owner
	b, err := os.ReadFile(path)
	if err != nil {
		return o, err
	}
	err = json.Unmarshal(b, &o)
	return o, err
}

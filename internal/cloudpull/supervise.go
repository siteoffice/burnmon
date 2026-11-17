package cloudpull

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

// LockPath is ~\.burnmon\cloud-pull.lock: whichever app holds it pulls, so
// burnmon.exe and BurnMon Dev open together still start one CLI at a time.
func LockPath() string { return filepath.Join(home(), ".burnmon", "cloud-pull.lock") }

// Locker is the cross-process lock the apps pass in (snapshotwriter.Lock on
// LockPath): an OS file lock the OS drops when the holding process dies.
type Locker interface {
	TryAcquire(exe string) (bool, error)
	Release() error
}

// Supervisor runs in every app. Each tick it reads the switch; when the switch
// is on and this app holds the lock, it runs one runner pass. When the switch
// is off it lets the lock go and clears a cost-guard stop.
type Supervisor struct {
	On        func() bool // the cloud_pull switch, read fresh
	Lock      Locker
	Exe       string // this app, for the lock's owner text
	StatePath string
	NewRunner func() (*Runner, error)

	r *Runner
}

// NewSupervisor builds the supervisor for one app on this computer.
func NewSupervisor(on func() bool, lock Locker) *Supervisor {
	exe, _ := os.Executable()
	return &Supervisor{
		On:        on,
		Lock:      lock,
		Exe:       filepath.Base(exe),
		StatePath: StatePath(),
		NewRunner: NewRunner,
	}
}

// Tick is one supervisor pass.
func (s *Supervisor) Tick(ctx context.Context) error {
	if !s.On() {
		s.r = nil
		if err := s.Lock.Release(); err != nil {
			return err
		}
		return ClearDisabled(s.StatePath)
	}
	held, err := s.Lock.TryAcquire(s.Exe)
	if err != nil || !held {
		return err
	}
	if s.r == nil {
		r, err := s.NewRunner()
		if err != nil {
			recordError(s.StatePath, err)
			return err
		}
		s.r = r
	}
	return s.r.Step(ctx)
}

// Run ticks every 30 seconds until ctx ends. Errors are in the state file,
// where the screen reads them; onErr may log them.
func (s *Supervisor) Run(ctx context.Context, onErr func(error)) {
	t := time.NewTicker(tickEvery)
	defer t.Stop()
	for {
		if err := s.Tick(ctx); err != nil && onErr != nil {
			onErr(err)
		}
		select {
		case <-ctx.Done():
			s.Lock.Release()
			return
		case <-t.C:
		}
	}
}

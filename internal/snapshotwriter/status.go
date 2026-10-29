package snapshotwriter

import (
	"sync"
	"time"
)

// Writer states, as a reader of Status sees them.
const (
	StateStarting  = "starting"   // no write yet since start
	StateWriting   = "writing"    // this process holds the lock and its last write landed
	StateOther     = "other"      // another process holds the lock
	StateFailed    = "failed"     // this process holds the lock and its last write failed
	StateLockError = "lock_error" // taking the lock failed
)

// StatusView is one copy of the writer state, safe to send to a page.
type StatusView struct {
	State       string `json:"state"`
	Error       string `json:"error,omitempty"`
	OwnerExe    string `json:"owner_exe,omitempty"`
	OwnerPID    int    `json:"owner_pid,omitempty"`
	LastWriteAt string `json:"last_write_at,omitempty"` // RFC3339 UTC
}

// Status is the writer state of this process. Run updates it on every lock
// attempt and write; a reader takes a copy with View. A nil *Status is safe
// for every method, so a Writer without one tracks nothing.
type Status struct {
	mu sync.Mutex
	v  StatusView
}

// NewStatus returns a Status in the starting state.
func NewStatus() *Status { return &Status{v: StatusView{State: StateStarting}} }

// View returns a copy of the state. On a nil Status it is the zero view
// (state ""), which a reader treats as unknown.
func (s *Status) View() StatusView {
	if s == nil {
		return StatusView{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.v
}

// Fail records a setup error before Run, such as a lock path that did not
// resolve: the writer then never takes the lock, so it is a lock error.
func (s *Status) Fail(err error) { s.setLockError(err) }

// set replaces the view with v in one step; the time of the last good write
// is kept unless v carries a new one.
func (s *Status) set(v StatusView) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.LastWriteAt == "" {
		v.LastWriteAt = s.v.LastWriteAt
	}
	s.v = v
}

func (s *Status) setLockError(err error) {
	s.set(StatusView{State: StateLockError, Error: err.Error()})
}

func (s *Status) setFailed(err error) {
	s.set(StatusView{State: StateFailed, Error: err.Error()})
}

// setOther records the lock owner from its lock file text. An unreadable
// owner (err) leaves exe empty and pid 0, so a reader names no process.
func (s *Status) setOther(o Owner, err error) {
	v := StatusView{State: StateOther}
	if err == nil {
		v.OwnerExe, v.OwnerPID = o.Exe, o.PID
	}
	s.set(v)
}

func (s *Status) setWrote(at time.Time) {
	s.set(StatusView{State: StateWriting, LastWriteAt: at.UTC().Format(time.RFC3339)})
}

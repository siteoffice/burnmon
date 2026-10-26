package controls

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"sync"
	"time"
)

// Preview timing: a token lives 5 s and cannot be confirmed in the first 400 ms,
// so a double-click never runs a control.
const (
	PreviewTTL    = 5 * time.Second
	MinConfirmGap = 400 * time.Millisecond
)

var (
	ErrNoPreview = errors.New("no preview, press once to see the command")
	ErrExpired   = errors.New("preview expired, press once to see the command again")
	ErrTooSoon   = errors.New("second press too soon")
)

// Target is what a preview was made for.
type Target struct {
	Control string
	// Claude Code
	AgentID   string
	PID       int
	StartedAt int64
	Project   string
	// Foreman gate
	FlowID, Gate, GateToken, FlowDir, Title string
}

// Key identifies the control and target; a new preview with the same key
// replaces the old one.
func (t Target) Key() string {
	if t.AgentID != "" {
		return t.Control + "|" + t.AgentID
	}
	// Flow ids and gates are not validated in this package, so the flow id is
	// length-prefixed: no id or gate containing ":" can collide with another.
	return t.Control + "|gate:" + strconv.Itoa(len(t.FlowID)) + ":" + t.FlowID + ":" + t.Gate
}

// Label is the human name of the target.
func (t Target) Label() string {
	if t.AgentID != "" {
		return t.Project + " (" + t.AgentID + ")"
	}
	return t.Title + " gate " + t.Gate + " (" + t.FlowID + ")"
}

// Pending is one open preview. Argv is what runs on confirm.
type Pending struct {
	Token   string
	Target  Target
	Argv    []string
	Command string
	Created time.Time
}

// Previews holds the open previews. The zero value is ready to use.
type Previews struct {
	Now     func() time.Time // nil means time.Now
	mu      sync.Mutex
	byToken map[string]Pending
}

func (p *Previews) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// Add stores a preview under a fresh 128-bit random token, drops expired
// previews and replaces a pending one with the same key.
func (p *Previews) Add(t Target, argv []string) Pending {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	if p.byToken == nil {
		p.byToken = map[string]Pending{}
	}
	for tok, pe := range p.byToken {
		if now.Sub(pe.Created) > PreviewTTL || pe.Target.Key() == t.Key() {
			delete(p.byToken, tok)
		}
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("controls: crypto/rand failed: " + err.Error())
	}
	pe := Pending{
		Token:   hex.EncodeToString(b[:]),
		Target:  t,
		Argv:    append([]string(nil), argv...),
		Command: CommandLine(argv),
		Created: now,
	}
	p.byToken[pe.Token] = pe
	// Return a copy so the caller's Argv is never the stored slice.
	pe.Argv = append([]string(nil), pe.Argv...)
	return pe
}

// Take returns and removes the preview for token. Unknown is ErrNoPreview,
// older than PreviewTTL is ErrExpired (removed), younger than MinConfirmGap
// is ErrTooSoon (kept).
func (p *Previews) Take(token string) (Pending, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pe, ok := p.byToken[token]
	if !ok {
		return Pending{}, ErrNoPreview
	}
	age := p.now().Sub(pe.Created)
	if age > PreviewTTL {
		delete(p.byToken, token)
		return Pending{}, ErrExpired
	}
	if age < MinConfirmGap {
		return Pending{}, ErrTooSoon
	}
	delete(p.byToken, token)
	pe.Argv = append([]string(nil), pe.Argv...)
	return pe, nil
}

// Peek returns the preview for token without taking it, so a refused press
// can be audited with the target it was made for.
func (p *Previews) Peek(token string) (Pending, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pe, ok := p.byToken[token]
	pe.Argv = append([]string(nil), pe.Argv...)
	return pe, ok
}

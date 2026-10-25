package snapshotwriter

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"burnmon/internal/waits"
)

// agentViewEvery is how often "claude agents --json" is run when the opt-in
// agent_view setting is on; agentViewTimeout bounds one run.
const (
	agentViewEvery   = 30 * time.Second
	agentViewTimeout = 10 * time.Second
)

// ClaudeAgentView returns a runner for "<claudePath> agents --json": hidden
// window, stdout only. A non-zero exit is an error that carries the first 200
// bytes of stderr. claudePath must come from ResolveClaude, which accepts only a
// directly startable binary (a .exe on Windows), so no cmd.exe sits between
// the timeout and claude. It never goes through cmd /c, WMI or CIM.
func ClaudeAgentView(claudePath string) func(ctx context.Context) ([]byte, error) {
	return func(ctx context.Context) ([]byte, error) {
		cmd := exec.CommandContext(ctx, claudePath, "agents", "--json")
		hideWindow(cmd)
		// Without a wait delay, a supervisor that inherits stdout keeps Output
		// blocked after the timeout kills claude, and no later poll would run.
		cmd.WaitDelay = 2 * time.Second
		out, err := cmd.Output()
		if err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) && len(ee.Stderr) > 0 {
				e := ee.Stderr
				if len(e) > 200 {
					e = e[:200]
				}
				return nil, fmt.Errorf("%w: %s", err, e)
			}
			return nil, err
		}
		return out, nil
	}
}

// agentPoll is the poller's state between ticks.
type agentPoll struct {
	mu      sync.Mutex
	running bool
	started time.Time // start of the newest run
	done    bool      // a run has finished
	at      time.Time // end of the last finished run
	err     error
	entries []waits.AgentEntry
	seen    map[string]time.Time
}

func (gr *GraphReaders) clock() time.Time {
	if gr.Now != nil {
		return gr.Now()
	}
	return time.Now()
}

// runAgentView runs the runner once and stores its result.
func (gr *GraphReaders) runAgentView(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, agentViewTimeout)
	defer cancel()
	out, err := gr.AgentView(ctx)
	var entries []waits.AgentEntry
	if err == nil {
		entries, err = waits.ParseAgentView(out)
		if err != nil {
			err = fmt.Errorf("parse agent view: %w", err)
		}
	}
	at := gr.clock()

	p := &gr.poll
	p.mu.Lock()
	defer p.mu.Unlock()
	p.running, p.done, p.at, p.err = false, true, at, err
	if err != nil {
		p.entries, p.seen = nil, nil
		return
	}
	seen := map[string]time.Time{}
	for i := range entries {
		if !entries[i].Waiting() {
			continue
		}
		k := entries[i].Key()
		first, ok := p.seen[k]
		if !ok {
			first = at
		}
		seen[k] = first
		entries[i].Since = first
	}
	p.entries, p.seen = entries, seen
}

// PollAgentViewNow runs one poll synchronously and stores its result. It does
// nothing when AgentView is nil.
func (gr *GraphReaders) PollAgentViewNow(ctx context.Context) {
	if gr.AgentView == nil {
		return
	}
	p := &gr.poll
	p.mu.Lock()
	p.running, p.started = true, gr.clock()
	p.mu.Unlock()
	gr.runAgentView(ctx)
}

// startAgentViewIfDue starts a poll in its own goroutine when none is in
// flight and agentViewEvery has passed, so a tick never waits for it.
func (gr *GraphReaders) startAgentViewIfDue(now time.Time) {
	if gr.AgentView == nil {
		return
	}
	p := &gr.poll
	p.mu.Lock()
	if p.running || (!p.started.IsZero() && now.Sub(p.started) < agentViewEvery) {
		p.mu.Unlock()
		return
	}
	p.running, p.started = true, now
	p.mu.Unlock()
	go gr.runAgentView(context.Background())
}

// agentViewState is the last finished poll: its entries, read time and error.
func (gr *GraphReaders) agentViewState() (entries []waits.AgentEntry, at time.Time, done bool, err error) {
	p := &gr.poll
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]waits.AgentEntry(nil), p.entries...), p.at, p.done, p.err
}

package controls

import (
	"context"
	"errors"
	"strings"

	"burnmon/internal/graph"
	"burnmon/internal/waits"
)

// Runner starts the previewed argv. Neither method uses a shell.
type Runner interface {
	// Run starts argv hidden and waits for it.
	Run(ctx context.Context, argv []string, dir string) (exitCode int, firstLine string, err error)
	// Start opens argv in a new console and does not wait.
	Start(argv []string) error
}

// CheckAgent compares the preview target with a fresh agent view read. It
// returns the fresh entry when the control may run.
func CheckAgent(t Target, fresh []waits.AgentEntry, self Self) (waits.AgentEntry, error) {
	var e waits.AgentEntry
	found := false
	for _, f := range fresh {
		if t.AgentID != "" && f.ID == t.AgentID {
			e, found = f, true
			break
		}
	}
	if !found {
		return waits.AgentEntry{}, errors.New("the session is no longer listed")
	}
	if e.Kind != "background" {
		return waits.AgentEntry{}, errors.New("only background sessions have controls")
	}
	if e.PID != t.PID || e.StartedAt != t.StartedAt {
		return waits.AgentEntry{}, errors.New("the session changed since the preview, press again")
	}
	if t.Control == Stop {
		switch e.State {
		case "stopped", "done", "failed":
			return waits.AgentEntry{}, errors.New("the session has already ended")
		}
	}
	if err := self.Refuses(e.PID, e.Cwd); err != nil {
		return waits.AgentEntry{}, err
	}
	return e, nil
}

// CheckGate compares the preview target with a fresh Foreman read.
func CheckGate(t Target, flows []graph.Flow) error {
	for _, f := range flows {
		if f.ID != t.FlowID {
			continue
		}
		if !f.GatePending {
			return errors.New("the gate is already decided")
		}
		if f.Gate != t.Gate || f.GateToken == "" || f.GateToken != t.GateToken || f.Dir != t.FlowDir {
			return errors.New("the gate changed since the preview, press again")
		}
		return nil
	}
	return errors.New("the flow is no longer listed")
}

// firstLine is the first non-empty line of stdout, else of stderr, cut to 120 runes.
func firstLine(stdout, stderr string) string {
	for _, s := range []string{stdout, stderr} {
		for _, l := range strings.Split(s, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				r := []rune(l)
				if len(r) > 120 {
					r = r[:120]
				}
				return string(r)
			}
		}
	}
	return ""
}

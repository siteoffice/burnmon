package controls

import (
	"context"
	"errors"
	"strings"

	"burnmon/internal/graph"
	"burnmon/internal/waits"
)

const errDupFlow = "the flow id is listed more than once"

// Runner starts the previewed argv. Neither method uses a shell, and on
// Windows both refuse an argv[0] that is not an absolute path ending in .exe,
// so the exact stored argv never depends on PATH, PATHEXT or a shim.
//
// A nonzero exit is not an error: Run returns (code, firstLine, nil) and the
// caller must test code != 0. err is non-nil only when the process could not
// start or was killed by the context (then code is -1).
//
// There is no Job object. Run kills the child it started when the context
// ends, but a grandchild can survive; WaitDelay (2 s) only bounds how long Run
// waits for the output pipes.
type Runner interface {
	// Run starts argv hidden and waits for it.
	Run(ctx context.Context, argv []string, dir string) (exitCode int, firstLine string, err error)
	// Start opens argv in a new console and does not wait.
	Start(argv []string) error
}

// LastLineRunner is a Runner that can also return the last stderr line of a
// failing process instead of the first line. Only the Close control asks for
// it: a Python script prints its reason last, after the traceback.
type LastLineRunner interface {
	RunLast(ctx context.Context, argv []string, dir string) (exitCode int, line string, err error)
}

// CheckAgent compares the preview target with a fresh agent view read. It
// returns the fresh entry when the control may run.
func CheckAgent(t Target, fresh []waits.AgentEntry, self Self) (waits.AgentEntry, error) {
	var e waits.AgentEntry
	matches := 0
	for _, f := range fresh {
		if t.AgentID != "" && f.ID == t.AgentID {
			e = f
			matches++
		}
	}
	if matches == 0 {
		return waits.AgentEntry{}, errors.New("the session is no longer listed")
	}
	if matches > 1 {
		return waits.AgentEntry{}, errors.New("the session id is listed more than once")
	}
	if e.Kind != "background" {
		return waits.AgentEntry{}, errors.New("only background sessions have controls")
	}
	// Allowlist, per the agent-view JSON states: working, blocked and done all
	// mean a live process (done is ready for the next prompt). Attach would
	// restart a stopped session, so the control is refused for it by decision,
	// as are failed and any state this build does not know.
	switch e.State {
	case "working", "blocked", "done":
	default:
		return waits.AgentEntry{}, errors.New("the session has ended, restart it from a terminal")
	}
	if e.PID <= 0 || t.PID <= 0 {
		return waits.AgentEntry{}, errors.New(errNoPID)
	}
	if e.PID != t.PID || e.StartedAt != t.StartedAt {
		return waits.AgentEntry{}, errors.New("the session changed since the preview, press again")
	}
	if err := self.Refuses(e.PID, e.Cwd); err != nil {
		return waits.AgentEntry{}, err
	}
	return e, nil
}

// CheckGate compares the preview target with a fresh Foreman read.
func CheckGate(t Target, flows []graph.Flow) error {
	matches := 0
	for _, f := range flows {
		if f.ID == t.FlowID {
			matches++
		}
	}
	if matches > 1 {
		return errors.New(errDupFlow)
	}
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

// lastLine is the last non-empty line of s, trimmed and cut to max runes. A
// Python error prints its reason last, after the traceback.
func lastLine(s string, max int) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			r := []rune(l)
			if len(r) > max {
				r = r[:max]
			}
			return string(r)
		}
	}
	return ""
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

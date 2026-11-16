package controls

import (
	"runtime"
	"time"

	"burnmon/internal/gitrefs"
	"burnmon/internal/graph"
	"burnmon/internal/pricing"
	"burnmon/internal/snapshot"
	"burnmon/internal/snapshotwriter"
)

// NewEnv builds the Env that BurnMon Dev and the Control Centre both use. The
// switches come from the caller, because each app reads them its own way; the
// audit path is the file the caller has already worked out. A claude or python
// that does not resolve through look leaves its path empty, which makes the
// controls that need it unavailable with a reason.
func NewEnv(cfg pricing.Config, snapPath, auditPath string, switches func() map[string]bool, look func(string) (string, error)) Env {
	env := Env{
		AgentViewOn: cfg.AgentView,
		ForemanRoot: cfg.ForemanRoot,
		ReadForeman: graph.ReadForeman,
		Snapshot:    func() snapshot.Reading { return snapshot.Load(snapPath, time.Now()) },
		Switches:    switches,
		Self:        CurrentSelf,
		Elevated:    Elevated,
		Runner:      ExecRunner{},
		AuditPath:   auditPath,
		Merged:      gitrefs.NewChecker().MergedOrGone,
	}
	if p, err := snapshotwriter.ResolveClaudeConfigured(cfg.ClaudePath, look, runtime.GOOS); err == nil {
		env.ClaudePath = p
		env.AgentView = snapshotwriter.ClaudeAgentView(p)
	}
	if p, err := ResolvePython(look, runtime.GOOS); err == nil {
		env.PythonPath = p
	}
	return env
}

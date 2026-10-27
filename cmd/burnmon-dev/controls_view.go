//go:build windows

// controls_view.go is the Go side of the BurnMon Dev controls. The page may
// send only two things: a control name plus a target id (PreviewRequest), and
// later the preview token. It never sends an argv, a path, a pid or a command;
// internal/controls builds, checks and audits everything. The switches live in
// burnmon-dev-view.json and are read on every call; each is off by default.
package main

import (
	"log"
	"os/exec"
	"runtime"
	"time"

	"burnmon/internal/controls"
	"burnmon/internal/gitrefs"
	"burnmon/internal/graph"
	"burnmon/internal/pricing"
	"burnmon/internal/snapshot"
	"burnmon/internal/snapshotwriter"
)

// newControlService resolves claude, python and the audit path once, with the
// real PATH lookup.
func newControlService(cfg pricing.Config, dataDir, snapPath, storePath string) *controls.Service {
	return newControlServiceWith(cfg, dataDir, snapPath, storePath, exec.LookPath)
}

// newControlServiceWith is newControlService with an injectable PATH lookup.
// A binary that does not resolve leaves its path empty, which makes the
// controls that need it unavailable with a reason.
func newControlServiceWith(cfg pricing.Config, dataDir, snapPath, storePath string, look func(string) (string, error)) *controls.Service {
	env := controls.Env{
		AgentViewOn: cfg.AgentView,
		ForemanRoot: cfg.ForemanRoot,
		ReadForeman: graph.ReadForeman,
		Snapshot:    func() snapshot.Reading { return snapshot.Load(snapPath, time.Now()) },
		Switches:    func() map[string]bool { return loadSections(dataDir).Controls },
		Self:        controls.CurrentSelf,
		Elevated:    controls.Elevated,
		Runner:      controls.ExecRunner{},
		AuditPath:   controls.AuditPath(storePath),
		Merged:      gitrefs.NewChecker().Merged,
	}
	if p, err := snapshotwriter.ResolveClaudeConfigured(cfg.ClaudePath, look, runtime.GOOS); err == nil {
		env.ClaudePath = p
		env.AgentView = snapshotwriter.ClaudeAgentView(p)
	}
	if p, err := controls.ResolvePython(look, runtime.GOOS); err == nil {
		env.PythonPath = p
	}
	return controls.NewService(env)
}

// bdevControlPreview is the first press: it returns the exact command and a
// token, or the reason the control cannot run on this target.
func bdevControlPreview(svc *controls.Service, req controls.PreviewRequest) (controls.PreviewResult, error) {
	res, err := svc.Preview(req)
	if err != nil {
		log.Println("bdevControlPreview:", err)
		return controls.PreviewResult{}, err
	}
	return res, nil
}

// bdevControlRun is the second press. The token is the only input. A refusal
// comes back as the error and also in the result's Refused text.
func bdevControlRun(svc *controls.Service, token string) (controls.RunResult, error) {
	res, err := svc.Run(token)
	if err != nil {
		log.Println("bdevControlRun:", err)
	}
	return res, err
}

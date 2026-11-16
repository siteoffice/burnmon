//go:build windows || darwin

// controls_view.go is the Go side of the BurnMon Dev controls. The page may
// send only two things: a control name plus a target id (PreviewRequest), and
// later the preview token. It never sends an argv, a path, a pid or a command;
// internal/controls builds, checks and audits everything. The switches live in
// burnmon-dev-view.json and are read on every call; each is off by default.
package main

import (
	"log"
	"os/exec"

	"burnmon/internal/controls"
	"burnmon/internal/pricing"
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
	switches := func() map[string]bool { return loadSections(dataDir).Controls }
	return controls.NewService(controls.NewEnv(cfg, snapPath, controls.AuditPath(storePath), switches, look))
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

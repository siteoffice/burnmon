//go:build windows || darwin

// decide.go is the Go side of a decision (spec Definitions): Approve or Send
// back on a waiting flow row, or Stop on an agent. The page sends only a
// control with a project and a slug, or with an agent id, and later the
// preview token; never a command, a path or an argv.
// internal\controls builds, checks, runs and audits everything.

package main

import (
	"errors"

	"burnmon/internal/controls"
	"burnmon/internal/snapshot"
	"burnmon/internal/waits"
)

// DecisionRequest is what the page sends with ccPreview.
type DecisionRequest struct {
	Control string `json:"control"`
	Project string `json:"project,omitempty"`
	Slug    string `json:"slug,omitempty"`
	Agent   string `json:"agent,omitempty"`
}

// RunView is what ccRun returns: the run result and the audit file it went to.
type RunView struct {
	controls.RunResult
	AuditPath string `json:"audit_path"`
}

// gateTarget maps a waiting flow row to its gate wait item in the BurnMon
// snapshot (choices item 21): the one gate item with title == slug and
// project == project. None or more than one is refused.
func gateTarget(items []waits.Item, project, slug string) (string, error) {
	id, n := "", 0
	for _, it := range items {
		if it.Kind == "gate" && it.Title == slug && it.Project == project {
			id = it.ID
			n++
		}
	}
	switch {
	case n == 0:
		return "", errors.New("this gate is not in the BurnMon snapshot: start BurnMon Dev")
	case n > 1:
		return "", errors.New("more than one gate matches this flow")
	}
	return id, nil
}

// decisionsOff is the refusal when main could not build the service.
func decisionsOff(e bindEnv) error {
	why := e.svcWhy
	if why == "" {
		why = "the controls service is not set up"
	}
	return errors.New("decisions are off: " + why)
}

// previewDecision is the ccPreview handler: it finds the gate for the row in
// the snapshot and asks the controls service for the command and a token.
// It runs nothing and writes nothing.
func previewDecision(e bindEnv, req DecisionRequest) (controls.PreviewResult, error) {
	if req.Control == controls.Stop {
		// Stop on an agent: the controls service finds the agent in the
		// snapshot, checks it against a fresh agent view and builds the argv.
		if req.Agent == "" {
			return controls.PreviewResult{}, errors.New("stop needs an agent")
		}
		if e.svc == nil {
			return controls.PreviewResult{}, decisionsOff(e)
		}
		return e.svc.Preview(controls.PreviewRequest{Control: controls.Stop, Target: req.Agent})
	}
	if req.Control != controls.Approve && req.Control != controls.SendBack {
		return controls.PreviewResult{}, errors.New("unknown control")
	}
	if req.Project == "" || req.Slug == "" {
		return controls.PreviewResult{}, errors.New(req.Control + " needs a project and a flow")
	}
	if e.svc == nil {
		return controls.PreviewResult{}, decisionsOff(e)
	}
	rd := snapshot.Load(e.snapPath, e.clock())
	if rd.Status != snapshot.StatusOK || rd.Snapshot == nil {
		return controls.PreviewResult{}, errors.New("BurnMon Dev is not running, so the gate cannot be checked")
	}
	target, err := gateTarget(rd.Snapshot.WaitItems, req.Project, req.Slug)
	if err != nil {
		return controls.PreviewResult{}, err
	}
	return e.svc.Preview(controls.PreviewRequest{Control: req.Control, Target: target})
}

// runDecision is the ccRun handler. The token is the only input. A refusal
// returns the view (Refused set) and the error.
func runDecision(e bindEnv, token string) (RunView, error) {
	if e.svc == nil {
		err := decisionsOff(e)
		return RunView{RunResult: controls.RunResult{ExitCode: -1, Refused: err.Error()}, AuditPath: e.auditPath}, err
	}
	res, err := e.svc.Run(token)
	return RunView{RunResult: res, AuditPath: e.auditPath}, err
}

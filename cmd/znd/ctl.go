package main

import (
	"burnmon/internal/controls"
	"burnmon/internal/controlsenv"
)

const (
	controlsApprove  = controls.Approve
	controlsSendBack = controls.SendBack
	gateExitChanged  = controls.GateExitChanged
	gateExitVerdict  = controls.GateExitVerdict
)

func previewRequest(control, target string) controls.PreviewRequest {
	return controls.PreviewRequest{Control: control, Target: target}
}

func newService(d deps, z controlsenv.ZndConfig) *controls.Service {
	return controls.NewService(d.env(z))
}

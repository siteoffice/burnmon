//go:build windows || darwin

package main

import (
	"errors"
	"path/filepath"

	"burnmon/internal/sections"
)

// ccControlIDs are the controls the Control Centre switches on its Settings
// page. BurnMon Dev keeps one control, Stop (decision 2026-11-11, second look
// 2026-11-15); the switches stay the same keys in burnmon-dev-view.json, so
// both apps read one file.
var ccControlIDs = []string{"attach", "approve", "send_back", "close_sprint"}

// ControlsView is what the Settings page draws: each switch, whether the
// control is usable now, and why not.
type ControlsView struct {
	IDs   []string          `json:"ids"`
	On    map[string]bool   `json:"on"`
	Ready map[string]bool   `json:"ready"`
	Why   map[string]string `json:"why"`
}

// ControlSwitch is one switch press from the Settings page.
type ControlSwitch struct {
	ID string `json:"id"`
	On bool   `json:"on"`
}

func viewStatePath(e bindEnv) string { return filepath.Join(e.dataDir, "burnmon-dev-view.json") }

// controlsView reads the switches from burnmon-dev-view.json and the
// readiness from the controls service (nil: svcWhy for each).
func controlsView(e bindEnv) ControlsView {
	st := sections.Load(viewStatePath(e))
	v := ControlsView{IDs: ccControlIDs, On: map[string]bool{}, Ready: map[string]bool{}, Why: map[string]string{}}
	for _, id := range ccControlIDs {
		v.On[id] = st.Controls[id]
		v.Why[id] = e.svcWhy
	}
	if e.svc == nil {
		return v
	}
	a := e.svc.Availability()
	for _, id := range ccControlIDs {
		v.Ready[id] = a.Ready[id]
		why := rewriteReason(a.Why[id])
		if !v.On[id] {
			why = "" // the unticked box says it
		}
		v.Why[id] = why
	}
	return v
}

// controlSet saves one of the four switches and returns the new view. Stop
// and unknown ids are refused and change nothing.
func controlSet(e bindEnv, c ControlSwitch) (ControlsView, error) {
	known := false
	for _, id := range ccControlIDs {
		known = known || id == c.ID
	}
	if !known {
		return controlsView(e), errors.New("the Control Centre switches attach, approve, send back and close sprint; Stop is switched in BurnMon Dev")
	}
	p := viewStatePath(e)
	if err := sections.Save(p, sections.Load(p).WithControl(c.ID, c.On)); err != nil {
		return controlsView(e), err
	}
	return controlsView(e), nil
}

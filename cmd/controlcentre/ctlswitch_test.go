//go:build windows || darwin

package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"burnmon/internal/sections"
)

// The Control Centre switches attach, approve, send_back and close_sprint in
// burnmon-dev-view.json, the same keys BurnMon Dev used; Stop stays BurnMon
// Dev's (second look 2026-11-15).
func TestControlSwitchesWriteTheViewFile(t *testing.T) {
	dir := t.TempDir()
	e := bindEnv{dataDir: dir, svcWhy: "the controls service is not set up"}
	p := filepath.Join(dir, "burnmon-dev-view.json")
	if err := sections.Save(p, sections.Default().WithControl("stop", true)); err != nil {
		t.Fatal(err)
	}
	v := controlsView(e)
	if !reflect.DeepEqual(v.IDs, []string{"attach", "approve", "send_back", "close_sprint"}) {
		t.Fatalf("ids %v", v.IDs)
	}
	for _, id := range v.IDs {
		if v.On[id] || v.Ready[id] || v.Why[id] != "the controls service is not set up" {
			t.Errorf("%s: on %v ready %v why %q", id, v.On[id], v.Ready[id], v.Why[id])
		}
	}
	got, err := controlSet(e, ControlSwitch{ID: "approve", On: true})
	if err != nil {
		t.Fatal(err)
	}
	if !got.On["approve"] {
		t.Error("approve is not on in the returned view")
	}
	st := sections.Load(p)
	if !st.Controls["approve"] || !st.Controls["stop"] || st.Controls["attach"] {
		t.Errorf("file controls %v, want approve and stop on, attach off", st.Controls)
	}
	if _, err := controlSet(e, ControlSwitch{ID: "stop", On: false}); err == nil || !strings.Contains(err.Error(), "BurnMon Dev") {
		t.Errorf("stop: err %v, want a refusal naming BurnMon Dev", err)
	}
	if !sections.Load(p).Controls["stop"] {
		t.Error("a refused stop switch changed the file")
	}
}

// The Settings page shows the four switches with their explanation and calls
// the two bindings.
func TestSettingsPageControls(t *testing.T) {
	mustContain(t, "pageSettings", "controlsRows();")
	mustContain(t, "controlsRows", "api.controls()", "api.controlSet(", "'Attach to a session'", "'Approve a gate'",
		"'Send a gate back'", "'Close sprint'", "python on PATH and foreman_root", "claude_path and agent_view")
}

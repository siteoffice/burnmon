package main

import (
	"path/filepath"
	"strings"
	"testing"

	"burnmon/internal/cloudpull"
	"burnmon/internal/pricing"
)

// The settings overlay carries the switch with the shared text, ticked from
// the config, and Save writes cloud_pull next to the subscription.
func TestSettingsCloudPullSwitch(t *testing.T) {
	a := &app{cfgPath: filepath.Join(t.TempDir(), "burnmon.json"), seat: "Standard"}
	html := a.settingsModalHTML()
	for _, want := range append([]string{
		`id="cc_s_cloudPull" type="checkbox">`,
		cloudpull.SwitchLabel,
		"cloudPull: document.getElementById('cc_s_cloudPull').checked",
	}, cloudpull.SwitchLines...) {
		if !strings.Contains(html, want) {
			t.Errorf("settings overlay lacks %q", want)
		}
	}
	if err := a.applySettings(settingsPayload{YourSeat: "Standard", CloudPull: true}); err != nil {
		t.Fatal(err)
	}
	if !pricing.ReadCloudPull(a.cfgPath) || !a.cfg.CloudPull {
		t.Fatal("Save did not store cloud_pull true")
	}
	if !strings.Contains(a.settingsModalHTML(), `id="cc_s_cloudPull" type="checkbox" checked>`) {
		t.Error("switch not ticked after saving it on")
	}
}

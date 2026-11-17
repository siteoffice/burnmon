//go:build windows

package main

import (
	"strings"
	"testing"

	"burnmon/internal/cloudpull"
)

// The gear carries the switch "Read Cowork (Cloud) sessions" with the same
// lines as burnmon.exe (cloudpull.SwitchLines is the one source), saves it
// through bdevSetCloudPull and ticks it from the payload's cloud_pull.
func TestPageCloudPullSwitch(t *testing.T) {
	for _, want := range append([]string{
		`id="secCloudPull"> ` + cloudpull.SwitchLabel + `</label>`,
		"window.bdevSetCloudPull(t.checked)",
		"cloudPull:!!p.cloud_pull",
		"cp.checked = !!secState.cloudPull",
	}, cloudpull.SwitchLines...) {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page.html lacks %q", want)
		}
	}
}

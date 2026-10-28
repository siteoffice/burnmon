//go:build windows

package main

import (
	"strings"

	"burnmon/internal/agentmap"
	"burnmon/internal/waits"
)

// blockedHasBox reports whether the layout shows a Blocked item: a box, a
// frame or an idle row carrying its id, or (for a gate) the flow it belongs
// to, which is how a gate with no agent is drawn.
func blockedHasBox(ly agentmap.Layout, it waits.Item) bool {
	ids := []string{it.ID}
	if parts := strings.Split(it.ID, ":"); parts[0] == "gate" && len(parts) >= 3 {
		ids = append(ids, "flow:"+parts[1])
	}
	for _, id := range ids {
		for _, b := range ly.Boxes {
			if strings.Contains(b.ID, id) {
				return true
			}
		}
		for _, f := range ly.Frames {
			if strings.Contains(f.ID, id) {
				return true
			}
		}
		for _, i := range ly.Idle {
			if strings.Contains(i.ID, id) {
				return true
			}
		}
	}
	return false
}

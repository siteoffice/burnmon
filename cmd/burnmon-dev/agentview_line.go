//go:build windows

// agentview_line.go is the one line Waits on you shows when the agent view is
// off or refused, with the burnmon.json key that fixes it. It reads the state
// the snapshot writer recorded, since that process (burnmon.exe or this
// window, whichever holds the lock) is the one that polls agent view.
package main

import (
	"os"
	"path/filepath"

	"burnmon/internal/snapshot"
)

// agentViewLine is empty when the agent view is on or the snapshot records no
// state (no graph sources, or a file written before B8); the refusal reason
// goes in the parentheses.
func agentViewLine(st *snapshot.AgentViewStatus) string {
	if st == nil {
		return ""
	}
	// A snapshot written before config_path existed, or by a writer that
	// loaded no file, names the default location.
	path := st.ConfigPath
	if path == "" {
		path = filepath.Join(os.Getenv("LOCALAPPDATA"), "burnmon", "burnmon.json")
	}
	switch st.State {
	case snapshot.AgentViewOff:
		return `Agent view off: set "agent_view": true in ` + path + `.`
	case snapshot.AgentViewRefused:
		return `Agent view refused (` + st.Reason + `): set "claude_path" in ` + path + ` to claude.exe.`
	}
	return ""
}

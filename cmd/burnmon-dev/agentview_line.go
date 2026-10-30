//go:build windows

// agentview_line.go is the one line Waits on you shows when the agent view is
// off or refused, with the burnmon.json key that fixes it. The config is read
// once at start, so main computes the line once.
package main

import (
	"burnmon/internal/pricing"
	"burnmon/internal/snapshotwriter"
)

// agentViewLine is empty when the agent view is on and claude resolves the
// way the snapshot writer resolves it (ResolveClaudeConfigured); the refusal
// reason goes in the parentheses.
func agentViewLine(cfg pricing.Config, look func(string) (string, error), goos string) string {
	if !cfg.AgentView {
		return `Agent view off: set "agent_view": true in burnmon.json.`
	}
	if _, err := snapshotwriter.ResolveClaudeConfigured(cfg.ClaudePath, look, goos); err != nil {
		return `Agent view refused (` + err.Error() + `): set "claude_path" in burnmon.json to claude.exe.`
	}
	return ""
}

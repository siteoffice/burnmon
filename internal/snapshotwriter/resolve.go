package snapshotwriter

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ResolveClaude finds the claude binary on PATH and checks it can be started
// directly (see ResolveClaudeFor).
func ResolveClaude() (string, error) { return ResolveClaudeFor(exec.LookPath, runtime.GOOS) }

// ResolveClaudeFor resolves "claude" with look and accepts it only when it can
// be started without a shell in between. On Windows that means a .exe: a .cmd,
// .bat or .ps1 (the npm shim) runs through cmd.exe, so a timeout would kill
// only cmd.exe and leave node and any supervisor running. Elsewhere any
// executable regular file is fine. The error text is shown as the agent_view
// source's reason.
func ResolveClaudeFor(look func(string) (string, error), goos string) (string, error) {
	p, err := look("claude")
	if err != nil {
		return "", fmt.Errorf("claude not found: %w", err)
	}
	if goos == "windows" {
		ext := strings.ToLower(filepath.Ext(p))
		if ext == ".exe" {
			return p, nil
		}
		if ext == "" {
			return "", errors.New("claude has no .exe extension; agent_view needs claude.exe")
		}
		return "", fmt.Errorf("claude is a %s shim; agent_view needs claude.exe", ext)
	}
	fi, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("claude: %w", err)
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm()&0o111 == 0 {
		return "", errors.New("claude is not an executable file; agent_view needs one")
	}
	return p, nil
}

// EnableAgentView turns the agent_view poller on when claude resolves to a
// directly startable binary; otherwise it starts no poller and keeps the
// reason for the agent_view source.
func (gr *GraphReaders) EnableAgentView(look func(string) (string, error)) {
	gr.EnableAgentViewFor(look, runtime.GOOS)
}

// EnableAgentViewFor is EnableAgentView with the OS named, for tests.
func (gr *GraphReaders) EnableAgentViewFor(look func(string) (string, error), goos string) {
	p, err := ResolveClaudeFor(look, goos)
	if err != nil {
		gr.AgentView, gr.agentViewRefused = nil, err
		return
	}
	gr.AgentView, gr.agentViewRefused = ClaudeAgentView(p), nil
}

// ResolveClaudeConfigured returns the claude binary named by claude_path in
// burnmon.json, or, when configured is empty, what ResolveClaudeFor finds.
// A configured path must be absolute, a .exe on Windows, and a regular file
// (not a link).
func ResolveClaudeConfigured(configured string, look func(string) (string, error), goos string) (string, error) {
	if configured == "" {
		return ResolveClaudeFor(look, goos)
	}
	if !filepath.IsAbs(configured) {
		return "", errors.New("claude_path is not absolute")
	}
	if goos == "windows" && strings.ToLower(filepath.Ext(configured)) != ".exe" {
		return "", errors.New("claude_path is not a .exe")
	}
	fi, err := os.Lstat(configured)
	if err != nil || !fi.Mode().IsRegular() {
		return "", errors.New("claude_path is not a file")
	}
	return configured, nil
}

// EnableAgentViewWith is EnableAgentView with the configured claude_path
// taken into account (see ResolveClaudeConfigured).
func (gr *GraphReaders) EnableAgentViewWith(configured string, look func(string) (string, error)) {
	p, err := ResolveClaudeConfigured(configured, look, runtime.GOOS)
	if err != nil {
		gr.AgentView, gr.agentViewRefused = nil, err
		return
	}
	gr.AgentView, gr.agentViewRefused = ClaudeAgentView(p), nil
}

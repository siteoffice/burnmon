package controls

import (
	"errors"
	"regexp"
)

// Control ids. Reply is not built: Claude Code documents no shell command
// that sends a reply to a background session.
const (
	Stop     = "stop"
	Attach   = "attach"
	Approve  = "approve"
	SendBack = "send_back"
)

// CloseSprint runs Foreman's own sprint_done on a sprint whose branch is in main.
const CloseSprint = "close_sprint"

// All lists every control in a stable order.
var All = []string{Stop, Attach, Approve, SendBack, CloseSprint}

var agentIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// ValidAgentID reports whether id is a safe agent view short id: it cannot
// start with a dash, hold a space, a quote or a shell character, or end in a
// newline.
func ValidAgentID(id string) bool {
	return agentIDRe.MatchString(id)
}

func claudeArgv(claudePath, verb, id string) ([]string, error) {
	if claudePath == "" {
		return nil, errors.New("needs claude.exe: set claude_path in burnmon.json")
	}
	if !ValidAgentID(id) {
		return nil, errors.New("not a valid session id")
	}
	return []string{claudePath, verb, id}, nil
}

// StopArgv is the documented "claude stop <id>".
func StopArgv(claudePath, id string) ([]string, error) {
	return claudeArgv(claudePath, "stop", id)
}

// AttachArgv is the documented "claude attach <id>".
func AttachArgv(claudePath, id string) ([]string, error) {
	return claudeArgv(claudePath, "attach", id)
}

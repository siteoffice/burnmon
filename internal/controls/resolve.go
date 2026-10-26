// Package controls runs the first BurnMon Dev controls: it builds the exact
// command for each control, holds the short-lived preview tokens, re-checks
// the target against a fresh read right before it runs, and writes one audit
// line per run. Rules: never a shell (no cmd /c, WMI, CIM or PSExec) and never
// a kill by pid; stopping a session is "claude stop <id>" only. Never-self: a
// control never targets the session or process that runs BurnMon Dev, and it
// refuses a session that works inside BurnMon Dev's folder or in a folder that
// contains it. The second direction implements "the session that asked for the
// build" conservatively, because nothing identifies that session.
package controls

import (
	"errors"
	"path/filepath"
	"strings"
)

// ResolvePython resolves "python" with look and accepts it only as a
// directly startable interpreter. On Windows that means a .exe that is not
// the Microsoft Store alias stub under a WindowsApps folder.
func ResolvePython(look func(string) (string, error), goos string) (string, error) {
	p, err := look("python")
	if err != nil {
		return "", errors.New("python not found")
	}
	if goos == "windows" {
		if strings.ToLower(filepath.Ext(p)) != ".exe" {
			return "", errors.New("python is not a .exe")
		}
		if strings.Contains(strings.ToLower(strings.ReplaceAll(p, "/", `\`)), `\windowsapps\`) {
			return "", errors.New("python is the Store alias")
		}
	}
	return p, nil
}

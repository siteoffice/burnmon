//go:build darwin && cgo

package main

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"

	"burnmon/internal/winbind"
)

// newFolderPicker returns the macOS folder picker: the standard choose
// folder sheet through osascript (AppleScript's "choose folder").
func newFolderPicker(w winbind.Window) func(start string) (string, bool, error) {
	return func(start string) (string, bool, error) {
		cmd := exec.Command("osascript", "-e", appleChooseFolder(start))
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		if err := cmd.Run(); err != nil {
			if strings.Contains(errb.String(), "-128") { // User canceled.
				return "", false, nil
			}
			msg := strings.TrimSpace(errb.String())
			if msg == "" {
				msg = err.Error()
			}
			return "", false, errors.New(msg)
		}
		return osascriptPath(out.String()), true, nil
	}
}

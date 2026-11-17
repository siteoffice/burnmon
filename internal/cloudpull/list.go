// Package cloudpull finds the Cowork (Cloud) sessions BurnMon may copy to this
// computer with the user's own Claude Code CLI.
//
// The list of cloud sessions comes from the Claude desktop app: it writes
// remote-session-spaces.json under local-agent-mode-sessions\<account>\<org>
// when a cloud session starts in the app, inside or outside a Project.
// Sessions started from the phone, the browser or another computer are not in
// that file, and no other local source lists them.
package cloudpull

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// listFile is the desktop app's list of cloud sessions, one per org folder.
const listFile = "remote-session-spaces.json"

// ErrNoList means no session list exists for the org on this computer.
var ErrNoList = errors.New("cloudpull: no cloud session list for this account on this computer")

// orgRe accepts a plain org id only, so it is safe to put into a glob.
var orgRe = regexp.MustCompile(`^[0-9A-Za-z-]+$`)

// DesktopDataRoots returns the Claude desktop app data folders that exist on
// this computer: the folders that hold local-agent-mode-sessions.
func DesktopDataRoots() []string {
	home, _ := os.UserHomeDir()
	return desktopDataRoots(os.Getenv("LOCALAPPDATA"), os.Getenv("APPDATA"), home)
}

func desktopDataRoots(localAppData, appData, home string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			seen[p] = true
			out = append(out, p)
		}
	}
	glob := func(pattern string) {
		hits, _ := filepath.Glob(pattern)
		for _, h := range hits {
			add(h)
		}
	}
	if localAppData != "" {
		// Current Windows builds keep the app data here, one folder per install.
		glob(filepath.Join(localAppData, "Claude-Data", "claude_*"))
		// Packaged (MSIX) install: LocalCache\Roaming and LocalCache\Local.
		glob(filepath.Join(localAppData, "Packages", "Claude_*", "LocalCache", "Roaming", "Claude"))
		glob(filepath.Join(localAppData, "Packages", "Claude_*", "LocalCache", "Local", "Claude"))
		add(filepath.Join(localAppData, "Claude"))
	}
	if appData != "" {
		add(filepath.Join(appData, "Claude"))
	}
	if home != "" {
		add(filepath.Join(home, "Library", "Application Support", "Claude"))
		add(filepath.Join(home, ".config", "Claude"))
	}
	return out
}

// ParseAuthOrg reads the signed-in org id from `claude auth status` output.
// The pull can only copy sessions of that org, so only its list is read.
func ParseAuthOrg(statusJSON []byte) (string, error) {
	var s struct {
		LoggedIn bool   `json:"loggedIn"`
		OrgID    string `json:"orgId"`
	}
	if err := json.Unmarshal(statusJSON, &s); err != nil {
		return "", fmt.Errorf("cloudpull: read claude auth status: %w", err)
	}
	if !s.LoggedIn {
		return "", errors.New("cloudpull: Claude Code is not signed in")
	}
	if s.OrgID == "" {
		return "", errors.New("cloudpull: claude auth status names no org")
	}
	return s.OrgID, nil
}

// SessionList returns the cloud session ids (prefix "session_") in the
// desktop app's list for orgID, in file order, each once. Lists of other orgs
// are never read. ErrNoList when no list for orgID exists under the roots.
func SessionList(roots []string, orgID string) ([]string, error) {
	if !orgRe.MatchString(orgID) {
		return nil, fmt.Errorf("cloudpull: not an org id: %q", orgID)
	}
	var out []string
	seen := map[string]bool{}
	found := false
	for _, root := range roots {
		files, _ := filepath.Glob(filepath.Join(root, "local-agent-mode-sessions", "*", orgID, listFile))
		for _, f := range files {
			raw, err := os.ReadFile(f)
			if err != nil {
				return nil, fmt.Errorf("cloudpull: read %s: %w", f, err)
			}
			var doc struct {
				Entries []struct {
					SessionID string `json:"sessionId"`
				} `json:"entries"`
			}
			if err := json.Unmarshal(raw, &doc); err != nil {
				return nil, fmt.Errorf("cloudpull: parse %s: %w", f, err)
			}
			found = true
			for _, e := range doc.Entries {
				if !strings.HasPrefix(e.SessionID, "session_") || seen[e.SessionID] {
					continue
				}
				seen[e.SessionID] = true
				out = append(out, e.SessionID)
			}
		}
	}
	if !found {
		return nil, ErrNoList
	}
	return out, nil
}

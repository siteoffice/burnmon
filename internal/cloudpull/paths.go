package cloudpull

import (
	"os"
	"path/filepath"
	"strings"
)

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

// PullDir is the folder the CLI runs in for a pull: ~\.burnmon\cloud-pull.
func PullDir() string { return filepath.Join(home(), ".burnmon", "cloud-pull") }

// StatePath is the runner's state file: ~\.burnmon\cloud-pull-state.json.
func StatePath() string { return filepath.Join(home(), ".burnmon", "cloud-pull-state.json") }

// ProjectDir is the Claude project folder the pull transcripts land in. It
// honours CLAUDE_CONFIG_DIR, as the CLI does.
func ProjectDir() string {
	cfg := os.Getenv("CLAUDE_CONFIG_DIR")
	if cfg == "" {
		cfg = filepath.Join(home(), ".claude")
	}
	return filepath.Join(cfg, "projects", ProjectDirName(PullDir()))
}

// MapPullFile tells the Claude parser what a transcript is. isPull: the file
// lies inside the pull project folder, so it is never a Claude Code session.
// mapped: it is a pull transcript the runner recorded, and cloudID is the
// cloud session it copies. A pull file that is not mapped (the runner has not
// saved its state yet, or a deeper file such as a subagent trail) is skipped.
func MapPullFile(path string) (cloudID string, isPull, mapped bool) {
	pd := ProjectDir()
	rel, err := filepath.Rel(pd, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false, false
	}
	if strings.ContainsRune(rel, filepath.Separator) || !strings.EqualFold(filepath.Ext(rel), ".jsonl") {
		return "", true, false
	}
	local := strings.TrimSuffix(rel, filepath.Ext(rel))
	st, err := LoadState(StatePath())
	if err != nil {
		return "", true, false
	}
	for id, e := range st.Sessions {
		if e != nil && e.SessionID == local {
			return id, true, true
		}
	}
	return "", true, false
}

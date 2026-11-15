package centre

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// GhTimeout is the most one gh pr list may take.
const GhTimeout = 20 * time.Second

// Lines the Git page shows when a tool is not there.
const (
	GitMissingLine  = "git is not on PATH, so dirty files and ahead/behind are not shown."
	GhMissingLine   = "gh is not installed, so pull requests are not shown."
	GhSignedOutLine = "gh is not signed in, so pull requests are not shown. Run gh auth login."
)

// GitStatusRunner returns a function that runs GitStatusArgs in a folder
// with GitStatusTimeout, or nil and GitMissingLine when git does not resolve.
func GitStatusRunner(look func(string) (string, error)) (func(dir string) (string, error), string) {
	git, err := look("git")
	if err != nil {
		return nil, GitMissingLine
	}
	return func(dir string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), GitStatusTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, git, GitStatusArgs...)
		cmd.Dir = dir
		hideWindow(cmd)
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		err := cmd.Run()
		if ctx.Err() == context.DeadlineExceeded {
			return "", errors.New("git status took over 5 s")
		}
		if err != nil {
			msg := strings.TrimSpace(errb.String())
			if i := strings.IndexByte(msg, '\n'); i > 0 {
				msg = msg[:i]
			}
			if msg == "" {
				msg = err.Error()
			}
			return "", errors.New(msg)
		}
		return out.String(), nil
	}, ""
}

// ErrGhMissing and ErrGhSignedOut are the gh failures with a fixed line.
var (
	ErrGhMissing   = errors.New(GhMissingLine)
	ErrGhSignedOut = errors.New(GhSignedOutLine)
)

// GhPRRunner returns a function that lists the open pull requests of one
// GitHub repo (owner/name) with gh. gh is looked up on every call, so
// installing it needs no restart.
func GhPRRunner(look func(string) (string, error)) func(slug string) ([]GitPR, error) {
	return func(slug string) ([]GitPR, error) {
		gh, err := look("gh")
		if err != nil {
			return nil, ErrGhMissing
		}
		ctx, cancel := context.WithTimeout(context.Background(), GhTimeout)
		defer cancel()
		args := append(append([]string{}, GhArgs...), "-R", slug)
		cmd := exec.CommandContext(ctx, gh, args...)
		hideWindow(cmd)
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		err = cmd.Run()
		if ctx.Err() == context.DeadlineExceeded {
			return nil, errors.New("gh took over 20 s")
		}
		if err != nil {
			return nil, ghError(errb.String(), err)
		}
		return ParseGhPRs(out.Bytes())
	}
}

func ghError(stderr string, err error) error {
	low := strings.ToLower(stderr)
	if strings.Contains(low, "gh auth login") || strings.Contains(low, "not logged") {
		return ErrGhSignedOut
	}
	msg := strings.TrimSpace(stderr)
	if i := strings.IndexByte(msg, '\n'); i > 0 {
		msg = msg[:i]
	}
	if msg == "" {
		msg = err.Error()
	}
	return errors.New("gh: " + msg)
}

type ghCheck struct {
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
}

type ghPR struct {
	Number            int       `json:"number"`
	Title             string    `json:"title"`
	HeadRefName       string    `json:"headRefName"`
	State             string    `json:"state"`
	IsDraft           bool      `json:"isDraft"`
	ReviewDecision    string    `json:"reviewDecision"`
	StatusCheckRollup []ghCheck `json:"statusCheckRollup"`
	UpdatedAt         string    `json:"updatedAt"`
}

// ParseGhPRs reads gh pr list --json output.
func ParseGhPRs(data []byte) ([]GitPR, error) {
	var raw []ghPR
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, errors.New("gh output did not parse: " + err.Error())
	}
	out := []GitPR{}
	for _, p := range raw {
		out = append(out, GitPR{Number: p.Number, Title: p.Title, Branch: p.HeadRefName, State: p.State,
			Draft: p.IsDraft, Review: p.ReviewDecision, Checks: checksOf(p.StatusCheckRollup), UpdatedAt: p.UpdatedAt})
	}
	return out, nil
}

// checksOf sums up a status check rollup: any failure is "fail", else any
// check still running is "pending", else "pass"; no checks is "".
func checksOf(cs []ghCheck) string {
	if len(cs) == 0 {
		return ""
	}
	pending := false
	for _, c := range cs {
		switch strings.ToUpper(c.Conclusion) {
		case "FAILURE", "ERROR", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE":
			return "fail"
		}
		switch strings.ToUpper(c.State) {
		case "FAILURE", "ERROR":
			return "fail"
		case "PENDING", "EXPECTED":
			pending = true
		}
		switch strings.ToUpper(c.Status) {
		case "QUEUED", "IN_PROGRESS", "PENDING", "WAITING", "REQUESTED":
			pending = true
		}
	}
	if pending {
		return "pending"
	}
	return "pass"
}

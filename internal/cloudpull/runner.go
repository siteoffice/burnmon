package cloudpull

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Timer tiers, by the last usage row of a session's newest pull. Measured and
// decided 2026-11-17 (step 2): one pull costs about 10 s, 8 CPU s and 650 MB,
// whatever the session's size, so the timer counts pulls.
const (
	hotWindow    = 30 * time.Minute
	hotEvery     = 2 * time.Minute
	warmWindow   = 24 * time.Hour
	warmEvery    = 30 * time.Minute
	coldWindow   = 31 * 24 * time.Hour
	coldEvery    = 24 * time.Hour
	retryAfter   = 15 * time.Minute
	tickEvery    = 30 * time.Second
	orgEvery     = 15 * time.Minute
	pullTimeout  = 120 * time.Second
	renameMarker = "/rename burnmon-pull"
)

// ErrCost means a pull started a model turn or cost money. Pulling stops and
// stays off until the state is cleared.
var ErrCost = errors.New("cloudpull: a pull cost tokens")

// ErrDisabled means the cost guard tripped earlier; nothing is pulled.
var ErrDisabled = errors.New("cloudpull: pulling is off after a pull cost tokens")

// Entry is the pull state of one cloud session.
type Entry struct {
	SessionID string    `json:"session_id,omitempty"` // local session id of the newest pull
	Path      string    `json:"path,omitempty"`       // its transcript
	PulledAt  time.Time `json:"pulled_at,omitempty"`
	LastUsage time.Time `json:"last_usage,omitempty"` // newest usage row; zero when it has none
	LastError string    `json:"last_error,omitempty"`
	FailedAt  time.Time `json:"failed_at,omitempty"`
}

// State is the runner's state file. Step 4 maps pull files to cloud ids with it.
type State struct {
	Disabled  string            `json:"disabled,omitempty"`
	LastError string            `json:"last_error,omitempty"`
	Sessions  map[string]*Entry `json:"sessions"`
}

// LoadState reads the state file; a missing file is an empty state.
func LoadState(path string) (*State, error) {
	st := &State{Sessions: map[string]*Entry{}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, st); err != nil {
		return nil, fmt.Errorf("cloudpull: parse %s: %w", path, err)
	}
	if st.Sessions == nil {
		st.Sessions = map[string]*Entry{}
	}
	return st, nil
}

// Save writes the state atomically: a temp file in the same folder, then rename.
func (s *State) Save(path string) error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cloud-pull-state-*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Result is the part of the CLI's JSON result the runner checks.
type Result struct {
	IsError      bool    `json:"is_error"`
	NumTurns     int     `json:"num_turns"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	SessionID    string  `json:"session_id"`
}

// ParseResult checks one pull's JSON result. A model turn or any cost is
// ErrCost; anything else that is not a clean result is a plain failure.
func ParseResult(out []byte) (Result, error) {
	var r Result
	if err := json.Unmarshal(out, &r); err != nil {
		return r, fmt.Errorf("cloudpull: no JSON result from the CLI: %s", firstLine(out))
	}
	if r.NumTurns > 0 || r.TotalCostUSD > 0 {
		return r, fmt.Errorf("%w: num_turns %d, total_cost_usd %g", ErrCost, r.NumTurns, r.TotalCostUSD)
	}
	if r.IsError {
		return r, errors.New("cloudpull: the CLI reported an error")
	}
	if r.SessionID == "" {
		return r, errors.New("cloudpull: the CLI result names no session")
	}
	return r, nil
}

func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// ProjectDirName is the folder name Claude Code gives a working folder under
// projects: every character that is not a letter or digit becomes "-".
func ProjectDirName(path string) string {
	return strings.Map(func(r rune) rune {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return r
		}
		return '-'
	}, path)
}

// ResolveCLI finds the Claude Code executable. For the npm shim (claude.cmd or
// claude.ps1) it returns the claude.exe the shim starts, so no cmd.exe quoting
// is involved.
func ResolveCLI(lookPath func(string) (string, error)) (string, error) {
	p, err := lookPath("claude")
	if err != nil || p == "" {
		return "", errors.New("cloudpull: Claude Code is not installed")
	}
	ext := strings.ToLower(filepath.Ext(p))
	if ext != ".cmd" && ext != ".ps1" {
		return p, nil // a native claude.exe, or claude on macOS and Linux
	}
	exe := filepath.Join(filepath.Dir(p), "node_modules", "@anthropic-ai", "claude-code", "bin", "claude.exe")
	if fi, err := os.Stat(exe); err == nil && !fi.IsDir() {
		return exe, nil
	}
	return "", fmt.Errorf("cloudpull: found the Claude Code shim %s but not the program it starts", p)
}

// LastUsage returns the newest timestamp of a row that carries input_tokens.
// The pull itself adds rows stamped with the pull time, so the last row of a
// file is not the session's last activity.
func LastUsage(path string) (time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()
	var last time.Time
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if !strings.Contains(string(line), `"input_tokens"`) {
			continue
		}
		var row struct {
			Timestamp time.Time `json:"timestamp"`
		}
		if json.Unmarshal(line, &row) != nil {
			continue
		}
		if row.Timestamp.After(last) {
			last = row.Timestamp
		}
	}
	return last.UTC(), sc.Err()
}

// Due returns the cloud ids to pull now, most urgent first: never pulled
// (newest first in list order), then Hot, Warm and Cold, each oldest pull
// first. A failed id waits retryAfter. Archive sessions are never pulled again.
func Due(now time.Time, list []string, st *State) []string {
	type item struct {
		id     string
		tier   int
		pulled time.Time
		pos    int
	}
	var items []item
	for i, id := range list {
		e := st.Sessions[id]
		if e != nil && !e.FailedAt.IsZero() && now.Sub(e.FailedAt) < retryAfter {
			continue
		}
		if e == nil || e.Path == "" {
			items = append(items, item{id: id, tier: 0, pos: -i})
			continue
		}
		since := now.Sub(e.PulledAt)
		age := now.Sub(e.LastUsage)
		switch {
		case e.LastUsage.IsZero():
			// No usage rows yet: check once a day.
			if since >= coldEvery {
				items = append(items, item{id, 3, e.PulledAt, i})
			}
		case age <= hotWindow:
			if since >= hotEvery {
				items = append(items, item{id, 1, e.PulledAt, i})
			}
		case age <= warmWindow:
			if since >= warmEvery {
				items = append(items, item{id, 2, e.PulledAt, i})
			}
		case age <= coldWindow:
			if since >= coldEvery {
				items = append(items, item{id, 3, e.PulledAt, i})
			}
		}
	}
	sort.SliceStable(items, func(a, b int) bool {
		x, y := items[a], items[b]
		if x.tier != y.tier {
			return x.tier < y.tier
		}
		if x.tier == 0 {
			return x.pos < y.pos
		}
		return x.pulled.Before(y.pulled)
	})
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.id
	}
	return out
}

// RunFunc runs the CLI once and returns its standard output.
type RunFunc func(ctx context.Context, exe, dir string, args ...string) ([]byte, error)

// Runner pulls cloud sessions one at a time into its pull folder.
type Runner struct {
	Exe        string
	PullDir    string // the CLI's working folder
	ProjectDir string // where the CLI writes the pull transcripts
	StatePath  string
	Run        RunFunc
	Now        func() time.Time
	Roots      func() []string // desktop data folders; DesktopDataRoots by default
}

// NewRunner builds a Runner for this computer: the CLI on PATH, the pull folder
// ~\.burnmon\cloud-pull and its Claude project folder.
func NewRunner() (*Runner, error) {
	exe, err := ResolveCLI(exec.LookPath)
	if err != nil {
		return nil, err
	}
	if home() == "" {
		return nil, errors.New("cloudpull: no home folder")
	}
	return &Runner{
		Exe:        exe,
		PullDir:    PullDir(),
		ProjectDir: ProjectDir(),
		StatePath:  StatePath(),
		Run:        execRun,
		Now:        time.Now,
		Roots:      DesktopDataRoots,
	}, nil
}

// execRun starts the CLI with a hidden window and a timeout. Never WMI, CIM
// or PSExec: a managed Defender rule blocks agents started that way.
func execRun(ctx context.Context, exe, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, pullTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	hideWindow(cmd)
	return cmd.Output()
}

// PullOne pulls one cloud session, checks the cost guard, records the state
// and deletes that session's previous pull.
func (r *Runner) PullOne(ctx context.Context, cloudID string) error {
	st, err := LoadState(r.StatePath)
	if err != nil {
		return err
	}
	if st.Disabled != "" {
		return ErrDisabled
	}
	if err := os.MkdirAll(r.PullDir, 0o755); err != nil {
		return err
	}
	now := r.Now().UTC()
	out, runErr := r.Run(ctx, r.Exe, r.PullDir,
		"--teleport", cloudID, "-p", renameMarker, "--model", "haiku", "--output-format", "json")
	res, perr := ParseResult(out)
	if errors.Is(perr, ErrCost) {
		st.Disabled = fmt.Sprintf("%s at %s: %v", cloudID, now.Format(time.RFC3339), perr)
		st.LastError = st.Disabled
		if err := st.Save(r.StatePath); err != nil {
			return errors.Join(perr, err)
		}
		return perr
	}
	if perr == nil && runErr != nil {
		perr = fmt.Errorf("cloudpull: the CLI exited with %v", runErr)
	}
	e := st.Sessions[cloudID]
	if e == nil {
		e = &Entry{}
		st.Sessions[cloudID] = e
	}
	if perr != nil {
		e.LastError, e.FailedAt = perr.Error(), now
		st.LastError = perr.Error()
		return errors.Join(perr, st.Save(r.StatePath))
	}
	newPath := filepath.Join(r.ProjectDir, res.SessionID+".jsonl")
	last, lerr := LastUsage(newPath)
	if lerr != nil {
		e.LastError, e.FailedAt = lerr.Error(), now
		st.LastError = lerr.Error()
		return errors.Join(lerr, st.Save(r.StatePath))
	}
	old := *e
	*e = Entry{SessionID: res.SessionID, Path: newPath, PulledAt: now, LastUsage: last}
	st.LastError = ""
	if err := st.Save(r.StatePath); err != nil {
		return err
	}
	if old.SessionID != "" && old.SessionID != res.SessionID {
		r.removeOld(old.SessionID)
	}
	return nil
}

// removeOld deletes an earlier pull's transcript and folder, only when both
// resolve inside the project folder. Nothing else is ever deleted.
func (r *Runner) removeOld(sessionID string) {
	for _, p := range []string{
		filepath.Join(r.ProjectDir, sessionID+".jsonl"),
		filepath.Join(r.ProjectDir, sessionID),
	} {
		rel, err := filepath.Rel(r.ProjectDir, p)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
			strings.ContainsRune(rel, filepath.Separator) {
			continue
		}
		os.RemoveAll(p)
	}
}

// Tick runs at most one pull: the most urgent due id. It returns that id, or
// "" when nothing was due.
func (r *Runner) Tick(ctx context.Context, list []string) (string, error) {
	st, err := LoadState(r.StatePath)
	if err != nil {
		return "", err
	}
	if st.Disabled != "" {
		return "", ErrDisabled
	}
	due := Due(r.Now().UTC(), list, st)
	if len(due) == 0 {
		return "", nil
	}
	return due[0], r.PullOne(ctx, due[0])
}

// Loop ticks every 30 s until ctx ends or the cost guard trips. It re-reads
// the signed-in org at most every 15 minutes and the session list every tick.
// Errors that do not stop the loop are recorded in the state file and passed
// to onErr.
func (r *Runner) Loop(ctx context.Context, onErr func(error)) error {
	roots := r.Roots
	if roots == nil {
		roots = DesktopDataRoots
	}
	var org string
	var orgAt time.Time
	report := func(err error) {
		if st, lerr := LoadState(r.StatePath); lerr == nil {
			st.LastError = err.Error()
			st.Save(r.StatePath)
		}
		if onErr != nil {
			onErr(err)
		}
	}
	t := time.NewTicker(tickEvery)
	defer t.Stop()
	for {
		now := r.Now()
		if org == "" || now.Sub(orgAt) >= orgEvery {
			out, _ := r.Run(ctx, r.Exe, r.PullDir, "auth", "status")
			o, err := ParseAuthOrg(out)
			if err != nil {
				report(err)
			} else {
				org, orgAt = o, now
			}
		}
		if org != "" {
			list, err := SessionList(roots(), org)
			if err == nil {
				_, err = r.Tick(ctx, list)
			}
			if errors.Is(err, ErrCost) || errors.Is(err, ErrDisabled) {
				return err
			}
			if err != nil {
				report(err)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

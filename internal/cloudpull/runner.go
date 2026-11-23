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
	"regexp"
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
	retryMax     = 24 * time.Hour
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
	Failures  int       `json:"failures,omitempty"` // failed pulls in a row; a good pull resets it
	Gone      bool      `json:"gone,omitempty"`     // the CLI said "Session not found": never pulled again
	// NeedsCheckout marks a session the CLI will only load from a checkout of its
	// repo. That is the session's own state, retried once a day, and it does not
	// feed the circuit breaker.
	NeedsCheckout bool `json:"needs_checkout,omitempty"`
	// Unloadable marks a session the API does not find for this login ("Couldn't
	// load the history of cloud session ..."). Retried once a day, outside the breaker.
	Unloadable bool `json:"unloadable,omitempty"`
}

// notFoundMsg is the CLI's stderr when the teleport-events call answers not found.
const notFoundMsg = "Couldn't load the history of cloud session"

// State is the runner's state file. Step 4 maps pull files to cloud ids with it.
type State struct {
	Disabled  string `json:"disabled,omitempty"`
	LastError string `json:"last_error,omitempty"`
	// PausedUntil and PauseReason are the circuit breaker: breakerSessions
	// different sessions in a row failed with the same error, so nothing is
	// pulled until PausedUntil. Streak is the run being counted.
	PausedUntil time.Time         `json:"paused_until,omitempty"`
	PauseReason string            `json:"pause_reason,omitempty"`
	Streak      *Streak           `json:"streak,omitempty"`
	Sessions    map[string]*Entry `json:"sessions"`
}

// Streak is a run of failures with one error text (session id removed) from
// different sessions.
type Streak struct {
	Error string   `json:"error"`
	IDs   []string `json:"ids"`
}

const (
	breakerSessions = 5
	breakerPause    = 6 * time.Hour
	checkoutRetry   = 24 * time.Hour
)

// statusNow is the clock ReadStatus uses; a var so tests can fix it.
var statusNow = time.Now

// errCLIReported marks a result the CLI itself flagged as an error, at no cost.
var errCLIReported = errors.New("cloudpull: the CLI reported an error")

var checkoutRe = regexp.MustCompile(`from a checkout of (\S+?)\.?$`)

// checkoutRepo returns the repo a failure says the session needs a checkout of.
func checkoutRepo(msg string) (string, bool) {
	if m := checkoutRe.FindStringSubmatch(strings.TrimSpace(msg)); m != nil {
		return m[1], true
	}
	return "", false
}

// noteFailure counts one failed pull of id toward the breaker. The same error
// text (session id removed) from breakerSessions different sessions in a row
// pauses pulling; a different error restarts the count.
func (s *State) noteFailure(id, msg string, now time.Time) {
	norm := strings.ReplaceAll(msg, id, "<id>")
	if s.Streak == nil || s.Streak.Error != norm {
		s.Streak = &Streak{Error: norm}
	}
	for _, x := range s.Streak.IDs {
		if x == id {
			return
		}
	}
	s.Streak.IDs = append(s.Streak.IDs, id)
	if len(s.Streak.IDs) >= breakerSessions {
		s.PausedUntil = now.Add(breakerPause)
		s.PauseReason = strings.TrimPrefix(norm, "cloudpull: ")
		s.Streak = nil
	}
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
	Result       string  `json:"result"`
}

// ParseResult checks one pull's JSON result. A model turn that is not an
// error, or any cost, is ErrCost; anything else that is not a clean result is a
// plain failure. An error result at no cost is a plain failure that keeps the
// CLI's words: a session too long for the rename turn ("Prompt is too long")
// reports one turn and cost 0 (measured 2026-11-23), and nothing was spent.
func ParseResult(out []byte) (Result, error) {
	var r Result
	if err := json.Unmarshal(out, &r); err != nil {
		return r, fmt.Errorf("cloudpull: no JSON result from the CLI: %s", firstLine(out))
	}
	if r.TotalCostUSD > 0 || (r.NumTurns > 0 && !r.IsError) {
		return r, fmt.Errorf("%w: num_turns %d, total_cost_usd %g", ErrCost, r.NumTurns, r.TotalCostUSD)
	}
	if r.IsError {
		if r.Result != "" {
			return r, fmt.Errorf("%w: %s", errCLIReported, firstLine([]byte(r.Result)))
		}
		return r, errCLIReported
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
			Message   struct {
				Model string `json:"model"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &row) != nil {
			continue
		}
		// The CLI's own synthetic rows (a failed rename turn, zero tokens) carry the pull time.
		if row.Message.Model == "<synthetic>" {
			continue
		}
		if row.Timestamp.After(last) {
			last = row.Timestamp
		}
	}
	return last.UTC(), sc.Err()
}

// retryDelay is how long an id waits after its n-th failure in a row: 15
// minutes, doubling each time, at most one day.
func retryDelay(failures int) time.Duration {
	d := retryAfter
	for i := 1; i < failures && d < retryMax; i++ {
		d *= 2
	}
	if d > retryMax {
		d = retryMax
	}
	return d
}

// sessionError marks a failure of one session's pull. It is not a runner-wide
// error: Step does not record it as the state's last_error.
type sessionError struct{ err error }

func (s sessionError) Error() string { return s.err.Error() }
func (s sessionError) Unwrap() error { return s.err }

// Due returns the cloud ids to pull now, most urgent first: Hot (so an active
// session never waits behind the first run), then never pulled (newest first
// in list order), then Warm and Cold, each oldest pull first. A failed id
// waits retryDelay and, once due, goes after every other id (oldest failure
// first), so failing ids never hold back the re-pulls. Gone and Archive
// sessions are never pulled again.
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
		if e != nil && e.Gone {
			continue
		}
		if e != nil && !e.FailedAt.IsZero() {
			wait := retryDelay(e.Failures)
			if e.NeedsCheckout || e.Unloadable {
				wait = checkoutRetry
			}
			if now.Sub(e.FailedAt) < wait {
				continue
			}
		}
		if e == nil || e.Path == "" {
			items = append(items, item{id: id, tier: 1, pos: -i})
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
				items = append(items, item{id, 0, e.PulledAt, i})
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
	for i := range items {
		if e := st.Sessions[items[i].id]; e != nil && !e.FailedAt.IsZero() {
			items[i].tier, items[i].pulled = 4, e.FailedAt
		}
	}
	sort.SliceStable(items, func(a, b int) bool {
		x, y := items[a], items[b]
		if x.tier != y.tier {
			return x.tier < y.tier
		}
		if x.tier == 1 {
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

	org   string // signed-in org, refreshed every orgEvery
	orgAt time.Time
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
	out, err := cmd.Output()
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		// The CLI explains a failure on stderr ("Session not found: ...").
		err = fmt.Errorf("%w: %s", err, firstLine(ee.Stderr))
	}
	return out, err
}

// sessionGone reports whether a failed pull means the cloud session no longer
// exists (measured 2026-11-17: exit 1, "Session not found: <id>" on stderr).
func sessionGone(out []byte, runErr error) bool {
	const msg = "Session not found"
	return strings.Contains(string(out), msg) || (runErr != nil && strings.Contains(runErr.Error(), msg))
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
	// The CLI writes the whole copy before its rename turn. When that turn fails at no
	// cost ("Prompt is too long" for a big history, measured 2026-11-23: 258 rows
	// on disk, 86 with usage), the copy is still good and counts.
	copied := false
	if errors.Is(perr, errCLIReported) && res.SessionID != "" {
		if fi, err := os.Stat(filepath.Join(r.ProjectDir, res.SessionID+".jsonl")); err == nil && fi.Size() > 0 {
			copied, perr = true, nil
		}
	}
	if !copied && runErr != nil && !errors.Is(perr, ErrCost) && !errors.Is(perr, errCLIReported) {
		// The CLI's own words (first line of its error stream) and its exit code,
		// in place of an empty "no JSON result" (measured 2026-11-20).
		perr = fmt.Errorf("cloudpull: the CLI exited with %v", runErr)
		if o := firstLine(out); o != "" {
			perr = fmt.Errorf("%w (output: %s)", perr, o)
		}
	}
	e := st.Sessions[cloudID]
	if e == nil {
		e = &Entry{}
		st.Sessions[cloudID] = e
	}
	if perr != nil && sessionGone(out, runErr) {
		e.Gone, e.LastError, e.FailedAt = true, "session not found", now
		return st.Save(r.StatePath)
	}
	// A failing session is that session's state, not the runner's: st.LastError
	// stays for errors that stop every pull (not signed in, no CLI).
	if perr != nil {
		e.LastError, e.FailedAt, e.Failures = perr.Error(), now, e.Failures+1
		if repo, ok := checkoutRepo(perr.Error()); ok {
			// The session's own state: its own words, a daily retry, no breaker.
			e.LastError, e.NeedsCheckout, e.Unloadable = "cloudpull: needs a checkout of "+repo, true, false
		} else if strings.Contains(perr.Error(), notFoundMsg) {
			// The API says not found for this session under this login (the browser may
			// still open it). That is the session's own state, not a runner fault: a
			// daily retry, and no breaker, which would starve the sessions that load.
			e.NeedsCheckout, e.Unloadable = false, true
		} else {
			e.NeedsCheckout, e.Unloadable = false, false
			st.noteFailure(cloudID, perr.Error(), now)
		}
		return errors.Join(sessionError{perr}, st.Save(r.StatePath))
	}
	st.Streak = nil
	newPath := filepath.Join(r.ProjectDir, res.SessionID+".jsonl")
	last, lerr := LastUsage(newPath)
	if lerr != nil {
		e.LastError, e.FailedAt, e.Failures = lerr.Error(), now, e.Failures+1
		return errors.Join(sessionError{lerr}, st.Save(r.StatePath))
	}
	old := *e
	*e = Entry{SessionID: res.SessionID, Path: newPath, PulledAt: now, LastUsage: last}
	st.LastError = ""
	if err := st.Save(r.StatePath); err != nil {
		return err
	}
	// The file watcher saw the transcript while the CLI wrote it, before this
	// mapping existed, and the parser skipped it then. Moving its modified
	// time now gives the watcher a second event, and the file is read as the
	// cloud session (measured 2026-11-17: without it a pull stayed unread
	// until the next app start).
	if err := os.Chtimes(newPath, now, now); err != nil {
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
	now := r.Now().UTC()
	if now.Before(st.PausedUntil) {
		return "", nil // the breaker is open; the screen says why
	}
	due := Due(now, list, st)
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
	t := time.NewTicker(tickEvery)
	defer t.Stop()
	for {
		err := r.Step(ctx)
		if errors.Is(err, ErrCost) || errors.Is(err, ErrDisabled) {
			return err
		}
		if err != nil && onErr != nil {
			onErr(err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// Step is one pass of the loop: refresh the signed-in org when due, read the
// session list, and run at most one pull. An error other than ErrCost and
// ErrDisabled is recorded in the state file, where the screen reads it.
func (r *Runner) Step(ctx context.Context) error {
	roots := r.Roots
	if roots == nil {
		roots = DesktopDataRoots
	}
	now := r.Now()
	if r.org == "" || now.Sub(r.orgAt) >= orgEvery {
		out, _ := r.Run(ctx, r.Exe, r.PullDir, "auth", "status")
		o, err := ParseAuthOrg(out)
		if err != nil {
			recordError(r.StatePath, err)
			return err
		}
		r.org, r.orgAt = o, now
	}
	list, err := SessionList(roots(), r.org)
	if err == nil {
		_, err = r.Tick(ctx, list)
	}
	var se sessionError
	if err != nil && !errors.Is(err, ErrCost) && !errors.Is(err, ErrDisabled) && !errors.As(err, &se) {
		recordError(r.StatePath, err)
	}
	return err
}

// recordError stores a runner-wide error for the screen.
func recordError(statePath string, err error) {
	if st, lerr := LoadState(statePath); lerr == nil {
		st.LastError = err.Error()
		st.Save(statePath)
	}
}

// Status is what the screen shows about the pull. OK means at least one
// session is read and nothing stops the pull; otherwise Reason says why it is
// not measured. Unread counts the sessions whose last pull failed (also those
// with an older copy) and UnreadReason is the newest such failure.
type Status struct {
	OK           bool
	Reason       string
	Unread       int
	UnreadReason string
	// LastPulledAt is the newest good copy of any session. A period that starts
	// after it holds no measured cloud usage, whatever its number says.
	LastPulledAt time.Time
	// Reasons counts the unread sessions per reason, largest first. Example is the
	// newest failed session of that reason.
	Reasons []Reason
}

// The kinds of Reason.
const (
	ReasonNotFound = "not_found" // the API does not find the session for this login
	ReasonCheckout = "checkout"  // the CLI wants a checkout of the session's repo
	ReasonTooLong  = "too_long"  // the history is too long for the CLI to open, and no copy was written
	ReasonOther    = "other"
)

// Reason is how many unread sessions failed for one reason.
type Reason struct {
	Kind    string
	N       int
	Example string // the newest failed session id of this kind
	Text    string // the error text, for ReasonOther only
}

// Plain is the reason in plain words, with its count: "5 are not found by ...".
func (r Reason) Plain() string {
	one := r.N == 1
	pick := func(a, b string) string {
		if one {
			return a
		}
		return b
	}
	n := fmt.Sprintf("%d ", r.N)
	switch r.Kind {
	case ReasonNotFound:
		return n + pick("is", "are") + " not found by the Claude Code service for your login"
	case ReasonCheckout:
		return n + pick("needs", "need") + " a checkout of their repo"
	case ReasonTooLong:
		return n + pick("is", "are") + " too long for the CLI to open"
	}
	return n + "failed: " + r.Text
}

// reasonOf classifies one unread session by what its last pull said.
func reasonOf(e *Entry) (kind, text string) {
	msg := strings.TrimPrefix(e.LastError, "cloudpull: ")
	switch {
	case e.NeedsCheckout:
		return ReasonCheckout, ""
	case e.Unloadable || strings.Contains(msg, notFoundMsg):
		return ReasonNotFound, ""
	case strings.Contains(msg, "Prompt is too long"):
		return ReasonTooLong, ""
	}
	msg = strings.TrimPrefix(msg, "the CLI exited with ")
	if r := []rune(msg); len(r) > 90 {
		msg = string(r[:90])
	}
	return ReasonOther, msg
}

// ReadStatus reads the state file for the screen.
func ReadStatus(statePath string) Status {
	st, err := LoadState(statePath)
	if err != nil {
		return Status{Reason: err.Error()}
	}
	if st.Disabled != "" {
		return Status{Reason: "pulling stopped because a pull cost tokens (" + st.Disabled + "); switch it off and on to start again"}
	}
	if st.LastError != "" {
		return Status{Reason: strings.TrimPrefix(st.LastError, "cloudpull: ")}
	}
	read, unread := 0, 0
	var newest *Entry
	var lastPulled time.Time
	byKind := map[string]*Reason{}
	newestOf := map[string]time.Time{}
	for id, e := range st.Sessions {
		if e == nil {
			continue
		}
		if e.Path != "" {
			read++
			if e.PulledAt.After(lastPulled) {
				lastPulled = e.PulledAt
			}
		}
		if !e.Gone && !e.FailedAt.IsZero() {
			unread++
			kind, text := reasonOf(e)
			br := byKind[kind]
			if br == nil {
				br = &Reason{Kind: kind}
				byKind[kind] = br
			}
			br.N++
			if e.FailedAt.After(newestOf[kind]) {
				newestOf[kind], br.Example, br.Text = e.FailedAt, id, text
			}
			if newest == nil || e.FailedAt.After(newest.FailedAt) {
				newest = e
			}
		}
	}
	reason := ""
	if newest != nil {
		reason = strings.TrimPrefix(newest.LastError, "cloudpull: ")
	}
	if statusNow().Before(st.PausedUntil) {
		reason = "pulling paused until " + st.PausedUntil.In(time.Local).Format("15:04") + ": " + st.PauseReason
	}
	if read == 0 {
		if reason == "" {
			reason = "no cloud session copied yet"
		}
		return Status{Reason: reason}
	}
	var reasons []Reason
	for _, br := range byKind {
		reasons = append(reasons, *br)
	}
	sort.Slice(reasons, func(i, j int) bool {
		if reasons[i].N != reasons[j].N {
			return reasons[i].N > reasons[j].N
		}
		return reasons[i].Kind < reasons[j].Kind
	})
	return Status{OK: true, Unread: unread, UnreadReason: reason, LastPulledAt: lastPulled, Reasons: reasons}
}

// ClearDisabled lifts a cost-guard stop and a runner-wide error, for when the
// switch goes off: switching on again starts clean.
func ClearDisabled(statePath string) error {
	st, err := LoadState(statePath)
	if err != nil {
		return err
	}
	if st.Disabled == "" && st.LastError == "" {
		return nil
	}
	st.Disabled, st.LastError = "", ""
	return st.Save(statePath)
}

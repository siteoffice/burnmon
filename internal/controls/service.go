package controls

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"burnmon/internal/graph"
	"burnmon/internal/snapshot"
	"burnmon/internal/waits"
)

// Env is everything the Service depends on. Every function is injected so a
// test never touches a real session, process or flow. A nil Self, Elevated,
// Switches or Runner turns all controls off.
type Env struct {
	Now         func() time.Time
	ClaudePath  string // resolved, "" when unavailable
	ClaudeWhy   string
	PythonPath  string
	PythonWhy   string
	AgentViewOn bool
	ForemanRoot string
	AgentView   func(ctx context.Context) ([]byte, error) // fresh read
	ReadForeman func(root string) ([]graph.Sprint, []graph.Flow, error)
	Snapshot    func() snapshot.Reading
	Switches    func() map[string]bool
	Self        func() (Self, error)
	Elevated    func() bool
	Runner      Runner
	AuditPath   string
	// Merged says whether branch is in the main branch of repo. known is false
	// when it cannot be told; the Close control then refuses.
	Merged func(repo, branch string) (merged, known bool)
}

// Service ties previews, fresh checks, the audit file and the runner together.
type Service struct {
	Env       Env
	previews  Previews
	runMu     sync.Mutex
	previewMu sync.Mutex                                // at most one `claude agents --json` at a time
	openAudit func(path string) (io.WriteCloser, error) // tests replace it
}

// NewService builds a Service. env.Now also drives the preview clock.
func NewService(env Env) *Service {
	s := &Service{Env: env, openAudit: openAuditFile}
	s.previews.Now = s.now
	return s
}

func (s *Service) now() time.Time {
	if s.Env.Now != nil {
		return s.Env.Now()
	}
	return time.Now()
}

// PreviewRequest names a control and its target: an agent id, or a gate wait
// item id.
type PreviewRequest struct {
	Control string `json:"control"`
	Target  string `json:"target"`
}

// PreviewResult is what the page shows after the first press.
type PreviewResult struct {
	Token       string `json:"token"`
	Command     string `json:"command"`
	Label       string `json:"label"`
	ExpiresInMs int    `json:"expires_in_ms"`
}

// RunResult is what the page shows after the second press. Failed is true for
// any nonzero exit and for any runner error; a refusal has Refused set.
type RunResult struct {
	ExitCode int    `json:"exit_code"`
	Line     string `json:"line,omitempty"`
	Refused  string `json:"refused,omitempty"`
	Failed   bool   `json:"failed,omitempty"`
}

// Availability says which controls may be pressed, and why not otherwise.
type Availability struct {
	Ready map[string]bool   `json:"ready"`
	Why   map[string]string `json:"why,omitempty"`
	// Foreman is true when foreman_root is set: the Foreman parts exist.
	Foreman bool `json:"foreman"`
}

const (
	whyElevated = "BurnMon Dev runs as administrator: controls are off"
	whyOff      = "switched off in the gear"
	whyClaude   = "needs claude.exe: set claude_path in burnmon.json"
	whyPython   = "needs python.exe on PATH"

	runStopTimeout  = 20 * time.Second
	runGateTimeout  = 30 * time.Second
	freshReadTimout = 10 * time.Second
)

func isGate(c string) bool { return c == Approve || c == SendBack }

func libDirOf(root string) string { return filepath.Join(root, "flows", "_lib") }

// assess works out, once, which controls are usable now and the Self that
// the decision used. A Self that came with an error is never returned.
func (s *Service) assess() (Availability, Self) {
	a := Availability{Ready: map[string]bool{}, Why: map[string]string{}, Foreman: s.Env.ForemanRoot != ""}
	var self Self
	global := ""
	switch {
	case s.Env.Elevated == nil:
		global = "cannot tell whether BurnMon Dev is elevated: controls are off"
	case s.Env.Elevated():
		global = whyElevated
	case s.Env.Self == nil:
		global = "cannot check BurnMon Dev's own process tree: controls are off"
	case s.Env.Runner == nil:
		global = "no runner: controls are off"
	}
	if global == "" {
		sf, err := s.Env.Self()
		if err != nil {
			global = "cannot check BurnMon Dev's own process tree: " + err.Error()
		} else {
			self = sf
		}
	}
	var sw map[string]bool
	if s.Env.Switches != nil {
		sw = s.Env.Switches()
	}
	for _, c := range All {
		why := global
		if why == "" {
			why = s.unusable(c)
		}
		if why == "" && !sw[c] {
			why = whyOff
		}
		a.Ready[c] = why == ""
		if why != "" {
			a.Why[c] = why
		}
	}
	return a, self
}

func (s *Service) unusable(c string) string {
	if c == CloseSprint {
		return s.unusableClose()
	}
	if !isGate(c) {
		switch {
		case s.Env.ClaudePath == "":
			if s.Env.ClaudeWhy != "" {
				return s.Env.ClaudeWhy
			}
			return whyClaude
		case !s.Env.AgentViewOn:
			return "needs agent_view on in burnmon.json"
		}
		return ""
	}
	switch {
	case s.Env.PythonPath == "":
		if s.Env.PythonWhy != "" {
			return s.Env.PythonWhy
		}
		return whyPython
	case s.Env.ForemanRoot == "":
		return "needs foreman_root in burnmon.json"
	}
	fi, err := os.Lstat(filepath.Join(libDirOf(s.Env.ForemanRoot), "flow_transition.py"))
	if err != nil || !fi.Mode().IsRegular() {
		return "needs flows\\_lib\\flow_transition.py under foreman_root"
	}
	return ""
}

// unusableClose says why the Close control cannot be used, "" when it can.
func (s *Service) unusableClose() string {
	switch {
	case s.Env.ForemanRoot == "":
		return "needs foreman_root in burnmon.json"
	case s.Env.PythonPath == "":
		if s.Env.PythonWhy != "" {
			return s.Env.PythonWhy
		}
		return whyPython
	}
	fi, err := os.Lstat(SprintScript(s.Env.ForemanRoot))
	if err != nil || !fi.Mode().IsRegular() {
		return "needs sprints\\_lib\\sprint_step.py under foreman_root"
	}
	return ""
}

// Availability reports which controls are ready: switch on and usable.
func (s *Service) Availability() Availability {
	a, _ := s.assess()
	return a
}

func known(c string) bool {
	for _, k := range All {
		if k == c {
			return true
		}
	}
	return false
}

func insideFlows(root, dir string) bool { return insideDir(filepath.Join(root, "flows"), dir) }

// insideDir reports whether dir is an absolute path strictly inside base.
func insideDir(base, dir string) bool {
	if dir == "" || !filepath.IsAbs(dir) {
		return false
	}
	rel, err := filepath.Rel(base, dir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return false
	}
	return true
}

func (s *Service) freshAgents() ([]waits.AgentEntry, error) {
	if s.Env.AgentView == nil {
		return nil, errors.New("cannot read the agent view")
	}
	ctx, cancel := context.WithTimeout(context.Background(), freshReadTimout)
	defer cancel()
	b, err := s.Env.AgentView(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot read the agent view: %w", err)
	}
	fresh, err := waits.ParseAgentView(b)
	if err != nil {
		return nil, fmt.Errorf("cannot read the agent view: %w", err)
	}
	return fresh, nil
}

func (s *Service) freshSprints() ([]graph.Sprint, error) {
	if s.Env.ReadForeman == nil {
		return nil, errors.New("cannot read Foreman")
	}
	sprints, _, err := s.Env.ReadForeman(s.Env.ForemanRoot)
	if err != nil {
		return nil, fmt.Errorf("cannot read Foreman: %w", err)
	}
	return sprints, nil
}

func (s *Service) freshFlows() ([]graph.Flow, error) {
	if s.Env.ReadForeman == nil {
		return nil, errors.New("cannot read Foreman")
	}
	_, flows, err := s.Env.ReadForeman(s.Env.ForemanRoot)
	if err != nil {
		return nil, fmt.Errorf("cannot read Foreman: %w", err)
	}
	return flows, nil
}

// Preview checks the target against the snapshot and a fresh read, keeps the
// argv server side and returns the exact command. It writes nothing.
func (s *Service) Preview(req PreviewRequest) (PreviewResult, error) {
	if !known(req.Control) {
		return PreviewResult{}, errors.New("unknown control")
	}
	a, self := s.assess()
	if !a.Ready[req.Control] {
		return PreviewResult{}, errors.New(a.Why[req.Control])
	}
	if s.Env.Snapshot == nil {
		return PreviewResult{}, errors.New("no snapshot")
	}
	rd := s.Env.Snapshot()
	if rd.Status != snapshot.StatusOK || rd.Snapshot == nil {
		return PreviewResult{}, errors.New("the snapshot is not current, nothing to control")
	}
	var (
		t    Target
		argv []string
		err  error
	)
	if isGate(req.Control) {
		t, argv, err = s.previewGate(req, rd.Snapshot)
	} else if req.Control == CloseSprint {
		t, argv, err = s.previewSprint(req, rd.Snapshot)
	} else {
		t, argv, err = s.previewAgent(req, rd.Snapshot, self)
	}
	if err != nil {
		return PreviewResult{}, err
	}
	pe := s.previews.Add(t, argv)
	return PreviewResult{Token: pe.Token, Command: pe.Command, Label: t.Label(), ExpiresInMs: int(PreviewTTL / time.Millisecond)}, nil
}

func (s *Service) previewAgent(req PreviewRequest, snap *snapshot.Snapshot, self Self) (Target, []string, error) {
	if !ValidAgentID(req.Target) {
		return Target{}, nil, errors.New("not a valid session id")
	}
	var as *snapshot.AgentSession
	for i := range snap.AgentSessions {
		if snap.AgentSessions[i].ID == req.Target && snap.AgentSessions[i].Kind == "background" {
			as = &snap.AgentSessions[i]
			break
		}
	}
	if as == nil {
		return Target{}, nil, errors.New("the session is not a listed background session")
	}
	// The snapshot has no cwd: only the pid part of never-self is decided
	// here, the cwd part runs in CheckAgent below and again at run.
	if err := self.Refuses(as.PID, ""); err != nil && err.Error() != errCwdUnsure {
		return Target{}, nil, err
	}
	var argv []string
	var err error
	if req.Control == Stop {
		argv, err = StopArgv(s.Env.ClaudePath, as.ID)
	} else {
		argv, err = AttachArgv(s.Env.ClaudePath, as.ID)
	}
	if err != nil {
		return Target{}, nil, err
	}
	// The snapshot is only a pointer: its project name is never shown. The
	// label comes from the fresh entry.
	t := Target{Control: req.Control, AgentID: as.ID, PID: as.PID, StartedAt: as.StartedAt}
	if !s.previewMu.TryLock() {
		return Target{}, nil, errors.New("another preview is running, try again")
	}
	fresh, err := func() ([]waits.AgentEntry, error) {
		defer s.previewMu.Unlock()
		return s.freshAgents()
	}()
	if err != nil {
		return Target{}, nil, err
	}
	e, err := CheckAgent(t, fresh, self)
	if err != nil {
		return Target{}, nil, err
	}
	t.Project = lastFolder(e.Cwd)
	return t, argv, nil
}

func (s *Service) previewGate(req PreviewRequest, snap *snapshot.Snapshot) (Target, []string, error) {
	var item *waits.Item
	for i := range snap.WaitItems {
		if snap.WaitItems[i].Kind == "gate" && snap.WaitItems[i].ID == req.Target {
			item = &snap.WaitItems[i]
			break
		}
	}
	if item == nil {
		return Target{}, nil, errors.New("the gate is not a listed wait item")
	}
	rest := strings.TrimPrefix(item.ID, "gate:")
	i := strings.LastIndex(rest, ":")
	if i <= 0 || i == len(rest)-1 {
		return Target{}, nil, errors.New("not a valid gate id")
	}
	flowID, gate := rest[:i], rest[i+1:]
	verdict, err := VerdictFor(req.Control, gate)
	if err != nil {
		return Target{}, nil, err
	}
	flows, err := s.freshFlows()
	if err != nil {
		return Target{}, nil, err
	}
	matches := 0
	for _, f := range flows {
		if f.ID == flowID {
			matches++
		}
	}
	if matches > 1 {
		return Target{}, nil, errors.New(errDupFlow)
	}
	for _, f := range flows {
		if f.ID != flowID {
			continue
		}
		if !f.GatePending || f.Gate != gate || f.GateToken == "" {
			return Target{}, nil, errors.New("the gate is not open for a decision")
		}
		if !insideFlows(s.Env.ForemanRoot, f.Dir) {
			return Target{}, nil, errors.New("the flow folder is outside foreman_root\\flows")
		}
		t := Target{Control: req.Control, FlowID: f.ID, Gate: gate, GateToken: f.GateToken, FlowDir: f.Dir, Title: flowLabel(f)}
		if err := CheckGate(t, flows); err != nil {
			return Target{}, nil, err
		}
		return t, GateArgv(s.Env.PythonPath, libDirOf(s.Env.ForemanRoot), f.Dir, verdict, gate, f.GateToken), nil
	}
	return Target{}, nil, errors.New("the flow is no longer listed")
}

// previewSprint checks a close-out item against a fresh Foreman read and a fresh
// merge check, and builds the close argv from the fresh values only.
func (s *Service) previewSprint(req PreviewRequest, snap *snapshot.Snapshot) (Target, []string, error) {
	var item *waits.Item
	for i := range snap.WaitItems {
		w := &snap.WaitItems[i]
		if w.Kind == waits.KindCloseOut && w.ID == req.Target && w.SprintID != "" {
			item = w
			break
		}
	}
	if item == nil {
		return Target{}, nil, errors.New("the sprint is not a listed close-out item")
	}
	sprints, err := s.freshSprints()
	if err != nil {
		return Target{}, nil, err
	}
	t := Target{Control: CloseSprint, SprintID: item.SprintID}
	for _, f := range sprints {
		if f.ID == item.SprintID {
			t.SprintDir, t.Branch, t.Title = f.Dir, f.Branch, f.Slug
			if t.Title == "" {
				t.Title = f.ID
			}
		}
	}
	if err := CheckSprint(t, sprints, s.Env.Merged, s.Env.ForemanRoot); err != nil {
		return Target{}, nil, err
	}
	return t, CloseArgv(s.Env.PythonPath, s.Env.ForemanRoot, t.SprintDir), nil
}

// flowLabel names a flow from the fresh read: its slug, else its id.
func flowLabel(f graph.Flow) string {
	if f.Slug != "" {
		return f.Slug
	}
	return f.ID
}

// lastFolder is the last folder name of a Windows or slash path.
func lastFolder(p string) string {
	p = strings.TrimRight(strings.ReplaceAll(p, "/", `\`), `\`)
	if i := strings.LastIndex(p, `\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func auditTarget(t Target) string {
	if t.SprintID != "" {
		return "sprint:" + t.SprintID
	}
	if t.AgentID != "" {
		return t.AgentID
	}
	return "gate:" + t.FlowID + ":" + t.Gate
}

// auditCommand is the previewed command with the Foreman gate token (the last
// argument of a gate command) left out.
func auditCommand(pe Pending) string {
	if isGate(pe.Target.Control) && len(pe.Argv) > 0 {
		argv := append([]string(nil), pe.Argv...)
		argv[len(argv)-1] = "<gate token>"
		return CommandLine(argv)
	}
	return pe.Command
}

// openAuditFile opens the audit file for append. A partial last line left by
// an earlier failed write is closed with a newline first, so the next line is
// never glued to it.
func openAuditFile(path string) (io.WriteCloser, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err == nil && fi.Size() > 0 {
		var last [1]byte
		if _, err = f.ReadAt(last[:], fi.Size()-1); err == nil && last[0] != '\n' {
			_, err = f.Write([]byte{'\n'})
		}
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// writeLine writes one line. After a failed write it tries to end a partial
// line with a newline; the caller must treat the error as fatal for the run.
func writeLine(w io.Writer, l AuditLine) error {
	if err := WriteAudit(w, l); err != nil {
		_, _ = w.Write([]byte{'\n'})
		return err
	}
	return nil
}

func auditErr(err error) error {
	return fmt.Errorf("audit file cannot be written: %w", err)
}

func (s *Service) line(pe Pending) AuditLine {
	return AuditLine{Time: s.now().UTC().Format(time.RFC3339), Control: pe.Target.Control,
		Target: auditTarget(pe.Target), Command: auditCommand(pe)}
}

// refuseAudited writes a refusal line (best effort: nothing runs either way)
// and returns the refusal.
func (s *Service) refuseAudited(w io.Writer, pe Pending, err error) (RunResult, error) {
	if w != nil {
		l := s.line(pe)
		l.ExitCode, l.Refused = -1, err.Error()
		_ = writeLine(w, l)
	}
	return RunResult{ExitCode: -1, Refused: err.Error()}, err
}

func cut120(s string) string {
	r := []rune(s)
	if len(r) > 120 {
		r = r[:120]
	}
	return string(r)
}

// Run is the second press. It carries only the token; what runs is the argv
// stored at preview, after a fresh re-check of the target.
func (s *Service) Run(token string) (RunResult, error) {
	if !s.runMu.TryLock() {
		err := errors.New("another control is running")
		return RunResult{ExitCode: -1, Refused: err.Error()}, err
	}
	defer s.runMu.Unlock()

	peeked, existed := s.previews.Peek(token)
	pe, takeErr := s.previews.Take(token)
	if takeErr != nil {
		if existed {
			var w io.WriteCloser
			if f, err := s.openAudit(s.Env.AuditPath); err == nil {
				w = f
				defer f.Close()
			}
			return s.refuseAudited(w, peeked, takeErr)
		}
		return RunResult{ExitCode: -1, Refused: takeErr.Error()}, takeErr
	}

	w, err := s.openAudit(s.Env.AuditPath)
	if err != nil {
		e := auditErr(err)
		return RunResult{ExitCode: -1, Refused: e.Error()}, e
	}
	defer w.Close()

	a, self := s.assess()
	if !a.Ready[pe.Target.Control] {
		return s.refuseAudited(w, pe, errors.New(a.Why[pe.Target.Control]))
	}
	var ferr error
	if isGate(pe.Target.Control) {
		var flows []graph.Flow
		if flows, ferr = s.freshFlows(); ferr == nil {
			ferr = CheckGate(pe.Target, flows)
		}
	} else if pe.Target.Control == CloseSprint {
		var sprints []graph.Sprint
		if sprints, ferr = s.freshSprints(); ferr == nil {
			ferr = CheckSprint(pe.Target, sprints, s.Env.Merged, s.Env.ForemanRoot)
		}
	} else {
		var fresh []waits.AgentEntry
		if fresh, ferr = s.freshAgents(); ferr == nil {
			_, ferr = CheckAgent(pe.Target, fresh, self)
		}
	}
	if ferr != nil {
		return s.refuseAudited(w, pe, ferr)
	}

	// The line before the run: when it cannot be written, nothing runs.
	start := s.line(pe)
	start.ExitCode, start.Phase = -1, "start"
	if err := writeLine(w, start); err != nil {
		e := auditErr(err)
		return RunResult{ExitCode: -1, Refused: e.Error()}, e
	}

	res := RunResult{}
	switch {
	case pe.Target.Control == Attach:
		if err := s.Env.Runner.Start(pe.Argv); err != nil {
			res = RunResult{ExitCode: -1, Line: cut120(err.Error()), Failed: true}
		}
	default:
		timeout, dir := runStopTimeout, ""
		if isGate(pe.Target.Control) {
			timeout, dir = runGateTimeout, libDirOf(s.Env.ForemanRoot)
		}
		if pe.Target.Control == CloseSprint {
			timeout, dir = runGateTimeout, filepath.Dir(SprintScript(s.Env.ForemanRoot))
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		code, first, rerr := s.Env.Runner.Run(ctx, pe.Argv, dir)
		cancel()
		switch {
		case rerr != nil:
			res = RunResult{ExitCode: -1, Line: cut120(rerr.Error()), Failed: true}
		case code != 0:
			res = RunResult{ExitCode: code, Line: cut120(first), Failed: true}
			if isGate(pe.Target.Control) {
				switch code {
				case GateExitChanged:
					res.Refused = "the gate changed or was already decided"
				case GateExitVerdict:
					res.Refused = "the verdict is not valid for this gate"
				}
			}
			if pe.Target.Control == CloseSprint && code == 1 {
				res.Refused = closeRefused
			}
		default:
			res = RunResult{Line: cut120(first)}
		}
	}

	l := s.line(pe)
	l.ExitCode, l.Refused = res.ExitCode, res.Refused
	if err := writeLine(w, l); err != nil {
		return res, fmt.Errorf("the control ran but its audit line could not be written: %w", err)
	}
	return res, nil
}

package graph

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ReadPidFiles reads Claude Code's %USERPROFILE%\.claude\sessions\<pid>.json
// files. A missing folder means no Claude Code here and returns nothing. The
// format is Claude Code's own, not a documented contract: if any file does
// not have the fields R6 relies on, every file is dropped and the error says
// which, so the caller marks the source not ok instead of building on half
// a format.
func ReadPidFiles(dir string) ([]PidFile, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []PidFile{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []PidFile{}
	for _, e := range entries {
		name := e.Name()
		stem := strings.TrimSuffix(name, ".json")
		if e.IsDir() || stem == name {
			continue
		}
		if _, err := strconv.Atoi(stem); err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue // the session ended between the listing and the read
			}
			return nil, err
		}
		var raw struct {
			PID       *int32 `json:"pid"`
			SessionID string `json:"sessionId"`
			Cwd       string `json:"cwd"`
			ProcStart any    `json:"procStart"`
			Status    string `json:"status"`
			UpdatedAt int64  `json:"updatedAt"`
		}
		if err := json.Unmarshal(b, &raw); err != nil {
			return nil, fmt.Errorf("%s: unknown format: %w", name, err)
		}
		start, ok := raw.ProcStart.(string)
		n, perr := strconv.ParseInt(start, 10, 64)
		if raw.PID == nil || raw.SessionID == "" || !ok || perr != nil {
			return nil, fmt.Errorf("%s: unknown format: pid, sessionId or procStart missing or changed", name)
		}
		pf := PidFile{PID: *raw.PID, ProcStart: n, SessionID: raw.SessionID, Cwd: raw.Cwd, Status: raw.Status}
		if raw.UpdatedAt > 0 {
			pf.UpdatedAt = time.UnixMilli(raw.UpdatedAt)
		}
		out = append(out, pf)
	}
	return out, nil
}

// SubagentMeta finds a Claude subagent's meta file
// (<projects root>\<project>\<parent>\subagents\agent-<id>.meta.json) and
// keeps what it found; a meta file does not change once written. Only the
// agent type and model are read; the description is prompt text and stays
// out.
type SubagentMeta struct {
	root  string
	mu    sync.Mutex
	cache map[string]SubagentInfo
}

func NewSubagentMeta(projectsRoot string) *SubagentMeta {
	return &SubagentMeta{root: projectsRoot, cache: map[string]SubagentInfo{}}
}

func (m *SubagentMeta) Lookup(parent, id string) (SubagentInfo, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := parent + "/" + id
	if info, ok := m.cache[key]; ok {
		return info, true
	}
	matches, _ := filepath.Glob(filepath.Join(m.root, "*", parent, "subagents", "agent-"+id+".meta.json"))
	if len(matches) == 0 {
		return SubagentInfo{}, false
	}
	b, err := os.ReadFile(matches[0])
	if err != nil {
		return SubagentInfo{}, false
	}
	var raw struct {
		AgentType string `json:"agentType"`
		Model     string `json:"model"`
	}
	if json.Unmarshal(b, &raw) != nil {
		return SubagentInfo{}, false
	}
	info := SubagentInfo{AgentType: raw.AgentType, Model: raw.Model}
	m.cache[key] = info
	return info, true
}

type mcpEntry struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	URL     string   `json:"url"`
}

// mcpToken is the text that shows in the command line of the process that
// runs a server: its last argument that is not a flag, else the command's
// file name. A server reached by URL has no local process and no token.
func mcpToken(e mcpEntry) string {
	if e.Command == "" {
		return ""
	}
	for i := len(e.Args) - 1; i >= 0; i-- {
		if a := strings.TrimSpace(e.Args[i]); a != "" && !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return filepath.Base(strings.ReplaceAll(e.Command, `\`, "/"))
}

func mcpServers(m map[string]mcpEntry) []MCPServer {
	var out []MCPServer
	for name, e := range m {
		if tok := mcpToken(e); tok != "" {
			out = append(out, MCPServer{Name: name, Token: tok})
		}
	}
	return out
}

type cachedFile struct {
	mod  time.Time
	size int64
	val  any
}

// MCPConfig reads Claude Code's MCP servers: the user's own and per project
// from ~/.claude.json, and a project's .mcp.json. Each file is parsed again
// only when it changes.
type MCPConfig struct {
	path  string
	mu    sync.Mutex
	files map[string]cachedFile
	err   error
}

func NewMCPConfig(claudeJSON string) *MCPConfig {
	return &MCPConfig{path: claudeJSON, files: map[string]cachedFile{}}
}

// Err is the last error reading ~/.claude.json, nil when it read fine or
// does not exist.
func (c *MCPConfig) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *MCPConfig) load(path string, parse func([]byte) (any, error)) (any, error) {
	fi, err := os.Stat(path)
	if err != nil {
		delete(c.files, path)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if f, ok := c.files[path]; ok && f.mod.Equal(fi.ModTime()) && f.size == fi.Size() {
		return f.val, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	v, err := parse(b)
	if err != nil {
		return nil, err
	}
	c.files[path] = cachedFile{mod: fi.ModTime(), size: fi.Size(), val: v}
	return v, nil
}

type claudeJSON struct {
	global   []MCPServer
	projects map[string][]MCPServer
}

// For returns the servers a Claude Code session in cwd can start.
func (c *MCPConfig) For(cwd string) []MCPServer {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []MCPServer
	v, err := c.load(c.path, func(b []byte) (any, error) {
		var raw struct {
			MCPServers map[string]mcpEntry `json:"mcpServers"`
			Projects   map[string]struct {
				MCPServers map[string]mcpEntry `json:"mcpServers"`
			} `json:"projects"`
		}
		if err := json.Unmarshal(b, &raw); err != nil {
			return nil, err
		}
		cj := claudeJSON{global: mcpServers(raw.MCPServers), projects: map[string][]MCPServer{}}
		for p, pv := range raw.Projects {
			if s := mcpServers(pv.MCPServers); len(s) > 0 {
				cj.projects[pathKey(p)] = s
			}
		}
		return cj, nil
	})
	c.err = err
	if cj, ok := v.(claudeJSON); ok {
		out = append(out, cj.global...)
		out = append(out, cj.projects[pathKey(cwd)]...)
	}
	if cwd != "" {
		v, _ := c.load(filepath.Join(cwd, ".mcp.json"), func(b []byte) (any, error) {
			var raw struct {
				MCPServers map[string]mcpEntry `json:"mcpServers"`
			}
			if err := json.Unmarshal(b, &raw); err != nil {
				return nil, err
			}
			return mcpServers(raw.MCPServers), nil
		})
		if s, ok := v.([]MCPServer); ok {
			out = append(out, s...)
		}
	}
	return out
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// ReadForeman reads Foreman's state under root: sprints\<project>\<slug>\
// sprint.json, flows\<project>\<flow>\flow.json and its foreman.json.
// Folders that start with "_" are Foreman's own library, not work. root is
// configured, so a root that does not exist is an error.
func ReadForeman(root string) ([]Sprint, []Flow, error) {
	if _, err := os.Stat(root); err != nil {
		return nil, nil, err
	}
	sprints := []Sprint{}
	paths, _ := filepath.Glob(filepath.Join(root, "sprints", "*", "*", "sprint.json"))
	for _, p := range paths {
		if underscore(root, p) {
			continue
		}
		var raw struct {
			ID, Slug, Phase, Status, UpdatedAt string
			WorktreePath                       string `json:"worktree_path"`
			WorktreeBranch                     string `json:"worktree_branch"`
		}
		if !readJSON(p, &raw, map[string]*string{"sprint_id": &raw.ID, "updated_at": &raw.UpdatedAt}) {
			continue
		}
		dir := filepath.Dir(p)
		var m map[string]json.RawMessage
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, &m)
		}
		times, parked := readActionLog(filepath.Join(dir, "action-log.ndjson"))
		s := Sprint{ID: raw.ID, Slug: raw.Slug, WorktreePath: raw.WorktreePath, Branch: raw.WorktreeBranch,
			Phase: raw.Phase, Status: raw.Status, UpdatedAt: parseTime(raw.UpdatedAt),
			LogTimes: times, Dir: dir,
			Project: rawString(m, "project"), SpecPath: rawString(m, "spec_path")}
		s.ResultPath = rawString(m, "report_path")
		if s.ResultPath == "" {
			if fi, err := os.Stat(filepath.Join(dir, "report.md")); err == nil && fi.Mode().IsRegular() {
				s.ResultPath = filepath.Join(dir, "report.md")
			}
		}
		if s.Status == "waiting-on-you" {
			s.ReportPath, s.ParkedLines = readReport(dir)
		}
		s.Test = rawCheck(m["last_test_result"])
		s.Verify = rawVerify(m["last_verify_result"])
		var tasks []map[string]json.RawMessage
		if json.Unmarshal(m["tasks"], &tasks) == nil {
			for _, t := range tasks {
				if rawString(t, "state") != "parked" {
					continue
				}
				var n float64
				_ = json.Unmarshal(t["n"], &n)
				pt := ParkedTask{N: int(n), Title: rawString(t, "title")}
				if e, ok := parked[pt.N]; ok {
					pt.Reason, pt.At = e.Reason, e.At
				}
				s.Parked = append(s.Parked, pt)
			}
		}
		for _, e := range parked {
			if e.At.After(s.LastParkedAt) {
				s.LastParkedAt = e.At
			}
		}
		sprints = append(sprints, s)
	}
	flows := []Flow{}
	paths, _ = filepath.Glob(filepath.Join(root, "flows", "*", "*", "flow.json"))
	for _, p := range paths {
		if underscore(root, p) {
			continue
		}
		var raw struct {
			ID, Slug, Status, UpdatedAt string
			Gate                        *struct{ ID, Status string } `json:"gate"`
		}
		if !readJSON(p, &raw, map[string]*string{"flow_id": &raw.ID, "updated_at": &raw.UpdatedAt}) {
			continue
		}
		f := Flow{ID: raw.ID, Slug: raw.Slug, Status: raw.Status, UpdatedAt: parseTime(raw.UpdatedAt), Dir: filepath.Dir(p)}
		if raw.Gate != nil && raw.Gate.ID != "" {
			f.Gate, f.GatePending = raw.Gate.ID, raw.Gate.Status != "decided"
		}
		var m map[string]json.RawMessage
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, &m)
		}
		f.Project, f.SpecPath = rawString(m, "project"), rawString(m, "spec_path")
		var g map[string]json.RawMessage
		if json.Unmarshal(m["gate"], &g) == nil {
			f.GateOpenedAt = parseTime(rawString(g, "opened_at"))
			f.GateToken = rawString(g, "token")
		}
		var fm struct {
			SessionID string `json:"session_id"`
		}
		if b, err := os.ReadFile(filepath.Join(filepath.Dir(p), "foreman.json")); err == nil && json.Unmarshal(b, &fm) == nil {
			f.SessionID = fm.SessionID
		}
		flows = append(flows, f)
	}
	return sprints, flows, nil
}

// maxLogTimes bounds how many action log entries one sprint can contribute.
const maxLogTimes = 20000

// maxLogLine is the longest action log line that is parsed; a longer line is
// skipped and the lines after it are still read.
const maxLogLine = 1 << 20

// rawString is the string value of key in m, "" when absent or not a string.
func rawString(m map[string]json.RawMessage, key string) string {
	var s string
	if json.Unmarshal(m[key], &s) != nil {
		return ""
	}
	return s
}

// rawCheck reads a last_test_result object; nil unless exit_code is a number.
func rawCheck(b json.RawMessage) *Check {
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	var code float64
	if json.Unmarshal(m["exit_code"], &code) != nil {
		return nil
	}
	return &Check{Failed: code != 0, Label: fmt.Sprintf("test exit %d", int(code)),
		Summary: rawString(m, "summary"), At: parseTime(rawString(m, "at"))}
}

// rawVerify reads a last_verify_result object; nil unless outcome is a string.
func rawVerify(b json.RawMessage) *Check {
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	var outcome string
	if json.Unmarshal(m["outcome"], &outcome) != nil {
		return nil
	}
	label := "verify pass"
	if outcome == "fail" {
		label = "verify fail"
	}
	return &Check{Failed: outcome == "fail", Label: label,
		Summary: rawString(m, "summary"), At: parseTime(rawString(m, "at"))}
}

// readActionLog reads a sprint's action-log.ndjson: the "ts" of every line in
// time order, and the newest task_parked entry per task number. A missing
// file, a line it cannot read, a line longer than maxLogLine and a line with
// no time are skipped: the log only says when the sprint was busy and why a
// task was parked.
func readActionLog(path string) ([]time.Time, map[int]ParkedTask) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil
	}
	defer f.Close()
	var out []time.Time
	parked := map[int]ParkedTask{}
	r := bufio.NewReaderSize(f, 64*1024)
	for len(out) < maxLogTimes {
		line, err := readLine(r)
		if len(line) > 0 {
			var l struct {
				TS     string  `json:"ts"`
				Action string  `json:"action"`
				Task   float64 `json:"task"`
				Detail string  `json:"detail"`
			}
			if json.Unmarshal(line, &l) == nil && l.TS != "" {
				if t := parseTime(l.TS); !t.IsZero() {
					out = append(out, t)
					if l.Action == "task_parked" {
						if old, ok := parked[int(l.Task)]; !ok || t.After(old.At) {
							parked[int(l.Task)] = ParkedTask{N: int(l.Task), Reason: l.Detail, At: t}
						}
					}
				}
			}
		}
		if err != nil {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out, parked
}

// readLine returns the next line without its newline. A line longer than
// maxLogLine is consumed and returned empty. The error is io.EOF at the end.
func readLine(r *bufio.Reader) ([]byte, error) {
	var buf []byte
	tooLong := false
	for {
		chunk, isPrefix, err := r.ReadLine()
		if err != nil {
			if tooLong {
				return nil, err
			}
			return buf, err
		}
		if !tooLong {
			if len(buf)+len(chunk) > maxLogLine {
				tooLong, buf = true, nil
			} else {
				buf = append(buf, chunk...)
			}
		}
		if !isPrefix {
			if tooLong {
				return nil, nil
			}
			return buf, nil
		}
	}
}

func underscore(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return true
	}
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if strings.HasPrefix(seg, "_") {
			return true
		}
	}
	return false
}

// readJSON decodes path into v, plus the snake_case keys in extra (Go's
// decoder matches field names case-insensitively but not across "_").
// False when the file is unreadable or has no id.
func readJSON(path string, v any, extra map[string]*string) bool {
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, v) != nil {
		return false
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return false
	}
	for k, dst := range extra {
		if s, ok := m[k].(string); ok {
			*dst = s
		}
	}
	for k, dst := range extra {
		if strings.HasSuffix(k, "_id") && *dst == "" {
			return false
		}
	}
	return true
}

// RepoLocator finds the git repo or linked worktree a folder sits in, and
// its checked-out branch. It reads .git and HEAD only, never runs git, and
// keeps each answer for a short while.
type RepoLocator struct {
	mu    sync.Mutex
	cache map[string]repoHit
	ttl   time.Duration
}

type repoHit struct {
	r  Repo
	ok bool
	at time.Time
}

func NewRepoLocator() *RepoLocator {
	return &RepoLocator{cache: map[string]repoHit{}, ttl: 30 * time.Second}
}

func (l *RepoLocator) For(cwd string) (Repo, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := pathKey(cwd)
	if h, ok := l.cache[key]; ok && time.Since(h.at) < l.ttl {
		return h.r, h.ok
	}
	r, ok := locate(cwd)
	l.cache[key] = repoHit{r: r, ok: ok, at: time.Now()}
	return r, ok
}

func locate(cwd string) (Repo, bool) {
	dir := filepath.Clean(cwd)
	for {
		dotgit := filepath.Join(dir, ".git")
		fi, err := os.Stat(dotgit)
		if err == nil {
			if fi.IsDir() {
				return Repo{Root: dir, Branch: headBranch(dotgit)}, true
			}
			b, err := os.ReadFile(dotgit)
			if err != nil {
				return Repo{}, false
			}
			gitdir := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(b)), "gitdir:"))
			if !filepath.IsAbs(gitdir) {
				gitdir = filepath.Join(dir, gitdir)
			}
			main := ""
			// <main>\.git\worktrees\<name>
			if wts := filepath.Dir(gitdir); filepath.Base(wts) == "worktrees" {
				main = filepath.Dir(filepath.Dir(wts))
			}
			return Repo{Root: dir, Main: main, Branch: headBranch(gitdir)}, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Repo{}, false
		}
		dir = parent
	}
}

func headBranch(gitdir string) string {
	b, err := os.ReadFile(filepath.Join(gitdir, "HEAD"))
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(b))
	if ref, ok := strings.CutPrefix(s, "ref: refs/heads/"); ok {
		return ref
	}
	return "" // detached HEAD: no branch
}

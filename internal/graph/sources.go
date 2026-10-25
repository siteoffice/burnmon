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
		sprints = append(sprints, Sprint{ID: raw.ID, Slug: raw.Slug, WorktreePath: raw.WorktreePath, Branch: raw.WorktreeBranch,
			Phase: raw.Phase, Status: raw.Status, UpdatedAt: parseTime(raw.UpdatedAt),
			LogTimes: readLogTimes(filepath.Join(filepath.Dir(p), "action-log.ndjson"))})
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
		f := Flow{ID: raw.ID, Slug: raw.Slug, Status: raw.Status, UpdatedAt: parseTime(raw.UpdatedAt)}
		if raw.Gate != nil && raw.Gate.ID != "" {
			f.Gate, f.GatePending = raw.Gate.ID, raw.Gate.Status != "decided"
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

// readLogTimes reads the "ts" of every line of a sprint's action-log.ndjson
// and returns them in time order. A missing file, a line it cannot read and a
// line with no time are skipped: the log only says when the sprint was busy.
func readLogTimes(path string) []time.Time {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []time.Time
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() && len(out) < maxLogTimes {
		var l struct {
			TS string `json:"ts"`
		}
		if json.Unmarshal(sc.Bytes(), &l) != nil || l.TS == "" {
			continue
		}
		if t := parseTime(l.TS); !t.IsZero() {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
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

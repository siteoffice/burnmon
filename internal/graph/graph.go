// Package graph is R6 of the agent control plan: the relation graph between
// the device, its harnesses, sessions, subagents, processes, MCP servers,
// ports, repos and Foreman work. Build is pure: it joins facts the caller
// already read (the store, the process walk, Claude Code's pid files, the
// Foreman state files, MCP config, the TCP table) and never reads anything
// itself. Every edge says how BurnMon knows it (Basis) and where the fact
// came from (Source). An edge BurnMon cannot see is left out, never guessed.
// Field meanings: docs\snapshot_schema.md.
package graph

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// Node kinds.
const (
	KindDevice   = "device"
	KindHarness  = "harness"
	KindSession  = "session"
	KindSubagent = "subagent"
	KindProcess  = "process"
	KindMCP      = "mcp"
	KindPort     = "port"
	KindRepo     = "repo"
	KindWorktree = "worktree"
	KindBranch   = "branch"
	KindFlow     = "flow"
	KindSprint   = "sprint"
	KindGate     = "gate"
	KindPerson   = "person"
)

// Edge kinds.
const (
	KindRunsOn    = "runs_on"    // harness to device
	KindRunsIn    = "runs_in"    // session to harness or to its own process; process to harness
	KindSpawned   = "spawned"    // session to subagent
	KindStarted   = "started"    // process to child process
	KindUses      = "uses"       // session to MCP server
	KindRunsAs    = "runs_as"    // MCP server to its process
	KindListensOn = "listens_on" // process to port
	KindWorksFor  = "works_for"  // session to sprint or flow
	KindWaitsOn   = "waits_on"   // session, sprint or gate to person
	KindTouches   = "touches"    // session or sprint to repo, worktree or branch
	KindPartOf    = "part_of"    // worktree to repo, gate to flow
)

// Basis says how BurnMon knows an edge.
const (
	BasisVendor   = "vendor"   // the source's own data states the link
	BasisInferred = "inferred" // BurnMon joined two facts that do not name each other
)

// Source names, the same names the snapshot lists in sources[].
const (
	SourceStore      = "store"
	SourceSysmon     = "sysmon"
	SourceClaudePids = "claude_pids"
	SourceForeman    = "foreman"
	SourceMCPConfig  = "mcp_config"
	SourceNetTables  = "net_tables"
	SourceGit        = "git"
)

const (
	deviceID = "device:local"
	personID = "person:you"
)

// Node is one thing in the graph.
type Node struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"`
	Label      string            `json:"label"`
	Vendor     string            `json:"vendor,omitempty"`
	Attrs      map[string]string `json:"attrs,omitempty"`
	Source     string            `json:"source"`
	AgeSeconds int64             `json:"age_seconds"`
}

// Edge is one relation between two nodes.
type Edge struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Kind       string `json:"kind"`
	Basis      string `json:"basis"`
	Source     string `json:"source"`
	AgeSeconds int64  `json:"age_seconds"`
	FirstSeen  string `json:"first_seen"` // RFC3339 UTC
	LastSeen   string `json:"last_seen"`  // RFC3339 UTC
}

// Key identifies an edge: one edge per (from, to, kind).
func (e Edge) Key() string { return e.From + "|" + e.Kind + "|" + e.To }

// Graph is the node and edge lists, both always non-nil.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Stamp sets FirstSeen from firstSeen (keyed on Edge.Key) where it has an
// entry; other edges keep the FirstSeen Build gave them (now).
func (g Graph) Stamp(firstSeen map[string]time.Time) {
	for i := range g.Edges {
		if t, ok := firstSeen[g.Edges[i].Key()]; ok {
			g.Edges[i].FirstSeen = utc(t)
		}
	}
}

// Session is one session or subagent from the store.
type Session struct {
	Vendor, Agent, ID, ParentID, Project, State string
	// FirstActivity is the session's first turn, zero when the source does
	// not say; the link to a sprint then rests on LastActivity alone.
	FirstActivity time.Time
	LastActivity  time.Time
	// Tool is the name of the tool the session is running now ("" when none),
	// from the stage the core works out. The name only, never its arguments.
	Tool string
}

// Proc is one process from the process walk. Cmdline is used for matching
// only and never leaves this package.
type Proc struct {
	PID, PPID  int32
	Name       string
	Cmdline    string
	CreateTime int64  // FILETIME, the process identity together with PID
	Harness    string // sysmon's harness classification, "" or "other" when none
}

// PidFile is one Claude Code %USERPROFILE%\.claude\sessions\<pid>.json.
type PidFile struct {
	PID       int32
	ProcStart int64 // FILETIME, compared with Proc.CreateTime
	SessionID string
	Cwd       string
	Status    string // "busy", "idle"
	UpdatedAt time.Time
}

// MCPServer is one configured MCP server. Token is the text that shows in
// the command line of the process that runs it.
type MCPServer struct {
	Name, Token string
}

// Listen is one listening socket.
type Listen struct {
	PID   int32
	Proto string
	Port  uint32
}

// Repo is what a cwd sits in: Root is the worktree or repo top folder, Main
// the main repo when Root is a linked worktree ("" otherwise).
type Repo struct {
	Root, Main, Branch string
}

// Sprint is one Foreman sprint.
type Sprint struct {
	ID, Slug, WorktreePath, Branch, Phase, Status string
	UpdatedAt                                     time.Time
	// LogTimes are the times of the entries in the sprint's own
	// action-log.ndjson, in order. They say when the sprint was busy.
	LogTimes []time.Time

	// Project is the sprint's project name, SpecPath its spec, ResultPath its
	// report ("" when none) and Dir the sprint folder.
	Project, SpecPath, ResultPath, Dir string
	// Parked lists the parked tasks, in task order.
	Parked []ParkedTask
	// Test and Verify are the last test and verify results, nil when absent.
	Test, Verify *Check
	// LastParkedAt is the newest task_parked entry in the action log.
	LastParkedAt time.Time
}

// Check is one recorded test or verify result.
type Check struct {
	Failed  bool
	Label   string // "test exit 1", "test exit 0", "verify fail", "verify pass"
	Summary string
	At      time.Time // zero when absent
}

// ParkedTask is a task a sprint parked for the person.
type ParkedTask struct {
	N      int
	Title  string
	Reason string    // detail of the newest task_parked entry for N, "" when none
	At     time.Time // ts of that entry
}

// linkGrace is how far from a session's activity a sprint log entry may fall
// and still count as written while the session was active.
const linkGrace = 10 * time.Minute

// sprintMainRepo is the repo a sprint's worktree hangs off: the folder before
// ".worktrees". It is "" for a path that is not under one.
func sprintMainRepo(worktree string) string {
	p := strings.TrimRight(worktree, `\/`)
	low := strings.ToLower(strings.ReplaceAll(p, "/", `\`))
	i := strings.Index(low, `\.worktrees\`)
	if i <= 0 {
		return ""
	}
	return p[:i]
}

// activeWhileLogged reports whether a sprint log entry falls inside the time
// the session was active (first turn to last turn, ten minutes either side).
// A session with no known first turn counts at its last turn alone.
func activeWhileLogged(s Session, logTimes []time.Time) bool {
	first, last := s.FirstActivity, s.LastActivity
	if first.IsZero() || first.After(last) {
		first = last
	}
	from, to := first.Add(-linkGrace), last.Add(linkGrace)
	for _, t := range logTimes {
		if !t.Before(from) && !t.After(to) {
			return true
		}
	}
	return false
}

// Flow is one Foreman flow.
type Flow struct {
	ID, Slug, SessionID, Status, Gate string
	GatePending                       bool
	UpdatedAt                         time.Time
	Project, SpecPath, Dir            string
	GateOpenedAt                      time.Time
}

// SubagentInfo is what a Claude subagent's meta file says about it.
type SubagentInfo struct {
	AgentType, Model string
}

// Input is everything Build joins. Any field may be empty; the func fields
// may be nil.
type Input struct {
	Now         time.Time
	Sessions    []Session
	Procs       []Proc
	ProcsAt     time.Time
	Pids        []PidFile
	MCPFor      func(cwd string) []MCPServer
	Listens     []Listen
	RepoFor     func(cwd string) (Repo, bool)
	Sprints     []Sprint
	Flows       []Flow
	SubagentFor func(parent, id string) (SubagentInfo, bool)
}

// harnessOfAgent maps a store agent name to sysmon's harness names, so a
// session and its processes hang under one harness node.
var harnessOfAgent = map[string]string{
	"claude-code": "claude",
	"cowork":      "claude-desktop",
}

var harnessLabels = map[string]string{
	"claude":         "Claude Code",
	"claude-desktop": "Claude Desktop",
	"cowork-cloud":   "Cowork cloud",
	"codex":          "Codex",
	"copilot-cli":    "Copilot CLI",
	"copilot-vscode": "Copilot in VS Code",
	"hermes":         "Hermes",
}

// Process trees kept whole. For the two app harnesses only the top process
// is kept: their children are the app's own helpers, not agent work.
var wholeTree = map[string]bool{"claude": true, "codex": true, "copilot-cli": true, "hermes": true}
var rootOnly = map[string]bool{"claude-desktop": true, "copilot-vscode": true}

type builder struct {
	in    Input
	g     Graph
	nodes map[string]int
	edges map[string]int
}

func newBuilder(in Input) *builder {
	return &builder{in: in, g: Graph{Nodes: []Node{}, Edges: []Edge{}}, nodes: map[string]int{}, edges: map[string]int{}}
}

func utc(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func (b *builder) age(t time.Time) int64 {
	if t.IsZero() || t.After(b.in.Now) {
		return 0
	}
	return int64(b.in.Now.Sub(t) / time.Second)
}

func (b *builder) node(n Node) {
	if _, ok := b.nodes[n.ID]; ok {
		return
	}
	b.nodes[n.ID] = len(b.g.Nodes)
	b.g.Nodes = append(b.g.Nodes, n)
}

func (b *builder) has(id string) bool { _, ok := b.nodes[id]; return ok }

// edge adds e when both ends exist. A second edge with the same key replaces
// the first only when it is vendor-stated and the first was inferred.
func (b *builder) edge(e Edge) {
	if !b.has(e.From) || !b.has(e.To) {
		return
	}
	if e.FirstSeen == "" {
		e.FirstSeen = utc(b.in.Now)
	}
	if e.LastSeen == "" {
		e.LastSeen = utc(b.in.Now)
	}
	if i, ok := b.edges[e.Key()]; ok {
		if b.g.Edges[i].Basis == BasisInferred && e.Basis == BasisVendor {
			b.g.Edges[i] = e
		}
		return
	}
	b.edges[e.Key()] = len(b.g.Edges)
	b.g.Edges = append(b.g.Edges, e)
}

func (b *builder) harness(name string) string {
	id := "harness:" + name
	if !b.has(id) {
		label := harnessLabels[name]
		if label == "" {
			label = name
		}
		b.node(Node{ID: id, Kind: KindHarness, Label: label, Source: SourceSysmon})
		b.edge(Edge{From: id, To: deviceID, Kind: KindRunsOn, Basis: BasisVendor, Source: SourceStore})
	}
	return id
}

func (b *builder) person() string {
	b.node(Node{ID: personID, Kind: KindPerson, Label: "You", Source: SourceStore})
	return personID
}

// pathKey normalises a Windows path for comparison and ids.
func pathKey(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "/", `\`)
	return strings.ToLower(strings.TrimRight(p, `\`))
}

func baseLabel(p string) string {
	p = strings.TrimRight(strings.ReplaceAll(p, "/", `\`), `\`)
	if i := strings.LastIndex(p, `\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// SessionID is a session node's id.
func SessionID(vendor, id string) string { return "session:" + vendor + ":" + id }

// ProcessID is a process node's id: pid plus start time, since Windows
// reuses pids.
func ProcessID(pid int32, createTime int64) string {
	return "process:" + strconv.Itoa(int(pid)) + ":" + strconv.FormatInt(createTime, 10)
}

func (b *builder) session(s Session) {
	id := SessionID(s.Vendor, s.ID)
	kind, label := KindSession, baseLabel(s.Project)
	attrs := map[string]string{"agent": s.Agent}
	if s.State != "" {
		attrs["state"] = s.State
	}
	if s.Tool != "" {
		attrs["tool"] = s.Tool
	}
	if s.ParentID != "" {
		kind = KindSubagent
		label = "subagent"
		if b.in.SubagentFor != nil {
			if info, ok := b.in.SubagentFor(s.ParentID, strings.TrimPrefix(s.ID, "agent-")); ok {
				if info.AgentType != "" {
					attrs["agent_type"], label = info.AgentType, info.AgentType
				}
				if info.Model != "" {
					attrs["model"] = info.Model
				}
			}
		}
	}
	if label == "" {
		label = s.ID
	}
	b.node(Node{ID: id, Kind: kind, Label: label, Vendor: s.Vendor, Attrs: attrs, Source: SourceStore, AgeSeconds: b.age(s.LastActivity)})
}

// Build joins in into one graph.
func Build(in Input) Graph {
	b := newBuilder(in)
	b.node(Node{ID: deviceID, Kind: KindDevice, Label: "This PC", Source: SourceSysmon})

	// Sessions and subagents from the store.
	byID := map[string]Session{}
	for _, s := range in.Sessions {
		b.session(s)
		byID[SessionID(s.Vendor, s.ID)] = s
	}
	for _, s := range in.Sessions {
		id := SessionID(s.Vendor, s.ID)
		h := harnessOfAgent[s.Agent]
		if h == "" {
			h = s.Agent
		}
		b.edge(Edge{From: id, To: b.harness(h), Kind: KindRunsIn, Basis: BasisVendor, Source: SourceStore, AgeSeconds: b.age(s.LastActivity)})
		if s.ParentID != "" {
			b.edge(Edge{From: SessionID(s.Vendor, s.ParentID), To: id, Kind: KindSpawned, Basis: BasisVendor, Source: SourceStore, AgeSeconds: b.age(s.LastActivity)})
		}
	}

	// Processes in harness trees.
	procs := map[int32]Proc{}
	children := map[int32][]int32{}
	for _, p := range in.Procs {
		procs[p.PID] = p
		children[p.PPID] = append(children[p.PPID], p.PID)
	}
	keep := map[int32]bool{}
	for _, p := range in.Procs {
		if wholeTree[p.Harness] {
			keep[p.PID] = true
		} else if rootOnly[p.Harness] && procs[p.PPID].Harness != p.Harness {
			keep[p.PID] = true
		}
	}
	procAge := b.age(in.ProcsAt)
	pids := make([]int32, 0, len(keep))
	for pid := range keep {
		pids = append(pids, pid)
	}
	sort.Slice(pids, func(i, j int) bool { return pids[i] < pids[j] })
	for _, pid := range pids {
		p := procs[pid]
		b.node(Node{ID: ProcessID(p.PID, p.CreateTime), Kind: KindProcess, Label: p.Name,
			Attrs: map[string]string{"pid": strconv.Itoa(int(p.PID)), "harness": p.Harness}, Source: SourceSysmon, AgeSeconds: procAge})
	}
	for _, pid := range pids {
		p := procs[pid]
		id := ProcessID(p.PID, p.CreateTime)
		if parent, ok := procs[p.PPID]; ok && keep[p.PPID] {
			b.edge(Edge{From: ProcessID(parent.PID, parent.CreateTime), To: id, Kind: KindStarted, Basis: BasisVendor, Source: SourceSysmon, AgeSeconds: procAge})
		} else {
			b.edge(Edge{From: id, To: b.harness(p.Harness), Kind: KindRunsIn, Basis: BasisVendor, Source: SourceSysmon, AgeSeconds: procAge})
		}
	}

	// Claude Code pid files: session to its own process, its MCP servers,
	// and its own idle status.
	idle := map[string]PidFile{}
	for _, pf := range in.Pids {
		p, ok := procs[pf.PID]
		if !ok || p.CreateTime != pf.ProcStart {
			continue // not running, or the pid was reused
		}
		sid := SessionID("anthropic", pf.SessionID)
		if !b.has(sid) {
			b.node(Node{ID: sid, Kind: KindSession, Label: baseLabel(pf.Cwd), Vendor: "anthropic",
				Attrs: map[string]string{"agent": "claude-code"}, Source: SourceClaudePids, AgeSeconds: b.age(pf.UpdatedAt)})
			byID[sid] = Session{Vendor: "anthropic", Agent: "claude-code", ID: pf.SessionID, Project: pf.Cwd}
			b.edge(Edge{From: sid, To: b.harness("claude"), Kind: KindRunsIn, Basis: BasisVendor, Source: SourceClaudePids, AgeSeconds: b.age(pf.UpdatedAt)})
		}
		b.edge(Edge{From: sid, To: ProcessID(p.PID, p.CreateTime), Kind: KindRunsIn, Basis: BasisVendor, Source: SourceClaudePids, AgeSeconds: b.age(pf.UpdatedAt)})
		if pf.Status == "idle" {
			idle[sid] = pf
		}
		if in.MCPFor == nil {
			continue
		}
		servers := in.MCPFor(pf.Cwd)
		for _, cpid := range children[pf.PID] {
			c := procs[cpid]
			cmd := strings.ToLower(c.Cmdline)
			for _, s := range servers {
				if s.Token == "" || !strings.Contains(cmd, strings.ToLower(s.Token)) {
					continue
				}
				mid := "mcp:" + s.Name
				b.node(Node{ID: mid, Kind: KindMCP, Label: s.Name, Source: SourceMCPConfig, AgeSeconds: procAge})
				b.edge(Edge{From: sid, To: mid, Kind: KindUses, Basis: BasisInferred, Source: SourceMCPConfig, AgeSeconds: procAge})
				b.edge(Edge{From: mid, To: ProcessID(c.PID, c.CreateTime), Kind: KindRunsAs, Basis: BasisInferred, Source: SourceMCPConfig, AgeSeconds: procAge})
				break
			}
		}
	}

	// Waits on you: Claude Code's own idle status first, else the stage.
	for _, s := range in.Sessions {
		id := SessionID(s.Vendor, s.ID)
		if pf, ok := idle[id]; ok {
			b.person()
			b.edge(Edge{From: id, To: personID, Kind: KindWaitsOn, Basis: BasisVendor, Source: SourceClaudePids, AgeSeconds: b.age(pf.UpdatedAt)})
		} else if s.State == "waiting" && s.ParentID == "" {
			// A subagent whose turn ended has finished; it asks no one anything.
			b.person()
			b.edge(Edge{From: id, To: personID, Kind: KindWaitsOn, Basis: BasisInferred, Source: SourceStore, AgeSeconds: b.age(s.LastActivity)})
		}
	}
	for id, pf := range idle {
		if _, ok := b.edges[id+"|"+KindWaitsOn+"|"+personID]; !ok {
			b.person()
			b.edge(Edge{From: id, To: personID, Kind: KindWaitsOn, Basis: BasisVendor, Source: SourceClaudePids, AgeSeconds: b.age(pf.UpdatedAt)})
		}
	}

	// Listening ports of kept processes.
	for _, l := range in.Listens {
		p, ok := procs[l.PID]
		if !ok || !keep[l.PID] {
			continue
		}
		port := strconv.FormatUint(uint64(l.Port), 10)
		pid := "port:" + l.Proto + ":" + port
		b.node(Node{ID: pid, Kind: KindPort, Label: l.Proto + " " + port, Source: SourceNetTables})
		b.edge(Edge{From: ProcessID(p.PID, p.CreateTime), To: pid, Kind: KindListensOn, Basis: BasisVendor, Source: SourceNetTables})
	}

	// Repos, worktrees and branches the sessions sit in.
	sessionIDs := make([]string, 0, len(byID))
	for id := range byID {
		sessionIDs = append(sessionIDs, id)
	}
	sort.Strings(sessionIDs)
	if in.RepoFor != nil {
		for _, id := range sessionIDs {
			s := byID[id]
			if s.Project == "" {
				continue
			}
			r, ok := in.RepoFor(s.Project)
			if !ok {
				continue
			}
			for _, e := range b.repoNodes(r, SourceGit) {
				b.edge(Edge{From: id, To: e, Kind: KindTouches, Basis: BasisInferred, Source: SourceGit})
			}
		}
	}

	// Foreman: sprints and flows linked to a session, and gates or sprints
	// that wait on you.
	for _, sp := range in.Sprints {
		sid := "sprint:" + sp.ID
		waiting := sp.Status == "waiting-on-you"
		// A session links to a sprint when its folder is the sprint's worktree
		// or its main repo and it was active while the sprint's own action log
		// was being written. An attended sprint's agents run in the main repo.
		// With no log to say when, only the worktree folder links, as before.
		var linked []string
		main := sprintMainRepo(sp.WorktreePath)
		for _, id := range sessionIDs {
			s := byID[id]
			inWT := sp.WorktreePath != "" && pathKey(s.Project) == pathKey(sp.WorktreePath)
			inRepo := main != "" && pathKey(s.Project) == pathKey(main)
			switch {
			case len(sp.LogTimes) == 0:
				if inWT {
					linked = append(linked, id)
				}
			case (inWT || inRepo) && activeWhileLogged(s, sp.LogTimes):
				linked = append(linked, id)
			}
		}
		if len(linked) == 0 && !waiting {
			continue
		}
		b.node(Node{ID: sid, Kind: KindSprint, Label: sp.Slug, Attrs: map[string]string{"phase": sp.Phase, "status": sp.Status},
			Source: SourceForeman, AgeSeconds: b.age(sp.UpdatedAt)})
		for _, id := range linked {
			b.edge(Edge{From: id, To: sid, Kind: KindWorksFor, Basis: BasisInferred, Source: SourceForeman, AgeSeconds: b.age(sp.UpdatedAt)})
		}
		if waiting {
			b.person()
			b.edge(Edge{From: sid, To: personID, Kind: KindWaitsOn, Basis: BasisVendor, Source: SourceForeman, AgeSeconds: b.age(sp.UpdatedAt)})
		}
		if sp.WorktreePath != "" {
			wt := "worktree:" + pathKey(sp.WorktreePath)
			b.node(Node{ID: wt, Kind: KindWorktree, Label: baseLabel(sp.WorktreePath), Source: SourceForeman})
			b.edge(Edge{From: sid, To: wt, Kind: KindTouches, Basis: BasisVendor, Source: SourceForeman, AgeSeconds: b.age(sp.UpdatedAt)})
		}
	}
	for _, f := range in.Flows {
		fid := "flow:" + f.ID
		sid := SessionID("anthropic", f.SessionID)
		linked := f.SessionID != "" && b.has(sid)
		if !linked && !f.GatePending {
			continue
		}
		b.node(Node{ID: fid, Kind: KindFlow, Label: f.Slug, Attrs: map[string]string{"status": f.Status},
			Source: SourceForeman, AgeSeconds: b.age(f.UpdatedAt)})
		if linked {
			b.edge(Edge{From: sid, To: fid, Kind: KindWorksFor, Basis: BasisVendor, Source: SourceForeman, AgeSeconds: b.age(f.UpdatedAt)})
		}
		if f.GatePending && f.Gate != "" {
			gid := "gate:" + f.ID + ":" + f.Gate
			b.node(Node{ID: gid, Kind: KindGate, Label: f.Gate, Source: SourceForeman, AgeSeconds: b.age(f.UpdatedAt)})
			b.edge(Edge{From: gid, To: fid, Kind: KindPartOf, Basis: BasisVendor, Source: SourceForeman, AgeSeconds: b.age(f.UpdatedAt)})
			b.person()
			b.edge(Edge{From: gid, To: personID, Kind: KindWaitsOn, Basis: BasisVendor, Source: SourceForeman, AgeSeconds: b.age(f.UpdatedAt)})
		}
	}
	return b.g
}

// repoNodes adds the worktree or repo node, its main repo and its branch,
// and returns the ids a session touches.
func (b *builder) repoNodes(r Repo, source string) []string {
	var touched []string
	top := "repo:" + pathKey(r.Root)
	kind := KindRepo
	main := r.Root
	if r.Main != "" {
		top, kind, main = "worktree:"+pathKey(r.Root), KindWorktree, r.Main
	}
	b.node(Node{ID: top, Kind: kind, Label: baseLabel(r.Root), Source: source})
	touched = append(touched, top)
	if r.Main != "" {
		rid := "repo:" + pathKey(r.Main)
		b.node(Node{ID: rid, Kind: KindRepo, Label: baseLabel(r.Main), Source: source})
		b.edge(Edge{From: top, To: rid, Kind: KindPartOf, Basis: BasisVendor, Source: source})
	}
	if r.Branch != "" {
		bid := "branch:" + pathKey(main) + "#" + r.Branch
		b.node(Node{ID: bid, Kind: KindBranch, Label: r.Branch, Source: source})
		touched = append(touched, bid)
	}
	return touched
}

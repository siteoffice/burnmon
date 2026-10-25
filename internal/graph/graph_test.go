package graph

import (
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

// scenario is one Claude Code session in a sprint worktree, with a subagent,
// an MCP server that listens on a port, a shell, and a pending Foreman gate.
func scenario() Input {
	wt := `C:\w\repo\.worktrees\sprint-x`
	return Input{
		Now: t0,
		Sessions: []Session{
			{Vendor: "anthropic", Agent: "claude-code", ID: "S1", Project: wt, State: "waiting", LastActivity: t0.Add(-30 * time.Second)},
			{Vendor: "anthropic", Agent: "claude-code", ID: "agent-a1", ParentID: "S1", Project: wt, State: "coding", LastActivity: t0.Add(-5 * time.Second)},
			{Vendor: "openai", Agent: "codex", ID: "C1", Project: `C:\w\other`, State: "thinking", LastActivity: t0},
		},
		ProcsAt: t0.Add(-1 * time.Second),
		Procs: []Proc{
			{PID: 1, PPID: 0, Name: "pwsh.exe", CreateTime: 10, Harness: ""},
			{PID: 2, PPID: 1, Name: "claude.exe", CreateTime: 100, Harness: "claude"},
			{PID: 3, PPID: 2, Name: "cmd.exe", Cmdline: `cmd.exe /d /s /c "npx ^"-y^" ^"@perplexity-ai/mcp-server^"" --token=SECRET`, CreateTime: 101, Harness: "claude"},
			{PID: 4, PPID: 3, Name: "node.exe", CreateTime: 102, Harness: "claude"},
			{PID: 5, PPID: 2, Name: "bash.exe", CreateTime: 103, Harness: "claude"},
			{PID: 6, PPID: 1, Name: "codex.exe", CreateTime: 200, Harness: "codex"},
			{PID: 7, PPID: 1, Name: "notepad.exe", CreateTime: 300, Harness: "other"},
		},
		Pids: []PidFile{
			{PID: 2, ProcStart: 100, SessionID: "S1", Cwd: wt, Status: "idle", UpdatedAt: t0.Add(-2 * time.Second)},
		},
		MCPFor: func(cwd string) []MCPServer {
			return []MCPServer{{Name: "perplexity", Token: "@perplexity-ai/mcp-server"}, {Name: "github", Token: "@modelcontextprotocol/server-github"}}
		},
		Listens: []Listen{{PID: 4, Proto: "tcp", Port: 8020}, {PID: 7, Proto: "tcp", Port: 9999}},
		RepoFor: func(cwd string) (Repo, bool) {
			if strings.EqualFold(cwd, wt) {
				return Repo{Root: wt, Main: `C:\w\repo`, Branch: "sprint/x"}, true
			}
			return Repo{}, false
		},
		Sprints: []Sprint{
			{ID: "p__x__1", Slug: "x", WorktreePath: wt + `\`, Branch: "sprint/x", Status: "running", UpdatedAt: t0.Add(-time.Minute)},
			{ID: "p__old__1", Slug: "old", WorktreePath: `C:\w\repo\.worktrees\old`, Status: "passing", UpdatedAt: t0.Add(-48 * time.Hour)},
		},
		Flows: []Flow{
			{ID: "f1", Slug: "flow-one", Status: "awaiting-owner", Gate: "G4", GatePending: true, UpdatedAt: t0.Add(-time.Hour)},
			{ID: "f2", Slug: "flow-two", Status: "closed", UpdatedAt: t0.Add(-time.Hour)},
		},
		SubagentFor: func(parent, id string) (SubagentInfo, bool) {
			if parent == "S1" && id == "a1" {
				return SubagentInfo{AgentType: "Explore", Model: "sonnet"}, true
			}
			return SubagentInfo{}, false
		},
	}
}

func edge(g Graph, from, to, kind string) (Edge, bool) {
	for _, e := range g.Edges {
		if e.From == from && e.To == to && e.Kind == kind {
			return e, true
		}
	}
	return Edge{}, false
}

func node(g Graph, id string) (Node, bool) {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}

func TestBuildEdges(t *testing.T) {
	g := Build(scenario())
	s1, sub := "session:anthropic:S1", "session:anthropic:agent-a1"
	claudeProc, mcpProc := "process:2:100", "process:3:101"
	cases := []struct{ from, to, kind, basis, source string }{
		{"harness:claude", "device:local", KindRunsOn, BasisVendor, SourceStore},
		{s1, "harness:claude", KindRunsIn, BasisVendor, SourceStore},
		{s1, sub, KindSpawned, BasisVendor, SourceStore},
		{s1, claudeProc, KindRunsIn, BasisVendor, SourceClaudePids},
		{claudeProc, mcpProc, KindStarted, BasisVendor, SourceSysmon},
		{claudeProc, "process:5:103", KindStarted, BasisVendor, SourceSysmon},
		{mcpProc, "process:4:102", KindStarted, BasisVendor, SourceSysmon},
		{s1, "mcp:perplexity", KindUses, BasisInferred, SourceMCPConfig},
		{"mcp:perplexity", mcpProc, KindRunsAs, BasisInferred, SourceMCPConfig},
		{"process:4:102", "port:tcp:8020", KindListensOn, BasisVendor, SourceNetTables},
		{s1, "person:you", KindWaitsOn, BasisVendor, SourceClaudePids},
		{s1, "sprint:p__x__1", KindWorksFor, BasisInferred, SourceForeman},
		{"sprint:p__x__1", `worktree:c:\w\repo\.worktrees\sprint-x`, KindTouches, BasisVendor, SourceForeman},
		{s1, `worktree:c:\w\repo\.worktrees\sprint-x`, KindTouches, BasisInferred, SourceGit},
		{s1, `branch:c:\w\repo#sprint/x`, KindTouches, BasisInferred, SourceGit},
		{`worktree:c:\w\repo\.worktrees\sprint-x`, `repo:c:\w\repo`, KindPartOf, BasisVendor, SourceGit},
		{"gate:f1:G4", "flow:f1", KindPartOf, BasisVendor, SourceForeman},
		{"gate:f1:G4", "person:you", KindWaitsOn, BasisVendor, SourceForeman},
		{"session:openai:C1", "harness:codex", KindRunsIn, BasisVendor, SourceStore},
		{"process:6:200", "harness:codex", KindRunsIn, BasisVendor, SourceSysmon},
	}
	for _, c := range cases {
		e, ok := edge(g, c.from, c.to, c.kind)
		if !ok {
			t.Errorf("missing edge %s -%s-> %s", c.from, c.kind, c.to)
			continue
		}
		if e.Basis != c.basis || e.Source != c.source {
			t.Errorf("edge %s -%s-> %s: basis %q source %q, want %q %q", c.from, c.kind, c.to, e.Basis, e.Source, c.basis, c.source)
		}
	}

	for _, absent := range [][3]string{
		{s1, "mcp:github", KindUses},                              // configured, not running
		{"process:7:300", "port:tcp:9999", KindListensOn},         // not in a harness tree
		{"session:openai:C1", "person:you", KindWaitsOn},          // not waiting
		{"session:anthropic:agent-a1", "person:you", KindWaitsOn}, // not waiting
	} {
		if _, ok := edge(g, absent[0], absent[1], absent[2]); ok {
			t.Errorf("unexpected edge %v", absent)
		}
	}
	for _, id := range []string{"process:7:300", "process:1:10", "sprint:p__old__1", "flow:f2"} {
		if _, ok := node(g, id); ok {
			t.Errorf("unexpected node %s", id)
		}
	}
}

func TestBuildNodes(t *testing.T) {
	g := Build(scenario())
	sub, ok := node(g, "session:anthropic:agent-a1")
	if !ok || sub.Kind != KindSubagent {
		t.Fatalf("subagent node = %+v, %v", sub, ok)
	}
	if sub.Attrs["agent_type"] != "Explore" || sub.Attrs["model"] != "sonnet" {
		t.Errorf("subagent attrs = %v", sub.Attrs)
	}
	if sub.AgeSeconds != 5 {
		t.Errorf("subagent age = %d, want 5", sub.AgeSeconds)
	}
	dev, _ := node(g, "device:local")
	if dev.Label != "This PC" {
		t.Errorf("device label = %q", dev.Label)
	}
	for _, n := range g.Nodes {
		for k, v := range n.Attrs {
			if strings.Contains(v, "SECRET") || k == "cmdline" {
				t.Fatalf("node %s carries a command line: %s=%s", n.ID, k, v)
			}
		}
	}
}

// Every edge points at nodes that are in the graph, and no edge is listed
// twice.
func TestBuildEdgesReferenceNodes(t *testing.T) {
	g := Build(scenario())
	ids := map[string]bool{}
	for _, n := range g.Nodes {
		if ids[n.ID] {
			t.Errorf("duplicate node %s", n.ID)
		}
		ids[n.ID] = true
	}
	seen := map[string]bool{}
	for _, e := range g.Edges {
		if !ids[e.From] || !ids[e.To] {
			t.Errorf("edge %s -%s-> %s points outside the graph", e.From, e.Kind, e.To)
		}
		k := e.Key()
		if seen[k] {
			t.Errorf("duplicate edge %s", k)
		}
		seen[k] = true
		if e.FirstSeen == "" || e.LastSeen == "" {
			t.Errorf("edge %s has no first_seen or last_seen", k)
		}
	}
}

// A pid file whose process start time does not match the running process
// is a reused pid: no edge from the session to that process.
func TestBuildPidReuse(t *testing.T) {
	in := scenario()
	in.Pids[0].ProcStart = 999
	g := Build(in)
	if _, ok := edge(g, "session:anthropic:S1", "process:2:100", KindRunsIn); ok {
		t.Fatal("session linked to a reused pid")
	}
	if _, ok := edge(g, "session:anthropic:S1", "person:you", KindWaitsOn); !ok {
		t.Fatal("waits_on from the stage should remain")
	}
	e, _ := edge(g, "session:anthropic:S1", "person:you", KindWaitsOn)
	if e.Basis != BasisInferred || e.Source != SourceStore {
		t.Errorf("waits_on without a pid file: basis %q source %q, want inferred store", e.Basis, e.Source)
	}
}

// A session running in Claude Code with no turn in the store yet still
// shows, from its pid file.
func TestBuildSessionFromPidFileOnly(t *testing.T) {
	in := scenario()
	in.Sessions = nil
	g := Build(in)
	n, ok := node(g, "session:anthropic:S1")
	if !ok || n.Source != SourceClaudePids {
		t.Fatalf("session from pid file = %+v, %v", n, ok)
	}
}

// Empty input still gives arrays, never nil, and only the device node.
func TestBuildEmpty(t *testing.T) {
	g := Build(Input{Now: t0})
	if g.Nodes == nil || g.Edges == nil {
		t.Fatal("nil lists")
	}
	if len(g.Nodes) != 1 || g.Nodes[0].ID != "device:local" {
		t.Fatalf("nodes = %+v", g.Nodes)
	}
}

func TestStampFirstSeen(t *testing.T) {
	g := Build(scenario())
	earlier := t0.Add(-time.Hour)
	g.Stamp(map[string]time.Time{g.Edges[0].Key(): earlier})
	if g.Edges[0].FirstSeen != earlier.Format(time.RFC3339) {
		t.Fatalf("first_seen = %s", g.Edges[0].FirstSeen)
	}
	if g.Edges[1].FirstSeen != t0.Format(time.RFC3339) {
		t.Fatalf("unstamped first_seen = %s, want now", g.Edges[1].FirstSeen)
	}
}

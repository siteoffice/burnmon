package graph

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadPidFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "17420.json"), `{"pid":17420,"sessionId":"s-1","cwd":"C:\\w","procStart":"134372210257685063","status":"busy","updatedAt":1792747430934,"kind":"interactive"}`)
	write(t, filepath.Join(dir, "17420.abc.key"), `not json`)
	got, err := ReadPidFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d pid files, want 1 (the .key file is not one)", len(got))
	}
	p := got[0]
	if p.PID != 17420 || p.SessionID != "s-1" || p.ProcStart != 134372210257685063 || p.Status != "busy" || p.Cwd != `C:\w` {
		t.Fatalf("pid file = %+v", p)
	}
	if !p.UpdatedAt.Equal(time.UnixMilli(1792747430934)) {
		t.Fatalf("updatedAt = %v", p.UpdatedAt)
	}
}

// A missing folder is no Claude Code here: no error, no rows.
func TestReadPidFilesMissingDir(t *testing.T) {
	got, err := ReadPidFiles(filepath.Join(t.TempDir(), "nope"))
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

// A changed format drops every pid file: the source is reported not ok, and
// no half-read edge reaches the graph.
func TestReadPidFilesFormatChange(t *testing.T) {
	for name, body := range map[string]string{
		"no procStart":  `{"pid":1,"sessionId":"s"}`,
		"procStart num": `{"pid":1,"sessionId":"s","procStart":5}`,
		"broken json":   `{"pid":`,
	} {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "1.json"), body)
		write(t, filepath.Join(dir, "2.json"), `{"pid":2,"sessionId":"s2","procStart":"7"}`)
		got, err := ReadPidFiles(dir)
		if err == nil || got != nil {
			t.Errorf("%s: got %v, %v; want nil and an error", name, got, err)
		}
	}
}

func TestSubagentMeta(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "C--w", "parent-1", "subagents", "agent-a1.meta.json"),
		`{"agentType":"Explore","description":"never read","toolUseId":"toolu_1","spawnDepth":1,"model":"sonnet"}`)
	m := NewSubagentMeta(root)
	info, ok := m.Lookup("parent-1", "a1")
	if !ok || info.AgentType != "Explore" || info.Model != "sonnet" {
		t.Fatalf("lookup = %+v, %v", info, ok)
	}
	if _, ok := m.Lookup("parent-1", "zz"); ok {
		t.Fatal("found a meta file that does not exist")
	}
}

func TestMCPConfig(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".claude.json"), `{
 "mcpServers": {
  "perplexity": {"type":"stdio","command":"npx","args":["-y","@perplexity-ai/mcp-server"]},
  "filesystem": {"type":"stdio","command":"C:\\Users\\x\\mcp-filesystem.bat","args":[]},
  "graph": {"type":"stdio","command":"python.exe","args":["-m","graphify.serve","C:\\dev\\a\\graph.json"]},
  "remote": {"type":"http","url":"https://example.invalid/mcp"}
 },
 "projects": {"C:/w/proj": {"mcpServers": {"context7": {"command":"npx","args":["-y","@upstash/context7-mcp@latest"]}}}}
}`)
	write(t, filepath.Join(home, "proj2", ".mcp.json"), `{"mcpServers":{"local":{"command":"uvx","args":["local-mcp"]}}}`)
	c := NewMCPConfig(filepath.Join(home, ".claude.json"))
	tokens := func(cwd string) map[string]string {
		m := map[string]string{}
		for _, s := range c.For(cwd) {
			m[s.Name] = s.Token
		}
		return m
	}
	g := tokens(`C:\elsewhere`)
	want := map[string]string{"perplexity": "@perplexity-ai/mcp-server", "filesystem": "mcp-filesystem.bat", "graph": `C:\dev\a\graph.json`}
	for k, v := range want {
		if g[k] != v {
			t.Errorf("token %s = %q, want %q", k, g[k], v)
		}
	}
	if _, ok := g["remote"]; ok {
		t.Error("an http server has no local process and no token")
	}
	if tokens(`C:\w\proj`)["context7"] != "@upstash/context7-mcp@latest" {
		t.Error("project server missing")
	}
	if tokens(filepath.Join(home, "proj2"))["local"] != "local-mcp" {
		t.Error(".mcp.json server missing")
	}
	if c.Err() != nil {
		t.Fatal(c.Err())
	}
}

func TestReadForeman(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "sprints", "p", "x", "sprint.json"), `{"sprint_id":"p__x__1","slug":"x","worktree_path":"C:\\w\\.worktrees\\sprint-x","worktree_branch":"sprint/x","phase":"report","status":"waiting-on-you","updated_at":"2026-10-23T10:00:00Z"}`)
	write(t, filepath.Join(root, "sprints", "_lib", "y", "sprint.json"), `{"sprint_id":"lib"}`)
	write(t, filepath.Join(root, "flows", "p", "f", "flow.json"), `{"flow_id":"f-1","slug":"f","status":"awaiting-owner","gate":{"id":"G4","status":"pending"},"updated_at":"2026-10-23T09:00:00Z"}`)
	write(t, filepath.Join(root, "flows", "p", "f", "foreman.json"), `{"session_id":"s-9"}`)
	write(t, filepath.Join(root, "flows", "p", "g", "flow.json"), `{"flow_id":"g-1","slug":"g","status":"closed","gate":{"id":"G2","status":"decided"}}`)
	sprints, flows, err := ReadForeman(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sprints) != 1 || sprints[0].ID != "p__x__1" || sprints[0].Branch != "sprint/x" || sprints[0].Status != "waiting-on-you" {
		t.Fatalf("sprints = %+v", sprints)
	}
	if !sprints[0].UpdatedAt.Equal(time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("updated_at = %v", sprints[0].UpdatedAt)
	}
	if len(flows) != 2 {
		t.Fatalf("flows = %+v", flows)
	}
	for _, f := range flows {
		switch f.ID {
		case "f-1":
			if !f.GatePending || f.Gate != "G4" || f.SessionID != "s-9" {
				t.Errorf("f-1 = %+v", f)
			}
		case "g-1":
			if f.GatePending {
				t.Errorf("a decided gate is not pending: %+v", f)
			}
		}
	}
}

func TestReadForemanMissingRoot(t *testing.T) {
	if _, _, err := ReadForeman(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("a configured Foreman folder that does not exist is an error")
	}
}

func TestRepoLocator(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	write(t, filepath.Join(repo, ".git", "HEAD"), "ref: refs/heads/main\n")
	write(t, filepath.Join(repo, ".git", "worktrees", "wt1", "HEAD"), "ref: refs/heads/sprint/x\n")
	wt := filepath.Join(repo, ".worktrees", "wt1")
	write(t, filepath.Join(wt, ".git"), "gitdir: "+filepath.Join(repo, ".git", "worktrees", "wt1")+"\n")
	if err := os.MkdirAll(filepath.Join(repo, "sub", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	l := NewRepoLocator()

	r, ok := l.For(filepath.Join(repo, "sub", "deep"))
	if !ok || r.Root != repo || r.Main != "" || r.Branch != "main" {
		t.Fatalf("repo = %+v, %v", r, ok)
	}
	r, ok = l.For(wt)
	if !ok || r.Root != wt || r.Main != repo || r.Branch != "sprint/x" {
		t.Fatalf("worktree = %+v, %v", r, ok)
	}
	if _, ok := l.For(root); ok {
		t.Fatal("a folder outside any repo has no repo")
	}
}

// TestReadForemanRealRoot tries a real Foreman folder: set BURNMON_FOREMAN_ROOT.
// It is skipped otherwise and checks nothing but that the folder reads.
func TestReadForemanRealRoot(t *testing.T) {
	root := os.Getenv("BURNMON_FOREMAN_ROOT")
	if root == "" {
		t.Skip("set BURNMON_FOREMAN_ROOT to a Foreman folder to try a real one")
	}
	sprints, flows, err := ReadForeman(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sprints {
		t.Logf("sprint %s status %s worktree %q main %q log entries %d", s.ID, s.Status, s.WorktreePath, sprintMainRepo(s.WorktreePath), len(s.LogTimes))
	}
	t.Logf("%d sprints, %d flows", len(sprints), len(flows))
}

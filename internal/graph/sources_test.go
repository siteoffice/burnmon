package graph

import (
	"os"
	"path/filepath"
	"strings"
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
	// A subagent that starts later is found through the remembered folder,
	// and a parent in another project folder still through the scan.
	write(t, filepath.Join(root, "C--w", "parent-1", "subagents", "agent-a2.meta.json"), `{"agentType":"Plan","model":"opus"}`)
	write(t, filepath.Join(root, "C--x", "parent-2", "subagents", "agent-b1.meta.json"), `{"agentType":"general","model":"haiku"}`)
	if info, ok := m.Lookup("parent-1", "a2"); !ok || info.AgentType != "Plan" {
		t.Fatalf("later subagent = %+v, %v", info, ok)
	}
	if info, ok := m.Lookup("parent-2", "b1"); !ok || info.Model != "haiku" {
		t.Fatalf("other project = %+v, %v", info, ok)
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
		t.Logf("sprint %s status %s worktree %q main %q log entries %d", s.ID, s.Status, s.WorktreePath, SprintMainRepo(s.WorktreePath), len(s.LogTimes))
	}
	t.Logf("%d sprints, %d flows", len(sprints), len(flows))
}

func TestReadForemanResultsAndPaths(t *testing.T) {
	root := t.TempDir()
	wdir := filepath.Join(root, "sprints", "p", "waiting")
	write(t, filepath.Join(wdir, "sprint.json"), `{"sprint_id":"p__waiting__1","slug":"waiting","project":"example","status":"waiting-on-you","updated_at":"2026-10-23T09:10:00Z","spec_path":"C:\\example\\specs\\w.md","tasks":[{"n":1,"title":"one","state":"done"},{"n":2,"title":"two","state":"parked"}],"last_verify_result":{"outcome":"fail","summary":"verify broke","at":"2026-10-23T09:00:00Z"}}`)
	write(t, filepath.Join(wdir, "report.md"), "# report\n")
	write(t, filepath.Join(wdir, "action-log.ndjson"), `{"ts":"2026-10-23T09:30:00Z","action":"task_parked","task":2,"detail":"needs a key"}`+"\n")
	ddir := filepath.Join(root, "sprints", "p", "done")
	write(t, filepath.Join(ddir, "sprint.json"), `{"sprint_id":"p__done__1","slug":"done","phase":"done","status":"passing","updated_at":"2026-10-23T09:20:00Z","report_path":"C:\\example\\r.md","last_test_result":{"exit_code":0,"summary":"ok","at":"2026-10-23T09:15:00Z"}}`)
	write(t, filepath.Join(root, "flows", "p", "open", "flow.json"), `{"flow_id":"f-open","slug":"open","project":"example","status":"awaiting-owner","spec_path":"C:\\example\\specs\\f.md","gate":{"id":"G4","status":"awaiting-owner","opened_at":"2026-10-23T08:00:00Z"},"updated_at":"2026-10-23T09:00:00Z"}`)
	write(t, filepath.Join(root, "flows", "p", "decided", "flow.json"), `{"flow_id":"f-dec","slug":"decided","gate":{"id":"G2","status":"decided"}}`)
	sprints, flows, err := ReadForeman(root)
	if err != nil {
		t.Fatal(err)
	}
	var w, d Sprint
	for _, s := range sprints {
		switch s.ID {
		case "p__waiting__1":
			w = s
		case "p__done__1":
			d = s
		}
	}
	if w.Project != "example" || w.SpecPath != `C:\example\specs\w.md` || w.Dir != wdir || w.ResultPath != filepath.Join(wdir, "report.md") {
		t.Errorf("waiting sprint = %+v", w)
	}
	at := time.Date(2026, 10, 23, 9, 30, 0, 0, time.UTC)
	if len(w.Parked) != 1 || w.Parked[0].N != 2 || w.Parked[0].Title != "two" || w.Parked[0].Reason != "needs a key" || !w.Parked[0].At.Equal(at) {
		t.Errorf("parked = %+v", w.Parked)
	}
	if !w.LastParkedAt.Equal(at) {
		t.Errorf("LastParkedAt = %v", w.LastParkedAt)
	}
	if w.Verify == nil || !w.Verify.Failed || w.Verify.Label != "verify fail" || w.Verify.Summary != "verify broke" || w.Test != nil {
		t.Errorf("checks = %+v %+v", w.Verify, w.Test)
	}
	if d.ResultPath != `C:\example\r.md` || d.Test == nil || d.Test.Failed || d.Test.Label != "test exit 0" {
		t.Errorf("done sprint = %+v", d)
	}
	for _, f := range flows {
		switch f.ID {
		case "f-open":
			if !f.GatePending || !f.GateOpenedAt.Equal(time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)) || f.SpecPath != `C:\example\specs\f.md` || f.Project != "example" {
				t.Errorf("open flow = %+v", f)
			}
		case "f-dec":
			if f.GatePending {
				t.Errorf("decided flow pending: %+v", f)
			}
		}
	}
}

func TestReadForemanOddFieldsAreSkipped(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "sprints", "p", "odd", "sprint.json"), `{"sprint_id":"p__odd__1","slug":"odd","last_test_result":"x","last_verify_result":{"outcome":3},"tasks":{"n":1},"spec_path":5}`)
	write(t, filepath.Join(root, "sprints", "p", "half", "sprint.json"), `{"sprint_id": "p__x`)
	write(t, filepath.Join(root, "sprints", "p", "fine", "sprint.json"), `{"sprint_id":"p__fine__1","slug":"fine"}`)
	write(t, filepath.Join(root, "flows", "p", "odd", "flow.json"), `{"flow_id":"f-odd","slug":"odd","gate":{"id":"G2","opened_at":5}}`)
	sprints, flows, err := ReadForeman(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sprints) != 2 {
		t.Fatalf("sprints = %+v", sprints)
	}
	for _, s := range sprints {
		if s.Test != nil || s.Verify != nil || len(s.Parked) != 0 {
			t.Errorf("odd fields kept: %+v", s)
		}
	}
	if len(flows) != 1 || flows[0].Gate != "G2" || !flows[0].GateOpenedAt.IsZero() {
		t.Fatalf("flows = %+v", flows)
	}
}

func TestReadForemanParkedReasonSurvivesBadLines(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sprints", "p", "x")
	write(t, filepath.Join(dir, "sprint.json"), `{"sprint_id":"p__x__1","slug":"x","tasks":[{"n":2,"title":"two","state":"parked"}]}`)
	log := strings.Repeat("a", 300000) + "\n" + strings.Repeat("b", 1100000) + "\n" + "not json\n" +
		`{"ts":"2026-10-23T09:40:00Z","action":"task_parked","task":2,"detail":"newer"}` + "\n" +
		`{"ts":"2026-10-23T09:30:00Z","action":"task_parked","task":2,"detail":"older"}` + "\n"
	write(t, filepath.Join(dir, "action-log.ndjson"), log)
	sprints, _, err := ReadForeman(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sprints) != 1 || len(sprints[0].Parked) != 1 || sprints[0].Parked[0].Reason != "newer" {
		t.Fatalf("sprints = %+v", sprints)
	}
}

func TestReadForemanGateToken(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "flows", "p", "a", "flow.json"), `{"flow_id":"f-a","slug":"a","gate":{"id":"G2","status":"awaiting-owner","token":"t1"}}`)
	write(t, filepath.Join(root, "flows", "p", "b", "flow.json"), `{"flow_id":"f-b","slug":"b","gate":{"id":"G2","status":"awaiting-owner","token":5}}`)
	_, flows, err := ReadForeman(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 2 {
		t.Fatalf("flows = %+v", flows)
	}
	for _, f := range flows {
		switch f.ID {
		case "f-a":
			if f.GateToken != "t1" || !f.GatePending {
				t.Errorf("f-a = %+v", f)
			}
		case "f-b":
			if f.GateToken != "" || f.Gate != "G2" {
				t.Errorf("f-b = %+v", f)
			}
		}
	}
}

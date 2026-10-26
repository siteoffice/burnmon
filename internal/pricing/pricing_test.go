package pricing

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOwnerForEmptyRulesMeansNoOwner guards P6's default: an empty Owners
// table means one owner, no owner column shown anywhere, so OwnerFor must
// return "" for every path rather than "personal".
func TestOwnerForEmptyRulesMeansNoOwner(t *testing.T) {
	cfg := Defaults()
	if got := cfg.OwnerFor(`C:\dev\Work\project`); got != "" {
		t.Fatalf("OwnerFor with no rules = %q, want empty", got)
	}
}

// TestOwnerForOrderedRulesFirstMatchWins guards the ordered-rule matching
// and the "personal" default once rules exist.
func TestOwnerForOrderedRulesFirstMatchWins(t *testing.T) {
	cfg := Defaults()
	cfg.Owners = []OwnerRule{
		{Match: `C:\dev\Work\*`, Owner: "ClientA"},
		{Match: `C:\ZND\*`, Owner: "ZND"},
	}

	cases := []struct {
		path string
		want string
	}{
		{`C:\dev\Work\project\file.go`, "ClientA"},
		{`C:\ZND\projects\burnmon`, "ZND"},
		// Case-insensitive match, same as the rest of the codebase's
		// Windows-path handling (dataset.adapterForPath and friends).
		{`c:\znd\projects\burnmon`, "ZND"},
		{`C:\Users\wilco\other`, "personal"},
	}
	for _, c := range cases {
		if got := cfg.OwnerFor(c.path); got != c.want {
			t.Errorf("OwnerFor(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// TestClientForRemoteBeatsPath guards K1's match order: a rule matched by
// git remote wins over a rule matched by path, regardless of list order.
func TestClientForRemoteBeatsPath(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, "repo", ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgText := "[remote \"origin\"]\n\turl = git@github.com:multica-ai/burnmon.git\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(cfgText), 0o644); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(root, "repo")

	cfg := Defaults()
	cfg.Owners = []OwnerRule{
		{Match: filepath.Join(root, "repo") + `\*`, Owner: "ZND", Client: "PathClient"},
		{Remote: "github.com/multica-ai/burnmon", Owner: "ZND", Client: "RemoteClient"},
	}
	if got := cfg.ClientFor(projectPath); got != "RemoteClient" {
		t.Fatalf("ClientFor = %q, want RemoteClient (remote rule beats path rule regardless of list order)", got)
	}
}

// TestClientForPathFallbackWhenNoRemoteMatches guards the path-rule fallback
// when no remote rule matches the project's git remote.
func TestClientForPathFallbackWhenNoRemoteMatches(t *testing.T) {
	cfg := Defaults()
	cfg.Owners = []OwnerRule{
		{Remote: "github.com/some/other-repo", Owner: "ZND", Client: "OtherClient"},
		{Match: `C:\ZND\projects\*`, Owner: "ZND", Client: "PathClient"},
	}
	if got := cfg.ClientFor(`C:\ZND\projects\burnmon`); got != "PathClient" {
		t.Fatalf("ClientFor = %q, want PathClient (no remote match, path rule wins)", got)
	}
}

// TestClientForUnassignedWhenNothingMatches guards K1's default: a
// configured client map with no matching rule returns "unassigned".
func TestClientForUnassignedWhenNothingMatches(t *testing.T) {
	cfg := Defaults()
	cfg.Owners = []OwnerRule{
		{Match: `C:\dev\Work\*`, Owner: "ClientA", Client: "ClientA-client"},
	}
	if got := cfg.ClientFor(`C:\Users\me\scratch`); got != "unassigned" {
		t.Fatalf("ClientFor = %q, want unassigned", got)
	}
}

// TestClientForEmptyOwnersReturnsUnassigned guards the no-rules-configured
// default (mirrors TestOwnerForEmptyRulesMeansNoOwner's empty-Owners case).
func TestClientForEmptyOwnersReturnsUnassigned(t *testing.T) {
	cfg := Defaults()
	if got := cfg.ClientFor(`C:\anything`); got != "unassigned" {
		t.Fatalf("ClientFor with no owner rules = %q, want unassigned", got)
	}
}

// TestOwnerForRemoteOnlyRuleDoesNotMatchEveryPath guards the Critical
// review finding: a remote-only rule (no Match) must not become an
// empty-prefix "match everything" rule for OwnerFor. A path with no git
// remote at all must fall through to the next rule (or "personal"), never
// pick up the remote-only rule's Owner.
func TestOwnerForRemoteOnlyRuleDoesNotMatchEveryPath(t *testing.T) {
	cfg := Defaults()
	cfg.Owners = []OwnerRule{
		{Remote: "github.com/multica-ai/dsi", Owner: "ZND", Client: "ClientB"},
		{Match: `C:\dev\Work\*`, Owner: "ClientA"},
	}
	if got := cfg.OwnerFor(`C:\dev\Work\project`); got != "ClientA" {
		t.Fatalf("OwnerFor = %q, want ClientA (a remote-only rule must not match a path with no git remote)", got)
	}
	if got := cfg.OwnerFor(`C:\Users\me\scratch`); got != "personal" {
		t.Fatalf("OwnerFor = %q, want personal (no rule matches: not even the remote-only one)", got)
	}
}

// TestMatchRuleUnifiesOwnerAndClientFromOneWinningRule guards Important 3:
// a rule matched by remote decides both Owner and Client together, so a
// project cannot end up with one rule's owner and a different rule's
// client.
func TestMatchRuleUnifiesOwnerAndClientFromOneWinningRule(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, "repo", ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgText := "[remote \"origin\"]\n\turl = https://github.com/multica-ai/dsi.git\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(cfgText), 0o644); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(root, "repo")

	cfg := Defaults()
	cfg.Owners = []OwnerRule{
		{Match: filepath.Join(root, "repo") + `\*`, Owner: "ClientA"},
		{Remote: "github.com/multica-ai/dsi", Owner: "ZND", Client: "ClientB"},
	}
	if got := cfg.OwnerFor(projectPath); got != "ZND" {
		t.Fatalf("OwnerFor = %q, want ZND (the remote rule wins the match, so it decides owner too, not the path rule)", got)
	}
	if got := cfg.ClientFor(projectPath); got != "ClientB" {
		t.Fatalf("ClientFor = %q, want ClientB", got)
	}
}

// TestRemoteMatchesWholeSegmentNotBareSubstring guards Important 2: a
// remote pattern for one repo must not also match a sibling repo whose
// name has it as a prefix (dsi vs dsi-engine).
func TestRemoteMatchesWholeSegmentNotBareSubstring(t *testing.T) {
	if remoteMatches("https://github.com/multica-ai/dsi-engine.git", "github.com/multica-ai/dsi") {
		t.Fatal("remoteMatches matched dsi-engine against a dsi pattern, want whole-segment match only")
	}
	if !remoteMatches("https://github.com/multica-ai/dsi.git", "github.com/multica-ai/dsi") {
		t.Fatal("remoteMatches should match the exact repo the pattern names")
	}
	if !remoteMatches("git@github.com:multica-ai/dsi.git", "github.com/multica-ai/dsi") {
		t.Fatal("remoteMatches should match an SSH remote against an https-shaped pattern")
	}
}

// TestClientForRemoteRuleWithEmptyClientIsUnassigned guards a matched
// remote rule that sets no Client: it still wins the match (for Owner) but
// answers "unassigned" for client, rather than an empty string.
func TestClientForRemoteRuleWithEmptyClientIsUnassigned(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, "repo", ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgText := "[remote \"origin\"]\n\turl = https://github.com/multica-ai/burnmon.git\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(cfgText), 0o644); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(root, "repo")

	cfg := Defaults()
	cfg.Owners = []OwnerRule{
		{Remote: "github.com/multica-ai/burnmon", Owner: "ZND"},
	}
	if got := cfg.ClientFor(projectPath); got != "unassigned" {
		t.Fatalf("ClientFor = %q, want unassigned (matched rule sets no Client)", got)
	}
	if got := cfg.OwnerFor(projectPath); got != "ZND" {
		t.Fatalf("OwnerFor = %q, want ZND (the rule still wins the match)", got)
	}
}

// TestClientForHTTPSRemoteEndToEnd guards ClientFor's own remote path (not
// just the lower-level remoteMatches helper) against an https-cloned repo.
func TestClientForHTTPSRemoteEndToEnd(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, "repo", ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgText := "[remote \"origin\"]\n\turl = https://github.com/multica-ai/burnmon.git\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(cfgText), 0o644); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(root, "repo")

	cfg := Defaults()
	cfg.Owners = []OwnerRule{
		{Remote: "github.com/multica-ai/burnmon", Owner: "ZND", Client: "HTTPSClient"},
	}
	if got := cfg.ClientFor(projectPath); got != "HTTPSClient" {
		t.Fatalf("ClientFor = %q, want HTTPSClient", got)
	}
}

// TestLoadV02ConfigWithOwnersOnlyIsUnchanged guards K1's compatibility
// promise: a v0.2 burnmon.json with owners-only rules (no client, no
// remote) loads unchanged and OwnerFor keeps matching exactly as before.
func TestLoadV02ConfigWithOwnersOnlyIsUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "burnmon.json")
	v02JSON := `{
  "owners": [
    {"match": "C:\\dev\\Work\\*", "owner": "ClientA"},
    {"match": "C:\\ZND\\*", "owner": "ZND"}
  ]
}`
	if err := os.WriteFile(path, []byte(v02JSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Owners) != 2 {
		t.Fatalf("got %d owner rules, want 2", len(cfg.Owners))
	}
	if cfg.Owners[0].Owner != "ClientA" || cfg.Owners[0].Client != "" || cfg.Owners[0].Remote != "" {
		t.Fatalf("owner rule 0 = %+v, want Owner=ClientA, Client and Remote empty", cfg.Owners[0])
	}
	if got := cfg.OwnerFor(`C:\dev\Work\project`); got != "ClientA" {
		t.Fatalf("OwnerFor = %q, want ClientA (v0.2 owner matching unchanged)", got)
	}
	if got := cfg.ClientFor(`C:\dev\Work\project`); got != "unassigned" {
		t.Fatalf("ClientFor = %q, want unassigned (no client set on any v0.2 rule)", got)
	}
}

// TestLoadModeAndCopilotPlan guards K3's burnmon.json field "copilot_plan"
// (which of the credit book's tiers the account is actually on), and that a
// stale "mode" key from a pre-cleanup config still loads without error (the
// dev/business toggle it used to drive is gone, but Load must not choke on
// the unknown key).
func TestLoadModeAndCopilotPlan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "burnmon.json")
	j := `{"mode": "business", "copilot_plan": "Pro"}`
	if err := os.WriteFile(path, []byte(j), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CopilotPlan != "Pro" {
		t.Errorf("CopilotPlan = %q, want %q", cfg.CopilotPlan, "Pro")
	}
}

// TestLoadKeepHistory guards B1's default: keep_history is on unless
// burnmon.json says false.
func TestLoadKeepHistory(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.KeepHistory {
		t.Fatal("Load(\"\").KeepHistory = false, want true")
	}
	dir := t.TempDir()
	for _, tc := range []struct {
		name, body string
		want       bool
	}{
		{"absent", `{"fx_usd_eur": 0.9}`, true},
		{"false", `{"keep_history": false}`, false},
	} {
		p := filepath.Join(dir, tc.name+".json")
		if err := os.WriteFile(p, []byte(tc.body), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := Load(p)
		if err != nil {
			t.Fatal(err)
		}
		if got.KeepHistory != tc.want {
			t.Errorf("%s: KeepHistory = %v, want %v", tc.name, got.KeepHistory, tc.want)
		}
	}
}

func TestAgentViewKey(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentView {
		t.Fatal("Load(\"\").AgentView = true, want false")
	}
	dir := t.TempDir()
	for _, tc := range []struct {
		name, body string
		want       bool
	}{
		{"absent", `{"fx_usd_eur": 0.9}`, false},
		{"true", `{"agent_view": true}`, true},
	} {
		p := filepath.Join(dir, tc.name+".json")
		if err := os.WriteFile(p, []byte(tc.body), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := Load(p)
		if err != nil {
			t.Fatal(err)
		}
		if got.AgentView != tc.want {
			t.Errorf("%s: AgentView = %v, want %v", tc.name, got.AgentView, tc.want)
		}
	}
}

func TestClaudePathKey(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClaudePath != "" {
		t.Fatalf("Load(\"\").ClaudePath = %q, want empty", cfg.ClaudePath)
	}
	dir := t.TempDir()
	for _, tc := range []struct{ name, body, want string }{
		{"absent", `{"fx_usd_eur": 0.9}`, ""},
		{"set", `{"claude_path": "C:\\example\\claude.exe"}`, `C:\example\claude.exe`},
	} {
		p := filepath.Join(dir, tc.name+".json")
		if err := os.WriteFile(p, []byte(tc.body), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := Load(p)
		if err != nil {
			t.Fatal(err)
		}
		if got.ClaudePath != tc.want {
			t.Errorf("%s: ClaudePath = %q, want %q", tc.name, got.ClaudePath, tc.want)
		}
	}
}

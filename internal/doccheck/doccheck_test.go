package doccheck

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// md turns apostrophes into backticks so test documents stay readable.
func md(s string) string { return strings.ReplaceAll(s, "'", "`") }

func set(items ...string) map[string]bool {
	m := map[string]bool{}
	for _, i := range items {
		m[i] = true
	}
	return m
}

// fakeCorpus has every item of realDoc and a root with one real file.
func fakeCorpus(t *testing.T) Corpus {
	t.Helper()
	root := t.TempDir()
	p := filepath.Join(root, "internal", "graph", "graph.go")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("package graph\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return Corpus{
		Root:        root,
		Keys:        set("keep_history", "burn_30min"),
		Subcommands: set("waits", "graph"),
		Flags:       set("foreman", "since"),
		Bindings:    set("bdevOpenItemFile"),
		Hotkeys:     set("m"),
		UICheck:     set("d36"),
		PartIDs:     set("agents_map"),
		GroupIDs:    set("status"),
	}
}

// realDoc holds one claim of each kind, with the section table last.
const realDoc = `Set 'keep_history' and read 'internal\graph\graph.go'.
Run 'burnmon-cli waits -foreman <dir>' then call 'bdevOpenItemFile'.
Run 'uicheck.ps1' with 'd36' and the 'burn_30min' part.

- **M** opens the map

| id | label | group |
|---|---|---|
| 'agents_map' | Agents map | status |`

func hasClaim(cs []Claim, kind, text string, line int) bool {
	for _, c := range cs {
		if c.Kind == kind && c.Text == text && c.Line == line {
			return true
		}
	}
	return false
}

func TestExtractClaims(t *testing.T) {
	cs := Extract(md(realDoc))
	want := []Claim{
		{"key", "keep_history", 1},
		{"path", `internal\graph\graph.go`, 1},
		{"subcommand", "waits", 2},
		{"flag", "foreman", 2},
		{"binding", "bdevOpenItemFile", 2},
		{"uicheck", "d36", 3},
		{"key", "burn_30min", 3},
		{"hotkey", "m", 5},
		{"section", "agents_map", 9},
	}
	for _, w := range want {
		if !hasClaim(cs, w.Kind, w.Text, w.Line) {
			t.Errorf("missing claim %+v in %+v", w, cs)
		}
	}
	if len(cs) != len(want) {
		t.Errorf("got %d claims, want %d: %+v", len(cs), len(want), cs)
	}
}

func TestExtractFencedBlocks(t *testing.T) {
	plain := md("Some text.\n```\nburnmon-cli graph -since 24h\n'burnmon_ghost_key'\n```\n")
	if cs := Extract(plain); len(cs) != 0 {
		t.Errorf("fenced block without 'runs in' gave claims: %+v", cs)
	}
	runs := md("This snippet runs in PowerShell 7:\n```\nburnmon-cli graph -since 24h\n```\n")
	cs := Extract(runs)
	if !hasClaim(cs, "subcommand", "graph", 3) || !hasClaim(cs, "flag", "since", 3) || len(cs) != 2 {
		t.Errorf("fenced block after 'runs in' gave %+v", cs)
	}
}

func TestVerifyAcceptsRealClaims(t *testing.T) {
	c := fakeCorpus(t)
	if fs := Verify("fake.md", md(realDoc), c, nil); len(fs) != 0 {
		t.Errorf("want no findings, got %+v", fs)
	}
}

func TestDocCheckFailsOnPlantedClaims(t *testing.T) {
	c := fakeCorpus(t)
	plants := []struct {
		name, add, kind, text string
		inTable               bool
	}{
		{"key", "Also 'burnmon_bogus_key'.", "key", "burnmon_bogus_key", false},
		{"binding", "Also 'bdevNoSuchBinding'.", "binding", "bdevNoSuchBinding", false},
		{"uicheck", "Also 'd99'.", "uicheck", "d99", false},
		{"flag", "Also 'burnmon-cli waits -no-such-flag'.", "flag", "no-such-flag", false},
		{"path", `Also 'internal\nope\nope.go'.`, "path", `internal\nope\nope.go`, false},
		{"section", "| 'bogus_part' | Bogus | status |", "section", "bogus_part", true},
		{"subcommand", "Also 'burnmon-cli nothing'.", "subcommand", "nothing", false},
		{"hotkey", "- **Z** does nothing", "hotkey", "z", false},
	}
	for _, p := range plants {
		sep := "\n\n"
		if p.inTable {
			sep = "\n"
		}
		fs := Verify("fake.md", md(realDoc+sep+p.add), c, nil)
		if len(fs) != 1 || fs[0].Kind != p.kind || fs[0].Text != p.text || fs[0].Why == "" || fs[0].Doc != "fake.md" || fs[0].Line == 0 {
			t.Errorf("plant %s: want one %s finding for %q, got %+v", p.name, p.kind, p.text, fs)
		}
	}
}

func TestAllowlistEntryNeedsAReason(t *testing.T) {
	c := fakeCorpus(t)
	doc := md(realDoc + "\n\nThe exe is 'C:\\example\\claude.exe'.")
	if fs := Verify("fake.md", doc, c, nil); len(fs) != 1 || fs[0].Kind != "path" {
		t.Fatalf("a runtime path without an allowlist entry must be one finding, got %+v", fs)
	}
	allow := map[string]string{`C:\example\claude.exe`: "invented example path, not in the repo"}
	if fs := Verify("fake.md", doc, c, allow); len(fs) != 0 {
		t.Errorf("allowlisted claim must pass, got %+v", fs)
	}
	if fs := CheckAllowlist(map[string]string{"fake.md": doc}, allow); len(fs) != 0 {
		t.Errorf("used entry with a reason must pass, got %+v", fs)
	}
	bad := map[string]string{`C:\example\claude.exe`: ""}
	if fs := CheckAllowlist(map[string]string{"fake.md": doc}, bad); len(fs) != 1 || fs[0].Kind != "allow" {
		t.Errorf("entry without a reason must be one finding, got %+v", fs)
	}
}

func TestStaleAllowlistEntryFails(t *testing.T) {
	allow := map[string]string{`C:\example\gone.exe`: "used to be in the README"}
	fs := CheckAllowlist(map[string]string{"fake.md": md(realDoc)}, allow)
	if len(fs) != 1 || fs[0].Kind != "allow" || fs[0].Text != `C:\example\gone.exe` {
		t.Errorf("stale entry must be one finding, got %+v", fs)
	}
}

func realRoot(t *testing.T) (string, Corpus) {
	t.Helper()
	root, err := FindRoot()
	if err != nil {
		t.Fatal(err)
	}
	c, err := LoadCorpus(root)
	if err != nil {
		t.Fatal(err)
	}
	return root, c
}

func TestLoadCorpusRejectsMissingRoot(t *testing.T) {
	if _, err := LoadCorpus(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("an unreadable root must be an error, not a pass")
	}
}

func TestRealCorpusHasWhatTheSpecNames(t *testing.T) {
	_, c := realRoot(t)
	for _, k := range []string{"p", "o", "i", "m", "t", "l"} {
		if !c.Hotkeys[k] {
			t.Errorf("hotkey %q missing", k)
		}
	}
	for _, s := range strings.Fields("price-check live reown graph waits backfill-parents tools insight export merge") {
		if !c.Subcommands[s] {
			t.Errorf("subcommand %q missing", s)
		}
	}
	if !c.Bindings["bdevOpenItemFile"] {
		t.Error("binding bdevOpenItemFile missing")
	}
	if !c.PartIDs["agents_map"] || len(c.PartIDs) != 15 {
		t.Errorf("part ids: want 15 incl. agents_map, got %d", len(c.PartIDs))
	}
	for _, id := range []string{"d36", "d37"} {
		if !c.UICheck[id] {
			t.Errorf("uicheck id %s missing", id)
		}
	}
	if !c.Keys["keep_history"] {
		t.Error("config key keep_history missing")
	}
	for _, f := range []string{"foreman", "since", "apply"} {
		if !c.Flags[f] {
			t.Errorf("flag %q missing", f)
		}
	}
}

// allowlist holds claims that are not repo files or code strings, each with a reason.
var allowlist = map[string]string{
	`~\.burnmon`:                           "runtime folder in the user's home, made by BurnMon at start, not a repo file",
	`~/.burnmon`:                           "runtime folder in the user's home (forward slash spelling), not a repo file",
	`dist\burnmon-cloud-<version>.plugin`:  "build output of tools\\burnmon-cloud\\build.ps1, not committed; <version> is a placeholder",
	`%LOCALAPPDATA%\burnmon\burnmon.db`:    "runtime store file, created on first run, not a repo file",
	`~/.config/burnmon/burnmon.db`:         "runtime store file on Linux, created on first run, not a repo file",
	`%USERPROFILE%\.burnmon\snapshot.json`: "runtime snapshot file written by the snapshot writer, not a repo file",
	`%USERPROFILE%\.burnmon\snapshot.lock`: "runtime writer lock file, created by internal\\snapshotwriter at run time, not a repo file",
	`%LOCALAPPDATA%\burnmon\burnmon.json`:  "optional user config file, never in the repo",
	`%HERMES_HOME%\state.db`:               "Hermes data file on the user's computer, outside the repo",
	`%LOCALAPPDATA%\Hermes\state.db`:       "Hermes data file on the user's computer, outside the repo",
	`$HERMES_HOME/state.db`:                "Hermes data file on the user's computer, outside the repo",
	`~/.hermes/state.db`:                   "Hermes data file on the user's computer, outside the repo",
	`%COPILOT_HOME%\session-store.db`:      "Copilot CLI data file on the user's computer, outside the repo",
	`~/.copilot/session-store.db`:          "Copilot CLI data file on the user's computer, outside the repo",
	`sprints\<project>\<slug>\sprint.json`: "Foreman state file under foreman_root, outside the repo",
	`flows\<project>\<flow>\flow.json`:     "Foreman state file under foreman_root, outside the repo",
	`flows\_lib\flow_transition.py`:        "Foreman script under foreman_root, outside the repo",

	// STATUS.md points at the sprint report that holds the B8 evidence.
	`C:\ZND\10_holding\sprints\burnmon\burnmon-b8-agent-half\report.md`: "B8 sprint report in the owner's Foreman tree, outside the repo",
}

func repoDocs(t *testing.T, root string) map[string]string {
	t.Helper()
	docs := map[string]string{}
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(b)
	}
	for _, rel := range trackedDocs(root) {
		switch rel {
		case "STATUS.md":
			docs["STATUS.md (first section)"] = StatusTop(read(rel))
		case "SESSION_LOG.md":
			// the log is history, only the em dash test reads it
		default:
			docs[rel] = read(rel)
		}
	}
	return docs
}

// trackedDocNames are the repo docs the checks read: an explicit list, never a
// glob of docs\*.md, because the working tree of the main checkout holds
// untracked notes there that are not part of the repo.
var trackedDocNames = []string{
	"README.md",
	"STATUS.md",
	"SESSION_LOG.md",
	"tools/burnmon-cloud/plugin/README.md",
	"docs/snapshot_schema.md",
}

// trackedDocs returns the listed docs that exist under root.
func trackedDocs(root string) []string {
	var out []string
	for _, rel := range trackedDocNames {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			out = append(out, rel)
		}
	}
	return out
}

func TestTrackedDocsSkipsStrayAndMissingFiles(t *testing.T) {
	root := t.TempDir()
	for rel, body := range map[string]string{
		"README.md":              "x\n",
		"docs/relation_graph.md": "untracked note\n",
		"docs/old_prompt.md":     "untracked prompt\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := trackedDocs(root)
	if len(got) != 1 || got[0] != "README.md" {
		t.Errorf("trackedDocs = %v, want only README.md (stray docs\\*.md and missing files skipped)", got)
	}
}

func report(t *testing.T, fs []Finding) {
	t.Helper()
	for _, f := range fs {
		t.Errorf("%s:%d [%s] %s: %s", f.Doc, f.Line, f.Kind, f.Text, f.Why)
	}
}

func TestDocCheckPassesOnRepoDocs(t *testing.T) {
	root, c := realRoot(t)
	docs := repoDocs(t, root)
	for name, doc := range docs {
		report(t, Verify(name, doc, c, allowlist))
	}
	report(t, Reverse(docs["README.md"], c))
	report(t, CheckAllowlist(docs, allowlist))
}

// reverseOf runs Reverse on the README and keeps one kind.
func reverseOf(t *testing.T, kind string) []Finding {
	root, c := realRoot(t)
	var out []Finding
	for _, f := range Reverse(repoDocs(t, root)["README.md"], c) {
		if f.Kind == kind {
			out = append(out, f)
		}
	}
	return out
}

func TestEveryKeydownKeyIsInReadme(t *testing.T) {
	report(t, reverseOf(t, "hotkey"))
}

func TestEveryCliSubcommandIsInReadme(t *testing.T) {
	report(t, reverseOf(t, "subcommand"))
}

func TestEveryPartIDIsInReadme(t *testing.T) {
	report(t, reverseOf(t, "section"))
}

func TestNewConfigKeysAreInReadme(t *testing.T) {
	report(t, reverseOf(t, "key"))
}

func TestStatusTopStopsAtFirstHistoryParagraph(t *testing.T) {
	s := "# T\n\nWhat is true now: a.\nmore.\n\nWhat is true at old: b.\n\nWhat was true at older: c.\n"
	got := StatusTop(s)
	if !strings.Contains(got, "a.") || strings.Contains(got, "b.") || strings.Contains(got, "c.") {
		t.Errorf("StatusTop = %q", got)
	}
}

func TestNoEmDashInChangedDocs(t *testing.T) {
	root, err := FindRoot()
	if err != nil {
		t.Fatal(err)
	}
	dash := string(rune(0x2014))
	for _, f := range trackedDocs(root) {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, dash) {
				t.Errorf("%s", fmt.Sprintf("%s:%d contains an em dash", f, i+1))
			}
		}
	}
}

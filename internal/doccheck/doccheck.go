// Package doccheck checks documents against the code. It pulls claims out of
// markdown (backticked config keys, paths, subcommands, flags, bindings,
// uicheck ids, section ids and keyboard keys) and verifies each one against a
// corpus read from the repo's own source files. Reverse checks the other
// direction: what the code has and the README must name. Standard library
// plus internal/sections only.
package doccheck

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"burnmon/internal/sections"
)

// Claim is one checkable statement found in a document.
// Kind: key, path, subcommand, flag, binding, uicheck, section, hotkey.
type Claim struct {
	Kind string
	Text string
	Line int
}

// Finding is a claim (or a missing mention) that failed.
type Finding struct {
	Doc  string
	Line int
	Kind string
	Text string
	Why  string
}

// Corpus is what the code has, by class.
type Corpus struct {
	Root        string
	Keys        map[string]bool // snake_case words found right after a quote
	Subcommands map[string]bool
	Flags       map[string]bool
	Bindings    map[string]bool
	Hotkeys     map[string]bool
	UICheck     map[string]bool
	PartIDs     map[string]bool
	GroupIDs    map[string]bool
}

// RequiredConfigKeys are the config keys B1 to B4 added; the README must name each.
var RequiredConfigKeys = []string{"keep_history", "foreman_root", "agent_view", "claude_path"}

const bt = "`"

var (
	spanRe     = regexp.MustCompile(bt + "([^" + bt + "\n]+)" + bt)
	cliRe      = regexp.MustCompile(`burnmon-cli(?:\.exe)?\s+([a-z][a-z-]*)`)
	bdevRe     = regexp.MustCompile(`^bdev[A-Z]\w*$`)
	uicheckRe  = regexp.MustCompile(`^[dw]\d+$`)
	keyRe      = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)+$`)
	extRe      = regexp.MustCompile(`\.[A-Za-z0-9]{1,8}$`)
	hotkeyRe   = regexp.MustCompile(`^\s*[-*]\s+\*\*([A-Za-z0-9])\*\*`)
	firstCell  = regexp.MustCompile("^" + bt + "([^" + bt + "]+)" + bt + "$")
	driveRe    = regexp.MustCompile(`^[A-Za-z]:`)
	srcKeyRe   = regexp.MustCompile(`["'` + bt + `]([a-z][a-z0-9]*(?:_[a-z0-9]+)+)`)
	subcmdRe   = regexp.MustCompile(`os\.Args\[1\] == "([a-z-]+)"`)
	flagRe     = regexp.MustCompile(`\b(?:fs|flag)\.(?:String|Bool|Int|Int64|Uint|Duration|Float64)\(\s*"([\w-]+)"`)
	flagVarRe  = regexp.MustCompile(`\bfs\.Var\([^,]+,\s*"([\w-]+)"`)
	bindRe     = regexp.MustCompile(`\.Bind\(\s*"(\w+)"`)
	keydownRe  = regexp.MustCompile(`k === '([a-z0-9])'`)
	uicheckFnm = regexp.MustCompile(`^check_([dw]\d+)\.go$`)
	caseFuncRe = regexp.MustCompile(`(?m)^func caseD(\d+)\(`)
	topFolders = []string{"cmd", "internal", "tools", "scripts", "testdata", "assets", "docs", "02_roadmap", "04_assets"}
)

// Extract returns the claims of a markdown document. Fenced blocks yield
// claims only when the line before the fence contains "runs in".
func Extract(doc string) []Claim {
	var out []Claim
	inFence, fenceOK, inSection := false, false, false
	prev := ""
	for i, line := range strings.Split(strings.ReplaceAll(doc, "\r\n", "\n"), "\n") {
		n := i + 1
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if !inFence {
				inFence = true
				fenceOK = strings.Contains(strings.ToLower(prev), "runs in")
			} else {
				inFence = false
			}
			prev = line
			continue
		}
		prev = line
		if inFence {
			if fenceOK {
				out = append(out, spanClaims(trimmed, n)...)
			}
			continue
		}
		rest := line
		if strings.HasPrefix(trimmed, "|") {
			cells := tableCells(trimmed)
			if len(cells) >= 2 && strings.EqualFold(cells[0], "id") && strings.EqualFold(cells[1], "label") {
				inSection = true
				continue
			}
			if inSection && len(cells) > 0 {
				if m := firstCell.FindStringSubmatch(cells[0]); m != nil {
					out = append(out, Claim{"section", m[1], n})
					rest = strings.Join(cells[1:], " | ")
				}
			}
		} else {
			inSection = false
		}
		if m := hotkeyRe.FindStringSubmatch(line); m != nil {
			out = append(out, Claim{"hotkey", strings.ToLower(m[1]), n})
		}
		for _, m := range spanRe.FindAllStringSubmatch(rest, -1) {
			out = append(out, spanClaims(strings.TrimSpace(m[1]), n)...)
		}
	}
	return out
}

func tableCells(row string) []string {
	row = strings.TrimSuffix(strings.TrimPrefix(row, "|"), "|")
	cells := strings.Split(row, "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

// spanClaims classifies one code span (or one command line in a fenced block).
func spanClaims(s string, line int) []Claim {
	if m := cliRe.FindStringSubmatch(s); m != nil {
		out := []Claim{{"subcommand", m[1], line}}
		for _, tok := range strings.Fields(s) {
			if len(tok) > 1 && tok[0] == '-' {
				name := strings.TrimLeft(tok, "-")
				if i := strings.Index(name, "="); i >= 0 {
					name = name[:i]
				}
				if name != "" && name[0] >= 'a' && name[0] <= 'z' {
					out = append(out, Claim{"flag", name, line})
				}
			}
		}
		return out
	}
	switch {
	case bdevRe.MatchString(s):
		return []Claim{{"binding", s, line}}
	case uicheckRe.MatchString(s):
		return []Claim{{"uicheck", s, line}}
	case keyRe.MatchString(s):
		return []Claim{{"key", s, line}}
	case isPath(s):
		return []Claim{{"path", s, line}}
	}
	return nil
}

func isPath(s string) bool {
	if strings.ContainsAny(s, " \t") || strings.Contains(s, "://") {
		return false
	}
	norm := strings.ReplaceAll(s, "\\", "/")
	if !strings.Contains(norm, "/") {
		return false
	}
	for _, f := range topFolders {
		if strings.HasPrefix(norm, f+"/") {
			return true
		}
	}
	last := norm[strings.LastIndex(norm, "/")+1:]
	return extRe.MatchString(last)
}

func isRuntimePath(s string) bool {
	return strings.ContainsAny(s[:1], "%~$") || driveRe.MatchString(s) || strings.Contains(s, "<")
}

// Verify checks every claim of doc against the corpus. A claim whose text is
// in allow is skipped (CheckAllowlist guards the entries themselves).
func Verify(docName, doc string, c Corpus, allow map[string]string) []Finding {
	var out []Finding
	for _, cl := range Extract(doc) {
		if _, ok := allow[cl.Text]; ok {
			continue
		}
		why := ""
		switch cl.Kind {
		case "key":
			if !c.Keys[cl.Text] {
				why = "no quoted string or json tag with this name in the code"
			}
		case "path":
			why = checkPath(c.Root, cl.Text)
		case "subcommand":
			if !c.Subcommands[cl.Text] {
				why = "no os.Args[1] case for this subcommand in cmd\\burnmon-cli\\main.go"
			}
		case "flag":
			if !c.Flags[cl.Text] {
				why = "no flag with this name in the commands"
			}
		case "binding":
			if !c.Bindings[cl.Text] {
				why = "no Bind call with this name"
			}
		case "uicheck":
			if !c.UICheck[cl.Text] {
				why = "no tools\\uicheck\\check_" + cl.Text + ".go and no case for it in a group_*.go file"
			}
		case "section":
			if !c.PartIDs[cl.Text] && !c.GroupIDs[cl.Text] {
				why = "not a part or group id in internal\\sections"
			}
		case "hotkey":
			if !c.Hotkeys[cl.Text] {
				why = "no k === '" + cl.Text + "' in cmd\\burnmon-dev\\page.html"
			}
		}
		if why != "" {
			out = append(out, Finding{docName, cl.Line, cl.Kind, cl.Text, why})
		}
	}
	return out
}

func checkPath(root, s string) string {
	if isRuntimePath(s) {
		return "runtime path: needs an allowlist entry with a reason"
	}
	rel := filepath.FromSlash(strings.TrimRight(strings.ReplaceAll(s, "\\", "/"), "/"))
	p := filepath.Join(root, rel)
	if strings.Contains(rel, "*") {
		if m, _ := filepath.Glob(p); len(m) > 0 {
			return ""
		}
		return "no file matches this pattern"
	}
	if _, err := os.Stat(p); err != nil {
		return "no such file"
	}
	return ""
}

// CheckAllowlist reports entries without a reason and entries that no
// document claim uses any more.
func CheckAllowlist(docs map[string]string, allow map[string]string) []Finding {
	used := map[string]bool{}
	for _, d := range docs {
		for _, cl := range Extract(d) {
			used[cl.Text] = true
		}
	}
	var keys []string
	for k := range allow {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []Finding
	for _, k := range keys {
		if strings.TrimSpace(allow[k]) == "" {
			out = append(out, Finding{"allowlist", 0, "allow", k, "entry has no reason"})
		}
		if !used[k] {
			out = append(out, Finding{"allowlist", 0, "allow", k, "no document claim uses this entry any more"})
		}
	}
	return out
}

// Reverse reports what the code has and the README does not name: CLI
// subcommands, keyboard keys, part ids and the config keys of RequiredConfigKeys.
func Reverse(readme string, c Corpus) []Finding {
	named := map[string]map[string]bool{}
	for _, cl := range Extract(readme) {
		if named[cl.Kind] == nil {
			named[cl.Kind] = map[string]bool{}
		}
		named[cl.Kind][cl.Text] = true
	}
	var out []Finding
	add := func(kind string, have map[string]bool, why string) {
		var names []string
		for k := range have {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			if !named[kind][k] {
				out = append(out, Finding{"README.md", 0, kind, k, why})
			}
		}
	}
	add("subcommand", c.Subcommands, "the CLI has this subcommand; the README has no burnmon-cli example for it")
	add("hotkey", c.Hotkeys, "the page handles this key; the README has no key bullet for it")
	add("section", c.PartIDs, "a part id in internal\\sections; the README section table lacks it")
	keys := map[string]bool{}
	for _, k := range RequiredConfigKeys {
		keys[k] = true
	}
	add("key", keys, "a config key added in B1 to B4; the README does not name it")
	return out
}

// StatusTop returns the live part of STATUS.md: everything before the first
// "What was true" paragraph and before the second "What is true" paragraph.
// Older paragraphs are history and may name things that no longer exist.
func StatusTop(status string) string {
	var keep []string
	seenIs := false
	for _, p := range strings.Split(strings.ReplaceAll(status, "\r\n", "\n"), "\n\n") {
		if strings.HasPrefix(p, "What was true") {
			break
		}
		if strings.HasPrefix(p, "What is true") {
			if seenIs {
				break
			}
			seenIs = true
		}
		keep = append(keep, p)
	}
	return strings.Join(keep, "\n\n")
}

// FindRoot walks up from the working directory to the folder holding go.mod.
func FindRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("doccheck: no go.mod above the working directory")
		}
		dir = parent
	}
}

func skipDir(rel string) bool {
	switch rel {
	case ".git", ".worktrees", "docs", "reports", "_log", "node_modules", "graphify-out",
		"internal/doccheck", "testdata/uicheck/out":
		return true
	}
	return false
}

var sourceExt = map[string]bool{".go": true, ".html": true, ".js": true, ".py": true, ".json": true, ".ps1": true}

func collect(re *regexp.Regexp, src string, into map[string]bool) {
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		into[m[1]] = true
	}
}

func goFiles(root string, dirs ...string) ([]string, error) {
	var out []string
	for _, d := range dirs {
		m, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(d), "*.go"))
		if err != nil {
			return nil, err
		}
		for _, f := range m {
			if !strings.HasSuffix(f, "_test.go") {
				out = append(out, f)
			}
		}
	}
	return out, nil
}

// LoadCorpus reads the repo's source files into a Corpus. An unreadable or
// wrong root is an error.
func LoadCorpus(root string) (Corpus, error) {
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return Corpus{}, fmt.Errorf("doccheck: root %s is not a repo root: %w", root, err)
	}
	c := Corpus{Root: root, Keys: map[string]bool{}, Subcommands: map[string]bool{}, Flags: map[string]bool{},
		Bindings: map[string]bool{}, Hotkeys: map[string]bool{}, UICheck: map[string]bool{},
		PartIDs: map[string]bool{}, GroupIDs: map[string]bool{}}

	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if skipDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if !sourceExt[filepath.Ext(p)] {
			return nil
		}
		if info, err := d.Info(); err != nil || info.Size() > 4<<20 {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		collect(srcKeyRe, string(b), c.Keys)
		return nil
	})
	if err != nil {
		return Corpus{}, err
	}

	cli, err := os.ReadFile(filepath.Join(root, "cmd", "burnmon-cli", "main.go"))
	if err != nil {
		return Corpus{}, err
	}
	collect(subcmdRe, string(cli), c.Subcommands)

	files, err := goFiles(root, "cmd/burnmon-cli", "cmd/burnmon", "cmd/burnmon-dev")
	if err != nil {
		return Corpus{}, err
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return Corpus{}, err
		}
		collect(flagRe, string(b), c.Flags)
		collect(flagVarRe, string(b), c.Flags)
		if !strings.Contains(filepath.ToSlash(f), "cmd/burnmon-cli/") {
			collect(bindRe, string(b), c.Bindings)
		}
	}

	page, err := os.ReadFile(filepath.Join(root, "cmd", "burnmon-dev", "page.html"))
	if err != nil {
		return Corpus{}, err
	}
	collect(keydownRe, string(page), c.Hotkeys)

	entries, err := os.ReadDir(filepath.Join(root, "tools", "uicheck"))
	if err != nil {
		return Corpus{}, err
	}
	for _, e := range entries {
		if m := uicheckFnm.FindStringSubmatch(e.Name()); m != nil {
			c.UICheck[m[1]] = true
		}
		// B7.2 moved the d checks into group files (group_layout_1.go and so
		// on), one func caseDNN per old check_dNN.go.
		if strings.HasPrefix(e.Name(), "group_") && strings.HasSuffix(e.Name(), ".go") {
			b, err := os.ReadFile(filepath.Join(root, "tools", "uicheck", e.Name()))
			if err != nil {
				return Corpus{}, err
			}
			for _, m := range caseFuncRe.FindAllStringSubmatch(string(b), -1) {
				c.UICheck["d"+m[1]] = true
			}
		}
	}
	for _, id := range sections.IDs {
		c.PartIDs[id] = true
	}
	for _, g := range sections.Groups {
		c.GroupIDs[g.ID] = true
	}
	return c, nil
}

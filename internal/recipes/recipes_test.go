package recipes

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const snipPath = "../../../groundwork/plugins/groundwork/engine/snip/snip.py"

const tick = "```"

func tools() Tools {
	return Tools{Git: "/x/git", Python: "/x/python3"}
}

// TestRecipesMatchSnip fails when the Go copy drifts from snip.py. It skips
// when the Groundwork repo is not beside this one.
func TestRecipesMatchSnip(t *testing.T) {
	raw, err := os.ReadFile(snipPath)
	if err != nil {
		t.Skip("snip.py not found: " + snipPath)
	}
	src := string(raw)

	m := regexp.MustCompile(`RECIPES = \(([^)]*)\)`).FindStringSubmatch(src)
	if m == nil {
		t.Fatal("RECIPES not found in snip.py")
	}
	var names []string
	for _, q := range regexp.MustCompile(`"(\w+)"`).FindAllStringSubmatch(m[1], -1) {
		names = append(names, q[1])
	}
	if strings.Join(names, ",") != strings.Join(Names, ",") {
		t.Errorf("recipe names differ: snip.py %v, Go %v", names, Names)
	}

	dict := func(name string) map[string]string {
		blk := regexp.MustCompile(name + ` = \{([^}]*)\}`).FindStringSubmatch(src)
		if blk == nil {
			t.Fatalf("%s not found in snip.py", name)
		}
		out := map[string]string{}
		for _, kv := range regexp.MustCompile(`"(\w+)":\s*"([^"]*)"`).FindAllStringSubmatch(blk[1], -1) {
			out[kv[1]] = kv[2]
		}
		return out
	}
	exp := dict("DEFAULT_EXPECT")
	for k, v := range exp {
		if DefaultExpect[k] != v {
			t.Errorf("DEFAULT_EXPECT %s differs: snip.py %q, Go %q", k, v, DefaultExpect[k])
		}
	}
	if len(exp) != len(DefaultExpect) {
		t.Errorf("DEFAULT_EXPECT size differs: snip.py %d, Go %d", len(exp), len(DefaultExpect))
	}
	runs := dict("DEFAULT_RUNS_IN")
	for k, v := range runs {
		if DefaultRunsIn[k] != v {
			t.Errorf("DEFAULT_RUNS_IN %s differs: snip.py %q, Go %q", k, v, DefaultRunsIn[k])
		}
	}
	if !strings.Contains(src, `DEFAULT_NEEDS = "`+DefaultNeeds+`"`) {
		t.Error("DEFAULT_NEEDS differs")
	}
	for _, lit := range []string{recipePrefix, expectPrefix, headerRunsIn, headerCwd, headerNeeds} {
		if !strings.Contains(src, `"`+lit+`"`) {
			t.Errorf("header text %q not found in snip.py", lit)
		}
	}
	for _, lit := range BodyLiterals {
		if !strings.Contains(src, `"`+lit+`"`) {
			t.Errorf("command text %q not found in snip.py", lit)
		}
	}
	for _, lit := range []string{`" land --apply --repo "`, `"py -3 "`, `"python3 "`, `"Set-Location -LiteralPath "`, `'cd -- "$HOME/'`} {
		if !strings.Contains(src, lit) {
			t.Errorf("text %s not found in snip.py", lit)
		}
	}
}

func block(os, cwd, recipe, body, expect string) string {
	fence, runs, set := "powershell", DefaultRunsIn["win"], "Set-Location -LiteralPath '"+cwd+"'"
	if os == "mac" {
		fence, runs, set = "sh", DefaultRunsIn["mac"], "cd -- '"+cwd+"'"
	}
	return tick + fence + "\n# runs in: " + runs + "\n# cwd: " + cwd + "\n# needs: nothing\n# recipe: git " + recipe + "\n" + set + "\n" + body + "# expect: " + expect + "\n" + tick + "\n"
}

// TestRecipesGolden holds the blocks hand-derived from snip.py, plus the golden
// files of testdata when Wilco has made them with snip.py itself.
func TestRecipesGolden(t *testing.T) {
	cases := []struct {
		name string
		req  Request
		want string
	}{
		{"share win", Request{Recipe: "share", OS: "win", Cwd: `C:\w\r`},
			block("win", `C:\w\r`, "share", "git push -u origin HEAD\ngit status -sb\n", DefaultExpect["share"])},
		{"sync mac", Request{Recipe: "sync", OS: "mac", Cwd: "/w/r"},
			block("mac", "/w/r", "sync", "git fetch --prune origin\ngit merge --ff-only '@{u}'\ngit status -sb\n", DefaultExpect["sync"])},
		{"tag win", Request{Recipe: "tag", OS: "win", Cwd: `C:\w\r`, Tag: "v1.0", Message: "it's v1"},
			block("win", `C:\w\r`, "tag", "git tag -a 'v1.0' -m 'it''s v1'\ngit push origin 'v1.0'\ngit ls-remote --tags origin 'v1.0'\n", "git ls-remote prints one line ending in refs/tags/v1.0")},
		{"save mac", Request{Recipe: "save", OS: "mac", Cwd: "/w/r", Files: []string{"a b.txt", "c.go"}, Message: "fix it", Body: "why", CoAuthor: "A B <a@b.c>"},
			block("mac", "/w/r", "save", "git add -- 'a b.txt' 'c.go'\ngit commit -m 'fix it' -m 'why' -m 'Co-Authored-By: A B <a@b.c>'\ngit status --short\n", DefaultExpect["save"])},
		{"land win", Request{Recipe: "land", OS: "win", Cwd: `C:\w\r`, Engine: `C:\g\git_flow.py`, Subject: "ship"},
			block("win", `C:\w\r`, "land", "py -3 'C:\\g\\git_flow.py' land --apply --repo 'C:\\w\\r' --subject 'ship'\n", DefaultExpect["land"])},
		{"land mac home", Request{Recipe: "land", OS: "mac", Cwd: "~/r", Engine: "/g/git_flow.py"},
			strings.Replace(block("mac", "~/r", "land", "python3 '/g/git_flow.py' land --apply --repo \"$HOME/r\"\n", DefaultExpect["land"]), "cd -- '~/r'", `cd -- "$HOME/r"`, 1)},
	}
	for _, c := range cases {
		p, err := Build(c.req, tools())
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if p.Block != c.want {
			t.Errorf("%s: block differs\n got: %q\nwant: %q", c.name, p.Block, c.want)
		}
	}

	files, _ := filepath.Glob("testdata/*_*.txt")
	if len(files) == 0 {
		t.Log("golden files missing: testdata is made from snip.py by Wilco")
	}
	for _, f := range files {
		want, _ := os.ReadFile(f)
		base := strings.TrimSuffix(filepath.Base(f), ".txt")
		i := strings.LastIndex(base, "_")
		req, ok := goldenRequest(base[:i], base[i+1:])
		if !ok {
			t.Errorf("%s: unknown golden name", f)
			continue
		}
		p, err := Build(req, tools())
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if p.Block != string(want) {
			t.Errorf("%s differs from Build", f)
		}
	}
}

// goldenRequest is the fixed call a golden file was made with; see testdata/README.txt.
func goldenRequest(recipe, osn string) (Request, bool) {
	cwd := map[string]string{"win": `C:\w\r`, "mac": "/w/r"}[osn]
	engine := map[string]string{"win": `C:\g\git_flow.py`, "mac": "/g/git_flow.py"}[osn]
	r := Request{Recipe: recipe, OS: osn, Cwd: cwd, Engine: engine, Files: []string{"a.txt"}, Message: "msg", Tag: "v1", Subject: "s"}
	return r, cwd != ""
}

func TestRecipesRefuse(t *testing.T) {
	save := func(files []string, msg string) Request {
		return Request{Recipe: "save", OS: "mac", Cwd: "/w/r", Files: files, Message: msg}
	}
	cases := []struct {
		name string
		req  Request
		want string
	}{
		{"dot", save([]string{"."}, "m"), "list each file"},
		{"star", save([]string{"*"}, "m"), "list each file"},
		{"-A", save([]string{"-A"}, "m"), "list each file"},
		{"--all", save([]string{"--all"}, "m"), "list each file"},
		{"absolute", save([]string{"/etc/x"}, "m"), "relative to the cwd"},
		{"win absolute", save([]string{`C:\x`}, "m"), "relative to the cwd"},
		{"long message", save([]string{"a"}, strings.Repeat("x", 73)), "longer than 72"},
		{"period", save([]string{"a"}, "done."), "ends with a period"},
		{"no files", save(nil, "m"), "--files is required"},
		{"em dash", save([]string{"a"}, "a\u2014b"), "em dash"},
		{"line break", save([]string{"a"}, "a\nb"), "line break"},
		{"fence", save([]string{"a"}, "a```b"), "three or more backticks"},
		{"bad co-author", Request{Recipe: "save", OS: "mac", Cwd: "/w/r", Files: []string{"a"}, Message: "m", CoAuthor: "nobody"}, "Name <email>"},
		{"relative cwd", Request{Recipe: "sync", OS: "mac", Cwd: "w/r"}, "must start with"},
		{"win cwd on mac", Request{Recipe: "sync", OS: "win", Cwd: "/w/r"}, "Windows path"},
		{"tag dash", Request{Recipe: "tag", OS: "mac", Cwd: "/w/r", Tag: "-x", Message: "m"}, "must not start with"},
		{"tag no message", Request{Recipe: "tag", OS: "mac", Cwd: "/w/r", Tag: "v1"}, "--message is empty"},
		{"land no engine", Request{Recipe: "land", OS: "mac", Cwd: "/w/r"}, "groundwork_engine"},
		{"land relative engine", Request{Recipe: "land", OS: "mac", Cwd: "/w/r", Engine: "git_flow.py"}, "absolute path"},
		{"unknown", Request{Recipe: "rebase", OS: "mac", Cwd: "/w/r"}, "unknown recipe"},
	}
	for _, c := range cases {
		_, err := Build(c.req, tools())
		if err == nil {
			t.Errorf("%s: no refusal", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: refusal %q lacks %q", c.name, err, c.want)
		}
	}
	// 72 characters is allowed.
	if _, err := Build(save([]string{"a"}, strings.Repeat("x", 72)), tools()); err != nil {
		t.Errorf("72 characters refused: %v", err)
	}
	// A host-system cwd that does not exist is refused when a check is wired.
	tl := tools()
	tl.DirExists = func(string) bool { return false }
	req := Request{Recipe: "sync", Cwd: t.TempDir()}
	if _, err := Build(req, tl); err == nil || !strings.Contains(err.Error(), "folder not found") {
		t.Errorf("missing folder not refused: %v", err)
	}
}

func TestArgvLines(t *testing.T) {
	p, err := Build(Request{Recipe: "sync", OS: "mac", Cwd: "/w/r"}, tools())
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"/x/git", "fetch", "--prune", "origin"},
		{"/x/git", "merge", "--ff-only", "@{u}"},
		{"/x/git", "status", "-sb"},
	}
	if len(p.Argv) != len(want) {
		t.Fatalf("argv %v", p.Argv)
	}
	for i := range want {
		if strings.Join(p.Argv[i], "|") != strings.Join(want[i], "|") {
			t.Errorf("line %d: %v, want %v", i, p.Argv[i], want[i])
		}
	}
	l, err := Build(Request{Recipe: "land", OS: "mac", Cwd: "/w/r", Engine: "/g/git_flow.py", Subject: "s"}, tools())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(l.Argv[0], "|"); got != "/x/python3|/g/git_flow.py|land|--apply|--repo|/w/r|--subject|s" {
		t.Errorf("land argv %s", got)
	}
}

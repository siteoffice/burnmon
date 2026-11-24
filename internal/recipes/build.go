package recipes

import "strings"

// Build checks one recipe call and returns its block and argv lines. The
// checks and the texts are those of snip.py v0.7.0, with one addition: a tag
// that starts with "-" is refused, because the argv line would read it as a flag.
func Build(r Request, t Tools) (Plan, error) {
	target := r.OS
	if target == "" {
		target = HostOS
	}
	if target != "win" && target != "mac" {
		return Plan{}, refuse("--os must be win or mac")
	}
	known := false
	for _, n := range Names {
		known = known || n == r.Recipe
	}
	if !known {
		return Plan{}, refuse("unknown recipe: %s", r.Recipe)
	}
	if t.Git == "" {
		return Plan{}, refuse("git was not found on PATH")
	}
	dir, err := checkCwd(target, r.Cwd, t)
	if err != nil {
		return Plan{}, err
	}
	expect := r.Expect
	if expect == "" {
		expect = DefaultExpect[r.Recipe]
	}
	git := func(a ...string) []string { return append([]string{t.Git}, a...) }
	q := func(v string) string { return quote(target, v) }

	var body []string
	var argv [][]string
	switch r.Recipe {
	case "save":
		if body, argv, err = saveLines(target, r, git); err != nil {
			return Plan{}, err
		}
	case "share":
		body = []string{"git push -u origin HEAD", "git status -sb"}
		argv = [][]string{git("push", "-u", "origin", "HEAD"), git("status", "-sb")}
	case "tag":
		if err := oneLine("--tag", r.Tag); err != nil {
			return Plan{}, err
		}
		if err := oneLine("--message", r.Message); err != nil {
			return Plan{}, err
		}
		if strings.HasPrefix(r.Tag, "-") {
			return Plan{}, refuse("--tag must not start with a dash")
		}
		body = []string{"git tag -a " + q(r.Tag) + " -m " + q(r.Message),
			"git push origin " + q(r.Tag),
			"git ls-remote --tags origin " + q(r.Tag)}
		argv = [][]string{git("tag", "-a", r.Tag, "-m", r.Message), git("push", "origin", r.Tag),
			git("ls-remote", "--tags", "origin", r.Tag)}
		if r.Expect == "" {
			expect = strings.Replace(DefaultExpect["tag"], "%s", r.Tag, 1)
		}
	case "sync":
		body = []string{"git fetch --prune origin", "git merge --ff-only " + q("@{u}"), "git status -sb"}
		argv = [][]string{git("fetch", "--prune", "origin"), git("merge", "--ff-only", "@{u}"), git("status", "-sb")}
	case "land":
		if body, argv, err = landLines(target, r, t, dir); err != nil {
			return Plan{}, err
		}
	}
	p, err := commandBlock(target, r, r.Recipe, body, expect, dir)
	if err != nil {
		return Plan{}, err
	}
	p.Argv = argv
	return p, nil
}

func saveLines(target string, r Request, git func(...string) []string) ([]string, [][]string, error) {
	if len(r.Files) == 0 {
		return nil, nil, refuse("--files is required")
	}
	q := func(v string) string { return quote(target, v) }
	var qfiles []string
	for _, f := range r.Files {
		if err := oneLine("--files", f); err != nil {
			return nil, nil, err
		}
		switch f {
		case ".", "*", "-A", "--all":
			return nil, nil, refuse("--files names %s: list each file", f)
		}
		if strings.HasPrefix(f, "/") || strings.HasPrefix(f, "~/") || winAbsRe.MatchString(f) {
			return nil, nil, refuse("--files must be relative to the cwd: %s", f)
		}
		qfiles = append(qfiles, q(f))
	}
	if err := oneLine("--message", r.Message); err != nil {
		return nil, nil, err
	}
	if utf8Len(r.Message) > 72 {
		return nil, nil, refuse("--message is longer than 72 characters")
	}
	if strings.HasSuffix(r.Message, ".") {
		return nil, nil, refuse("--message ends with a period")
	}
	commit := "git commit -m " + q(r.Message)
	cargv := git("commit", "-m", r.Message)
	if r.Body != "" {
		if err := oneLine("--body", r.Body); err != nil {
			return nil, nil, err
		}
		commit += " -m " + q(r.Body)
		cargv = append(cargv, "-m", r.Body)
	}
	if r.CoAuthor != "" {
		if err := oneLine("--co-author", r.CoAuthor); err != nil {
			return nil, nil, err
		}
		if !coAuthorRe.MatchString(r.CoAuthor) {
			return nil, nil, refuse("--co-author must look like: Name <email>")
		}
		commit += " -m " + q("Co-Authored-By: "+r.CoAuthor)
		cargv = append(cargv, "-m", "Co-Authored-By: "+r.CoAuthor)
	}
	body := []string{"git add -- " + strings.Join(qfiles, " "), commit, "git status --short"}
	argv := [][]string{git(append([]string{"add", "--"}, r.Files...)...), cargv, git("status", "--short")}
	return body, argv, nil
}

func landLines(target string, r Request, t Tools, dir string) ([]string, [][]string, error) {
	if r.Engine == "" {
		return nil, nil, refuse("land needs groundwork_engine in burnmon.json (the full path of git_flow.py)")
	}
	if err := oneLine("--engine", r.Engine); err != nil {
		return nil, nil, err
	}
	abs := strings.HasPrefix(r.Engine, "/")
	if target == "win" {
		abs = winAbsRe.MatchString(r.Engine)
	}
	if !abs {
		return nil, nil, refuse("--engine must be an absolute path for --os %s", target)
	}
	if t.Python == "" {
		return nil, nil, refuse("land needs python on PATH")
	}
	repo := quote(target, r.Cwd)
	if strings.HasPrefix(r.Cwd, "~/") {
		repo = `"$HOME/` + r.Cwd[2:] + `"`
	}
	py := "python3 "
	if target == "win" {
		py = "py -3 "
	}
	line := py + quote(target, r.Engine) + " land --apply --repo " + repo
	repoDir := r.Cwd
	if dir != "" {
		repoDir = dir
	}
	args := []string{r.Engine, "land", "--apply", "--repo", repoDir}
	if r.Subject != "" {
		if err := oneLine("--subject", r.Subject); err != nil {
			return nil, nil, err
		}
		line += " --subject " + quote(target, r.Subject)
		args = append(args, "--subject", r.Subject)
	}
	return []string{line}, [][]string{append([]string{t.Python}, args...)}, nil
}
